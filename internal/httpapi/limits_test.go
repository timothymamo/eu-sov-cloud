package httpapi_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/timothymamo/eu-sov-cloud/internal/httpapi"
)

// settle is how long a test waits for an answer before it calls the handler
// broken: long enough that a slow machine never trips it, short enough that a
// handler that queues instead of refusing fails the test rather than hanging it.
const settle = 10 * time.Second

// TestConcurrencyCap pins the assessment cap: a second assessment while the
// first holds the only slot is refused at once with 503 busy and Retry-After: 1,
// never queued, and once the slot is free the next assessment runs. The health
// and target routes are not limited by the cap.
func TestConcurrencyCap(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	gate := make(chan struct{})
	started := make(chan struct{})
	h.jsonReporter.gate = gate
	h.jsonReporter.started = started

	handler := h.handler(t, httpapi.WithMaxConcurrent(1))

	// The first assessment holds the only slot until gate is closed.
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- serve(handler, newAssessRequest(assessBody(t, alphaKey))) }()

	select {
	case <-started:
	case <-time.After(settle):
		t.Fatal("the first assessment never reached the reporter")
	}

	// The second assessment is refused at once, without waiting for the slot.
	busy := serveWithin(t, handler, newAssessRequest(assessBody(t, betaKey)), settle)

	if busy.Code != http.StatusServiceUnavailable {
		t.Fatalf("the second POST /v1/assessments = %d, want %d; body: %q",
			busy.Code, http.StatusServiceUnavailable, busy.Body.String())
	}

	requireHardeningHeaders(t, busy)

	if got := busy.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want %q", got, "1")
	}

	decodeError(t, busy, "busy")

	// The other routes are served while the assessment holds the slot.
	for _, path := range []string{"/v1/targets", "/healthz", "/readyz"} {
		rec := serveWithin(t, handler, newRequest(http.MethodGet, path), settle)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s while an assessment runs = %d, want %d; body: %q",
				path, rec.Code, http.StatusOK, rec.Body.String())
		}
	}

	close(gate)

	select {
	case rec := <-firstDone:
		if rec.Code != http.StatusOK {
			t.Fatalf("the first POST /v1/assessments = %d, want %d; body: %q",
				rec.Code, http.StatusOK, rec.Body.String())
		}
	case <-time.After(settle):
		t.Fatal("the first assessment did not finish after the gate opened")
	}

	// The slot is free again: the next assessment runs.
	third := serveWithin(t, handler, newAssessRequest(assessBody(t, alphaKey)), settle)

	if third.Code != http.StatusOK {
		t.Fatalf("the third POST /v1/assessments = %d, want %d; body: %q",
			third.Code, http.StatusOK, third.Body.String())
	}
}

// TestBodyReadTimeoutReleasesTheSlot pins the timeout on reading a request
// body: a client that announces a body and then stalls neither holds the
// assessment slot nor the server forever. It reads and then stops, the handler
// ends the read at the deadline and answers, and the next client's assessment
// runs, so a stalled request costs one timeout and not the whole API.
//
// The test uses a real connection, because a read deadline lives on the
// connection of the server, not on a recorded request.
func TestBodyReadTimeoutReleasesTheSlot(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	handler := h.handler(t,
		httpapi.WithMaxConcurrent(1),
		httpapi.WithBodyReadTimeout(200*time.Millisecond),
	)

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	addr := strings.TrimPrefix(server.URL, "http://")

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dialing the test server: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	// Client A announces 1000 bytes, sends a few and stalls without closing.
	if _, err := fmt.Fprintf(conn,
		"POST %s HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n%s",
		assessmentsPath, addr, `{"inventory"`); err != nil {
		t.Fatalf("writing the stalled request: %v", err)
	}

	status, closed := stalledAnswer(t, conn)

	if !closed && status != http.StatusBadRequest && status != http.StatusRequestTimeout {
		t.Errorf("the stalled request was answered %d, want the connection closed or a 400/408", status)
	}

	// Waiting for the deadline is the behaviour under test, so client B polls
	// until the slot is free: a handler that keeps it makes the deadline fail.
	deadline := time.Now().Add(settle)

	var last string

	for {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
			server.URL+assessmentsPath, strings.NewReader(assessBody(t, alphaKey)))
		if err != nil {
			t.Fatalf("building client B's request: %v", err)
		}

		req.Header.Set("Content-Type", jsonContentType)

		resp, err := (&http.Client{Timeout: settle}).Do(req)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				return
			}

			last = fmt.Sprintf("%d %q", resp.StatusCode, body)
		} else {
			last = err.Error()
		}

		if time.Now().After(deadline) {
			t.Fatalf("client B was not served within %s after the stalled body's deadline; last answer: %s",
				settle, last)
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// stalledAnswer reads client A's answer within the settle bound and returns its
// status, or reports that the connection was closed instead. It fails the test
// when the server neither answers nor closes within the bound.
func stalledAnswer(t *testing.T, conn net.Conn) (int, bool) {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(settle)); err != nil {
		t.Fatalf("setting the read deadline: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)

	switch {
	case err == nil:
		defer func() { _ = resp.Body.Close() }()

		_, _ = io.Copy(io.Discard, resp.Body)

		return resp.StatusCode, false
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, net.ErrClosed):
		return 0, true
	default:
		t.Fatalf("reading the stalled request's answer: %v", err)

		return 0, false
	}
}

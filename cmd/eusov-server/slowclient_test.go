// White-box tests of the server's behaviour towards clients that stall: one that
// never finishes its headers, and one that announces a body and then sends only
// part of it. Neither may hold the server or a concurrency slot for longer than
// the timeouts the composition root sets (ADR 0019, the slow-client test of PRD
// #103's milestone 3).
//
// No test here hard-codes a timeout the implementation picks: each asserts that
// the server reacts within a bound any sane choice meets, and says in its failure
// message what it waited for, so a timeout that is too long is visible as the
// test's own bound rather than as a hang.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

const (
	// slowClientBound bounds how long the server may keep a stalled client, and
	// how long a test waits for the reaction. It is well above the 10 s
	// read-header timeout and the 30 s body read timeout run sets (ADR 0019),
	// so a regression shows as a failure rather than as a stuck CI job.
	slowClientBound = 90 * time.Second

	// pollInterval is how often a test asks the server whether a slot is free
	// again while another client holds it.
	pollInterval = 250 * time.Millisecond
)

// TestServerStalledHeaderClient pins that a client which sends a request line
// and some headers and then stops is disconnected by the server within its
// read-header timeout, and that another client is served all the while: a stalled
// connection holds a connection, not a worker.
func TestServerStalledHeaderClient(t *testing.T) {
	t.Parallel()

	s := startServer(t, loopbackListen, nil)

	conn, _ := dial(t, s)

	// A request that never ends its headers.
	partial := "POST /v1/assessments HTTP/1.1\r\nHost: " + s.addr + "\r\nContent-Type: application/json\r\n"

	if _, err := conn.Write([]byte(partial)); err != nil {
		t.Fatalf("writing the partial request: %v", err)
	}

	// Another client is served while the first stalls.
	served := make(chan error, 1)

	go func() {
		status, err := s.probe(http.MethodGet, "/healthz", nil)
		switch {
		case err != nil:
			served <- err
		case status != http.StatusOK:
			served <- fmt.Errorf("GET /healthz = %d, want %d", status, http.StatusOK)
		default:
			served <- nil
		}
	}()

	select {
	case err := <-served:
		if err != nil {
			t.Errorf("another client was not served while one stalled: %v; stderr = %q", err, s.stderr.String())
		}
	case <-time.After(slowClientBound):
		t.Errorf("GET /healthz did not answer within %s while another client stalled", slowClientBound)
	}

	// The stalled connection is closed, and the read that reports it is the
	// server's close: a timeout here means the server kept it open.
	if err := waitClosed(conn); err != nil {
		t.Errorf("the stalled client: %v; the server must close a connection whose headers never arrive", err)
	}

	if code := s.stop(t); code != 0 {
		t.Errorf("run returned %d after a cancelled context, want 0; stderr = %q", code, s.stderr.String())
	}
}

// TestServerStalledBodyClient pins that a client which announces a body and then
// stalls does not hold the one assessment slot forever: with --concurrency 1, a
// request that stalls after its headers is answered or disconnected, the slot is
// freed, and the next assessment is served. Without the body read timeout of
// ADR 0019 one such client would keep every other client out.
func TestServerStalledBodyClient(t *testing.T) {
	t.Parallel()

	s := startServer(t, loopbackListen, nil, "--concurrency", "1")

	body := assessRequest(t, fixtureInventory(t), "ovhcloud")

	conn, _ := dial(t, s)

	headers := fmt.Sprintf("POST /v1/assessments HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\n"+
		"Content-Length: %d\r\n\r\n", s.addr, len(body))

	if _, err := conn.Write(append([]byte(headers), body[:len(body)/4]...)); err != nil {
		t.Fatalf("writing the headers and part of the body: %v", err)
	}

	// Until the server's body timeout frees the slot, the next assessment is
	// refused at once with 503 busy; afterwards it is served. Either answer is a
	// working server; a slot that is never freed shows as neither.
	deadline := time.Now().Add(slowClientBound)

	var (
		lastStatus int
		lastErr    error
		served     bool
	)

	for time.Now().Before(deadline) {
		lastStatus, lastErr = s.probe(http.MethodPost, "/v1/assessments", body)
		if lastErr == nil && lastStatus == http.StatusOK {
			served = true

			break
		}

		time.Sleep(pollInterval)
	}

	if !served {
		t.Fatalf("the stalled body held the concurrency slot for the whole %s: the next assessment last got %d (%v); stderr = %q",
			slowClientBound, lastStatus, lastErr, s.stderr.String())
	}

	// The stalled request was answered or disconnected by then, so the client
	// waiting on it is not left hanging either.
	if err := waitClosed(conn); err != nil {
		t.Errorf("the stalled body: %v; the server must answer or close a request whose body stops arriving", err)
	}

	if code := s.stop(t); code != 0 {
		t.Errorf("run returned %d after a cancelled context, want 0; stderr = %q", code, s.stderr.String())
	}
}

// waitClosed waits up to slowClientBound for the server to close conn and returns
// nil when it did: only a read that times out means the connection is still open.
func waitClosed(conn net.Conn) error {
	closed := make(chan error, 1)

	go func() {
		if err := conn.SetReadDeadline(time.Now().Add(slowClientBound)); err != nil {
			closed <- err

			return
		}

		// io.Copy ends when the server closes the connection or the connection
		// is reset; a deadline that passes first reports a timeout error.
		_, err := io.Copy(io.Discard, conn)

		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			closed <- fmt.Errorf("the connection was still open after %s", slowClientBound)

			return
		}

		closed <- nil
	}()

	select {
	case err := <-closed:
		return err
	case <-time.After(slowClientBound + pollInterval):
		return fmt.Errorf("the connection was still open after %s", slowClientBound)
	}
}

// probe sends one request through the server's client and returns the status,
// or the error the request failed with. It takes no *testing.T, so a goroutine
// can call it without failing the test from outside its own goroutine.
func (s *server) probe(method, path string, body []byte) (int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, "http://"+s.addr+path, reader)
	if err != nil {
		return 0, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}

	defer func() { _ = resp.Body.Close() }()

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return resp.StatusCode, err
	}

	return resp.StatusCode, nil
}

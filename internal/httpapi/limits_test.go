package httpapi_test

import (
	"net/http"
	"net/http/httptest"
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

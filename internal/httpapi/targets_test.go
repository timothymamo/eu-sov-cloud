package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/httpapi"
)

// targetsDocument is what GET /v1/targets answers: every target with the name a
// report shows for it and its catalog snapshot.
type targetsDocument struct {
	Targets []targetEntry `json:"targets"`
}

// targetEntry is one target of GET /v1/targets, with exactly the keys a report's
// target object has.
type targetEntry struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Snapshot struct {
		Source    string `json:"source"`
		Retrieved string `json:"retrieved"`
	} `json:"snapshot"`
}

// TestTargets pins GET /v1/targets: every supported target, sorted by key, with
// its display name and its snapshot, whose date is the UTC date.
func TestTargets(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	handler := h.handler(t)

	rec := serve(handler, newRequest(http.MethodGet, "/v1/targets"))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/targets = %d, want %d; body: %q", rec.Code, http.StatusOK, rec.Body.String())
	}

	requireHardeningHeaders(t, rec)
	requireJSONContentType(t, rec)

	var got targetsDocument
	strictDecode(t, rec, &got)

	want := make([]targetEntry, 0, len(supportedKeys))
	for _, key := range supportedKeys {
		var entry targetEntry

		entry.Key = key
		entry.Name = targetFixtures[key].name
		entry.Snapshot.Source = targetFixtures[key].sourceURL
		entry.Snapshot.Retrieved = fixtureDate(key)

		want = append(want, entry)
	}

	if diff := cmp.Diff(want, got.Targets); diff != "" {
		t.Errorf("GET /v1/targets body mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(gammaTarget, got.Targets[len(got.Targets)-1].Name); diff != "" {
		t.Errorf("the targets are not sorted by key (-want +got):\n%s", diff)
	}

	// The gamma fixture is taken just after midnight CET, so its UTC date is the
	// day before: a report dates a snapshot in UTC, not in the catalog's zone.
	if gotDate := got.Targets[len(got.Targets)-1].Snapshot.Retrieved; gotDate != "2026-12-31" {
		t.Errorf("gamma's snapshot date = %q, want the UTC date %q", gotDate, "2026-12-31")
	}
}

// TestTargetsCatalogError pins that a catalog that cannot be loaded is 500
// internal, with nothing of the failure in the body.
func TestTargetsCatalogError(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.catalogs[betaKey].err = errors.New("catalog: MARKER-C-9")
	handler := h.handler(t)

	rec := serve(handler, newRequest(http.MethodGet, "/v1/targets"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("GET /v1/targets = %d, want %d; body: %q",
			rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	requireHardeningHeaders(t, rec)

	if message := decodeError(t, rec, "internal"); message != "internal error" {
		t.Errorf("the 500 message = %q, want %q", message, "internal error")
	}

	requireNoEcho(t, rec, "MARKER-C-9")
}

// statusDocument is what the health endpoints answer.
type statusDocument struct {
	Status string `json:"status"`
}

// TestHealthz pins that the liveness endpoint always answers ok.
func TestHealthz(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.catalogs[alphaKey].err = errors.New("catalog: down")
	handler := h.handler(t)

	rec := serve(handler, newRequest(http.MethodGet, "/healthz"))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want %d; body: %q", rec.Code, http.StatusOK, rec.Body.String())
	}

	requireHardeningHeaders(t, rec)
	requireJSONContentType(t, rec)

	var got statusDocument
	strictDecode(t, rec, &got)

	if diff := cmp.Diff(statusDocument{Status: "ok"}, got); diff != "" {
		t.Errorf("GET /healthz body mismatch (-want +got):\n%s", diff)
	}
}

// TestReadyz pins the readiness endpoint: ready without a check, ready when the
// configured check passes, and 503 not_ready when it fails, without echoing why.
func TestReadyz(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		readiness  func(context.Context) error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "no readiness check configured",
			wantStatus: http.StatusOK,
		},
		{
			name:       "the readiness check passes",
			readiness:  func(context.Context) error { return nil },
			wantStatus: http.StatusOK,
		},
		{
			name:       "the readiness check fails",
			readiness:  func(context.Context) error { return errors.New("readiness: MARKER-R-8") },
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "not_ready",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			var opts []httpapi.Option
			if tc.readiness != nil {
				opts = append(opts, httpapi.WithReadiness(tc.readiness))
			}

			handler := mustNew(t, h.deps(), opts...)

			rec := serve(handler, newRequest(http.MethodGet, "/readyz"))

			if rec.Code != tc.wantStatus {
				t.Fatalf("GET /readyz = %d, want %d; body: %q", rec.Code, tc.wantStatus, rec.Body.String())
			}

			requireHardeningHeaders(t, rec)

			if tc.wantCode != "" {
				decodeError(t, rec, tc.wantCode)
				requireNoEcho(t, rec, "MARKER-R-8")

				return
			}

			requireJSONContentType(t, rec)

			var got statusDocument
			strictDecode(t, rec, &got)

			if diff := cmp.Diff(statusDocument{Status: "ready"}, got); diff != "" {
				t.Errorf("GET /readyz body mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

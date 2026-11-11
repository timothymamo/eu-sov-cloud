package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/httpapi"
)

// TestNewRejectsIncompleteDeps pins that the handler the composition root gets
// from New is complete: a Deps missing any field is refused instead of served,
// so a missing port shows up when the server starts, never as a nil panic in a
// request.
func TestNewRejectsIncompleteDeps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*httpapi.Deps)
	}{
		{
			name:   "no request decoder",
			mutate: func(deps *httpapi.Deps) { deps.DecodeRequest = nil },
		},
		{
			name:   "no inventory source constructor",
			mutate: func(deps *httpapi.Deps) { deps.NewInventorySource = nil },
		},
		{
			name:   "no targets",
			mutate: func(deps *httpapi.Deps) { deps.Targets = nil },
		},
		{
			name:   "an empty target map",
			mutate: func(deps *httpapi.Deps) { deps.Targets = map[string]func() analysis.CatalogSource{} },
		},
		{
			name:   "no JSON reporter",
			mutate: func(deps *httpapi.Deps) { deps.JSON.NewReporter = nil },
		},
		{
			name:   "no JSON comparison reporter",
			mutate: func(deps *httpapi.Deps) { deps.JSON.NewComparisonReporter = nil },
		},
		{
			name:   "no Markdown reporter",
			mutate: func(deps *httpapi.Deps) { deps.Markdown.NewReporter = nil },
		},
		{
			name:   "no Markdown comparison reporter",
			mutate: func(deps *httpapi.Deps) { deps.Markdown.NewComparisonReporter = nil },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deps := newHarness(t).deps()
			tc.mutate(&deps)

			handler, err := httpapi.New(deps)
			if err == nil {
				t.Fatalf("httpapi.New() returned no error for a Deps with %s", tc.name)
			}

			if handler != nil {
				t.Errorf("httpapi.New() returned a non-nil handler beside the error")
			}
		})
	}
}

// TestNewReturnsAServingHandler pins that a complete Deps gives a handler that
// serves, so the cases above are not satisfied by refusing everything.
func TestNewReturnsAServingHandler(t *testing.T) {
	t.Parallel()

	handler := mustNew(t, newHarness(t).deps())

	rec := serve(handler, newRequest(http.MethodGet, "/healthz"))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want %d; body: %q", rec.Code, http.StatusOK, rec.Body.String())
	}
}

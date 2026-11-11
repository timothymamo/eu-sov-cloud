package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/internal/httpapi"
)

// The origins the CORS tests use: one the handler is configured for and one it
// is not.
const (
	allowedOrigin = "https://app.example.com"
	otherOrigin   = "https://evil.example.com"
)

// corsHeaders returns every Access-Control-* response header, so a test can see
// both what is there and what is not.
func corsHeaders(rec *httptest.ResponseRecorder) map[string]string {
	headers := make(map[string]string)

	for name, values := range rec.Header() {
		if strings.HasPrefix(http.CanonicalHeaderKey(name), "Access-Control-") {
			headers[name] = strings.Join(values, ", ")
		}
	}

	return headers
}

// requireNoCORSHeaders fails when any Access-Control-* header is set.
func requireNoCORSHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if headers := corsHeaders(rec); len(headers) != 0 {
		t.Errorf("the response carries CORS headers it must not: %v", headers)
	}
}

// requireNoWildcardOrCredentials fails when the response allows any origin or
// sends credentials: the API is not a browser app's backend with cookies.
func requireNoWildcardOrCredentials(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want a named origin or none", got)
	}

	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want no credentials", got)
	}
}

// TestCORSDisabled pins that CORS is off unless the composition root turns it
// on: no Access-Control-* header, whatever Origin a request carries, and OPTIONS
// is as unsupported as any other wrong method.
func TestCORSDisabled(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	handler := h.handler(t)

	req := newAssessRequest(assessBody(t, alphaKey))
	req.Header.Set("Origin", allowedOrigin)

	rec := serve(handler, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/assessments = %d, want %d; body: %q", rec.Code, http.StatusOK, rec.Body.String())
	}

	requireNoCORSHeaders(t, rec)

	preflight := serve(handler, newRequest(http.MethodOptions, "/v1/assessments"))

	if preflight.Code != http.StatusMethodNotAllowed {
		t.Fatalf("OPTIONS /v1/assessments = %d, want %d; body: %q",
			preflight.Code, http.StatusMethodNotAllowed, preflight.Body.String())
	}

	requireNoCORSHeaders(t, preflight)
}

// TestCORSAllowedOrigin pins that only an origin exactly in the configured list
// is allowed, with Vary: Origin and never a wildcard or credentials.
func TestCORSAllowedOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// origin is the request Origin; empty sets none.
		origin    string
		wantAllow bool
	}{
		{name: "the configured origin", origin: allowedOrigin, wantAllow: true},
		{name: "another origin", origin: otherOrigin},
		{name: "no origin"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			handler := h.handler(t, httpapi.WithAllowedOrigins(allowedOrigin))

			req := newAssessRequest(assessBody(t, alphaKey))
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}

			rec := serve(handler, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("POST /v1/assessments = %d, want %d; body: %q",
					rec.Code, http.StatusOK, rec.Body.String())
			}

			requireNoWildcardOrCredentials(t, rec)

			if !tc.wantAllow {
				requireNoCORSHeaders(t, rec)

				return
			}

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, allowedOrigin)
			}

			if vary := rec.Header().Get("Vary"); !headerHasToken(vary, "Origin") {
				t.Errorf("Vary = %q, want it to name Origin", vary)
			}
		})
	}
}

// TestCORSPreflight pins the preflight answer: 204 with the allowed methods, the
// three request headers the API reads and a max age, but only from an allowed
// origin.
func TestCORSPreflight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		origin     string
		wantStatus int
		wantAllow  bool
	}{
		{name: "from the allowed origin", origin: allowedOrigin, wantStatus: http.StatusNoContent, wantAllow: true},
		{name: "from another origin", origin: otherOrigin, wantStatus: http.StatusMethodNotAllowed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			handler := h.handler(t, httpapi.WithAllowedOrigins(allowedOrigin))

			req := newRequest(http.MethodOptions, "/v1/assessments")
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Access-Control-Request-Method", http.MethodPost)
			req.Header.Set("Access-Control-Request-Headers", "authorization, content-type, accept")

			rec := serve(handler, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("preflight = %d, want %d; body: %q", rec.Code, tc.wantStatus, rec.Body.String())
			}

			requireHardeningHeaders(t, rec)
			requireNoWildcardOrCredentials(t, rec)

			if !tc.wantAllow {
				requireNoCORSHeaders(t, rec)

				return
			}

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, allowedOrigin)
			}

			if methods := rec.Header().Get("Access-Control-Allow-Methods"); !headerHasToken(methods, http.MethodPost) {
				t.Errorf("Access-Control-Allow-Methods = %q, want it to name POST", methods)
			}

			allowed := rec.Header().Get("Access-Control-Allow-Headers")
			for _, header := range []string{"Authorization", "Content-Type", "Accept"} {
				if !headerHasToken(allowed, header) {
					t.Errorf("Access-Control-Allow-Headers = %q, want it to name %s", allowed, header)
				}
			}

			if got := rec.Header().Get("Access-Control-Max-Age"); got != "600" {
				t.Errorf("Access-Control-Max-Age = %q, want %q", got, "600")
			}

			if got := rec.Header().Get("Content-Length"); got != "0" && got != "" {
				t.Errorf("a 204 preflight has Content-Length %q", got)
			}
		})
	}
}

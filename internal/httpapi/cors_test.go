package httpapi_test

import (
	"errors"
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

// TestNewRejectsAnInvalidAllowedOrigin pins that every origin the composition
// root configures is one a browser can send and the handler can compare: an
// origin is a scheme and a host, never a wildcard, a path, a query, user info or
// another scheme, and never the string "null" or an empty value.
func TestNewRejectsAnInvalidAllowedOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		origin string
	}{
		{name: "the string null", origin: "null"},
		{name: "a wildcard", origin: "*"},
		{name: "an empty origin", origin: ""},
		{name: "no scheme", origin: "example.com"},
		{name: "a trailing slash", origin: "https://app.example.com/"},
		{name: "a path", origin: "https://app.example.com/x"},
		{name: "another scheme", origin: "ftp://x.example.com"},
		{name: "user info", origin: "https://user@app.example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler, err := httpapi.New(newHarness(t).deps(), httpapi.WithAllowedOrigins(tc.origin))
			if err == nil {
				t.Fatalf("httpapi.New() returned no error for the origin %q", tc.origin)
			}

			if handler != nil {
				t.Errorf("httpapi.New() returned a non-nil handler beside the error")
			}
		})
	}
}

// TestAllowedOriginMatchIgnoresCase pins that an origin is compared the way a
// browser normalises it: the scheme and the host are case-insensitive, so a
// configured https://App.Example.com allows a request from
// https://app.example.com and the answer echoes the origin the request sent. A
// loopback origin with a port is a valid origin too.
func TestAllowedOriginMatchIgnoresCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured string
		origin     string
	}{
		{
			name:       "a configured host with capitals",
			configured: "https://App.Example.com",
			origin:     "https://app.example.com",
		},
		{
			name:       "a configured origin with capitals",
			configured: "https://app.example.com",
			origin:     "https://APP.EXAMPLE.COM",
		},
		{
			name:       "a loopback origin with a port",
			configured: "http://localhost:3000",
			origin:     "http://localhost:3000",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			handler := h.handler(t, httpapi.WithAllowedOrigins(tc.configured))

			req := newAssessRequest(assessBody(t, alphaKey))
			req.Header.Set("Origin", tc.origin)

			rec := serve(handler, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("POST /v1/assessments = %d, want %d; body: %q",
					rec.Code, http.StatusOK, rec.Body.String())
			}

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.origin {
				t.Errorf("Access-Control-Allow-Origin = %q, want the request's origin %q", got, tc.origin)
			}

			requireVaryOrigin(t, rec)
		})
	}
}

// requireVaryOrigin fails unless the response's Vary header names Origin, in any
// of its values.
func requireVaryOrigin(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if vary := strings.Join(rec.Header().Values("Vary"), ", "); !headerHasToken(vary, "Origin") {
		t.Errorf("Vary = %q, want it to name Origin", vary)
	}
}

// TestVaryOriginWhenCORSIsConfigured pins that a handler with CORS configured
// tells every cache that its answer depends on the request's Origin: every
// response carries Vary: Origin, one to an allowed origin, one to a disallowed
// origin, one without an Origin and an error answer too.
func TestVaryOriginWhenCORSIsConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		origin     string
		configure  func(*harness)
		request    func(t *testing.T) *http.Request
		wantStatus int
	}{
		{
			name:       "an allowed origin",
			origin:     allowedOrigin,
			request:    func(t *testing.T) *http.Request { t.Helper(); return newAssessRequest(assessBody(t, alphaKey)) },
			wantStatus: http.StatusOK,
		},
		{
			name:       "a disallowed origin",
			origin:     otherOrigin,
			request:    func(t *testing.T) *http.Request { t.Helper(); return newAssessRequest(assessBody(t, alphaKey)) },
			wantStatus: http.StatusOK,
		},
		{
			name:       "no origin",
			request:    func(t *testing.T) *http.Request { t.Helper(); return newAssessRequest(assessBody(t, alphaKey)) },
			wantStatus: http.StatusOK,
		},
		{
			name:      "an error answer to a disallowed origin",
			origin:    otherOrigin,
			configure: func(h *harness) { h.inventory.err = errors.New("inventory: refused") },
			request: func(t *testing.T) *http.Request {
				t.Helper()

				return newAssessRequest(assessBody(t, alphaKey))
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "the target list to a disallowed origin",
			origin:     otherOrigin,
			request:    func(*testing.T) *http.Request { return newRequest(http.MethodGet, "/v1/targets") },
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			if tc.configure != nil {
				tc.configure(h)
			}

			handler := h.handler(t, httpapi.WithAllowedOrigins(allowedOrigin))

			req := tc.request(t)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}

			rec := serve(handler, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("%s %s = %d, want %d; body: %q",
					req.Method, req.URL.Path, rec.Code, tc.wantStatus, rec.Body.String())
			}

			requireVaryOrigin(t, rec)
		})
	}
}

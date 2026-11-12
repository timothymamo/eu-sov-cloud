package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/internal/httpapi"
)

// TestUnknownPath pins that a path the API does not serve is 404 not_found, and
// that the path itself is never echoed.
func TestUnknownPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		marker string
	}{
		{name: "another version", path: "/v2/assessments"},
		{name: "the root", path: "/"},
		{name: "below assessments", path: "/v1/assessments/x"},
		{name: "below targets", path: "/v1/targets/alpha"},
		{name: "a marker in the path", path: "/v1/MARKER-P-99", marker: "MARKER-P-99"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			handler := h.handler(t)

			rec := serve(handler, newRequest(http.MethodGet, tc.path))

			if rec.Code != http.StatusNotFound {
				t.Fatalf("GET %s = %d, want %d; body: %q", tc.path, rec.Code, http.StatusNotFound, rec.Body.String())
			}

			requireHardeningHeaders(t, rec)

			message := decodeError(t, rec, "not_found")
			if strings.Contains(message, tc.path) {
				t.Errorf("the 404 message %q echoes the path %q", message, tc.path)
			}

			if tc.marker != "" {
				requireNoEcho(t, rec, tc.marker)
			}
		})
	}
}

// TestWrongMethod pins that a known path with another method is 405
// method_not_allowed, that the Allow header names the method the path serves,
// and that the path is never echoed.
func TestWrongMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		method, path string
		wantAllow    string
	}{
		{name: "GET assessments", method: http.MethodGet, path: "/v1/assessments", wantAllow: http.MethodPost},
		{name: "PUT assessments", method: http.MethodPut, path: "/v1/assessments", wantAllow: http.MethodPost},
		{name: "POST targets", method: http.MethodPost, path: "/v1/targets", wantAllow: http.MethodGet},
		{name: "DELETE healthz", method: http.MethodDelete, path: "/healthz", wantAllow: http.MethodGet},
		{name: "POST readyz", method: http.MethodPost, path: "/readyz", wantAllow: http.MethodGet},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			handler := h.handler(t)

			rec := serve(handler, newRequest(tc.method, tc.path))

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s = %d, want %d; body: %q",
					tc.method, tc.path, rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
			}

			requireHardeningHeaders(t, rec)

			allow := rec.Header().Get("Allow")
			if !headerHasToken(allow, tc.wantAllow) {
				t.Errorf("Allow = %q, want it to name %s", allow, tc.wantAllow)
			}

			message := decodeError(t, rec, "method_not_allowed")
			if strings.Contains(message, tc.path) {
				t.Errorf("the 405 message %q echoes the path %q", message, tc.path)
			}
		})
	}
}

// TestResponseHeadersAndErrorShape pins the two hardening headers on every
// response and the one error shape on every error: exactly the documented keys,
// no unknown field, nothing after the object.
func TestResponseHeadersAndErrorShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// options are the handler options this case needs.
		options func(h *harness) []httpapi.Option
		// configure changes the fakes before the handler is built.
		configure  func(*harness)
		req        func(h *harness) *http.Request
		wantStatus int
		// wantContentType is the success's media type; empty means
		// application/json.
		wantContentType string
		// wantCode is the error code; empty for a success.
		wantCode string
	}{
		{
			name:       "a JSON report",
			req:        func(*harness) *http.Request { return newAssessRequest(assessBody(t, alphaKey)) },
			wantStatus: http.StatusOK,
		},
		{
			name: "a Markdown report",
			req: func(*harness) *http.Request {
				return withAccept(newAssessRequest(assessBody(t, alphaKey)), "text/markdown")
			},
			wantStatus:      http.StatusOK,
			wantContentType: markdownContentType,
		},
		{
			name:       "the target list",
			req:        func(*harness) *http.Request { return newRequest(http.MethodGet, "/v1/targets") },
			wantStatus: http.StatusOK,
		},
		{
			name:       "health",
			req:        func(*harness) *http.Request { return newRequest(http.MethodGet, "/healthz") },
			wantStatus: http.StatusOK,
		},
		{
			name:       "readiness",
			req:        func(*harness) *http.Request { return newRequest(http.MethodGet, "/readyz") },
			wantStatus: http.StatusOK,
		},
		{
			name:       "an undecodable request",
			configure:  func(h *harness) { h.decoder.err = errors.New("decode request: bad") },
			req:        func(*harness) *http.Request { return newAssessRequest(`{}`) },
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
		},
		{
			name:       "an invalid inventory",
			configure:  func(h *harness) { h.inventory.err = errors.New("inventory: bad") },
			req:        func(*harness) *http.Request { return newAssessRequest(assessBody(t, alphaKey)) },
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_inventory",
		},
		{
			name:       "an invalid target list",
			req:        func(*harness) *http.Request { return newAssessRequest(assessBody(t, "nope")) },
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_targets",
		},
		{
			name: "a missing token",
			options: func(*harness) []httpapi.Option {
				return []httpapi.Option{httpapi.WithBearerToken(bearerToken)}
			},
			req:        func(*harness) *http.Request { return newAssessRequest(assessBody(t, alphaKey)) },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthorized",
		},
		{
			name:       "an unknown path",
			req:        func(*harness) *http.Request { return newRequest(http.MethodGet, "/v2/assessments") },
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			name:       "a wrong method",
			req:        func(*harness) *http.Request { return newRequest(http.MethodDelete, "/healthz") },
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "method_not_allowed",
		},
		{
			name: "an unacceptable type",
			req: func(*harness) *http.Request {
				return withAccept(newAssessRequest(assessBody(t, alphaKey)), "text/html")
			},
			wantStatus: http.StatusNotAcceptable,
			wantCode:   "not_acceptable",
		},
		{
			name: "an oversized body",
			options: func(*harness) []httpapi.Option {
				return []httpapi.Option{httpapi.WithMaxBodyBytes(64)}
			},
			req:        func(*harness) *http.Request { return newAssessRequest(assessBody(t, alphaKey)) },
			wantStatus: http.StatusRequestEntityTooLarge,
			wantCode:   "payload_too_large",
		},
		{
			name: "a body of another media type",
			req: func(*harness) *http.Request {
				req := newAssessRequest(assessBody(t, alphaKey))
				req.Header.Set("Content-Type", "text/plain")

				return req
			},
			wantStatus: http.StatusUnsupportedMediaType,
			wantCode:   "unsupported_media_type",
		},
		{
			name:       "a failing catalog",
			configure:  func(h *harness) { h.catalogs[alphaKey].err = errors.New("catalog: bad") },
			req:        func(*harness) *http.Request { return newAssessRequest(assessBody(t, alphaKey)) },
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal",
		},
		{
			name: "a failing readiness check",
			options: func(*harness) []httpapi.Option {
				return []httpapi.Option{httpapi.WithReadiness(func(context.Context) error { return errors.New("not ready") })}
			},
			req:        func(*harness) *http.Request { return newRequest(http.MethodGet, "/readyz") },
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "not_ready",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			if tc.configure != nil {
				tc.configure(h)
			}

			var opts []httpapi.Option
			if tc.options != nil {
				opts = tc.options(h)
			}

			handler := h.handler(t, opts...)

			rec := serve(handler, tc.req(h))

			if rec.Code != tc.wantStatus {
				t.Fatalf("%s = %d, want %d; body: %q", tc.name, rec.Code, tc.wantStatus, rec.Body.String())
			}

			requireHardeningHeaders(t, rec)

			if tc.wantCode == "" {
				want := tc.wantContentType
				if want == "" {
					want = jsonContentType
				}

				requireMediaType(t, rec, want)

				return
			}

			decodeError(t, rec, tc.wantCode)
		})
	}
}

// headerHasToken reports whether the comma-separated header value holds token,
// compared without case.
func headerHasToken(value, token string) bool {
	for _, part := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}

	return false
}

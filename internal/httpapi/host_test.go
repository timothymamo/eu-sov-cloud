package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/internal/httpapi"
)

// This file pins the optional host allow list: when the composition root names
// the hosts the API serves, a request for any other host is 421
// misdirected_request before authentication and before the body is read, so a
// request that reached the wrong virtual host never costs a token comparison or
// an inventory read. The health endpoints stay reachable by IP, as a platform
// probes them (PRD #103, audit L3).

// allowedHost is the host the handler is configured for in these tests.
const allowedHost = "api.example.com"

// assessRequestForHost returns a POST /v1/assessments sent to host.
func assessRequestForHost(t *testing.T, host string) *http.Request {
	t.Helper()

	req := newAssessRequest(assessBody(t, alphaKey))
	req.Host = host

	return req
}

// TestAllowedHosts pins the host check: only the configured hosts are served,
// compared without their port and case-insensitively, and IPv6 hosts keep their
// brackets. A request for another host is 421 misdirected_request with the
// documented error shape, before the body is decoded.
func TestAllowedHosts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured []string
		host       string
		wantStatus int
	}{
		{
			name:       "the allowed host",
			configured: []string{allowedHost},
			host:       allowedHost,
			wantStatus: http.StatusOK,
		},
		{
			name:       "the allowed host with a port",
			configured: []string{allowedHost},
			host:       allowedHost + ":8443",
			wantStatus: http.StatusOK,
		},
		{
			name:       "the allowed host in another case, with a port",
			configured: []string{allowedHost},
			host:       "API.Example.COM:8443",
			wantStatus: http.StatusOK,
		},
		{
			name:       "another host",
			configured: []string{allowedHost},
			host:       "other.example.com",
			wantStatus: http.StatusMisdirectedRequest,
		},
		{
			name:       "the allowed host beside another one",
			configured: []string{allowedHost, "api2.example.com"},
			host:       "api2.example.com",
			wantStatus: http.StatusOK,
		},
		{
			name:       "an IPv6 host in brackets, with a port",
			configured: []string{allowedHost, "[2001:db8::1]"},
			host:       "[2001:DB8::1]:9000",
			wantStatus: http.StatusOK,
		},
		{
			name:       "another IPv6 host",
			configured: []string{allowedHost, "[2001:db8::1]"},
			host:       "[2001:db8::2]",
			wantStatus: http.StatusMisdirectedRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			handler := h.handler(t, httpapi.WithAllowedHosts(tc.configured...))

			rec := serve(handler, assessRequestForHost(t, tc.host))

			if rec.Code != tc.wantStatus {
				t.Fatalf("POST /v1/assessments with Host %q = %d, want %d; body: %q",
					tc.host, rec.Code, tc.wantStatus, rec.Body.String())
			}

			requireHardeningHeaders(t, rec)

			if tc.wantStatus == http.StatusOK {
				return
			}

			decodeError(t, rec, "misdirected_request")

			if got := h.decoder.callCount(); got != 0 {
				t.Errorf("the request decoder ran %d times for a request to a host the API does not serve, want 0", got)
			}
		})
	}
}

// TestAllowedHostsComesBeforeAuth pins that the host is checked before the
// bearer token: a request for a host the API does not serve is 421, not 401,
// and it never reaches the token comparison.
func TestAllowedHostsComesBeforeAuth(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	handler := h.handler(t,
		httpapi.WithAllowedHosts(allowedHost),
		httpapi.WithBearerToken(bearerToken),
	)

	rec := serve(handler, assessRequestForHost(t, "other.example.com"))

	if rec.Code != http.StatusMisdirectedRequest {
		t.Fatalf("POST /v1/assessments for another host = %d, want %d; body: %q",
			rec.Code, http.StatusMisdirectedRequest, rec.Body.String())
	}

	requireHardeningHeaders(t, rec)
	decodeError(t, rec, "misdirected_request")

	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Errorf("WWW-Authenticate = %q on a misdirected request, want no header", got)
	}

	if got := h.decoder.callCount(); got != 0 {
		t.Errorf("the request decoder ran %d times for a misdirected request, want 0", got)
	}
}

// TestAllowedHostsLeavesHealthOpen pins that the platform's probes reach the
// health endpoints by IP even when the API serves a named host: they are not
// virtual-host routed.
func TestAllowedHostsLeavesHealthOpen(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	handler := h.handler(t, httpapi.WithAllowedHosts(allowedHost))

	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			req := newRequest(http.MethodGet, path)
			req.Host = "10.0.0.1"

			rec := serve(handler, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s from an address the allow list does not name = %d, want %d; body: %q",
					path, rec.Code, http.StatusOK, rec.Body.String())
			}
		})
	}
}

// TestNoAllowedHostsAcceptsEveryHost pins that the allow list is off unless it
// is configured: without the option, every Host is served.
func TestNoAllowedHostsAcceptsEveryHost(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	handler := h.handler(t)

	rec := serve(handler, assessRequestForHost(t, "anything.example.com"))

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/assessments without a host allow list = %d, want %d; body: %q",
			rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestAllowedHostsUsesTheRequestHost verifies the check reads the request's Host
// and not a forwarded header a client controls.
func TestAllowedHostsUsesTheRequestHost(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	handler := h.handler(t, httpapi.WithAllowedHosts(allowedHost))

	req := assessRequestForHost(t, "other.example.com")
	req.Header.Set("X-Forwarded-Host", allowedHost)

	rec := serve(handler, req)

	if rec.Code != http.StatusMisdirectedRequest {
		t.Fatalf("POST /v1/assessments for another host with a forged X-Forwarded-Host = %d, want %d; body: %q",
			rec.Code, http.StatusMisdirectedRequest, rec.Body.String())
	}
}

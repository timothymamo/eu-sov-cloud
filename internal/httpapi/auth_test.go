package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/internal/httpapi"
)

// TestBearerToken pins the optional bearer token: every /v1/ request needs it,
// compared before the body is read, and no answer ever holds it.
func TestBearerToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// authorization is the Authorization header; empty sets none.
		authorization string
		wantStatus    int
	}{
		{name: "no Authorization header", wantStatus: http.StatusUnauthorized},
		{name: "a wrong token", authorization: "Bearer not-the-token", wantStatus: http.StatusUnauthorized},
		{name: "another scheme", authorization: "Basic dXNlcjpwYXNz", wantStatus: http.StatusUnauthorized},
		{name: "the bearer scheme with nothing", authorization: "Bearer", wantStatus: http.StatusUnauthorized},
		{name: "the bearer scheme with only a space", authorization: "Bearer ", wantStatus: http.StatusUnauthorized},
		{
			name:          "the right token",
			authorization: "Bearer " + bearerToken,
			wantStatus:    http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			handler := h.handler(t, httpapi.WithBearerToken(bearerToken))

			req := newAssessRequest(assessBody(t, alphaKey))
			if tc.authorization != "" {
				req.Header.Set("Authorization", tc.authorization)
			}

			rec := serve(handler, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("POST /v1/assessments = %d, want %d; body: %q", rec.Code, tc.wantStatus, rec.Body.String())
			}

			requireHardeningHeaders(t, rec)
			requireNoEcho(t, rec, bearerToken)

			if tc.wantStatus == http.StatusOK {
				requireJSONContentType(t, rec)

				want := singleBody(jsonFormat, alphaTarget, inventoryName)
				if got := rec.Body.String(); got != want {
					t.Errorf("body = %q, want %q", got, want)
				}

				return
			}

			if got := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer") {
				t.Errorf("WWW-Authenticate = %q, want the Bearer scheme", got)
			}

			decodeError(t, rec, "unauthorized")

			if got := h.decoder.callCount(); got != 0 {
				t.Errorf("the request decoder ran %d times for an unauthenticated request, want 0", got)
			}
		})
	}
}

// TestBearerTokenLeavesHealthOpen pins that the health endpoints need no token.
func TestBearerTokenLeavesHealthOpen(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			handler := h.handler(t, httpapi.WithBearerToken(bearerToken))

			rec := serve(handler, newRequest(http.MethodGet, path))

			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d, want %d; body: %q", path, rec.Code, http.StatusOK, rec.Body.String())
			}

			requireNoEcho(t, rec, bearerToken)
		})
	}
}

// TestNoTokenNeedsNoAuth pins that the API is open by default: without the
// option there is no authentication at all.
func TestNoTokenNeedsNoAuth(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	handler := h.handler(t)

	rec := serve(handler, newAssessRequest(assessBody(t, alphaKey)))

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/assessments = %d, want %d; body: %q", rec.Code, http.StatusOK, rec.Body.String())
	}

	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Errorf("WWW-Authenticate = %q on a handler without a token, want no header", got)
	}
}

// TestNewRejectsAnInvalidToken pins that a token the composition root gives
// explicitly must be usable as one: an empty, blank or control-character token
// is a configuration error, never a quiet "authentication off", and never a
// token no request could ever match.
func TestNewRejectsAnInvalidToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		token string
	}{
		{name: "an empty token", token: ""},
		{name: "white space only", token: "   "},
		{name: "leading white space", token: " " + bearerToken},
		{name: "trailing white space", token: bearerToken + " "},
		{name: "a space inside", token: "s3cret token"},
		{name: "a control character", token: "s3cret\x00token"},
		{name: "a line break", token: "s3cret\ntoken"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler, err := httpapi.New(newHarness(t).deps(), httpapi.WithBearerToken(tc.token))
			if err == nil {
				t.Fatalf("httpapi.New() returned no error for %s", tc.name)
			}

			if handler != nil {
				t.Errorf("httpapi.New() returned a non-nil handler beside the error")
			}
		})
	}
}

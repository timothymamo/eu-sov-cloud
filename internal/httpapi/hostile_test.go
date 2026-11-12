package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/internal/httpapi"
)

// The values a hostile client sends that must never come back in a response.
const (
	markerTarget = "MARKER-T-77"
	markerPath   = "MARKER-P-99"
	markerHeader = "MARKER-H-11"
)

// hostileRequest is one hostile request and the marker it carries, if any.
type hostileRequest struct {
	req    *http.Request
	marker string
}

// TestHostileInputs pins how the API answers the input an untrusted client can
// send: a documented status, the one error shape, and nothing of the request
// echoed in the response.
func TestHostileInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// request builds the request a fresh handler is sent.
		request    func(t *testing.T) hostileRequest
		wantStatus int
		wantCode   string
		// marker is a value the response must not echo, if any.
		marker string
	}{
		{
			name: "a body of binary garbage",
			request: func(*testing.T) hostileRequest {
				body := make([]byte, 0, 512)
				for i := range 512 {
					body = append(body, byte(i))
				}

				return hostileRequest{req: newAssessRequest(string(body))}
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
		},
		{
			name: "a 1 MiB JSON string",
			request: func(*testing.T) hostileRequest {
				return hostileRequest{req: newAssessRequest(`"` + strings.Repeat("a", 1<<20) + `"`)}
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
		},
		{
			name: "deeply nested arrays",
			request: func(*testing.T) hostileRequest {
				return hostileRequest{req: newAssessRequest(strings.Repeat("[", 100_000))}
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
		},
		{
			name: "a huge header value",
			request: func(*testing.T) hostileRequest {
				req := newRequest(http.MethodGet, "/healthz")
				req.Header.Set("X-Hostile", markerHeader+strings.Repeat("x", 256<<10))

				return hostileRequest{req: req, marker: markerHeader}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "a marker in the target list",
			request: func(t *testing.T) hostileRequest {
				t.Helper()

				return hostileRequest{req: newAssessRequest(assessBody(t, markerTarget))}
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_targets",
			marker:     markerTarget,
		},
		{
			name: "a marker in the path",
			request: func(*testing.T) hostileRequest {
				return hostileRequest{req: newRequest(http.MethodGet, "/v1/"+markerPath)}
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
			marker:     markerPath,
		},
		{
			name: "a marker in the Content-Type",
			request: func(t *testing.T) hostileRequest {
				t.Helper()

				req := newAssessRequest(assessBody(t, alphaKey))
				req.Header.Set("Content-Type", markerHeader)

				return hostileRequest{req: req, marker: markerHeader}
			},
			wantStatus: http.StatusUnsupportedMediaType,
			wantCode:   "unsupported_media_type",
			marker:     markerHeader,
		},
		{
			name: "a marker in the Accept header",
			request: func(t *testing.T) hostileRequest {
				t.Helper()

				return hostileRequest{
					req:    withAccept(newAssessRequest(assessBody(t, alphaKey)), markerHeader),
					marker: markerHeader,
				}
			},
			wantStatus: http.StatusNotAcceptable,
			wantCode:   "not_acceptable",
			marker:     markerHeader,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			// The 1 MiB and 100 kB bodies are meant to reach the decoder, so the
			// cap is set well above them: this test pins the decoding, not the cap.
			handler := h.handler(t, httpapi.WithMaxBodyBytes(8<<20))

			built := tc.request(t)

			rec := serve(handler, built.req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("%s: status = %d, want %d; body: %q",
					tc.name, rec.Code, tc.wantStatus, rec.Body.String())
			}

			requireHardeningHeaders(t, rec)

			if tc.wantCode != "" {
				decodeError(t, rec, tc.wantCode)
			}

			marker := tc.marker
			if marker == "" {
				marker = built.marker
			}

			if marker != "" {
				requireNoEcho(t, rec, marker)
			}
		})
	}
}

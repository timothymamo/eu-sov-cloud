// White-box tests of the bearer token the composition root reads: from the
// environment or from a file, never from a flag, because a token in an argument
// shows in the process list of every user on the host. The seam is run: a
// startup error returns 2 with one line on stderr, and the token never reaches
// that line.
package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validToken is a token the API accepts (RFC 6750 token68) and that no error
// line may ever quote.
const validToken = "tok-0123456789abcdef"

// TestServerBearerToken pins the token from EUSOV_SERVER_TOKEN: every /v1/
// request without Authorization: Bearer <token> is refused with 401 and without
// a hint of the token, the same request with it answers 200, and the health
// endpoints stay open, because a platform's probe carries no credentials.
func TestServerBearerToken(t *testing.T) {
	t.Parallel()

	s := startServer(t, loopbackListen, map[string]string{tokenEnv: validToken})

	tests := []struct {
		name   string
		path   string
		header string
		want   int
	}{
		{name: "targets without a token", path: "/v1/targets", want: http.StatusUnauthorized},
		{name: "targets with a wrong token", path: "/v1/targets", header: "Bearer tok-wrong", want: http.StatusUnauthorized},
		{name: "targets with no bearer scheme", path: "/v1/targets", header: validToken, want: http.StatusUnauthorized},
		{name: "targets with the token", path: "/v1/targets", header: "Bearer " + validToken, want: http.StatusOK},
		{name: "healthz stays open", path: "/healthz", want: http.StatusOK},
		{name: "readyz stays open", path: "/readyz", want: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := newRequest(t, http.MethodGet, tc.path, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			got := s.do(t, req)

			if got.status != tc.want {
				t.Errorf("GET %s (Authorization %q) = %d, want %d; body: %q",
					tc.path, tc.header, got.status, tc.want, got.body)
			}

			if strings.Contains(string(got.body), validToken) {
				t.Errorf("the %s answer holds the configured token", tc.path)
			}

			// A refused client is told which scheme to use.
			if tc.want == http.StatusUnauthorized {
				if scheme := got.header.Get("WWW-Authenticate"); !strings.EqualFold(scheme, "Bearer") {
					t.Errorf("WWW-Authenticate for GET %s = %q, want Bearer", tc.path, scheme)
				}
			}
		})
	}
}

// TestServerTokenFromFile pins --token-file: the token is the file's content
// with the surrounding white space trimmed, since an editor writes a line feed
// after the token, and the trimmed token is the one a client must send.
func TestServerTokenFromFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{name: "one line feed", content: validToken + "\n"},
		{name: "spaces and line feeds", content: "  \t\n " + validToken + " \n\t\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := writeTokenFile(t, tc.content)

			s := startServer(t, loopbackListen, nil, "--token-file", path)

			tests := []struct {
				header string
				want   int
			}{
				{header: "", want: http.StatusUnauthorized},
				{header: "Bearer " + validToken, want: http.StatusOK},
			}

			for _, ct := range tests {
				req := newRequest(t, http.MethodGet, "/v1/targets", nil)
				if ct.header != "" {
					req.Header.Set("Authorization", ct.header)
				}

				got := s.do(t, req)

				if got.status != ct.want {
					t.Errorf("GET /v1/targets (Authorization %q) = %d, want %d; body: %q",
						ct.header, got.status, ct.want, got.body)
				}
			}

			if strings.Contains(s.stderr.String(), validToken) {
				t.Errorf("stderr holds the configured token: %q", s.stderr.String())
			}
		})
	}
}

// TestServerTokenConfigurationErrors pins every token source the server must
// refuse at startup: one that is configured but holds no token, a file that does
// not exist, and two sources at once, which leave the operator unsure which one
// wins. Each is exit code 2 with one line, and no line quotes the token.
//
// The one source this seam cannot express is an environment variable that is set
// to the empty string: run takes a getenv of the type os.Getenv has, which
// reports "" for an unset variable as well, so the server cannot tell the two
// apart. The empty-source rule is therefore pinned through a value that trims to
// empty, which the API refuses in exactly the same way (WithBearerToken), and
// through the empty file, which is the case ADR 0019 spells out.
func TestServerTokenConfigurationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		vars map[string]string
		args func(t *testing.T) []string
		// wantNames lists the values the error line must name, so the operator
		// learns which source is wrong.
		wantNames []string
	}{
		{
			name:      "an environment value that trims to nothing",
			vars:      map[string]string{tokenEnv: "  \t\n "},
			wantNames: []string{tokenEnv},
		},
		{
			name: "an empty token file",
			args: func(t *testing.T) []string {
				t.Helper()

				return []string{"--token-file", writeTokenFile(t, "")}
			},
			wantNames: []string{"--token-file"},
		},
		{
			name: "a token file of white space",
			args: func(t *testing.T) []string {
				t.Helper()

				return []string{"--token-file", writeTokenFile(t, " \n\t\n")}
			},
			wantNames: []string{"--token-file"},
		},
		{
			name: "a token file that does not exist",
			args: func(t *testing.T) []string {
				t.Helper()

				return []string{"--token-file", filepath.Join(t.TempDir(), "no-such-token")}
			},
			wantNames: []string{"--token-file"},
		},
		{
			// A token file the operator cannot read is no token source either.
			name: "a token file that is a directory",
			args: func(t *testing.T) []string {
				t.Helper()

				return []string{"--token-file", t.TempDir()}
			},
			wantNames: []string{"--token-file"},
		},
		{
			// Two sources leave the operator unsure which one wins, so neither
			// is used.
			name: "the environment and a token file at once",
			vars: map[string]string{tokenEnv: validToken},
			args: func(t *testing.T) []string {
				t.Helper()

				return []string{"--token-file", writeTokenFile(t, "another-"+validToken+"\n")}
			},
			wantNames: []string{tokenEnv, "--token-file"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			args := []string{"--listen", loopbackListen}
			if tc.args != nil {
				args = append(args, tc.args(t)...)
			}

			got := runOnce(t, tc.vars, args...)

			if got.listened {
				t.Fatalf("run listened on %v, want a startup error: %q", got.addr, got.stderr)
			}

			requireStartupError(t, got)

			for _, want := range tc.wantNames {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want it to name %q", got.stderr, want)
				}
			}

			if strings.Contains(got.stderr, validToken) {
				t.Errorf("stderr = %q, want it to quote no token", got.stderr)
			}
		})
	}
}

// TestServerHasNoTokenFlag pins that the token never comes from an argument:
// --token is an unknown flag, so the run is refused with exit code 2 rather
// than reading a token from the process list.
func TestServerHasNoTokenFlag(t *testing.T) {
	t.Parallel()

	got := runOnce(t, nil, "--listen", loopbackListen, "--token", validToken)

	if got.listened {
		t.Fatalf("run listened on %v, want it to refuse --token", got.addr)
	}

	requireFlagError(t, got, "token")

	if strings.Contains(got.stderr, validToken) {
		t.Errorf("stderr = %q, want the unknown flag refused without quoting its value", got.stderr)
	}
}

// writeTokenFile writes content to a file in a temporary directory and returns
// its path.
func writeTokenFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "token")

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the token file: %v", err)
	}

	return path
}

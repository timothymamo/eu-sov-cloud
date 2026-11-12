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
	"runtime"
	"strings"
	"testing"
)

const (
	// validToken is a token the API accepts (RFC 6750 token68) and that no error
	// line may ever quote. It is exactly the shortest token the server accepts, so
	// a server that demands longer than the minimum fails a case below.
	validToken = "tok-0123456789abcdef0123456789ab"

	// shortToken is one character short of the minimum: the server refuses it.
	shortToken = "tok-0123456789012345678901234567"

	// tokenMinLength is the shortest token the server accepts, as its error line
	// names it.
	tokenMinLength = "32"
)

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
// A source that is set to nothing is refused too, and run's lookupEnv is what
// makes the environment case observable: os.LookupEnv reports a variable that is
// set to the empty string as set, so ADR 0019 reads that as a configured source
// with no token, which is a startup error and not the unset variable that lets
// the server run without one on loopback.
func TestServerTokenConfigurationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		vars map[string]string
		args func(t *testing.T) []string
		// wantNames lists the values the error line must name, so the operator
		// learns which source is wrong.
		wantNames []string
		// wantQuiet is a value the error line must never quote: a token.
		wantQuiet string
	}{
		{
			// Set to the empty string, which is not the same as unset: the
			// operator configured a source and it holds no token.
			name:      "an environment variable set to the empty string",
			vars:      map[string]string{tokenEnv: ""},
			wantNames: []string{tokenEnv},
		},
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
		{
			// A token a client could guess at is no token: the line names the
			// source and the minimum, never the token.
			name:      "an environment token shorter than the minimum",
			vars:      map[string]string{tokenEnv: shortToken},
			wantNames: []string{tokenEnv, tokenMinLength},
			wantQuiet: shortToken,
		},
		{
			// The minimum is measured after the white space is trimmed, so a
			// short token padded with it is refused as well.
			name:      "an environment token that is short once trimmed",
			vars:      map[string]string{tokenEnv: "  " + shortToken + "\n"},
			wantNames: []string{tokenEnv, tokenMinLength},
			wantQuiet: shortToken,
		},
		{
			name: "a token file whose token is shorter than the minimum",
			args: func(t *testing.T) []string {
				t.Helper()

				return []string{"--token-file", writeTokenFile(t, shortToken+"\n")}
			},
			wantNames: []string{"--token-file", tokenMinLength},
			wantQuiet: shortToken,
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

			if tc.wantQuiet != "" && strings.Contains(got.stderr, tc.wantQuiet) {
				t.Errorf("stderr = %q, want it to quote none of the token %q", got.stderr, tc.wantQuiet)
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

// TestServerTokenFilePermissions pins the warning a token file that other users
// of the host can read earns (audit L4): the server names the path, never the
// token, and starts anyway, because refusing to start would turn a permission
// mistake into an outage. A file only its owner can read warns nothing.
func TestServerTokenFilePermissions(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Windows has no group and world permission bits")
	}

	tests := []struct {
		name string
		mode os.FileMode
		// warns reports whether the server must say that the file is readable
		// by others.
		warns bool
	}{
		{name: "readable by everyone", mode: 0o644, warns: true},
		{name: "readable by the group", mode: 0o640, warns: true},
		{name: "executable by the group only", mode: 0o610, warns: true},
		{name: "readable by its owner only", mode: 0o600},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := writeTokenFileWithMode(t, validToken+"\n", tc.mode)

			s := startServer(t, loopbackListen, nil, "--token-file", path)

			req := newRequest(t, http.MethodGet, "/v1/targets", nil)
			req.Header.Set("Authorization", "Bearer "+validToken)

			if got := s.do(t, req); got.status != http.StatusOK {
				t.Fatalf("GET /v1/targets with the token from the file = %d, want %d; body: %q",
					got.status, http.StatusOK, got.body)
			}

			logged := s.stderr.String()

			if strings.Contains(logged, validToken) {
				t.Errorf("stderr = %q, want it to quote no token", logged)
			}

			if !tc.warns {
				if strings.Contains(logged, "warning:") {
					t.Errorf("stderr = %q, want no warning for a token file only its owner can read", logged)
				}

				return
			}

			line := s.waitForLine(t, errorPrefix+"warning: ")

			if !strings.Contains(line, path) {
				t.Errorf("the warning line %q does not name the token file %s", line, path)
			}
		})
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

// writeTokenFileWithMode writes content to a token file in a temporary directory
// and gives it the permission bits of mode. os.WriteFile's perm is narrowed by
// the process umask, so the mode is set afterwards, which no umask applies to.
func writeTokenFileWithMode(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()

	path := writeTokenFile(t, content)

	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("setting the token file's mode to %v: %v", mode, err)
	}

	return path
}

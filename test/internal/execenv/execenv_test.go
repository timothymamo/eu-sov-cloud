package execenv_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/timothymamo/shift2eu/test/internal/execenv"
)

// These tests pin the two rules of issue #195 on probe sources under testdata
// (their .go.txt names, not .go, keep the go command, golangci-lint and
// test/lint's own scan of every Go file under test/ away from them, and keep
// the environ probes from tripping that scan). Each probe states one way an
// e2e test could hand a child the whole environment of the test process, where
// the agents' keys and tokens live (CLAUDE.md rule 3), or read it without a
// child. test/lint runs the checkers over the repository in
// TestE2ETestsSetExecEnv and TestTestTreeNamesNoProcEnviron.
//
// A finding's line is the line a reader fixes: the Run, Start, Output or
// CombinedOutput call for CheckExecEnv, the literal for
// CheckEnvironLiterals. Its message is not pinned, only that it says
// something, so the wording can change without touching a probe.

// noExemption refuses no command name, the exemption function of a caller that
// lets nothing inherit the environment.
func noExemption(string) bool { return false }

// toolchainExemption admits the go toolchain's command name and no other, as
// test/lint's rule does for the `go build` and `go list` calls of the e2e tests.
func toolchainExemption(name string) bool { return name == "go" }

func TestCheckExecEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		probe  string
		exempt func(string) bool
		want   []execenv.Finding
		// wantErr is set for a probe that does not parse: both checkers read
		// the file as Go and return the parse error.
		wantErr bool
	}{
		{
			name:  "Run, Start, Output and CombinedOutput are each refused on a command with no Env",
			probe: "methods.go.txt",
			want:  find("methods.go.txt", 7, 10, 13, 16),
		},
		{
			name:  "a command built by CommandContext is refused as a command built by Command is",
			probe: "commandcontext.go.txt",
			want:  find("commandcontext.go.txt", 10),
		},
		{
			name:  "an Env assigned after the call comes too late",
			probe: "env_after.go.txt",
			want:  find("env_after.go.txt", 7),
		},
		{
			name:  "a chained Command call is refused, though it binds no name to hold an Env",
			probe: "chained.go.txt",
			want:  find("chained.go.txt", 6),
		},
		{
			name:  "an Env assigned before the call is accepted",
			probe: "env_set.go.txt",
		},
		{
			name:   "an exempt command is accepted with no Env",
			probe:  "toolchain.go.txt",
			exempt: toolchainExemption,
		},
		{
			name:  "an unexempted go command is refused, and CommandContext's first argument is its name",
			probe: "toolchain.go.txt",
			want:  find("toolchain.go.txt", 10, 15),
		},
		{
			name:  "the Env of another command in the same function does not count",
			probe: "other_env.go.txt",
			want:  find("other_env.go.txt", 10),
		},
		{
			name:  "a command whose name is no literal is refused, whatever the exemption admits",
			probe: "name_variable.go.txt",
			want:  find("name_variable.go.txt", 7),
		},
		{
			// exec.Command hands back a fresh *exec.Cmd, with a nil Env, so an
			// Env assigned before the variable was rebuilt says nothing about
			// the command that runs.
			name:  "an Env assigned before the command was rebuilt is refused",
			probe: "reassigned.go.txt",
			want:  find("reassigned.go.txt", 10),
		},
		{
			name:    "a source that does not parse is an error, not a finding",
			probe:   "syntaxerror.go.txt",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src, err := os.ReadFile(filepath.Join("testdata", tc.probe))
			if err != nil {
				t.Fatalf("read the probe: %v", err)
			}

			exempt := tc.exempt
			if exempt == nil {
				exempt = noExemption
			}

			findings, err := execenv.CheckExecEnv("testdata/"+tc.probe, src, exempt)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("CheckExecEnv(%s) = %v, want an error", tc.probe, findings)
				}
				return
			}
			if err != nil {
				t.Fatalf("CheckExecEnv(%s): %v", tc.probe, err)
			}

			for _, f := range findings {
				if f.Msg == "" {
					t.Errorf("finding %s:%d gives no message, want one that says what to fix", f.File, f.Line)
				}
			}

			// EquateEmpty: a probe with nothing to refuse may come back as nil
			// or as an empty slice.
			if diff := cmp.Diff(tc.want, findings, cmpopts.IgnoreFields(execenv.Finding{}, "Msg"), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("findings for %s (-want +got):\n%s", tc.probe, diff)
			}
		})
	}
}

func TestCheckEnvironLiterals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		probe   string
		want    []execenv.Finding
		wantErr bool
	}{
		{
			name:  "self, a pid and thread-self are each refused",
			probe: "environ_literals.go.txt",
			want:  find("environ_literals.go.txt", 5, 6, 7),
		},
		{
			name:  "another /proc path of the same process is accepted",
			probe: "environ_status.go.txt",
		},
		{
			name:  "the path in a comment is no literal and is accepted",
			probe: "environ_comment.go.txt",
		},
		{
			name:    "a source that does not parse is an error, not a finding",
			probe:   "syntaxerror.go.txt",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src, err := os.ReadFile(filepath.Join("testdata", tc.probe))
			if err != nil {
				t.Fatalf("read the probe: %v", err)
			}

			findings, err := execenv.CheckEnvironLiterals("testdata/"+tc.probe, src)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("CheckEnvironLiterals(%s) = %v, want an error", tc.probe, findings)
				}
				return
			}
			if err != nil {
				t.Fatalf("CheckEnvironLiterals(%s): %v", tc.probe, err)
			}

			for _, f := range findings {
				if f.Msg == "" {
					t.Errorf("finding %s:%d gives no message, want one that says what to fix", f.File, f.Line)
				}
			}

			if diff := cmp.Diff(tc.want, findings, cmpopts.IgnoreFields(execenv.Finding{}, "Msg"), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("findings for %s (-want +got):\n%s", tc.probe, diff)
			}
		})
	}
}

// find returns the findings a probe file is expected to produce, one per line,
// in line order.
func find(probe string, lines ...int) []execenv.Finding {
	findings := make([]execenv.Finding, 0, len(lines))
	for _, line := range lines {
		findings = append(findings, execenv.Finding{File: "testdata/" + probe, Line: line})
	}
	return findings
}

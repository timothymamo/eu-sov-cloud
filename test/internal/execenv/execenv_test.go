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
// The rule fails closed: a command the checker cannot follow, a use of it it
// cannot vouch for, and an import it cannot name are findings too. Each probe
// below is one such shape, and the accepted ones are the shapes the tier really
// writes.
//
// A finding's line is the line a reader fixes: the Run, Start, Output or
// CombinedOutput call for CheckExecEnv, the call, assignment or import that
// hands the command on, and the literal for CheckEnvironLiterals. Its message
// is not pinned, only that it says something, so the wording can change
// without touching a probe.

// noExemption refuses no command name, the exemption function of a caller that
// lets nothing inherit the environment.
func noExemption(string, string) bool { return false }

// toolchainExemption admits the go toolchain's two commands whose calls in the
// e2e tier inherit the environment because the go command needs it (`go build`
// and `go list`), keyed on the name and the subcommand after it, as test/lint's
// rule is. `go run` and `go env` print or hand on that environment, so they are
// no part of it, and neither is a name or a subcommand the source builds at run
// time.
func toolchainExemption(name, sub string) bool {
	return name == "go" && (sub == "build" || sub == "list")
}

func TestCheckExecEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		probe  string
		exempt func(name, sub string) bool
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
			// The e2e tier's runs assign Dir, Stdout and Stderr before Env
			// (inventory_guide_test.go's runJQ), which hands the checker no
			// reason to doubt the command.
			name:  "fields other than Env assigned before Env are accepted",
			probe: "field_before_env.go.txt",
		},
		{
			// The exemption is keyed on the name and the subcommand, so it
			// admits both commands the e2e tests start with the whole
			// environment and nothing else the go command does.
			name:   "an exempt go build and go list are accepted with no Env",
			probe:  "toolchain.go.txt",
			exempt: toolchainExemption,
		},
		{
			name:  "an unexempted go command is refused, and CommandContext's first argument is its name",
			probe: "toolchain.go.txt",
			want:  find("toolchain.go.txt", 10, 15),
		},
		{
			name:   "a go subcommand the exemption does not name is refused",
			probe:  "subcommand_run.go.txt",
			exempt: toolchainExemption,
			want:   find("subcommand_run.go.txt", 7),
		},
		{
			name:   "go env is refused, the exemption naming build and list alone",
			probe:  "subcommand_env.go.txt",
			exempt: toolchainExemption,
			want:   find("subcommand_env.go.txt", 7),
		},
		{
			name:   "a go subcommand the source builds at run time is refused",
			probe:  "subcommand_variable.go.txt",
			exempt: toolchainExemption,
			want:   find("subcommand_variable.go.txt", 7),
		},
		{
			name:   "a go call with no subcommand is refused",
			probe:  "subcommand_missing.go.txt",
			exempt: toolchainExemption,
			want:   find("subcommand_missing.go.txt", 7),
		},
		{
			// The Path is the command the child runs, so a go build whose
			// Path now names something else is no go build: the exemption
			// covers the command the source wrote, not the one this call
			// starts.
			name:   "a Path assigned on an exempt command takes the exemption away",
			probe:  "path_reassigned.go.txt",
			exempt: toolchainExemption,
			want:   find("path_reassigned.go.txt", 8),
		},
		{
			name:   "Args assigned on an exempt command take the exemption away",
			probe:  "args_reassigned.go.txt",
			exempt: toolchainExemption,
			want:   find("args_reassigned.go.txt", 8),
		},
		{
			// A composite literal is a command the checker never saw built:
			// it states its own Path and, with no Env key, inherits the
			// environment.
			name:  "a composite literal of exec.Cmd with no Env is refused, with the value and with the pointer",
			probe: "composite.go.txt",
			want:  find("composite.go.txt", 6, 7),
		},
		{
			name:  "a composite literal of exec.Cmd with an Env key is accepted",
			probe: "composite_env.go.txt",
		},
		{
			name:  "a blank import of os/exec is refused, not silently skipped",
			probe: "blank_import.go.txt",
			want:  find("blank_import.go.txt", 3),
		},
		{
			name:  "a dot import of os/exec is refused, not silently skipped",
			probe: "dot_import.go.txt",
			want:  find("dot_import.go.txt", 3),
		},
		{
			name:  "a command passed straight to a helper is refused at the construction",
			probe: "helper_argument.go.txt",
			want:  find("helper_argument.go.txt", 6),
		},
		{
			name:  "a command stored in a slice element is refused at the construction",
			probe: "slice_element.go.txt",
			want:  find("slice_element.go.txt", 6),
		},
		{
			name:  "a command assigned to a struct field is refused at the construction",
			probe: "struct_field_target.go.txt",
			want:  find("struct_field_target.go.txt", 10),
		},
		{
			name:  "a command copied to another name before its Env is set is refused at the copy",
			probe: "alias.go.txt",
			want:  find("alias.go.txt", 7),
		},
		{
			name:  "a method value taken before Env is set is refused",
			probe: "method_value.go.txt",
			want:  find("method_value.go.txt", 7),
		},
		{
			name:  "a method expression called on a command before Env is set is refused",
			probe: "method_expression.go.txt",
			want:  find("method_expression.go.txt", 7),
		},
		{
			name:  "a closure that captures a command before Env is set is refused",
			probe: "closure_capture.go.txt",
			want:  find("closure_capture.go.txt", 7),
		},
		{
			name:  "a parenthesised receiver is unwrapped and refused",
			probe: "paren_receiver.go.txt",
			want:  find("paren_receiver.go.txt", 7),
		},
		{
			// The outer cmd is the command that runs, whatever an inner block
			// did to a name of its own.
			name:  "a shadowing command in an inner block sets no Env on the outer one",
			probe: "shadowing.go.txt",
			want:  find("shadowing.go.txt", 12),
		},
		{
			// A nil Env is the value that inherits the environment, so
			// assigning it is no assignment.
			name:  "Env assigned nil, or as a nil slice, is refused",
			probe: "env_nil.go.txt",
			want:  find("env_nil.go.txt", 8, 12),
		},
		{
			// The accepted gap: the checker follows the source, not the
			// value, so an Env assigned anywhere in the function counts.
			name:  "an Env assigned in a branch that never runs is accepted, the check following the source",
			probe: "env_if_false.go.txt",
		},
		{
			// The shape server_test.go's startServerBinary has: Env is set
			// first, and the command is then stored, captured and started.
			name:  "a command used after its Env is set is accepted, stored in a struct and captured too",
			probe: "struct_after_env.go.txt",
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
			name:  "a function literal in a package-level var is a function of its own and is checked",
			probe: "funclit_var.go.txt",
			want:  find("funclit_var.go.txt", 7),
		},
		{
			name:  "a function literal nested in a function body is checked as a function of its own",
			probe: "funclit_nested.go.txt",
			want:  find("funclit_nested.go.txt", 8),
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
		name  string
		probe string
		// tier is the part of the tree the file is read in: TierTest is the
		// zero value, so a case that names none is held to the whole-value
		// rule of the rest of test/.
		tier    execenv.Tier
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
			// The widened rule reads path.Clean(value), so a path that only
			// reaches the same file through a doubled slash, a dot segment,
			// a deeper /proc entry or no leading slash is refused too, and a
			// raw string is read as the quoted one.
			name:  "the whole-value rule reads a cleaned path, so a nested, doubled, dotted or relative one is refused",
			probe: "environ_tiers.go.txt",
			want:  find("environ_tiers.go.txt", 5, 10, 11, 12, 13, 14),
		},
		{
			// The e2e tier has no fixture literals, so it holds every literal
			// to the segment: no path into /proc and none into /dev/fd, /proc
			// itself and /proc/self/status included.
			name:  "the e2e tier refuses any literal with an environment path segment",
			probe: "environ_tiers.go.txt",
			tier:  execenv.TierE2E,
			want:  find("environ_tiers.go.txt", 5, 6, 7, 8, 10, 11, 12, 13, 14),
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

			findings, err := execenv.CheckEnvironLiterals("testdata/"+tc.probe, src, tc.tier)
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
				t.Errorf("findings for %s in tier %v (-want +got):\n%s", tc.probe, tc.tier, diff)
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

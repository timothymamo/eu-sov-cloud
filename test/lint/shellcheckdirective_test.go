package lint_test

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/test/lint"
)

// This file is the failing half of issue #164: a shell script a reader of this
// repository can run must not silence ShellCheck without saying which checks it
// silences and why, the way nolintlint (nolint_test.go) refuses a bare //nolint
// directive in Go. It does not compile until the coder has added the two
// functions below to test/lint/shellcheckdirective.go, and the coder must not
// change this file.
//
// test/lint has only doc.go and _test.go files, so the checked code needs a
// non-test file of its own, test/lint/shellcheckdirective.go, in package lint.
// The two functions are exported because every test in this directory is in the
// external lint_test package (CLAUDE.md, Testing). The coder adds exactly:
//
//	// CheckShellcheckDirectives returns what in src, the content of the file
//	// at path, breaks the rule that a shellcheck disable directive names the
//	// codes it silences and gives its reason, one line each and in line
//	// order, each of the form "path:line: <why>"; nil when nothing does.
//	func CheckShellcheckDirectives(path string, src []byte) []string
//
//	// IsShellScript reports whether the tracked file at path, with content
//	// src, is a shell script: its name ends in .sh, or its first line is a
//	// #! shebang that runs sh or bash, directly or through env, whose
//	// options and VAR=value assignments are skipped.
//	func IsShellScript(path string, src []byte) bool
//
// What the coder must implement, as ShellCheck reads it: a shellcheck
// directive is a comment whose text, with the leading "#" and the spaces after
// it removed, starts with the word "shellcheck" (so prose that merely mentions
// shellcheck is not a directive, and neither is a directive on a later line
// than the "#"). The fields after that word, separated by spaces, are
// key=value directives; one of them may be "disable", and only that field is
// checked here:
//
//   - Its value is one or more codes separated by commas, and every code is
//     exactly "SC" and four digits. The codes are compared exactly, so a
//     space around a code, a lower-case code, "SC208", "SC20866" and "all" are
//     all refused. An empty value ("disable=") is refused too: it names no
//     code.
//   - The reason is the text of the comment after the next "#" on the same
//     line, with the spaces around it and a trailing "\r" removed, and it must
//     not be empty: a directive without that second "#", with "#" and nothing
//     after it, and with "#" and only spaces after it, is refused.
//
// A directive that names another key (shell=bash, source=lib.sh, ...) and no
// disable field is not a violation, and neither is a disable field with a
// reason in a comment that also sets one. A line is one directive: a reason on
// the line after a directive is no reason.
//
// The message after "path:line:" is the coder's wording; these tests pin which
// directive is refused, never how it is explained.

// location returns the "path:line" a problem line reports, whatever column and
// wording the checker puts after it.
func location(problem string) string {
	fields := strings.Split(problem, ":")
	if len(fields) < 2 {
		return problem
	}
	return fields[0] + ":" + fields[1]
}

// locations maps problem lines to the lines they point at, nil when there is
// none, so a checker that returns an empty slice for no problem compares equal
// to a nil want.
func locations(problems []string) []string {
	if len(problems) == 0 {
		return nil
	}
	found := make([]string, 0, len(problems))
	for _, problem := range problems {
		found = append(found, location(problem))
	}
	return found
}

func TestCheckShellcheckDirectives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		src  string
		want []string // the "path:line" of each refused directive, in order
	}{
		{
			name: "a code and a reason",
			src:  "# shellcheck disable=SC2086 # word splitting is intended: FLAGS is a list\n",
		},
		{
			name: "several codes and a reason",
			src:  "# shellcheck disable=SC2086,SC2034 # both are intended here\n",
		},
		{
			name: "a directive indented in a script",
			src:  "#!/bin/sh\n\nif true; then\n  # shellcheck disable=SC1091 # the path is resolved at run time\n  . ./lib.sh\nfi\n",
		},
		{
			name: "a directive trailing a line of code",
			src:  "eval \"$cmd\" # shellcheck disable=SC2086 # the list must split\n",
		},
		{
			name: "another key alone",
			src:  "# shellcheck source=lib.sh\n",
		},
		{
			name: "the shell key alone",
			src:  "# shellcheck shell=bash\n",
		},
		{
			name: "another key beside a reasoned disable",
			src:  "# shellcheck shell=bash disable=SC2086 # a generated script\n",
		},
		{
			name: "prose that mentions shellcheck",
			src:  "# shellcheck is the linter this rule comes from\n",
		},
		{
			name: "prose before a directive",
			src:  "# See shellcheck disable=all in the ShellCheck manual\n",
		},
		{
			name: "a directive in a CRLF file",
			src:  "#!/bin/sh\r\n# shellcheck disable=SC2086 # the list must split\r\n",
		},
		{
			name: "nothing to check",
			src:  "#!/bin/sh\necho hello # nothing here is a directive\n",
		},
		{
			name: "all",
			src:  "# shellcheck disable=all\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "all with a reason",
			src:  "# shellcheck disable=all # the whole file is generated\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "no code",
			src:  "# shellcheck disable=\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "no code with a reason",
			src:  "# shellcheck disable= # nothing to name\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "a code too short",
			src:  "# shellcheck disable=SC208 # a digit short\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "a code too long",
			src:  "# shellcheck disable=SC20866 # a digit too many\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "a lower-case code",
			src:  "# shellcheck disable=sc2086 # lower case\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "a code of another tool",
			src:  "# shellcheck disable=DL3008 # apt pinning\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "a good code beside all",
			src:  "# shellcheck disable=SC2086,all # mixed\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "a space in the list",
			src:  "# shellcheck disable=SC2086, SC2034 # a space after the comma\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "no reason",
			src:  "# shellcheck disable=SC2086\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "no reason on a line of code",
			src:  "eval \"$cmd\" # shellcheck disable=SC2086\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "an empty reason",
			src:  "# shellcheck disable=SC2086 #\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "a reason of spaces",
			src:  "# shellcheck disable=SC2086 #   \n",
			want: []string{"a.sh:1"},
		},
		{
			name: "an empty reason in a CRLF file",
			src:  "#!/bin/sh\r\n# shellcheck disable=SC2086 #\r\n",
			want: []string{"a.sh:2"},
		},
		{
			name: "a reason on the next line",
			src:  "# shellcheck disable=SC2086\n# word splitting is intended here\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "a bad directive beside another key",
			src:  "# shellcheck disable=SC2086 shell=bash\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "every refused directive in line order",
			path: "dev/example.sh",
			src: "#!/usr/bin/env bash\n" +
				"# shellcheck disable=SC2086\n" +
				"# shellcheck disable=SC2086 # word splitting is intended\n" +
				"# shellcheck disable=all # everything\n" +
				"# shellcheck disable= # nothing\n",
			want: []string{"dev/example.sh:2", "dev/example.sh:4", "dev/example.sh:5"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := tc.path
			if path == "" {
				path = "a.sh"
			}
			if got := locations(lint.CheckShellcheckDirectives(path, []byte(tc.src))); !slices.Equal(got, tc.want) {
				t.Errorf("CheckShellcheckDirectives(%q, ...):\n%s", path, cmp.Diff(tc.want, got))
			}
		})
	}
}

func TestIsShellScript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		src  string
		want bool
	}{
		{name: "the .sh extension", path: "dev/worktree.sh", src: "#!/usr/bin/env bash\n", want: true},
		{name: "the .sh extension without a shebang", path: "dev/worktree.sh", src: "echo hello\n", want: true},
		{name: "a sh shebang", path: ".githooks/pre-commit", src: "#!/bin/sh\n", want: true},
		{name: "an env bash shebang", path: ".githooks/pre-push", src: "#!/usr/bin/env bash\n", want: true},
		{name: "a bash shebang with options", path: ".githooks/pre-commit", src: "#!/bin/bash -eu\n", want: true},
		{name: "an env shebang with options", path: ".githooks/pre-commit", src: "#!/usr/bin/env -S bash -eu\n", want: true},
		{name: "an env shebang with an assignment", path: ".githooks/pre-commit", src: "#!/usr/bin/env LC_ALL=C sh\n", want: true},
		{name: "a sh shebang on a later line", path: ".githooks/pre-commit", src: "echo hello\n#!/bin/sh\n", want: false},
		{name: "another shell", path: ".githooks/pre-commit", src: "#!/bin/dash\n", want: false},
		{name: "another language", path: ".githooks/pre-commit", src: "#!/usr/bin/env python3\n", want: false},
		{name: "no shebang and no extension", path: "Makefile", src: "all:\n", want: false},
		{name: "the extension elsewhere", path: "docs/notes.sh.txt", src: "# notes\n", want: false},
		{name: "not a shebang", path: ".githooks/pre-commit", src: "# !/bin/sh\n", want: false},
		{name: "an empty file", path: ".githooks/pre-commit", src: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := lint.IsShellScript(tc.path, []byte(tc.src)); got != tc.want {
				t.Errorf("IsShellScript(%q, %q) = %t, want %t", tc.path, tc.src, got, tc.want)
			}
		})
	}
}

// TestTrackedShellScriptsNameTheirShellcheckCodes reads every shell script git
// tracks, IsShellScript deciding which one is a script: the files ending in
// .sh and the files whose first line runs sh or bash, the hooks under .githooks
// among them. Each shellcheck disable directive in one of them must name the
// codes it silences and give its reason (issue #164); a directive that would
// silence every check, or name none, hides a finding no reader can look up.
func TestTrackedShellScriptsNameTheirShellcheckCodes(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)

	// -z keeps a path with a space in it whole, as test/architecture reads the
	// index too.
	out, err := exec.CommandContext(t.Context(), "git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("git ls-files in %s: %v", root, err)
	}

	var checked, withSuffix []string
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" {
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // tracked but not in this checkout, as a sparse one has it
			}
			t.Fatalf("read %s: %v", rel, err)
		}
		if strings.HasSuffix(rel, ".sh") {
			withSuffix = append(withSuffix, rel)
		}
		if !lint.IsShellScript(rel, src) {
			continue
		}
		checked = append(checked, rel)
		for _, problem := range lint.CheckShellcheckDirectives(rel, src) {
			t.Error(problem)
		}
	}

	// Guard the walk itself: a repository with no script in scope, or a scan
	// that skipped the .sh files, would pass this test whatever the scripts
	// say.
	if len(checked) == 0 {
		t.Fatal("found no tracked shell script to check; IsShellScript or the git walk is broken")
	}
	for _, rel := range withSuffix {
		if !slices.Contains(checked, rel) {
			t.Errorf("the scan skipped %s, a tracked .sh file", rel)
		}
	}
	for _, rel := range []string{".githooks/pre-commit", ".githooks/pre-push"} {
		if !slices.Contains(checked, rel) {
			t.Errorf("the scan skipped %s, whose #!/bin/sh shebang puts it in scope", rel)
		}
	}
	t.Logf("checked %d tracked shell scripts", len(checked))
}

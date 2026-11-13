package shellcheck_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/test/internal/shellcheck"
)

// These tests pin the rule of issue #164: a shell script a reader of this
// repository can run must not silence ShellCheck without saying which checks it
// silences and why, the way nolintlint (test/lint/nolint_test.go) refuses a
// bare //nolint directive in Go. test/lint's
// TestTrackedShellScriptsNameTheirShellcheckCodes and
// TestTrackedWorkflowsNameTheirShellcheckCodes run the checker over the
// repository. It lived in test/lint until issue #177 moved it here: test/lint
// holds doc.go and _test.go files only, under the strict test-docs-are-pure
// rule, and test/architecture imports IsShellScript too.
//
// What the checker implements, as ShellCheck reads it: a shellcheck
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
// The message after "path:line:" is the checker's wording; these tests pin which
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

func TestCheckDirectives(t *testing.T) {
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
		// The shell starts a comment at the beginning of any word, so a
		// directive after an operator, or after a "#" inside a word the
		// shell quotes, is a directive all the same, and a good one must
		// not be refused.
		{
			name: "a directive after a semicolon",
			src:  "true;# shellcheck disable=SC2086 # the list must split\n",
		},
		{
			name: "a directive after a double ampersand",
			src:  "true &&# shellcheck disable=SC2086 # the list must split\n",
		},
		{
			name: "a directive after a pipe",
			src:  "x |# shellcheck disable=SC2086 # the list must split\n",
		},
		{
			name: "a directive after a parenthesis",
			src:  "(# shellcheck disable=SC2086 # a subshell of its own\n",
		},
		{
			name: "a directive after a quoted hash",
			src:  "x=\"a #b\"; echo $1 # shellcheck disable=SC2086 # the list must split\n",
		},
		{
			name: "a quoted hash and a reason holding one",
			src:  "x=\"a #b\"; echo $1 # shellcheck disable=SC2086 # FLAGS # is a list\n",
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
			name: "all after a semicolon",
			src:  "true;# shellcheck disable=all # the whole file is generated\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "no reason after a double ampersand",
			src:  "a &&# shellcheck disable=SC2086\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "no reason after a parenthesis",
			src:  "(# shellcheck disable=SC2086\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "no code after a pipe",
			src:  "x |# shellcheck disable=\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "a bad directive after a quoted hash",
			src:  "x=\"a #b\"; echo $1 # shellcheck disable=all # the whole file is generated\n",
			want: []string{"a.sh:1"},
		},
		{
			name: "a directive in the text of a workflow run block",
			path: ".github/workflows/ci.yml",
			src: "      - name: shellcheck the tracked shell scripts\n" +
				"        run: |\n" +
				"          # shellcheck disable=all # the workflow is generated\n" +
				"          dev/shellcheck.sh\n",
			want: []string{".github/workflows/ci.yml:3"},
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
			if got := locations(shellcheck.CheckDirectives(path, []byte(tc.src))); !slices.Equal(got, tc.want) {
				t.Errorf("CheckDirectives(%q, ...):\n%s", path, cmp.Diff(tc.want, got))
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

			if got := shellcheck.IsShellScript(tc.path, []byte(tc.src)); got != tc.want {
				t.Errorf("IsShellScript(%q, %q) = %t, want %t", tc.path, tc.src, got, tc.want)
			}
		})
	}
}

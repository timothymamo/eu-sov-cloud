package lint_test

import (
	"regexp"
	"strings"
	"testing"
)

// guardrails are the linters whose issues are fixed, never silenced: depguard
// enforces the dependency rule (CLAUDE.md), and forbidigo's cloud-read-only
// group keeps a write call out of the source adapters (CLAUDE.md rule 1).
// checkSource refuses a nolint directive that would hide an issue of either.
var guardrails = []string{"depguard", "forbidigo"}

// directive matches what golangci-lint reads as a nolint directive once it has
// trimmed the comment's leading slashes and spaces (pkg/result/processors/
// nolint_filter.go in golangci-lint v2). The text is lowercased first, so an
// upper-case directive is refused too, in case a later release reads it.
var directive = regexp.MustCompile(`^nolint( |:|$)`)

// silences reports whether golangci-lint reads comment, the text of one
// comment, as a nolint directive that hides linter's issues on the lines it
// covers. golangci-lint reads a directive that names no linter, any directive
// whose list starts with all, and any list that holds all as silencing every
// linter, and lowercases and trims each name in the list.
func silences(comment, linter string) bool {
	text := strings.ToLower(strings.TrimLeft(comment, "/ "))
	if !directive.MatchString(text) {
		return false
	}
	names, specific := strings.CutPrefix(text, "nolint:")
	if !specific || strings.HasPrefix(names, "all") {
		return true
	}
	names, _, _ = strings.Cut(names, "//") // the explanation follows a second //
	for name := range strings.SplitSeq(names, ",") {
		switch strings.TrimSpace(name) {
		case "all", linter:
			return true
		}
	}
	return false
}

func TestSilences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		comment string
		linter  string
		want    bool
	}{
		{name: "names depguard", comment: "//nolint:depguard // probe", linter: "depguard", want: true},
		{name: "names depguard without a reason", comment: "//nolint:depguard", linter: "depguard", want: true},
		{name: "names depguard in a list", comment: "//nolint:gosec,depguard // probe", linter: "depguard", want: true},
		{name: "names depguard after a space", comment: "//nolint:gosec, depguard // probe", linter: "depguard", want: true},
		{name: "names depguard in another case", comment: "//nolint:DepGuard // probe", linter: "depguard", want: true},
		{name: "leading space", comment: "// nolint:depguard // probe", linter: "depguard", want: true},
		{name: "also silences nolintlint", comment: "//nolint:depguard,nolintlint // probe", linter: "depguard", want: true},
		{name: "upper-case directive", comment: "//NOLINT:depguard // probe", linter: "depguard", want: true},
		{name: "names forbidigo", comment: "//nolint:forbidigo // probe", linter: "forbidigo", want: true},
		{name: "names forbidigo first in a list", comment: "//nolint:forbidigo,gosec // probe", linter: "forbidigo", want: true},
		{name: "names forbidigo after a space", comment: "//nolint: forbidigo // probe", linter: "forbidigo", want: true},
		{name: "names forbidigo in another case", comment: "//nolint:ForbidIgo // probe", linter: "forbidigo", want: true},
		{name: "bare", comment: "//nolint", linter: "depguard", want: true},
		{name: "bare with a reason", comment: "//nolint // probe", linter: "forbidigo", want: true},
		{name: "names all", comment: "//nolint:all // probe", linter: "forbidigo", want: true},
		{name: "names all after a space", comment: "//nolint: all // probe", linter: "depguard", want: true},
		{name: "names all in a list", comment: "//nolint:wsl,all // probe", linter: "forbidigo", want: true},
		{name: "list starts with all", comment: "//nolint:allx // probe", linter: "depguard", want: true},
		{name: "names the other guardrail", comment: "//nolint:forbidigo // probe", linter: "depguard", want: false},
		{name: "a longer name", comment: "//nolint:forbidigox // probe", linter: "forbidigo", want: false},
		{name: "names another linter", comment: "//nolint:gosec // a test golden file, not user input", linter: "depguard", want: false},
		{name: "names other linters", comment: "//nolint:gosec,revive // probe", linter: "forbidigo", want: false},
		{name: "reason mentions depguard", comment: "//nolint:gosec // not depguard", linter: "depguard", want: false},
		{name: "reason mentions forbidigo", comment: "//nolint:gosec // not forbidigo", linter: "forbidigo", want: false},
		{name: "prose", comment: "// Do not add //nolint:depguard; fix the design.", linter: "depguard", want: false},
		{name: "nolintlint", comment: "//nolintlint checks nolint directives", linter: "depguard", want: false},
		// golangci-lint reads no directive in a block comment.
		{name: "block comment", comment: "/* nolint:forbidigo */", linter: "forbidigo", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := silences(tc.comment, tc.linter); got != tc.want {
				t.Errorf("silences(%q, %q) = %t, want %t", tc.comment, tc.linter, got, tc.want)
			}
		})
	}
}

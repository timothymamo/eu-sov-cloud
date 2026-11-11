package lint_test

import (
	"regexp"
	"strings"
	"testing"
)

// directive matches what golangci-lint reads as a nolint directive once it has
// trimmed the comment's leading slashes and spaces (pkg/result/processors/
// nolint_filter.go in golangci-lint v2). The text is lowercased first, so an
// upper-case directive is refused too, in case a later release reads it.
var directive = regexp.MustCompile(`^nolint( |:|$)`)

// silencesDepguard reports whether golangci-lint reads comment, the text of one
// comment, as a nolint directive that hides depguard's issues on the lines it
// covers. golangci-lint reads a directive that names no linter, any directive
// whose list starts with all, and any list that holds all as silencing every
// linter, and lowercases and trims each name in the list.
func silencesDepguard(comment string) bool {
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
		case "all", "depguard":
			return true
		}
	}
	return false
}

func TestSilencesDepguard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		comment string
		want    bool
	}{
		{name: "names depguard", comment: "//nolint:depguard // probe", want: true},
		{name: "names depguard without a reason", comment: "//nolint:depguard", want: true},
		{name: "names depguard in a list", comment: "//nolint:gosec,depguard // probe", want: true},
		{name: "names depguard after a space", comment: "//nolint:gosec, depguard // probe", want: true},
		{name: "names depguard in another case", comment: "//nolint:DepGuard // probe", want: true},
		{name: "leading space", comment: "// nolint:depguard // probe", want: true},
		{name: "also silences nolintlint", comment: "//nolint:depguard,nolintlint // probe", want: true},
		{name: "upper-case directive", comment: "//NOLINT:depguard // probe", want: true},
		{name: "bare", comment: "//nolint", want: true},
		{name: "bare with a reason", comment: "//nolint // probe", want: true},
		{name: "names all", comment: "//nolint:all // probe", want: true},
		{name: "names all after a space", comment: "//nolint: all // probe", want: true},
		{name: "names all in a list", comment: "//nolint:wsl,all // probe", want: true},
		{name: "list starts with all", comment: "//nolint:allx // probe", want: true},
		{name: "names another linter", comment: "//nolint:gosec // a test golden file, not user input", want: false},
		{name: "names other linters", comment: "//nolint:gosec,revive // probe", want: false},
		{name: "reason mentions depguard", comment: "//nolint:gosec // not depguard", want: false},
		{name: "prose", comment: "// Do not add //nolint:depguard; fix the design.", want: false},
		{name: "nolintlint", comment: "//nolintlint checks nolint directives", want: false},
		// golangci-lint reads no directive in a block comment.
		{name: "block comment", comment: "/* nolint:depguard */", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := silencesDepguard(tc.comment); got != tc.want {
				t.Errorf("silencesDepguard(%q) = %t, want %t", tc.comment, got, tc.want)
			}
		})
	}
}

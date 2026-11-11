package lint_test

import (
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoNolintSilencesDepguard fails on every nolint directive, in any Go file
// golangci-lint lints, that would hide a depguard issue. depguard enforces the
// dependency rule; a violation is fixed in the design, never silenced
// (CLAUDE.md).
func TestNoNolintSilencesDepguard(t *testing.T) {
	t.Parallel()

	// go test runs a test in its package's directory, test/lint.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root: %v", err)
	}

	files := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			skip, err := skipDir(path, d.Name())
			if err != nil || !skip {
				return err
			}
			return filepath.SkipDir
		}
		if !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		files++

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, group := range f.Comments {
			for _, c := range group.List {
				if silencesDepguard(c.Text) {
					pos := fset.Position(c.Slash)
					t.Errorf("%s:%d:%d: %s would hide depguard issues; fix the design instead (CLAUDE.md)",
						filepath.ToSlash(rel), pos.Line, pos.Column, c.Text)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if files == 0 {
		t.Fatalf("no Go files found under %s", root)
	}
}

// skipDir reports whether the go command's ./... pattern, which golangci-lint
// lints, leaves the directory out: one whose name starts with . or _, a
// testdata directory, or the root of another module.
func skipDir(path, name string) (bool, error) {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" {
		return true, nil
	}
	_, err := os.Stat(filepath.Join(path, "go.mod"))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

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

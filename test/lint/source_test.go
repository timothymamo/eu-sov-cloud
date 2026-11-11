package lint_test

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestSourcesHideNothingFromDepguard fails on every place, in any Go file
// golangci-lint lints, that would hide code from depguard (see the package
// documentation). depguard enforces the dependency rule; a violation is fixed
// in the design, never hidden (CLAUDE.md).
func TestSourcesHideNothingFromDepguard(t *testing.T) {
	t.Parallel()

	// go test runs a test in its package's directory, test/lint.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root: %v", err)
	}

	problems, files, err := scanSources(root)
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if files == 0 {
		t.Fatalf("no Go files found under %s", root)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// whyNotRegular is the problem with a Go file that is not a regular file.
const whyNotRegular = "is not a regular file: golangci-lint lints a symbolic link under the link's path, " +
	"with the nolint directives of its target, which can lie where test/lint does not look; make it a regular file"

// scanSources reads every Go file the go command's ./... pattern reaches from
// root, whatever its build constraints, so every file any golangci-lint run of
// make lint lints (natively or for a release target, with every build tag
// set), and returns what in them would hide code from depguard, one line each,
// and the number of Go files it read. It refuses a Go file that is not a
// regular file itself and leaves every other check to checkSource.
func scanSources(root string) ([]string, int, error) {
	// Every access goes through an os.Root, so no path can reach outside root.
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, 0, err
	}
	defer r.Close()

	var problems []string
	files := 0
	// fs.WalkDir walks slash-separated paths relative to root, as reported.
	err = fs.WalkDir(r.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			skip, err := skipDir(r, rel, d.Name())
			if err != nil || !skip {
				return err
			}
			return fs.SkipDir
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		if !d.Type().IsRegular() {
			problems = append(problems, rel+": "+whyNotRegular)
			return nil
		}
		files++

		src, err := r.ReadFile(filepath.FromSlash(rel))
		if err != nil {
			return err
		}
		found, err := checkSource(rel, src)
		if err != nil {
			return err
		}
		problems = append(problems, found...)
		return nil
	})
	return problems, files, err
}

// checkSource returns what in src, the content of the Go file at rel, a
// slash-separated path from the module root, would hide code from depguard,
// one line each.
func checkSource(rel string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var problems []string
	for _, group := range f.Comments {
		for _, c := range group.List {
			pos := fset.Position(c.Slash)
			at := fmt.Sprintf("%s:%d:%d: %s", rel, pos.Line, pos.Column, c.Text)
			if silencesDepguard(c.Text) {
				problems = append(problems, at+" would hide depguard issues; fix the design instead (CLAUDE.md)")
			}
			// The go command reads build constraints above the package
			// clause only.
			if c.Pos() < f.Package {
				if why := constraintHides(c.Text); why != "" {
					problems = append(problems, at+" "+why)
				}
			}
		}
	}
	return problems, nil
}

// skipDir reports whether the go command's ./... pattern, which golangci-lint
// lints, leaves the directory out: one whose name starts with . or _, a
// testdata directory, or the root of another module. rel is the directory's
// slash-separated path within r.
func skipDir(r *os.Root, rel, name string) (bool, error) {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" {
		return true, nil
	}
	_, err := r.Stat(filepath.FromSlash(path.Join(rel, "go.mod")))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

func TestScanSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		files    map[string]string // content by slash-separated path
		symlinks map[string]string // target by slash-separated path
		want     []string
	}{
		{
			name: "nothing hidden",
			files: map[string]string{
				"a/a.go":      "package a\n",
				"a/a_test.go": "//go:build e2e\n\npackage a\n\nvar _ = 1 //nolint:gosec // probe\n",
			},
		},
		{
			name:  "nolint directive",
			files: map[string]string{"a/a.go": "package a\n\nimport \"os\" //nolint:depguard // probe\n\nvar _ = os.Getpagesize\n"},
			want: []string{
				"a/a.go:3:13: //nolint:depguard // probe would hide depguard issues; fix the design instead (CLAUDE.md)",
			},
		},
		{
			name:  "negated e2e tag",
			files: map[string]string{"a/a.go": "//go:build !e2e\n\npackage a\n"},
			want:  []string{"a/a.go:1:1: //go:build !e2e " + constraintHides("//go:build !e2e")},
		},
		{
			name:  "negated integration tag in a +build line",
			files: map[string]string{"a/a.go": "// +build !integration\n\npackage a\n"},
			want:  []string{"a/a.go:1:1: // +build !integration " + constraintHides("// +build !integration")},
		},
		{
			name:  "constraint below the package clause",
			files: map[string]string{"a/a.go": "package a\n\n//go:build !e2e\n"},
		},
		{
			name:  "skipped directories",
			files: map[string]string{"_x/a.go": "//go:build !e2e\n\npackage x\n", "a/testdata/a.go": "//go:build !e2e\n\npackage a\n"},
		},
		{
			// golangci-lint lints the link with the directive of its target,
			// which the walk does not read.
			name:     "symbolic link to a Go file",
			files:    map[string]string{"_hidden/zz.go": "package a\n\nimport \"os\" //nolint: all // probe\n"},
			symlinks: map[string]string{"a/zz.go": "../_hidden/zz.go"},
			want:     []string{"a/zz.go: " + whyNotRegular},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			mkdir := func(path string) {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			for name, content := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(name))
				mkdir(path)
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for name, target := range tc.symlinks {
				path := filepath.Join(root, filepath.FromSlash(name))
				mkdir(path)
				if err := os.Symlink(filepath.FromSlash(target), path); err != nil {
					t.Fatal(err)
				}
			}

			got, _, err := scanSources(root)
			if err != nil {
				t.Fatalf("scanSources: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("scanSources mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

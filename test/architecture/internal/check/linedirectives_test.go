package check_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// TestLineDirectives pins which comments are line directives and where they
// are: the two forms the Go compiler reads, "//line " and "/*line ", wherever
// the comment sits on its line, at the physical position in the source rather
// than the one a directive rewrites.
func TestLineDirectives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		want    []string
		wantErr bool
	}{
		{
			name: "a line directive at the start of a line",
			src:  "//line a.go:1\npackage x\n",
			want: []string{"x.go:1:1"},
		},
		{
			name: "a line directive after code on the line",
			src:  "package x //line a.go:1\n",
			want: []string{"x.go:1:11"},
		},
		{
			name: "the issue's block directive at the start of the file",
			src:  "/*line ../../version/version.go:1*/package inventory\n",
			want: []string{"x.go:1:1"},
		},
		{
			name: "a block directive mid-line",
			src:  "var v int /*line a.go:2*/\n",
			want: []string{"x.go:1:11"},
		},
		{
			name: "a block directive after a string literal",
			src:  "var s = \"a\" /*line b.go:2*/\n",
			want: []string{"x.go:1:13"},
		},
		{
			name: "the physical line numbers, not the rewritten ones",
			src:  "package x\n\n//line a.go:1\nvar y int\n\n//line b.go:5\nvar z int\n",
			want: []string{"x.go:3:1", "x.go:6:1"},
		},
		{
			name: "the text in a string literal is not a comment",
			src:  "var s = \"//line a.go:1\"\n",
		},
		{
			name: "the text in a raw string literal is not a comment",
			src:  "var r = `/*line a.go:1*/`\n",
		},
		{
			name: "a space after the slashes is not a directive",
			src:  "// line a.go:1\npackage x\n",
		},
		{
			name: "a longer word is not a directive",
			src:  "//lines\npackage x\n",
		},
		{
			name:    "an unterminated block comment is an error",
			src:     "package x\n/*line a.go:1\n",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := check.LineDirectives("x.go", []byte(tc.src))

			if tc.wantErr {
				if err == nil {
					t.Fatalf("LineDirectives() = %v, nil; want an error", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("LineDirectives(): %v", err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("LineDirectives() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestFindLineDirectives pins which files are scanned: every .go file git
// tracks or would add (untracked and not ignored), in the testdata and _
// directories too, reported under its slash-separated path relative to the
// root, in path order. A symlink among them is reported instead of followed, so
// the repository test fails on one rather than reading a file lint never sees.
// What git ignores and does not track, a local bin/ or dist/ or an editor's
// directory, is not looked at: it is never committed, so CI never builds it.
func TestFindLineDirectives(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	git(t, root, "init", "--quiet")

	directive := "//line a.go:1\npackage x\n"

	for _, file := range []struct{ name, content string }{
		{name: ".gitignore", content: "/bin/\n/dist\nc/\n"},
		{name: "a/x.go", content: directive},
		{name: "testdata/y.go", content: directive},
		{name: "_z/z.go", content: directive},
		{name: "a/notes.txt", content: directive},
		{name: "a/clean.go", content: "package x\n"},
		{name: "c/kept.go", content: directive},
		{name: "gone.go", content: directive},
		{name: "b/untracked.go", content: directive},
		{name: "bin/ignored.go", content: directive},
		{name: "c/ignored.go", content: directive},
	} {
		path := filepath.Join(root, filepath.FromSlash(file.name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("create the directory for %s: %v", file.name, err)
		}

		if err := os.WriteFile(path, []byte(file.content), 0o600); err != nil {
			t.Fatalf("write %s: %v", file.name, err)
		}
	}

	symlinks := []struct{ name, target string }{
		{name: "linkfile", target: filepath.Join(root, "a", "x.go")},
		{name: "linkdir", target: filepath.Join(root, "testdata")},
		{name: "bin/link", target: filepath.Join(root, "a")},
		{name: "dist", target: filepath.Join(root, "a")},
	}

	for _, link := range symlinks {
		if err := os.Symlink(link.target, filepath.Join(root, filepath.FromSlash(link.name))); err != nil {
			t.Fatalf("create the symlink %s: %v", link.name, err)
		}
	}

	// Tracked: c/kept.go although c/ is ignored, the symlink linkfile, and
	// gone.go, removed from the working tree after it was added.
	git(t, root, "add", ".gitignore", "a", "testdata", "_z", "linkfile", "gone.go")
	git(t, root, "add", "--force", "c/kept.go")

	if err := os.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatalf("remove gone.go: %v", err)
	}

	got, err := check.FindLineDirectives(t.Context(), root)
	if err != nil {
		t.Fatalf("FindLineDirectives(): %v", err)
	}

	want := []string{
		"_z/z.go:1:1",
		"a/x.go:1:1",
		"b/untracked.go:1:1",
		"c/kept.go:1:1",
		"linkdir: symlink, not scanned",
		"linkfile: symlink, not scanned",
		"testdata/y.go:1:1",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("FindLineDirectives() mismatch (-want +got):\n%s", diff)
	}
}

// TestFindLineDirectivesOutsideGit pins that the check fails closed: when git
// cannot list the files, FindLineDirectives returns an error rather than
// scanning nothing.
func TestFindLineDirectivesOutsideGit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// A .git file pointing nowhere stops git from looking further up for a
	// repository the temporary directory might sit in.
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: missing\n"), 0o600); err != nil {
		t.Fatalf("write .git: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "x.go"), []byte("//line a.go:1\npackage x\n"), 0o600); err != nil {
		t.Fatalf("write x.go: %v", err)
	}

	got, err := check.FindLineDirectives(t.Context(), root)
	if err == nil {
		t.Fatalf("FindLineDirectives() = %q, nil; want an error", got)
	}
}

// git runs git in dir with the GIT_ variables of the environment left out, so
// a hook that runs the tests cannot point it at another repository.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)

	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

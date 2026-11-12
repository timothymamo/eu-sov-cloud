package check_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// TestNonExecutableScripts pins that a script under a directory, a file whose
// staged content starts with #!, is 100755 in the git index. The Makefile runs
// the scripts under dev/ directly, not through a shell, so one that loses its
// executable bit fails with Permission denied, as dev/claude-settings.sh did
// after a mass rename (issue #125). The check reads the mode and the content
// from the index, never from the working tree: the index is what a commit
// records, and a checkout's file system may not keep modes at all.
func TestNonExecutableScripts(t *testing.T) {
	t.Parallel()

	const script = "#!/usr/bin/env bash\necho hi\n"

	tests := []struct {
		name string
		// setup writes and stages files in the repository root.
		setup func(t *testing.T, root string)
		want  []string
	}{
		{
			name: "an executable script",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeFileContent(t, root, "dev/run.sh", script)
				git(t, root, "add", "dev/run.sh")
				git(t, root, "update-index", "--chmod=+x", "dev/run.sh")
			},
		},
		{
			name: "a script that is not executable",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeFileContent(t, root, "dev/run.sh", script)
				git(t, root, "add", "dev/run.sh")
				git(t, root, "update-index", "--chmod=-x", "dev/run.sh")
			},
			want: []string{"dev/run.sh"},
		},
		{
			name: "a script without the .sh suffix",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeFileContent(t, root, "dev/nested/run", "#!/bin/sh\n")
				git(t, root, "add", "dev/nested/run")
				git(t, root, "update-index", "--chmod=-x", "dev/nested/run")
			},
			want: []string{"dev/nested/run"},
		},
		{
			name: "executable on disk, not in the index",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeFileContent(t, root, "dev/run.sh", script)
				chmod(t, root, "dev/run.sh", 0o700)
				git(t, root, "add", "dev/run.sh")
				git(t, root, "update-index", "--chmod=-x", "dev/run.sh")
			},
			want: []string{"dev/run.sh"},
		},
		{
			name: "executable in the index, not on disk",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeFileContent(t, root, "dev/run.sh", script)
				git(t, root, "add", "dev/run.sh")
				git(t, root, "update-index", "--chmod=+x", "dev/run.sh")
				chmod(t, root, "dev/run.sh", 0o600)
			},
		},
		{
			name: "a script in the index whose working copy lost its shebang",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeFileContent(t, root, "dev/run.sh", script)
				git(t, root, "add", "dev/run.sh")
				git(t, root, "update-index", "--chmod=-x", "dev/run.sh")
				writeFileContent(t, root, "dev/run.sh", "no shebang\n")
			},
			want: []string{"dev/run.sh"},
		},
		{
			name: "data files that are not executable",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeFileContent(t, root, "dev/settings.json", "{}\n")
				writeFileContent(t, root, "dev/lima.yaml", "# #!not a shebang\n")
				writeFileContent(t, root, "dev/empty", "")
				git(t, root, "add", "dev")
			},
		},
		{
			name: "a script outside the directory",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeFileContent(t, root, "scripts/run.sh", script)
				writeFileContent(t, root, "devtools/run.sh", script)
				git(t, root, "add", "scripts/run.sh", "devtools/run.sh")
				git(t, root, "update-index", "--chmod=-x", "scripts/run.sh", "devtools/run.sh")
			},
		},
		{
			name: "an untracked script",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeFileContent(t, root, "dev/run.sh", script)
			},
		},
		{
			name: "several, sorted",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeFileContent(t, root, "dev/b.sh", script)
				writeFileContent(t, root, "dev/a.sh", script)
				writeFileContent(t, root, "dev/ok.sh", script)
				git(t, root, "add", "dev")
				git(t, root, "update-index", "--chmod=-x", "dev/a.sh", "dev/b.sh")
				git(t, root, "update-index", "--chmod=+x", "dev/ok.sh")
			},
			want: []string{"dev/a.sh", "dev/b.sh"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			git(t, root, "init", "--quiet")
			tc.setup(t, root)

			got, err := check.NonExecutableScripts(t.Context(), root, "dev")
			if err != nil {
				t.Fatalf("NonExecutableScripts(): %v", err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("NonExecutableScripts() mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("outside git", func(t *testing.T) {
		t.Parallel()

		// A .git file pointing nowhere stops git from looking further up for a
		// repository, so git fails here.
		root := t.TempDir()
		writeFileContent(t, root, ".git", "gitdir: missing\n")

		if got, err := check.NonExecutableScripts(t.Context(), root, "dev"); err == nil {
			t.Fatalf("NonExecutableScripts() = %q, nil; want an error", got)
		}
	})
}

func chmod(t *testing.T, root, name string, mode os.FileMode) {
	t.Helper()

	if err := os.Chmod(filepath.Join(root, filepath.FromSlash(name)), mode); err != nil {
		t.Fatalf("chmod %s: %v", name, err)
	}
}

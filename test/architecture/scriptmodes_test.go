package architecture_test

import (
	"testing"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// TestScriptsRunDirectlyAreExecutable pins that every script under each
// directory below, a file whose staged content starts with #!, is 100755 in the
// git index. Both directories hold scripts that something runs directly, not
// through a shell, so a script that loses its executable bit fails there with
// Permission denied.
//
// The Makefile runs the scripts under dev/ directly (dev/worktree.sh for make
// worktree, dev/claude-settings.sh in the VM for make vm-claude-settings); the
// rename in #117 lost dev/claude-settings.sh's executable bit and nothing
// noticed (issue #125). The CI workflow runs .github/scripts/release-please-dry-run.sh
// directly too (run: .github/scripts/release-please-dry-run.sh), and it was
// committed as 100644 (issue #142).
//
// The mode is read from the index with git ls-files -s, so the test does not
// depend on what the checkout's file system keeps.
func TestScriptsRunDirectlyAreExecutable(t *testing.T) {
	t.Parallel()

	// dirs are the directories, slash-separated relative to the repository
	// root, whose scripts something runs directly.
	dirs := []string{"dev", ".github/scripts"}

	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			scripts, err := check.NonExecutableScripts(t.Context(), repositoryRoot(t), dir)
			if err != nil {
				t.Fatalf("NonExecutableScripts(%s): %v", dir, err)
			}

			for _, script := range scripts {
				t.Errorf("%s starts with #! but is not 100755 in the git index, so running it directly fails with Permission denied: git update-index --chmod=+x %s", script, script)
			}
		})
	}
}

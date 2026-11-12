package architecture_test

import (
	"testing"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// TestDevScriptsAreExecutable pins that every script under dev/, a file whose
// staged content starts with #!, is 100755 in the git index. The Makefile runs
// them directly, not through a shell (dev/worktree.sh for make worktree,
// dev/claude-settings.sh in the VM for make vm-claude-settings), so a script
// that loses its executable bit fails there with Permission denied. The rename
// in #117 lost dev/claude-settings.sh's, and nothing noticed (issue #125).
// The mode is read from the index with git ls-files -s, so the test does not
// depend on what the checkout's file system keeps.
func TestDevScriptsAreExecutable(t *testing.T) {
	t.Parallel()

	scripts, err := check.NonExecutableScripts(t.Context(), repositoryRoot(t), "dev")
	if err != nil {
		t.Fatalf("NonExecutableScripts(dev): %v", err)
	}

	for _, script := range scripts {
		t.Errorf("%s starts with #! but is not 100755 in the git index, so running it directly fails with Permission denied: git update-index --chmod=+x %s", script, script)
	}
}

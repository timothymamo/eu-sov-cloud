package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// TestStrayLintConfigs pins the golangci-lint documents at the repository root
// that run in place of .golangci.yml. golangci-lint looks its config up through
// viper, which searches .golangci.json, .golangci.toml, .golangci.yaml and
// then .golangci.yml, and it uses the first one it finds: a committed
// .golangci.json, .golangci.toml or .golangci.yaml silently replaces the file
// every other check in this package reads, and lint then runs with the settings
// it sets, forbidigo off and any issue it wants hidden (issue #81). What counts
// is what git could commit, as git ls-files --cached --others
// --exclude-standard lists it: a developer's ignored copy never reaches CI, a
// tracked or force-added one does.
func TestStrayLintConfigs(t *testing.T) {
	t.Parallel()

	names := []string{".golangci.json", ".golangci.toml", ".golangci.yaml"}

	// repo returns a new git repository that ignores .golangci.json, so a case
	// can tell an ignored copy from one git would commit.
	repo := func(t *testing.T) string {
		t.Helper()

		root := t.TempDir()
		git(t, root, "init", "--quiet")
		writeFileContent(t, root, ".gitignore", ".golangci.json\n")

		return root
	}

	t.Run("a clean root", func(t *testing.T) {
		t.Parallel()

		root := repo(t)
		writeFile(t, root, ".golangci.yml")
		writeFile(t, root, "go.mod")
		writeFile(t, root, "sub/.golangci.json")
		writeFile(t, root, "sub/.golangci.toml")
		writeFile(t, root, "sub/.golangci.yaml")

		assertNoStrayLintConfigs(t, root)
	})

	t.Run("an ignored .golangci.json", func(t *testing.T) {
		t.Parallel()

		root := repo(t)
		writeFile(t, root, ".golangci.json")

		assertNoStrayLintConfigs(t, root)
	})

	for _, name := range names {
		t.Run("untracked "+name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			git(t, root, "init", "--quiet")
			writeFile(t, root, name)

			assertStrayLintConfig(t, root, name)
		})

		t.Run("force-added "+name, func(t *testing.T) {
			t.Parallel()

			root := repo(t)
			writeFile(t, root, name)
			git(t, root, "add", "--force", name)

			assertStrayLintConfig(t, root, name)
		})
	}

	t.Run("a tracked .golangci.yaml gone from the working tree", func(t *testing.T) {
		t.Parallel()

		root := repo(t)
		writeFile(t, root, ".golangci.yaml")
		git(t, root, "add", ".golangci.yaml")

		if err := os.Remove(filepath.Join(root, ".golangci.yaml")); err != nil {
			t.Fatalf("remove .golangci.yaml: %v", err)
		}

		assertStrayLintConfig(t, root, ".golangci.yaml")
	})

	t.Run("a .golangci.toml symlink to nothing", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		git(t, root, "init", "--quiet")

		if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, ".golangci.toml")); err != nil {
			t.Fatalf("create the symlink: %v", err)
		}

		assertStrayLintConfig(t, root, ".golangci.toml")
	})

	t.Run("outside git", func(t *testing.T) {
		t.Parallel()

		// A .git file pointing nowhere stops git from looking further up for a
		// repository, so git fails here.
		root := t.TempDir()
		writeFileContent(t, root, ".git", "gitdir: missing\n")

		if got, err := check.StrayLintConfigs(t.Context(), root); err == nil {
			t.Fatalf("StrayLintConfigs() = %q, nil; want an error", got)
		}
	})
}

func assertNoStrayLintConfigs(t *testing.T, root string) {
	t.Helper()

	got, err := check.StrayLintConfigs(t.Context(), root)
	if err != nil {
		t.Fatalf("StrayLintConfigs(): %v", err)
	}

	if len(got) != 0 {
		t.Errorf("StrayLintConfigs() = %q, want no message", got)
	}
}

func assertStrayLintConfig(t *testing.T, root, name string) {
	t.Helper()

	got, err := check.StrayLintConfigs(t.Context(), root)
	if err != nil {
		t.Fatalf("StrayLintConfigs(): %v", err)
	}

	if len(got) != 1 || !strings.HasPrefix(got[0], name+" ") {
		t.Errorf("StrayLintConfigs() = %q, want one message starting %q", got, name+" ")
	}
}

package architecture_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// adapterModulePrefix is the import path of internal/adapter, which each deny
// entry of adapters-are-built-in-cmd extends by one subtree.
const adapterModulePrefix = "github.com/timothymamo/shift2eu/internal/adapter/"

// TestAdaptersBuiltInCmdNamesEveryAdapterSubtree fails on every entry of
// internal/adapter that is not a directory with a deny entry of its own,
// "<module>/internal/adapter/<dir>/", in adapters-are-built-in-cmd. The rule
// names each subtree rather than internal/adapter/ once, because depguard
// compares an import only with the entry that sorts nearest before it and a
// broad entry would be shadowed by the internal/ helper entry (issue #119). A
// subtree added later, or a Go file placed in internal/adapter itself, would
// match no entry and escape the rule without this check. A directory make lint
// never visits (testdata, or a name starting with _ or .) needs no entry.
func TestAdaptersBuiltInCmdNamesEveryAdapterSubtree(t *testing.T) {
	t.Parallel()

	rules, err := check.DepguardRules(readRepositoryFile(t, ".golangci.yml"))
	if err != nil {
		t.Fatalf("DepguardRules(.golangci.yml): %v", err)
	}

	i := slices.IndexFunc(rules, func(r check.DepguardRule) bool { return r.Name == "adapters-are-built-in-cmd" })
	if i < 0 {
		t.Fatal(".golangci.yml: no depguard rule adapters-are-built-in-cmd")
	}
	deny := rules[i].Deny

	entries, err := os.ReadDir(filepath.Join(repositoryRoot(t), "internal", "adapter"))
	if err != nil {
		t.Fatalf("read internal/adapter: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("internal/adapter is empty, so this check would pass on nothing")
	}

	for _, e := range entries {
		name := e.Name()
		if name == "testdata" || name[0] == '_' || name[0] == '.' {
			continue
		}

		if !e.IsDir() {
			t.Errorf("internal/adapter/%s: a file directly in internal/adapter matches no deny entry of "+
				"adapters-are-built-in-cmd; place it in a subtree the rule names (issue #119)", name)

			continue
		}

		if want := adapterModulePrefix + name + "/"; !slices.Contains(deny, want) {
			t.Errorf(".golangci.yml: adapters-are-built-in-cmd has no deny entry %q, so an import of a package under "+
				"internal/adapter/%s from outside the composition roots is not refused; add the entry, and to "+
				"pinnedDepguardRules (issue #119)", want, name)
		}
	}
}

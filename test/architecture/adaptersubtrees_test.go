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

// adapterHelpersDir is the directory under internal/adapter that holds the
// helpers the adapters share (ADR 0009), one directory each (jsondoc).
const adapterHelpersDir = "internal"

// TestAdaptersBuiltInCmdNamesEveryAdapterSubtree fails on every entry of
// internal/adapter that is not a directory with a deny entry of its own,
// "<module>/internal/adapter/<dir>/", in adapters-are-built-in-cmd, and on
// every adapter subtree that has no entry for the package at its root,
// "<module>/internal/adapter/<dir>$". The rule names each subtree rather than
// internal/adapter/ once, because depguard compares an import only with the
// entry that sorts nearest before it and a broad entry would be shadowed by
// the internal/ helper entry (issue #119). A subtree added later, or a Go file
// placed in internal/adapter itself, would match no entry and escape the rule
// without this check. A directory make lint never visits (testdata, or a name
// starting with _ or .) needs no entry.
//
// A subtree's entry ends in "/", which matches a package under the subtree but
// not the package at its root: depguard's prefix match needs the trailing
// separator, so an import of internal/adapter/report itself is refused by no
// entry and a package shared in a subtree root would escape the rule. Each
// subtree therefore also needs the exact entry "<module>/internal/adapter/<dir>$",
// beside the "/" one. An exact entry is not shadowed by the "/" entry it sits
// beside (the two are no strict prefix of one another) and depguard compares an
// import with it exactly, so the pair refuses the whole subtree. The helpers
// subtree is exempt: it holds no package at its root, only a directory per
// helper, and a helper is named by its own exact allow entry, not by this rule.
//
// It also fails when it names nothing, so a layout it skipped whole cannot pass
// it on nothing.
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

	checked := 0

	for _, e := range entries {
		name := e.Name()
		if name == "testdata" || name[0] == '_' || name[0] == '.' {
			continue
		}

		checked++

		if !e.IsDir() {
			t.Errorf("internal/adapter/%s: a file directly in internal/adapter matches no deny entry of "+
				"adapters-are-built-in-cmd; place it in a subtree the rule names (issue #119)", name)

			continue
		}

		if want := adapterModulePrefix + name + "/"; !slices.Contains(deny, want) {
			t.Errorf(".golangci.yml: adapters-are-built-in-cmd has no deny entry %q, so an import of a package under "+
				"internal/adapter/%s from outside the composition roots is not refused; add the entry there and to "+
				"pinnedDepguardRules (issue #119)", want, name)
		}

		if name == adapterHelpersDir {
			// The helpers subtree holds no package at its root, only a directory
			// per helper, and each helper is named by its own exact allow entry.
			continue
		}

		if want := adapterModulePrefix + name + "$"; !slices.Contains(deny, want) {
			t.Errorf(".golangci.yml: adapters-are-built-in-cmd has no deny entry %q, so an import of internal/adapter/%s "+
				"itself, the package at the root of that subtree, is not refused: the %q entry matches only the packages "+
				"under it; add the exact entry there and to pinnedDepguardRules (issue #119)",
				want, name, adapterModulePrefix+name+"/")
		}
	}

	if checked == 0 {
		t.Fatal("internal/adapter holds no subtree or file that adapters-are-built-in-cmd must name, so this check " +
			"would pass on nothing")
	}
}

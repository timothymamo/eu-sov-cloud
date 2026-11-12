package architecture_test

import (
	"go/build"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// gostdEntry is the entry depguard expands to one prefix entry per top-level
// directory of GOROOT/src.
const gostdEntry = "$gostd"

// TestDepguardEntriesShadowNothing fails on every pair of entries of one
// depguard list where the shorter entry matches by prefix and is a strict
// prefix of the longer one. depguard v2.2.1 does not test an import against
// every entry: it sorts the entries and compares the import with the one entry
// that sorts nearest before it, by string prefix unless the entry ends in $,
// and then exactly. A shorter entry therefore stops deciding the imports of a
// longer entry that sorts after it: those imports start with the shorter entry
// but sort after the longer one, are compared with the longer one alone, and
// neither entry refuses them. That is how adapters-are-built-in-cmd came to
// report nothing (issue #119): its deny list held internal/adapter/ and
// internal/adapter/internal/, and every real adapter subpackage (report,
// request, source, target) sorts after internal/ and matched neither.
//
// A shadowed deny entry lets an import the rule means to refuse through when
// the rule is lax, and loses that entry's message when it is strict; a
// shadowed allow entry of a strict rule refuses an import the rule means to
// allow, and one of a lax rule loses an import the deny list would otherwise
// refuse. Either way the two entries do not mean what they say. The entries are
// read with the same reader as the pin in depguardpin_test.go, so a rule that
// shares a deny list through an alias is checked under each rule that uses it.
//
// depguard expands an entry written as $gostd to one prefix entry per top-level
// directory of GOROOT/src (net, encoding, os, and the go that sorts before
// every github.com path), and a standard package added beside it as net/http or
// encoding/json shadows such a root exactly as it would shadow a written entry:
// the root sorts first and every import that starts with it but sorts after the
// longer entry is compared with the longer one alone. The check therefore
// expands $gostd before it compares entries, and compares a bare root with the
// imports depguard compares it with: depguard never matches an import that
// holds a dot with an entry that holds neither a dot nor a slash, so a module
// path under the go root is no pair.
func TestDepguardEntriesShadowNothing(t *testing.T) {
	t.Parallel()

	rules, err := check.DepguardRules(readRepositoryFile(t, ".golangci.yml"))
	if err != nil {
		t.Fatalf("DepguardRules(.golangci.yml): %v", err)
	}

	if len(rules) == 0 {
		t.Fatal(".golangci.yml holds no depguard rule, so this check would pass on nothing")
	}

	roots := goStdRoots(t)

	for _, rule := range rules {
		for _, list := range []struct {
			name    string
			entries []string
		}{
			{name: "allow", entries: rule.Allow},
			{name: "deny", entries: rule.Deny},
		} {
			for _, pair := range shadowedEntries(list.entries, roots) {
				t.Errorf(".golangci.yml: depguard rule %s: the %s entry %q is a strict prefix of %q, so it is shadowed: "+
					"depguard compares an import with the entry that sorts nearest before it, and every import that starts "+
					"with %q but sorts after %q is compared with %q alone. Name each package that %q covers with an entry "+
					"of its own, so the entry nearest before an import is the one that means it (issue #119). "+
					"An entry written as $gostd counts as one per top-level directory of GOROOT/src.",
					rule.Name, list.name, pair[0], pair[1], pair[0], pair[1], pair[1], pair[0])
			}
		}
	}
}

// shadowedEntries returns the pairs of entries of one depguard list that the
// list cannot mean, each as the shorter prefix entry and the longer entry that
// sorts after it. stdRoots is what depguard expands an entry written as $gostd
// to, one prefix entry per top-level directory of GOROOT/src (goStdRoots), and
// every $gostd is replaced by it before the entries are compared, so a standard
// package named beside $gostd is found. An entry ending in $ matches one import
// exactly and shadows nothing below it, so it is never the shorter one; that
// does not spare the entry below an exact one, which loses the imports that
// sort after it. A repeated entry is not a strict prefix of itself, and a bare
// entry, one with no dot and no slash as each std root is, is no prefix of an
// entry that holds a dot, because depguard never matches such an entry against
// an import that holds one (settings.go, strInPrefixList).
func shadowedEntries(entries, stdRoots []string) [][2]string {
	expanded := make([]string, 0, len(entries)+len(stdRoots))
	for _, entry := range entries {
		if entry == gostdEntry {
			expanded = append(expanded, stdRoots...)
			continue
		}
		expanded = append(expanded, entry)
	}

	var pairs [][2]string
	for _, short := range expanded {
		if strings.HasSuffix(short, "$") {
			continue
		}
		for _, long := range expanded {
			if short == long || !strings.HasPrefix(long, short) {
				continue
			}
			if !strings.ContainsAny(short, "./") && strings.ContainsRune(long, '.') {
				continue
			}
			pairs = append(pairs, [2]string{short, long})
		}
	}
	return pairs
}

// goStdRoots returns the entries depguard expands $gostd to: the name of every
// directory directly under GOROOT/src, as depguard's gostdExpander reads them,
// without a network call. GOROOT is read through build.Default, which is $GOROOT
// when that is set and the tree the test was built with otherwise, the same two
// depguard's findGOROOT prefers; runtime.GOROOT is deprecated. It fails when
// there are no directories, or when they hold neither net nor encoding, so the
// expansion cannot quietly become nothing and leave the check above passing on
// a $gostd it never expanded.
func goStdRoots(t *testing.T) []string {
	t.Helper()

	src := filepath.Join(build.Default.GOROOT, "src")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read %s, whose directories depguard expands $gostd to: %v", src, err)
	}

	roots := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			roots = append(roots, e.Name())
		}
	}

	for _, want := range []string{"encoding", "net"} {
		if !slices.Contains(roots, want) {
			t.Fatalf("%s holds no %s directory, so $gostd does not expand to what this check assumes: %v", src, want, roots)
		}
	}

	return roots
}

// TestShadowedEntries pins shadowedEntries on small lists, so the check in
// TestDepguardEntriesShadowNothing cannot quietly stop finding a pair: the
// order of the entries in the list does not matter, a longer entry need not
// follow the shorter one, equal entries and a pair of siblings shadow nothing,
// and an entry ending in $ is exact and never the shorter one. It pins the
// expansion of $gostd too: roots is what depguard would expand it to (the
// directories of GOROOT/src, or a few of them here), so a standard package
// named beside $gostd is found, an exact entry beside it is no way out, and a
// module path under a bare root is no pair. goStdRoots(t) is the expansion of
// the toolchain at hand; a literal root pins the cases that must hold for any
// release of Go.
func TestShadowedEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entries []string
		roots   []string
		want    [][2]string
	}{
		{
			name:    "no entry",
			entries: nil,
		},
		{
			name:    "one entry",
			entries: []string{"github.com/x/internal/adapter/"},
		},
		{
			name:    "siblings",
			entries: []string{"github.com/x/report/", "github.com/x/request/", "github.com/x/source/", "github.com/x/target/"},
		},
		{
			name:    "a repeated entry",
			entries: []string{"github.com/x/adapter/", "github.com/x/adapter/"},
		},
		{
			// An entry ending in $ matches one import exactly: it is never the
			// shorter one, even where another entry happens to start with it.
			name:    "an exact entry",
			entries: []string{"github.com/x/adapter$", "github.com/x/adapter$more"},
		},
		{
			// The fix for issue #119: the exact entry of a subtree root beside
			// the prefix entry of the packages under it. Neither is a strict
			// prefix of the other, because the entry ends in / and the exact one
			// in $, so the pair shadows nothing: depguard compares an import of
			// the root with the exact entry and an import below it with the
			// prefix entry.
			name:    "an exact entry beside the prefix entry of the packages under it",
			entries: []string{"github.com/x/adapter/report/", "github.com/x/adapter/report$"},
		},
		{
			// A prefix entry is shadowed below an exact entry too: an import of
			// a package under that one is decided by the exact entry alone.
			name:    "a prefix entry and the exact entry of a package below it",
			entries: []string{"github.com/x/adapter/internal/", "github.com/x/adapter/internal/jsondoc$"},
			want:    [][2]string{{"github.com/x/adapter/internal/", "github.com/x/adapter/internal/jsondoc$"}},
		},
		{
			name:    "a prefix entry before the longer one",
			entries: []string{"github.com/x/adapter/", "github.com/x/adapter/internal/"},
			want:    [][2]string{{"github.com/x/adapter/", "github.com/x/adapter/internal/"}},
		},
		{
			name:    "the longer entry first",
			entries: []string{"github.com/x/adapter/internal/", "github.com/x/adapter/"},
			want:    [][2]string{{"github.com/x/adapter/", "github.com/x/adapter/internal/"}},
		},
		{
			name:    "every longer entry of one prefix entry",
			entries: []string{"net", "net/http", "net/mail", "net/rpc", "net/smtp"},
			want: [][2]string{
				{"net", "net/http"},
				{"net", "net/mail"},
				{"net", "net/rpc"},
				{"net", "net/smtp"},
			},
		},
		{
			// $gostd stands for the directories of GOROOT/src, so the net it
			// holds is shadowed by a net/http beside it exactly as a written net
			// would be, and the check finds it only after the expansion.
			name:    "$gostd beside a standard package",
			entries: []string{"$gostd", "net/http"},
			roots:   goStdRoots(t),
			want:    [][2]string{{"net", "net/http"}},
		},
		{
			// An exact entry is no way out beside a root: with allow [$gostd,
			// net/http$] golangci-lint 2.15.0 refuses net/mail and net/url,
			// because they sort after net/http and are compared with the exact
			// entry alone. So the pair is reported, as the written pair above is.
			name:    "$gostd beside the exact entry of a standard package",
			entries: []string{"$gostd", "net/http$"},
			roots:   goStdRoots(t),
			want:    [][2]string{{"net", "net/http$"}},
		},
		{
			// A module path is no standard package, even under a root that starts
			// it: GOROOT/src holds a go directory, the go of github.com, and
			// depguard never matches an import that holds a dot with an entry that
			// holds neither a dot nor a slash.
			name:    "$gostd beside a module path",
			entries: []string{"$gostd", "github.com/timothymamo/shift2eu/internal/testutil$"},
			roots:   []string{"go"},
		},
		{
			// The same for a written root: a module path holds a dot, so no bare
			// root is a prefix of it, while a std package beside it is one.
			name:    "a written root beside module paths",
			entries: []string{"go", "github.com/x/go-tool", "golang.org/x/tools"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(tt.want, shadowedEntries(tt.entries, tt.roots)); diff != "" {
				t.Errorf("shadowedEntries(%v, %v) mismatch (-want +got):\n%s", tt.entries, tt.roots, diff)
			}
		})
	}
}

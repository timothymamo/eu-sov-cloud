package architecture_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

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
func TestDepguardEntriesShadowNothing(t *testing.T) {
	t.Parallel()

	rules, err := check.DepguardRules(readRepositoryFile(t, ".golangci.yml"))
	if err != nil {
		t.Fatalf("DepguardRules(.golangci.yml): %v", err)
	}

	if len(rules) == 0 {
		t.Fatal(".golangci.yml holds no depguard rule, so this check would pass on nothing")
	}

	for _, rule := range rules {
		for _, list := range []struct {
			name    string
			entries []string
		}{
			{name: "allow", entries: rule.Allow},
			{name: "deny", entries: rule.Deny},
		} {
			for _, pair := range shadowedEntries(list.entries) {
				t.Errorf(".golangci.yml: depguard rule %s: the %s entry %q is a strict prefix of %q, so it is shadowed: "+
					"depguard compares an import with the entry that sorts nearest before it, and every import that starts "+
					"with %q but sorts after %q is compared with %q alone. Name each package that %q covers with an entry "+
					"of its own, so the entry nearest before an import is the one that means it (issue #119)",
					rule.Name, list.name, pair[0], pair[1], pair[0], pair[1], pair[1], pair[0])
			}
		}
	}
}

// shadowedEntries returns the pairs of entries of one depguard list that the
// list cannot mean, each as the shorter prefix entry and the longer entry that
// sorts after it. An entry ending in $ matches one import exactly and shadows
// nothing below it, so it is never the shorter one; a repeated entry is not a
// strict prefix of itself.
func shadowedEntries(entries []string) [][2]string {
	var pairs [][2]string
	for _, short := range entries {
		if strings.HasSuffix(short, "$") {
			continue
		}
		for _, long := range entries {
			if short != long && strings.HasPrefix(long, short) {
				pairs = append(pairs, [2]string{short, long})
			}
		}
	}
	return pairs
}

// TestShadowedEntries pins shadowedEntries on small lists, so the check in
// TestDepguardEntriesShadowNothing cannot quietly stop finding a pair: the
// order of the entries in the list does not matter, a longer entry need not
// follow the shorter one, equal entries and a pair of siblings shadow nothing,
// and an entry ending in $ is exact and never the shorter one.
func TestShadowedEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entries []string
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(tt.want, shadowedEntries(tt.entries)); diff != "" {
				t.Errorf("shadowedEntries(%v) mismatch (-want +got):\n%s", tt.entries, diff)
			}
		})
	}
}

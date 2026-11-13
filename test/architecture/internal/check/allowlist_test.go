package check_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// golangciYAML is a cut-down .golangci.yml holding the shapes the check must
// read and the ones it must refuse: rules sharing a prefix, a deny list after
// the allow list, a comment line inside the allow list, quoted entries, and the
// list-modes and entries that make a rule unusable as an allow list.
const golangciYAML = `version: "2"

linters:
  enable:
    - depguard
  settings:
    depguard:
      rules:
        # The domain packages and the standard packages they use, each ending
        # in $ so the entry covers that package only.
        domain-is-pure:
          list-mode: strict
          files:
            - "**/internal/domain/**"
            - "!$test"
          allow:
            - errors$
            # A comment line between two entries is not an entry.
            - fmt$
            - "strings$"
            - github.com/timothymamo/shift2eu/internal/domain/text$
            # Non-ASCII printable text is fine (naïve — ünïcode).
          deny:
            - pkg: os
              desc: the domain has no OS access
        # Shares the prefix of the rule above, and its own entries.
        domain-is-pure-too:
          list-mode: strict
          allow:
            - os$
        lax-rule:
          list-mode: lax
          files:
            - "**/internal/cli/**"
          allow:
            - errors$
        no-allow:
          list-mode: strict
          files:
            - "**/internal/config/**"
          deny:
            - pkg: os
              desc: the allow list already refuses it
        bare-entry:
          list-mode: strict
          allow:
            - os
        sdk-prefixes:
          list-mode: strict
          files:
            - "**/internal/adapter/source/**"
          allow:
            - errors$
            - github.com/aws/aws-sdk-go-v2/
            - cloud.google.com/go/
            - google.golang.org/api/googleapi$
            - github.com/google/go-cmp/cmp$
            - github.com/google/go-cmp/cmp/
        gostd-entry:
          list-mode: strict
          allow:
            - $gostd
            - errors$
        last-rule:
          list-mode: strict
          allow:
            - unicode$
            - unicode/utf8$
`

// golangciDoc wraps the lines below `rules:` in the rest of a .golangci.yml, so
// a case writes only the rule it breaks.
func golangciDoc(rules string) string {
	return `version: "2"

linters:
  settings:
    depguard:
      rules:
` + rules
}

// oneRuleDoc writes one depguard rule named r whose list-mode is strict and
// whose allow list is errors$, with comment as a comment line inside it: the
// character cases put the character where today's reader skips it.
func oneRuleDoc(comment string) string {
	return golangciDoc("        r:\n          list-mode: strict\n          " + comment +
		"\n          allow:\n            - errors$\n")
}

// TestAllowList pins the allow list the check reads back from a depguard rule:
// its entries in file order, without their quotes, each either exact (its
// trailing $ removed) or a prefix (ending in /), and an error for every rule it
// cannot read an allow list from. depguard reads an entry without $ as a plain
// string prefix, so `os` would allow os/exec and `github.com/aws` would allow
// github.com/awsfoo: the check reads only a prefix written with its trailing /
// and refuses the rest, and it refuses a rule whose deny list collides with its
// allow list, since depguard decides that import by entry length. A rule that
// allows nothing is read from an explicit empty flow sequence, allow: [], and
// returns no entries; a missing allow key, or an allow key left null, is still
// refused (issue #166).
func TestAllowList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// golangci is the .golangci.yml to read; empty means golangciYAML.
		golangci string
		rule     string
		want     []check.AllowEntry
		wantErr  bool
		// wantErrFragment is the text a refusal message must name.
		wantErrFragment string
	}{
		{
			name: "a strict rule returns its entries in file order",
			rule: "domain-is-pure",
			want: []check.AllowEntry{
				{Path: "errors"},
				{Path: "fmt"},
				{Path: "strings"},
				{Path: "github.com/timothymamo/shift2eu/internal/domain/text"},
			},
		},
		{
			name: "a rule sharing a prefix is read on its own",
			rule: "domain-is-pure-too",
			want: []check.AllowEntry{{Path: "os"}},
		},
		{
			name: "the last rule of the file is read",
			rule: "last-rule",
			want: []check.AllowEntry{{Path: "unicode"}, {Path: "unicode/utf8"}},
		},
		{
			// The SDK-only rule mixes the two: the standard packages and the
			// SDK helper packages it names one by one end in $, the SDK roots
			// it allows every package of end in /. Dropping the distinction
			// would let an exact entry through as a prefix, or refuse the rule
			// the check has to hold.
			name: "exact and prefix entries are told apart",
			rule: "sdk-prefixes",
			want: []check.AllowEntry{
				{Path: "errors"},
				{Path: "github.com/aws/aws-sdk-go-v2/", Prefix: true},
				{Path: "cloud.google.com/go/", Prefix: true},
				{Path: "google.golang.org/api/googleapi"},
				{Path: "github.com/google/go-cmp/cmp"},
				{Path: "github.com/google/go-cmp/cmp/", Prefix: true},
			},
		},
		{
			// A prefix entry names a whole SDK root: the trailing / is what
			// keeps it from allowing a look-alike path as well.
			name:     "an entry ending in / is a prefix entry",
			golangci: golangciDoc("        r:\n          list-mode: strict\n          allow:\n            - github.com/aws/aws-sdk-go-v2/\n            - errors$\n"),
			rule:     "r",
			want: []check.AllowEntry{
				{Path: "github.com/aws/aws-sdk-go-v2/", Prefix: true},
				{Path: "errors"},
			},
		},
		{
			// depguard reads an entry without $ as a prefix, so os allows
			// os/exec and osfoo too: an entry that lost its $ is a widening the
			// check cannot tell from an intended prefix. Only an entry written
			// with its trailing / is a prefix the check reads.
			name:    "a prefix entry not ending in / is an error",
			rule:    "bare-entry",
			wantErr: true,
		},
		{
			// github.com/aws would allow the look-alike github.com/awsfoo as
			// well as the SDK root.
			name:     "a prefix entry over a look-alike path is an error",
			golangci: golangciDoc("        r:\n          list-mode: strict\n          allow:\n            - github.com/aws\n"),
			rule:     "r",
			wantErr:  true,
		},
		{
			// depguard applies the longest match between the allow and the
			// deny list in strict mode, so a deny entry that an allow entry
			// also allows makes the rule mean what its entry lengths decide:
			// fail closed rather than read a rule whose deny list depguard may
			// never apply.
			name:     "a deny entry an allow entry allows is an error",
			golangci: golangciDoc("        r:\n          list-mode: strict\n          allow:\n            - errors$\n            - os$\n          deny:\n            - pkg: os\n              desc: the domain has no OS access\n"),
			rule:     "r",
			wantErr:  true,
		},
		{
			name:     "a deny entry under a prefix allow entry is an error",
			golangci: golangciDoc("        r:\n          list-mode: strict\n          allow:\n            - net/\n          deny:\n            - net/http\n"),
			rule:     "r",
			wantErr:  true,
		},
		{
			// The shape domain-is-pure has: net/netip and net/url are allowed
			// one by one and net is denied, so an import of net/netip matches
			// both lists and the longer allow entry wins. No deny entry is
			// itself allowed, so this rule is read, not refused.
			name:     "a deny entry no allow entry matches is read",
			golangci: golangciDoc("        r:\n          list-mode: strict\n          allow:\n            - net/netip$\n            - net/url$\n          deny:\n            - net\n"),
			rule:     "r",
			want:     []check.AllowEntry{{Path: "net/netip"}, {Path: "net/url"}},
		},
		{
			name:    "a rule that is not there is an error",
			rule:    "no-such-rule",
			wantErr: true,
		},
		{
			name:    "a lax rule is an error",
			rule:    "lax-rule",
			wantErr: true,
		},
		{
			name:    "a rule without an allow key is an error",
			rule:    "no-allow",
			wantErr: true,
		},
		{
			// $gostd is depguard's name for the standard library, not an
			// import path: the check cannot say what it allows.
			name:    "a $gostd entry is an error",
			rule:    "gostd-entry",
			wantErr: true,
		},

		{
			// app-tests-use-fakes-not-adapters shares the deny list of
			// app-uses-ports-not-adapters through *app-deny (issue #144): the
			// check reads the alias as the anchor's entries, so the
			// allow/deny shadowing check covers the shared list too. The anchor
			// sits in the rule above, which this reader of one rule never
			// reads.
			name:     "a deny list written as an alias is read from its anchor above",
			golangci: golangciDoc("        anchor-rule:\n          list-mode: strict\n          allow:\n            - errors$\n          deny: &shared\n            - pkg: net\n              desc: the application layer has no network access\n        r:\n          list-mode: strict\n          allow:\n            - errors$\n            - net/netip$\n          deny: *shared\n"),
			rule:     "r",
			want:     []check.AllowEntry{{Path: "errors"}, {Path: "net/netip"}},
		},
		{
			// The alias is followed, so an allow entry that allows a deny entry
			// of the anchored list is refused as one written in the rule itself
			// would be: depguard would decide that import by entry length.
			name:            "an allow entry that allows an aliased deny entry is an error",
			golangci:        golangciDoc("        anchor-rule:\n          list-mode: strict\n          allow:\n            - errors$\n          deny: &shared\n            - pkg: os\n              desc: the application layer has no OS access\n        r:\n          list-mode: strict\n          allow:\n            - errors$\n            - os$\n          deny: *shared\n"),
			rule:            "r",
			wantErr:         true,
			wantErrFragment: "deny entry os is allowed by allow entry os$",
		},
		{
			// An alias is the whole list: entries written below it would be a
			// second list the anchor does not hold.
			name:            "an alias with lines below it is an error",
			golangci:        golangciDoc("        anchor-rule:\n          list-mode: strict\n          allow:\n            - errors$\n          deny: &shared\n            - pkg: net\n              desc: the application layer has no network access\n        r:\n          list-mode: strict\n          allow:\n            - errors$\n          deny: *shared\n            - pkg: os\n              desc: the application layer has no OS access\n"),
			rule:            "r",
			wantErr:         true,
			wantErrFragment: "has lines below it",
		},
		{
			// Nothing above the alias anchors that name, so the check cannot
			// tell what the rule's deny list holds: fail closed rather than read
			// the allow list as if it shadowed nothing.
			name:            "an alias to an anchor the document does not hold is an error",
			golangci:        golangciDoc("        r:\n          list-mode: strict\n          allow:\n            - errors$\n          deny: *nope\n"),
			rule:            "r",
			wantErr:         true,
			wantErrFragment: "*nope",
		},

		// The character rules are the same as the ones releasetarget.Read holds
		// the release document to: no two YAML readers agree on what the
		// characters that look like line breaks do.
		{
			name:     "a lone CR is an error",
			golangci: oneRuleDoc("# a lone\rCR"),
			rule:     "r",
			wantErr:  true,
		},
		{
			name:     "U+2028 is an error",
			golangci: oneRuleDoc("# a line separator:\u2028x"),
			rule:     "r",
			wantErr:  true,
		},
		{
			name:     "U+0085 is an error",
			golangci: oneRuleDoc("# a NEL:\u0085x"),
			rule:     "r",
			wantErr:  true,
		},
		{
			name:     "a tab is an error",
			golangci: oneRuleDoc("#\ta tab"),
			rule:     "r",
			wantErr:  true,
		},
		{
			name:     "a repeated allow key is an error",
			golangci: golangciDoc("        r:\n          list-mode: strict\n          allow:\n            - errors$\n          allow:\n            - fmt$\n"),
			rule:     "r",
			wantErr:  true,
		},
		{
			name:     "an aliased allow list is an error",
			golangci: golangciDoc("        r:\n          list-mode: strict\n          allow: *x\n"),
			rule:     "r",
			wantErr:  true,
		},
		{
			name:     "a merge key in the rule is an error",
			golangci: golangciDoc("        r:\n          list-mode: strict\n          <<: *base\n          allow:\n            - errors$\n"),
			rule:     "r",
			wantErr:  true,
		},
		{
			name:     "a quoted rule name is an error",
			golangci: golangciDoc("        \"r\":\n          list-mode: strict\n          allow:\n            - errors$\n"),
			rule:     "r",
			wantErr:  true,
		},
		{
			name:     "a document marker is an error",
			golangci: "---\n" + golangciYAML,
			rule:     "domain-is-pure",
			wantErr:  true,
		},
		{
			name:     "a rule without list-mode is an error",
			golangci: golangciDoc("        r:\n          allow:\n            - errors$\n"),
			rule:     "r",
			wantErr:  true,
		},
		{
			// An explicit empty flow sequence is how a strict rule says it
			// allows nothing at all, which the doc.go-only packages need
			// (issue #166): the rule is read, with no entries, not refused.
			name:     "an explicit empty allow list allows nothing",
			golangci: golangciDoc("        r:\n          list-mode: strict\n          allow: []\n"),
			rule:     "r",
			want:     []check.AllowEntry{},
		},
		{
			// Only [] means "allows nothing": an allow key left null (an
			// empty block sequence) is refused, so a rule that was parsed as
			// empty by mistake cannot pass for one that allows nothing.
			name:            "an allow key with no items is an error",
			golangci:        golangciDoc("        r:\n          list-mode: strict\n          allow:\n"),
			rule:            "r",
			wantErr:         true,
			wantErrFragment: "the allow list is empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := tc.golangci
			if doc == "" {
				doc = golangciYAML
			}

			got, err := check.AllowList([]byte(doc), tc.rule)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("AllowList(%q) = %v, nil; want an error", tc.rule, got)
				}

				if tc.wantErrFragment != "" && !strings.Contains(err.Error(), tc.wantErrFragment) {
					t.Errorf("AllowList(%q) error does not name %q: %v", tc.rule, tc.wantErrFragment, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("AllowList(%q): %v", tc.rule, err)
			}

			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("AllowList(%q) mismatch (-want +got):\n%s", tc.rule, diff)
			}
		})
	}
}

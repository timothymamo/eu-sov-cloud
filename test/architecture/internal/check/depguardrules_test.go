package check_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// depguardRulesYAML holds the rules DepguardRules reads whole: a strict rule
// with files, allow and deny, a deny-only rule with no list-mode and no allow,
// and two rules that share one deny list through a YAML anchor and an alias, as
// app-uses-ports-not-adapters and app-tests-use-fakes-not-adapters do in
// .golangci.yml.
const depguardRulesYAML = `version: "2"

linters:
  settings:
    depguard:
      rules:
        domain-is-pure:
          list-mode: strict
          files:
            - "**/internal/domain/**"
            - "!$test"
          allow:
            - errors$
            - net/netip$
          deny:
            - pkg: os
              desc: the domain has no OS access
            - pkg: net
              desc: the domain has no network access (net/netip is allowed)
        testutil-only-in-tests:
          files:
            - "$all"
            - "!$test"
          deny:
            - pkg: github.com/timothymamo/shift2eu/internal/testutil
              desc: testutil may only be imported from _test.go files
        adapters-are-built-in-cmd:
          list-mode: lax
          files:
            - "$all"
          deny: &adapters
            - pkg: github.com/timothymamo/shift2eu/internal/adapter/
              desc: adapters are constructed only in cmd/shift2eu
            - pkg: net$
              desc: an adapter opens no connection of its own
        app-tests-use-fakes-not-adapters:
          list-mode: lax
          files:
            - "**/internal/app/**/*_test.go"
          deny: *adapters
`

// TestDepguardRules pins what the package check reads from the depguard section
// of .golangci.yml: every rule, strict and deny-only alike, with its list-mode,
// its files list, its allow list and the pkg of each deny entry, in document
// order. Issue #80 lost a rule and an exact allow entry to a merge conflict
// resolution that no test noticed, so the check reads all of a rule, not only
// the parts the package-by-package checks use.
func TestDepguardRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		golangci string
		want     []check.DepguardRule
		wantErr  bool
	}{
		{
			name:     "every rule comes back whole, in document order",
			golangci: depguardRulesYAML,
			want: []check.DepguardRule{
				{
					Name:     "domain-is-pure",
					ListMode: "strict",
					Files:    []string{"**/internal/domain/**", "!$test"},
					Allow:    []string{"errors$", "net/netip$"},
					Deny:     []string{"os", "net"},
				},
				{
					Name:     "testutil-only-in-tests",
					ListMode: "",
					Files:    []string{"$all", "!$test"},
					Allow:    nil,
					Deny:     []string{"github.com/timothymamo/shift2eu/internal/testutil"},
				},
				{
					Name:     "adapters-are-built-in-cmd",
					ListMode: "lax",
					Files:    []string{"$all"},
					Allow:    nil,
					Deny:     []string{"github.com/timothymamo/shift2eu/internal/adapter/", "net$"},
				},
				{
					Name:     "app-tests-use-fakes-not-adapters",
					ListMode: "lax",
					Files:    []string{"**/internal/app/**/*_test.go"},
					Allow:    nil,
					Deny:     []string{"github.com/timothymamo/shift2eu/internal/adapter/", "net$"},
				},
			},
		},
		{
			// A deny entry without pkg would be a rule holding nothing, which
			// the check cannot tell from one that was emptied: refuse it.
			name:     "a deny entry without pkg is an error",
			golangci: golangciDoc("        reports-stay-offline:\n          list-mode: strict\n          files:\n            - \"**/internal/adapter/report/**\"\n          deny:\n            - desc: no package\n"),
			wantErr:  true,
		},
		{
			// A key in a deny entry that the reader skipped could hide the
			// pkg it holds: only pkg and desc belong there.
			name: "a deny entry with a key other than pkg or desc is an error",
			golangci: golangciDoc(`        reports-stay-offline:
          list-mode: strict
          files:
            - "**/internal/adapter/report/**"
          deny:
            - pkg: os
              why: the reporters open no file
`),
			wantErr: true,
		},
		{
			// The desc of a deny entry is one scalar on its line. A reader
			// that skipped it would accept YAML it does not read, and a later
			// alias could resolve to an anchor it never saw.
			name: "a desc with lines below it is an error",
			golangci: golangciDoc(`        reporters-stay-offline:
          list-mode: strict
          files:
            - "**/internal/adapter/report/**"
          deny:
            - pkg: os
              desc:
                text: the reporters open no file
`),
			wantErr: true,
		},
		{
			name: "a desc carrying an anchor is an error",
			golangci: golangciDoc(`        reporters-stay-offline:
          list-mode: strict
          files:
            - "**/internal/adapter/report/**"
          deny:
            - pkg: os
              desc: &reason the reporters open no file
`),
			wantErr: true,
		},
		{
			name: "a desc that is an alias is an error",
			golangci: golangciDoc(`        reporters-stay-offline:
          list-mode: strict
          files:
            - "**/internal/adapter/report/**"
          deny:
            - pkg: os
              desc: *reason
`),
			wantErr: true,
		},
		{
			// The plain, single-quoted and double-quoted spellings of a
			// one-line desc are prose the check reads and drops.
			name: "a one-line desc in any quoting still reads its pkg",
			golangci: golangciDoc(`        reporters-stay-offline:
          list-mode: strict
          files:
            - "**/internal/adapter/report/**"
          deny:
            - pkg: os
              desc: the reporters open no file
            - pkg: net
              desc: 'the reporters make no call'
            - pkg: syscall
              desc: "the reporters make no syscall"
`),
			want: []check.DepguardRule{
				{
					Name:     "reporters-stay-offline",
					ListMode: "strict",
					Files:    []string{"**/internal/adapter/report/**"},
					Deny:     []string{"os", "net", "syscall"},
				},
			},
		},
		{
			// Two rules sharing one deny list through an anchor: a repeated
			// anchor would silently rebind the entries the first one holds.
			name: "a repeated anchor is an error",
			golangci: golangciDoc(`        first:
          list-mode: lax
          files:
            - "$all"
          deny: &shared
            - pkg: os
        second:
          list-mode: lax
          files:
            - "$all"
          deny: &shared
            - pkg: net
`),
			wantErr: true,
		},
		{
			// An alias stands for the list written above it; entries under it
			// would be read as part of the anchored list it names.
			name: "an alias with lines below it is an error",
			golangci: golangciDoc(`        first:
          list-mode: lax
          files:
            - "$all"
          deny: &shared
            - pkg: os
        second:
          list-mode: lax
          files:
            - "$all"
          deny: *shared
            - pkg: net
`),
			wantErr: true,
		},
		{
			name: "a deny with a value that is no anchor or alias is an error",
			golangci: golangciDoc(`        reports-stay-offline:
          list-mode: lax
          files:
            - "$all"
          deny: os net
`),
			wantErr: true,
		},
		{
			name: "an anchor name with a character the reader refuses is an error",
			golangci: golangciDoc(`        reports-stay-offline:
          list-mode: lax
          files:
            - "$all"
          deny: &shared&more
            - pkg: os
`),
			wantErr: true,
		},
		{
			// A list-mode the tool does not know reads as a rule of the wrong
			// strength, so the reader refuses it in either spelling.
			name:     "a list-mode in upper case is an error",
			golangci: golangciDoc("        reports-stay-offline:\n          list-mode: Strict\n          files:\n            - \"$all\"\n"),
			wantErr:  true,
		},
		{
			name:     "a list-mode that is no mode is an error",
			golangci: golangciDoc("        reports-stay-offline:\n          list-mode: none\n          files:\n            - \"$all\"\n"),
			wantErr:  true,
		},
		{
			// A key the reader ignored could hide a weakening: a files list
			// moved under another key still reads as a rule the check pins.
			name:     "a key the check does not know is an error",
			golangci: golangciDoc("        reports-stay-offline:\n          list-mode: strict\n          file:\n            - \"**/internal/adapter/report/**\"\n          files:\n            - \"**/internal/adapter/report/**\"\n"),
			wantErr:  true,
		},
		{
			name:     "an alias with no anchor is an error",
			golangci: golangciDoc("        reports-stay-offline:\n          list-mode: lax\n          files:\n            - \"$all\"\n          deny: *nowhere\n"),
			wantErr:  true,
		},
		{
			name:     "no depguard section is an error",
			golangci: "version: \"2\"\n\nlinters:\n  settings:\n    gosec:\n      excludes:\n        - G104\n",
			wantErr:  true,
		},
		{
			name:     "a character the reader refuses is an error",
			golangci: "# a lone\rCR\n" + depguardRulesYAML,
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := check.DepguardRules([]byte(tc.golangci))

			if tc.wantErr {
				if err == nil {
					t.Fatalf("DepguardRules() = %v, nil; want an error", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("DepguardRules(): %v", err)
			}

			// An absent list and an empty one are the same rule: a deny-only
			// rule has no allow list, and either spelling of that is fine.
			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("DepguardRules() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// pinnedRules is the rule set PinnedRules compares against: two rules, each with
// a files list, an allow list holding an exact entry, and a deny list.
func pinnedRules() []check.DepguardRule {
	return []check.DepguardRule{
		{
			Name:     "domain-is-pure",
			ListMode: "strict",
			Files:    []string{"**/internal/domain/**", "!$test"},
			Allow: []string{
				"errors$",
				"github.com/timothymamo/shift2eu/internal/adapter/internal/jsondoc$",
			},
			Deny: []string{"os", "net"},
		},
		{
			Name:     "reporters-stay-offline",
			ListMode: "strict",
			Files:    []string{"**/internal/adapter/report/**", "!$test"},
			Allow:    []string{"fmt$", "io$"},
			Deny:     []string{"net", "os"},
		},
	}
}

// TestPinnedRules pins the comparison a pinned rule set gets: an identical set
// is silent, and any difference - a rule missing or extra, a changed list-mode,
// an entry removed, changed or added in files, allow or deny - is a message
// that names the rule it is about and says what changed. A weakening such as
// dropping the $ of an exact allow entry is a difference like any other.
func TestPinnedRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// mutate returns the rule set read from the document, as got.
		mutate func(rules []check.DepguardRule) []check.DepguardRule
		// rule is the name every difference the test asserts belongs to.
		rule string
		// mentionsAny are the texts the message about rule may name to say
		// what changed; one of them is enough.
		mentionsAny []string
		// wantSilent is set when got and want must match with no message.
		wantSilent bool
	}{
		{
			name:       "the same rules are silent",
			mutate:     func(rules []check.DepguardRule) []check.DepguardRule { return rules },
			wantSilent: true,
		},
		{
			name:        "a rule the document lost is reported",
			mutate:      func(rules []check.DepguardRule) []check.DepguardRule { return rules[1:] },
			rule:        "domain-is-pure",
			mentionsAny: []string{"domain-is-pure"},
		},
		{
			name: "a rule the document gained is reported",
			mutate: func(rules []check.DepguardRule) []check.DepguardRule {
				return append(rules, check.DepguardRule{Name: "reporters-use-the-clock", ListMode: "lax", Files: []string{"$all"}})
			},
			rule:        "reporters-use-the-clock",
			mentionsAny: []string{"reporters-use-the-clock"},
		},
		{
			name: "a list-mode that changed is reported",
			mutate: func(rules []check.DepguardRule) []check.DepguardRule {
				rules[0].ListMode = "lax"

				return rules
			},
			rule:        "domain-is-pure",
			mentionsAny: []string{"list-mode"},
		},
		{
			name: "a files entry that was removed is reported",
			mutate: func(rules []check.DepguardRule) []check.DepguardRule {
				rules[0].Files = []string{"**/internal/domain/**"}

				return rules
			},
			rule:        "domain-is-pure",
			mentionsAny: []string{"!$test"},
		},
		{
			name: "an allow entry that was removed is reported",
			mutate: func(rules []check.DepguardRule) []check.DepguardRule {
				rules[0].Allow = rules[0].Allow[:1]

				return rules
			},
			rule:        "domain-is-pure",
			mentionsAny: []string{"jsondoc"},
		},
		{
			// The difference issue #80 lost: the exact entry jsondoc$ turned
			// into the prefix jsondoc, which allows every package under it.
			name: "an exact allow entry widened to a prefix is reported",
			mutate: func(rules []check.DepguardRule) []check.DepguardRule {
				rules[0].Allow[1] = strings.TrimSuffix(rules[0].Allow[1], "$")

				return rules
			},
			rule:        "domain-is-pure",
			mentionsAny: []string{"jsondoc"},
		},
		{
			name: "an allow entry that was added is reported",
			mutate: func(rules []check.DepguardRule) []check.DepguardRule {
				rules[0].Allow = append(rules[0].Allow, "os$")

				return rules
			},
			rule:        "domain-is-pure",
			mentionsAny: []string{"os$"},
		},
		{
			name: "a deny entry that was removed is reported",
			mutate: func(rules []check.DepguardRule) []check.DepguardRule {
				rules[0].Deny = []string{"os"}

				return rules
			},
			rule:        "domain-is-pure",
			mentionsAny: []string{"net"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			messages := check.PinnedRules(tc.mutate(pinnedRules()), pinnedRules())

			if tc.wantSilent {
				if len(messages) != 0 {
					t.Fatalf("PinnedRules() = %v; want no message", messages)
				}

				return
			}

			if len(messages) == 0 {
				t.Fatalf("PinnedRules() returned no message; want one about %s", tc.rule)
			}

			named := false

			for _, message := range messages {
				if !strings.HasPrefix(message, tc.rule+": ") {
					t.Errorf("message %q does not start with %q", message, tc.rule+": ")

					continue
				}

				named = true

				if len(tc.mentionsAny) != 0 && !slices.ContainsFunc(tc.mentionsAny, func(s string) bool {
					return strings.Contains(message, s)
				}) {
					t.Errorf("message %q says nothing of %v, so it does not say what changed", message, tc.mentionsAny)
				}
			}

			if !named {
				t.Errorf("PinnedRules() = %v; want a message about %s", messages, tc.rule)
			}
		})
	}
}

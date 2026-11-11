package check_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// filesDoc writes a .golangci.yml holding one strict rule named r whose files:
// list is files, with comment as a comment line at the head of the rule: the
// character cases put the character where today's reader skips it.
func filesDoc(comment string, files ...string) string {
	var b strings.Builder

	b.WriteString("        r:\n          list-mode: strict\n")

	if comment != "" {
		b.WriteString("          " + comment + "\n")
	}

	b.WriteString("          files:\n")

	for _, file := range files {
		b.WriteString("            - " + strconv.Quote(file) + "\n")
	}

	b.WriteString("          allow:\n            - errors$\n")

	return golangciDoc(b.String())
}

// TestRuleDirs pins what the check reads back from a depguard rule's files:
// list: the directories the rule holds, the directories its negated globs take
// out of it, both in file order, and whether it holds test files. The
// SDK-only rule is why the last two exist: it names one directory and excludes
// two under it, and it holds test files, which go list -deps lists separately.
//
// A rule that writes !$test holds no test files; one without it does. Every
// other shape is still refused.
func TestRuleDirs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// golangci is the .golangci.yml to read; empty means filesDoc of files.
		golangci string
		// files are the entries of the rule's files: list when golangci is
		// empty.
		files []string
		rule  string
		want  check.RuleFiles
		// wantErr is set for every document the check must refuse.
		wantErr bool
		// wantErrFragment is the entry a refusal message must name.
		wantErrFragment string
	}{
		{
			name:  "one directory comes back without its globs",
			files: []string{"**/internal/domain/**", "!$test"},
			want:  check.RuleFiles{Dirs: []string{"internal/domain"}},
		},
		{
			// The shape embedded-snapshot-targets-stay-offline has: every
			// target that embeds a snapshot is listed by its own entry.
			name: "two directories come back in file order",
			files: []string{
				"**/internal/adapter/target/ovhcloud/**",
				"**/internal/adapter/target/scaleway/**",
				"!$test",
			},
			want: check.RuleFiles{Dirs: []string{"internal/adapter/target/ovhcloud", "internal/adapter/target/scaleway"}},
		},
		{
			// The shape cloud-adapters-reach-the-cloud-through-the-sdk has: one
			// directory, two directories under it taken out, and no !$test, so
			// the rule holds test files too.
			name: "the excluded directories and the test files come back too",
			files: []string{
				"**/internal/adapter/source/**",
				"!**/internal/adapter/source/inventoryfile/**",
				"!**/internal/adapter/source/sourcetest/**",
			},
			want: check.RuleFiles{
				Dirs:     []string{"internal/adapter/source"},
				Excludes: []string{"internal/adapter/source/inventoryfile", "internal/adapter/source/sourcetest"},
				Tests:    true,
			},
		},
		{
			// Both lists keep the order the document writes them in, so a rule
			// that adds a directory after its exclusions is read as written.
			name: "directories and exclusions come back in file order",
			files: []string{
				"**/a/source/**",
				"!**/a/source/skip/**",
				"**/a/other/**",
				"!$test",
			},
			want: check.RuleFiles{
				Dirs:     []string{"a/source", "a/other"},
				Excludes: []string{"a/source/skip"},
			},
		},
		{
			name:  "a rule without !$test holds test files",
			files: []string{"**/internal/domain/**"},
			want:  check.RuleFiles{Dirs: []string{"internal/domain"}, Tests: true},
		},
		{
			// $all names every file of the module, not a directory the check
			// could hold packages to.
			name:            "$all is refused",
			files:           []string{"$all", "!$test"},
			wantErr:         true,
			wantErrFragment: "$all",
		},
		{
			name:            "a file glob is refused",
			files:           []string{"**/internal/domain/*.go", "!$test"},
			wantErr:         true,
			wantErrFragment: "**/internal/domain/*.go",
		},
		{
			// Only a whole directory is a shape the check can take out of a
			// rule: a negated file glob would hold some files of a package and
			// not others, which the package check cannot say.
			name:            "a negated file glob is refused",
			files:           []string{"**/internal/domain/**", "!**/internal/domain/*.go", "!$test"},
			wantErr:         true,
			wantErrFragment: "!**/internal/domain/*.go",
		},
		{
			name:            "a bare path is refused",
			files:           []string{"internal/domain", "!$test"},
			wantErr:         true,
			wantErrFragment: "internal/domain",
		},
		{
			name:            "a negated bare path is refused",
			files:           []string{"**/internal/domain/**", "!internal/domain/text", "!$test"},
			wantErr:         true,
			wantErrFragment: "!internal/domain/text",
		},
		{
			name:            "a negated $all is refused",
			files:           []string{"**/internal/domain/**", "!$all", "!$test"},
			wantErr:         true,
			wantErrFragment: "!$all",
		},
		{
			// A rule whose every directory is taken out again holds nothing,
			// which the check cannot tell from a rule that was emptied.
			name:    "a rule that only excludes directories is an error",
			files:   []string{"!**/a/source/**", "!$test"},
			wantErr: true,
		},
		{
			name:    "a rule that names no directory is an error",
			files:   []string{"!$test"},
			wantErr: true,
		},
		{
			name:    "a rule whose files list is empty is an error",
			files:   []string{},
			wantErr: true,
		},
		{
			name:    "a rule that is not there is an error",
			files:   []string{"**/internal/domain/**", "!$test"},
			rule:    "no-such-rule",
			wantErr: true,
		},
		{
			// The character rules are the same as the ones AllowList holds the
			// document to.
			name:            "a character the reader refuses is an error",
			golangci:        filesDoc("# a lone\rCR", "**/internal/domain/**", "!$test"),
			wantErr:         true,
			wantErrFragment: "U+000D",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := tc.golangci
			if doc == "" {
				doc = filesDoc("", tc.files...)
			}

			rule := tc.rule
			if rule == "" {
				rule = "r"
			}

			got, err := check.RuleDirs([]byte(doc), rule)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("RuleDirs(%q) = %v, nil; want an error", rule, got)
				}

				if tc.wantErrFragment != "" && !strings.Contains(err.Error(), tc.wantErrFragment) {
					t.Errorf("RuleDirs(%q) error does not name %q: %v", rule, tc.wantErrFragment, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("RuleDirs(%q): %v", rule, err)
			}

			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("RuleDirs(%q) mismatch (-want +got):\n%s", rule, diff)
			}
		})
	}
}

// sourceProbeModule is the fixture module whose a/source tree mirrors the shape
// the SDK-only rule holds: a sibling helper directory beside the adapters, an
// excluded directory with a subpackage, and test files that import net/http,
// one of them behind //go:build integration.
const sourceProbeModule = "testdata/sourceprobe"

// TestRuleDirsHoldEveryPackageOfTheTestdataTree pins what the files: shape the
// SDK-only rule needs resolves to in a real tree: a directory, a negated glob
// that takes one directory under it - and its subpackages - out of the rule,
// and a sibling helper directory the rule holds too.
//
// Every package under a/source is listed, and each one the rule holds is
// flagged by giving the rule an empty allow list: the flagged set is exactly
// the held set. The excluded directory and its subpackage are listed too and
// stay silent, held by the rule of their own the fixture gives them, as
// inventoryfile holds its own rule in .golangci.yml.
func TestRuleDirsHoldEveryPackageOfTheTestdataTree(t *testing.T) {
	t.Parallel()

	const rule = "cloud-adapters-reach-the-cloud-through-the-sdk"

	doc := golangciDoc(`        ` + rule + `:
          list-mode: strict
          files:
            - "**/a/source/**"
            - "!**/a/source/skip/**"
          allow:
            - errors$
`)

	files, err := check.RuleDirs([]byte(doc), rule)
	if err != nil {
		t.Fatalf("RuleDirs(%q): %v", rule, err)
	}

	want := check.RuleFiles{Dirs: []string{"a/source"}, Excludes: []string{"a/source/skip"}, Tests: true}
	if diff := cmp.Diff(want, files, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("RuleDirs(%q) mismatch (-want +got):\n%s", rule, diff)
	}

	root := fixtureDir(t, sourceProbeModule)

	pkgs, err := check.ListPackages(t.Context(), root, check.Target{GOOS: "linux", GOARCH: "amd64"}, false, nil, "./a/source/...")
	if err != nil {
		t.Fatalf("ListPackages(%s, ./a/source/...): %v", sourceProbeModule, err)
	}

	// The rule as read: one entry per directory, with the exclusions it read.
	var rules []check.Rule

	for _, dir := range files.Dirs {
		rules = append(rules, check.Rule{
			Name:     rule,
			Dir:      dir,
			Excludes: files.Excludes,
			Tests:    files.Tests,
			// Nothing is allowed, so every package the rule holds is flagged
			// for its own imports: the flagged set is the held set.
		})
	}

	// The excluded directory holds a rule of its own, as inventoryfile does in
	// .golangci.yml: without one the package check would report it as outside
	// every directory, which is what the exclusion says it is not.
	rules = append(rules, excludedSourceDirRule())

	messages := check.Packages(root, "example.com/sourceprobe", rules, pkgs)

	// flaggedPaths matches a message to the package it names, and the fixture's
	// packages import standard ones: only the module's own packages name
	// themselves in a message.
	var own []check.Package

	for _, pkg := range pkgs {
		if !pkg.Standard {
			own = append(own, pkg)
		}
	}

	wantFlagged := []string{
		"example.com/sourceprobe/a/source/helper",
		"example.com/sourceprobe/a/source/integration",
		"example.com/sourceprobe/a/source/probe",
		"example.com/sourceprobe/a/source/xprobe",
	}

	if diff := cmp.Diff(wantFlagged, flaggedPaths(t, messages, own)); diff != "" {
		t.Errorf("flagged packages (-want +got):\n%s\nmessages:\n%s", diff, strings.Join(messages, "\n"))
	}
}

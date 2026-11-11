package lint_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/test/internal/yamlread"
)

// forbidigoTags are the tags of forbidigo's groups, each of which starts the
// msg of every entry in its group (.golangci.yml, linters.settings.forbidigo).
var forbidigoTags = []string{"cloud-read-only", "domain-is-pure"}

// exclusionRule is one rule of linters.exclusions.rules in .golangci.yml.
type exclusionRule struct {
	line    int               // of its first key
	fields  map[string]string // each scalar key, unquoted
	linters []string          // nil when the rule names no linters
}

// TestForbidigoExclusionsDropOneGroup checks every rule of
// linters.exclusions.rules that drops forbidigo issues, read from
// .golangci.yml, offline and without golangci-lint. Such a rule names
// forbidigo alone, and its text is exactly 'because "<tag>: [^"]*"$' for a
// known group tag, so it matches the quoted msg at the end of an issue and
// cannot drop another group's issue on a call that passes the tag as a string.
// The rules of every group are pinned, so a new group states its own: the
// cloud-read-only group has none, and the domain-is-pure group exactly two,
// one for every path outside internal/domain/ and one for every _test.go file.
// A rule that names no linters would drop forbidigo issues too, so every rule
// names its linters.
func TestForbidigoExclusionsDropOneGroup(t *testing.T) {
	t.Parallel()

	got := map[string][]map[string]string{}
	for _, tag := range forbidigoTags {
		got[tag] = nil
	}
	for _, r := range exclusionRules(t, moduleRoot(t)) {
		if r.linters == nil {
			t.Errorf(".golangci.yml:%d: an exclusion rule with no linters drops every linter's issues, forbidigo's included; name its linters", r.line)
			continue
		}
		if !slices.Contains(r.linters, "forbidigo") {
			continue
		}
		if len(r.linters) != 1 {
			t.Errorf(".golangci.yml:%d: an exclusion rule for forbidigo names linters %q; give forbidigo a rule of its own", r.line, r.linters)
			continue
		}
		text := r.fields["text"]
		tag, ok := groupOfText(text)
		if !ok {
			t.Errorf(".golangci.yml:%d: forbidigo exclusion text %q; want exactly 'because \"<tag>: [^\"]*\"$' with a tag of %q",
				r.line, text, forbidigoTags)
			continue
		}
		got[tag] = append(got[tag], r.fields)
	}

	domainIsPure := `because "domain-is-pure: [^"]*"$`
	want := map[string][]map[string]string{
		"cloud-read-only": nil,
		"domain-is-pure": {
			{"path-except": `^internal/domain/`, "text": domainIsPure},
			{"path": `_test\.go$`, "text": domainIsPure},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the forbidigo exclusion rules of .golangci.yml, by group (-want +got):\n%s", diff)
	}
}

// groupOfText returns the tag whose exclusion text text is.
func groupOfText(text string) (string, bool) {
	for _, tag := range forbidigoTags {
		if text == `because "`+tag+`: [^"]*"$` {
			return tag, true
		}
	}
	return "", false
}

// exclusionRules reads linters.exclusions.rules from .golangci.yml with the
// shared line reader of test/internal/yamlread, which refuses what it does not
// follow rather than guess at it: every rule is a block mapping of plain keys,
// each key's value one scalar on its line but linters, a list of linter names
// in flow ([a, b]) or block (- a) form.
// A rule written another way fails the test, so it is not silently skipped.
func exclusionRules(t *testing.T, root string) []exclusionRule {
	t.Helper()

	config, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	lines, err := yamlread.SplitLines(config)
	if err != nil {
		t.Fatalf("read .golangci.yml: %v", err)
	}
	block, err := yamlread.Walk(lines, "linters", "exclusions", "rules")
	if err != nil {
		t.Fatalf("read .golangci.yml: %v", err)
	}
	items, err := yamlread.Sequence(block)
	if err != nil {
		t.Fatalf("read .golangci.yml: linters: exclusions: rules: %v", err)
	}

	rules := make([]exclusionRule, 0, len(items))
	for _, it := range items {
		rules = append(rules, exclusionRuleOf(t, it))
	}
	if len(rules) == 0 {
		t.Fatal(".golangci.yml holds no exclusion rule")
	}
	return rules
}

// exclusionRuleOf reads one entry of linters.exclusions.rules.
func exclusionRuleOf(t *testing.T, it yamlread.Item) exclusionRule {
	t.Helper()

	entries, err := yamlread.Mapping(it.Lines)
	if err != nil {
		t.Fatalf(".golangci.yml:%d: an exclusion rule test/lint cannot read: %v", it.Num, err)
	}
	if len(entries) == 0 {
		t.Fatalf(".golangci.yml:%d: an empty exclusion rule", it.Num)
	}

	r := exclusionRule{line: it.Num, fields: map[string]string{}}
	for _, e := range entries {
		if e.Key == "linters" {
			linters, err := yamlread.List(e)
			if err != nil {
				t.Fatalf(".golangci.yml:%d: exclusion rule linters: %v", e.Num, err)
			}
			// An empty list names no linter, so it drops every linter's
			// issues, as a rule with no linters key does.
			if len(linters) > 0 {
				r.linters = linters
			}
			continue
		}
		value, err := yamlread.Single(e)
		if err != nil {
			t.Fatalf(".golangci.yml:%d: an exclusion rule key test/lint cannot read: %v", e.Num, err)
		}
		r.fields[e.Key] = value
	}
	return r
}

package lint_test

import (
	"fmt"
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

// exclusionRules reads linters.exclusions.rules from .golangci.yml under root
// through parseExclusionRules, and fails the test when the file holds no rule.
func exclusionRules(t *testing.T, root string) []exclusionRule {
	t.Helper()

	config, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	rules, err := parseExclusionRules(config)
	if err != nil {
		t.Fatalf("read .golangci.yml: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal(".golangci.yml holds no exclusion rule")
	}
	return rules
}

// parseExclusionRules reads linters.exclusions.rules from doc, a whole
// .golangci.yml, with the shared line reader of test/internal/yamlread, which
// refuses what it does not follow rather than guess at it: every rule is a
// block mapping of plain keys, each key's value one scalar on its line but
// linters, a list of linter names in flow ([a, b]) or block (- a) form. A rule
// written another way is an error, so it is never silently skipped.
func parseExclusionRules(doc []byte) ([]exclusionRule, error) {
	lines, err := yamlread.SplitLines(doc)
	if err != nil {
		return nil, err
	}
	block, err := yamlread.Walk(lines, "linters", "exclusions", "rules")
	if err != nil {
		return nil, err
	}
	items, err := yamlread.Sequence(block)
	if err != nil {
		return nil, fmt.Errorf("linters: exclusions: rules: %w", err)
	}

	rules := make([]exclusionRule, 0, len(items))
	for _, it := range items {
		r, err := exclusionRuleOf(it)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// exclusionRuleOf reads one entry of linters.exclusions.rules.
func exclusionRuleOf(it yamlread.Item) (exclusionRule, error) {
	entries, err := yamlread.Mapping(it.Lines)
	if err != nil {
		return exclusionRule{}, fmt.Errorf("line %d: an exclusion rule test/lint cannot read: %w", it.Num, err)
	}
	if len(entries) == 0 {
		return exclusionRule{}, fmt.Errorf("line %d: an empty exclusion rule", it.Num)
	}

	r := exclusionRule{line: it.Num, fields: map[string]string{}}
	for _, e := range entries {
		if e.Key == "linters" {
			linters, err := yamlread.List(e)
			if err != nil {
				return exclusionRule{}, fmt.Errorf("line %d: exclusion rule linters: %w", e.Num, err)
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
			return exclusionRule{}, fmt.Errorf("line %d: an exclusion rule key test/lint cannot read: %w", e.Num, err)
		}
		r.fields[e.Key] = value
	}
	return r, nil
}

// exclusionDoc returns a .golangci.yml whose linters.exclusions.rules hold the
// lines rules, each already indented under the rules key: the keys
// parseExclusionRules walks, so a case of TestParseExclusionRules below holds
// nothing but the rules it is about. Its first rule starts on line 4.
func exclusionDoc(rules string) string {
	return "linters:\n  exclusions:\n    rules:\n" + rules
}

// exclusionDocTrailingKeys closes the rules block with a top-level key of its
// own. A case whose one rule is a key with no value at all needs it: a rules
// key with no entry below it is a document no caller accepts, so without a key
// after the rules there would be nothing to read the empty entry from.
const exclusionDocTrailingKeys = "\nissues:\n  max-issues-per-linter: 0\n"

// TestParseExclusionRules pins parseExclusionRules, the seam that reads
// linters.exclusions.rules from .golangci.yml: exclusionRules reads the file
// and hands the document to it, so a rule no test reads from disk is still
// read here.
//
// golangci-lint matches a rule that names no linter against every linter, so an
// empty linters list must never be read as "not a forbidigo rule". It is read
// as a rule that names no linter (nil linters), not as an error: the reader
// says what the document says, and TestForbidigoExclusionsDropOneGroup is what
// rejects such a rule, with a message that says why.
//
// Every other way of writing a rule that this reader does not follow is an
// error rather than a guess, so a rule is never silently skipped: a repeated
// key, an upper-case key, a block scalar, a double-quoted text with an escape,
// an anchor or an alias, a single-quoted text that doubles a quote to escape
// it (which YAML would unescape, and this reader refuses to decode), and a
// closing bracket inside a comment, which a comment swallows.
func TestParseExclusionRules(t *testing.T) {
	t.Parallel()

	// ruleLine is the line of the one rule of a document exclusionDoc builds.
	const ruleLine = 4

	tests := []struct {
		name    string
		config  string
		want    []exclusionRule
		wantErr bool
	}{
		{
			name:   "a linters list in block form names its one linter",
			config: exclusionDoc("      - linters:\n          - forbidigo\n"),
			want: []exclusionRule{{
				line:    ruleLine,
				fields:  map[string]string{},
				linters: []string{"forbidigo"},
			}},
		},
		{
			name:   "a linters list in flow form names every linter in it, in order",
			config: exclusionDoc("      - linters: [forbidigo, gosec]\n"),
			want: []exclusionRule{{
				line:    ruleLine,
				fields:  map[string]string{},
				linters: []string{"forbidigo", "gosec"},
			}},
		},
		{
			name: "a rule reads every key and the linters together",
			config: exclusionDoc(`      - linters: [forbidigo]
        path: _test\.go$
        text: 'because "domain-is-pure: [^"]*"$'
`),
			want: []exclusionRule{{
				line: ruleLine,
				fields: map[string]string{
					"path": `_test\.go$`,
					"text": `because "domain-is-pure: [^"]*"$`,
				},
				linters: []string{"forbidigo"},
			}},
		},

		// A rule that names no linter matches every linter's issues, forbidigo's
		// included: the reader reports it as naming none, never as a rule that
		// names one or as no rule at all.
		{
			name:   "an empty linters flow list names no linter",
			config: exclusionDoc("      - linters: []\n"),
			want: []exclusionRule{{
				line:   ruleLine,
				fields: map[string]string{},
			}},
		},
		{
			name:   "an empty linters key names no linter",
			config: exclusionDoc("      - linters:\n") + exclusionDocTrailingKeys,
			want: []exclusionRule{{
				line:   ruleLine,
				fields: map[string]string{},
			}},
		},
		{
			name:   "a rule with no linters key names no linter",
			config: exclusionDoc("      - path: _test\\.go$\n"),
			want: []exclusionRule{{
				line:   ruleLine,
				fields: map[string]string{"path": `_test\.go$`},
			}},
		},

		// Every other way of writing a rule is refused, not guessed at.
		{
			name:    "a repeated key is an error",
			config:  exclusionDoc("      - linters: [forbidigo]\n        text: 'a'\n        text: 'b'\n"),
			wantErr: true,
		},
		{
			name:    "an upper-case key is an error",
			config:  exclusionDoc("      - linters: [forbidigo]\n        Text: 'a'\n"),
			wantErr: true,
		},
		{
			name:    "a literal block scalar text is an error",
			config:  exclusionDoc("      - linters: [forbidigo]\n        text: |\n          because a tag: x$\n"),
			wantErr: true,
		},
		{
			name: "a double-quoted text with an escape is an error",
			config: exclusionDoc(`      - linters: [forbidigo]
        text: "because \"x\"$"
`),
			wantErr: true,
		},
		{
			name:    "an anchored text is an error",
			config:  exclusionDoc("      - linters: [forbidigo]\n        text: &t 'a'\n"),
			wantErr: true,
		},
		{
			name:    "an aliased text is an error",
			config:  exclusionDoc("      - linters: [forbidigo]\n        text: *t\n"),
			wantErr: true,
		},
		{
			// YAML reads 'it''s' as it's; this reader refuses to decode the
			// escape rather than keep the quotes it does not decode.
			name:    "a single-quoted text with '' escaping is an error",
			config:  exclusionDoc("      - linters: [forbidigo]\n        text: 'it''s'\n"),
			wantErr: true,
		},
		{
			// A comment runs to the end of the line, so the bracket in it does
			// not close the list.
			name:    "a bracket in a comment inside a linters flow list is an error",
			config:  exclusionDoc("      - linters: [forbidigo # ]\n"),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseExclusionRules([]byte(tc.config))

			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseExclusionRules() = %v, nil; want an error", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseExclusionRules(): %v", err)
			}

			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(exclusionRule{})); diff != "" {
				t.Errorf("parseExclusionRules() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

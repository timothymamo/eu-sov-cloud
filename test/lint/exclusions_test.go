package lint_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
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

// exclusionRules reads linters.exclusions.rules from .golangci.yml line by
// line, as every rule there is written: "- <key>: <scalar>" opens a rule at
// six spaces, its other keys follow at eight, and linters is a block list at
// ten. A line in the list that fits none of these fails the test, so a rule
// written another way is not silently skipped.
func exclusionRules(t *testing.T, root string) []exclusionRule {
	t.Helper()

	config, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(config), "\n")
	start := -1
	for i := range lines {
		if lines[i] == "    rules:" && sectionOf(lines, i) == "linters" && parentOf(lines, i) == "  exclusions:" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatal(".golangci.yml holds no linters.exclusions.rules")
	}

	var rules []exclusionRule
	inLinters := false
	for i := start; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent < 6 {
			break
		}
		switch {
		case indent == 6 && strings.HasPrefix(trimmed, "- "):
			rules = append(rules, exclusionRule{line: i + 1, fields: map[string]string{}})
			inLinters = setRuleKey(t, &rules[len(rules)-1], i+1, strings.TrimPrefix(trimmed, "- "))
		case indent == 8 && len(rules) > 0:
			inLinters = setRuleKey(t, &rules[len(rules)-1], i+1, trimmed)
		case indent == 10 && inLinters && strings.HasPrefix(trimmed, "- "):
			r := &rules[len(rules)-1]
			r.linters = append(r.linters, yamlScalar(t, i+1, strings.TrimPrefix(trimmed, "- ")))
		default:
			t.Fatalf(".golangci.yml:%d: an exclusion rule line test/lint cannot read: %q", i+1, line)
		}
	}
	if len(rules) == 0 {
		t.Fatal(".golangci.yml holds no exclusion rule")
	}
	return rules
}

// setRuleKey sets the key on line kv of rule r, and reports whether it opens
// the linters list.
func setRuleKey(t *testing.T, r *exclusionRule, line int, kv string) bool {
	t.Helper()

	key, value, ok := strings.Cut(kv, ":")
	value = strings.TrimSpace(value)
	if !ok || key == "" || strings.ContainsAny(key, " \t") {
		t.Fatalf(".golangci.yml:%d: an exclusion rule line test/lint cannot read: %q", line, kv)
	}
	if _, dup := r.fields[key]; dup || (key == "linters" && r.linters != nil) {
		t.Fatalf(".golangci.yml:%d: exclusion rule key %s set twice", line, key)
	}
	if key == "linters" {
		if value != "" {
			t.Fatalf(".golangci.yml:%d: write linters as a block list, one linter a line", line)
		}
		r.linters = []string{}
		return true
	}
	r.fields[key] = yamlScalar(t, line, value)
	return false
}

// yamlScalar unquotes a single-quoted, double-quoted or plain YAML scalar.
func yamlScalar(t *testing.T, line int, s string) string {
	t.Helper()

	switch {
	case len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'':
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	case len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"':
		v, err := strconv.Unquote(s)
		if err != nil {
			t.Fatalf(".golangci.yml:%d: double-quoted scalar %s: %v", line, s, err)
		}
		return v
	case s == "" || strings.ContainsAny(s[:1], `'"[{&*!|>%@`+"`") || strings.Contains(s, " #"):
		t.Fatalf(".golangci.yml:%d: a scalar test/lint cannot read: %q", line, s)
	}
	return s
}

// sectionOf returns the top-level key above line i.
func sectionOf(lines []string, i int) string {
	for ; i >= 0; i-- {
		if l := lines[i]; l != "" && l[0] != ' ' && l[0] != '#' {
			return strings.TrimSuffix(l, ":")
		}
	}
	return ""
}

// parentOf returns the nearest line above line i indented less than it, which
// is not a comment.
func parentOf(lines []string, i int) string {
	indent := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
	for j := i - 1; j >= 0; j-- {
		l := lines[j]
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if len(l)-len(strings.TrimLeft(l, " ")) < indent {
			return l
		}
	}
	return ""
}

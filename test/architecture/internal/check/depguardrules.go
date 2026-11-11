package check

import (
	"fmt"
	"slices"
	"strings"
)

// DepguardRule is one depguard rule of a .golangci.yml document, read whole:
// what TestDepguardRulesArePinned compares with its pinned copy (issue #80).
type DepguardRule struct {
	// Name is the rule's key under linters: settings: depguard: rules:.
	Name string
	// ListMode is the rule's list-mode, empty when it sets none.
	ListMode string
	// Files are the entries of the rule's files list, as written, without
	// their quotes.
	Files []string
	// Allow are the entries of the rule's allow list, as written, without
	// their quotes: the trailing $ of an exact entry is kept.
	Allow []string
	// Deny is the pkg of each entry of the rule's deny list, in file order.
	Deny []string
}

// DepguardRules returns every depguard rule of a .golangci.yml document, in
// file order: its list-mode, its files and allow lists and the pkg of each deny
// entry. The desc of a deny entry is free prose and is not read.
//
// A deny list may be shared through a YAML anchor on one rule's deny: key
// (deny: &name) and an alias on a later one (deny: *name); the alias reads as
// the anchored entries. That is the only anchor or alias it follows.
//
// It fails closed: an error when the document has no depguard rules, when a
// rule body holds a key other than list-mode, files, allow and deny, when a
// list-mode is written other than original, strict or lax, when a deny entry
// has no pkg or a key other than pkg and desc, when an alias names no anchor
// above it, and wherever the line reader refuses the document.
func DepguardRules(golangci []byte) ([]DepguardRule, error) {
	lines, err := splitLines(golangci)
	if err != nil {
		return nil, fmt.Errorf("read .golangci.yml: %w", err)
	}

	const path = "linters: settings: depguard: rules"

	block, err := walk(lines, "linters", "settings", "depguard", "rules")
	if err != nil {
		return nil, fmt.Errorf("read the depguard rules: %w", err)
	}

	entries, err := mapping(block)
	if err != nil {
		return nil, fmt.Errorf("read the depguard rules: %w", err)
	}

	anchors := map[string][]string{}
	rules := make([]DepguardRule, 0, len(entries))

	for _, e := range entries {
		rule, err := readDepguardRule(e, path+": "+e.key, anchors)
		if err != nil {
			return nil, fmt.Errorf("read depguard rule %s: %w", e.key, err)
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

// readDepguardRule reads the rule at e, recording the anchor of its deny list,
// if any, in anchors and resolving an alias from it.
func readDepguardRule(e entry, path string, anchors map[string][]string) (DepguardRule, error) {
	rule := DepguardRule{Name: e.key}

	body, err := blockValue(e, path)
	if err != nil {
		return rule, err
	}

	entries, err := mapping(body)
	if err != nil {
		return rule, err
	}

	rule.ListMode, _, err = listMode(entries)
	if err != nil {
		return rule, err
	}

	for _, field := range entries {
		switch field.key {
		case "list-mode":
			// Read by listMode above.
		case "files":
			rule.Files, err = list(field)
		case "allow":
			rule.Allow, err = list(field)
		case "deny":
			rule.Deny, err = readDeny(field, anchors)
		default:
			err = fmt.Errorf("line %d: key %s, which this check does not read: want list-mode, files, allow or deny", field.num, field.key)
		}

		if err != nil {
			return rule, err
		}
	}

	return rule, nil
}

// readDeny returns the pkg of each entry of a deny list: a block sequence below
// the key, optionally anchored (deny: &name), or an alias of an anchor read
// before it (deny: *name).
func readDeny(e entry, anchors map[string][]string) ([]string, error) {
	anchor := ""

	switch {
	case e.value == "":
	case strings.HasPrefix(e.value, "*"):
		name, err := anchorName(e, e.value[1:])
		if err != nil {
			return nil, err
		}

		if len(e.block) != 0 {
			return nil, fmt.Errorf("line %d: deny is the alias *%s and has lines below it", e.num, name)
		}

		pkgs, ok := anchors[name]
		if !ok {
			return nil, fmt.Errorf("line %d: deny is the alias *%s, and no deny list above it has that anchor", e.num, name)
		}

		return slices.Clone(pkgs), nil
	case strings.HasPrefix(e.value, "&"):
		name, err := anchorName(e, e.value[1:])
		if err != nil {
			return nil, err
		}

		if _, ok := anchors[name]; ok {
			return nil, fmt.Errorf("line %d: the anchor &%s is repeated", e.num, name)
		}

		anchor = name
	default:
		return nil, fmt.Errorf("line %d: deny has %q on its own line, want an anchor, an alias or nothing", e.num, e.value)
	}

	items, err := sequence(e.block)
	if err != nil {
		return nil, fmt.Errorf("deny: %w", err)
	}

	pkgs := make([]string, 0, len(items))

	for _, it := range items {
		pkg, err := denyPkg(it)
		if err != nil {
			return nil, err
		}

		pkgs = append(pkgs, pkg)
	}

	if anchor != "" {
		anchors[anchor] = pkgs
	}

	return pkgs, nil
}

// anchorName returns the name of an anchor or alias written on e's line,
// refusing an empty name and anything after it but a comment.
func anchorName(e entry, text string) (string, error) {
	name, rest, _ := strings.Cut(text, " ")
	if rest = strings.TrimSpace(rest); rest != "" && !strings.HasPrefix(rest, "#") {
		return "", fmt.Errorf("line %d: %s has text after its anchor or alias", e.num, e.key)
	}

	if name == "" || strings.ContainsAny(name, "&*!|>{}[],#'\"") {
		return "", fmt.Errorf("line %d: %s has an anchor or alias name this reader does not follow", e.num, e.key)
	}

	return name, nil
}

// denyPkg returns the pkg of one deny entry, a mapping of pkg and desc. The
// desc is prose and is not read.
func denyPkg(it item) (string, error) {
	entries, err := mapping(it.lines)
	if err != nil {
		return "", fmt.Errorf("line %d: a deny entry: %w", it.num, err)
	}

	pkg := ""

	for _, e := range entries {
		switch e.key {
		case "pkg":
			pkg, err = single(e)
			if err != nil {
				return "", fmt.Errorf("a deny entry: %w", err)
			}
		case "desc":
		default:
			return "", fmt.Errorf("line %d: a deny entry has key %s, want pkg and desc", e.num, e.key)
		}
	}

	if pkg == "" {
		return "", fmt.Errorf("line %d: a deny entry has no pkg", it.num)
	}

	return pkg, nil
}

// PinnedRules compares the depguard rules read from a document, got, with
// their pinned copy, want, and returns one message per difference, sorted, each
// starting with the name of the rule it is about: a rule missing or not
// pinned, a changed list-mode, and every files, allow or deny entry removed or
// added. A changed entry reads as one removed and one added. The order of a
// list's entries is not compared, their number is; an absent list and an empty
// one are the same. It returns nil when the rules match.
func PinnedRules(got, want []DepguardRule) []string {
	var messages []string

	for _, w := range want {
		i := slices.IndexFunc(got, func(r DepguardRule) bool { return r.Name == w.Name })
		if i < 0 {
			messages = append(messages, w.Name+": the pinned rule is missing from .golangci.yml")

			continue
		}

		g := got[i]

		if g.ListMode != w.ListMode {
			messages = append(messages, fmt.Sprintf("%s: list-mode is %s, the pin has %s", w.Name, modeText(g.ListMode), modeText(w.ListMode)))
		}

		messages = append(messages, entryDiff(w.Name, "files", g.Files, w.Files)...)
		messages = append(messages, entryDiff(w.Name, "allow", g.Allow, w.Allow)...)
		messages = append(messages, entryDiff(w.Name, "deny", g.Deny, w.Deny)...)
	}

	for _, g := range got {
		if !slices.ContainsFunc(want, func(r DepguardRule) bool { return r.Name == g.Name }) {
			messages = append(messages, g.Name+": the rule is in .golangci.yml but not pinned")
		}
	}

	slices.Sort(messages)

	return messages
}

// modeText returns a list-mode for a message, naming the absent one.
func modeText(mode string) string {
	if mode == "" {
		return "unset"
	}

	return mode
}

// entryDiff returns a message for every entry of a rule's list that the pin
// has and the document lost, and every one the document has beyond the pin,
// counting repeated entries.
func entryDiff(rule, list string, got, want []string) []string {
	var messages []string

	for _, entry := range multisetMinus(want, got) {
		messages = append(messages, fmt.Sprintf("%s: %s entry %q is pinned but missing from .golangci.yml", rule, list, entry))
	}

	for _, entry := range multisetMinus(got, want) {
		messages = append(messages, fmt.Sprintf("%s: %s entry %q is in .golangci.yml but not pinned", rule, list, entry))
	}

	return messages
}

// multisetMinus returns the entries of a that b does not account for, each
// entry of b cancelling one equal entry of a.
func multisetMinus(a, b []string) []string {
	left := map[string]int{}
	for _, s := range b {
		left[s]++
	}

	var out []string

	for _, s := range a {
		if left[s] > 0 {
			left[s]--

			continue
		}

		out = append(out, s)
	}

	return out
}

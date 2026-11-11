package check

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// AllowEntry is one entry of a depguard allow list, read the way depguard reads
// it.
type AllowEntry struct {
	// Path is the import path the entry names, without the trailing $ of an
	// exact entry. A prefix entry keeps what it is written with, a trailing /
	// included.
	Path string
	// Prefix is set for an entry written without a trailing $: depguard allows
	// every import path that starts with Path, as a plain string prefix, so
	// github.com/aws/aws-sdk-go-v2/ allows every package of that SDK and os
	// would allow os/exec and osfoo too. An entry with $ allows Path alone.
	Prefix bool
}

// allows reports whether the entry allows the import path imp, as depguard
// matches it: exactly, or by plain string prefix for a prefix entry.
func (e AllowEntry) allows(imp string) bool {
	if e.Prefix {
		return strings.HasPrefix(imp, e.Path)
	}

	return imp == e.Path
}

// AllowList returns the allow list of the depguard rule named rule in a
// .golangci.yml document: the entries under linters: settings: depguard: rules:
// <rule>: allow:, in file order, without their quotes. An entry that ends in $
// comes back exact, its $ removed; any other comes back as a prefix entry,
// written as it is, since depguard allows every import path that starts with
// it (the SDK-only rule names the cloud SDK roots that way, issue #79).
//
// The rule is found by walking those keys by indentation, not by its line or
// its place among the rules, so edits elsewhere in the file do not move it. It
// fails closed: an error when the rule is missing, when its list-mode is not
// strict (depguard's default mode lets through what no list names), when it
// has no allow entry, or when an entry does not name an import path: an exact
// one once the $ is removed, a prefix one once a trailing / is removed. $gostd,
// which depguard expands to the standard library's top-level directories, is
// no import path and so is refused.
func AllowList(golangci []byte, rule string) ([]AllowEntry, error) {
	lines, err := splitLines(golangci)
	if err != nil {
		return nil, fmt.Errorf("read .golangci.yml: %w", err)
	}

	body, err := walk(lines, "linters", "settings", "depguard", "rules", rule)
	if err != nil {
		return nil, fmt.Errorf("read depguard rule %s: %w", rule, err)
	}

	allow, err := readAllow(body)
	if err != nil {
		return nil, fmt.Errorf("read depguard rule %s: %w", rule, err)
	}

	return allow, nil
}

// StrictRules returns the name of every depguard rule in a .golangci.yml
// document whose list-mode is strict, in file order: in any other mode
// depguard allows what no list names, so these are the rules whose allow lists
// say all a package may import.
//
// It fails closed: an error when the document has no depguard rules, when a
// rule is not a block mapping the reader can read, and when a list-mode is
// written other than original, strict or lax. A mode in another spelling, such
// as Strict, is refused rather than counted as not strict, since this check
// cannot tell what depguard makes of it.
func StrictRules(golangci []byte) ([]string, error) {
	lines, err := splitLines(golangci)
	if err != nil {
		return nil, fmt.Errorf("read .golangci.yml: %w", err)
	}

	const path = "linters: settings: depguard: rules"

	block, err := walk(lines, "linters", "settings", "depguard", "rules")
	if err != nil {
		return nil, fmt.Errorf("read the depguard rules: %w", err)
	}

	rules, err := mapping(block)
	if err != nil {
		return nil, fmt.Errorf("read the depguard rules: %w", err)
	}

	var strict []string

	for _, rule := range rules {
		body, err := blockValue(rule, path+": "+rule.key)
		if err != nil {
			return nil, fmt.Errorf("read depguard rule %s: %w", rule.key, err)
		}

		entries, err := mapping(body)
		if err != nil {
			return nil, fmt.Errorf("read depguard rule %s: %w", rule.key, err)
		}

		mode, _, err := listMode(entries)
		if err != nil {
			return nil, fmt.Errorf("read depguard rule %s: %w", rule.key, err)
		}

		if mode == "strict" {
			strict = append(strict, rule.key)
		}
	}

	return strict, nil
}

// listMode returns the list-mode of a rule's body and the line it is on, or
// an empty mode when the body has none. It refuses a mode written other than
// original, strict or lax.
func listMode(entries []entry) (string, int, error) {
	for _, e := range entries {
		if e.key != "list-mode" {
			continue
		}

		mode, err := single(e)
		if err != nil {
			return "", 0, err
		}

		switch mode {
		case "original", "strict", "lax":
			return mode, e.num, nil
		default:
			return "", 0, fmt.Errorf("line %d: list-mode %s, want original, strict or lax, written in lower case", e.num, mode)
		}
	}

	return "", 0, nil
}

// readAllow checks the list-mode of a rule's body and returns its allow
// entries.
func readAllow(body []line) ([]AllowEntry, error) {
	entries, err := mapping(body)
	if err != nil {
		return nil, err
	}

	mode, modeLine, err := listMode(entries)
	if err != nil {
		return nil, err
	}

	if mode == "" {
		return nil, errors.New("no list-mode, want strict")
	}

	if mode != "strict" {
		return nil, fmt.Errorf("line %d: list-mode is %s, want strict: in any other mode depguard allows what no list names", modeLine, mode)
	}

	var allow *entry

	for i := range entries {
		if entries[i].key == "allow" {
			allow = &entries[i]
		}
	}

	if allow == nil {
		return nil, errors.New("no allow list")
	}

	values, err := list(*allow)
	if err != nil {
		return nil, err
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("line %d: the allow list is empty", allow.num)
	}

	out := make([]AllowEntry, 0, len(values))

	for _, value := range values {
		if name, exact := strings.CutSuffix(value, "$"); exact {
			if !isImportPath(name) {
				return nil, fmt.Errorf("allow entry %s does not name an import path", value)
			}

			out = append(out, AllowEntry{Path: name})

			continue
		}

		if !isImportPath(strings.TrimSuffix(value, "/")) {
			return nil, fmt.Errorf("allow entry %s does not name an import path or an import path prefix", value)
		}

		out = append(out, AllowEntry{Path: value, Prefix: true})
	}

	return out, nil
}

// isImportPath reports whether name looks like an import path: clean, relative,
// in the characters Go allows in one.
func isImportPath(name string) bool {
	if name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.HasPrefix(name, ".") {
		return false
	}

	for _, r := range name {
		if !isASCIIAlnum(r) && !strings.ContainsRune("._~/+-", r) {
			return false
		}
	}

	return true
}

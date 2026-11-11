package check

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/timothymamo/eu-sov-cloud/test/internal/yamlread"
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

// allows reports whether the entry, on its own, allows the import path imp:
// exactly, or by plain string prefix for a prefix entry. depguard tests an
// import against one entry of a list only, the nearest preceding one
// (allowedBy).
func (e AllowEntry) allows(imp string) bool {
	if e.Prefix {
		return strings.HasPrefix(imp, e.Path)
	}

	return imp == e.Path
}

// written returns the entry as .golangci.yml writes it: an exact entry with
// its trailing $.
func (e AllowEntry) written() string {
	if e.Prefix {
		return e.Path
	}

	return e.Path + "$"
}

// allowedBy reports whether an allow list allows the import path imp the way
// depguard v2.2.1 decides it (settings.go, strInPrefixList): the entries as
// written, $ included, are sorted; the entry that sorts nearest before imp,
// comparing each with its trailing $ removed, is the only one tested. So with
// a/ and a/b$ in one list, a/c is refused although a/ alone would allow it.
//
// allow must already be sorted by sortAllow: Packages sorts every rule's list
// once (sortedAllowLists) rather than once per import.
func allowedBy(allow []AllowEntry, imp string) bool {
	i := sort.Search(len(allow), func(i int) bool { return allow[i].Path > imp }) - 1
	if i < 0 {
		return false
	}

	return allow[i].allows(imp)
}

// sortAllow sorts an allow list in place the way depguard sorts it: by the
// entries as written, $ included.
func sortAllow(allow []AllowEntry) {
	slices.SortFunc(allow, func(a, b AllowEntry) int { return strings.Compare(a.written(), b.written()) })
}

// sortedAllowLists returns a copy of rules whose allow lists are copies sorted
// by sortAllow, leaving the caller's rules as they are.
func sortedAllowLists(rules []Rule) []Rule {
	sorted := slices.Clone(rules)
	for i := range sorted {
		sorted[i].Allow = slices.Clone(sorted[i].Allow)
		sortAllow(sorted[i].Allow)
	}

	return sorted
}

// AllowList returns the allow list of the depguard rule named rule in a
// .golangci.yml document: the entries under linters: settings: depguard: rules:
// <rule>: allow:, in file order, without their quotes. An entry that ends in $
// comes back exact, its $ removed; one that ends in / comes back as a prefix
// entry, written as it is, since depguard allows every import path that starts
// with it (the SDK-only rule names the cloud SDK roots that way, issue #79).
// Any other entry is refused: depguard reads it as a plain string prefix, so os
// would allow osfoo and an entry that lost its $ would widen the rule.
//
// The rule's deny list is read too, its entries a scalar path or a mapping of
// pkg and desc, and the rule is refused when an allow entry allows the path a
// deny entry names: depguard then decides that import by entry length, so the
// allow list would not say what the rule allows. A deny list given as an
// alias is refused, as this reader of one rule cannot see its anchor.
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
	lines, err := yamlread.SplitLines(golangci)
	if err != nil {
		return nil, fmt.Errorf("read .golangci.yml: %w", err)
	}

	body, err := yamlread.Walk(lines, "linters", "settings", "depguard", "rules", rule)
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
	lines, err := yamlread.SplitLines(golangci)
	if err != nil {
		return nil, fmt.Errorf("read .golangci.yml: %w", err)
	}

	const path = "linters: settings: depguard: rules"

	block, err := yamlread.Walk(lines, "linters", "settings", "depguard", "rules")
	if err != nil {
		return nil, fmt.Errorf("read the depguard rules: %w", err)
	}

	rules, err := yamlread.Mapping(block)
	if err != nil {
		return nil, fmt.Errorf("read the depguard rules: %w", err)
	}

	var strict []string

	for _, rule := range rules {
		body, err := yamlread.BlockValue(rule, path+": "+rule.Key)
		if err != nil {
			return nil, fmt.Errorf("read depguard rule %s: %w", rule.Key, err)
		}

		entries, err := yamlread.Mapping(body)
		if err != nil {
			return nil, fmt.Errorf("read depguard rule %s: %w", rule.Key, err)
		}

		mode, _, err := listMode(entries)
		if err != nil {
			return nil, fmt.Errorf("read depguard rule %s: %w", rule.Key, err)
		}

		if mode == "strict" {
			strict = append(strict, rule.Key)
		}
	}

	return strict, nil
}

// listMode returns the list-mode of a rule's body and the line it is on, or
// an empty mode when the body has none. It refuses a mode written other than
// original, strict or lax.
func listMode(entries []yamlread.Entry) (string, int, error) {
	for _, e := range entries {
		if e.Key != "list-mode" {
			continue
		}

		mode, err := yamlread.Single(e)
		if err != nil {
			return "", 0, err
		}

		switch mode {
		case "original", "strict", "lax":
			return mode, e.Num, nil
		default:
			return "", 0, fmt.Errorf("line %d: list-mode %s, want original, strict or lax, written in lower case", e.Num, mode)
		}
	}

	return "", 0, nil
}

// readAllow checks the list-mode of a rule's body and returns its allow
// entries.
func readAllow(body []yamlread.Line) ([]AllowEntry, error) {
	entries, err := yamlread.Mapping(body)
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

	var allow *yamlread.Entry

	for i := range entries {
		if entries[i].Key == "allow" {
			allow = &entries[i]
		}
	}

	if allow == nil {
		return nil, errors.New("no allow list")
	}

	values, err := yamlread.List(*allow)
	if err != nil {
		return nil, err
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("line %d: the allow list is empty", allow.Num)
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

		root, ok := strings.CutSuffix(value, "/")
		if !ok {
			return nil, fmt.Errorf("allow entry %s ends in neither $ nor /: depguard would allow every import path that starts with it, look-alikes such as %sfoo included; write %s$ for the package or %s/ for its tree", value, value, value, value)
		}

		if !isImportPath(root) {
			return nil, fmt.Errorf("allow entry %s does not name an import path or an import path prefix", value)
		}

		out = append(out, AllowEntry{Path: value, Prefix: true})
	}

	if err := checkDeny(entries, out); err != nil {
		return nil, err
	}

	return out, nil
}

// checkDeny reads the deny list of a rule's body, if it has one, and returns
// an error when an allow entry allows the path a deny entry names: depguard
// then decides that import by which entry is longer, not by which list holds
// it, so the allow list alone no longer says what the rule allows. A deny
// entry no allow entry allows, such as net beside net/netip$, leaves the allow
// list as the whole answer and is fine.
func checkDeny(entries []yamlread.Entry, allow []AllowEntry) error {
	for _, e := range entries {
		if e.Key != "deny" {
			continue
		}

		paths, err := readDenyPaths(e)
		if err != nil {
			return err
		}

		for _, p := range paths {
			name := strings.TrimSuffix(p, "$")

			for _, a := range allow {
				if a.allows(name) {
					return fmt.Errorf("line %d: deny entry %s is allowed by allow entry %s: depguard decides that import by entry length, so the allow list does not say what the rule allows", e.Num, p, a.written())
				}
			}
		}
	}

	return nil
}

// readDenyPaths returns the path of each entry of a deny list: a block
// sequence below the key, optionally anchored (deny: &name), whose entries are
// a scalar path or a mapping of pkg and desc. It fails closed on an alias,
// whose entries this reader of one rule cannot see.
func readDenyPaths(e yamlread.Entry) ([]string, error) {
	if e.Value != "" && !strings.HasPrefix(e.Value, "&") {
		return nil, fmt.Errorf("line %d: deny has %q on its own line, want an anchor or nothing", e.Num, e.Value)
	}

	if strings.HasPrefix(e.Value, "&") {
		if _, err := anchorName(e, e.Value[1:]); err != nil {
			return nil, err
		}
	}

	items, err := yamlread.Sequence(e.Block)
	if err != nil {
		return nil, fmt.Errorf("deny: %w", err)
	}

	paths := make([]string, 0, len(items))

	for _, it := range items {
		if len(it.Lines) == 1 {
			if _, _, isKey := yamlread.KeyValue(it.Lines[0].Text); !isKey {
				value, err := yamlread.Scalar(it.Lines[0].Text)
				if err != nil {
					return nil, fmt.Errorf("line %d: a deny entry: %w", it.Num, err)
				}

				paths = append(paths, value)

				continue
			}
		}

		pkg, err := denyPkg(it)
		if err != nil {
			return nil, err
		}

		paths = append(paths, pkg)
	}

	return paths, nil
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

// isASCIIAlnum reports whether r is an ASCII letter or digit.
func isASCIIAlnum(r rune) bool {
	return ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9')
}

package check

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file reads the small part of YAML that .goreleaser.yaml and
// .golangci.yml use on the paths the checks follow, with the standard library
// only. It reads by indentation, line by line: a key's value is the text on its
// own line and the lines below it indented further. Along the path it walks and
// in the values it reads, it refuses everything else (anchors, aliases, merge
// keys, quoted or upper-case keys, tags, escapes, multi-line flow lists, folded
// or chomped block scalars, a repeated key) rather than guess at it; a literal
// block scalar (|) it reads only where a check asks for one (literalBlock). The values of the keys it steps over
// are kept whole, by indentation, and never read. Anywhere in the document,
// comments included, it refuses the characters YAML readers do not agree on
// (checkLine).

// line is one line of a YAML document that holds content: neither blank nor
// only a comment.
type line struct {
	// num is the line number, counted from 1.
	num int
	// indent is the number of spaces before text.
	indent int
	// text is the rest of the line, trailing white space removed.
	text string
}

// entry is one key of a block mapping.
type entry struct {
	key string
	// num is the line number of the key.
	num int
	// value is the text after the key on its own line, empty when there is
	// none or only a comment.
	value string
	// block are the lines below the key that belong to it.
	block []line
}

// item is one entry of a block sequence.
type item struct {
	// num is the line number of the entry's dash.
	num int
	// lines are the entry's value: the text after the dash, as a line indented
	// to the column it starts in, then the lines below that belong to it.
	lines []line
}

// splitLines returns the content lines of a YAML document. Lines end in LF or
// CRLF. It refuses a character checkLine refuses, anywhere in the document,
// comments included, and a directive or document marker at the start of a
// line, which could start a second document.
func splitLines(doc []byte) ([]line, error) {
	var out []line

	for i, raw := range strings.Split(string(doc), "\n") {
		num := i + 1
		raw = strings.TrimSuffix(raw, "\r")

		if err := checkLine(raw); err != nil {
			return nil, fmt.Errorf("line %d: %w", num, err)
		}

		text := strings.TrimLeft(raw, " ")
		indent := len(raw) - len(text)

		text = strings.TrimRight(text, " ")
		if text == "" || text[0] == '#' {
			continue
		}

		if indent == 0 && (text[0] == '%' || strings.HasPrefix(text, "---") || strings.HasPrefix(text, "...")) {
			return nil, fmt.Errorf("line %d: a directive or document marker, which this reader does not follow", num)
		}

		out = append(out, line{num: num, indent: indent, text: text})
	}

	return out, nil
}

// checkLine refuses invalid UTF-8 and every character that YAML readers do not
// agree on, wherever it sits on the line: a control character (a tab, a CR not
// followed by LF, any other C0 control, DEL and the C1 controls, NEL among
// them), the line and paragraph separators U+2028 and U+2029, and the
// byte-order mark U+FEFF. YAML 1.2 reads a lone CR as a line break, and
// libyaml-derived readers NEL, LS and PS too, where this reader would keep
// them inside a comment: a list entry after one would be hidden from the
// check, not from the tool. Other printable text, non-ASCII included, is
// accepted.
func checkLine(raw string) error {
	if !utf8.ValidString(raw) {
		return errors.New("invalid UTF-8")
	}

	column := 0

	for _, r := range raw {
		column++

		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || r == '\uFEFF' {
			return fmt.Errorf("column %d: character %U, which YAML readers do not agree on", column, r)
		}
	}

	return nil
}

// mapping reads lines as one block mapping: every line at the indentation of
// the first is a key, and the lines after a key that are indented further, or
// that are sequence entries at its own indentation when the key has no value on
// its line, belong to it. It refuses a line indented less than the first, a
// line at the mapping's indentation that is not a plain key, and a key the
// mapping already has.
func mapping(lines []line) ([]entry, error) {
	if len(lines) == 0 {
		return nil, nil
	}

	indent := lines[0].indent

	var entries []entry

	for _, l := range lines {
		if l.indent < indent {
			return nil, fmt.Errorf("line %d: indented less than the mapping it is in", l.num)
		}

		if len(entries) > 0 {
			last := &entries[len(entries)-1]
			if l.indent > indent || (isSequenceEntry(l.text) && last.value == "") {
				last.block = append(last.block, l)

				continue
			}
		}

		key, value, ok := keyValue(l.text)
		if !ok {
			return nil, fmt.Errorf("line %d: %q is not a plain key this reader understands", l.num, l.text)
		}

		for _, e := range entries {
			if e.key == key {
				return nil, fmt.Errorf("line %d: key %s is repeated from line %d", l.num, key, e.num)
			}
		}

		entries = append(entries, entry{key: key, num: l.num, value: value})
	}

	return entries, nil
}

// sequence reads lines as one block sequence: every line at the indentation of
// the first starts an entry with a dash, and the lines indented further belong
// to the entry above them. It refuses any other line.
func sequence(lines []line) ([]item, error) {
	if len(lines) == 0 {
		return nil, nil
	}

	indent := lines[0].indent

	var items []item

	for _, l := range lines {
		switch {
		case l.indent < indent:
			return nil, fmt.Errorf("line %d: indented less than the sequence it is in", l.num)
		case l.indent > indent:
			last := &items[len(items)-1]
			last.lines = append(last.lines, l)
		case !isSequenceEntry(l.text):
			return nil, fmt.Errorf("line %d: %q is not a sequence entry", l.num, l.text)
		default:
			it := item{num: l.num}
			if rest := strings.TrimLeft(l.text[1:], " "); rest != "" {
				it.lines = []line{{num: l.num, indent: l.indent + len(l.text) - len(rest), text: rest}}
			}

			items = append(items, it)
		}
	}

	return items, nil
}

// walk follows keys from the top-level mapping of lines, each key's value a
// block mapping below it, and returns the lines of the last key's value.
func walk(lines []line, keys ...string) ([]line, error) {
	for i, key := range keys {
		path := strings.Join(keys[:i+1], ": ")

		entries, err := mapping(lines)
		if err != nil {
			return nil, fmt.Errorf("read the mapping that holds %s: %w", path, err)
		}

		found := false

		for _, e := range entries {
			if e.key != key {
				continue
			}

			lines, err = blockValue(e, path)
			if err != nil {
				return nil, err
			}

			found = true
		}

		if !found {
			return nil, fmt.Errorf("no %s", path)
		}
	}

	return lines, nil
}

// blockValue returns the lines of e's value, a block below the key at path. It
// refuses a value on the key's own line and an empty one.
func blockValue(e entry, path string) ([]line, error) {
	if e.value != "" {
		return nil, fmt.Errorf("line %d: %s has %q on its own line, want a block below it", e.num, path, e.value)
	}

	if len(e.block) == 0 {
		return nil, fmt.Errorf("line %d: %s is empty", e.num, path)
	}

	return e.block, nil
}

// list reads the value of e as a list of scalars: a flow list on the key's own
// line ([a, b]) or a block sequence below it (- a). A key with no value is an
// empty list.
func list(e entry) ([]string, error) {
	if e.value != "" {
		if len(e.block) != 0 {
			return nil, fmt.Errorf("line %d: %s has a value on its own line and lines below it", e.num, e.key)
		}

		values, err := flowList(e.value)
		if err != nil {
			return nil, fmt.Errorf("line %d: %s: %w", e.num, e.key, err)
		}

		return values, nil
	}

	items, err := sequence(e.block)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", e.key, err)
	}

	values := make([]string, 0, len(items))

	for _, it := range items {
		if len(it.lines) != 1 {
			return nil, fmt.Errorf("line %d: an entry of %s that is not one scalar on its line", it.num, e.key)
		}

		value, err := scalar(it.lines[0].text)
		if err != nil {
			return nil, fmt.Errorf("line %d: an entry of %s: %w", it.num, e.key, err)
		}

		values = append(values, value)
	}

	return values, nil
}

// single reads the value of e as one scalar on the key's own line.
func single(e entry) (string, error) {
	if len(e.block) != 0 {
		return "", fmt.Errorf("line %d: %s has lines below it, want one value on its line", e.num, e.key)
	}

	value, err := scalar(e.value)
	if err != nil {
		return "", fmt.Errorf("line %d: %s: %w", e.num, e.key, err)
	}

	return value, nil
}

// literalBlock reads the value of e, a literal block scalar (|) with clip
// chomping and its indentation taken from its first line, from raw, the lines
// of the document e is in with their line ends removed. It returns the block's
// lines, that indentation removed, joined by newlines, without the newline
// after the last one. It reads raw rather than e.block because a block's lines
// are content, not comments: a line in it that starts with # is part of the
// value, and so is a blank line between two others.
//
// It refuses a block whose first line is not its first content line, and a
// content line indented less than the first: YAML ends the block there, where
// the mapping reader would keep the line in it.
func literalBlock(raw []string, e entry) (string, error) {
	if e.value != "|" {
		return "", fmt.Errorf("line %d: %s is %q, want a literal block (|)", e.num, e.key, e.value)
	}

	if len(e.block) == 0 {
		return "", fmt.Errorf("line %d: %s is an empty literal block", e.num, e.key)
	}

	first, last := e.block[0], e.block[len(e.block)-1]

	var out []string

	// raw[e.num] is the line after the key's, line numbers counting from 1.
	for i := e.num; i < len(raw); i++ {
		text := strings.TrimLeft(raw[i], " ")
		indent := len(raw[i]) - len(text)

		if text == "" {
			out = append(out, "")

			continue
		}

		if i+1 < first.num {
			return "", fmt.Errorf("line %d: %s has a line before its first content line, which this reader does not follow", i+1, e.key)
		}

		if indent < first.indent {
			if i+1 <= last.num {
				return "", fmt.Errorf("line %d: a line of %s indented less than its first", i+1, e.key)
			}

			break
		}

		out = append(out, raw[i][first.indent:])
	}

	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}

	return strings.Join(out, "\n"), nil
}

// flowList reads a flow list written on one line, [a, b], whose entries are
// scalars.
func flowList(text string) ([]string, error) {
	if !strings.HasPrefix(text, "[") {
		return nil, fmt.Errorf("%q is not a list", text)
	}

	end := strings.IndexByte(text, ']')
	if end < 0 {
		return nil, fmt.Errorf("%q does not close its list on the same line", text)
	}

	if rest := strings.TrimSpace(text[end+1:]); rest != "" && !strings.HasPrefix(rest, "#") {
		return nil, fmt.Errorf("%q has text after its list", text)
	}

	inner := strings.TrimSpace(text[1:end])
	if inner == "" {
		return nil, nil
	}

	if strings.ContainsAny(inner, "[{}") {
		return nil, fmt.Errorf("%q nests a collection", text)
	}

	parts := strings.Split(inner, ",")
	values := make([]string, 0, len(parts))

	for _, part := range parts {
		value, err := scalar(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("an entry of %q: %w", text, err)
		}

		values = append(values, value)
	}

	return values, nil
}

// scalar reads one scalar: plain, or in single or double quotes without
// escapes, followed by nothing but a comment. It refuses an empty value, and a
// plain one that starts with a YAML indicator (an anchor, alias, tag, block
// scalar or flow collection) or holds a key.
func scalar(text string) (string, error) {
	if text == "" {
		return "", errors.New("an empty value")
	}

	if quote := text[0]; quote == '"' || quote == '\'' {
		end := strings.IndexByte(text[1:], quote)
		if end < 0 {
			return "", fmt.Errorf("%s does not close its quote", text)
		}

		value := text[1 : 1+end]
		if rest := strings.TrimSpace(text[2+end:]); rest != "" && !strings.HasPrefix(rest, "#") {
			return "", fmt.Errorf("%s has text after its closing quote", text)
		}

		if quote == '"' && strings.ContainsRune(value, '\\') {
			return "", fmt.Errorf("%s holds an escape, which this reader does not decode", text)
		}

		return value, nil
	}

	if strings.ContainsRune("&*!|>{}[]%@`#,?:", rune(text[0])) {
		return "", fmt.Errorf("%s starts with a YAML indicator this reader does not follow", text)
	}

	value := text
	if i := strings.Index(value, " #"); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}

	if strings.Contains(value, ": ") || strings.HasSuffix(value, ":") {
		return "", fmt.Errorf("%s is a key, want a scalar", text)
	}

	return value, nil
}

// keyValue splits a mapping line into its plain key and the value after it,
// with a value that is only a comment returned as empty.
func keyValue(text string) (key, value string, ok bool) {
	for i := range len(text) {
		if text[i] != ':' || (i+1 < len(text) && text[i+1] != ' ') {
			continue
		}

		key = text[:i]
		if !isPlainKey(key) {
			return "", "", false
		}

		value = strings.TrimSpace(text[i+1:])
		if strings.HasPrefix(value, "#") {
			value = ""
		}

		return key, value, true
	}

	return "", "", false
}

// isPlainKey reports whether key is a key this reader takes as written:
// lower-case ASCII letters, digits, _, . and -, starting with a letter, digit
// or _. An upper-case letter is refused: golangci-lint reads its configuration
// keys without regard to case, so List-Mode would be list-mode to it, and
// Domain-Is-Pure a repeat of domain-is-pure, where this reader compares keys
// as written.
func isPlainKey(key string) bool {
	if key == "" || key[0] == '.' || key[0] == '-' {
		return false
	}

	for _, r := range key {
		if ('A' <= r && r <= 'Z') || (!isASCIIAlnum(r) && r != '_' && r != '.' && r != '-') {
			return false
		}
	}

	return true
}

// isSequenceEntry reports whether text starts a block sequence entry.
func isSequenceEntry(text string) bool {
	return text == "-" || strings.HasPrefix(text, "- ")
}

// isASCIIAlnum reports whether r is an ASCII letter or digit.
func isASCIIAlnum(r rune) bool {
	return ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9')
}

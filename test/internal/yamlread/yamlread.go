package yamlread

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Line is one line of a YAML document that holds content: neither blank nor
// only a comment.
type Line struct {
	// Num is the line number, counted from 1.
	Num int
	// Indent is the number of spaces before text.
	Indent int
	// Text is the rest of the line, trailing white space removed.
	Text string
}

// Entry is one key of a block mapping.
type Entry struct {
	Key string
	// Num is the line number of the key.
	Num int
	// Value is the text after the key on its own line, empty when there is
	// none or only a comment.
	Value string
	// Block are the lines below the key that belong to it.
	Block []Line
}

// Item is one entry of a block sequence.
type Item struct {
	// Num is the line number of the entry's dash.
	Num int
	// Lines are the entry's value: the text after the dash, as a line indented
	// to the column it starts in, then the lines below that belong to it.
	Lines []Line
}

// SplitLines returns the content lines of a YAML document. Lines end in LF or
// CRLF. It refuses a character checkLine refuses, anywhere in the document,
// comments included, and a directive or document marker at the start of a
// line, which could start a second document.
func SplitLines(doc []byte) ([]Line, error) {
	var out []Line

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

		out = append(out, Line{Num: num, Indent: indent, Text: text})
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

// Mapping reads lines as one block mapping: every line at the indentation of
// the first is a key, and the lines after a key that are indented further, or
// that are sequence entries at its own indentation when the key has no value on
// its line, belong to it. It refuses a line indented less than the first, a
// line at the mapping's indentation that is not a plain key, and a key the
// mapping already has.
func Mapping(lines []Line) ([]Entry, error) {
	if len(lines) == 0 {
		return nil, nil
	}

	indent := lines[0].Indent

	var entries []Entry

	for _, l := range lines {
		if l.Indent < indent {
			return nil, fmt.Errorf("line %d: indented less than the mapping it is in", l.Num)
		}

		if len(entries) > 0 {
			last := &entries[len(entries)-1]
			if l.Indent > indent || (isSequenceEntry(l.Text) && last.Value == "") {
				last.Block = append(last.Block, l)

				continue
			}
		}

		key, value, ok := KeyValue(l.Text)
		if !ok {
			return nil, fmt.Errorf("line %d: %q is not a plain key this reader understands", l.Num, l.Text)
		}

		for _, e := range entries {
			if e.Key == key {
				return nil, fmt.Errorf("line %d: key %s is repeated from line %d", l.Num, key, e.Num)
			}
		}

		entries = append(entries, Entry{Key: key, Num: l.Num, Value: value})
	}

	return entries, nil
}

// Sequence reads lines as one block sequence: every line at the indentation of
// the first starts an entry with a dash, and the lines indented further belong
// to the entry above them. It refuses any other line.
func Sequence(lines []Line) ([]Item, error) {
	if len(lines) == 0 {
		return nil, nil
	}

	indent := lines[0].Indent

	var items []Item

	for _, l := range lines {
		switch {
		case l.Indent < indent:
			return nil, fmt.Errorf("line %d: indented less than the sequence it is in", l.Num)
		case l.Indent > indent:
			last := &items[len(items)-1]
			last.Lines = append(last.Lines, l)
		case !isSequenceEntry(l.Text):
			return nil, fmt.Errorf("line %d: %q is not a sequence entry", l.Num, l.Text)
		default:
			it := Item{Num: l.Num}
			if rest := strings.TrimLeft(l.Text[1:], " "); rest != "" {
				it.Lines = []Line{{Num: l.Num, Indent: l.Indent + len(l.Text) - len(rest), Text: rest}}
			}

			items = append(items, it)
		}
	}

	return items, nil
}

// Walk follows keys from the top-level mapping of lines, each key's value a
// block mapping below it, and returns the lines of the last key's value.
func Walk(lines []Line, keys ...string) ([]Line, error) {
	for i, key := range keys {
		path := strings.Join(keys[:i+1], ": ")

		entries, err := Mapping(lines)
		if err != nil {
			return nil, fmt.Errorf("read the mapping that holds %s: %w", path, err)
		}

		found := false

		for _, e := range entries {
			if e.Key != key {
				continue
			}

			lines, err = BlockValue(e, path)
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

// BlockValue returns the lines of e's value, a block below the key at path. It
// refuses a value on the key's own line and an empty one.
func BlockValue(e Entry, path string) ([]Line, error) {
	if e.Value != "" {
		return nil, fmt.Errorf("line %d: %s has %q on its own line, want a block below it", e.Num, path, e.Value)
	}

	if len(e.Block) == 0 {
		return nil, fmt.Errorf("line %d: %s is empty", e.Num, path)
	}

	return e.Block, nil
}

// List reads the value of e as a list of scalars: a flow list on the key's own
// line ([a, b]) or a block sequence below it (- a). A key with no value is an
// empty list.
func List(e Entry) ([]string, error) {
	if e.Value != "" {
		if len(e.Block) != 0 {
			return nil, fmt.Errorf("line %d: %s has a value on its own line and lines below it", e.Num, e.Key)
		}

		values, err := flowList(e.Value)
		if err != nil {
			return nil, fmt.Errorf("line %d: %s: %w", e.Num, e.Key, err)
		}

		return values, nil
	}

	items, err := Sequence(e.Block)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", e.Key, err)
	}

	values := make([]string, 0, len(items))

	for _, it := range items {
		if len(it.Lines) != 1 {
			return nil, fmt.Errorf("line %d: an entry of %s that is not one scalar on its line", it.Num, e.Key)
		}

		value, err := Scalar(it.Lines[0].Text)
		if err != nil {
			return nil, fmt.Errorf("line %d: an entry of %s: %w", it.Num, e.Key, err)
		}

		values = append(values, value)
	}

	return values, nil
}

// Single reads the value of e as one scalar on the key's own line.
func Single(e Entry) (string, error) {
	if len(e.Block) != 0 {
		return "", fmt.Errorf("line %d: %s has lines below it, want one value on its line", e.Num, e.Key)
	}

	value, err := Scalar(e.Value)
	if err != nil {
		return "", fmt.Errorf("line %d: %s: %w", e.Num, e.Key, err)
	}

	return value, nil
}

// LiteralBlock reads the value of e, a literal block scalar (|) with clip
// chomping and its indentation taken from its first line, from raw, the lines
// of the document e is in with their line ends removed. It returns the block's
// lines, that indentation removed, joined by newlines, without the newline
// after the last one. It reads raw rather than e.Block because a block's lines
// are content, not comments: a line in it that starts with # is part of the
// value, and so is a blank line between two others.
//
// It refuses a block whose first line is not its first content line, and a
// content line indented less than the first: YAML ends the block there, where
// the mapping reader would keep the line in it.
func LiteralBlock(raw []string, e Entry) (string, error) {
	return readLiteralBlock(raw, e, false)
}

// LiteralBlockLeadingHashes reads e as LiteralBlock does, but accepts lines
// starting with # before the block's first content line when they sit at
// exactly its indentation: YAML reads them as the block's first lines, and so
// does this reader, keeping them in the value. Any other line there is still
// refused.
func LiteralBlockLeadingHashes(raw []string, e Entry) (string, error) {
	return readLiteralBlock(raw, e, true)
}

// readLiteralBlock is LiteralBlock, with lines starting with # at the block's
// indentation accepted before its first content line when leadingHashes.
func readLiteralBlock(raw []string, e Entry, leadingHashes bool) (string, error) {
	if e.Value != "|" {
		return "", fmt.Errorf("line %d: %s is %q, want a literal block (|)", e.Num, e.Key, e.Value)
	}

	if len(e.Block) == 0 {
		return "", fmt.Errorf("line %d: %s is an empty literal block", e.Num, e.Key)
	}

	first, last := e.Block[0], e.Block[len(e.Block)-1]

	var out []string

	// raw[e.num] is the line after the key's, line numbers counting from 1.
	for i := e.Num; i < len(raw); i++ {
		text := strings.TrimLeft(raw[i], " ")
		indent := len(raw[i]) - len(text)

		if text == "" {
			out = append(out, "")

			continue
		}

		if i+1 < first.Num && (!leadingHashes || indent != first.Indent || !strings.HasPrefix(text, "#")) {
			return "", fmt.Errorf("line %d: %s has a line before its first content line, which this reader does not follow", i+1, e.Key)
		}

		if indent < first.Indent {
			if i+1 <= last.Num {
				return "", fmt.Errorf("line %d: a line of %s indented less than its first", i+1, e.Key)
			}

			break
		}

		out = append(out, raw[i][first.Indent:])
	}

	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}

	return strings.Join(out, "\n"), nil
}

// flowList reads a flow list written on one line, [a, b], whose entries are
// scalars. It refuses a # before the list's closing bracket: a comment runs to
// the end of the line, so a bracket after it would not close the list, and a #
// inside an entry is refused with it rather than told apart from one.
func flowList(text string) ([]string, error) {
	if !strings.HasPrefix(text, "[") {
		return nil, fmt.Errorf("%q is not a list", text)
	}

	end := strings.IndexByte(text, ']')
	if end < 0 {
		return nil, fmt.Errorf("%q does not close its list on the same line", text)
	}

	if strings.ContainsRune(text[:end], '#') {
		return nil, fmt.Errorf("%q has a # inside its list, which this reader does not follow", text)
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
		value, err := Scalar(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("an entry of %q: %w", text, err)
		}

		values = append(values, value)
	}

	return values, nil
}

// Scalar reads one scalar: plain, or in single or double quotes without
// escapes, followed by nothing but a comment. It refuses an empty value, and a
// plain one that starts with a YAML indicator (an anchor, alias, tag, block
// scalar or flow collection) or holds a key.
func Scalar(text string) (string, error) {
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

// KeyValue splits a mapping line into its plain key and the value after it,
// with a value that is only a comment returned as empty.
func KeyValue(text string) (key, value string, ok bool) {
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

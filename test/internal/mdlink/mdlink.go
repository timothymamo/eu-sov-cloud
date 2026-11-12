package mdlink

import (
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// repositoryURL is this repository on GitHub. A link to a file or directory
// in it by URL (blob/ or tree/) breaks when the pages move to a site, so it
// is a problem: a relative link belongs there (docs/README.md).
const repositoryURL = "https://github.com/timothymamo/shift2eu/"

// Problem is one link that does not resolve. File is the Markdown file it is
// in, Line its 1-based line, Link the destination as written.
type Problem struct {
	File   string
	Line   int
	Link   string
	Reason string
}

// Heading is one ATX heading outside fenced code: its 1-based line, its level
// (the number of #) and its text without the closing #s.
type Heading struct {
	Line  int
	Level int
	Text  string
}

var (
	// atxHeading is a heading line: up to three spaces, one to six #, then
	// white space or the end of the line.
	atxHeading = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?[ \t]*$`)
	// closingHashes is an ATX heading's optional closing sequence.
	closingHashes = regexp.MustCompile(`(?:^|[ \t]+)#+$`)
	// scheme starts an absolute URL (https:, mailto:, ...).
	scheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	// inlineLink is a link or image inside heading text, kept as its text.
	inlineLink = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
)

// Check reads each of files from fsys and returns a Problem for every inline
// link or image whose destination is relative and does not resolve: a file or
// directory that does not exist, a #anchor that is not a heading of the
// Markdown file it names (GitHub's anchors: see anchors), a #anchor into a file
// that is not Markdown, a path that leaves fsys or is absolute, and a link to
// this repository's files by URL. Other URLs are not checked, and links inside
// fenced code or code spans are not links. The problems come in the order of
// files, then of lines. Check fails only when it cannot read one of files.
func Check(fsys fs.FS, files []string) ([]Problem, error) {
	anchorCache := map[string]map[string]bool{}
	anchorsOf := func(name string) (map[string]bool, error) {
		if a, ok := anchorCache[name]; ok {
			return a, nil
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		a := anchors(Headings(data))
		anchorCache[name] = a
		return a, nil
	}

	var problems []Problem
	for _, file := range files {
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", file, err)
		}
		for _, l := range Links(data) {
			reason, err := resolve(fsys, file, l.Dest, anchorsOf)
			if err != nil {
				return nil, err
			}
			if reason != "" {
				problems = append(problems, Problem{File: file, Line: l.Line, Link: l.Dest, Reason: reason})
			}
		}
	}
	return problems, nil
}

// resolve returns why dest, a link in file, does not resolve, or "" when it
// does or is not checked.
func resolve(fsys fs.FS, file, dest string, anchorsOf func(string) (map[string]bool, error)) (string, error) {
	if scheme.MatchString(dest) {
		if strings.HasPrefix(dest, repositoryURL+"blob/") || strings.HasPrefix(dest, repositoryURL+"tree/") {
			return "the link names the repository by URL; use a relative path", nil
		}
		return "", nil
	}
	if strings.HasPrefix(dest, "/") {
		return "the link is absolute; use a path relative to " + file, nil
	}

	target, fragment, _ := strings.Cut(dest, "#")
	if unescaped, err := url.PathUnescape(target); err == nil {
		target = unescaped
	}

	name := file
	if target != "" {
		name = path.Join(path.Dir(file), target)
		if name == ".." || strings.HasPrefix(name, "../") {
			return "the link leaves the repository", nil
		}
		if _, err := fs.Stat(fsys, name); err != nil {
			return name + " does not exist", nil
		}
	}
	if fragment == "" {
		return "", nil
	}
	if path.Ext(name) != ".md" {
		return name + " is not Markdown, so #" + fragment + " names no heading", nil
	}
	a, err := anchorsOf(name)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", name, err)
	}
	if !a[fragment] {
		return name + " has no heading with the anchor #" + fragment, nil
	}
	return "", nil
}

// Headings returns the ATX headings of a Markdown document, outside fenced
// code, in order. Setext headings (text underlined with = or -) are not read:
// the pages use # headings only.
func Headings(data []byte) []Heading {
	var headings []Heading
	var fence fenceState
	for i, line := range strings.Split(string(data), "\n") {
		if fence.step(line) {
			continue
		}
		m := atxHeading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text := closingHashes.ReplaceAllString(m[2], "")
		if m[2] != "" && strings.Trim(m[2], "#") == "" {
			text = ""
		}
		headings = append(headings, Heading{Line: i + 1, Level: len(m[1]), Text: strings.TrimSpace(text)})
	}
	return headings
}

// anchors returns the anchors GitHub gives headings: the text with links
// reduced to their text, lower case, with every character but letters,
// digits, marks, spaces, hyphens and underscores removed (so * and ` go) and
// each space made a hyphen; a repeated anchor gets -1, -2, ... in order.
// Underscores stay, as GitHub keeps them in snake_case words; a heading with
// _emphasis_ would differ from GitHub's anchor, and the pages use none.
func anchors(headings []Heading) map[string]bool {
	seen := map[string]int{}
	out := map[string]bool{}
	for _, h := range headings {
		text := inlineLink.ReplaceAllString(h.Text, "$1")
		var b strings.Builder
		for _, r := range strings.ToLower(text) {
			switch {
			case r == ' ':
				b.WriteRune('-')
			case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r):
				b.WriteRune(r)
			}
		}
		slug := b.String()
		if n, ok := seen[slug]; ok {
			seen[slug] = n + 1
			slug += "-" + strconv.Itoa(n+1)
		} else {
			seen[slug] = 0
		}
		out[slug] = true
	}
	return out
}

// Link is one inline link or image: its 1-based line and its destination,
// without angle brackets or a title.
type Link struct {
	Line int
	Dest string
}

// Links returns the inline links and images of a Markdown document, outside
// fenced code and code spans, in order. Reference links ([text][label]) are
// not read: the pages use inline links only.
func Links(data []byte) []Link {
	var out []Link
	var fence fenceState
	for i, line := range strings.Split(string(data), "\n") {
		if fence.step(line) {
			continue
		}
		line = stripCodeSpans(line)
		for rest := line; ; {
			at := strings.Index(rest, "](")
			if at < 0 {
				break
			}
			rest = rest[at+2:]
			dest, n := destination(rest)
			rest = rest[n:]
			if dest != "" {
				out = append(out, Link{Line: i + 1, Dest: dest})
			}
		}
	}
	return out
}

// destination reads a link destination from the start of s, which follows
// "](", and returns it without angle brackets or a title, and how much of s it
// took.
func destination(s string) (string, int) {
	if strings.HasPrefix(s, "<") {
		end := strings.Index(s, ">")
		if end < 0 {
			return "", 0
		}
		return s[1:end], end + 1
	}
	depth := 0
	for i, r := range s {
		switch {
		case r == '(':
			depth++
		case r == ')' && depth == 0:
			return s[:i], i
		case r == ')':
			depth--
		case r == ' ' || r == '\t':
			return s[:i], i
		}
	}
	return s, len(s)
}

// stripCodeSpans blanks the code spans of a line: a run of backticks up to
// the next run of the same length. A run with no match stays as it is.
func stripCodeSpans(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		if line[i] != '`' {
			b.WriteByte(line[i])
			i++
			continue
		}
		n := 0
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		end := closingRun(line[i+n:], n)
		if end < 0 {
			b.WriteString(line[i : i+n])
			i += n
			continue
		}
		i += n + end + n
	}
	return b.String()
}

// closingRun returns where in s a run of exactly n backticks starts, or -1.
func closingRun(s string, n int) int {
	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		m := 0
		for i+m < len(s) && s[i+m] == '`' {
			m++
		}
		if m == n {
			return i
		}
		i += m
	}
	return -1
}

// fenceState follows fenced code blocks line by line. Fences may be indented
// any amount, as they are inside list items.
type fenceState struct {
	char byte // '`' or '~' inside a fence, 0 outside
	size int  // the opening fence's length
}

// step reads one line and reports whether it is part of a fenced code block,
// fences included.
func (f *fenceState) step(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	n := 0
	for n < len(trimmed) && (trimmed[n] == '`' || trimmed[n] == '~') && trimmed[n] == trimmed[0] {
		n++
	}
	if f.char == 0 {
		// A backtick fence's info string may not hold a backtick: such a line
		// is a code span, not a fence.
		if n >= 3 && (trimmed[0] != '`' || !strings.Contains(trimmed[n:], "`")) {
			f.char, f.size = trimmed[0], n
			return true
		}
		return false
	}
	if n >= f.size && trimmed[0] == f.char && strings.TrimSpace(trimmed[n:]) == "" {
		f.char, f.size = 0, 0
	}
	return true
}

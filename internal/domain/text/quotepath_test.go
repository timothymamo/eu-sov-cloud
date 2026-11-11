package text_test

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// The shape of a path longer than the 128-rune limit: the first 32 runes and the
// last 96 runes, each quoted, then the original byte length. The head keeps the
// start of the path and the tail keeps the base name, which is what says which
// file failed. 32 + 96 is the 128 runes text.Quote keeps, so the limit itself
// does not change.
const (
	quotePathHead = 32
	quotePathTail = 96
)

// runeParts splits s the way text.Quote counts runes: each invalid byte is one
// part and keeps the byte it is, so a kept piece quotes to the same escapes as
// the input and never to U+FFFD.
func runeParts(s string) []string {
	parts := make([]string, 0, len(s))

	for i := 0; i < len(s); {
		_, size := utf8.DecodeRuneInString(s[i:])
		parts = append(parts, s[i:i+size])
		i += size
	}

	return parts
}

// quotedPathOf returns what QuotePath must return for s, computed from the runes
// of s rather than from the function under test: at most 128 runes are quoted
// whole, a longer value keeps its first 32 and last 96 runes with the dropped
// middle marked, and the original byte length follows.
func quotedPathOf(s string) string {
	parts := runeParts(s)

	if len(parts) <= quotePathHead+quotePathTail {
		return strconv.Quote(s)
	}

	return strconv.Quote(strings.Join(parts[:quotePathHead], "")) + "..." +
		strconv.Quote(strings.Join(parts[len(parts)-quotePathTail:], "")) +
		"(" + strconv.Itoa(len(s)) + " bytes)"
}

// A path of at most 128 runes is short enough to keep whole, so QuotePath is
// exactly Quote: the escaping and the double quotes are what makes a path safe to
// print.
func TestQuotePathMatchesQuoteForShortInput(t *testing.T) {
	t.Parallel()

	// 2 + 56*2 + 14 = 128 runes: the longest path Quote does not truncate.
	exactly128Runes := "/a" + strings.Repeat("a/", 56) + "inventory.json"
	if got := utf8.RuneCountInString(exactly128Runes); got != 128 {
		t.Fatalf("the test path is %d runes, want 128", got)
	}

	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "short path", in: "/srv/inventories/inventory.json"},
		{name: "relative path", in: "testdata/valid.json"},
		{name: "exactly 128 runes", in: exactly128Runes},
		{name: "newline is escaped", in: "/srv/line\nbreak/inventory.json"},
		{name: "escape is escaped", in: "/srv/ansi\x1bseq/inventory.json"},
		{name: "invalid utf8 is escaped", in: "/srv/caf\xe9/inventory.json"},
		{name: "multi byte runes", in: "/srv/inventaires-café/inventory.json"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want := text.Quote(tc.in)
			if diff := cmp.Diff(want, text.QuotePath(tc.in)); diff != "" {
				t.Errorf("QuotePath(%q) mismatch (-want +got):\n%s", tc.in, diff)
			}
		})
	}
}

// A path longer than 128 runes keeps both ends instead of only its start, so the
// base name survives: the result is the quoted first 32 runes, an ellipsis, the
// quoted last 96 runes and the original byte length. Runes are counted as
// text.Quote counts them, so a multi-byte character is one rune and each invalid
// byte is one rune of its own.
func TestQuotePathKeepsBothEndsOfLongInput(t *testing.T) {
	t.Parallel()

	deepPath := strings.Repeat("d/", 100) + "inventory.json"
	if got := utf8.RuneCountInString(deepPath); got != 214 {
		t.Fatalf("the deep path is %d runes, want 214", got)
	}

	// 32 + 1 + 96 = 129 runes: the shortest path that is truncated, and exactly
	// the middle rune is the one dropped.
	middleDropped := strings.Repeat("h", quotePathHead) + "M" + strings.Repeat("t", quotePathTail)

	// 157 runes in 317 bytes: the cut counts runes, not bytes, so the original
	// byte length is the one the result reports.
	multiByte := "/" + strings.Repeat("äöü", 40) + "/" + strings.Repeat("€", 20) + "/inventory.json"

	// 129 runes in 129 bytes: an invalid byte is one rune, 10 in the head and 5
	// in the tail, so the rune count and the byte count coincide.
	invalidBytes := strings.Repeat("\xff", 10) + strings.Repeat("h", 100) + strings.Repeat("\xff", 5) + "inventory.json"

	tests := []struct {
		name string
		in   string
		// wantPrefix and wantSuffix are checkable ends of the result, and
		// wantContains the escapes a kept piece must hold. wantAbsent pins what
		// the shape drops or escapes.
		wantPrefix   string
		wantSuffix   string
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:         "deep path keeps its base name",
			in:           deepPath,
			wantPrefix:   strconv.Quote(string([]rune(deepPath)[:quotePathHead])),
			wantSuffix:   `inventory.json"(214 bytes)`,
			wantContains: []string{`..."`},
		},
		{
			name:       "129 runes drops only the middle rune",
			in:         middleDropped,
			wantSuffix: `"(129 bytes)`,
			wantAbsent: []string{"M"},
		},
		{
			// The cut counts runes, not bytes: 157 runes of two and three bytes
			// each, and the reported length is the original byte length.
			name:       "multi byte runes count as one rune each",
			in:         multiByte,
			wantSuffix: `inventory.json"(` + strconv.Itoa(len(multiByte)) + " bytes)",
		},
		{
			// An invalid byte is one rune, in the head and in the tail alike, and
			// its quote is the escape of the byte, never U+FFFD.
			name:         "invalid bytes count as one rune each",
			in:           invalidBytes,
			wantSuffix:   `inventory.json"(` + strconv.Itoa(len(invalidBytes)) + " bytes)",
			wantContains: []string{`\xff`},
		},
		{
			// The kept tail is quoted, so a control character in the base name of
			// a directory near the end of the path is escaped, not printed.
			name:         "control character in the kept tail is escaped",
			in:           strings.Repeat("h", 40) + "/dir/" + strings.Repeat("t", 100) + "/line\nbreak/inventory.json",
			wantContains: []string{`\n`},
			wantAbsent:   []string{"\n"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want := quotedPathOf(tc.in)
			got := text.QuotePath(tc.in)

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("QuotePath(%q) mismatch (-want +got):\n%s", tc.in, diff)
			}

			if tc.wantPrefix != "" && !strings.HasPrefix(got, tc.wantPrefix) {
				t.Errorf("QuotePath(%d bytes) = %q, want it to start with %q", len(tc.in), got, tc.wantPrefix)
			}

			if tc.wantSuffix != "" && !strings.HasSuffix(got, tc.wantSuffix) {
				t.Errorf("QuotePath(%d bytes) = %q, want it to end with %q", len(tc.in), got, tc.wantSuffix)
			}

			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("QuotePath(%d bytes) = %q, want it to contain %q", len(tc.in), got, want)
				}
			}

			for _, unwanted := range tc.wantAbsent {
				if strings.Contains(got, unwanted) {
					t.Errorf("QuotePath(%d bytes) = %q, want it without %q", len(tc.in), got, unwanted)
				}
			}

			if !utf8.ValidString(got) {
				t.Errorf("QuotePath(%d bytes) = %q, want valid UTF-8", len(tc.in), got)
			}
		})
	}
}

// A long path is a path too, so its quoted form is safe to print: no raw control
// character and no invalid byte survives, whatever the path holds.
func TestQuotePathEmitsNoRawControlOrInvalidBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "escape sequences", in: strings.Repeat("/a\x1b[2J", 100)},
		{name: "invalid utf8", in: strings.Repeat("/\xff\xfe", 300)},
		{name: "mixed", in: strings.Repeat("/a\x1b\xffb\u202e", 100)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := text.QuotePath(tc.in)

			if !utf8.ValidString(got) {
				t.Errorf("QuotePath(%d bytes) = %q, want valid UTF-8", len(tc.in), got)
			}

			if strings.ContainsAny(got, "\x00\n\r\x1b") {
				t.Errorf("QuotePath(%d bytes) = %q, want no raw control character", len(tc.in), got)
			}

			if strings.Contains(got, "\xff") {
				t.Errorf("QuotePath(%d bytes) = %q, want no raw invalid byte", len(tc.in), got)
			}

			if strings.ContainsRune(got, '\u202e') {
				t.Errorf("QuotePath(%d bytes) = %q, want no raw right-to-left override", len(tc.in), got)
			}

			if len(got) >= len(tc.in) {
				t.Errorf("len(QuotePath(%d bytes)) = %d, want it truncated", len(tc.in), len(got))
			}
		})
	}
}

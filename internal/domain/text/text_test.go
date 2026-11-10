package text_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// MaxLen is pinned literally: it is the limit every free-text field of the
// domain obeys, so inventory files, catalogs and reports all depend on it.
func TestMaxLen(t *testing.T) {
	t.Parallel()

	if text.MaxLen != 2048 {
		t.Errorf("MaxLen = %d, want 2048 bytes", text.MaxLen)
	}
}

// Ordinary identifiers, types, regions, names, ARNs and non-ASCII text are all
// valid. Emptiness is the caller's decision, so the empty string is valid too.
func TestValidateAccepts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "instance id", in: "i-0123456789abcdef0"},
		{name: "raw type", in: "AWS::EC2::Instance"},
		{name: "region", in: "eu-west-1"},
		{name: "offering name", in: "Managed Database for PostgreSQL"},
		{name: "arn", in: "arn:aws:rds:eu-west-1:123456789012:db:db-example"},
		{name: "non ascii", in: "Zürich"},
		// A precomposed accent is one printable rune, and a combining mark after a
		// letter is legitimate text: neither is a look-alike blank.
		{name: "precomposed accent", in: "caf\u00e9"},
		{name: "combining acute after a letter", in: "e\u0301"},
		// A private-use code point is not a control or format character: the source
		// named it, so it is kept and left to the renderer to escape.
		{name: "private use code point", in: "a\ue000b"},
		// A pipe is printable; escaping it for a Markdown table is the reporter's job.
		{name: "pipe", in: "primary|replica"},
		{name: "exactly max len bytes", in: strings.Repeat("a", text.MaxLen)},
		{name: "exactly max len bytes multibyte", in: strings.Repeat("é", text.MaxLen/2)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := text.Validate(tc.in); err != nil {
				t.Errorf("Validate(%q) = %v, want nil", tc.in, err)
			}
		})
	}
}

// Control characters, bidi overrides and invalid UTF-8 would corrupt a terminal
// or a report, so they are rejected instead of rendered.
func TestValidateRejectsInvalidText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "tab", in: "a\tb"},
		{name: "newline", in: "row\nvalue"},
		{name: "carriage return", in: "a\rb"},
		{name: "nul", in: "a\x00b"},
		{name: "escape sequence", in: "\x1b[2J"},
		{name: "delete", in: "a\x7fb"},
		{name: "c1 control low", in: "a\u0080b"},
		{name: "c1 control high", in: "a\u009fb"},
		{name: "right to left override", in: "a\u202eb"},
		{name: "left to right embedding", in: "a\u202ab"},
		{name: "left to right isolate", in: "a\u2066b"},
		{name: "pop directional isolate", in: "a\u2069b"},
		{name: "zero width space", in: "a\u200bb"},
		{name: "zero width joiner", in: "a\u200db"},
		{name: "byte order mark", in: "a\ufeffb"},
		// Some renderers treat Zl and Zp as line breaks, the same risk as "\n".
		{name: "line separator", in: "a\u2028b"},
		{name: "paragraph separator", in: "a\u2029b"},
		// Invisible characters that render as nothing or as a blank: in a report
		// they hide the difference between two values.
		{name: "combining grapheme joiner", in: "a\u034fb"},
		{name: "variation selector", in: "a\ufe0fb"},
		{name: "variation selector supplement", in: "a\U000e0100b"},
		{name: "hangul filler", in: "\u3164"},
		{name: "hangul choseong filler", in: "a\u115fb"},
		{name: "halfwidth hangul filler", in: "a\uffa0b"},
		{name: "khmer inherent vowel", in: "a\u17b4b"},
		{name: "braille pattern blank", in: "\u2800"},
		// Noncharacters are permanently unassigned and never text.
		{name: "noncharacter fffe", in: "a\ufffeb"},
		{name: "noncharacter 10ffff", in: "a\U0010ffffb"},
		{name: "invalid utf8", in: "a\xffb"},
		{name: "truncated utf8", in: "Zürich"[0:2]},
		{name: "one byte too long", in: strings.Repeat("a", text.MaxLen+1)},
		{name: "one rune too long multibyte", in: strings.Repeat("é", text.MaxLen/2+1)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := text.Validate(tc.in)
			if !errors.Is(err, text.ErrInvalid) {
				t.Fatalf("Validate(%q) error = %v, want an error wrapping text.ErrInvalid", tc.in, err)
			}
		})
	}
}

// Short strings are exactly strconv.Quote: the double quotes and the escaping
// are what makes a value safe to put in a report.
func TestQuoteMatchesStrconvForShortInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "instance id", in: "i-0123456789abcdef0"},
		{name: "raw type", in: "AWS::EC2::Instance"},
		{name: "offering name", in: "Managed Database for PostgreSQL"},
		{name: "non ascii", in: "Zürich"},
		{name: "newline is escaped", in: "row\nvalue"},
		{name: "escape is escaped", in: "\x1b[2J"},
		{name: "invalid utf8 is escaped", in: "a\xffb"},
		{name: "quote is escaped", in: `a"b`},
		{name: "backslash is escaped", in: `a\b`},
		// Exactly 128 runes fit; only longer input is truncated.
		{name: "128 runes is not truncated", in: strings.Repeat("a", 128)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want := strconv.Quote(tc.in)
			if diff := cmp.Diff(want, text.Quote(tc.in)); diff != "" {
				t.Errorf("Quote(%q) mismatch (-want +got):\n%s", tc.in, diff)
			}
		})
	}
}

// A value from a file can be megabytes long, so Quote truncates the value and
// reports the original byte length instead of echoing all of it.
func TestQuoteTruncatesLongInput(t *testing.T) {
	t.Parallel()

	const oneMiB = 1 << 20

	tests := []struct {
		name        string
		in          string
		wantLen     string
		wantRepeats string
	}{
		{name: "one mib ascii", in: strings.Repeat("a", oneMiB), wantLen: "1048576", wantRepeats: "a"},
		{name: "one mib multibyte", in: strings.Repeat("é", oneMiB/2), wantLen: "1048576", wantRepeats: "é"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := text.Quote(tc.in)

			if len(got) >= 1024 {
				t.Errorf("len(Quote(%d bytes)) = %d, want less than 1024", len(tc.in), len(got))
			}

			if !strings.Contains(got, tc.wantLen) {
				t.Errorf("Quote(%d bytes) = %q, want it to contain the original byte length %q", len(tc.in), got, tc.wantLen)
			}

			if !utf8.ValidString(got) {
				t.Errorf("Quote(%d bytes) = %q, want valid UTF-8", len(tc.in), got)
			}

			if !strings.HasPrefix(got, `"`) {
				t.Errorf("Quote(%d bytes) = %q, want the quoted form to start with a double quote", len(tc.in), got)
			}

			if n := strings.Count(got, tc.wantRepeats); n > 128 {
				t.Errorf("Quote(%d bytes) kept %d runes, want at most 128", len(tc.in), n)
			}
		})
	}
}

// Truncation has an exact shape: the first 128 runes, quoted, then the original
// byte length. Report golden files and the maximum error size depend on it, and an
// invalid byte counts as one rune, so it is never split by the cut.
func TestQuoteTruncatesTo128Runes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "129 runes",
			in:   strings.Repeat("a", 129),
			want: strconv.Quote(strings.Repeat("a", 128)) + "...(129 bytes)",
		},
		{
			// The invalid byte is the 129th rune, so it falls outside the kept
			// prefix but still counts as the one byte it is in the reported length:
			// 128 + 1 + 1 bytes.
			name: "invalid byte at the boundary",
			in:   strings.Repeat("a", 128) + "\xffb",
			want: strconv.Quote(strings.Repeat("a", 128)) + "...(130 bytes)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := text.Quote(tc.in); got != tc.want {
				t.Errorf("Quote(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Truncation is a summary, so a truncated value still shows a prefix of it.
func TestQuoteKeepsPrefixOfLongInput(t *testing.T) {
	t.Parallel()

	got := text.Quote(strings.Repeat("a", 200))

	if n := strings.Count(got, "a"); n == 0 || n > 128 {
		t.Errorf("Quote(200 runes) kept %d runes, want between 1 and 128", n)
	}
}

// The whole point of Quote is that its result is safe to print: no raw control
// character and no invalid byte survives, even from hostile input.
func TestQuoteEmitsNoRawControlOrInvalidBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "escape sequences", in: strings.Repeat("a\x1b[2J", 300)},
		{name: "invalid utf8", in: strings.Repeat("\xff\xfe", 300)},
		{name: "mixed", in: strings.Repeat("a\x1b\xffb\u202e", 300)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := text.Quote(tc.in)

			if !utf8.ValidString(got) {
				t.Errorf("Quote(%d bytes) = %q, want valid UTF-8", len(tc.in), got)
			}

			if strings.ContainsRune(got, '\x1b') {
				t.Errorf("Quote(%d bytes) = %q, want no raw escape character", len(tc.in), got)
			}

			if strings.ContainsRune(got, '\u202e') {
				t.Errorf("Quote(%d bytes) = %q, want no raw right-to-left override", len(tc.in), got)
			}

			if strings.Contains(got, "\xff") {
				t.Errorf("Quote(%d bytes) = %q, want no raw invalid byte", len(tc.in), got)
			}

			if len(got) >= len(tc.in) {
				t.Errorf("len(Quote(%d bytes)) = %d, want it truncated", len(tc.in), len(got))
			}
		})
	}
}

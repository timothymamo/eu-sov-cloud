package cli

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxErrorBytes is the longest error message, after escaping, that printError
// writes. A longer one is cut and ends in a truncation marker.
const maxErrorBytes = 1024

// sanitise returns msg as text that is safe to print on one line of a terminal
// or a log. It escapes, as strconv does inside a quoted string, every character
// a terminal or a log reader would act on rather than show:
//
//   - control characters (C0, DEL and C1), such as the line feed that would
//     start a forged line and the escape that starts a terminal sequence;
//   - format characters (category Cf), such as the bidi controls that reorder
//     what follows them;
//   - line and paragraph separators (categories Zl and Zp);
//   - each byte that is not valid UTF-8, as \xNN.
//
// So a line feed becomes \n, an escape \x1b and U+202E \u202e. Everything else,
// the backslash included, is kept as it is: the messages already hold values
// quoted by text.Quote, whose escapes must not be doubled, so the result is safe
// to show but not meant to be unquoted.
//
// A result longer than maxErrorBytes is cut before the first character that
// would cross the limit, never inside an escape or a UTF-8 sequence, and
// followed by a marker that gives the length of msg.
func sanitise(msg string) string {
	var b strings.Builder

	for i := 0; i < len(msg); {
		r, size := utf8.DecodeRuneInString(msg[i:])
		piece := msg[i : i+size]

		switch {
		case r == utf8.RuneError && size == 1:
			piece = fmt.Sprintf(`\x%02x`, msg[i])
		case escaped(r):
			quoted := strconv.QuoteRune(r)
			piece = quoted[1 : len(quoted)-1]
		}

		if b.Len()+len(piece) > maxErrorBytes {
			fmt.Fprintf(&b, "...(truncated, %d bytes)", len(msg))

			return b.String()
		}

		b.WriteString(piece)

		i += size
	}

	return b.String()
}

// escaped reports whether sanitise escapes r.
func escaped(r rune) bool {
	return unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)
}

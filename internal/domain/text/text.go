package text

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxLen is the longest valid free-text value, in bytes.
const MaxLen = 2048

// quoteRunes is how many runes of a value Quote keeps before truncating it.
const quoteRunes = 128

// ErrInvalid is returned for free text that is not valid.
var ErrInvalid = errors.New("invalid text")

// brailleBlank is U+2800 BRAILLE PATTERN BLANK: a printable symbol (category So)
// that renders as an empty cell, so it can pass for a space.
const brailleBlank = '\u2800'

// Validate reports whether s is valid free text: at most MaxLen bytes of valid
// UTF-8 with none of these characters:
//
//   - control characters (C0, DEL or C1, including tab and newline);
//   - format characters (Unicode category Cf: bidi controls, zero-width
//     characters, the byte order mark);
//   - line and paragraph separators (categories Zl and Zp: U+2028, U+2029),
//     which some renderers treat as line breaks;
//   - characters that render as nothing or as a blank and so make two different
//     values look the same: variation selectors (Variation_Selector), the other
//     default-ignorable code points (Other_Default_Ignorable_Code_Point, such as
//     the combining grapheme joiner, the Hangul fillers and the Khmer inherent
//     vowels) and U+2800 BRAILLE PATTERN BLANK;
//   - noncharacters (Noncharacter_Code_Point, such as U+FFFE and U+10FFFF),
//     which are never text.
//
// s must also not start or end with white space (unicode.IsSpace, what
// strings.TrimSpace removes): padding makes a value look the same as the one it
// pads, so a value that is only white space is invalid too. White space inside
// a value is part of it and valid, non-ASCII space separators such as U+00A0
// included. Validate never trims: values are stored as given or rejected.
//
// Private-use code points and combining marks such as the accent in "e\u0301"
// are valid. The empty string is valid; whether a value may be empty is the
// caller's decision.
//
// The error wraps ErrInvalid and says what is wrong and where, without echoing s.
func Validate(s string) error {
	if len(s) > MaxLen {
		return fmt.Errorf("%w: %d bytes, longer than %d", ErrInvalid, len(s), MaxLen)
	}

	for i, r := range s {
		if r == utf8.RuneError {
			if _, size := utf8.DecodeRuneInString(s[i:]); size == 1 {
				return fmt.Errorf("%w: invalid UTF-8 at byte %d", ErrInvalid, i)
			}
		}

		if disallowed(r) {
			return fmt.Errorf("%w: disallowed character %U at byte %d", ErrInvalid, r, i)
		}
	}

	if s != strings.TrimSpace(s) {
		return fmt.Errorf("%w: leading or trailing white space", ErrInvalid)
	}

	return nil
}

// disallowed reports whether r is a character Validate rejects.
func disallowed(r rune) bool {
	return r == brailleBlank || unicode.IsControl(r) || unicode.In(r,
		unicode.Cf, unicode.Zl, unicode.Zp,
		unicode.Variation_Selector,
		unicode.Other_Default_Ignorable_Code_Point,
		unicode.Noncharacter_Code_Point,
	)
}

// Quote returns s as a Go double-quoted string literal (strconv.Quote), safe to
// print whatever s holds. A value longer than 128 runes is truncated to its first
// 128 runes, and the quoted prefix is followed by its original length, as in
// "aaaa"...(8388608 bytes), so the result stays short however long s is: at
// most 128 escaped runes of up to 10 bytes each, the quotes and the suffix. Each
// invalid byte of s counts as one rune.
func Quote(s string) string {
	n := 0
	for i := range s {
		if n == quoteRunes {
			return strconv.Quote(s[:i]) + "...(" + strconv.Itoa(len(s)) + " bytes)"
		}

		n++
	}

	return strconv.Quote(s)
}

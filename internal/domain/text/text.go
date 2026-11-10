package text

import (
	"errors"
	"fmt"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// MaxLen is the longest valid free-text value, in bytes.
const MaxLen = 2048

// quoteRunes is how many runes of a value Quote keeps before truncating it.
const quoteRunes = 128

// ErrInvalid is returned for free text that is not valid.
var ErrInvalid = errors.New("invalid text")

// Validate reports whether s is valid free text: at most MaxLen bytes of valid
// UTF-8 with no control character (C0, DEL or C1, including tab and newline) and
// no format character (Unicode category Cf: bidi controls, zero-width
// characters, the byte order mark). The empty string is valid; whether a value
// may be empty is the caller's decision.
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

		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return fmt.Errorf("%w: disallowed character %U at byte %d", ErrInvalid, r, i)
		}
	}

	return nil
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

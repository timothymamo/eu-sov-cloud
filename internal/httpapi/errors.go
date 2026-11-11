package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// errorDocument is the one shape of every error body.
type errorDocument struct {
	Error errorBody `json:"error"`
}

// errorBody is the code and message of an error.
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// internalBody is the body of every 500: fixed, with nothing internal in it.
const internalBody = `{"error":{"code":"internal","message":"internal error"}}`

// writeError writes the error shape with status, code and message. message
// must hold nothing from the request beyond what escape lets through.
func writeError(w http.ResponseWriter, status int, code, message string) {
	body, err := json.Marshal(errorDocument{Error: errorBody{Code: code, Message: message}})
	if err != nil {
		writeInternal(w)

		return
	}

	writeBody(w, status, body)
}

// writeInternal writes 500 internal with its fixed message.
func writeInternal(w http.ResponseWriter) {
	writeBody(w, http.StatusInternalServerError, []byte(internalBody))
}

// writeBody writes a JSON body with status.
func writeBody(w http.ResponseWriter, status int, body []byte) {
	header := w.Header()
	header.Set("Content-Type", jsonMediaType)
	header.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body) // A client gone mid-write can be told nothing.
}

// maxMessageBytes is the longest message, after escaping, escape returns. A
// longer one is cut and ends in a truncation marker.
const maxMessageBytes = 1024

// escape returns msg as one line that is safe to show, as the CLI's sanitise
// does for its error messages: every character strconv does not call
// printable (control characters such as the line feed and the escape, format
// characters such as the bidi controls, the line and paragraph separators,
// spaces other than U+0020) is written as its Go escape, so a line feed becomes
// \n and an escape \x1b, and each byte that is not valid UTF-8 as \xNN.
// Everything else, the backslash included, is kept: the messages already hold
// values quoted by text.Quote, whose escapes must not be doubled.
//
// A result longer than maxMessageBytes is cut before the first character that
// would cross the limit, never inside an escape or a UTF-8 sequence, and
// followed by a marker that gives the length of msg.
func escape(msg string) string {
	var b strings.Builder

	for i := 0; i < len(msg); {
		r, size := utf8.DecodeRuneInString(msg[i:])
		piece := msg[i : i+size]

		switch {
		case r == utf8.RuneError && size == 1:
			piece = fmt.Sprintf(`\x%02x`, msg[i])
		case !strconv.IsPrint(r):
			quoted := strconv.QuoteRune(r)
			piece = quoted[1 : len(quoted)-1]
		}

		if b.Len()+len(piece) > maxMessageBytes {
			fmt.Fprintf(&b, "...(truncated, %d bytes)", len(msg))

			return b.String()
		}

		b.WriteString(piece)

		i += size
	}

	return b.String()
}

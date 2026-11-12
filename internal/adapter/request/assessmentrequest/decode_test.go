// Package assessmentrequest_test pins the strict decoder of a
// POST /v1/assessments request body (PRD #103 milestone 2): the envelope it
// accepts, the inventory bytes and target keys it returns, and its refusal of
// everything else, with an error that never echoes a value from the body. It is
// black-box: only the package's exported API is used.

package assessmentrequest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/request/assessmentrequest"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/inventoryfile"
)

// bodyMargin is the small margin the request-body cap adds to the inventory cap,
// so the envelope around the inventory fits too. It is written out here so the
// pin below holds the value, not the expression the package happens to use.
const bodyMargin = 64 << 10

// compactInventory is an inventory document in the shape the API receives it:
// one JSON object on one line, which the decoder must return byte for byte.
const compactInventory = `{"schemaVersion":1,"source":"aws","resources":[]}`

// prettyInventory is the same inventory with white space between its tokens.
// JSON allows it, an inventory reader accepts it, and it is returned as it
// arrived.
const prettyInventory = `{
  "schemaVersion": 1,
  "source": "aws",
  "resources": []
}`

// TestMaxBodySize pins the cap the composition root hands to the HTTP layer:
// the inventory cap plus a small margin for the envelope (PRD #103).
func TestMaxBodySize(t *testing.T) {
	t.Parallel()

	want := int64(inventoryfile.MaxFileSize + bodyMargin)

	if got := int64(assessmentrequest.MaxBodySize); got != want {
		t.Errorf("MaxBodySize = %d, want inventoryfile.MaxFileSize + %d = %d", got, bodyMargin, want)
	}
}

// TestDecode pins what a well-formed request body decodes to: the inventory
// object as the raw bytes it was, for the inventory reader to decode later, and
// the target keys in the order they were sent. This package validates no target
// key: whether one names a target is the HTTP layer's decision.
func TestDecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		// wantInventory is the inventory object the body holds.
		wantInventory string
		// normalise compares the returned inventory with wantInventory after
		// json.Compact, because the case's bytes are not compact. The default
		// comparison is exact: a compact inventory comes back byte for byte.
		normalise   bool
		wantTargets []string
	}{
		{
			name:          "one target",
			body:          `{"inventory":` + compactInventory + `,"targets":["scaleway"]}`,
			wantInventory: compactInventory,
			wantTargets:   []string{"scaleway"},
		},
		{
			// The report runs the targets in the order they were asked for, so the
			// order must survive the decode.
			name:          "several targets keep their order",
			body:          `{"inventory":` + compactInventory + `,"targets":["upcloud","scaleway","ovhcloud"]}`,
			wantInventory: compactInventory,
			wantTargets:   []string{"upcloud", "scaleway", "ovhcloud"},
		},
		{
			// "all" is the comparison every target, and this package passes it on
			// as a key like any other.
			name:          `the key "all" passes through`,
			body:          `{"inventory":` + compactInventory + `,"targets":["all"]}`,
			wantInventory: compactInventory,
			wantTargets:   []string{"all"},
		},
		{
			// No target list is known here, so a key that names none is passed on
			// too; the HTTP layer refuses it.
			name:          "an unknown target key passes through",
			body:          `{"inventory":` + compactInventory + `,"targets":["not-a-target"]}`,
			wantInventory: compactInventory,
			wantTargets:   []string{"not-a-target"},
		},
		{
			name:          "the envelope keys in either order",
			body:          `{"targets":["ovhcloud"],"inventory":` + compactInventory + `}`,
			wantInventory: compactInventory,
			wantTargets:   []string{"ovhcloud"},
		},
		{
			name:          "an empty inventory object",
			body:          `{"inventory":{},"targets":["ovhcloud"]}`,
			wantInventory: `{}`,
			wantTargets:   []string{"ovhcloud"},
		},
		{
			name: "an inventory with resources",
			body: `{"inventory":{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"db-example","category":"database.postgresql"}]},"targets":["ovhcloud"]}`,
			wantInventory: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"db-example","category":"database.postgresql"}]}`,
			wantTargets: []string{"ovhcloud"},
		},
		{
			// White space between the tokens of the inventory is the inventory
			// reader's business, not this decoder's: the bytes are passed on, so
			// the inventory reader decides what they mean.
			name:          "an inventory written with white space",
			body:          `{"inventory":` + prettyInventory + `,"targets":["ovhcloud"]}`,
			wantInventory: prettyInventory,
			normalise:     true,
			wantTargets:   []string{"ovhcloud"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotInventory, gotTargets, err := assessmentrequest.Decode([]byte(tt.body))
			if err != nil {
				t.Fatalf("Decode() returned unexpected error: %v", err)
			}

			wantInventory := []byte(tt.wantInventory)
			if tt.normalise {
				wantInventory = compact(t, wantInventory)
				gotInventory = compact(t, gotInventory)
			}

			if diff := cmp.Diff(string(wantInventory), string(gotInventory)); diff != "" {
				t.Errorf("Decode() inventory differs (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tt.wantTargets, gotTargets); diff != "" {
				t.Errorf("Decode() targets differ (-want +got):\n%s", diff)
			}
		})
	}
}

// compact returns data with the white space between its JSON tokens removed.
func compact(t *testing.T, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		t.Fatalf("compacting the JSON %q: %v", data, err)
	}

	return buf.Bytes()
}

// TestDecodeErrors pins every request body the decoder refuses: the envelope
// must be one JSON object with exactly the keys inventory (an object) and
// targets (a non-empty array of strings), and nothing else, exactly once, and
// nothing after it. Every refusal wraps ErrDecode and is one line.
func TestDecodeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body []byte
	}{
		{name: "nil body", body: nil},
		{name: "empty body", body: []byte{}},
		{name: "white space only", body: []byte(" \n\t ")},
		{name: "not JSON", body: []byte("not json")},
		{name: "top level array", body: []byte(`[]`)},
		{name: "top level string", body: []byte(`"scaleway"`)},
		{name: "top level number", body: []byte(`42`)},
		{name: "top level null", body: []byte(`null`)},
		{name: "missing inventory", body: []byte(`{"targets":["scaleway"]}`)},
		{name: "missing targets", body: []byte(`{"inventory":{}}`)},
		{name: "inventory is an array", body: []byte(`{"inventory":[],"targets":["scaleway"]}`)},
		{name: "inventory is a string", body: []byte(`{"inventory":"{}","targets":["scaleway"]}`)},
		{name: "inventory is null", body: []byte(`{"inventory":null,"targets":["scaleway"]}`)},
		{name: "inventory is a number", body: []byte(`{"inventory":1,"targets":["scaleway"]}`)},
		{name: "targets is not an array", body: []byte(`{"inventory":{},"targets":"scaleway"}`)},
		{name: "targets is null", body: []byte(`{"inventory":{},"targets":null}`)},
		{name: "targets is an object", body: []byte(`{"inventory":{},"targets":{}}`)},
		{name: "targets is empty", body: []byte(`{"inventory":{},"targets":[]}`)},
		{name: "target is a number", body: []byte(`{"inventory":{},"targets":[42]}`)},
		{name: "target is null", body: []byte(`{"inventory":{},"targets":[null]}`)},
		{name: "target is an object", body: []byte(`{"inventory":{},"targets":[{}]}`)},
		{name: "target is an array", body: []byte(`{"inventory":{},"targets":[["scaleway"]]}`)},
		{
			name: "unknown top-level key",
			body: []byte(`{"inventory":{},"targets":["scaleway"],"extra":true}`),
		},
		{
			name: "duplicate targets key",
			body: []byte(`{"inventory":{},"targets":["scaleway"],"targets":["ovhcloud"]}`),
		},
		{
			name: "duplicate inventory key",
			body: []byte(`{"inventory":{},"inventory":{},"targets":["scaleway"]}`),
		},
		{
			// The shared decoder refuses a repeated key whatever its case, as
			// encoding/json would silently keep the last of the two.
			name: "case-folded duplicate targets key",
			body: []byte(`{"inventory":{},"Targets":["scaleway"],"targets":["ovhcloud"]}`),
		},
		{
			name: "trailing data after the object",
			body: []byte(`{"inventory":{},"targets":["scaleway"]} {}`),
		},
		{
			name: "trailing byte after the object",
			body: []byte(`{"inventory":{},"targets":["scaleway"]} x`),
		},
		{
			// encoding/json would read the byte as U+FFFD and decode on, so the
			// inventory would differ from the bytes without a word.
			name: "invalid UTF-8 in a value",
			body: []byte(`{"inventory":{"source":"a` + "\xff" + `b"},"targets":["scaleway"]}`),
		},
		{
			name: "byte order mark before the object",
			body: []byte("\xef\xbb\xbf" + `{"inventory":{},"targets":["scaleway"]}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inventoryBytes, targets, err := assessmentrequest.Decode(tt.body)
			if err == nil {
				t.Fatalf("Decode() returned inventory %q and targets %q, want an error wrapping %v",
					inventoryBytes, targets, assessmentrequest.ErrDecode)
			}

			if !errors.Is(err, assessmentrequest.ErrDecode) {
				t.Errorf("Decode() error = %v, want it to wrap %v", err, assessmentrequest.ErrDecode)
			}

			if msg := err.Error(); strings.ContainsAny(msg, "\r\n") {
				t.Errorf("Decode() error = %q, want a single line", msg)
			}

			// A refused body yields nothing: a caller that ignores the error must
			// not get a half-decoded envelope.
			if len(inventoryBytes) != 0 || len(targets) != 0 {
				t.Errorf("Decode() returned inventory %q and targets %q beside an error, want neither",
					inventoryBytes, targets)
			}
		})
	}
}

// TestDecodeErrorNeverEchoesAValue pins that no value from the body reaches the
// error, however the body is refused: the error may name a key or a field
// through text.Quote and place it, never repeat a value the client sent.
func TestDecodeErrorNeverEchoesAValue(t *testing.T) {
	t.Parallel()

	const marker = "MARKER-VALUE-9"

	tests := []struct {
		name string
		body string
	}{
		{
			name: "a value under an unknown top-level key",
			body: `{"inventory":{},"targets":["ovhcloud"],"extra":"` + marker + `"}`,
		},
		{
			name: "an inventory that is an array holding it",
			body: `{"inventory":["` + marker + `"],"targets":["ovhcloud"]}`,
		},
		{
			name: "an inventory beside an unknown top-level key",
			body: `{"inventory":{"source":"` + marker + `"},"targets":["ovhcloud"],"extra":true}`,
		},
		{
			name: "a targets value that is a string, not the array",
			body: `{"inventory":{},"targets":"` + marker + `"}`,
		},
		{
			name: "a target that is a number, beside it",
			body: `{"inventory":{},"targets":[42,"` + marker + `"]}`,
		},
		{
			name: "a target that is an object around it",
			body: `{"inventory":{},"targets":[{"key":"` + marker + `"}]}`,
		},
		{
			name: "a repeated targets key whose second value holds it",
			body: `{"inventory":{},"targets":["ovhcloud"],"targets":["` + marker + `"]}`,
		},
		{
			name: "data after the object holding it",
			body: `{"inventory":{},"targets":["ovhcloud"]} "` + marker + `"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := assessmentrequest.Decode([]byte(tt.body))
			if err == nil {
				t.Fatalf("Decode() returned no error, want one wrapping %v", assessmentrequest.ErrDecode)
			}

			if !errors.Is(err, assessmentrequest.ErrDecode) {
				t.Errorf("Decode() error = %v, want it to wrap %v", err, assessmentrequest.ErrDecode)
			}

			if msg := err.Error(); strings.Contains(msg, marker) {
				t.Errorf("Decode() error = %q, want it not to echo the body value %q", msg, marker)
			}
		})
	}
}

// TestDecodeErrorEscapesAKey pins that an unknown key reaches the error only
// through text.Quote: a key that holds a control character or a quote appears
// escaped, never as the raw bytes, so the error stays one printable line.
func TestDecodeErrorEscapesAKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// body holds the key with its JSON escapes.
		body string
		// raw is the key as JSON decodes it.
		raw string
		// escaped is how text.Quote shows it, and how the error must show it.
		escaped string
	}{
		{
			name:    "a key holding a newline",
			body:    `{"inventory":{},"targets":["ovhcloud"],"bad\nkey":true}`,
			raw:     "bad\nkey",
			escaped: `bad\nkey`,
		},
		{
			name:    "a key holding a tab",
			body:    `{"inventory":{},"targets":["ovhcloud"],"bad\tkey":true}`,
			raw:     "bad\tkey",
			escaped: `bad\tkey`,
		},
		{
			name:    "a key holding a quote",
			body:    `{"inventory":{},"targets":["ovhcloud"],"ev\"il":true}`,
			raw:     `ev"il`,
			escaped: `ev\"il`,
		},
		{
			name:    "a key holding an escape character",
			body:    `{"inventory":{},"targets":["ovhcloud"],"bad\u001bkey":true}`,
			raw:     "bad\x1bkey",
			escaped: `bad\x1bkey`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := assessmentrequest.Decode([]byte(tt.body))
			if err == nil {
				t.Fatalf("Decode() returned no error, want one wrapping %v", assessmentrequest.ErrDecode)
			}

			if !errors.Is(err, assessmentrequest.ErrDecode) {
				t.Errorf("Decode() error = %v, want it to wrap %v", err, assessmentrequest.ErrDecode)
			}

			msg := err.Error()

			if !strings.Contains(msg, tt.escaped) {
				t.Errorf("Decode() error = %q, want it to name the key as %q through text.Quote", msg, tt.escaped)
			}

			if strings.Contains(msg, tt.raw) {
				t.Errorf("Decode() error = %q, want the key escaped, never the raw bytes", msg)
			}

			for _, r := range msg {
				if unicode.IsControl(r) {
					t.Errorf("Decode() error = %q, want no raw control character %q in it", msg, r)
				}
			}
		})
	}
}

// This file pins FromBytes, the in-memory constructor of the JSON
// inventory-file source: an inventory document that arrives as bytes, never as
// a path, is read under the same rules as a file (PRD #103 milestone 2). It is
// part of the black-box package inventoryfile_test and reuses that package's
// fixtures and helpers.

package inventoryfile_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/internal/adapter/source/inventoryfile"
	"github.com/timothymamo/shift2eu/internal/adapter/source/sourcetest"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	"github.com/timothymamo/shift2eu/internal/domain/text"
)

// FromBytes must build the source type New builds: pin the signature at compile
// time, without calling it with a document that is not an inventory.
var _ func([]byte) *inventoryfile.Source = inventoryfile.FromBytes

// fixtureBytes returns the contents of a sanitised fixture file.
func fixtureBytes(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the fixture %s: %v", path, err)
	}

	return data
}

// inventoryFromBytes decodes doc through FromBytes and returns the inventory.
func inventoryFromBytes(t *testing.T, doc []byte) (inventory.Inventory, error) {
	t.Helper()

	return inventoryfile.FromBytes(doc).Inventory(t.Context())
}

// TestInventoryFromBytesContract runs the source contract suite against the
// in-memory constructor: the recorded inventory, the same one on every call, and
// cancellation. The bytes are the ones testdata/valid.json holds, so the
// expected inventory is the one the file tests expect too.
func TestInventoryFromBytesContract(t *testing.T) {
	t.Parallel()

	sourcetest.Run(t, inventoryfile.FromBytes(fixtureBytes(t, validPath)), wantValidInventory(t))
}

// TestInventoryFromBytesContractVersion2 runs the contract suite on the version 2
// fixture through FromBytes: the API is given whoever's document, and a source
// that drops a version 2 field between calls fails it here too.
func TestInventoryFromBytesContractVersion2(t *testing.T) {
	t.Parallel()

	sourcetest.Run(t, inventoryfile.FromBytes(fixtureBytes(t, validV2Path)), wantVersion2Inventory(t))
}

// TestInventoryFromBytesMatchesFile pins that a document read through FromBytes
// describes the same inventory as the same bytes written to a file and read
// through New: the cap, the decoder and every rule are the file reader's, only
// the way the bytes arrive differs.
func TestInventoryFromBytesMatchesFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// document returns the inventory document the case reads. A fixture is
		// read from disk; a document no fixture holds is written out here.
		document func(t *testing.T) []byte
	}{
		{
			name: "valid fixture",
			document: func(t *testing.T) []byte {
				t.Helper()

				return fixtureBytes(t, validPath)
			},
		},
		{
			// A version 2 document reaches the same inventory through FromBytes as
			// through New, every version 2 field included: the API takes version 2
			// inventories.
			name: "version 2 fixture",
			document: func(t *testing.T) []byte {
				t.Helper()

				return fixtureBytes(t, validV2Path)
			},
		},
		{
			// An empty size or usage object, an empty relations list, a null and an
			// empty workload are all an absent field: the in-memory reader must read
			// them exactly as the file reader does.
			name: "absent version 2 fields read as null and empty",
			document: func(t *testing.T) []byte {
				t.Helper()

				return []byte(schemaDocument(2, `,"size":{},"usage":null,"relations":[],"workload":""`))
			},
		},
		{
			// ADR 0006: null is an absent value for every field but resources, so
			// the in-memory reader must read it exactly as the file reader does.
			name: "null read as absent",
			document: func(t *testing.T) []byte {
				t.Helper()

				return []byte(`{"schemaVersion":1,"source":"aws","resources":[` +
					`{"id":"i-0123456789abcdef0","rawType":null,"category":"compute.vm",` +
					`"region":null,"capabilities":null}]}`)
			},
		},
		{
			name: "no resources",
			document: func(t *testing.T) []byte {
				t.Helper()

				return []byte(`{"schemaVersion":1,"source":"aws","resources":[]}`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := tt.document(t)

			fromBytes, err := inventoryFromBytes(t, doc)
			if err != nil {
				t.Fatalf("FromBytes(...).Inventory() returned unexpected error: %v", err)
			}

			fromFile, err := inventoryfile.New(writeFile(t, t.TempDir(), string(doc))).Inventory(t.Context())
			if err != nil {
				t.Fatalf("New(...).Inventory() on the same document returned unexpected error: %v", err)
			}

			if diff := cmp.Diff(fromFile.Source(), fromBytes.Source()); diff != "" {
				t.Errorf("Source() of FromBytes differs from New (-New +FromBytes):\n%s", diff)
			}

			if diff := cmp.Diff(fromFile.Resources(), fromBytes.Resources()); diff != "" {
				t.Errorf("Resources() of FromBytes differ from New (-New +FromBytes):\n%s", diff)
			}
		})
	}
}

// TestInventoryFromBytesAcceptsDocumentOfMaxSize pins that the cap is the file
// reader's: a document of exactly MaxFileSize bytes is read, one more byte is
// refused for its size, whatever way the bytes arrive.
func TestInventoryFromBytesAcceptsDocumentOfMaxSize(t *testing.T) {
	t.Parallel()

	doc := []byte(paddedInventory(t, inventoryfile.MaxFileSize))

	inv, err := inventoryFromBytes(t, doc)
	if err != nil {
		t.Fatalf("FromBytes(...).Inventory() for a document of exactly MaxFileSize (%d) bytes returned error: %v",
			inventoryfile.MaxFileSize, err)
	}

	if got := inv.Source(); got != "aws" {
		t.Errorf("Source() = %q, want %q", got, "aws")
	}
}

// TestInventoryFromBytesErrors pins every way decoding an in-memory document
// fails: the sentinel the error wraps, and that the error is one line. A nil or
// empty document is a decode failure, never a panic.
func TestInventoryFromBytesErrors(t *testing.T) {
	t.Parallel()

	const validDocument = `{"schemaVersion":1,"source":"aws","resources":[]}`

	tests := []struct {
		name string
		// document is the bytes FromBytes is given.
		document []byte
		// wantErr lists every sentinel the error must wrap.
		wantErr []error
	}{
		{
			name:     "nil document",
			document: nil,
			wantErr:  []error{inventoryfile.ErrDecode},
		},
		{
			name:     "empty document",
			document: []byte{},
			wantErr:  []error{inventoryfile.ErrDecode},
		},
		{
			name:     "malformed JSON",
			document: []byte(`{"schemaVersion":1,`),
			wantErr:  []error{inventoryfile.ErrDecode},
		},
		{
			name:     "not JSON",
			document: []byte("not json"),
			wantErr:  []error{inventoryfile.ErrDecode},
		},
		{
			name:     "unknown top-level field",
			document: []byte(`{"schemaVersion":1,"source":"aws","resources":[],"bogusField":true}`),
			wantErr:  []error{inventoryfile.ErrDecode},
		},
		{
			name:     "unknown resource field",
			document: []byte(`{"schemaVersion":1,"source":"aws","resources":[{"id":"i-0123456789abcdef0","category":"compute.vm","bogusResourceField":true}]}`),
			wantErr:  []error{inventoryfile.ErrDecode},
		},
		{
			name:     "duplicate resources key",
			document: []byte(`{"schemaVersion":1,"source":"aws","resources":[],"resources":[]}`),
			wantErr:  []error{inventoryfile.ErrDecode},
		},
		{
			name:     "trailing data after the object",
			document: []byte(validDocument + " x"),
			wantErr:  []error{inventoryfile.ErrDecode},
		},
		{
			// Versions 1 and 2 are read; a document of another version is refused
			// for its version alone, before any key of either is checked.
			name:     "unsupported schema version",
			document: []byte(`{"schemaVersion":3,"source":"aws","resources":[]}`),
			wantErr:  []error{inventoryfile.ErrSchemaVersion},
		},
		{
			name:     "missing schema version",
			document: []byte(`{"source":"aws","resources":[]}`),
			wantErr:  []error{inventoryfile.ErrSchemaVersion},
		},
		{
			// The strict rules of version 1 hold for every version 2 object too.
			name:     "unknown key in a size object",
			document: []byte(schemaDocument(2, `,"size":{"vcpu":4}`)),
			wantErr:  []error{inventoryfile.ErrDecode},
		},
		{
			name:     "repeated key in a size object",
			document: []byte(schemaDocument(2, `,"size":{"vcpus":2,"vcpus":4}`)),
			wantErr:  []error{inventoryfile.ErrDecode},
		},
		{
			name:     "relation without a target",
			document: []byte(relationDocument(`[{"type":"attached-to"}]`)),
			wantErr:  []error{inventoryfile.ErrDecode},
		},
		{
			name:     "relation to a resource that is not in the document",
			document: []byte(relationDocument(`[{"type":"replica-of","target":"db-missing"}]`)),
			wantErr:  []error{inventory.ErrUnknownRelationTarget},
		},
		{
			name:     "document larger than MaxFileSize",
			document: []byte(paddedInventory(t, inventoryfile.MaxFileSize+1)),
			wantErr:  []error{inventoryfile.ErrTooLarge},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := inventoryFromBytes(t, tt.document)
			if err == nil {
				t.Fatalf("FromBytes(...).Inventory() returned no error, want one wrapping %v", tt.wantErr)
			}

			for _, sentinel := range tt.wantErr {
				if !errors.Is(err, sentinel) {
					t.Errorf("FromBytes(...).Inventory() error = %v, want it to wrap %v", err, sentinel)
				}
			}

			if msg := err.Error(); msg == "" {
				t.Error("FromBytes(...).Inventory() error is empty, want a message naming the problem")
			} else if strings.ContainsAny(msg, "\r\n") {
				t.Errorf("FromBytes(...).Inventory() error = %q, want a single line", msg)
			}
		})
	}
}

// TestInventoryFromBytesErrorNeverEchoesAValue pins that a value in the document
// never reaches the error, however the document is refused: an in-memory
// document has no file name to fall back on, and the error is read by whoever
// sent the bytes. The error still has to say where the document is wrong and
// which field is at fault, by position and name, never by quoting the value.
func TestInventoryFromBytesErrorNeverEchoesAValue(t *testing.T) {
	t.Parallel()

	const marker = "ACCT-MARKER-4242"

	tests := []struct {
		name     string
		document string
		wantErr  error
		// value is the document value the error must not echo.
		value string
		// wantMentions lists what the error must still say: where the resource
		// is and which field is at fault. Matched case-insensitively, so a
		// message naming its field as a key or in prose both pass.
		wantMentions []string
		// notMentions lists the fields the error must not name: when two fields
		// of one resource are refused, the one the domain checks first is at
		// fault, and naming the other sends the reader to the wrong value.
		notMentions []string
	}{
		{
			// The unknown key is named; the ID beside it is not.
			name: "unknown field beside the marker",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"` + marker + `","category":"compute.vm","bogus":true}]}`,
			wantErr: inventoryfile.ErrDecode,
			value:   marker,
		},
		{
			// The wrong type is named by its path, and the value that is wrong is
			// the number, not the ID before it.
			name: "wrong type after the marker",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"` + marker + `","category":42}]}`,
			wantErr: inventoryfile.ErrDecode,
			value:   marker,
		},
		{
			// A version that is not an integer is described by its type, not by
			// the value it holds.
			name:     "marker as an unsupported schema version",
			document: `{"schemaVersion":"` + marker + `","source":"aws","resources":[]}`,
			wantErr:  inventoryfile.ErrSchemaVersion,
			value:    marker,
		},
		{
			name: "marker in an unknown resource field name's value",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"i-0123456789abcdef0","category":"compute.vm","regoin":"` + marker + `"}]}`,
			wantErr: inventoryfile.ErrDecode,
			value:   marker,
		},
		{
			// The duplicate is named by the position of the second resource and
			// the field, never by repeating the ID both share.
			name: "a duplicate resource ID",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"` + marker + `","category":"compute.vm"},` +
				`{"id":"` + marker + `","category":"compute.vm"}]}`,
			wantErr:      inventory.ErrDuplicateID,
			value:        marker,
			wantMentions: []string{"resources[1]", "id"},
		},
		{
			// The category value is the second resource's, named by position and
			// field, not by quoting the value the taxonomy does not know.
			name: "an unknown category value",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"instance-one","category":"compute.vm"},` +
				`{"id":"instance-two","category":"` + marker + `"}]}`,
			wantErr:      taxonomy.ErrUnknownCategory,
			value:        marker,
			wantMentions: []string{"1", "category"},
		},
		{
			// Likewise a capability the vocabulary does not hold: the resource's
			// position and the capabilities field identify it.
			name: "an unknown capability value",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"instance-one","category":"compute.vm","capabilities":["` + marker + `"]}]}`,
			wantErr:      taxonomy.ErrUnknownCapability,
			value:        marker,
			wantMentions: []string{"0", "capabilit"},
		},
		{
			// A schemaVersion that is the wrong number is named by its key and
			// the version the reader wants, never by the number it read.
			name:         "a schema version number",
			document:     `{"schemaVersion":4242,"source":"aws","resources":[]}`,
			wantErr:      inventoryfile.ErrSchemaVersion,
			value:        "4242",
			wantMentions: []string{"schemaVersion"},
		},
		{
			// The document repeats an ID and the second resource also carries a
			// category the taxonomy does not know. The domain refuses the repeat
			// first, so the ID is the field at fault; naming the category would
			// send the reader to a value that is not what the error is about.
			// The path and the field are asserted together: the sentinel's own
			// text already holds "ID", so a bare "id" mention proves nothing.
			name: "a duplicate resource ID beside an unknown category",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"` + marker + `","category":"compute.vm"},` +
				`{"id":"` + marker + `","category":"nope"}]}`,
			wantErr:      inventory.ErrDuplicateID,
			value:        marker,
			wantMentions: []string{"resources[1].id"},
			notMentions:  []string{"category"},
		},
		{
			// Two fields of the one resource are invalid: its ID holds a no-break
			// space, which validateID refuses, and its raw type starts with a
			// space. The domain checks the ID first, so the ID is the field at
			// fault.
			name: "an invalid ID beside an invalid raw type",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"` + marker + `\u00a0x","rawType":" x","category":"compute.vm"}]}`,
			wantErr:      text.ErrInvalid,
			value:        marker,
			wantMentions: []string{"resources[0].id"},
			notMentions:  []string{"rawType"},
		},
		{
			// A version 2 key in a version 1 document is the unknown key it is,
			// named by its path; the value beside it is never echoed.
			name: "a version 2 key in a version 1 document",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"i-0123456789abcdef0","category":"compute.vm","workload":"` + marker + `"}]}`,
			wantErr:      inventoryfile.ErrDecode,
			value:        marker,
			wantMentions: []string{"resources[0].workload"},
		},
		{
			// A version 2 size that is not a number is named by its path, never by
			// the value that stands where the number belongs.
			name: "a size member of the wrong type",
			document: `{"schemaVersion":2,"source":"aws","resources":[` +
				`{"id":"i-0123456789abcdef0","category":"compute.vm",` +
				`"size":{"vcpus":0,"memoryGiB":"` + marker + `"}}]}`,
			wantErr:      inventoryfile.ErrDecode,
			value:        marker,
			wantMentions: []string{"resources[0].size.memoryGiB"},
		},
		{
			// A relation to a resource the document does not hold is named by the
			// relation's position and field, not by quoting the target.
			name: "a relation to a resource that is not in the document",
			document: `{"schemaVersion":2,"source":"aws","resources":[` +
				`{"id":"db-primary","category":"database.postgresql"},` +
				`{"id":"db-replica","category":"database.postgresql",` +
				`"relations":[{"type":"replica-of","target":"` + marker + `"}]}]}`,
			wantErr:      inventory.ErrUnknownRelationTarget,
			value:        marker,
			wantMentions: []string{"resources[1].relations[0].target"},
		},
		{
			// A relation type outside the vocabulary is named by the relation's
			// position and field, not by quoting the type.
			name: "a relation type outside the vocabulary",
			document: `{"schemaVersion":2,"source":"aws","resources":[` +
				`{"id":"db-primary","category":"database.postgresql"},` +
				`{"id":"db-replica","category":"database.postgresql",` +
				`"relations":[{"type":"` + marker + `","target":"db-primary"}]}]}`,
			wantErr:      inventory.ErrUnknownRelationType,
			value:        marker,
			wantMentions: []string{"resources[1].relations[0].type"},
		},
		{
			// A currency the domain refuses is named by position and field.
			name: "a currency outside the ISO 4217 shape",
			document: `{"schemaVersion":2,"source":"aws","resources":[` +
				`{"id":"i-0123456789abcdef0","category":"compute.vm",` +
				`"monthlyCost":{"amount":1,"currency":"` + marker + `"}}]}`,
			wantErr:      inventory.ErrInvalidCost,
			value:        marker,
			wantMentions: []string{"resources[0].monthlyCost.currency"},
		},
		{
			// The guard against over-correcting: with a valid ID and only a raw
			// type at fault, the raw type is named and the ID beside it is not.
			name: "an invalid raw type alone",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"i-0123456789abcdef0","rawType":" x","category":"compute.vm"}]}`,
			wantErr:      text.ErrInvalid,
			value:        " x",
			wantMentions: []string{"resources[0].rawType"},
			notMentions:  []string{"resources[0].id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := inventoryFromBytes(t, []byte(tt.document))
			if err == nil {
				t.Fatalf("FromBytes(...).Inventory() returned no error, want one wrapping %v", tt.wantErr)
			}

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("FromBytes(...).Inventory() error = %v, want it to wrap %v", err, tt.wantErr)
			}

			msg := err.Error()
			if strings.Contains(msg, tt.value) {
				t.Errorf("FromBytes(...).Inventory() error = %q, want it not to echo the document value %q", msg, tt.value)
			}

			lowered := strings.ToLower(msg)
			for _, mention := range tt.wantMentions {
				if !strings.Contains(lowered, strings.ToLower(mention)) {
					t.Errorf("FromBytes(...).Inventory() error = %q, want it to name %q", msg, mention)
				}
			}

			for _, mention := range tt.notMentions {
				if strings.Contains(lowered, strings.ToLower(mention)) {
					t.Errorf("FromBytes(...).Inventory() error = %q, want it not to name the field %q", msg, mention)
				}
			}
		})
	}
}

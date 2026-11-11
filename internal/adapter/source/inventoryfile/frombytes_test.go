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

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/inventoryfile"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/sourcetest"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
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
			// The version is checked before any key or type of version 1, so a
			// document of another version is refused for its version alone.
			name:     "unsupported schema version",
			document: []byte(`{"schemaVersion":2,"source":"aws","resources":[]}`),
			wantErr:  []error{inventoryfile.ErrSchemaVersion},
		},
		{
			name:     "missing schema version",
			document: []byte(`{"source":"aws","resources":[]}`),
			wantErr:  []error{inventoryfile.ErrSchemaVersion},
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
// sent the bytes.
func TestInventoryFromBytesErrorNeverEchoesAValue(t *testing.T) {
	t.Parallel()

	const marker = "SECRET-MARKER-123"

	tests := []struct {
		name     string
		document string
		wantErr  error
	}{
		{
			// The unknown key is named; the ID beside it is not.
			name: "unknown field beside the marker",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"` + marker + `","category":"compute.vm","bogus":true}]}`,
			wantErr: inventoryfile.ErrDecode,
		},
		{
			// The wrong type is named by its path, and the value that is wrong is
			// the number, not the ID before it.
			name: "wrong type after the marker",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"` + marker + `","category":42}]}`,
			wantErr: inventoryfile.ErrDecode,
		},
		{
			// A version that is not an integer is described by its type, not by
			// the value it holds.
			name:     "marker as an unsupported schema version",
			document: `{"schemaVersion":"` + marker + `","source":"aws","resources":[]}`,
			wantErr:  inventoryfile.ErrSchemaVersion,
		},
		{
			name: "marker in an unknown resource field name's value",
			document: `{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"i-0123456789abcdef0","category":"compute.vm","regoin":"` + marker + `"}]}`,
			wantErr: inventoryfile.ErrDecode,
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

			if msg := err.Error(); strings.Contains(msg, marker) {
				t.Errorf("FromBytes(...).Inventory() error = %q, want it not to echo the document value %q", msg, marker)
			}
		})
	}
}

// Package inventoryfile_test pins the JSON inventory-file source adapter: the
// source contract in internal/adapter/source/sourcetest, and the adapter's own
// error contract - one line, the file named through text.Quote, the sentinel it
// wraps. It is black-box: only the package's exported API is used.
//
// Every fixture under testdata/ is sanitised: resource IDs such as
// i-0123456789abcdef0 and db-example, the region eu-west-1.
package inventoryfile_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/inventoryfile"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/sourcetest"
	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// The adapter implements the inventory source port: pin that at compile time.
var _ analysis.InventorySource = inventoryfile.New(validPath)

// validPath is the sanitised inventory the fixture-driven tests read. It lists
// its resources out of ID order on purpose, so reading it also pins that the
// inventory, not the file, decides the order.
const validPath = "testdata/valid.json"

// fixture returns a function producing the path of a sanitised fixture, for the
// table-driven error tests, which build their path inside the subtest.
func fixture(name string) func(t *testing.T) string {
	return func(t *testing.T) string {
		t.Helper()

		return filepath.Join("testdata", name)
	}
}

// inventoryOf reads path with a fresh source and returns the inventory.
func inventoryOf(t *testing.T, path string) inventory.Inventory {
	t.Helper()

	inv, err := inventoryfile.New(path).Inventory(t.Context())
	if err != nil {
		t.Fatalf("Inventory(%q) returned unexpected error: %v", path, err)
	}

	return inv
}

// resourceIDs returns the IDs of resources, in the order given.
func resourceIDs(resources []inventory.Resource) []string {
	ids := make([]string, 0, len(resources))
	for _, r := range resources {
		ids = append(ids, r.ID)
	}

	return ids
}

// mentions reports whether s contains at least one of keywords. The adapter's
// exact wording is not part of the contract; naming the problem is.
func mentions(s string, keywords ...string) bool {
	return slices.ContainsFunc(keywords, func(keyword string) bool {
		return strings.Contains(s, keyword)
	})
}

// writeFile writes content to a fresh file in dir and returns its path.
func writeFile(t *testing.T, dir, content string) string {
	t.Helper()

	path := filepath.Join(dir, "inventory.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the fixture %s: %v", text.Quote(path), err)
	}

	return path
}

// paddedInventory returns a valid inventory document of exactly size bytes: an
// empty inventory padded with trailing spaces, which JSON allows.
func paddedInventory(t *testing.T, size int) string {
	t.Helper()

	const document = `{"schemaVersion":1,"source":"aws","resources":[]}`
	if size < len(document) {
		t.Fatalf("paddedInventory(%d): smaller than the %d-byte document", size, len(document))
	}

	return document + strings.Repeat(" ", size-len(document))
}

// emptyResourcesDocument returns an inventory document holding n empty resource
// objects: the shortest resource JSON allows, for the tests that need far more
// resources than a document small enough to read can carry with full ones.
func emptyResourcesDocument(n int) string {
	var b strings.Builder
	b.WriteString(`{"schemaVersion":1,"source":"aws","resources":[`)

	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}

		b.WriteString("{}")
	}

	b.WriteString("]}")

	return b.String()
}

// resourcesDocument returns an inventory document holding n resources: input
// too large for a fixture.
func resourcesDocument(t *testing.T, n int) string {
	t.Helper()

	var b strings.Builder
	b.WriteString(`{"schemaVersion":1,"source":"aws","resources":[`)

	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}

		b.WriteString(`{"id":"i-`)
		b.WriteString(strconv.Itoa(i))
		b.WriteString(`","category":"compute.vm","region":"eu-west-1"}`)
	}

	b.WriteString("]}")

	return b.String()
}

// missingFileInDir returns the path of a file that does not exist, in a fresh
// directory under t.TempDir() named name.
func missingFileInDir(t *testing.T, name string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("creating the directory %s: %v", text.Quote(dir), err)
	}

	return filepath.Join(dir, "inventory.json")
}

// wantValidInventory is the inventory testdata/valid.json must produce. It is
// built with inventory.New from the same data, so the domain's own rules
// (capabilities sorted, resources in ID order) apply to the expectation too: a
// direct-able compute.vm with a capability, a partial-able database with two,
// a storage.block with no capabilities, and an explicitly unclassified resource
// with neither raw type nor region.
func wantValidInventory(t *testing.T) inventory.Inventory {
	t.Helper()

	inv, err := inventory.New("aws", []inventory.Resource{
		{
			ID:           "i-0123456789abcdef0",
			RawType:      "AWS::EC2::Instance",
			Category:     taxonomy.ComputeVM,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.IPv6},
		},
		{
			ID:           "db-example",
			RawType:      "AWS::RDS::DBInstance",
			Category:     taxonomy.DatabasePostgreSQL,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.ReadReplicas, taxonomy.PointInTimeRestore},
		},
		{
			ID:       "volume-example",
			RawType:  "AWS::EC2::Volume",
			Category: taxonomy.StorageBlock,
			Region:   "eu-west-1",
		},
		{
			ID:       "global-example",
			Category: taxonomy.Unclassified,
		},
	})
	if err != nil {
		t.Fatalf("inventory.New() returned unexpected error: %v", err)
	}

	return inv
}

// TestInventoryContract runs the source contract suite every source adapter must
// pass: the recorded inventory, the same one on every call, and cancellation.
func TestInventoryContract(t *testing.T) {
	t.Parallel()

	sourcetest.Run(t, inventoryfile.New(validPath), wantValidInventory(t))
}

// Documents that are written by the test instead of installed as fixtures.
//
// A fixture cannot hold them reliably: an editor, a formatter or a git filter
// drops a byte order mark or re-encodes invalid UTF-8 without saying so, and a
// repeated key is easy to "fix" by accident. Spelling the bytes out here keeps
// each case exact.
const (
	// duplicateCapabilitiesDocument repeats a resource's capabilities key. The
	// first value carries a capability, the second is empty, so a decoder that
	// accepts duplicates silently decides which one the resource has.
	duplicateCapabilitiesDocument = `{"schemaVersion":1,"source":"aws","resources":[` +
		`{"id":"db-example","category":"database.postgresql",` +
		`"capabilities":["point-in-time-restore"],"capabilities":[]}]}`
	// duplicateCategoryDocument repeats a resource's category key.
	duplicateCategoryDocument = `{"schemaVersion":1,"source":"aws","resources":[` +
		`{"id":"db-example","category":"database.postgresql","category":"unclassified"}]}`
	// duplicateResourcesDocument repeats the top-level resources key.
	duplicateResourcesDocument = `{"schemaVersion":1,"source":"aws","resources":[],"resources":[]}`
	// inexactCapabilitiesDocument capitalises a resource's capabilities key.
	inexactCapabilitiesDocument = `{"schemaVersion":1,"source":"aws","resources":[` +
		`{"id":"i-0123456789abcdef0","category":"compute.vm","Capabilities":[]}]}`
	// inexactIDDocument capitalises a resource's id key, and carries a valid
	// lowercase id beside it, so the resource is otherwise complete.
	inexactIDDocument = `{"schemaVersion":1,"source":"aws","resources":[` +
		`{"id":"i-0123456789abcdef0","ID":"i-0123456789abcdef1","category":"compute.vm"}]}`
	// inexactCapabilitiesLongSDocument spells the s of capabilities with U+017F
	// LATIN SMALL LETTER LONG S, which Unicode case-folds to s: a decoder that
	// folds case accepts it as the right key.
	inexactCapabilitiesLongSDocument = `{"schemaVersion":1,"source":"aws","resources":[` +
		`{"id":"i-0123456789abcdef0","category":"compute.vm","capabilitie` + "\u017f" + `":[]}]}`
	// inexactSourceLongSDocument adds a second source key spelled with the long s.
	inexactSourceLongSDocument = `{"schemaVersion":1,"source":"aws","resources":[],` +
		`"` + "\u017f" + `ource":"aws"}`
	// unknownResourceFieldInSecondDocument carries the unknown key regoin in the
	// second resource only: the message must say which resource it is.
	unknownResourceFieldInSecondDocument = `{"schemaVersion":1,"source":"aws","resources":[` +
		`{"id":"db-example","category":"database.postgresql"},` +
		`{"id":"volume-example","category":"storage.block","regoin":"eu-west-1"}]}`
	// bomUTF8Document starts with the UTF-8 encoding of U+FEFF, EF BB BF, which
	// JSON does not allow: its text is white space, not a token, and a reader
	// that skips it reads a different document than the bytes say.
	bomUTF8Document = "\xef\xbb\xbf" + `{"schemaVersion":1,"source":"aws","resources":[]}`
	// bomUTF16LEDocument starts with the UTF-16LE byte order mark, FF FE, in a
	// file that is not UTF-16: it is refused, never guessed at.
	bomUTF16LEDocument = "\xff\xfe" + `{"schemaVersion":1,"source":"aws","resources":[]}`
	// invalidUTF8Document holds the byte FF, which is never valid UTF-8, in a
	// resource id: encoding/json would replace it with U+FFFD and read on.
	invalidUTF8Document = `{"schemaVersion":1,"source":"aws","resources":[` +
		`{"id":"a` + "\xff" + `","category":"compute.vm"}]}`
	// stringSchemaVersionDocument spells the version as the string "1", which is
	// not the number 1.
	stringSchemaVersionDocument = `{"schemaVersion":"1","source":"aws","resources":[]}`
	// fractionalSchemaVersionDocument spells the version as the number 1.0, which
	// is a different JSON number from 1.
	fractionalSchemaVersionDocument = `{"schemaVersion":1.0,"source":"aws","resources":[]}`
)

// generated returns a function that writes document to a fresh file, for the
// cases whose input is not a fixture.
func generated(document string) func(t *testing.T) string {
	return func(t *testing.T) string {
		t.Helper()

		return writeFile(t, t.TempDir(), document)
	}
}

// TestInventoryErrors pins every way reading an inventory file fails: the
// sentinel the error wraps, that it names the file through text.Quote(path), and
// that it is one line that echoes the path quoted, never the OS error holding it
// unquoted. A decode failure also says where in the document it is, as a line
// and a column, and which key or location it is about.
func TestInventoryErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// path returns the file to read. Cases a fixture cannot hold write their
		// input here.
		path func(t *testing.T) string
		// wantErr lists every sentinel the error must wrap.
		wantErr []error
		// wantNotErr lists sentinels the error must not wrap: a file refused for
		// one reason is never also reported as another.
		wantNotErr []error
		// wantMentions is a list of alternatives: the message, with the quoted
		// path removed, must contain at least one keyword per group.
		wantMentions [][]string
		// wantQuoted lists values the message must show through text.Quote: the
		// key the problem is about, or where in the document it is.
		wantQuoted []string
		// wantPosition is where the message must place the problem: a 1-based
		// line and column in the document, never a byte offset.
		wantPosition string
		// wantAbsent lists substrings the message must not contain.
		wantAbsent []string
	}{
		{
			name:    "missing file",
			path:    fixture("missing.json"),
			wantErr: []error{inventoryfile.ErrRead},
		},
		{
			// not-json.json holds "not json": the decoder reads n, o, t, then
			// refuses the o on the first line, in its second column.
			name:         "not JSON",
			path:         fixture("not-json.json"),
			wantErr:      []error{inventoryfile.ErrDecode},
			wantPosition: "line 1, column 2",
			wantAbsent:   []string{"byte offset"},
		},
		{
			name:    "trailing data after the object",
			path:    fixture("trailing-data.json"),
			wantErr: []error{inventoryfile.ErrDecode},
		},
		{
			name:       "unknown top-level field",
			path:       fixture("unknown-field.json"),
			wantErr:    []error{inventoryfile.ErrDecode},
			wantQuoted: []string{"bogusField"},
		},
		{
			name:       "unknown resource field",
			path:       fixture("unknown-resource-field.json"),
			wantErr:    []error{inventoryfile.ErrDecode},
			wantQuoted: []string{"bogusResourceField"},
		},
		{
			// Naming the key is not enough: with several resources, the message
			// must say which one the key is in, through text.Quote.
			name:       "unknown field in the second resource",
			path:       generated(unknownResourceFieldInSecondDocument),
			wantErr:    []error{inventoryfile.ErrDecode},
			wantQuoted: []string{"resources.1.regoin"},
		},
		{
			// encoding/json accepts a repeated key and keeps the last value, so a
			// file could give a resource two capabilities lists and one would win
			// silently. The key and where it is repeated are both named.
			name:         "duplicate capabilities key",
			path:         generated(duplicateCapabilitiesDocument),
			wantErr:      []error{inventoryfile.ErrDecode},
			wantMentions: [][]string{{"duplicate", "twice", "repeated", "again", "more than once"}},
			wantQuoted:   []string{"resources.0.capabilities"},
		},
		{
			name:         "duplicate category key",
			path:         generated(duplicateCategoryDocument),
			wantErr:      []error{inventoryfile.ErrDecode},
			wantMentions: [][]string{{"duplicate", "twice", "repeated", "again", "more than once"}},
			wantQuoted:   []string{"resources.0.category"},
		},
		{
			name:         "duplicate resources key",
			path:         generated(duplicateResourcesDocument),
			wantErr:      []error{inventoryfile.ErrDecode},
			wantMentions: [][]string{{"duplicate", "twice", "repeated", "again", "more than once"}},
			wantQuoted:   []string{"resources"},
		},
		{
			// A key must be byte-equal to the one the format declares: encoding/json
			// matches case-insensitively, and Unicode case folding also maps the
			// long s to s. Neither spelling is the capability the format names.
			name:       "inexact capabilities key",
			path:       generated(inexactCapabilitiesDocument),
			wantErr:    []error{inventoryfile.ErrDecode},
			wantQuoted: []string{"Capabilities"},
		},
		{
			name:       "inexact id key",
			path:       generated(inexactIDDocument),
			wantErr:    []error{inventoryfile.ErrDecode},
			wantQuoted: []string{"ID"},
		},
		{
			name:       "inexact capabilities key spelled with a long s",
			path:       generated(inexactCapabilitiesLongSDocument),
			wantErr:    []error{inventoryfile.ErrDecode},
			wantQuoted: []string{"capabilitie\u017f"},
		},
		{
			name:       "inexact source key spelled with a long s",
			path:       generated(inexactSourceLongSDocument),
			wantErr:    []error{inventoryfile.ErrDecode},
			wantQuoted: []string{"\u017fource"},
		},
		{
			// wrong-type.json holds 42 where a resource id string belongs. The 42
			// starts the document's only line in column 54.
			name:         "wrong type for a resource field",
			path:         fixture("wrong-type.json"),
			wantErr:      []error{inventoryfile.ErrDecode},
			wantPosition: "line 1, column 54",
			wantAbsent:   []string{"byte offset"},
		},
		{
			// A byte order mark is not JSON and not UTF-8 text either way: the
			// reader refuses it and says what it found.
			name:         "UTF-8 byte order mark",
			path:         generated(bomUTF8Document),
			wantErr:      []error{inventoryfile.ErrDecode},
			wantMentions: [][]string{{"byte order mark"}},
		},
		{
			name:         "UTF-16 byte order mark",
			path:         generated(bomUTF16LEDocument),
			wantErr:      []error{inventoryfile.ErrDecode},
			wantMentions: [][]string{{"byte order mark"}},
		},
		{
			// encoding/json replaces an invalid byte with U+FFFD and decodes on,
			// which would make the id in the report differ from the id in the file.
			name:    "invalid UTF-8 in a value",
			path:    generated(invalidUTF8Document),
			wantErr: []error{inventoryfile.ErrDecode},
		},
		{
			// The version is checked on the envelope before any field of this
			// version is decoded, so a version 2 file that carries a field version
			// 2 has and version 1 does not is refused for its version alone.
			name:         "unsupported schema version",
			path:         fixture("schema-version-2.json"),
			wantErr:      []error{inventoryfile.ErrSchemaVersion},
			wantNotErr:   []error{inventoryfile.ErrDecode},
			wantMentions: [][]string{{"schemaVersion"}, {"2"}},
		},
		{
			// A version is a JSON number: the string "1" is not it. The message
			// says what it found, in the package's own words, so the value from the
			// file is quoted once and never double-escaped.
			name:         "schema version as a string",
			path:         generated(stringSchemaVersionDocument),
			wantErr:      []error{inventoryfile.ErrSchemaVersion},
			wantMentions: [][]string{{"schemaVersion"}, {"string"}},
			wantAbsent:   []string{`\"`},
		},
		{
			name:         "schema version as a fractional number",
			path:         generated(fractionalSchemaVersionDocument),
			wantErr:      []error{inventoryfile.ErrSchemaVersion},
			wantMentions: [][]string{{"schemaVersion"}},
		},
		{
			name:         "missing schema version",
			path:         fixture("no-schema-version.json"),
			wantErr:      []error{inventoryfile.ErrSchemaVersion},
			wantMentions: [][]string{{"schemaVersion"}, {"0", "absent", "missing", "not set", "not present"}},
		},
		{
			name:         "missing resources",
			path:         fixture("missing-resources.json"),
			wantErr:      []error{inventoryfile.ErrDecode},
			wantMentions: [][]string{{"resources"}},
		},
		{
			// An unknown category is a domain error, never a silent unclassified:
			// the error must name the resource and the category it carries.
			name:         "unknown category",
			path:         fixture("unknown-category.json"),
			wantErr:      []error{taxonomy.ErrUnknownCategory},
			wantMentions: [][]string{{"i-0123456789abcdef0"}, {"compute.mv"}},
		},
		{
			name:    "empty source",
			path:    fixture("empty-source.json"),
			wantErr: []error{inventory.ErrEmptySource},
		},
		{
			name: "file larger than MaxFileSize",
			path: func(t *testing.T) string {
				t.Helper()

				return writeFile(t, t.TempDir(), paddedInventory(t, inventoryfile.MaxFileSize+1))
			},
			wantErr: []error{inventoryfile.ErrTooLarge},
		},
		{
			name: "more resources than MaxResources",
			path: func(t *testing.T) string {
				t.Helper()

				return writeFile(t, t.TempDir(), resourcesDocument(t, inventoryfile.MaxResources+1))
			},
			wantErr: []error{inventoryfile.ErrTooManyResources},
		},
		{
			// The path is untrusted too: a newline in a directory name must not
			// put a line break in an error, which a terminal shows as two errors.
			name:    "path whose directory name holds a newline",
			path:    func(t *testing.T) string { t.Helper(); return missingFileInDir(t, "line\nbreak") },
			wantErr: []error{inventoryfile.ErrRead},
		},
		{
			// An escape sequence in a path must be escaped too, never repeated raw
			// into a terminal.
			name:    "path whose directory name holds an escape sequence",
			path:    func(t *testing.T) string { t.Helper(); return missingFileInDir(t, "ansi\x1bseq") },
			wantErr: []error{inventoryfile.ErrRead},
		},
		{
			// A directory opens, and only reading it fails: the failure is still a
			// read failure, and its path must not put a newline into the message.
			name: "path that is a directory whose name holds a newline",
			path: func(t *testing.T) string {
				t.Helper()

				path := filepath.Join(t.TempDir(), "dir\nname")
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatalf("creating the directory %s: %v", text.Quote(path), err)
				}

				return path
			},
			wantErr: []error{inventoryfile.ErrRead},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.path(t)

			_, err := inventoryfile.New(path).Inventory(t.Context())
			if err == nil {
				t.Fatalf("Inventory() for %s returned no error, want one wrapping %v", text.Quote(path), tt.wantErr)
			}

			for _, sentinel := range tt.wantErr {
				if !errors.Is(err, sentinel) {
					t.Errorf("Inventory() error = %v, want it to wrap %v", err, sentinel)
				}
			}

			for _, sentinel := range tt.wantNotErr {
				if errors.Is(err, sentinel) {
					t.Errorf("Inventory() error = %v, want it not to wrap %v", err, sentinel)
				}
			}

			msg := err.Error()
			quotedPath := text.Quote(path)

			if !strings.Contains(msg, quotedPath) {
				t.Errorf("Inventory() error = %q, want it to name the file through text.Quote(path) = %s", msg, quotedPath)
			}

			if strings.ContainsAny(msg, "\r\n") {
				t.Errorf("Inventory() error = %q, want a single line", msg)
			}

			if strings.Contains(msg, "\x1b") {
				t.Errorf("Inventory() error = %q, want no raw escape sequence", msg)
			}

			// A fixture name may hold a word a case looks for (resources, for one),
			// so the wording check looks at the message without the quoted path.
			rest := strings.ReplaceAll(msg, quotedPath, "")

			for i, group := range tt.wantMentions {
				if !mentions(rest, group...) {
					t.Errorf("Inventory() error = %q, want it to name problem %d (one of %v)", msg, i+1, group)
				}
			}

			for _, value := range tt.wantQuoted {
				if quoted := text.Quote(value); !strings.Contains(msg, quoted) {
					t.Errorf("Inventory() error = %q, want it to name %s through text.Quote", msg, quoted)
				}
			}

			for _, absent := range tt.wantAbsent {
				if strings.Contains(msg, absent) {
					t.Errorf("Inventory() error = %q, want it not to contain %q", msg, absent)
				}
			}

			// Where the problem is in the document is a line and a column a reader
			// of the file can look up, not a byte offset it has to count to.
			if tt.wantPosition != "" && !strings.Contains(msg, tt.wantPosition) {
				t.Errorf("Inventory() error = %q, want it to report the position as %q", msg, tt.wantPosition)
			}

			// An adapter that passes the OS error through echoes the path unquoted:
			// every OS error spells it "<path>: <reason>". The quoted form the
			// contract requires ends the path with a quote, so an unquoted
			// "<path>: " must never appear, whatever operation produced the error.
			// (A decoder's wording cannot be told apart from its quoted form -
			// text.Quote keeps ASCII unchanged - so decoder failures are pinned by
			// ErrDecode alone.)
			if strings.Contains(msg, path+": ") {
				t.Errorf("Inventory() error = %q, want it not to pass the OS error for the path through unquoted", msg)
			}
		})
	}
}

// TestInventoryHonoursCancelledContext pins that a cancelled run reads nothing:
// the file does not even have to exist.
func TestInventoryHonoursCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// The file does not exist, so an adapter that read it before checking the
	// context would fail with ErrRead instead of the cancellation.
	path := filepath.Join("testdata", "missing.json")

	_, err := inventoryfile.New(path).Inventory(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Inventory() with a cancelled context returned error %v, want one wrapping %v", err, context.Canceled)
	}

	if errors.Is(err, inventoryfile.ErrRead) {
		t.Errorf("Inventory() with a cancelled context returned error %v, want it not to wrap ErrRead: the file was read before the context was checked", err)
	}
}

// TestInventoryKeepsUnclassifiedResource pins that an explicitly unclassified
// resource is a fact about the resource, not a value to drop.
func TestInventoryKeepsUnclassifiedResource(t *testing.T) {
	t.Parallel()

	resources := inventoryOf(t, validPath).Resources()

	i := slices.IndexFunc(resources, func(r inventory.Resource) bool {
		return r.ID == "global-example"
	})
	if i < 0 {
		t.Fatalf("Resources() dropped the unclassified resource global-example: got IDs %v", resourceIDs(resources))
	}

	want := inventory.Resource{ID: "global-example", Category: taxonomy.Unclassified}
	if diff := cmp.Diff(want, resources[i]); diff != "" {
		t.Errorf("unclassified resource differs (-want +got):\n%s", diff)
	}
}

// TestInventorySortsResourcesByID pins that file order never reaches the report:
// valid.json lists its resources out of ID order and every read returns them in
// ID order.
func TestInventorySortsResourcesByID(t *testing.T) {
	t.Parallel()

	want := []string{"db-example", "global-example", "i-0123456789abcdef0", "volume-example"}

	for range 2 {
		if diff := cmp.Diff(want, resourceIDs(inventoryOf(t, validPath).Resources())); diff != "" {
			t.Errorf("Resources() IDs differ (-want +got):\n%s", diff)
		}
	}
}

// TestInventoryCapsResourcesBeforeDecoding pins that the resource count is
// checked while the document is read, before its resources are decoded: a file
// holding many times MaxResources resources is refused for its count, and the
// call allocates memory in proportion to the file, not to the resources the
// file claims.
//
// The test is not parallel: TotalAlloc counts the whole process, so a test
// allocating at the same time would be counted here.
func TestInventoryCapsResourcesBeforeDecoding(t *testing.T) {
	// MaxResources*70 empty resources is a 10.5 MB document, larger than
	// MaxFileSize, so the file would be refused for its size and never reach the
	// resource cap. 60 * MaxResources stays under the size cap and still writes
	// 3000000 resources, which decoding all of would allocate far more than the
	// budget below.
	const multiplier = 60

	document := emptyResourcesDocument(inventoryfile.MaxResources * multiplier)
	if len(document) >= inventoryfile.MaxFileSize {
		t.Fatalf("the generated document is %d bytes, want fewer than MaxFileSize (%d) so the file is refused for its resource count, not its size",
			len(document), inventoryfile.MaxFileSize)
	}

	path := writeFile(t, t.TempDir(), document)

	runtime.GC()

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	_, err := inventoryfile.New(path).Inventory(t.Context())

	runtime.ReadMemStats(&after)

	if !errors.Is(err, inventoryfile.ErrTooManyResources) {
		t.Fatalf("Inventory() error = %v, want one wrapping %v", err, inventoryfile.ErrTooManyResources)
	}

	// A decoder that reads every resource before counting them allocates about
	// 1.6 GiB for this document; stopping at the cap allocates the file and very
	// little else.
	const budget = 64 << 20
	if grew := after.TotalAlloc - before.TotalAlloc; grew >= budget {
		t.Errorf("Inventory() allocated %d MiB, want less than %d MiB: the resource cap must refuse the document before its resources are decoded",
			grew>>20, budget>>20)
	}
}

// TestInventoryAcceptsMaxResources pins the boundary of the resource cap:
// exactly MaxResources resources is accepted, so the cap rejects only what is
// more.
func TestInventoryAcceptsMaxResources(t *testing.T) {
	t.Parallel()

	path := writeFile(t, t.TempDir(), resourcesDocument(t, inventoryfile.MaxResources))

	inv, err := inventoryfile.New(path).Inventory(t.Context())
	if err != nil {
		t.Fatalf("Inventory() returned an error for exactly MaxResources (%d) resources: %v",
			inventoryfile.MaxResources, err)
	}

	if got := len(inv.Resources()); got != inventoryfile.MaxResources {
		t.Errorf("len(Resources()) = %d, want %d", got, inventoryfile.MaxResources)
	}
}

// TestInventoryAcceptsFileOfMaxSize pins the boundary of the size cap: exactly
// MaxFileSize bytes is accepted, so the cap rejects only what is larger.
func TestInventoryAcceptsFileOfMaxSize(t *testing.T) {
	t.Parallel()

	path := writeFile(t, t.TempDir(), paddedInventory(t, inventoryfile.MaxFileSize))

	inv, err := inventoryfile.New(path).Inventory(t.Context())
	if err != nil {
		t.Fatalf("Inventory() returned an error for a file of exactly MaxFileSize (%d) bytes: %v",
			inventoryfile.MaxFileSize, err)
	}

	if got := inv.Source(); got != "aws" {
		t.Errorf("Source() = %q, want %q", got, "aws")
	}
}

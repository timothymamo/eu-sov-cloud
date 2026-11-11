package inventoryfile

import (
	"errors"
	"fmt"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/internal/jsondoc"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// This file holds the in-memory mode's errors (FromBytes). A document held in
// memory may come from an HTTP client, and no value from it is ever repeated
// in an error: a refused value is named by its position and field,
// resources[1].category, and the error still wraps the sentinel the file
// reader's would. A file's errors (New) are unchanged: they quote the value, as
// a reader of their own file wants.

// valueFreeError says where a refused value is and what is wrong with it, by
// the sentinel's text alone, and unwraps to the error that refused it, so
// errors.Is and errors.As see the same sentinels as for a file.
type valueFreeError struct {
	msg string
	err error
}

func (e *valueFreeError) Error() string { return e.msg }

func (e *valueFreeError) Unwrap() error { return e.err }

// versionValueFree refuses a schemaVersion that is present and not the integer
// 1, naming its JSON type alone; a missing one is left to
// jsondoc.Document.Version, whose message names no value.
func versionValueFree(doc jsondoc.Document) error {
	raw, _, err := doc.Lookup(keySchemaVersion)
	if err != nil {
		return err
	}

	switch string(raw) {
	case "", "null", "1":
		return nil
	}

	return fmt.Errorf("%w: %s is %s, want the integer %d", ErrSchemaVersion, keySchemaVersion, jsonKind(raw[0]), schemaVersion)
}

// jsonKind names, with its article, the JSON type of the value whose first
// byte is b; a number is "a number other than 1", the one number it can be
// here.
func jsonKind(b byte) string {
	switch b {
	case '{':
		return "an object"
	case '[':
		return "an array"
	case '"':
		return "a string"
	case 't', 'f':
		return "a boolean"
	default:
		return "a number other than 1"
	}
}

// domainSentinels are the sentinels inventory.New wraps, most specific first.
// A valueFreeError says what is wrong by the first one its error wraps.
var domainSentinels = []error{
	inventory.ErrEmptySource,
	inventory.ErrEmptyID,
	inventory.ErrDuplicateID,
	taxonomy.ErrUnknownCategory,
	taxonomy.ErrUnknownCapability,
	text.ErrInvalid,
}

// newValueFree builds the inventory through inventory.New and, when the domain
// refuses it, returns a valueFreeError that places the refused value by
// position and field instead of the domain's message, which quotes it.
//
// The domain checks the source, then each resource in order, so the faulty
// resource is the last of the shortest prefix inventory.New refuses, found by
// binary search; its field is found by checking it alone. Only a refused
// document pays for this, with O(log n) more calls to inventory.New.
func newValueFree(doc document) (inventory.Inventory, error) {
	inv, err := inventory.New(doc.source, doc.resources)
	if err == nil {
		return inv, nil
	}

	if _, srcErr := inventory.New(doc.source, nil); srcErr != nil {
		return inventory.Inventory{}, valueFree(keySource, srcErr)
	}

	// The shortest refused prefix has n resources: inventory.New accepts
	// resources[:n-1] and refuses resources[:n].
	lo, hi := 0, len(doc.resources)
	for lo+1 < hi {
		mid := lo + (hi-lo)/2
		if _, prefixErr := inventory.New(doc.source, doc.resources[:mid]); prefixErr != nil {
			hi = mid
		} else {
			lo = mid
		}
	}

	i := hi - 1

	return inventory.Inventory{}, valueFree(resourceField(doc.source, i, doc.resources[i]), err)
}

// resourceField names the field of r, the resource at index i, that the domain
// refuses: the field of the first check r fails alone, or its id when r alone
// is valid and so repeats an earlier ID.
func resourceField(source string, i int, r inventory.Resource) string {
	at := fmt.Sprintf("%s[%d]", keyResources, i)

	_, err := inventory.New(source, []inventory.Resource{r})

	switch {
	case err == nil, errors.Is(err, inventory.ErrEmptyID):
		return at + "." + keyID
	case errors.Is(err, taxonomy.ErrUnknownCategory):
		return at + "." + keyCategory
	case errors.Is(err, taxonomy.ErrUnknownCapability):
		for j, c := range r.Capabilities {
			if _, capErr := taxonomy.ParseCapability(string(c)); capErr != nil {
				return fmt.Sprintf("%s.%s[%d]", at, keyCapabilities, j)
			}
		}

		return at + "." + keyCapabilities
	case text.Validate(r.RawType) != nil:
		return at + "." + keyRawType
	case text.Validate(r.Region) != nil:
		return at + "." + keyRegion
	default:
		return at + "." + keyID
	}
}

// valueFree wraps err, a refusal of the domain, in a valueFreeError that names
// field and the first of domainSentinels err wraps.
func valueFree(field string, err error) error {
	what := "invalid value"

	for _, sentinel := range domainSentinels {
		if errors.Is(err, sentinel) {
			what = sentinel.Error()

			break
		}
	}

	return &valueFreeError{msg: field + ": " + what, err: err}
}

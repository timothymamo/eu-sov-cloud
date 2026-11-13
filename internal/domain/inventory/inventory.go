package inventory

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	"github.com/timothymamo/shift2eu/internal/domain/text"
)

// Resource is one resource found in a source, classified into the taxonomy.
type Resource struct {
	// ID identifies the resource uniquely within its inventory, for example an
	// EC2 instance ID or an Azure resource ID. Its only space character is the
	// ASCII space, and two IDs that differ only in the length of their runs of
	// white space are the same ID.
	ID string
	// RawType is the source's own type name, for example AWS::EC2::Instance. It is
	// kept for traceability; mapping goes through Category only. It is empty when
	// the raw type is unknown, but never only whitespace.
	RawType string
	// Category is the taxonomy category the source classified the resource into.
	Category taxonomy.Category
	// Region is the source region the resource runs in, as the source names it.
	// It is empty for a global resource that is not bound to a region, but never
	// only whitespace.
	Region string
	// Capabilities lists the features the resource depends on. In an Inventory
	// they are sorted and free of duplicates, and nil when there are none.
	Capabilities []taxonomy.Capability

	// The fields below come with inventory format version 2 (ADR 0024). Each is
	// optional, and "not given" is never 0.

	// Size is how big the resource is, or nil when it is not given. In an
	// Inventory a size with no field given is nil.
	Size *Size
	// Usage is how much of a month the resource runs, or nil when it is not
	// given. In an Inventory a usage with no field given is nil.
	Usage *Usage
	// Workload names the application the resource belongs to, as free text. It
	// is empty when not given.
	Workload string
	// Relations point at other resources of the same inventory. In an Inventory
	// they are sorted by type then target, free of duplicates, and nil when
	// there are none.
	Relations []Relation
	// MonthlyCost is what the source charges for the resource in a month, or nil
	// when it is not given.
	MonthlyCost *Cost
}

// Inventory is a validated list of the resources found in one source. Build it
// with New; its contents cannot be changed afterwards. A zero Inventory has not
// been validated: later layers only ever receive values built by New.
type Inventory struct {
	source    string
	resources []Resource
}

var (
	// ErrEmptySource is returned for an inventory whose source is empty or only
	// whitespace.
	ErrEmptySource = errors.New("inventory has an empty source")
	// ErrEmptyID is returned for a resource whose ID is empty or only whitespace.
	ErrEmptyID = errors.New("resource has an empty ID")
	// ErrDuplicateID is returned when two resources share an ID, compared with
	// every run of white space treated as a single space.
	ErrDuplicateID = errors.New("duplicate resource ID")
)

// New validates resources and returns them as the inventory of source.
//
// source must not be blank. Every resource needs a unique ID that is not blank, a
// taxonomy category (Unclassified is valid; an unknown value is an error, never
// Unclassified) and capabilities from the taxonomy vocabulary. RawType and Region
// may be empty, for an unknown raw type and a global resource. source and each
// resource's ID, RawType and Region must be valid free text (text.Validate), so
// none of them starts or ends with white space or is only white space; they are
// stored as given, never trimmed.
//
// An ID is what a reader copies to find the resource, so it must not contain a
// Unicode space separator (category Zs) other than the ASCII space U+0020: a
// no-break or ideographic space looks like a space but cannot be retyped as
// one. IDs are unique when compared with every run of white space treated as a
// single space, so "i-0 abc" and "i-0  abc" are duplicates. Look-alike letters
// (homoglyphs) and Unicode normalisation (NFC and NFD) are not compared.
//
// The inventory holds its own copy of the resources, sorted by ID, with each
// resource's capabilities sorted, de-duplicated and nil when there are none.
//
// The version 2 fields are optional; a size, usage or relations list that gives
// nothing is stored as nil. A size or usage field must be one the category
// takes (ErrFieldNotForCategory): compute.vm, compute.baremetal,
// kubernetes.managed, database.postgresql and database.mysql take every one;
// storage.block takes storageGiB and count; storage.object takes storageGiB;
// serverless.function takes memoryGiB; every other category takes none. Each
// given number must be in its range (ErrInvalidSize, ErrInvalidUsage): vcpus
// and memoryGiB more than 0 and at most 1,000,000, storageGiB at least 0 and at
// most 1,000,000,000, count at least 1 and at most 100,000, hoursPerMonth more
// than 0 and at most 744. A count or hoursPerMonth that is not given is kept as
// not given: later milestones read them as 1 and 730. The workload is free text
// (text.Validate), or empty. A monthly cost needs an amount from 0 to
// 1,000,000,000,000 and a currency of three upper-case ASCII letters
// (ErrInvalidCost). A relation needs a type from the vocabulary
// (ParseRelationType) and a target that names another resource of the
// inventory (ErrUnknownRelationTarget, ErrSelfRelation), compared as IDs are;
// it is stored with the target's ID as that resource writes it, and the
// relations are de-duplicated and sorted by type then target. Relations may
// form cycles.
//
// Errors name the field and the resource at fault: by ID, or by position as
// resources[i] when the ID itself is at fault. A version 2 field is named by its
// path in the order the resources were given, resources[1].size.vcpus, as a
// *FieldError. Values echoed in errors are quoted with text.Quote, so errors
// stay short however long the input is.
func New(source string, resources []Resource) (Inventory, error) {
	if strings.TrimSpace(source) == "" {
		return Inventory{}, ErrEmptySource
	}

	if err := text.Validate(source); err != nil {
		return Inventory{}, fmt.Errorf("source: %w", err)
	}

	// seen holds the index of each resource by its ID with runs of white space
	// collapsed: the key duplicates and relation targets are compared by.
	seen := make(map[string]int, len(resources))
	normalised := make([]Resource, 0, len(resources))

	for i, r := range resources {
		if err := validateID(r.ID); err != nil {
			return Inventory{}, fmt.Errorf("resources[%d]: %w", i, err)
		}

		key := collapseSpace(r.ID)
		if _, dup := seen[key]; dup {
			return Inventory{}, fmt.Errorf("resources[%d]: %w %s", i, ErrDuplicateID, text.Quote(r.ID))
		}

		seen[key] = i

		n, err := normalise(r)
		if err != nil {
			return Inventory{}, fmt.Errorf("resource %s: %w", text.Quote(r.ID), err)
		}

		if n, err = checkVersion2(i, n); err != nil {
			return Inventory{}, err
		}

		normalised = append(normalised, n)
	}

	// A relation may point at any resource, so relations are resolved once
	// every ID is known, still in the order the resources were given.
	for i := range normalised {
		relations, err := resolveRelations(i, normalised[i], normalised, seen)
		if err != nil {
			return Inventory{}, err
		}

		normalised[i].Relations = relations
		normalised[i] = canonical(normalised[i])
	}

	slices.SortFunc(normalised, func(a, b Resource) int {
		return strings.Compare(a.ID, b.ID)
	})

	return Inventory{source: source, resources: normalised}, nil
}

// Source names the source the inventory was taken from, for example aws.
func (inv Inventory) Source() string {
	return inv.source
}

// Resources returns the resources sorted by ID. The result is a deep copy:
// changing it does not change the inventory.
func (inv Inventory) Resources() []Resource {
	out := make([]Resource, len(inv.resources))
	for i, r := range inv.resources {
		out[i] = clone(r)
	}

	return out
}

// validateID checks that id is not blank, is valid free text and has no space
// separator other than the ASCII space. The error for invalid text wraps
// text.ErrInvalid.
func validateID(id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrEmptyID
	}

	if err := text.Validate(id); err != nil {
		return fmt.Errorf("id: %w", err)
	}

	for i, r := range id {
		if r != ' ' && unicode.Is(unicode.Zs, r) {
			return fmt.Errorf("id: %w: non-ASCII space %U at byte %d", text.ErrInvalid, r, i)
		}
	}

	return nil
}

// collapseSpace returns s with every run of white space replaced by one ASCII
// space: the key under which two values that look the same are the same.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// normalise checks the free text, category and capabilities of a resource with a
// valid ID and returns a copy of it whose capabilities are sorted, de-duplicated
// and nil when there are none.
func normalise(r Resource) (Resource, error) {
	if err := text.Validate(r.RawType); err != nil {
		return Resource{}, fmt.Errorf("raw type: %w", err)
	}

	if err := text.Validate(r.Region); err != nil {
		return Resource{}, fmt.Errorf("region: %w", err)
	}

	if _, err := taxonomy.ParseCategory(string(r.Category)); err != nil {
		return Resource{}, err
	}

	for _, c := range r.Capabilities {
		if _, err := taxonomy.ParseCapability(string(c)); err != nil {
			return Resource{}, err
		}
	}

	r = clone(r)
	slices.Sort(r.Capabilities)
	r.Capabilities = slices.Compact(r.Capabilities)

	if len(r.Capabilities) == 0 {
		r.Capabilities = nil
	}

	return r, nil
}

// clone returns a copy of r that shares no memory with it.
func clone(r Resource) Resource {
	r.Capabilities = slices.Clone(r.Capabilities)
	r.Size = cloneSize(r.Size)
	r.Usage = cloneUsage(r.Usage)
	r.Relations = slices.Clone(r.Relations)
	r.MonthlyCost = cloneCost(r.MonthlyCost)

	return r
}

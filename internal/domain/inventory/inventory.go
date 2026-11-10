package inventory

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
)

// Resource is one resource found in a source, classified into the taxonomy.
type Resource struct {
	// ID identifies the resource uniquely within its inventory, for example an
	// EC2 instance ID or an Azure resource ID.
	ID string
	// RawType is the source's own type name, for example AWS::EC2::Instance. It is
	// kept for traceability; mapping goes through Category only.
	RawType string
	// Category is the taxonomy category the source classified the resource into.
	Category taxonomy.Category
	// Region is the source region the resource runs in, as the source names it.
	Region string
	// Capabilities lists the features the resource depends on. In an Inventory
	// they are sorted and free of duplicates.
	Capabilities []taxonomy.Capability
}

// Inventory is a validated list of the resources found in one source. Build it
// with New; its contents cannot be changed afterwards.
type Inventory struct {
	source    string
	resources []Resource
}

var (
	// ErrEmptyID is returned for a resource without an ID.
	ErrEmptyID = errors.New("resource has an empty ID")
	// ErrDuplicateID is returned when two resources share an ID.
	ErrDuplicateID = errors.New("duplicate resource ID")
)

// New validates resources and returns them as the inventory of source.
//
// Every resource needs a unique, non-empty ID, a taxonomy category (Unclassified
// is valid; an unknown value is an error, never Unclassified) and capabilities
// from the taxonomy vocabulary. The inventory holds its own copy of the
// resources, sorted by ID, with each resource's capabilities sorted and
// de-duplicated.
func New(source string, resources []Resource) (Inventory, error) {
	seen := make(map[string]struct{}, len(resources))
	normalised := make([]Resource, 0, len(resources))

	for i, r := range resources {
		if r.ID == "" {
			return Inventory{}, fmt.Errorf("resources[%d]: %w", i, ErrEmptyID)
		}

		if _, dup := seen[r.ID]; dup {
			return Inventory{}, fmt.Errorf("%w %q", ErrDuplicateID, r.ID)
		}

		seen[r.ID] = struct{}{}

		n, err := normalise(r)
		if err != nil {
			return Inventory{}, fmt.Errorf("resource %q: %w", r.ID, err)
		}

		normalised = append(normalised, n)
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

// normalise checks r's category and capabilities and returns a copy of r whose
// capabilities are sorted and de-duplicated.
func normalise(r Resource) (Resource, error) {
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

	return r, nil
}

// clone returns a copy of r that shares no memory with it.
func clone(r Resource) Resource {
	r.Capabilities = slices.Clone(r.Capabilities)

	return r
}

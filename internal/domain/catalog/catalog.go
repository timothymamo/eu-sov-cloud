package catalog

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
)

// Snapshot records where and when a catalog's data was taken, so a report can say
// how current it is.
type Snapshot struct {
	// SourceURL is the page or API the data was taken from.
	SourceURL string
	// RetrievedOn is when the data was retrieved.
	RetrievedOn time.Time
}

// Offering is one service a target sells, described in taxonomy terms.
type Offering struct {
	// Name is the target's name for the service, for example Kapsule. It is unique
	// within a catalog.
	Name string
	// Categories lists the taxonomy categories the offering serves. One offering
	// may serve several.
	Categories []taxonomy.Category
	// Capabilities lists the features the offering provides.
	Capabilities []taxonomy.Capability
	// Regions lists the EU regions the offering is available in, as the target
	// names them.
	Regions []string
}

// Has reports whether the offering provides capability c.
func (o Offering) Has(c taxonomy.Capability) bool {
	return slices.Contains(o.Capabilities, c)
}

// serves reports whether the offering serves category c.
func (o Offering) serves(c taxonomy.Category) bool {
	return slices.Contains(o.Categories, c)
}

// Catalog is a validated description of what one target offers, taken from a
// dated snapshot. Build it with New; its contents cannot be changed afterwards.
type Catalog struct {
	target    string
	snapshot  Snapshot
	offerings []Offering
}

var (
	// ErrInvalidCatalog is returned for a catalog without a target or a dated,
	// sourced snapshot, or with an offering that claims to serve
	// taxonomy.Unclassified.
	ErrInvalidCatalog = errors.New("invalid catalog")
	// ErrEmptyOfferingName is returned for an offering without a name.
	ErrEmptyOfferingName = errors.New("offering has an empty name")
	// ErrDuplicateOffering is returned when two offerings share a name.
	ErrDuplicateOffering = errors.New("duplicate offering name")
	// ErrNoCategory is returned for an offering that serves no category.
	ErrNoCategory = errors.New("offering serves no taxonomy category")
)

// New validates offerings and returns them as the catalog of target, taken from
// snapshot.
//
// target, snapshot.SourceURL and snapshot.RetrievedOn must be set. Every offering
// needs a unique, non-empty name and at least one category; its categories and
// capabilities must come from the taxonomy, and taxonomy.Unclassified is never a
// category an offering serves. The catalog holds its own copy of the offerings,
// sorted by name.
func New(target string, snapshot Snapshot, offerings []Offering) (Catalog, error) {
	switch {
	case target == "":
		return Catalog{}, fmt.Errorf("%w: empty target", ErrInvalidCatalog)
	case snapshot.SourceURL == "":
		return Catalog{}, fmt.Errorf("%w: snapshot has no source URL", ErrInvalidCatalog)
	case snapshot.RetrievedOn.IsZero():
		return Catalog{}, fmt.Errorf("%w: snapshot has no retrieval date", ErrInvalidCatalog)
	}

	seen := make(map[string]struct{}, len(offerings))
	validated := make([]Offering, 0, len(offerings))

	for i, o := range offerings {
		if o.Name == "" {
			return Catalog{}, fmt.Errorf("offerings[%d]: %w", i, ErrEmptyOfferingName)
		}

		if _, dup := seen[o.Name]; dup {
			return Catalog{}, fmt.Errorf("%w %q", ErrDuplicateOffering, o.Name)
		}

		seen[o.Name] = struct{}{}

		if err := validate(o); err != nil {
			return Catalog{}, fmt.Errorf("offering %q: %w", o.Name, err)
		}

		validated = append(validated, clone(o))
	}

	slices.SortFunc(validated, func(a, b Offering) int {
		return strings.Compare(a.Name, b.Name)
	})

	return Catalog{target: target, snapshot: snapshot, offerings: validated}, nil
}

// Target names the target the catalog describes, for example scaleway.
func (c Catalog) Target() string {
	return c.target
}

// Snapshot returns where and when the catalog's data was taken.
func (c Catalog) Snapshot() Snapshot {
	return c.snapshot
}

// Offerings returns every offering, sorted by name. The result is a deep copy:
// changing it does not change the catalog.
func (c Catalog) Offerings() []Offering {
	out := make([]Offering, len(c.offerings))
	for i, o := range c.offerings {
		out[i] = clone(o)
	}

	return out
}

// OfferingsFor returns the offerings that serve category, sorted by name, or nil
// if none does. The result is a deep copy: changing it does not change the
// catalog.
func (c Catalog) OfferingsFor(category taxonomy.Category) []Offering {
	var out []Offering

	for _, o := range c.offerings {
		if o.serves(category) {
			out = append(out, clone(o))
		}
	}

	return out
}

// validate checks the categories and capabilities of a named offering.
func validate(o Offering) error {
	if len(o.Categories) == 0 {
		return ErrNoCategory
	}

	for _, cat := range o.Categories {
		if _, err := taxonomy.ParseCategory(string(cat)); err != nil {
			return err
		}

		if cat == taxonomy.Unclassified {
			return fmt.Errorf("%w: an offering cannot serve %q", ErrInvalidCatalog, taxonomy.Unclassified)
		}
	}

	for _, capability := range o.Capabilities {
		if _, err := taxonomy.ParseCapability(string(capability)); err != nil {
			return err
		}
	}

	return nil
}

// clone returns a copy of o that shares no memory with it.
func clone(o Offering) Offering {
	o.Categories = slices.Clone(o.Categories)
	o.Capabilities = slices.Clone(o.Capabilities)
	o.Regions = slices.Clone(o.Regions)

	return o
}

package catalog

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// Snapshot records where and when a catalog's data was taken, so a report can say
// how current it is.
type Snapshot struct {
	// SourceURL is the page or API the data was taken from: an absolute https URL
	// with a host name, no user info, RFC 3986 characters only.
	SourceURL string
	// RetrievedOn is when the data was retrieved. In a Catalog it is in UTC, and
	// reporters format it as a UTC calendar date.
	RetrievedOn time.Time
}

// Offering is one service a target sells, described in taxonomy terms.
type Offering struct {
	// Name is the target's name for the service, for example Kapsule. It is unique
	// within a catalog.
	Name string
	// Categories lists the taxonomy categories the offering serves. One offering
	// may serve several. In a Catalog they are sorted and free of duplicates.
	Categories []taxonomy.Category
	// Capabilities lists the features the offering provides. In a Catalog they are
	// sorted and free of duplicates, and nil when there are none.
	Capabilities []taxonomy.Capability
	// Regions lists the EU regions the offering is available in, as the target
	// names them. In a Catalog they are sorted and free of duplicates.
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
// A zero Catalog has not been validated: later layers only ever receive values
// built by New.
type Catalog struct {
	target    string
	snapshot  Snapshot
	offerings []Offering
}

var (
	// ErrInvalidCatalog is returned for a catalog without a valid target or a
	// dated snapshot sourced from a valid source URL (see New), or with an
	// offering that claims to serve taxonomy.Unclassified.
	ErrInvalidCatalog = errors.New("invalid catalog")
	// ErrEmptyOfferingName is returned for an offering whose name is empty or only
	// whitespace.
	ErrEmptyOfferingName = errors.New("offering has an empty name")
	// ErrDuplicateOffering is returned when two offerings share a name.
	ErrDuplicateOffering = errors.New("duplicate offering name")
	// ErrNoCategory is returned for an offering that serves no category.
	ErrNoCategory = errors.New("offering serves no taxonomy category")
	// ErrNoRegion is returned for an offering that lists no region, or lists a
	// region whose name is empty or only whitespace.
	ErrNoRegion = errors.New("offering is available in no region")
)

// New validates offerings and returns them as the catalog of target, taken from
// snapshot.
//
// target is the target's human-readable display name, used as is in reports, for
// example Scaleway. It must not be blank and must be valid free text
// (text.Validate). snapshot.SourceURL must be valid free text and an absolute
// https URL with a host name, no user info, RFC 3986 characters only: unreserved
// characters, the reserved general and sub-delimiters, and '%' for valid
// percent-encoding. A query and a fragment are allowed; escaping the URL for an
// output format is the reporter's job. snapshot.RetrievedOn must be set; the
// catalog keeps it in UTC.
//
// Every offering needs a unique name that is not blank, at least one category and
// at least one region. Names and regions must be valid free text and are stored as
// given, never trimmed. Categories and capabilities must come from the taxonomy,
// and taxonomy.Unclassified is never a category an offering serves. The catalog
// holds its own copy of the offerings, sorted by name, with each offering's
// categories, capabilities and regions sorted, de-duplicated and nil when empty.
//
// Errors name the field and the offering at fault. Values echoed in errors are
// quoted with text.Quote, so errors stay short however long the input is. The
// source URL is never echoed: it may carry credentials.
func New(target string, snapshot Snapshot, offerings []Offering) (Catalog, error) {
	if err := validateTarget(target); err != nil {
		return Catalog{}, err
	}

	if err := validateSnapshot(snapshot); err != nil {
		return Catalog{}, err
	}

	snapshot.RetrievedOn = snapshot.RetrievedOn.UTC()

	seen := make(map[string]struct{}, len(offerings))
	normalised := make([]Offering, 0, len(offerings))

	for i, o := range offerings {
		if err := validateName(o.Name); err != nil {
			return Catalog{}, fmt.Errorf("offerings[%d]: %w", i, err)
		}

		if _, dup := seen[o.Name]; dup {
			return Catalog{}, fmt.Errorf("offerings[%d]: %w %s", i, ErrDuplicateOffering, text.Quote(o.Name))
		}

		seen[o.Name] = struct{}{}

		n, err := normalise(o)
		if err != nil {
			return Catalog{}, fmt.Errorf("offering %s: %w", text.Quote(o.Name), err)
		}

		normalised = append(normalised, n)
	}

	slices.SortFunc(normalised, func(a, b Offering) int {
		return strings.Compare(a.Name, b.Name)
	})

	return Catalog{target: target, snapshot: snapshot, offerings: normalised}, nil
}

// Target is the human-readable display name of the target the catalog
// describes, as reports show it, for example Scaleway.
func (c Catalog) Target() string {
	return c.target
}

// Snapshot returns where and when the catalog's data was taken. RetrievedOn is
// in UTC.
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

// validateTarget checks that target is valid free text and not blank.
func validateTarget(target string) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("%w: empty target", ErrInvalidCatalog)
	}

	if err := text.Validate(target); err != nil {
		return fmt.Errorf("%w: target: %w", ErrInvalidCatalog, err)
	}

	return nil
}

// validateSnapshot checks that snapshot is dated and has a valid source URL.
func validateSnapshot(snapshot Snapshot) error {
	if err := validateSourceURL(snapshot.SourceURL); err != nil {
		return fmt.Errorf("%w: snapshot source URL: %w", ErrInvalidCatalog, err)
	}

	if snapshot.RetrievedOn.IsZero() {
		return fmt.Errorf("%w: snapshot has no retrieval date", ErrInvalidCatalog)
	}

	return nil
}

// errBadSourceURL is the reason given for a source URL that is valid free text
// but not an acceptable URL. The URL itself is never echoed: it may carry
// credentials.
var errBadSourceURL = errors.New(
	"not an absolute https URL with a host name, no user info, RFC 3986 characters only")

// validateSourceURL checks that raw is valid free text and an absolute https URL
// with a host name, no user info and only RFC 3986 characters.
func validateSourceURL(raw string) error {
	if raw == "" {
		return errors.New("missing")
	}

	if err := text.Validate(raw); err != nil {
		return err
	}

	for i, r := range raw {
		if !isURLChar(r) {
			return fmt.Errorf("%w: disallowed character %U at byte %d", errBadSourceURL, r, i)
		}
	}

	// The parse error is not wrapped: it echoes the whole URL.
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Opaque != "" || u.User != nil || u.Hostname() == "" {
		return errBadSourceURL
	}

	return nil
}

// isURLChar reports whether r may appear as given in a URL (RFC 3986): an
// ASCII unreserved character, a general or sub-delimiter, or the '%' that
// starts a percent-encoded byte.
func isURLChar(r rune) bool {
	switch {
	case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9':
		return true
	default:
		return strings.ContainsRune("-._~:/?#[]@!$&'()*+,;=%", r)
	}
}

// validateName checks that an offering name is valid free text and not blank.
func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrEmptyOfferingName
	}

	if err := text.Validate(name); err != nil {
		return fmt.Errorf("name: %w", err)
	}

	return nil
}

// normalise checks the categories, capabilities and regions of an offering with
// a valid name and returns a copy of it whose slices are sorted, de-duplicated
// and nil when empty.
func normalise(o Offering) (Offering, error) {
	if len(o.Categories) == 0 {
		return Offering{}, ErrNoCategory
	}

	for _, cat := range o.Categories {
		if _, err := taxonomy.ParseCategory(string(cat)); err != nil {
			return Offering{}, err
		}

		if cat == taxonomy.Unclassified {
			return Offering{}, fmt.Errorf("%w: an offering cannot serve %s",
				ErrInvalidCatalog, text.Quote(string(taxonomy.Unclassified)))
		}
	}

	for _, capability := range o.Capabilities {
		if _, err := taxonomy.ParseCapability(string(capability)); err != nil {
			return Offering{}, err
		}
	}

	if len(o.Regions) == 0 {
		return Offering{}, ErrNoRegion
	}

	for i, region := range o.Regions {
		if strings.TrimSpace(region) == "" {
			return Offering{}, fmt.Errorf("regions[%d]: %w", i, ErrNoRegion)
		}

		if err := text.Validate(region); err != nil {
			return Offering{}, fmt.Errorf("regions[%d]: %w", i, err)
		}
	}

	o = clone(o)
	o.Categories = sortedUnique(o.Categories)
	o.Capabilities = sortedUnique(o.Capabilities)
	o.Regions = sortedUnique(o.Regions)

	return o, nil
}

// sortedUnique sorts s in place and removes duplicates, returning nil when
// nothing is left.
func sortedUnique[T ~string](s []T) []T {
	slices.Sort(s)

	s = slices.Compact(s)
	if len(s) == 0 {
		return nil
	}

	return s
}

// clone returns a copy of o that shares no memory with it.
func clone(o Offering) Offering {
	o.Categories = slices.Clone(o.Categories)
	o.Capabilities = slices.Clone(o.Capabilities)
	o.Regions = slices.Clone(o.Regions)

	return o
}

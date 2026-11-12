package scaleway

import (
	"context"
	_ "embed" // the catalog snapshot is embedded with //go:embed
	"fmt"

	"github.com/timothymamo/shift2eu/internal/adapter/target/snapshot"
	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/text"
)

// embeddedName is the name of the embedded catalog file, as errors show it.
const embeddedName = "catalog.json"

// embedded is the dated catalog snapshot the adapter serves. SOURCES.md records
// where its data comes from.
//
//go:embed catalog.json
var embedded []byte

// Source serves the Scaleway catalog from the embedded snapshot. It implements
// the analysis.CatalogSource port.
type Source struct{}

// New returns a Source that serves the embedded Scaleway catalog snapshot.
func New() *Source {
	return &Source{}
}

// Catalog returns the catalog the embedded snapshot describes, read through
// snapshot.Parse. Every call returns the same catalog.
//
// A cancelled ctx is reported before the snapshot is read. Every error is one
// line that names the embedded file and wraps ctx.Err() or an error of
// snapshot.Parse.
func (s *Source) Catalog(ctx context.Context) (catalog.Catalog, error) {
	if err := ctx.Err(); err != nil {
		return catalog.Catalog{}, fmt.Errorf("embedded Scaleway catalog %s: %w", text.Quote(embeddedName), err)
	}

	doc, err := snapshot.Parse(embedded)
	if err != nil {
		return catalog.Catalog{}, fmt.Errorf("embedded Scaleway catalog %s: %w", text.Quote(embeddedName), err)
	}

	return doc.Catalog, nil
}

// Regions returns the EU regions the embedded snapshot declares, in the order
// the file lists them. Every offering of the catalog is available only in these
// regions. The result is the caller's own copy.
//
// It returns nil if the embedded snapshot does not parse, which the package's
// tests rule out; Catalog reports why.
func Regions() []string {
	doc, err := snapshot.Parse(embedded)
	if err != nil {
		return nil
	}

	return doc.Regions
}

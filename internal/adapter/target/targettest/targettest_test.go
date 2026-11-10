package targettest_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/targettest"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/testutil"
)

// scalewayTarget is the target display name the fixture catalog carries.
const scalewayTarget = "Scaleway"

// scalewayExpectations says what the Scaleway adapter must deliver for the
// shared fixture: its display name, its EU regions and the categories the
// catalog has to serve.
func scalewayExpectations() targettest.Expectations {
	return targettest.Expectations{
		Target:  scalewayTarget,
		Regions: []string{"fr-par", "nl-ams", "pl-waw"},
		Categories: []taxonomy.Category{
			taxonomy.ComputeVM,
			taxonomy.DatabaseMySQL,
			taxonomy.DatabasePostgreSQL,
			taxonomy.KubernetesManaged,
			taxonomy.StorageObject,
		},
	}
}

// newCatalog builds the only kind of catalog an adapter can return: one that
// went through catalog.New.
func newCatalog(t *testing.T, target string, offerings []catalog.Offering) catalog.Catalog {
	t.Helper()

	cat, err := catalog.New(target, testutil.ScalewaySnapshot(), offerings)
	if err != nil {
		t.Fatalf("catalog.New() returned unexpected error: %v", err)
	}

	return cat
}

// computeOffering is one minimal, valid offering, for catalogs a test breaks on
// purpose.
func computeOffering(name string, regions ...string) catalog.Offering {
	return catalog.Offering{
		Name:       name,
		Categories: []taxonomy.Category{taxonomy.ComputeVM},
		Regions:    regions,
	}
}

// fakeTarget is an in-memory CatalogSource whose answers a test scripts. It
// returns catalogs in call order, repeating the last one, and records how often
// it was called. Unless it ignores the context, it returns ctx.Err() once the
// context is done, like a well-behaved adapter.
type fakeTarget struct {
	catalogs      []catalog.Catalog
	ignoreContext bool
	calls         int
}

func (f *fakeTarget) Catalog(ctx context.Context) (catalog.Catalog, error) {
	f.calls++

	if !f.ignoreContext {
		if err := ctx.Err(); err != nil {
			return catalog.Catalog{}, err
		}
	}

	if len(f.catalogs) == 0 {
		return catalog.Catalog{}, nil
	}

	i := min(f.calls-1, len(f.catalogs)-1)

	return f.catalogs[i], nil
}

// mentions reports whether err says at least one of keywords. The suite's exact
// wording is not part of the contract; that every violation names its check is.
func mentions(err error, keywords ...string) bool {
	msg := err.Error()

	return slices.ContainsFunc(keywords, func(keyword string) bool {
		return strings.Contains(msg, keyword)
	})
}

// TestCheck pins the first checks of the target contract suite: the catalog
// describes the expected target, is a real snapshot, stays inside the target's EU
// regions, serves every expected category, is stable and honours cancellation.
func TestCheck(t *testing.T) {
	t.Parallel()

	scaleway := testutil.ScalewayCatalog(t)
	want := scalewayExpectations()

	// wantMentions is a list of alternatives: the violation must contain at
	// least one keyword of every group.
	tests := []struct {
		name         string
		tgt          *fakeTarget
		wantErr      bool
		wantMentions [][]string
		wantCancel   bool
	}{
		{
			name: "conforming target",
			tgt:  &fakeTarget{catalogs: []catalog.Catalog{scaleway}},
		},
		{
			name:         "catalog for another target",
			tgt:          &fakeTarget{catalogs: []catalog.Catalog{newCatalog(t, "OVHcloud", testutil.ScalewayOfferings())}},
			wantErr:      true,
			wantMentions: [][]string{{"target"}},
		},
		{
			name:         "zero catalog",
			tgt:          &fakeTarget{catalogs: []catalog.Catalog{{}}},
			wantErr:      true,
			wantMentions: [][]string{{"snapshot", "source URL", "SourceURL", "retrieved", "RetrievedOn"}},
		},
		{
			name: "offering outside the EU regions",
			tgt: &fakeTarget{catalogs: []catalog.Catalog{
				newCatalog(t, scalewayTarget, []catalog.Offering{computeOffering("Instances", "us-east-1")}),
			}},
			wantErr:      true,
			wantMentions: [][]string{{"us-east-1"}, {"Instances"}},
		},
		{
			name: "expected category not served",
			tgt: &fakeTarget{catalogs: []catalog.Catalog{
				newCatalog(t, scalewayTarget, []catalog.Offering{
					computeOffering("Instances", "fr-par"),
					{
						Name:       "Object Storage",
						Categories: []taxonomy.Category{taxonomy.StorageObject},
						Regions:    []string{"fr-par"},
					},
					{
						Name:       "Kapsule",
						Categories: []taxonomy.Category{taxonomy.KubernetesManaged},
						Regions:    []string{"fr-par"},
					},
					{
						Name:       "Managed Database for PostgreSQL",
						Categories: []taxonomy.Category{taxonomy.DatabasePostgreSQL},
						Regions:    []string{"fr-par"},
					},
				}),
			}},
			wantErr:      true,
			wantMentions: [][]string{{"database.mysql"}},
		},
		{
			name: "changes its answer between calls",
			tgt: &fakeTarget{catalogs: []catalog.Catalog{
				scaleway,
				newCatalog(t, scalewayTarget, []catalog.Offering{computeOffering("Instances", "fr-par")}),
			}},
			wantErr:      true,
			wantMentions: [][]string{{"repeat", "consecutive", "same", "equal", "stable", "state"}},
		},
		{
			name:         "ignores a cancelled context",
			tgt:          &fakeTarget{catalogs: []catalog.Catalog{scaleway}, ignoreContext: true},
			wantErr:      true,
			wantMentions: [][]string{{"cancel", "context"}},
			wantCancel:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := targettest.Check(t.Context(), tt.tgt, want)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Check() returned unexpected error: %v", err)
				}

				if tt.tgt.calls < 2 {
					t.Errorf("Check() called Catalog() %d times, want at least 2 to check stability", tt.tgt.calls)
				}

				return
			}

			if err == nil {
				t.Fatal("Check() returned nil, want an error naming the violated check")
			}

			for i, group := range tt.wantMentions {
				if !mentions(err, group...) {
					t.Errorf("Check() error = %q, want it to mention violation %d (one of %v)", err, i+1, group)
				}
			}

			if got := errors.Is(err, context.Canceled); got != tt.wantCancel {
				t.Errorf("Check() error wraps context.Canceled = %t, want %t (error: %v)", got, tt.wantCancel, err)
			}
		})
	}
}

// TestRun pins that a conforming target passes the suite.
func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("conforming target", func(t *testing.T) {
		t.Parallel()

		targettest.Run(t, &fakeTarget{catalogs: []catalog.Catalog{testutil.ScalewayCatalog(t)}}, scalewayExpectations())
	})
}

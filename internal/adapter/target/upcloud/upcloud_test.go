// Package upcloud_test pins the UpCloud catalog adapter: the target contract in
// internal/adapter/target/targettest, the snapshot header the catalog reports, a
// golden change detector for the parsed catalog, the declared EU zones, that a
// cancelled run reads nothing, that no error of one target names another, and the
// verdicts PRD 77 asks of the data: categories UpCloud does not serve are a
// blocker, no offering claims zone-level high availability, and every capability
// that holds only on some plans lives on a tier variant named after that plan,
// with the base offering not listing it (ADR 0011). The snapshot document format
// itself, its strictness, its errors and the SOURCES.md rule are pinned once, in
// the shared reader's own tests (internal/adapter/target/snapshot). It is
// black-box: only the package's exported API is used.
package upcloud_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/snapshot"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/targettest"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/upcloud"
	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
	"github.com/timothymamo/eu-sov-cloud/internal/testutil"
)

// The adapter implements the catalog source port: pin that at compile time.
var _ analysis.CatalogSource = upcloud.New()

// The catalog the adapter embeds, and the golden file its parsed form renders to.
// Both are relative to this package's directory.
const (
	catalogFile = "catalog.json"
	goldenFile  = "testdata/catalog.golden"
)

// wantRegions is the EU zone list catalog.json declares, written out here so the
// test cannot agree with a wrong declaration by construction: the contract
// expectations below and TestRegions compare against this list, never against
// what Regions() returns. A zone is one UpCloud data centre, and PRD 77 keeps the
// catalog to zones in EU member states, in the order the file lists them.
func wantRegions() []string {
	return []string{"de-fra1", "dk-cph1", "es-mad1", "fi-hel1", "fi-hel2", "nl-ams1", "pl-waw1", "se-sto1"}
}

// nonEUZones are UpCloud zones outside the EU: London and Stavanger, which PRD 77
// leaves out with the United Kingdom and Norway, and the Americas and
// Asia-Pacific. The guard is readable on purpose: it names what a refresh must
// not add.
var nonEUZones = []string{"uk-lon1", "no-svg1", "us-nyc1", "us-chi1", "us-sjo1", "sg-sin1", "au-syd1"}

// objectStorageZones are the zones that host one of UpCloud's Managed Object
// Storage regions: EUROPE-1 in fi-hel2, EUROPE-2 in de-fra1, EUROPE-3 in se-sto1
// and EUROPE-4 in dk-cph1. The snapshot places Object Storage by the zone hosting
// each region, the way PRD 77 decides, so it is the one offering that is not in
// every declared zone.
func objectStorageZones() []string {
	return []string{"de-fra1", "dk-cph1", "fi-hel2", "se-sto1"}
}

// TestCatalogContract runs the target contract suite every target adapter must
// pass: the display name, every offering only in a declared EU zone, the six
// categories PRD 77 requires, the same catalog on every call, and cancellation.
func TestCatalogContract(t *testing.T) {
	t.Parallel()

	targettest.Run(t, upcloud.New(), targettest.Expectations{
		Target:  "UpCloud",
		Regions: wantRegions(),
		Categories: []taxonomy.Category{
			taxonomy.ComputeVM,
			taxonomy.KubernetesManaged,
			taxonomy.StorageObject,
			taxonomy.StorageBlock,
			taxonomy.DatabasePostgreSQL,
			taxonomy.DatabaseMySQL,
		},
	})
}

// catalogOf returns the catalog a fresh source serves.
func catalogOf(t *testing.T) catalog.Catalog {
	t.Helper()

	cat, err := upcloud.New().Catalog(t.Context())
	if err != nil {
		t.Fatalf("Catalog() returned unexpected error: %v", err)
	}

	return cat
}

// TestCatalogSnapshotHeader pins that the snapshot comes from the file header: the
// documentation root the data was compiled from, and its retrieval date as UTC
// midnight. A report states both, so neither may be invented by the adapter.
func TestCatalogSnapshotHeader(t *testing.T) {
	t.Parallel()

	snap := catalogOf(t).Snapshot()

	const wantSourceURL = "https://upcloud.com/docs/"
	if snap.SourceURL != wantSourceURL {
		t.Errorf("Snapshot().SourceURL = %s, want %s", text.Quote(snap.SourceURL), text.Quote(wantSourceURL))
	}

	wantRetrievedOn := time.Date(2026, time.November, 11, 0, 0, 0, 0, time.UTC)
	if !snap.RetrievedOn.Equal(wantRetrievedOn) {
		t.Errorf("Snapshot().RetrievedOn = %s, want %s",
			snap.RetrievedOn.Format(time.RFC3339), wantRetrievedOn.Format(time.RFC3339))
	}

	if got := snap.RetrievedOn.Location(); got != time.UTC {
		t.Errorf("Snapshot().RetrievedOn location = %v, want time.UTC", got)
	}
}

// TestCatalogGolden pins the whole parsed catalog against a golden file: every
// offering with its categories, capabilities and zones, in the order and canonical
// form the domain gives them. It is a change detector for the snapshot data and
// for the parsing, so a refresh of catalog.json has to update this file in the
// same review. The golden file is generated with -update once catalog.json
// exists; it is not hand-written, so a wrong snapshot cannot quietly become the
// spec.
func TestCatalogGolden(t *testing.T) {
	t.Parallel()

	cat := catalogOf(t)

	testutil.Golden(t, goldenFile, dump(cat))

	// The embed and the shared reader must agree: the golden file is the
	// specification of both, and the analysis may reach the catalog either way.
	data, err := os.ReadFile(catalogFile)
	if err != nil {
		t.Fatalf("reading the embedded catalog %s: %v", text.Quote(catalogFile), err)
	}

	parsed, err := snapshot.Parse(data)
	if err != nil {
		t.Fatalf("snapshot.Parse() of %s returned unexpected error: %v", text.Quote(catalogFile), err)
	}

	if diff := cmp.Diff(string(dump(cat)), string(dump(parsed.Catalog))); diff != "" {
		t.Errorf("snapshot.Parse() of the embedded file differs from Catalog() (-Catalog +Parse):\n%s", diff)
	}
}

// dump renders cat as the golden file's lines: a header with where and when the
// data was taken, then one line per offering, sorted by name, with every list
// comma-separated and an empty list shown as "-".
func dump(cat catalog.Catalog) []byte {
	var b strings.Builder

	snap := cat.Snapshot()

	fmt.Fprintf(&b, "target: %s\n", cat.Target())
	fmt.Fprintf(&b, "sourceURL: %s\n", snap.SourceURL)
	fmt.Fprintf(&b, "retrievedOn: %s\n", snap.RetrievedOn.UTC().Format(time.DateOnly))

	for _, o := range cat.Offerings() {
		fmt.Fprintf(&b, "%s | %s | %s | %s\n",
			o.Name, join(o.Categories), join(o.Capabilities), join(o.Regions))
	}

	return []byte(b.String())
}

// join formats values as a comma-separated list, or "-" when there are none.
func join[S ~string](values []S) string {
	if len(values) == 0 {
		return "-"
	}

	strs := make([]string, len(values))
	for i, v := range values {
		strs[i] = string(v)
	}

	return strings.Join(strs, ", ")
}

// TestRegions pins the declared EU zone list the reviewer checks against
// UpCloud's locations page, that callers get a copy, and that no offering is
// placed outside that list: no offering lists London, Stavanger or any zone in
// the Americas or Asia-Pacific. It also pins Managed Object Storage to the four
// zones that host one of its regions, the one offering that is not in every zone.
func TestRegions(t *testing.T) {
	t.Parallel()

	want := wantRegions()

	got := upcloud.Regions()
	if !slices.Equal(got, want) {
		t.Fatalf("Regions() = %v, want %v", got, want)
	}

	for _, zone := range got {
		if slices.Contains(nonEUZones, zone) {
			t.Errorf("Regions() declares %s, an UpCloud zone outside the EU: PRD 77 keeps the catalog to EU member states",
				text.Quote(zone))
		}
	}

	for i := range got {
		got[i] = "tampered"
	}

	if again := upcloud.Regions(); !slices.Equal(again, want) {
		t.Errorf("Regions() after changing the returned slice = %v, want %v: the result is not a copy", again, want)
	}

	cat := catalogOf(t)

	for _, o := range cat.Offerings() {
		for _, zone := range o.Regions {
			if !slices.Contains(want, zone) {
				t.Errorf("offering %s is available in zone %s, want one of the declared EU zones %v",
					text.Quote(o.Name), text.Quote(zone), want)
			}
		}
	}

	o := offeringNamed(t, cat, objectStorageName)

	if diff := cmp.Diff(objectStorageZones(), o.Regions); diff != "" {
		t.Errorf("offering %s zones mismatch (-want +got):\n%s", text.Quote(o.Name), diff)
	}
}

// objectStorageName is the one offering PRD 77 places by the zone that hosts each
// of its regions rather than in every zone.
const objectStorageName = "Managed Object Storage"

// TestNoOfferingForUnservedCategories pins PRD 77's criterion that a resource of a
// category UpCloud does not serve gets fit none, the target-neutral reason naming
// UpCloud and the category, and is listed as a blocker. The snapshot serves
// neither serverless functions nor message queues: UpCloud's documentation has no
// FaaS or queue product.
func TestNoOfferingForUnservedCategories(t *testing.T) {
	t.Parallel()

	cat := catalogOf(t)

	// PRD 77 expects no generally available UpCloud offering to serve these. A
	// change that adds one updates the PRD with the snapshot, so this test fails
	// first and the review sees it.
	unserved := []taxonomy.Category{taxonomy.ServerlessFunction, taxonomy.MessagingQueue}

	for _, o := range cat.Offerings() {
		for _, category := range o.Categories {
			if slices.Contains(unserved, category) {
				t.Errorf("offering %s serves %s, want no UpCloud offering to serve it",
					text.Quote(o.Name), text.Quote(string(category)))
			}
		}
	}

	for _, category := range unserved {
		t.Run(string(category), func(t *testing.T) {
			t.Parallel()

			r := newResource(t, "res-example", category)

			got := mapping.Map(r, cat)
			if got.Fit != mapping.None {
				t.Errorf("Map() fit = %q, want %q: UpCloud serves no %s",
					got.Fit, mapping.None, text.Quote(string(category)))
			}

			wantReasons := []mapping.Reason{{Code: mapping.NoOffering, Target: "UpCloud", Category: category}}
			if diff := cmp.Diff(wantReasons, got.Reasons); diff != "" {
				t.Errorf("Map() reasons mismatch (-want +got):\n%s", diff)
			}

			wantSentence := "no offering in the UpCloud catalog for " + string(category)
			if reasons := got.Reasons; len(reasons) == 1 && reasons[0].String() != wantSentence {
				t.Errorf("reason = %q, want %q", reasons[0].String(), wantSentence)
			}

			inv, err := inventory.New("aws", []inventory.Resource{r})
			if err != nil {
				t.Fatalf("inventory.New() returned unexpected error: %v", err)
			}

			gotBlockers := assessment.Assess(inv, cat).Blockers
			wantBlockers := []assessment.Blocker{{ResourceID: r.ID, Reason: wantReasons[0]}}
			if diff := cmp.Diff(wantBlockers, gotBlockers); diff != "" {
				t.Errorf("Assess() blockers mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestNoOfferingClaimsZoneHighAvailability pins PRD 77's criterion that no
// offering lists zone-high-availability unless its SOURCES.md section quotes
// documentation that states the service keeps serving through the loss of a whole
// zone or data centre without manual failover (ADR 0011's target rule). Each
// UpCloud zone is one data centre, so two nodes in one zone survive the loss of a
// node, not of the zone, and the list is empty. When a refresh makes it non-empty,
// this test fails first and the review sees the documentation that justifies it.
func TestNoOfferingClaimsZoneHighAvailability(t *testing.T) {
	t.Parallel()

	cat := catalogOf(t)

	var claiming []string

	for _, o := range cat.Offerings() {
		if o.Has(taxonomy.ZoneHighAvailability) {
			claiming = append(claiming, o.Name)
		}
	}

	if diff := cmp.Diff([]string(nil), claiming); diff != "" {
		t.Errorf("offerings claiming %s mismatch (-want +got):\n%s",
			text.Quote(string(taxonomy.ZoneHighAvailability)), diff)
	}
}

// newResource builds r the way an inventory holds it, so the domain gets the
// canonical form: an inventory validates and normalises its resources, and
// mapping.Map expects one that went through it.
func newResource(t *testing.T, id string, category taxonomy.Category, capabilities ...taxonomy.Capability) inventory.Resource {
	t.Helper()

	r := inventory.Resource{
		ID:           id,
		RawType:      "AWS::Example::Resource",
		Category:     category,
		Region:       "eu-west-1",
		Capabilities: capabilities,
	}

	inv, err := inventory.New("aws", []inventory.Resource{r})
	if err != nil {
		t.Fatalf("inventory.New() returned unexpected error: %v", err)
	}

	return inv.Resources()[0]
}

// candidateExpectation is what the tests below pin about one candidate: its
// name, its fit, and the capabilities it lacks. Its fields are exported because
// cmp cannot compare a struct with unexported ones.
type candidateExpectation struct {
	Name    string
	Fit     mapping.Fit
	Missing []taxonomy.Capability
}

// candidatesOf lists the candidates a mapping returned, best first. Ties are not
// a field of mapping's answer: two candidates tie when they have the same fit and
// the same number of missing capabilities, and mapping lists them in offering
// name order. Pinning the whole ordered list therefore pins both the ranking and
// the ties, which is what points a resource at a base offering when its variant
// adds nothing the resource needs.
func candidatesOf(candidates []mapping.Candidate) []candidateExpectation {
	out := make([]candidateExpectation, len(candidates))
	for i, c := range candidates {
		out[i] = candidateExpectation{Name: c.Offering.Name, Fit: c.Fit, Missing: c.Missing}
	}

	return out
}

// checkMapping fails unless got has the wanted fit and exactly the wanted
// candidates, in order.
func checkMapping(t *testing.T, got mapping.Result, wantFit mapping.Fit, want []candidateExpectation) {
	t.Helper()

	r := got.Resource

	if got.Fit != wantFit {
		t.Errorf("Map() of %s with capabilities %v: fit = %q, want %q",
			text.Quote(string(r.Category)), r.Capabilities, got.Fit, wantFit)
	}

	if diff := cmp.Diff(want, candidatesOf(got.Candidates)); diff != "" {
		t.Errorf("Map() of %s with capabilities %v: candidates mismatch (-want +got):\n%s",
			text.Quote(string(r.Category)), r.Capabilities, diff)
	}
}

// TestTierVariantIsBestOffering pins ADR 0011's tier-variant rule for UpCloud: a
// capability that holds only on some plans is listed on a variant named after that
// plan, and the base offering does not list it. A resource that needs the
// capability is pointed at the variant; one that needs nothing the variant adds is
// pointed at the base offering, which wins the tie by name, and the variant ties
// behind it. A resource that needs zone level is partial on both, because neither
// survives the loss of a data centre, and the base is named.
func TestTierVariantIsBestOffering(t *testing.T) {
	t.Parallel()

	cat := catalogOf(t)

	tests := []struct {
		name         string
		category     taxonomy.Category
		capabilities []taxonomy.Capability
		wantFit      mapping.Fit
		want         []candidateExpectation
	}{
		{
			name:     "block storage needs nothing",
			category: taxonomy.StorageBlock,
			wantFit:  mapping.Direct,
			want: []candidateExpectation{
				{Name: "Block Storage", Fit: mapping.Direct},
				{Name: "Block Storage Encrypted", Fit: mapping.Direct},
			},
		},
		{
			name:         "block storage needs encryption at rest",
			category:     taxonomy.StorageBlock,
			capabilities: []taxonomy.Capability{taxonomy.EncryptionAtRest},
			wantFit:      mapping.Direct,
			want: []candidateExpectation{
				{Name: "Block Storage Encrypted", Fit: mapping.Direct},
				{Name: "Block Storage", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.EncryptionAtRest}},
			},
		},
		{
			name:     "kubernetes needs nothing",
			category: taxonomy.KubernetesManaged,
			wantFit:  mapping.Direct,
			want: []candidateExpectation{
				{Name: "Managed Kubernetes", Fit: mapping.Direct},
				{Name: "Managed Kubernetes Production", Fit: mapping.Direct},
			},
		},
		{
			name:         "kubernetes needs node-level high availability",
			category:     taxonomy.KubernetesManaged,
			capabilities: []taxonomy.Capability{taxonomy.NodeHighAvailability},
			wantFit:      mapping.Direct,
			want: []candidateExpectation{
				{Name: "Managed Kubernetes Production", Fit: mapping.Direct},
				{Name: "Managed Kubernetes", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.NodeHighAvailability}},
			},
		},
		{
			name:         "kubernetes needs autoscaling, which no tier lists",
			category:     taxonomy.KubernetesManaged,
			capabilities: []taxonomy.Capability{taxonomy.Autoscaling},
			wantFit:      mapping.Partial,
			want: []candidateExpectation{
				{Name: "Managed Kubernetes", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.Autoscaling}},
				{Name: "Managed Kubernetes Production", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.Autoscaling}},
			},
		},
		{
			name:     "postgresql needs nothing",
			category: taxonomy.DatabasePostgreSQL,
			wantFit:  mapping.Direct,
			want: []candidateExpectation{
				{Name: "Managed PostgreSQL", Fit: mapping.Direct},
				{Name: "Managed PostgreSQL Multi-node", Fit: mapping.Direct},
			},
		},
		{
			name:         "postgresql needs node-level high availability",
			category:     taxonomy.DatabasePostgreSQL,
			capabilities: []taxonomy.Capability{taxonomy.NodeHighAvailability},
			wantFit:      mapping.Direct,
			want: []candidateExpectation{
				{Name: "Managed PostgreSQL Multi-node", Fit: mapping.Direct},
				{Name: "Managed PostgreSQL", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.NodeHighAvailability}},
			},
		},
		{
			name:         "postgresql needs read replicas",
			category:     taxonomy.DatabasePostgreSQL,
			capabilities: []taxonomy.Capability{taxonomy.ReadReplicas},
			wantFit:      mapping.Direct,
			want: []candidateExpectation{
				{Name: "Managed PostgreSQL Multi-node", Fit: mapping.Direct},
				{Name: "Managed PostgreSQL", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ReadReplicas}},
			},
		},
		{
			name:         "postgresql needs zone-level high availability",
			category:     taxonomy.DatabasePostgreSQL,
			capabilities: []taxonomy.Capability{taxonomy.ZoneHighAvailability},
			wantFit:      mapping.Partial,
			want: []candidateExpectation{
				{Name: "Managed PostgreSQL", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ZoneHighAvailability}},
				{Name: "Managed PostgreSQL Multi-node", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ZoneHighAvailability}},
			},
		},
		{
			name:         "postgresql needs zone-level high availability and read replicas",
			category:     taxonomy.DatabasePostgreSQL,
			capabilities: []taxonomy.Capability{taxonomy.ZoneHighAvailability, taxonomy.ReadReplicas},
			wantFit:      mapping.Partial,
			want: []candidateExpectation{
				{Name: "Managed PostgreSQL Multi-node", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ZoneHighAvailability}},
				{Name: "Managed PostgreSQL", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ReadReplicas, taxonomy.ZoneHighAvailability}},
			},
		},
		{
			name:     "mysql needs nothing",
			category: taxonomy.DatabaseMySQL,
			wantFit:  mapping.Direct,
			want: []candidateExpectation{
				{Name: "Managed MySQL", Fit: mapping.Direct},
				{Name: "Managed MySQL Multi-node", Fit: mapping.Direct},
			},
		},
		{
			name:         "mysql needs node-level high availability",
			category:     taxonomy.DatabaseMySQL,
			capabilities: []taxonomy.Capability{taxonomy.NodeHighAvailability},
			wantFit:      mapping.Direct,
			want: []candidateExpectation{
				{Name: "Managed MySQL Multi-node", Fit: mapping.Direct},
				{Name: "Managed MySQL", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.NodeHighAvailability}},
			},
		},
		{
			name:         "mysql needs read replicas",
			category:     taxonomy.DatabaseMySQL,
			capabilities: []taxonomy.Capability{taxonomy.ReadReplicas},
			wantFit:      mapping.Direct,
			want: []candidateExpectation{
				{Name: "Managed MySQL Multi-node", Fit: mapping.Direct},
				{Name: "Managed MySQL", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ReadReplicas}},
			},
		},
		{
			name:         "mysql needs zone-level high availability",
			category:     taxonomy.DatabaseMySQL,
			capabilities: []taxonomy.Capability{taxonomy.ZoneHighAvailability},
			wantFit:      mapping.Partial,
			want: []candidateExpectation{
				{Name: "Managed MySQL", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ZoneHighAvailability}},
				{Name: "Managed MySQL Multi-node", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ZoneHighAvailability}},
			},
		},
		{
			name:         "mysql needs zone-level high availability and read replicas",
			category:     taxonomy.DatabaseMySQL,
			capabilities: []taxonomy.Capability{taxonomy.ZoneHighAvailability, taxonomy.ReadReplicas},
			wantFit:      mapping.Partial,
			want: []candidateExpectation{
				{Name: "Managed MySQL Multi-node", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ZoneHighAvailability}},
				{Name: "Managed MySQL", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ReadReplicas, taxonomy.ZoneHighAvailability}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			checkMapping(t, mapping.Map(newResource(t, "res-example", tc.category, tc.capabilities...), cat), tc.wantFit, tc.want)
		})
	}
}

// TestBaseOfferingsOmitVariantCapabilities pins the other half of ADR 0011's
// tier-variant rule: a capability that a tier variant adds is listed on the
// variant alone. If a base offering carried it too, a resource needing nothing
// would tie with the variant for the wrong reason, and the variant would stop
// meaning anything.
func TestBaseOfferingsOmitVariantCapabilities(t *testing.T) {
	t.Parallel()

	cat := catalogOf(t)

	tests := []struct {
		offering     string
		capabilities []taxonomy.Capability
	}{
		{
			offering:     "Block Storage",
			capabilities: []taxonomy.Capability{taxonomy.EncryptionAtRest},
		},
		{
			offering:     "Managed Kubernetes",
			capabilities: []taxonomy.Capability{taxonomy.NodeHighAvailability},
		},
		{
			offering:     "Managed PostgreSQL",
			capabilities: []taxonomy.Capability{taxonomy.NodeHighAvailability, taxonomy.ReadReplicas},
		},
		{
			offering:     "Managed MySQL",
			capabilities: []taxonomy.Capability{taxonomy.NodeHighAvailability, taxonomy.ReadReplicas},
		},
	}

	for _, tc := range tests {
		t.Run(tc.offering, func(t *testing.T) {
			t.Parallel()

			o := offeringNamed(t, cat, tc.offering)

			for _, capability := range tc.capabilities {
				if o.Has(capability) {
					t.Errorf("offering %s lists %s, want it on the tier variant that adds it alone",
						text.Quote(o.Name), text.Quote(string(capability)))
				}
			}
		})
	}
}

// offeringNamed returns the catalog's offering called name, and fails the test if
// there is none.
func offeringNamed(t *testing.T, cat catalog.Catalog, name string) catalog.Offering {
	t.Helper()

	for _, o := range cat.Offerings() {
		if o.Name == name {
			return o
		}
	}

	t.Fatalf("the catalog has no offering called %s: the tier-variant rule above assumes it", text.Quote(name))

	return catalog.Offering{}
}

// TestCatalogHonoursCancelledContext pins that a cancelled run reads nothing, and
// that the error names the embedded UpCloud catalog and its file on one line.
func TestCatalogHonoursCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := upcloud.New().Catalog(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Catalog() with a cancelled context returned error %v, want one wrapping %v", err, context.Canceled)
	}

	if strings.ContainsAny(err.Error(), "\r\n") {
		t.Errorf("Catalog() error = %q, want a single line", err.Error())
	}

	for _, want := range []string{"embedded UpCloud catalog", catalogFile} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Catalog() error = %q, want it to name %q", err.Error(), want)
		}
	}
}

// TestCatalogErrorNamesNoOtherTarget pins PRD 77's rule that no error says
// "OVHcloud" or "Scaleway" when the target is UpCloud. The other adapters' own
// tests pin the other directions.
func TestCatalogErrorNamesNoOtherTarget(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := upcloud.New().Catalog(ctx)
	if err == nil {
		t.Fatalf("Catalog() with a cancelled context returned no error, want one naming the UpCloud catalog")
	}

	for _, other := range []string{"OVHcloud", "Scaleway"} {
		if strings.Contains(err.Error(), other) {
			t.Errorf("Catalog() error = %q, want no other target's name in it", err.Error())
		}
	}
}

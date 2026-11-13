// Package scaleway_test pins the Scaleway catalog adapter: the target contract in
// internal/adapter/target/targettest, the snapshot header the catalog reports, a
// golden change detector for the parsed catalog, the declared EU regions, that a
// cancelled run reads nothing, and the verdicts PRD 14 asks of the data: high
// availability scoped to a node or a zone, one offering per tier, and a
// capability claimed only when the documentation meets it in full. The snapshot
// document format itself, its strictness and its errors are pinned once, in the
// shared reader's own tests (internal/adapter/target/snapshot). It is black-box:
// only the package's exported API is used.
package scaleway_test

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

	"github.com/timothymamo/shift2eu/internal/adapter/target/scaleway"
	"github.com/timothymamo/shift2eu/internal/adapter/target/snapshot"
	"github.com/timothymamo/shift2eu/internal/adapter/target/targettest"
	"github.com/timothymamo/shift2eu/internal/app/analysis"
	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
	"github.com/timothymamo/shift2eu/internal/domain/mapping"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	"github.com/timothymamo/shift2eu/internal/domain/text"
	"github.com/timothymamo/shift2eu/internal/testutil"
)

// The adapter implements the catalog source port: pin that at compile time.
var _ analysis.CatalogSource = scaleway.New()

// The catalog the adapter embeds, and the golden file its parsed form renders to.
// Both are relative to this package's directory.
const (
	catalogFile = "catalog.json"
	goldenFile  = "testdata/catalog.golden"
)

// wantRegions is the EU region list catalog.json declares, written out here so
// the test cannot agree with a wrong declaration by construction: the contract
// expectations below and TestRegions compare against this list, never against
// what Regions() returns.
func wantRegions() []string {
	return []string{"fr-par", "it-mil", "nl-ams", "pl-waw"}
}

// TestCatalogContract runs the target contract suite every target adapter must
// pass: the display name, every offering only in a declared EU region, the four
// categories PRD 2 requires, the same catalog on every call, and cancellation.
func TestCatalogContract(t *testing.T) {
	t.Parallel()

	targettest.Run(t, scaleway.New(), targettest.Expectations{
		Target:  "Scaleway",
		Regions: wantRegions(),
		Categories: []taxonomy.Category{
			taxonomy.ComputeVM,
			taxonomy.KubernetesManaged,
			taxonomy.StorageObject,
			taxonomy.DatabasePostgreSQL,
		},
	})
}

// catalogOf returns the catalog a fresh source serves.
func catalogOf(t *testing.T) catalog.Catalog {
	t.Helper()

	cat, err := scaleway.New().Catalog(t.Context())
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

	snapshot := catalogOf(t).Snapshot()

	const wantSourceURL = "https://www.scaleway.com/en/docs/"
	if snapshot.SourceURL != wantSourceURL {
		t.Errorf("Snapshot().SourceURL = %s, want %s", text.Quote(snapshot.SourceURL), text.Quote(wantSourceURL))
	}

	wantRetrievedOn := time.Date(2026, time.November, 11, 0, 0, 0, 0, time.UTC)
	if !snapshot.RetrievedOn.Equal(wantRetrievedOn) {
		t.Errorf("Snapshot().RetrievedOn = %s, want %s",
			snapshot.RetrievedOn.Format(time.RFC3339), wantRetrievedOn.Format(time.RFC3339))
	}

	if got := snapshot.RetrievedOn.Location(); got != time.UTC {
		t.Errorf("Snapshot().RetrievedOn location = %v, want time.UTC", got)
	}
}

// TestCatalogGolden pins the whole parsed catalog against a hand-written golden
// file: every offering with its categories, capabilities and regions, in the
// order and canonical form the domain gives them. It is a change detector for the
// snapshot data and for the parsing, so a refresh of catalog.json has to update
// this file in the same review.
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

	snapshot := cat.Snapshot()

	fmt.Fprintf(&b, "target: %s\n", cat.Target())
	fmt.Fprintf(&b, "sourceURL: %s\n", snapshot.SourceURL)
	fmt.Fprintf(&b, "retrievedOn: %s\n", snapshot.RetrievedOn.UTC().Format(time.DateOnly))

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

// TestRegions pins the declared EU region list the reviewer checks against
// Scaleway's published regions, and that callers get a copy: a driving adapter
// cannot change what the adapter declares.
func TestRegions(t *testing.T) {
	t.Parallel()

	want := wantRegions()

	got := scaleway.Regions()
	if !slices.Equal(got, want) {
		t.Fatalf("Regions() = %v, want %v", got, want)
	}

	for i := range got {
		got[i] = "tampered"
	}

	if again := scaleway.Regions(); !slices.Equal(again, want) {
		t.Errorf("Regions() after changing the returned slice = %v, want %v: the result is not a copy", again, want)
	}
}

// TestComputeCategoriesSeparateVirtualMachinesFromBareMetal pins the taxonomy split
// issue #187 asks for: compute.vm is a virtual machine only, and a dedicated
// physical server is compute.baremetal. Elastic Metal is a bare-metal server, so
// it must not tie with Instances for a VM workload and win the tie by name. The
// category values are written out as catalog.json holds them: an offering's
// categories are snapshot data, not a reference to a Go constant.
func TestComputeCategoriesSeparateVirtualMachinesFromBareMetal(t *testing.T) {
	t.Parallel()

	cat := catalogOf(t)

	tests := []struct {
		name     string
		category taxonomy.Category
		want     []string
	}{
		{
			name:     "virtual machines",
			category: taxonomy.Category("compute.vm"),
			want:     []string{"Instances"},
		},
		{
			name:     "bare metal servers",
			category: taxonomy.Category("compute.baremetal"),
			want:     []string{"Elastic Metal"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got []string

			for _, o := range cat.Offerings() {
				if slices.Contains(o.Categories, tc.category) {
					got = append(got, o.Name)
				}
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("offerings serving %s mismatch (-want +got):\n%s", text.Quote(string(tc.category)), diff)
			}
		})
	}
}

// newResource builds r the way an inventory holds it, so the domain gets the
// canonical form: an inventory validates and normalises its resources, and
// mapping.Map expects one that went through it.
func newResource(t *testing.T, category taxonomy.Category, capabilities ...taxonomy.Capability) inventory.Resource {
	t.Helper()

	r := inventory.Resource{
		ID:           "res-example",
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
// the ties.
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

// TestDatabaseHighAvailabilityScope pins PRD 14 milestone 2's split of high
// availability for Scaleway's Managed Database: the base offering is the
// Standalone tier and lacks high availability, and the High Availability option
// is a variant named after it. The variant survives the loss of a node only, so
// a resource needing zone level is partial on both offerings and pointed at
// neither.
func TestDatabaseHighAvailabilityScope(t *testing.T) {
	t.Parallel()

	cat := catalogOf(t)

	const (
		standalone       = "Managed Database for PostgreSQL and MySQL"
		highAvailability = "Managed Database for PostgreSQL and MySQL High Availability"
	)

	tests := []struct {
		name         string
		capabilities []taxonomy.Capability
		wantFit      mapping.Fit
		want         []candidateExpectation
	}{
		{
			name:         "zone level is offered by neither the node-level variant nor the base offering",
			capabilities: []taxonomy.Capability{taxonomy.ZoneHighAvailability},
			wantFit:      mapping.Partial,
			want: []candidateExpectation{
				{Name: standalone, Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ZoneHighAvailability}},
				{Name: highAvailability, Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ZoneHighAvailability}},
			},
		},
		{
			name:         "node level is the High Availability variant only",
			capabilities: []taxonomy.Capability{taxonomy.NodeHighAvailability},
			wantFit:      mapping.Direct,
			want: []candidateExpectation{
				{Name: highAvailability, Fit: mapping.Direct},
				{Name: standalone, Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.NodeHighAvailability}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			checkMapping(t, mapping.Map(newResource(t, taxonomy.DatabasePostgreSQL, tc.capabilities...), cat), tc.wantFit, tc.want)
		})
	}
}

// TestKubernetesKapsuleTiers pins PRD 14 milestone 2's tier variant for
// Scaleway's managed Kubernetes: Mutualized is the default tier and the only
// offering for a resource that needs nothing above autoscaling, and the Dedicated
// control plane is a variant named after it that adds node-level high
// availability. Scaleway's control plane is unreachable if the primary zone
// fails, so neither offering claims zone level.
func TestKubernetesKapsuleTiers(t *testing.T) {
	t.Parallel()

	cat := catalogOf(t)

	const (
		mutualized = "Kubernetes Kapsule"
		dedicated  = "Kubernetes Kapsule Dedicated"
	)

	tests := []struct {
		name         string
		capabilities []taxonomy.Capability
		wantFit      mapping.Fit
		want         []candidateExpectation
	}{
		{
			name:         "node level is the Dedicated variant only",
			capabilities: []taxonomy.Capability{taxonomy.NodeHighAvailability},
			wantFit:      mapping.Direct,
			want: []candidateExpectation{
				{Name: dedicated, Fit: mapping.Direct},
				{Name: mutualized, Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.NodeHighAvailability}},
			},
		},
		{
			name:    "no capabilities leaves the default tier and its variant tied",
			wantFit: mapping.Direct,
			want: []candidateExpectation{
				{Name: mutualized, Fit: mapping.Direct},
				{Name: dedicated, Fit: mapping.Direct},
			},
		},
		{
			name:         "zone level is offered by neither tier",
			capabilities: []taxonomy.Capability{taxonomy.ZoneHighAvailability},
			wantFit:      mapping.Partial,
			want: []candidateExpectation{
				{Name: mutualized, Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ZoneHighAvailability}},
				{Name: dedicated, Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.ZoneHighAvailability}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			checkMapping(t, mapping.Map(newResource(t, taxonomy.KubernetesManaged, tc.capabilities...), cat), tc.wantFit, tc.want)
		})
	}
}

// TestServerlessFunctionsIPv6 pins PRD 14's strict reading of ipv6: it means
// connectivity in both directions, and Scaleway documents Serverless Functions as
// reachable over IPv6 only. The offering therefore lists ipv6 for neither
// direction, and a function that depends on it is partial.
func TestServerlessFunctionsIPv6(t *testing.T) {
	t.Parallel()

	cat := catalogOf(t)

	got := mapping.Map(newResource(t, taxonomy.ServerlessFunction, taxonomy.IPv6), cat)

	checkMapping(t, got, mapping.Partial, []candidateExpectation{
		{Name: "Serverless Functions", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.IPv6}},
	})
}

// TestNoServerlessSQLDatabaseOffering pins PRD 14's decision that Scaleway
// Serverless SQL Database leaves the snapshot: it is PostgreSQL-compatible with
// documented differences, so it does not fully serve database.postgresql and must
// not stand in for a PostgreSQL database, even one whose only capability is
// autoscaling. Managed Database does not autoscale, so that database is partial on
// the two tiers that do serve the category.
func TestNoServerlessSQLDatabaseOffering(t *testing.T) {
	t.Parallel()

	cat := catalogOf(t)

	const removed = "Serverless SQL Database"

	for _, o := range cat.Offerings() {
		if o.Name == removed {
			t.Errorf("the catalog lists an offering called %s, want none: it is not fully compatible with %s",
				text.Quote(o.Name), text.Quote(string(taxonomy.DatabasePostgreSQL)))
		}
	}

	got := mapping.Map(newResource(t, taxonomy.DatabasePostgreSQL, taxonomy.Autoscaling), cat)

	checkMapping(t, got, mapping.Partial, []candidateExpectation{
		{Name: "Managed Database for PostgreSQL and MySQL", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.Autoscaling}},
		{Name: "Managed Database for PostgreSQL and MySQL High Availability", Fit: mapping.Partial, Missing: []taxonomy.Capability{taxonomy.Autoscaling}},
	})

	for _, c := range got.Candidates {
		if c.Offering.Name == removed {
			t.Errorf("Map() lists %s as a candidate, want none: it does not fully serve the category",
				text.Quote(c.Offering.Name))
		}
	}
}

// TestHighAvailabilityIsScopedAndStrictlyClaimed pins PRD 14's strict-definition
// rule: Object Storage claims neither high-availability level, because Scaleway
// documents that it rebuilds the data after a whole-zone failure, which is
// recovery, not continuity without manual failover. That every offering is scoped
// is no longer this test's job: ADR 0011 retired the unscoped value, and
// catalog.New refuses a snapshot that lists it (pinned in
// internal/domain/catalog/catalog_test.go), so no offering here can carry it.
func TestHighAvailabilityIsScopedAndStrictlyClaimed(t *testing.T) {
	t.Parallel()

	cat := catalogOf(t)

	var objectStorage catalog.Offering
	for _, o := range cat.Offerings() {
		if o.Name == "Object Storage" {
			objectStorage = o
		}
	}

	if objectStorage.Name == "" {
		t.Fatalf("the catalog has no offering called %s: the strict-definition rule below assumes it", text.Quote("Object Storage"))
	}

	for _, capability := range []taxonomy.Capability{taxonomy.NodeHighAvailability, taxonomy.ZoneHighAvailability} {
		if objectStorage.Has(capability) {
			t.Errorf("offering %s lists %s, want neither high-availability level: rebuilding the data after a zone failure is recovery, not continuity",
				text.Quote(objectStorage.Name), text.Quote(string(capability)))
		}
	}
}

// TestCatalogHonoursCancelledContext pins that a cancelled run reads nothing.
func TestCatalogHonoursCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := scaleway.New().Catalog(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Catalog() with a cancelled context returned error %v, want one wrapping %v", err, context.Canceled)
	}

	if strings.ContainsAny(err.Error(), "\r\n") {
		t.Errorf("Catalog() error = %q, want a single line", err.Error())
	}

	// PRD 11: no error says "OVHcloud" or "UpCloud" when the target is Scaleway.
	// The other adapters' own tests pin the other directions.
	for _, other := range []string{"OVHcloud", "UpCloud"} {
		if strings.Contains(err.Error(), other) {
			t.Errorf("Catalog() error = %q, want no other target's name in it, but it names %q", err.Error(), other)
		}
	}
}

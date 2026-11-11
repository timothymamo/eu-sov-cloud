package jsonreport_test

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/report/jsonreport"
	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/testutil"
)

// The keys the comparison fixtures select their targets with: the CLI's keys,
// lower case and one word, not the catalogs' display names.
const (
	ovhcloudKey = "ovhcloud"
	scalewayKey = "scaleway"
	upcloudKey  = "upcloud"
)

// comparisonResources is the inventory the three-target document assesses, chosen
// so the comparison has to tell four things apart:
//
//   - bucket-example is direct on OVHcloud and Scaleway and partial on UpCloud,
//     whose object storage lacks versioning;
//   - db-example is direct on OVHcloud and Scaleway and blocked on UpCloud only,
//     which has no database offering;
//   - fn-example and misc-example are blocked on every target, one for a category
//     no target serves and one for being unclassified, so they are the shared
//     blockers;
//   - partial-example is partial on every target.
//
// The IDs sort into the order the resources must be written in, which is also the
// order the shared blockers must be listed in.
func comparisonResources() []inventory.Resource {
	return []inventory.Resource{
		{
			ID:           "bucket-example",
			RawType:      "AWS::S3::Bucket",
			Category:     taxonomy.StorageObject,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.ObjectVersioning},
		},
		{
			ID:           "db-example",
			RawType:      "AWS::RDS::DBInstance",
			Category:     taxonomy.DatabasePostgreSQL,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.PointInTimeRestore, taxonomy.ReadReplicas},
		},
		{
			ID:       "fn-example",
			RawType:  "AWS::Lambda::Function",
			Category: taxonomy.ServerlessFunction,
			Region:   "eu-west-1",
		},
		{
			ID:       "misc-example",
			RawType:  "AWS::CloudFormation::CustomResource",
			Category: taxonomy.Unclassified,
			Region:   "eu-west-1",
		},
		{
			ID:           "partial-example",
			RawType:      "AWS::EC2::Instance",
			Category:     taxonomy.ComputeVM,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.NodeHighAvailability, taxonomy.Snapshots},
		},
	}
}

// twoTargetResources is the inventory the no-shared-blocker document assesses:
// OVHcloud and Scaleway both serve everything in it.
func twoTargetResources() []inventory.Resource {
	return []inventory.Resource{
		{
			ID:           "bucket-example",
			RawType:      "AWS::S3::Bucket",
			Category:     taxonomy.StorageObject,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.ObjectVersioning},
		},
		{
			ID:           "partial-example",
			RawType:      "AWS::EC2::Instance",
			Category:     taxonomy.ComputeVM,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.NodeHighAvailability, taxonomy.Snapshots},
		},
	}
}

// comparisonInventory is the three-target document's inventory.
func comparisonInventory(t *testing.T) inventory.Inventory {
	t.Helper()

	return newInventory(t, sourceName, comparisonResources())
}

// comparisonFixtureCatalog returns the fixture catalog of one target key. Each
// catalog has its own target display name, source URL and retrieval date, so the
// document has to keep three targets apart.
func comparisonFixtureCatalog(t *testing.T, key string) catalog.Catalog {
	t.Helper()

	switch key {
	case ovhcloudKey:
		return testutil.OVHcloudCatalog(t)
	case scalewayKey:
		return testutil.ScalewayCatalog(t)
	case upcloudKey:
		return testutil.UpCloudCatalog(t)
	default:
		t.Fatalf("no fixture catalog for target key %q", key)

		return catalog.Catalog{}
	}
}

// assessTargets assesses inv against the fixture catalog of each key, in the
// order given, and returns the target assessments Compare takes.
func assessTargets(t *testing.T, inv inventory.Inventory, keys []string) []assessment.TargetAssessment {
	t.Helper()

	targets := make([]assessment.TargetAssessment, 0, len(keys))
	for _, key := range keys {
		targets = append(targets, assessment.TargetAssessment{
			Key:        key,
			Assessment: assessment.Assess(inv, comparisonFixtureCatalog(t, key)),
		})
	}

	return targets
}

// threeTargetKeys is the target list the three-target document compares.
func threeTargetKeys() []string {
	return []string{ovhcloudKey, scalewayKey, upcloudKey}
}

// renderComparison writes the comparison document of c and meta and returns it.
func renderComparison(tb testing.TB, c assessment.Comparison, meta analysis.ReportMeta) []byte {
	tb.Helper()

	var out bytes.Buffer
	if err := jsonreport.New().ReportComparison(tb.Context(), &out, c, meta); err != nil {
		tb.Fatalf("ReportComparison() returned unexpected error: %v", err)
	}

	return out.Bytes()
}

// TestReportComparison pins the whole comparison document against the fixture
// catalogs: the version and kind, the metadata, one entry per target in the
// comparison's order with that target's own heading, summary, resources and
// blockers, and the blockers every target shares.
func TestReportComparison(t *testing.T) {
	t.Parallel()

	c := assessment.Compare(assessTargets(t, comparisonInventory(t), threeTargetKeys()))

	got := renderComparison(t, c, analysis.ReportMeta{InventoryName: inventoryFileName})
	testutil.Golden(t, "testdata/comparison-three.golden", got)
	checkDocument(t, got)

	document := decodeJSON(t, got)

	if diff := cmp.Diff(float64(1), valueAt(t, document, "schemaVersion")); diff != "" {
		t.Errorf("schemaVersion mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff("comparison", valueAt(t, document, "kind")); diff != "" {
		t.Errorf("kind mismatch (-want +got):\n%s", diff)
	}

	// The targets keep the comparison's order, which Compare sorts by key.
	if diff := cmp.Diff(comparisonTargetKeys(c), documentTargetKeys(t, document)); diff != "" {
		t.Errorf("targets mismatch (-want +got):\n%s", diff)
	}

	// The shared blockers are the two resources blocked on every target, in
	// resource ID order, each naming every target's reasons.
	wantShared := []string{"fn-example", "misc-example"}

	shared := arrayAt(t, document, "sharedBlockers")
	if len(shared) != len(wantShared) {
		t.Fatalf("the document has %d shared blockers, want %d", len(shared), len(wantShared))
	}

	for i, resourceID := range wantShared {
		at := []any{"sharedBlockers", i}

		if diff := cmp.Diff(resourceID, valueAt(t, document, extend(at, "resourceId")...)); diff != "" {
			t.Errorf("sharedBlockers[%d].resourceId mismatch (-want +got):\n%s", i, diff)
		}

		var keys []string

		for j := range arrayAt(t, document, extend(at, "targets")...) {
			key, ok := valueAt(t, document, extend(at, "targets", j, "key")...).(string)
			if !ok {
				t.Fatalf("sharedBlockers[%d].targets[%d].key is not a string", i, j)
			}

			keys = append(keys, key)
		}

		if diff := cmp.Diff(threeTargetKeys(), keys); diff != "" {
			t.Errorf("%q shared blocker targets mismatch (-want +got):\n%s", resourceID, diff)
		}
	}
}

// TestReportComparisonShape pins the comparison document's schema and content
// without a golden file: the key set of every object (the schemas allow no other
// key), each target entry holding that target's own assessment in that
// assessment's order, and the shared blockers.
func TestReportComparisonShape(t *testing.T) {
	t.Parallel()

	c := assessment.Compare(assessTargets(t, comparisonInventory(t), threeTargetKeys()))
	document := decodeJSON(t, renderComparison(t, c, analysis.ReportMeta{InventoryName: inventoryFileName}))

	t.Run("object keys", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			path []any
			want []string
		}{
			{
				name: "the document",
				path: nil,
				want: []string{
					"inventoryName", "kind", "schemaVersion", "sharedBlockers", "source", "targets",
				},
			},
			{
				name: "a target entry",
				path: []any{"targets", 0},
				want: []string{"blockers", "key", "resources", "summary", "target"},
			},
			{name: "a target's target", path: []any{"targets", 0, "target"}, want: []string{"name", "snapshot"}},
			{
				name: "a target's snapshot",
				path: []any{"targets", 0, "target", "snapshot"},
				want: []string{"retrieved", "source"},
			},
			{
				name: "a target's summary",
				path: []any{"targets", 0, "summary"},
				want: []string{"direct", "none", "partial", "total"},
			},
			{
				name: "a target's resource",
				path: []any{"targets", 0, "resources", 0},
				want: []string{
					"bestOffering", "category", "fit", "id", "rawType", "reasons", "region", "requiredChanges",
					"ties",
				},
			},
			{
				name: "a target's blocker",
				path: []any{"targets", 0, "blockers", 0},
				want: []string{"reason", "resourceId"},
			},
			{
				name: "a shared blocker",
				path: []any{"sharedBlockers", 0},
				want: []string{"resourceId", "targets"},
			},
			{
				name: "a shared blocker's target",
				path: []any{"sharedBlockers", 0, "targets", 0},
				want: []string{"key", "reasons"},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				if diff := cmp.Diff(test.want, objectKeysAt(t, document, test.path...)); diff != "" {
					t.Errorf("%v keys mismatch (-want +got):\n%s", test.path, diff)
				}
			})
		}
	})

	t.Run("each target holds its own assessment", func(t *testing.T) {
		t.Parallel()

		targets := arrayAt(t, document, "targets")
		if len(targets) != len(c.Targets) {
			t.Fatalf("the document has %d targets, the comparison has %d", len(targets), len(c.Targets))
		}

		for i, target := range c.Targets {
			at := []any{"targets", i}

			if got := valueAt(t, document, extend(at, "key")...); got != target.Key {
				t.Errorf("targets[%d].key = %v, want %q", i, got, target.Key)
			}

			if got := valueAt(t, document, extend(at, "target", "name")...); got != target.Assessment.Target {
				t.Errorf("%q target name = %v, want %q", target.Key, got, target.Assessment.Target)
			}

			if got := valueAt(t, document, extend(at, "target", "snapshot", "source")...); got !=
				target.Assessment.Snapshot.SourceURL {
				t.Errorf("%q snapshot source = %v, want %q", target.Key, got, target.Assessment.Snapshot.SourceURL)
			}

			wantDate := snapshotDate(target.Assessment.Snapshot)
			if got := valueAt(t, document, extend(at, "target", "snapshot", "retrieved")...); got != wantDate {
				t.Errorf("%q snapshot retrieved = %v, want %q", target.Key, got, wantDate)
			}

			// The resources keep that target's verdict order.
			resources := arrayAt(t, document, extend(at, "resources")...)
			if len(resources) != len(target.Assessment.Verdicts) {
				t.Fatalf("%q holds %d resources, its assessment has %d verdicts",
					target.Key, len(resources), len(target.Assessment.Verdicts))
			}

			var ids []string

			for j := range resources {
				id, ok := valueAt(t, document, extend(at, "resources", j, "id")...).(string)
				if !ok {
					t.Fatalf("%q resources[%d].id is not a string", target.Key, j)
				}

				if want := target.Assessment.Verdicts[j].Resource.ID; id != want {
					t.Errorf("%q resources[%d].id = %q, want its verdict %q", target.Key, j, id, want)
				}

				ids = append(ids, id)
			}

			wantIDs := make([]string, 0, len(target.Assessment.Verdicts))
			for _, v := range target.Assessment.Verdicts {
				wantIDs = append(wantIDs, v.Resource.ID)
			}

			if diff := cmp.Diff(wantIDs, ids); diff != "" {
				t.Errorf("%q resource order mismatch (-want +got):\n%s", target.Key, diff)
			}
		}
	})

	t.Run("shared blocker reasons", func(t *testing.T) {
		t.Parallel()

		// The format writes every target's reasons for a shared blocker; the
		// reason object is the single document's reason object, so its text names
		// the catalog of the target it belongs to, not one catalog for them all,
		// as the Markdown comparison's comparison-three.golden shows.
		keys := threeTargetKeys()

		wantText := func(resourceID, key string) string {
			if resourceID == "misc-example" {
				return "not classified"
			}

			return "no offering in the " + comparisonFixtureCatalog(t, key).Target() +
				" catalog for serverless.function"
		}

		shared := arrayAt(t, document, "sharedBlockers")
		if len(shared) != 2 {
			t.Fatalf("the document has %d shared blockers, want 2", len(shared))
		}

		for i := range shared {
			at := []any{"sharedBlockers", i}

			resourceID, ok := valueAt(t, document, extend(at, "resourceId")...).(string)
			if !ok {
				t.Fatalf("sharedBlockers[%d].resourceId is not a string", i)
			}

			targets := arrayAt(t, document, extend(at, "targets")...)
			if len(targets) != len(keys) {
				t.Fatalf("%q shared blocker holds %d targets, want %d", resourceID, len(targets), len(keys))
			}

			for j, key := range keys {
				if got := valueAt(t, document, extend(at, "targets", j, "key")...); got != key {
					t.Errorf("%q shared blocker targets[%d].key = %v, want %q", resourceID, j, got, key)
				}

				if got := valueAt(t, document, extend(at, "targets", j, "reasons", 0, "code")...); got == "" {
					t.Errorf("%q shared blocker reason for target %q has no code", resourceID, key)
				}

				want := wantText(resourceID, key)
				if got := valueAt(t, document, extend(at, "targets", j, "reasons", 0, "text")...); got != want {
					t.Errorf("%q shared blocker reason text for target %q = %v, want %q",
						resourceID, key, got, want)
				}
			}
		}
	})
}

// TestReportComparisonNoSharedBlocker pins that a comparison without a resource
// blocked on every target writes an empty sharedBlocker array, never null.
func TestReportComparisonNoSharedBlocker(t *testing.T) {
	t.Parallel()

	c := assessment.Compare(assessTargets(t, newInventory(t, sourceName, twoTargetResources()),
		[]string{ovhcloudKey, scalewayKey}))

	if len(c.SharedBlockers) != 0 {
		t.Fatalf("the fixture has %d shared blockers, want none", len(c.SharedBlockers))
	}

	document := decodeJSON(t, renderComparison(t, c, analysis.ReportMeta{InventoryName: inventoryFileName}))

	shared, ok := valueAt(t, document, "sharedBlockers").([]any)
	if !ok {
		t.Errorf("sharedBlockers is not an array; an array is never null")
	}

	if len(shared) != 0 {
		t.Errorf("sharedBlockers holds %d entries, want an empty array", len(shared))
	}

	if got := valueAt(t, document, "kind"); got != "comparison" {
		t.Errorf("kind = %v, want %q", got, "comparison")
	}
}

// TestReportComparisonRefusesMalformedComparison pins that the document is
// written only for a comparison the domain accepts: the targets must assess the
// same resources in the same order, and every shared blocker must name only
// compared targets. A document written from such a comparison would hold a
// resource set the reader cannot line up across targets, or a blocker naming a
// target the document does not hold. Each is an error, and nothing is written.
func TestReportComparisonRefusesMalformedComparison(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(t *testing.T, c *assessment.Comparison)
	}{
		{
			name: "the targets assess the same resources in another order",
			change: func(_ *testing.T, c *assessment.Comparison) {
				c.Targets[1].Assessment.Verdicts[0], c.Targets[1].Assessment.Verdicts[1] =
					c.Targets[1].Assessment.Verdicts[1], c.Targets[1].Assessment.Verdicts[0]
			},
		},
		{
			name: "a later target holds a verdict the first target lacks",
			change: func(t *testing.T, c *assessment.Comparison) {
				t.Helper()

				c.Targets[1].Assessment.Verdicts = append(slices.Clone(c.Targets[1].Assessment.Verdicts),
					assessment.Verdict{
						Resource: inventory.Resource{
							ID:       "extra-example",
							RawType:  "AWS::SQS::Queue",
							Category: taxonomy.Unclassified,
						},
						Fit:     mapping.None,
						Reasons: []mapping.Reason{{Code: mapping.NotClassified}},
					})
			},
		},
		{
			name: "a shared blocker names a target the comparison does not hold",
			change: func(t *testing.T, c *assessment.Comparison) {
				t.Helper()

				if len(c.SharedBlockers) == 0 {
					t.Fatal("precondition failed: the fixture comparison has no shared blocker")
				}

				c.SharedBlockers[0].Targets = append(c.SharedBlockers[0].Targets, assessment.TargetReasons{
					Key:     "missing",
					Reasons: []mapping.Reason{{Code: mapping.NotClassified}},
				})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := assessment.Compare(assessTargets(t, comparisonInventory(t), threeTargetKeys()))
			tc.change(t, &c)

			var out bytes.Buffer

			err := jsonreport.New().ReportComparison(t.Context(), &out, c,
				analysis.ReportMeta{InventoryName: inventoryFileName})
			if err == nil {
				t.Fatalf("ReportComparison() error = nil, want an error when %s", tc.name)
			}

			if out.Len() != 0 {
				t.Errorf("ReportComparison() wrote %d bytes to the writer, want none:\n%s", out.Len(), out.String())
			}
		})
	}
}

// hostileComparisonKeys are the hostile comparison's target keys: the document
// writes each one as targets[].key and as a shared blocker target's key, so a key
// is a value like any other.
func hostileComparisonKeys() []string {
	return []string{
		"a\"\\\n\t\x00\x1b\u2028</script>\xff",
		"k\"ey\x00\x1b\u2028</script>\xff",
		"z<z|`\n\x1b\xff",
	}
}

// hostileComparisonTargets builds the three target assessments of the hostile
// document. Each target has the same hostile display name and snapshot source and
// its own retrieval date; one resource is blocked on every target for a category
// no target serves, one is blocked on every target for being unclassified, and
// one is direct, partial and none on the three targets.
func hostileComparisonTargets() []assessment.TargetAssessment {
	keys := hostileComparisonKeys()
	retrieved := []time.Time{
		time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC),
	}

	targets := make([]assessment.TargetAssessment, 0, len(keys))

	for i, key := range keys {
		a := assessment.Assessment{
			Target:   hostileTarget,
			Snapshot: catalog.Snapshot{SourceURL: hostileSourceURL, RetrievedOn: retrieved[i]},
			Source:   hostileSource,
			Verdicts: []assessment.Verdict{
				hostileNoOfferingVerdict(),
				hostileUnclassifiedVerdict(),
				hostileVaryingVerdict(i),
			},
		}

		for _, v := range a.Verdicts {
			switch v.Fit {
			case mapping.Direct:
				a.Summary.Direct++
			case mapping.Partial:
				a.Summary.Partial++
			case mapping.None:
				a.Summary.None++

				for _, reason := range v.Reasons {
					a.Blockers = append(a.Blockers, assessment.Blocker{ResourceID: v.Resource.ID, Reason: reason})
				}
			}
		}

		a.Summary.Total = len(a.Verdicts)

		targets = append(targets, assessment.TargetAssessment{Key: key, Assessment: a})
	}

	return targets
}

// hostileNoOfferingVerdict is the verdict of the resource blocked on every target
// for a category no target serves.
func hostileNoOfferingVerdict() assessment.Verdict {
	category := hostileCategory()

	return assessment.Verdict{
		Resource: inventory.Resource{
			ID:       hostileID,
			RawType:  hostileRawType,
			Category: category,
			Region:   hostileRegion,
		},
		Fit:     mapping.None,
		Reasons: []mapping.Reason{{Code: mapping.NoOffering, Target: hostileTarget, Category: category}},
	}
}

// hostileUnclassifiedVerdict is the verdict of the second resource blocked on
// every target.
func hostileUnclassifiedVerdict() assessment.Verdict {
	return assessment.Verdict{
		Resource: inventory.Resource{
			ID:       hostileBlockedID,
			RawType:  hostileRawType,
			Category: hostileCategory(),
			Region:   hostileRegion,
		},
		Fit:     mapping.None,
		Reasons: []mapping.Reason{{Code: mapping.NotClassified}},
	}
}

// hostileVaryingVerdict is the verdict of the resource that is direct on the
// first target, partial on the second and none on the third, so the document
// holds a hostile offering, a hostile missing capability and a hostile literal
// reason.
func hostileVaryingVerdict(target int) assessment.Verdict {
	resource := inventory.Resource{
		ID:       hostileID + "-v",
		RawType:  hostileRawType,
		Category: taxonomy.ComputeVM,
		Region:   hostileRegion,
	}

	switch target {
	case 0:
		return assessment.Verdict{
			Resource:     resource,
			Fit:          mapping.Direct,
			BestOffering: hostileOffering,
			Candidates: []mapping.Candidate{{
				Offering: catalog.Offering{Name: hostileOffering, Categories: []taxonomy.Category{taxonomy.ComputeVM}},
				Fit:      mapping.Direct,
			}},
			Reasons: []mapping.Reason{{Code: mapping.AllCapabilities, Offering: hostileOffering}},
		}
	case 1:
		return assessment.Verdict{
			Resource:     resource,
			Fit:          mapping.Partial,
			BestOffering: hostileOffering,
			Reasons: []mapping.Reason{{
				Code:       mapping.MissingCapability,
				Offering:   hostileOffering,
				Capability: hostileCapability,
			}},
			RequiredChanges: []taxonomy.Capability{hostileCapability},
		}
	default:
		return assessment.Verdict{
			Resource: resource,
			Fit:      mapping.None,
			Reasons:  []mapping.Reason{{Code: hostileReasonCode}},
		}
	}
}

// TestReportComparisonHostileValues pins that no value reaches the comparison
// document unencoded: the output is valid JSON and valid UTF-8, and decoding it
// gives every value back, with each invalid UTF-8 byte replaced by U+FFFD.
func TestReportComparisonHostileValues(t *testing.T) {
	t.Parallel()

	c := assessment.Compare(hostileComparisonTargets())
	got := renderComparison(t, c, analysis.ReportMeta{InventoryName: hostileInventoryName})

	testutil.Golden(t, "testdata/comparison-hostile.golden", got)
	checkDocument(t, got)

	document := decodeJSON(t, got)

	keys := documentTargetKeys(t, document)
	if len(keys) != len(c.Targets) {
		t.Fatalf("the document has %d targets, the comparison has %d", len(keys), len(c.Targets))
	}

	for i, key := range keys {
		if want := strings.ToValidUTF8(c.Targets[i].Key, "\uFFFD"); key != want {
			t.Errorf("targets[%d].key = %q, want %q", i, key, want)
		}
	}

	tests := []struct {
		name string
		path []any
		want string
	}{
		{name: "inventory name", path: []any{"inventoryName"}, want: hostileInventoryName},
		{name: "source", path: []any{"source"}, want: hostileSource},
		{name: "target name", path: []any{"targets", 0, "target", "name"}, want: hostileTarget},
		{
			name: "snapshot source",
			path: []any{"targets", 0, "target", "snapshot", "source"},
			want: hostileSourceURL,
		},
		{name: "snapshot retrieved", path: []any{"targets", 1, "target", "snapshot", "retrieved"}, want: "2026-11-01"},
		{name: "blocked resource ID", path: []any{"targets", 0, "resources", 0, "id"}, want: hostileID},
		{
			name: "blocked resource raw type",
			path: []any{"targets", 0, "resources", 0, "rawType"},
			want: hostileRawType,
		},
		{
			name: "no-offering reason text",
			path: []any{"targets", 0, "resources", 0, "reasons", 0, "text"},
			want: "no offering in the " + hostileTarget + " catalog for " + string(hostileCategory()),
		},
		{
			name: "unknown reason text is the code",
			path: []any{"targets", 2, "resources", 2, "reasons", 0, "text"},
			want: string(hostileReasonCode),
		},
		{
			name: "required change",
			path: []any{"targets", 1, "resources", 2, "requiredChanges", 0},
			want: string(hostileCapability),
		},
		// The shared blockers are in resource ID order: the unclassified one
		// first, then the one blocked for a category no target serves.
		{
			name: "shared blocker resource ID",
			path: []any{"sharedBlockers", 0, "resourceId"},
			want: hostileBlockedID,
		},
		{
			name: "shared blocker reason text",
			path: []any{"sharedBlockers", 0, "targets", 0, "reasons", 0, "text"},
			want: "not classified",
		},
		{
			name: "shared blocker target key",
			path: []any{"sharedBlockers", 1, "targets", 2, "key"},
			want: hostileComparisonKeys()[2],
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := valueAt(t, document, test.path...).(string)
			if !ok {
				t.Fatalf("%v is not a string", test.path)
			}

			if want := strings.ToValidUTF8(test.want, "\uFFFD"); got != want {
				t.Errorf("%v = %q, want %q", test.path, got, want)
			}
		})
	}
}

// TestReportComparisonIsDeterministic pins that the same comparison always
// renders the same bytes, and that the order the targets were assessed in does
// not reach the document: Compare sorts them by key.
func TestReportComparisonIsDeterministic(t *testing.T) {
	t.Parallel()

	meta := analysis.ReportMeta{InventoryName: inventoryFileName}

	first := renderComparison(t, assessment.Compare(assessTargets(t, comparisonInventory(t), threeTargetKeys())), meta)
	second := renderComparison(t, assessment.Compare(assessTargets(t, comparisonInventory(t), threeTargetKeys())), meta)

	if !bytes.Equal(first, second) {
		t.Errorf("two documents of the same comparison differ:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	reversed := threeTargetKeys()
	slices.Reverse(reversed)

	reordered := renderComparison(t, assessment.Compare(assessTargets(t, comparisonInventory(t), reversed)), meta)
	if !bytes.Equal(first, reordered) {
		t.Errorf("the document depends on the order the targets were given in:\nfirst:\n%s\nreordered:\n%s",
			first, reordered)
	}
}

// comparisonTargetKeys returns the target keys of c, in the comparison's order.
func comparisonTargetKeys(c assessment.Comparison) []string {
	keys := make([]string, 0, len(c.Targets))
	for _, target := range c.Targets {
		keys = append(keys, target.Key)
	}

	return keys
}

// documentTargetKeys returns the target keys the comparison document writes, in
// document order.
func documentTargetKeys(t *testing.T, document any) []string {
	t.Helper()

	var keys []string

	for i := range arrayAt(t, document, "targets") {
		key, ok := valueAt(t, document, "targets", i, "key").(string)
		if !ok {
			t.Fatalf("targets[%d].key is not a string", i)
		}

		keys = append(keys, key)
	}

	return keys
}

// snapshotDate is the date the document writes for a snapshot, as the Markdown
// report prints it: the retrieval date in UTC, YYYY-MM-DD.

package jsonreport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/internal/adapter/report/jsonreport"
	"github.com/timothymamo/shift2eu/internal/app/analysis"
	"github.com/timothymamo/shift2eu/internal/domain/assessment"
	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
	"github.com/timothymamo/shift2eu/internal/domain/mapping"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	"github.com/timothymamo/shift2eu/internal/testutil"
)

// The JSON reporter renders the same content as the Markdown report, so it must
// be usable wherever the use cases expect a reporter: the goldens pin what it
// writes, this pins the ports it implements. It implements both, because a run
// with several targets writes a comparison document (docs/adr/0016).
var (
	_ analysis.Reporter           = jsonreport.New()
	_ analysis.ComparisonReporter = jsonreport.New()
)

const (
	// inventoryFileName is the base name of the fixture inventory file, and what
	// a document shows as inventoryName.
	inventoryFileName = "inventory.json"
	// sourceName is the source the ordinary fixture inventory was taken from.
	sourceName = "aws"
	// scalewayTarget is the display name of the fixture catalog.
	scalewayTarget = "Scaleway"
	// scalewaySourceURL is the source URL of the fixture catalog's snapshot.
	scalewaySourceURL = "https://www.scaleway.com/en/pricing/"
	// scalewayRetrievedOn is the retrieval date of the fixture catalog's
	// snapshot, as the document prints it.
	scalewayRetrievedOn = "2026-11-01"
)

// awsResources is one inventory covering every fit: direct, partial, none for a
// category no offering serves, and unclassified. It is the inventory of the
// Markdown report's report.golden, so the two documents carry the same content.
func awsResources() []inventory.Resource {
	return []inventory.Resource{
		{
			ID:           "bucket-example",
			RawType:      "AWS::S3::Bucket",
			Category:     taxonomy.StorageObject,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.IPv6, taxonomy.ObjectVersioning},
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
			ID:           "i-0123456789abcdef0",
			RawType:      "AWS::EC2::Instance",
			Category:     taxonomy.ComputeVM,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.IPv6},
		},
		{
			ID:       "misc-example",
			RawType:  "AWS::CloudFormation::CustomResource",
			Category: taxonomy.Unclassified,
			Region:   "eu-west-1",
		},
		{
			// Two missing capabilities, so the resource is partial and lists two
			// required changes.
			ID:           "partial-example",
			RawType:      "AWS::EC2::Instance",
			Category:     taxonomy.ComputeVM,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.NodeHighAvailability, taxonomy.Snapshots},
		},
	}
}

// newInventory builds the only kind of inventory the domain lets through.
func newInventory(t *testing.T, source string, resources []inventory.Resource) inventory.Inventory {
	t.Helper()

	inv, err := inventory.New(source, resources)
	if err != nil {
		t.Fatalf("inventory.New() returned unexpected error: %v", err)
	}

	return inv
}

// render writes the document of a and meta and returns it.
func render(tb testing.TB, a assessment.Assessment, meta analysis.ReportMeta) []byte {
	tb.Helper()

	var out bytes.Buffer
	if err := jsonreport.New().Report(tb.Context(), &out, a, meta); err != nil {
		tb.Fatalf("Report() returned unexpected error: %v", err)
	}

	return out.Bytes()
}

// TestReport pins the whole document against the Scaleway fixture catalog: the
// version and kind, the metadata, the summary, one resource per verdict in the
// assessment's order, and the blockers, with the same content as the Markdown
// report's report.golden.
func TestReport(t *testing.T) {
	t.Parallel()

	scaleway := testutil.ScalewayCatalog(t)

	tests := []struct {
		name       string
		inventory  inventory.Inventory
		meta       analysis.ReportMeta
		goldenFile string
	}{
		{
			name:       "every fit with the inventory file name",
			inventory:  newInventory(t, sourceName, awsResources()),
			meta:       analysis.ReportMeta{InventoryName: inventoryFileName},
			goldenFile: "testdata/report.golden",
		},
		{
			name:       "no inventory file name writes an empty name",
			inventory:  newInventory(t, sourceName, awsResources()),
			meta:       analysis.ReportMeta{},
			goldenFile: "testdata/report-no-inventory-name.golden",
		},
		{
			name:       "empty inventory keeps empty arrays and no blockers",
			inventory:  newInventory(t, sourceName, nil),
			meta:       analysis.ReportMeta{InventoryName: inventoryFileName},
			goldenFile: "testdata/report-empty.golden",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := render(t, assessment.Assess(test.inventory, scaleway), test.meta)
			testutil.Golden(t, test.goldenFile, got)

			checkDocument(t, got)
		})
	}
}

// checkDocument asserts the properties every document of the format has: it is
// valid JSON, it is valid UTF-8, and it ends with exactly one newline.
func checkDocument(t *testing.T, out []byte) {
	t.Helper()

	if !json.Valid(out) {
		t.Errorf("the document is not valid JSON:\n%s", out)
	}

	if !utf8.Valid(out) {
		t.Errorf("the document is not valid UTF-8:\n%q", out)
	}

	if !bytes.HasSuffix(out, []byte("\n")) || bytes.HasSuffix(out, []byte("\n\n")) {
		t.Errorf("the document must end with exactly one newline, it ends with %q",
			out[max(0, len(out)-2):])
	}
}

// snapshotDate is the date the document writes for a snapshot, as the Markdown
// report prints it: the retrieval date in UTC, YYYY-MM-DD.
func snapshotDate(snapshot catalog.Snapshot) string {
	return snapshot.RetrievedOn.UTC().Format("2006-01-02")
}

// TestReportSnapshotDateIsUTC pins that the retrieval date is the UTC date, as
// the Markdown report prints it: a snapshot taken just after midnight in a zone
// east of UTC must not show the previous day.
func TestReportSnapshotDateIsUTC(t *testing.T) {
	t.Parallel()

	a := assessment.Assessment{
		Target: scalewayTarget,
		Snapshot: catalog.Snapshot{
			SourceURL:   scalewaySourceURL,
			RetrievedOn: time.Date(2026, 11, 2, 0, 30, 0, 0, time.FixedZone("CET", 60*60)),
		},
		Source: sourceName,
	}

	want := snapshotDate(a.Snapshot)
	if want != "2026-11-01" {
		t.Fatalf("snapshotDate() = %q for a date just after midnight CET, want the previous UTC day", want)
	}

	document := decodeJSON(t, render(t, a, analysis.ReportMeta{}))

	if got := valueAt(t, document, "target", "snapshot", "retrieved"); got != want {
		t.Errorf("target.snapshot.retrieved = %v, want the UTC date %q", got, want)
	}
}

// TestReportDocumentShape pins the document's schema without a golden file: the
// schema version and kind, the metadata, the summary, the resources in the
// assessment's order with the ties and required changes the Markdown report
// shows, the blockers, and the exact key set of every object, because the
// schemas allow no key the format does not define.
func TestReportDocumentShape(t *testing.T) {
	t.Parallel()

	scaleway := testutil.ScalewayCatalog(t)
	a := assessment.Assess(newInventory(t, sourceName, awsResources()), scaleway)

	document := decodeJSON(t, render(t, a, analysis.ReportMeta{InventoryName: inventoryFileName}))

	t.Run("fields", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			path []any
			want any
		}{
			// encoding/json decodes every JSON number into a float64.
			{name: "schema version", path: []any{"schemaVersion"}, want: float64(1)},
			{name: "kind", path: []any{"kind"}, want: "assessment"},
			{name: "inventory name", path: []any{"inventoryName"}, want: inventoryFileName},
			{name: "source", path: []any{"source"}, want: sourceName},
			{name: "target name", path: []any{"target", "name"}, want: scalewayTarget},
			{name: "snapshot source", path: []any{"target", "snapshot", "source"}, want: scalewaySourceURL},
			{
				name: "snapshot retrieval date is the date the Markdown report prints",
				path: []any{"target", "snapshot", "retrieved"},
				want: scalewayRetrievedOn,
			},
			{name: "summary direct", path: []any{"summary", "direct"}, want: float64(3)},
			{name: "summary partial", path: []any{"summary", "partial"}, want: float64(1)},
			{name: "summary none", path: []any{"summary", "none"}, want: float64(2)},
			{name: "summary total", path: []any{"summary", "total"}, want: float64(6)},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				if diff := cmp.Diff(test.want, valueAt(t, document, test.path...)); diff != "" {
					t.Errorf("%v mismatch (-want +got):\n%s", test.path, diff)
				}
			})
		}
	})

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
					"blockers", "inventoryName", "kind", "resources", "schemaVersion", "source", "summary",
					"target",
				},
			},
			{name: "the target", path: []any{"target"}, want: []string{"name", "snapshot"}},
			{
				name: "the snapshot",
				path: []any{"target", "snapshot"},
				want: []string{"retrieved", "source"},
			},
			{
				name: "the summary",
				path: []any{"summary"},
				want: []string{"direct", "none", "partial", "total"},
			},
			{
				name: "a direct resource",
				path: []any{"resources", 0},
				want: []string{
					"bestOffering", "category", "fit", "id", "rawType", "reasons", "region", "requiredChanges",
					"ties",
				},
			},
			{
				name: "a reason that sets only code and offering",
				path: []any{"resources", 0, "reasons", 0},
				want: []string{"code", "offering", "text"},
			},
			{
				name: "a reason that sets code, offering and capability",
				path: []any{"resources", 5, "reasons", 0},
				want: []string{"capability", "code", "offering", "text"},
			},
			{
				name: "a reason that sets nothing but its code",
				path: []any{"blockers", 1, "reason"},
				want: []string{"code", "text"},
			},
			{
				name: "a reason that sets code, target and category",
				path: []any{"blockers", 0, "reason"},
				want: []string{"category", "code", "target", "text"},
			},
			{name: "a blocker", path: []any{"blockers", 0}, want: []string{"reason", "resourceId"}},
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

	t.Run("resources", func(t *testing.T) {
		t.Parallel()

		// The expected content of the fixture's resources, in resource ID order,
		// as the Markdown report's report.golden shows it. ties and
		// requiredChanges are always set, to an empty slice where the resource has
		// none, because the format writes [] and never null: arrayAt above fails a
		// document that writes null for either.
		tests := []struct {
			id              string
			fit             string
			bestOffering    string
			ties            []string
			requiredChanges []string
			reasonCodes     []string
			reasonTexts     []string
		}{
			{
				id:              "bucket-example",
				fit:             "direct",
				bestOffering:    "Object Storage",
				ties:            []string{},
				requiredChanges: []string{},
				reasonCodes:     []string{"all-capabilities"},
				reasonTexts:     []string{"Object Storage offers every capability the resource uses"},
			},
			{
				id:              "db-example",
				fit:             "direct",
				bestOffering:    "Managed Database for PostgreSQL",
				ties:            []string{"Managed Database for PostgreSQL and MySQL"},
				requiredChanges: []string{},
				reasonCodes:     []string{"all-capabilities"},
				reasonTexts:     []string{"Managed Database for PostgreSQL offers every capability the resource uses"},
			},
			{
				id:              "fn-example",
				fit:             "none",
				ties:            []string{},
				requiredChanges: []string{},
				reasonCodes:     []string{"no-offering"},
				reasonTexts:     []string{"no offering in the Scaleway catalog for serverless.function"},
			},
			{
				id:              "i-0123456789abcdef0",
				fit:             "direct",
				bestOffering:    "Instances",
				ties:            []string{},
				requiredChanges: []string{},
				reasonCodes:     []string{"all-capabilities"},
				reasonTexts:     []string{"Instances offers every capability the resource uses"},
			},
			{
				id:              "misc-example",
				fit:             "none",
				ties:            []string{},
				requiredChanges: []string{},
				reasonCodes:     []string{"not-classified"},
				reasonTexts:     []string{"not classified"},
			},
			{
				id:              "partial-example",
				fit:             "partial",
				bestOffering:    "Instances",
				ties:            []string{},
				requiredChanges: []string{"node-high-availability", "snapshots"},
				reasonCodes:     []string{"missing-capability", "missing-capability"},
				reasonTexts: []string{
					"Instances does not offer node-high-availability",
					"Instances does not offer snapshots",
				},
			},
		}

		resources := arrayAt(t, document, "resources")
		if len(resources) != len(a.Verdicts) {
			t.Fatalf("the document has %d resources, the assessment has %d verdicts",
				len(resources), len(a.Verdicts))
		}

		if len(tests) != len(a.Verdicts) {
			t.Fatalf("the test lists %d resources, the assessment has %d verdicts",
				len(tests), len(a.Verdicts))
		}

		for i, test := range tests {
			t.Run(test.id, func(t *testing.T) {
				t.Parallel()

				at := []any{"resources", i}

				// The order is the assessment's verdict order.
				if got := valueAt(t, document, extend(at, "id")...); got != a.Verdicts[i].Resource.ID {
					t.Errorf("resources[%d].id = %v, want the assessment's verdict %q",
						i, got, a.Verdicts[i].Resource.ID)
				}

				if got := valueAt(t, document, extend(at, "fit")...); got != test.fit {
					t.Errorf("%q fit = %v, want %q", test.id, got, test.fit)
				}

				if got := valueAt(t, document, extend(at, "bestOffering")...); got != test.bestOffering {
					t.Errorf("%q bestOffering = %v, want %q", test.id, got, test.bestOffering)
				}

				if diff := cmp.Diff(test.ties, stringsAt(t, document, extend(at, "ties")...)); diff != "" {
					t.Errorf("%q ties mismatch (-want +got):\n%s", test.id, diff)
				}

				changes := stringsAt(t, document, extend(at, "requiredChanges")...)
				if diff := cmp.Diff(test.requiredChanges, changes); diff != "" {
					t.Errorf("%q requiredChanges mismatch (-want +got):\n%s", test.id, diff)
				}

				var codes, texts []string

				for j := range arrayAt(t, document, extend(at, "reasons")...) {
					atReason := extend(at, "reasons", j)

					code, ok := valueAt(t, document, extend(atReason, "code")...).(string)
					if !ok {
						t.Fatalf("%q reasons[%d].code is not a string", test.id, j)
					}

					text, ok := valueAt(t, document, extend(atReason, "text")...).(string)
					if !ok {
						t.Fatalf("%q reasons[%d].text is not a string", test.id, j)
					}

					codes = append(codes, code)
					texts = append(texts, text)
				}

				if diff := cmp.Diff(test.reasonCodes, codes); diff != "" {
					t.Errorf("%q reason codes mismatch (-want +got):\n%s", test.id, diff)
				}

				if diff := cmp.Diff(test.reasonTexts, texts); diff != "" {
					t.Errorf("%q reason texts mismatch (-want +got):\n%s", test.id, diff)
				}
			})
		}
	})

	t.Run("blockers", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			resourceID string
			code       string
			text       string
			target     string
			category   string
		}{
			{
				resourceID: "fn-example",
				code:       "no-offering",
				text:       "no offering in the Scaleway catalog for serverless.function",
				target:     scalewayTarget,
				category:   "serverless.function",
			},
			{resourceID: "misc-example", code: "not-classified", text: "not classified"},
		}

		blockers := arrayAt(t, document, "blockers")
		if len(blockers) != len(tests) {
			t.Fatalf("the document has %d blockers, want %d", len(blockers), len(tests))
		}

		for i, test := range tests {
			t.Run(test.resourceID, func(t *testing.T) {
				t.Parallel()

				at := []any{"blockers", i}
				if got := valueAt(t, document, extend(at, "resourceId")...); got != test.resourceID {
					t.Errorf("blockers[%d].resourceId = %v, want %q", i, got, test.resourceID)
				}

				reason := extend(at, "reason")
				if got := valueAt(t, document, extend(reason, "code")...); got != test.code {
					t.Errorf("%q blocker code = %v, want %q", test.resourceID, got, test.code)
				}

				if got := valueAt(t, document, extend(reason, "text")...); got != test.text {
					t.Errorf("%q blocker text = %v, want %q", test.resourceID, got, test.text)
				}

				// The target and the category are written only when the reason
				// sets them, which the key set above pins.
				if test.target != "" {
					if got := valueAt(t, document, extend(reason, "target")...); got != test.target {
						t.Errorf("%q blocker target = %v, want %q", test.resourceID, got, test.target)
					}
				}

				if test.category != "" {
					if got := valueAt(t, document, extend(reason, "category")...); got != test.category {
						t.Errorf("%q blocker category = %v, want %q", test.resourceID, got, test.category)
					}
				}
			})
		}
	})

	t.Run("an empty assessment keeps empty arrays", func(t *testing.T) {
		t.Parallel()

		empty := assessment.Assess(newInventory(t, sourceName, nil), scaleway)
		emptyDocument := decodeJSON(t, render(t, empty, analysis.ReportMeta{InventoryName: inventoryFileName}))

		for _, name := range []string{"resources", "blockers"} {
			array, ok := valueAt(t, emptyDocument, name).([]any)
			if !ok {
				t.Errorf("%q is not an array; an array is never null:\n%s", name, render(t, empty, analysis.ReportMeta{}))
			}

			if len(array) != 0 {
				t.Errorf("%q holds %d elements, want an empty array", name, len(array))
			}
		}

		want := map[string]any{
			"direct":  float64(0),
			"partial": float64(0),
			"none":    float64(0),
			"total":   float64(0),
		}
		if diff := cmp.Diff(want, valueAt(t, emptyDocument, "summary")); diff != "" {
			t.Errorf("the empty assessment's summary mismatch (-want +got):\n%s", diff)
		}
	})
}

// TestReportWritesEmptyValues pins the fields the format writes even when they
// hold nothing: an inventory that did not come from a file, a resource with no
// raw type and no region, a verdict without a best offering, and every array
// present and empty rather than null.
func TestReportWritesEmptyValues(t *testing.T) {
	t.Parallel()

	// The verdict holds no reasons, so the resources[0].reasons array the loop
	// below checks really is empty; the blocker still carries the reason.
	reason := mapping.Reason{Code: mapping.NotClassified}
	a := assessment.Assessment{
		Target:   scalewayTarget,
		Snapshot: testutil.ScalewaySnapshot(),
		Source:   sourceName,
		Verdicts: []assessment.Verdict{{
			Resource: inventory.Resource{
				ID:       "global-example",
				Category: taxonomy.StorageObject,
			},
			Fit: mapping.None,
		}},
		Blockers: []assessment.Blocker{{ResourceID: "global-example", Reason: reason}},
		Summary:  assessment.Summary{Total: 1, None: 1},
	}

	document := decodeJSON(t, render(t, a, analysis.ReportMeta{}))

	tests := []struct {
		name string
		path []any
		want string
	}{
		{name: "an inventory that is not a file has an empty name", path: []any{"inventoryName"}, want: ""},
		{name: "a resource without a raw type keeps the key", path: []any{"resources", 0, "rawType"}, want: ""},
		{name: "a resource without a region keeps the key", path: []any{"resources", 0, "region"}, want: ""},
		{name: "a verdict without a candidate has an empty best offering", path: []any{"resources", 0, "bestOffering"}, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(test.want, valueAt(t, document, test.path...)); diff != "" {
				t.Errorf("%v mismatch (-want +got):\n%s", test.path, diff)
			}
		})
	}

	for _, name := range []string{"ties", "requiredChanges", "reasons"} {
		array, ok := valueAt(t, document, "resources", 0, name).([]any)
		if !ok {
			t.Errorf("resources[0].%s is not an array; an array is never null", name)

			continue
		}

		if len(array) != 0 {
			t.Errorf("resources[0].%s holds %d elements, want an empty array", name, len(array))
		}
	}
}

// The hostile fixture: every value the document shows carries a quote, a
// backslash, a control character, a line ending, a tab, a line separator, a
// paragraph separator, an HTML end tag, an ampersand and a byte that is not valid
// UTF-8, so a report that concatenated strings instead of JSON-encoding them
// would not be valid JSON. The domain constructors refuse several of these
// values, so the assessment is built by hand, as the Markdown reporter's escaping
// tests build theirs.
var (
	// hostileInventoryName is the inventory's base name as the run reports it.
	hostileInventoryName = "inv\"\\\n\t\x00\x1b\u2028</script>\xff.json"
	// hostileSource is the inventory's source.
	hostileSource = "aws\"\\\n\t\x00\x1b\u2028</script>\xff"
	// hostileTarget is the catalog's display name.
	hostileTarget = "Evil \"target\" \\ \t\x00\x1b\u2028</script>\xff"
	// hostileSourceURL is the catalog snapshot's source.
	hostileSourceURL = "https://example.com/a\"b\\c\nd\te\x00\x1b\u2028</script>\xff"
	// hostileID, hostileRawType and hostileRegion are the direct resource's
	// values. hostileID also carries a paragraph separator and an ampersand, the
	// characters JSON escapes besides < and >.
	hostileID      = "i-\"1\"\\x\n\t\x00\x1b\u2028\u2029</script>&x\xff"
	hostileRawType = "AWS::\"X\"\\Y\t\x00\x1b\u2028</script>\xff"
	hostileRegion  = "eu-\"west\"\\1\n\x00\x1b\u2028</script>\xff"
	// hostileOffering is the best offering's name, and hostileTie the name of the
	// other candidate the direct verdict ties with.
	hostileOffering = "Off\"er\\ing\x00\x1b\u2028</script>\xff"
	hostileTie      = "Tie\"\\\n\t\x00\x1b\u2028</script>\xff"
	// hostileCapability is the capability the partial verdict's offering lacks.
	hostileCapability = taxonomy.Capability("cap\"a\\b\n\t\x00\x1b\u2028</script>\xff")
	// hostileReasonCode is a code the domain does not know, so the reason's text
	// is the code itself.
	hostileReasonCode = mapping.ReasonCode("reason\"\\\n\t\x00\x1b\u2028</script>\xff")
	// hostileBlockedID is the blocked resource's ID, which the blocker repeats.
	hostileBlockedID = "fn\"\\\n\t\x00\x1b\u2028</script>\xff"
)

// hostileCategory is the category of the hostile resources: a value the
// document writes as given.
func hostileCategory() taxonomy.Category {
	return taxonomy.Category("cat\"\\\n\t\x00\x1b\u2028</script>\xff")
}

// hostileMetadata is the metadata of the hostile document.
func hostileMetadata() analysis.ReportMeta {
	return analysis.ReportMeta{InventoryName: hostileInventoryName}
}

// hostileAssessment is an assessment whose every text value is hostile: a direct
// verdict with two tied candidates, a partial verdict with a missing capability,
// and a blocked verdict with an unknown reason code and a no-offering reason
// that repeats the hostile target and category. Its blockers repeat the blocked
// resource's hostile ID.
func hostileAssessment() assessment.Assessment {
	category := hostileCategory()

	noOffering := mapping.Reason{Code: mapping.NoOffering, Target: hostileTarget, Category: category}

	return assessment.Assessment{
		Target:   hostileTarget,
		Snapshot: catalog.Snapshot{SourceURL: hostileSourceURL, RetrievedOn: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)},
		Source:   hostileSource,
		Verdicts: []assessment.Verdict{
			{
				Resource: inventory.Resource{
					ID:           hostileID,
					RawType:      hostileRawType,
					Category:     category,
					Region:       hostileRegion,
					Capabilities: []taxonomy.Capability{hostileCapability},
				},
				Fit:          mapping.Direct,
				BestOffering: hostileOffering,
				Candidates: []mapping.Candidate{
					{
						Offering: catalog.Offering{Name: hostileOffering, Categories: []taxonomy.Category{category}},
						Fit:      mapping.Direct,
					},
					{
						Offering: catalog.Offering{Name: hostileTie, Categories: []taxonomy.Category{category}},
						Fit:      mapping.Direct,
					},
				},
				Reasons: []mapping.Reason{{Code: mapping.AllCapabilities, Offering: hostileOffering}},
			},
			{
				Resource: inventory.Resource{
					ID:       "partial-example",
					RawType:  "AWS::EC2::Instance",
					Category: category,
				},
				Fit:          mapping.Partial,
				BestOffering: hostileOffering,
				Reasons: []mapping.Reason{{
					Code:       mapping.MissingCapability,
					Offering:   hostileOffering,
					Capability: hostileCapability,
				}},
				RequiredChanges: []taxonomy.Capability{hostileCapability},
			},
			{
				Resource: inventory.Resource{
					ID:       hostileBlockedID,
					RawType:  hostileRawType,
					Category: category,
					Region:   hostileRegion,
				},
				Fit:     mapping.None,
				Reasons: []mapping.Reason{{Code: hostileReasonCode}, noOffering},
			},
		},
		Blockers: []assessment.Blocker{
			{ResourceID: hostileBlockedID, Reason: mapping.Reason{Code: hostileReasonCode}},
			{ResourceID: hostileBlockedID, Reason: noOffering},
		},
		Summary: assessment.Summary{Total: 3, Direct: 1, Partial: 1, None: 1},
	}
}

// TestReportHostileValues pins that no value reaches the document unencoded: the
// output is valid JSON and valid UTF-8, and decoding it gives every value back,
// with each invalid UTF-8 byte replaced by U+FFFD as encoding/json does.
func TestReportHostileValues(t *testing.T) {
	t.Parallel()

	got := render(t, hostileAssessment(), hostileMetadata())

	testutil.Golden(t, "testdata/report-hostile.golden", got)
	checkDocument(t, got)
	checkJSONEscapes(t, got)

	document := decodeJSON(t, got)

	// want is the string the document holds before JSON encoding: the hostile
	// value as the assessment carries it, or the sentence the format builds from
	// it.
	tests := []struct {
		name string
		path []any
		want string
	}{
		{name: "inventory name", path: []any{"inventoryName"}, want: hostileInventoryName},
		{name: "source", path: []any{"source"}, want: hostileSource},
		{name: "target name", path: []any{"target", "name"}, want: hostileTarget},
		{name: "snapshot source", path: []any{"target", "snapshot", "source"}, want: hostileSourceURL},
		{name: "resource ID", path: []any{"resources", 0, "id"}, want: hostileID},
		{name: "raw type", path: []any{"resources", 0, "rawType"}, want: hostileRawType},
		{name: "region", path: []any{"resources", 0, "region"}, want: hostileRegion},
		{name: "category", path: []any{"resources", 0, "category"}, want: string(hostileCategory())},
		{name: "best offering", path: []any{"resources", 0, "bestOffering"}, want: hostileOffering},
		{name: "tie", path: []any{"resources", 0, "ties", 0}, want: hostileTie},
		{
			name: "required change",
			path: []any{"resources", 1, "requiredChanges", 0},
			want: string(hostileCapability),
		},
		{
			name: "missing capability reason text",
			path: []any{"resources", 1, "reasons", 0, "text"},
			want: hostileOffering + " does not offer " + string(hostileCapability),
		},
		{
			name: "unknown reason code",
			path: []any{"resources", 2, "reasons", 0, "code"},
			want: string(hostileReasonCode),
		},
		{
			name: "unknown reason text is the code",
			path: []any{"resources", 2, "reasons", 0, "text"},
			want: string(hostileReasonCode),
		},
		{
			name: "no-offering reason text",
			path: []any{"resources", 2, "reasons", 1, "text"},
			want: "no offering in the " + hostileTarget + " catalog for " + string(hostileCategory()),
		},
		{
			name: "no-offering reason target",
			path: []any{"resources", 2, "reasons", 1, "target"},
			want: hostileTarget,
		},
		{
			name: "no-offering reason category",
			path: []any{"resources", 2, "reasons", 1, "category"},
			want: string(hostileCategory()),
		},
		{name: "blocker resource ID", path: []any{"blockers", 0, "resourceId"}, want: hostileBlockedID},
		{
			name: "blocker reason text",
			path: []any{"blockers", 0, "reason", "text"},
			want: string(hostileReasonCode),
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

// TestReportIsDeterministic pins that the same assessment always renders the
// same bytes: a document never depends on the clock, the environment or map
// iteration.
func TestReportIsDeterministic(t *testing.T) {
	t.Parallel()

	scaleway := testutil.ScalewayCatalog(t)
	meta := analysis.ReportMeta{InventoryName: inventoryFileName}

	first := render(t, assessment.Assess(newInventory(t, sourceName, awsResources()), scaleway), meta)
	second := render(t, assessment.Assess(newInventory(t, sourceName, awsResources()), scaleway), meta)

	if !bytes.Equal(first, second) {
		t.Errorf("two documents of the same assessment differ:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// failingWriter is a writer that always fails, so a test can see that Report
// reports a write failure instead of swallowing it.
type failingWriter struct {
	err error
}

// Write returns the writer's error and writes nothing.
func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

// TestReportReturnsWriteError pins that a failing writer is reported to the
// caller, wrapped or as it came, never ignored: the driving adapter decides what
// to do with a partial document.
func TestReportReturnsWriteError(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("disk is full")
	a := assessment.Assess(newInventory(t, sourceName, awsResources()), testutil.ScalewayCatalog(t))

	err := jsonreport.New().Report(t.Context(), failingWriter{err: writeErr}, a,
		analysis.ReportMeta{InventoryName: inventoryFileName})
	if !errors.Is(err, writeErr) {
		t.Fatalf("Report() error = %v, want an error wrapping %v", err, writeErr)
	}
}

// TestReportsCancelledContext pins that a cancelled context stops both documents
// before anything is encoded: the caller gets the cancellation, and the writer
// never sees a report the caller has stopped waiting for.
func TestReportsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	a := assessment.Assess(newInventory(t, sourceName, awsResources()), testutil.ScalewayCatalog(t))
	c := assessment.Compare(assessTargets(t, comparisonInventory(t), threeTargetKeys()))
	meta := analysis.ReportMeta{InventoryName: inventoryFileName}

	tests := []struct {
		name  string
		write func(io.Writer) error
	}{
		{
			name: "the single-target document",
			write: func(w io.Writer) error {
				return jsonreport.New().Report(ctx, w, a, meta)
			},
		},
		{
			name: "the comparison document",
			write: func(w io.Writer) error {
				return jsonreport.New().ReportComparison(ctx, w, c, meta)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer

			if err := tc.write(&out); !errors.Is(err, context.Canceled) {
				t.Fatalf("%s: error = %v, want an error wrapping context.Canceled", tc.name, err)
			}

			if out.Len() != 0 {
				t.Errorf("%s wrote %d bytes to the writer, want none:\n%s", tc.name, out.Len(), out.String())
			}
		})
	}
}

// checkJSONEscapes asserts that the document holds the escaped form of the
// characters a reader could act on and never the raw character: the golden is
// read by a browser and a log reader as well as by a JSON parser, so U+2029 and &
// must reach it as the escapes encoding/json writes.
func checkJSONEscapes(t *testing.T, out []byte) {
	t.Helper()

	for _, escaped := range []string{`\u2029`, `\u0026`} {
		if !bytes.Contains(out, []byte(escaped)) {
			t.Errorf("the document does not hold the escape %s:\n%s", escaped, out)
		}
	}

	for _, raw := range []string{"\u2029", "&"} {
		if bytes.Contains(out, []byte(raw)) {
			t.Errorf("the document holds the raw character %q:\n%s", raw, out)
		}
	}
}

// decodeJSON decodes data into the generic form encoding/json produces, which is
// how a test sees the shape of the document itself rather than of a Go struct.
func decodeJSON(tb testing.TB, data []byte) map[string]any {
	tb.Helper()

	if !json.Valid(data) {
		tb.Fatalf("the document is not valid JSON:\n%s", data)
	}

	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		tb.Fatalf("the document is not a JSON object: %v\n%s", err, data)
	}

	return document
}

// valueAt returns the value at path in document, where a string is an object key
// and an int an array index. It fails the test when the path is missing, because
// a document without a field the format promises is a failure of its own.
func valueAt(tb testing.TB, document any, path ...any) any {
	tb.Helper()

	at := "$"

	for _, step := range path {
		switch key := step.(type) {
		case string:
			object, ok := document.(map[string]any)
			if !ok {
				tb.Fatalf("%s is %T, not an object, so %q cannot be read from it", at, document, key)
			}

			value, ok := object[key]
			if !ok {
				tb.Fatalf("%s holds no %q key", at, key)
			}

			document = value
			at += "." + key
		case int:
			array, ok := document.([]any)
			if !ok {
				tb.Fatalf("%s is %T, not an array, so index %d cannot be read from it", at, document, key)
			}

			if key < 0 || key >= len(array) {
				tb.Fatalf("%s holds %d elements, so index %d cannot be read from it", at, len(array), key)
			}

			document = array[key]
			at += "[" + strconv.Itoa(key) + "]"
		default:
			tb.Fatalf("unsupported path step %T", step)
		}
	}

	return document
}

// arrayAt returns the array at path in document.
func arrayAt(tb testing.TB, document any, path ...any) []any {
	tb.Helper()

	value := valueAt(tb, document, path...)

	array, ok := value.([]any)
	if !ok {
		tb.Fatalf("%v is %T, want an array", path, value)
	}

	return array
}

// stringsAt returns the string array at path in document.
func stringsAt(tb testing.TB, document any, path ...any) []string {
	tb.Helper()

	array := arrayAt(tb, document, path...)
	out := make([]string, 0, len(array))

	for i, element := range array {
		str, ok := element.(string)
		if !ok {
			tb.Fatalf("%v[%d] is %T, want a string", path, i, element)
		}

		out = append(out, str)
	}

	return out
}

// objectKeysAt returns the keys of the object at path in document, sorted, so a
// test can assert the key set the format defines.
func objectKeysAt(tb testing.TB, document any, path ...any) []string {
	tb.Helper()

	object, ok := valueAt(tb, document, path...).(map[string]any)
	if !ok {
		tb.Fatalf("%v is not an object", path)
	}

	return slices.Sorted(maps.Keys(object))
}

// extend returns base with steps appended, so a subtest can build a path from one
// it shares without changing that one.
func extend(base []any, steps ...any) []any {
	return append(slices.Clone(base), steps...)
}

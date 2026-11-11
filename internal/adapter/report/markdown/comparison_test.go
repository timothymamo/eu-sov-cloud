package markdown_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/report/markdown"
	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/testutil"
)

// The reporter must be usable wherever the comparison use case expects a
// ComparisonReporter: the golden files pin its output, this pins the shape of
// the port it renders. It is a second port (docs/adr/0016), so a reporter may
// implement Reporter without implementing this one.
var _ analysis.ComparisonReporter = markdown.New()

// The keys the comparison fixtures select their targets with: the CLI's keys,
// lower case and one word, not the catalogs' display names.
const (
	ovhcloudKey = "ovhcloud"
	scalewayKey = "scaleway"
	upcloudKey  = "upcloud"
)

// comparisonResources is the inventory the three-target golden assesses, chosen
// so the comparison has to tell four things apart:
//
//   - bucket-example is direct on OVHcloud and Scaleway and partial on UpCloud,
//     whose object storage lacks versioning;
//   - db-example is direct on OVHcloud and Scaleway, with a tie on Scaleway, and
//     blocked on UpCloud only, which has no database offering;
//   - fn-example and misc-example are blocked on every target, one for a category
//     no target serves and one for being unclassified, so they are the shared
//     blockers;
//   - partial-example is partial on every target, with a tie on Scaleway.
//
// The IDs sort into the order the tables must use, which is also the order the
// shared blockers must be listed in.
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

// twoTargetResources is the inventory the two-target golden assesses: OVHcloud
// and Scaleway both serve everything in it, so the comparison has no blocker on
// every target and its shared-blocker section says "None.".
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

// comparisonInventory is the three-target golden's inventory.
func comparisonInventory(t *testing.T) inventory.Inventory {
	t.Helper()

	return newInventory(t, sourceName, comparisonResources())
}

// comparisonFixtureCatalog returns the fixture catalog of one target key. Each
// catalog has its own target display name, source URL and retrieval date, so the
// report has to keep three targets apart.
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

// threeTargetKeys is the target list the three-target golden compares.
func threeTargetKeys() []string {
	return []string{ovhcloudKey, scalewayKey, upcloudKey}
}

// renderComparison writes the comparison report of c and meta and returns it.
func renderComparison(tb testing.TB, c assessment.Comparison, meta analysis.ReportMeta) []byte {
	tb.Helper()

	var out bytes.Buffer
	if err := markdown.New().ReportComparison(tb.Context(), &out, c, meta); err != nil {
		tb.Fatalf("ReportComparison() returned unexpected error: %v", err)
	}

	return out.Bytes()
}

// TestReportComparison pins the whole comparison against the fixture catalogs:
// the title naming every target, one catalog line per target with that target's
// own source URL and retrieval date, the comparison (a summary table, one table
// row per resource with each target's fit and best offering, and the blockers
// every target shares) and then one section per target with that target's
// summary, resources, required changes and blockers.
//
// The tie note the single-target table appends is left out of the comparison
// table, which the Scaleway section keeps, and the shared-blocker section says
// "None." when no resource is blocked on every target.
func TestReportComparison(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		inventory  inventory.Inventory
		keys       []string
		goldenFile string
	}{
		{
			name:       "three targets with blockers every target shares",
			inventory:  comparisonInventory(t),
			keys:       threeTargetKeys(),
			goldenFile: "testdata/comparison-three.golden",
		},
		{
			name:       "two targets without a shared blocker say None",
			inventory:  newInventory(t, sourceName, twoTargetResources()),
			keys:       []string{ovhcloudKey, scalewayKey},
			goldenFile: "testdata/comparison-two.golden",
		},
	}

	meta := analysis.ReportMeta{InventoryName: inventoryFileName}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := renderComparison(t, assessment.Compare(assessTargets(t, test.inventory, test.keys)), meta)
			testutil.Golden(t, test.goldenFile, got)

			checkTableShape(t, got)
		})
	}
}

// TestReportComparisonNoInventoryName pins that the inventory file line is left
// out of the comparison header when the metadata holds no file name, exactly as
// the single-target report leaves it out.
func TestReportComparisonNoInventoryName(t *testing.T) {
	t.Parallel()

	c := assessment.Compare(assessTargets(t, comparisonInventory(t), threeTargetKeys()))

	out := string(renderComparison(t, c, analysis.ReportMeta{}))
	if strings.Contains(out, "- Inventory file:") {
		t.Errorf("the comparison shows an inventory file line although the metadata holds no name:\n%s", out)
	}

	if got := lineWithPrefix(t, []byte(out), "- Source: "); got != "- Source: `aws`" {
		t.Errorf("source line = %q, want %q", got, "- Source: `aws`")
	}
}

// TestReportComparisonHostileValues pins that no value a renderer would read as
// syntax can end a line, a heading or a table row: every value from the
// comparison (keys, target display names, resource IDs, categories, raw types,
// offering names, reason text, the inventory file name, the source and the
// snapshot URLs) is a code span, and a line ending inside a code span is a space.
// Table cell counts are checked with checkTableShape, which a forged cell
// boundary would break.
func TestReportComparisonHostileValues(t *testing.T) {
	t.Parallel()

	c := assessment.Compare(hostileComparisonTargets())

	out := renderComparison(t, c, analysis.ReportMeta{InventoryName: hostileComparisonInventoryName})
	testutil.Golden(t, "testdata/comparison-hostile.golden", out)

	checkTableShape(t, out)

	for _, value := range hostileComparisonValues() {
		if strings.Contains(string(out), value) {
			t.Errorf("the comparison contains the raw value %q, so a line ending was not escaped:\n%s", value, out)
		}
	}
}

// TestReportComparisonSectionsReuseReport pins that each target's section is
// that target's single-target report, rendered by Report with the same metadata,
// with the title and header lines the comparison already gave removed and every
// heading one level lower: same content, same row order. The expectation is
// derived from Report, not written down twice.
func TestReportComparisonSectionsReuseReport(t *testing.T) {
	t.Parallel()

	meta := analysis.ReportMeta{InventoryName: inventoryFileName}

	targets := assessTargets(t, comparisonInventory(t), threeTargetKeys())

	// Compare sorts by key, so the comparison's sections do not depend on the
	// order the targets were handed over in.
	c := assessment.Compare([]assessment.TargetAssessment{targets[2], targets[0], targets[1]})

	out := renderComparison(t, c, meta)

	for _, target := range c.Targets {
		want := demotedReportLines(t, string(render(t, target.Assessment, meta)))
		got := sectionLines(t, out, targetSectionHeading(target.Assessment.Target))

		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("the %q section is not that target's report one heading level lower (-want +got):\n%s",
				target.Key, diff)
		}
	}
}

// targetSectionHeading returns the comparison heading of one target's section.
// The fixture display names hold no Markdown metacharacter, so the code span of a
// name is the name between two backticks.
func targetSectionHeading(displayName string) string {
	return "## `" + displayName + "`"
}

// demotedReportLines turns a single-target report into the body its comparison
// section must hold: the title and the header lines are replaced by the
// comparison's own header, and every heading moves one level down. Blank lines
// are dropped, because the golden files pin the blank-line layout.
func demotedReportLines(t *testing.T, singleTargetReport string) []string {
	t.Helper()

	lines := strings.Split(singleTargetReport, "\n")

	// The body starts at the report's first section heading: everything before it
	// is the title and the header lines.
	start := slices.IndexFunc(lines, func(line string) bool {
		return strings.HasPrefix(line, "## ")
	})
	if start < 0 {
		t.Fatalf("the single-target report has no section heading:\n%s", singleTargetReport)
	}

	demoted := make([]string, 0, len(lines)-start)

	for _, line := range lines[start:] {
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "#") {
			line = "#" + line
		}

		demoted = append(demoted, line)
	}

	return demoted
}

// TestReportComparisonHasNoRanking pins that a comparison states facts per target
// and does not rank them or recommend one: the words a ranking would use do not
// appear, while "best offering", a per-resource fact, stays.
func TestReportComparisonHasNoRanking(t *testing.T) {
	t.Parallel()

	c := assessment.Compare(assessTargets(t, comparisonInventory(t), threeTargetKeys()))

	out := renderComparison(t, c, analysis.ReportMeta{InventoryName: inventoryFileName})
	lowered := strings.ToLower(string(out))

	for _, word := range []string{"best target", "recommend", "winner"} {
		if strings.Contains(lowered, word) {
			t.Errorf("the comparison holds the ranking word %q:\n%s", word, out)
		}
	}

	if !strings.Contains(lowered, "best offering") {
		t.Errorf("the comparison does not name each target's best offering:\n%s", out)
	}
}

// TestReportComparisonIsDeterministic pins that the same comparison always
// renders the same bytes, and that the order the targets were assessed in does
// not reach the output: Compare sorts them by key.
func TestReportComparisonIsDeterministic(t *testing.T) {
	t.Parallel()

	meta := analysis.ReportMeta{InventoryName: inventoryFileName}

	first := renderComparison(t, assessment.Compare(assessTargets(t, comparisonInventory(t), threeTargetKeys())), meta)
	second := renderComparison(t, assessment.Compare(assessTargets(t, comparisonInventory(t), threeTargetKeys())), meta)

	if !bytes.Equal(first, second) {
		t.Errorf("two renders of the same comparison differ:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	reversedKeys := threeTargetKeys()
	slices.Reverse(reversedKeys)

	reordered := renderComparison(t, assessment.Compare(assessTargets(t, comparisonInventory(t), reversedKeys)), meta)
	if !bytes.Equal(first, reordered) {
		t.Errorf("the comparison depends on the order the targets were given in:\nfirst:\n%s\nreordered:\n%s",
			first, reordered)
	}
}

// TestReportComparisonCancelledContext pins that a cancelled context stops
// ReportComparison before anything is rendered: the caller gets the cancellation,
// and the writer never sees a report the caller has stopped waiting for.
func TestReportComparisonCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	c := assessment.Compare(assessTargets(t, comparisonInventory(t), threeTargetKeys()))

	var out bytes.Buffer

	err := markdown.New().ReportComparison(ctx, &out, c, analysis.ReportMeta{InventoryName: inventoryFileName})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ReportComparison() error = %v, want an error wrapping context.Canceled", err)
	}

	if out.Len() != 0 {
		t.Errorf("ReportComparison() wrote %d bytes to the writer, want none:\n%s", out.Len(), out.String())
	}
}

// TestReportComparisonReturnsWriteError pins that a failing writer is reported to
// the caller, wrapped or as it came, never ignored: the driving adapter decides
// what to do with a partial report.
func TestReportComparisonReturnsWriteError(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("disk is full")

	c := assessment.Compare(assessTargets(t, comparisonInventory(t), threeTargetKeys()))

	err := markdown.New().ReportComparison(t.Context(), failingWriter{err: writeErr}, c,
		analysis.ReportMeta{InventoryName: inventoryFileName})
	if !errors.Is(err, writeErr) {
		t.Fatalf("ReportComparison() error = %v, want an error wrapping %v", err, writeErr)
	}
}

// The hostile comparison fixture: every value the report shows would be syntax in
// a renderer, and each carries a pipe, an angle bracket, a backtick or a line
// ending. The domain constructors refuse a line ending in a value, so the
// assessments are built by hand, as the single-target escaping tests do.
const (
	// hostileComparisonInventoryName is a file name with a pipe, a backtick, a
	// link and a line ending.
	hostileComparisonInventoryName = "inv|`x`\n[y](z).json"
	// hostileComparisonSource is an inventory source that would start a heading if
	// it were not a code span, and carries a pipe, a backtick and a line ending.
	hostileComparisonSource = "aws|`src`\n# heading"
	// hostileComparisonName1, 2 and 3 are the targets' display names, and
	// hostileComparisonOffering is an offering name: each carries the characters
	// that end a table cell, start HTML, open a code span or end a line.
	hostileComparisonName1 = "Evil | `x` <b> [a](b) *s*"
	hostileComparisonName2 = "Tick\n`Name`"
	hostileComparisonName3 = "<img src=x> | `t`"
	// hostileComparisonOffering is the best offering of the direct verdict.
	hostileComparisonOffering = "`tick` [row](x) | Name"
	// hostileComparisonReason is a reason code the domain does not know, so it is
	// rendered as escaped literal text, with a pipe, a backtick and a line ending.
	hostileComparisonReason = "<b>x|`y`\nz"
)

// hostileComparisonCategory is a taxonomy category the fixture holds: it carries
// a pipe, a backtick, an angle bracket and a line ending.
func hostileComparisonCategory() taxonomy.Category {
	return taxonomy.Category("store|age`<x>\nz")
}

// hostileComparisonTargets builds the three fixed target assessments of the
// hostile golden, named by hostile keys, in key order.
//
// One resource is blocked on every target for a category no target serves, with
// a reason that names that target, one is blocked on every target for being
// unclassified, and one is direct, partial and none on the three targets, so the
// comparison has both a shared blocker list and a merely blocked resource.
func hostileComparisonTargets() []assessment.TargetAssessment {
	names := []string{hostileComparisonName1, hostileComparisonName2, hostileComparisonName3}
	keys := []string{"a|b", "k`ey\nx", "z<z"}
	urls := []string{
		"https://example.com/a|b`c`<d>\ne",
		"https://example.com/|`url`\n<x>",
		"https://example.com/<x>|`u`\nv",
	}
	retrieved := []time.Time{
		time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC),
	}

	targets := make([]assessment.TargetAssessment, 0, len(names))

	for i, name := range names {
		a := assessment.Assessment{
			Target:   name,
			Snapshot: catalog.Snapshot{SourceURL: urls[i], RetrievedOn: retrieved[i]},
			Source:   hostileComparisonSource,
			Verdicts: []assessment.Verdict{
				hostileNoOfferingVerdict(name),
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

		targets = append(targets, assessment.TargetAssessment{Key: keys[i], Assessment: a})
	}

	return targets
}

// hostileNoOfferingVerdict is the verdict of the resource that is blocked on
// target displayName for a category no target serves, with a hostile ID, raw type
// and category.
func hostileNoOfferingVerdict(displayName string) assessment.Verdict {
	return assessment.Verdict{
		Resource: inventory.Resource{
			ID:       "`tick`",
			RawType:  "AWS::X|<y>`\n",
			Category: hostileComparisonCategory(),
		},
		Fit:     mapping.None,
		Reasons: []mapping.Reason{{Code: mapping.NoOffering, Target: displayName, Category: hostileComparisonCategory()}},
	}
}

// hostileUnclassifiedVerdict is the verdict of the second resource blocked on
// every target.
func hostileUnclassifiedVerdict() assessment.Verdict {
	return assessment.Verdict{
		Resource: inventory.Resource{
			ID:       "a`b",
			RawType:  "AWS::Y",
			Category: taxonomy.Unclassified,
		},
		Fit:     mapping.None,
		Reasons: []mapping.Reason{{Code: mapping.NotClassified}},
	}
}

// hostileVaryingVerdict is the verdict of the resource that is direct on the
// first target, partial on the second and none on the third, so the comparison
// table holds a hostile offering name, a hostile missing capability and a hostile
// literal reason.
func hostileVaryingVerdict(target int) assessment.Verdict {
	resource := inventory.Resource{
		ID:       "i-1 | x",
		RawType:  "AWS::EC2::Instance",
		Category: taxonomy.ComputeVM,
	}

	switch target {
	case 0:
		return assessment.Verdict{
			Resource:     resource,
			Fit:          mapping.Direct,
			BestOffering: hostileComparisonOffering,
			Candidates: []mapping.Candidate{{
				Offering: catalog.Offering{
					Name:       hostileComparisonOffering,
					Categories: []taxonomy.Category{taxonomy.ComputeVM},
				},
				Fit: mapping.Direct,
			}},
			Reasons: []mapping.Reason{{Code: mapping.AllCapabilities, Offering: hostileComparisonOffering}},
		}
	case 1:
		return assessment.Verdict{
			Resource:        resource,
			Fit:             mapping.Partial,
			BestOffering:    "<b>Offering</b>",
			Reasons:         []mapping.Reason{{Code: mapping.MissingCapability, Offering: "<b>Offering</b>", Capability: taxonomy.Snapshots}},
			RequiredChanges: []taxonomy.Capability{taxonomy.Snapshots},
		}
	default:
		return assessment.Verdict{
			Resource: resource,
			Fit:      mapping.None,
			Reasons:  []mapping.Reason{{Code: mapping.ReasonCode(hostileComparisonReason)}},
		}
	}
}

// hostileComparisonValues returns every fixture value that carries a line ending,
// which no report may show raw: a value that kept one would end the line, the
// heading or the table row it is in.
func hostileComparisonValues() []string {
	return []string{
		hostileComparisonInventoryName,
		hostileComparisonSource,
		hostileComparisonName2,
		"k`ey\nx",
		"https://example.com/a|b`c`<d>\ne",
		"https://example.com/|`url`\n<x>",
		"https://example.com/<x>|`u`\nv",
		string(hostileComparisonCategory()),
		"AWS::X|<y>`\n",
		hostileComparisonReason,
	}
}

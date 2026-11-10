package markdown_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/report/markdown"
	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/testutil"
)

// The reporter must be usable wherever the use case expects a Reporter: the
// golden files pin its output, this pins the shape of the port it renders.
var _ analysis.Reporter = markdown.New()

const (
	// inventoryFileName is the base name of the fixture inventory file, and what
	// a report shows above the summary.
	inventoryFileName = "inventory.json"
	// sourceName is the source the ordinary fixture inventory was taken from.
	sourceName = "aws"
	// hostileSource is an inventory source full of Markdown metacharacters: it
	// must be escaped like every other text value, so a source cannot forge a
	// heading.
	hostileSource = "# heading"
	// hostileTarget is a catalog target whose name carries every Markdown
	// metacharacter the reporter escapes. It is also the name of one offering.
	hostileTarget = "Evil | `x` <b> [a](b) *s*"
	// hostileOffering is an offering name with the same characters as
	// hostileTarget, in a different order.
	hostileOffering = "`tick` [row](x) | Name"
	// hostileInventoryName is a file name that must be escaped before a report
	// shows it.
	hostileInventoryName = "inv|`x`[y](z).json"
	// hostileSourceURL is a catalog source URL with parentheses, a stored newline
	// escape and a stored right-to-left override escape. The reporter emits the
	// stored string, never a decoded form, so "%0A" and "%E2%80%AE" appear as
	// given.
	hostileSourceURL = "https://example.com/a(b)%0A%E2%80%AE"
)

// awsResources is one inventory covering every fit: direct, partial, none for a
// category no offering serves, and unclassified. The IDs sort into the order the
// report's table and blockers section must use.
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
			// Two missing capabilities, so the reason cell joins two sentences.
			ID:           "partial-example",
			RawType:      "AWS::EC2::Instance",
			Category:     taxonomy.ComputeVM,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.HighAvailability, taxonomy.Snapshots},
		},
	}
}

// hostileResources is an inventory whose every text value carries Markdown
// metacharacters: an ID with a stored pipe, an ID with link syntax, an ID with a
// backslash, a raw type with HTML and a source that looks like a heading. The
// raw type also carries dots, which are not Markdown metacharacters here and so
// must stay unescaped.
func hostileResources() []inventory.Resource {
	return []inventory.Resource{
		{
			ID:       "[l](javascript:x)",
			RawType:  "`tick`",
			Category: taxonomy.ServerlessFunction,
			Region:   "eu-west-1",
		},
		{
			ID:       "`tick`",
			RawType:  "a*b_c",
			Category: taxonomy.Unclassified,
			Region:   "eu-west-1",
		},
		{
			ID:           "a*b_c",
			RawType:      "AWS::X.Y <img src=x>",
			Category:     taxonomy.ComputeVM,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.IPv6},
		},
		{
			ID:           `back\slash`,
			RawType:      "# heading",
			Category:     taxonomy.StorageObject,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.ObjectVersioning},
		},
		{
			ID:       "i-1 | x",
			RawType:  "[l](javascript:x)",
			Category: taxonomy.ComputeVM,
			Region:   "eu-west-1",
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

// hostileCatalog is a catalog whose target, offering names and source URL carry
// Markdown metacharacters. Its retrieval date is in a zone east of UTC and just
// after midnight, so a report that forgets to format it in UTC shows the
// previous day.
func hostileCatalog(t *testing.T) catalog.Catalog {
	t.Helper()

	cat, err := catalog.New(hostileTarget, catalog.Snapshot{
		SourceURL:   hostileSourceURL,
		RetrievedOn: time.Date(2026, 11, 2, 0, 30, 0, 0, time.FixedZone("CET", 60*60)),
	}, []catalog.Offering{
		{
			Name:         hostileTarget,
			Categories:   []taxonomy.Category{taxonomy.ComputeVM},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6},
			Regions:      []string{"fr-par"},
		},
		{
			Name:         hostileOffering,
			Categories:   []taxonomy.Category{taxonomy.StorageObject},
			Capabilities: []taxonomy.Capability{taxonomy.IPv6},
			Regions:      []string{"fr-par"},
		},
	})
	if err != nil {
		t.Fatalf("catalog.New() returned unexpected error: %v", err)
	}

	return cat
}

// render writes the report of a and meta and returns it.
func render(tba testing.TB, a assessment.Assessment, meta analysis.ReportMeta) []byte {
	tba.Helper()

	var out bytes.Buffer
	if err := markdown.New().Report(tba.Context(), &out, a, meta); err != nil {
		tba.Fatalf("Report() returned unexpected error: %v", err)
	}

	return out.Bytes()
}

// TestReport pins the whole rendering against the Scaleway fixture catalog: the
// header, the summary, one table row per resource and the blockers.
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
			name:       "no inventory file name omits the line",
			inventory:  newInventory(t, sourceName, awsResources()),
			meta:       analysis.ReportMeta{},
			goldenFile: "testdata/report-no-inventory-name.golden",
		},
		{
			name:       "empty inventory keeps the header rows and no blockers",
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
		})
	}
}

// TestReportHostileValues pins the escaping of every untrusted value: a value
// with a pipe, a backtick, HTML, link syntax or a star may neither end a table
// cell nor start a heading, and the catalog's source URL is emitted as stored,
// percent-encoded only where a Markdown autolink needs it.
func TestReportHostileValues(t *testing.T) {
	t.Parallel()

	a := assessment.Assess(newInventory(t, hostileSource, hostileResources()), hostileCatalog(t))
	got := render(t, a, analysis.ReportMeta{InventoryName: hostileInventoryName})

	testutil.Golden(t, "testdata/report-hostile.golden", got)

	checkTableShape(t, got)
	checkHeadings(t, got)
}

// checkTableShape asserts that every row of every table has as many cells as its
// header: a value that forged a cell boundary would break the row count.
func checkTableShape(t *testing.T, out []byte) {
	t.Helper()

	lines := strings.Split(string(out), "\n")
	columns := -1

	for i, line := range lines {
		if !strings.HasPrefix(line, "|") {
			columns = -1
			continue
		}

		cells := tableCells(line)

		if isSeparatorRow(cells) {
			continue
		}

		if columns < 0 {
			if i+1 >= len(lines) || !isSeparatorRow(tableCells(lines[i+1])) {
				t.Errorf("line %d: a table starts without a header and its separator: %q", i+1, line)
			}

			columns = len(cells)

			continue
		}

		if len(cells) != columns {
			t.Errorf("line %d: %d cells, the header has %d: %q", i+1, len(cells), columns, line)
		}
	}
}

// checkHeadings asserts that the only lines that start a Markdown heading are the
// title and the three section headings, so no value can forge one.
func checkHeadings(t *testing.T, out []byte) {
	t.Helper()

	var headings []string

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "#") {
			headings = append(headings, line)
		}
	}

	want := []string{
		"# Migration assessment: Evil \\| \\`x\\` \\<b\\> \\[a\\]\\(b\\) \\*s\\*",
		"## Summary",
		"## Resources",
		"## Blockers",
	}

	if !slices.Equal(headings, want) {
		t.Errorf("headings = %q, want the title and the three sections %q", headings, want)
	}
}

// tableCells splits a Markdown table row on the pipes that are not escaped and
// drops the row's outer pipes. It keeps the escapes in the cell text because only
// the count matters here.
func tableCells(line string) []string {
	inner := strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")

	var (
		cells   []string
		cell    strings.Builder
		escaped bool
	)

	for _, r := range inner {
		switch {
		case escaped:
			cell.WriteRune(r)
			escaped = false
		case r == '\\':
			cell.WriteRune(r)
			escaped = true
		case r == '|':
			cells = append(cells, cell.String())
			cell.Reset()
		default:
			cell.WriteRune(r)
		}
	}

	return append(cells, cell.String())
}

// isSeparatorRow reports whether cells is a table's header separator row, the
// "|---|---|" line under a header.
func isSeparatorRow(cells []string) bool {
	for _, cell := range cells {
		trimmed := strings.TrimSpace(cell)
		if trimmed == "" || strings.Trim(trimmed, "-") != "" {
			return false
		}
	}

	return true
}

// TestReportIsDeterministic pins that the same assessment always renders the same
// bytes, whatever order the inventory was built from: a report never depends on
// the clock, the environment or map iteration.
func TestReportIsDeterministic(t *testing.T) {
	t.Parallel()

	scaleway := testutil.ScalewayCatalog(t)
	meta := analysis.ReportMeta{InventoryName: inventoryFileName}

	first := render(t, assessment.Assess(newInventory(t, sourceName, awsResources()), scaleway), meta)
	second := render(t, assessment.Assess(newInventory(t, sourceName, awsResources()), scaleway), meta)
	if !bytes.Equal(first, second) {
		t.Errorf("two reports of the same assessment differ:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	shuffled := awsResources()
	slices.Reverse(shuffled)

	reordered := render(t, assessment.Assess(newInventory(t, sourceName, shuffled), scaleway), meta)
	if !bytes.Equal(first, reordered) {
		t.Errorf("the report depends on the order resources were given in:\nfirst:\n%s\nreordered:\n%s",
			first, reordered)
	}
}

// TestReportEscapesUnknownReasonCode pins that a reason the domain does not know
// renders as escaped literal text: an unknown code arrives as a literal part, and
// a literal part is escaped like any other text value.
func TestReportEscapesUnknownReasonCode(t *testing.T) {
	t.Parallel()

	unknown := mapping.Reason{Code: "<b>x|y"}
	a := assessment.Assessment{
		Target:   testutil.ScalewayCatalog(t).Target(),
		Snapshot: testutil.ScalewaySnapshot(),
		Source:   sourceName,
		Verdicts: []assessment.Verdict{{
			Resource: inventory.Resource{
				ID:       "i-1",
				RawType:  "AWS::EC2::Instance",
				Category: taxonomy.ComputeVM,
			},
			Fit:     mapping.None,
			Reasons: []mapping.Reason{unknown},
		}},
		Blockers: []assessment.Blocker{{ResourceID: "i-1", Reason: unknown}},
		Summary:  assessment.Summary{Total: 1, None: 1},
	}

	out := string(render(t, a, analysis.ReportMeta{}))

	if want := `\<b\>x\|y`; !strings.Contains(out, want) {
		t.Errorf("report does not contain the escaped reason %q:\n%s", want, out)
	}

	if strings.Contains(out, "<b>") {
		t.Errorf("report contains the unescaped reason code, so the literal part was not escaped:\n%s", out)
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
// to do with a partial report.
func TestReportReturnsWriteError(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("disk is full")
	a := assessment.Assess(newInventory(t, sourceName, awsResources()), testutil.ScalewayCatalog(t))

	err := markdown.New().Report(t.Context(), failingWriter{err: writeErr}, a,
		analysis.ReportMeta{InventoryName: inventoryFileName})
	if !errors.Is(err, writeErr) {
		t.Fatalf("Report() error = %v, want an error wrapping %v", err, writeErr)
	}
}

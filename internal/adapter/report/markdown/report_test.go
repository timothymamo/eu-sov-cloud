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
func render(tb testing.TB, a assessment.Assessment, meta analysis.ReportMeta) []byte {
	tb.Helper()

	var out bytes.Buffer
	if err := markdown.New().Report(tb.Context(), &out, a, meta); err != nil {
		tb.Fatalf("Report() returned unexpected error: %v", err)
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

// controlSnapshot is a snapshot for the assessments the escaping tests build by
// hand: the domain constructors reject the line endings, control characters and
// edge spaces those tests must reach the renderer with, so the guard, not the
// constructor, is what is under test.
var controlSnapshot = catalog.Snapshot{
	SourceURL:   "https://example.com/catalog",
	RetrievedOn: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
}

// TestReportAutolinksSourceURL pins that a source URL is emitted as stored,
// percent-encoded only where a Markdown autolink cannot hold the character: the
// characters that would end the autolink, end the line or end the table cell are
// escaped, and an escape the provider stored is shown as given, never decoded.
func TestReportAutolinksSourceURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "parentheses",
			url:  "https://example.com/a(b)",
			want: "https://example.com/a%28b%29",
		},
		{
			name: "square brackets",
			url:  "https://example.com/a[b]",
			want: "https://example.com/a%5Bb%5D",
		},
		{
			name: "angle brackets",
			url:  "https://example.com/a<b>",
			want: "https://example.com/a%3Cb%3E",
		},
		{
			name: "pipe",
			url:  "https://example.com/a|b",
			want: "https://example.com/a%7Cb",
		},
		{
			name: "backtick",
			url:  "https://example.com/a`b",
			want: "https://example.com/a%60b",
		},
		{
			name: "space",
			url:  "https://example.com/a b",
			want: "https://example.com/a%20b",
		},
		{
			name: "line feed",
			url:  "https://example.com/a\nb",
			want: "https://example.com/a%0Ab",
		},
		{
			name: "carriage return",
			url:  "https://example.com/a\rb",
			want: "https://example.com/a%0Db",
		},
		{
			name: "tab",
			url:  "https://example.com/a\tb",
			want: "https://example.com/a%09b",
		},
		{
			name: "null",
			url:  "https://example.com/a\x00b",
			want: "https://example.com/a%00b",
		},
		{
			name: "unit separator",
			url:  "https://example.com/a\x1fb",
			want: "https://example.com/a%1Fb",
		},
		{
			name: "delete",
			url:  "https://example.com/a\x7fb",
			want: "https://example.com/a%7Fb",
		},
		{
			name: "stored line feed escape stays literal",
			url:  "https://example.com/a%0Ab",
			want: "https://example.com/a%0Ab",
		},
		{
			name: "stored override escape stays literal",
			url:  "https://example.com/%E2%80%AE",
			want: "https://example.com/%E2%80%AE",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			a := assessment.Assessment{
				Target:   "ovhcloud",
				Snapshot: catalog.Snapshot{SourceURL: test.url, RetrievedOn: controlSnapshot.RetrievedOn},
				Source:   sourceName,
			}

			out := string(render(t, a, analysis.ReportMeta{}))

			want := " snapshot from <" + test.want + ">, retrieved 2026-11-02"
			if !strings.Contains(out, want) {
				t.Errorf("report does not contain the autolink %q:\n%s", want, out)
			}
		})
	}
}

// TestReportEscapesLineEndings pins that a line ending inside a value never
// reaches the report as one: running text shows it as a character reference and
// a code span shows it as a space, as CommonMark would, so no value can end a
// line, a heading or a table row.
func TestReportEscapesLineEndings(t *testing.T) {
	t.Parallel()

	reason := mapping.Reason{Code: mapping.NotClassified}
	a := assessment.Assessment{
		Target:   "tar\nget",
		Snapshot: catalog.Snapshot{SourceURL: "https://example.com/a%0Ab", RetrievedOn: controlSnapshot.RetrievedOn},
		Source:   "sou\rce",
		Verdicts: []assessment.Verdict{{
			Resource: inventory.Resource{
				ID:       "i\nd",
				RawType:  "AWS::X\rY",
				Category: taxonomy.Category("cat\nx"),
			},
			Fit:          mapping.None,
			BestOffering: "off\rering",
			Reasons:      []mapping.Reason{reason},
		}},
		Blockers: []assessment.Blocker{{ResourceID: "i\nd", Reason: reason}},
		Summary:  assessment.Summary{Total: 1, None: 1},
	}

	out := string(render(t, a, analysis.ReportMeta{InventoryName: "i\nnv.json"}))

	escaped := []string{
		"# Migration assessment: tar&#10;get",
		"- Inventory file: i&#10;nv.json",
		"- Source: sou&#13;ce",
		"| i&#10;d | AWS::X&#13;Y | `cat x` | none | off&#13;ering | not classified |",
		"- i&#10;d: not classified",
	}
	for _, want := range escaped {
		if !strings.Contains(out, want) {
			t.Errorf("report does not contain %q:\n%s", want, out)
		}
	}

	lines := []string{"tar\nget", "i\nnv.json", "sou\rce", "i\nd", "AWS::X\rY", "cat\nx", "off\rering"}
	for _, line := range lines {
		if strings.Contains(out, line) {
			t.Errorf("report contains the raw value %q, so a line ending was not escaped:\n%s", line, out)
		}
	}

	checkTableShape(t, []byte(out))
}

// TestReportEscapesCodeSpanEdges pins the padding rule of a code span: a value
// that starts or ends with a space keeps it, because CommonMark strips one
// surrounding space; a value of spaces only is never stripped, and an empty
// value renders as an empty cell.
func TestReportEscapesCodeSpanEdges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		category taxonomy.Category
		want     string
	}{
		{name: "leading space is padded", category: " x", want: "`  x `"},
		{name: "trailing space is padded", category: "x ", want: "` x  `"},
		{name: "both edges are padded", category: " x ", want: "`  x  `"},
		{name: "spaces only are not padded", category: "   ", want: "`   `"},
		{name: "backtick edges are padded", category: "`x`", want: "`` `x` ``"},
		{name: "no edges are not padded", category: taxonomy.ComputeVM, want: "`compute.vm`"},
		{name: "empty is an empty cell", category: "", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			a := assessment.Assessment{
				Target:   "ovhcloud",
				Snapshot: controlSnapshot,
				Source:   sourceName,
				Verdicts: []assessment.Verdict{{
					Resource: inventory.Resource{
						ID:       "i-1",
						RawType:  "AWS::EC2::Instance",
						Category: test.category,
					},
					Fit:     mapping.None,
					Reasons: []mapping.Reason{{Code: mapping.NotClassified}},
				}},
				Summary: assessment.Summary{Total: 1, None: 1},
			}

			cells := tableRow(t, render(t, a, analysis.ReportMeta{}), "i-1")
			if len(cells) != 6 {
				t.Fatalf("the resource row has %d cells, want 6: %q", len(cells), cells)
			}

			if got := cells[2]; got != test.want {
				t.Errorf("Category cell = %q, want %q", got, test.want)
			}
		})
	}
}

// TestReportBlockersStartWithResourceID pins that a blocker line is one list
// item whose text starts with the escaped resource ID: an ID that would start a
// list item of its own is escaped where the line starts, so the line stays flat.
// The same ID in a table cell is not escaped that way, because a cell is not a
// line of its own. A star and a closing parenthesis are escaped on every line
// already, as emphasis and link syntax; a dash, a plus and the dot of an ordered
// marker are harmless anywhere but at the start of a line, so only there do they
// need the escape.
func TestReportBlockersStartWithResourceID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
		// want is the ID as it starts a blocker line.
		want string
		// wantCell is the ID as it appears in a table cell.
		wantCell string
	}{
		{name: "dash starts a bullet", id: "- x", want: `\- x`, wantCell: "- x"},
		{name: "plus starts a bullet", id: "+ x", want: `\+ x`, wantCell: "+ x"},
		{name: "star starts a bullet", id: "* x", want: `\* x`, wantCell: `\* x`},
		{name: "digit and dot start an ordered item", id: "1. x", want: `1\. x`, wantCell: "1. x"},
		{name: "digits and parenthesis start an ordered item", id: "12) x", want: `12\) x`, wantCell: `12\) x`},
		{name: "dot inside the ID is not escaped", id: "x 1. y", want: "x 1. y", wantCell: "x 1. y"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			reason := mapping.Reason{Code: mapping.NotClassified}
			a := assessment.Assessment{
				Target:   "ovhcloud",
				Snapshot: controlSnapshot,
				Source:   sourceName,
				Verdicts: []assessment.Verdict{{
					Resource: inventory.Resource{
						ID:       test.id,
						RawType:  "AWS::S3::Bucket",
						Category: taxonomy.Unclassified,
					},
					Fit:     mapping.None,
					Reasons: []mapping.Reason{reason},
				}},
				Blockers: []assessment.Blocker{{ResourceID: test.id, Reason: reason}},
				Summary:  assessment.Summary{Total: 1, None: 1},
			}

			out := render(t, a, analysis.ReportMeta{})

			want := "- " + test.want + ": not classified"
			if got := blockerLines(t, out); !slices.Equal(got, []string{want}) {
				t.Errorf("blockers = %q, want the one list item %q", got, want)
			}

			wantTableRow(t, out, test.wantCell)
		})
	}
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

// tableRow returns the cells of the table row whose first cell is firstCell, with
// the padding writeRow puts around every cell removed. It fails the test when no
// row starts with that cell.
func tableRow(t *testing.T, out []byte, firstCell string) []string {
	t.Helper()

	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}

		cells := tableCells(line)
		if isSeparatorRow(cells) || strings.TrimSpace(cells[0]) != firstCell {
			continue
		}

		for i, cell := range cells {
			cells[i] = strings.TrimSpace(cell)
		}

		return cells
	}

	t.Fatalf("no table row starts with %q:\n%s", firstCell, out)

	return nil
}

// wantTableRow fails the test unless the table has a row whose first cell is
// firstCell, as given, with the six cells of a resources row.
func wantTableRow(t *testing.T, out []byte, firstCell string) {
	t.Helper()

	if cells := tableRow(t, out, firstCell); len(cells) != 6 {
		t.Errorf("the resource row %q has %d cells, want 6: %q", firstCell, len(cells), cells)
	}
}

// blockerLines returns the non-empty lines of the report's Blockers section, in
// the order they appear.
func blockerLines(t *testing.T, out []byte) []string {
	t.Helper()

	lines := strings.Split(string(out), "\n")

	start := slices.Index(lines, "## Blockers")
	if start < 0 {
		t.Fatalf("report has no Blockers section:\n%s", out)
	}

	var blockers []string

	for _, line := range lines[start+1:] {
		if line != "" {
			blockers = append(blockers, line)
		}
	}

	return blockers
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

package markdown_test

import (
	"bytes"
	"context"
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
	// hostileSource is an inventory source full of Markdown metacharacters. It is
	// shown as a code span like every other value, so a source cannot forge a
	// heading.
	hostileSource = "# heading"
	// hostileTarget is a catalog target whose name carries every Markdown
	// metacharacter that matters. It is also the name of one offering.
	hostileTarget = "Evil | `x` <b> [a](b) *s*"
	// hostileOffering is an offering name with the same characters as
	// hostileTarget, in a different order.
	hostileOffering = "`tick` [row](x) | Name"
	// hostileInventoryName is a file name whose Markdown syntax must reach the
	// report as text.
	hostileInventoryName = "inv|`x`[y](z).json"
	// scalewayTarget is the display name of the fixture catalog, as the test cases
	// that build their own catalog spell it.
	scalewayTarget = "Scaleway"
	// hostileSourceURL is a catalog source URL with parentheses, a stored newline
	// escape, a stored right-to-left override escape and a character reference.
	// Renderers decode all three, so a report that showed the URL as an autolink
	// would end its line and display a right-to-left override the provider never
	// sent; the stored bytes are shown in a code span instead.
	hostileSourceURL = "https://example.com/a(b)%0A%E2%80%AE&#8238;x"
)

// hostileValues returns every fixture value a report must show verbatim, each
// inside a code span.
//
// Markdown escapes do not survive the trip through a renderer: GitHub resolves
// them before it applies its math ($...$), mention (@x), reference (#1, GH-1,
// SHA), emoji (:x:), task-list ([ ] at the start of a list item) and bare-link
// filters, and the last of those would turn a bare URL into a link. A code span
// is literal in every renderer, and GitHub's filters skip code, so it is the one
// form that keeps a value from being read as anything but text.
func hostileValues() []string {
	return []string{
		// The catalog, its offerings and the inventory's own metadata.
		hostileTarget,
		hostileOffering,
		hostileInventoryName,
		hostileSource,
		hostileSourceURL,
		// IDs and raw types from the earlier hostile fixture.
		"[l](javascript:x)",
		"`tick`",
		"a*b_c",
		"AWS::X.Y <img src=x>",
		"# heading",
		`back\slash`,
		"i-1 | x",
		// IDs and raw types that a renderer's extensions would otherwise rewrite.
		"#1",
		"$\\Huge X$",
		"@octocat",
		":x:",
		"[ ] x",
		"[x] done",
		"https://evil.example/%E2%80%AEmoc.elgoog",
		"x&#x202E;y",
		"a`b",
		"``",
		"a|b",
	}
}

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
			// Two missing capabilities, so the reason cell joins two sentences and
			// the required-changes section lists both.
			ID:           "partial-example",
			RawType:      "AWS::EC2::Instance",
			Category:     taxonomy.ComputeVM,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.HighAvailability, taxonomy.Snapshots},
		},
	}
}

// hostileResources is an inventory whose every text value would be syntax in a
// renderer: an ID with link syntax, an ID with a pipe, an ID with a backslash, a
// raw type with HTML, a raw type that looks like a heading, math, a mention, a
// reference, an emoji, a task list, a bare link and an HTML character reference.
// A value must reach the report verbatim, inside a code span.
func hostileResources() []inventory.Resource {
	return []inventory.Resource{
		{
			ID:       "#1",
			RawType:  "$\\Huge X$",
			Category: taxonomy.ServerlessFunction,
			Region:   "eu-west-1",
		},
		{
			ID:       "[ ] x",
			RawType:  "@octocat",
			Category: taxonomy.Unclassified,
			Region:   "eu-west-1",
		},
		{
			ID:       "[l](javascript:x)",
			RawType:  "``",
			Category: taxonomy.ServerlessFunction,
			Region:   "eu-west-1",
		},
		{
			ID:       "[x] done",
			RawType:  ":x:",
			Category: taxonomy.StorageObject,
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
			ID:           "a`b",
			RawType:      "https://evil.example/%E2%80%AEmoc.elgoog",
			Category:     taxonomy.ComputeVM,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.HighAvailability},
		},
		{
			ID:           "a|b",
			RawType:      "x&#x202E;y",
			Category:     taxonomy.StorageObject,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.ObjectVersioning},
		},
		{
			ID:           `back\slash`,
			RawType:      "# heading",
			Category:     taxonomy.StorageObject,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.IPv6},
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

// newCatalog builds a catalog from offerings on the shared Scaleway snapshot.
func newCatalog(t *testing.T, offerings []catalog.Offering) catalog.Catalog {
	t.Helper()

	cat, err := catalog.New(scalewayTarget, testutil.ScalewaySnapshot(), offerings)
	if err != nil {
		t.Fatalf("catalog.New() returned unexpected error: %v", err)
	}

	return cat
}

// computeOffering is a compute.vm offering in one region, with the capabilities
// the candidate ordering compares.
func computeOffering(name string, capabilities ...taxonomy.Capability) catalog.Offering {
	return catalog.Offering{
		Name:         name,
		Categories:   []taxonomy.Category{taxonomy.ComputeVM},
		Capabilities: capabilities,
		Regions:      []string{"fr-par"},
	}
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
// header, the summary, one table row per resource, the required changes and the
// blockers, with every value from the assessment in a code span.
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

			checkTableShape(t, got)
			checkHeadings(t, got, "# Migration assessment: `Scaleway`")
		})
	}
}

// TestReportHostileValues pins that every untrusted value, and the source URL,
// reaches the report verbatim inside a code span: a renderer decodes the escapes
// an autolink needed and then applies its own filters, so no value may be left
// as running text. A value with a pipe may neither end a table cell nor start a
// heading, and nothing the provider stored is decoded or percent-encoded.
func TestReportHostileValues(t *testing.T) {
	t.Parallel()

	a := assessment.Assess(newInventory(t, hostileSource, hostileResources()), hostileCatalog(t))
	got := render(t, a, analysis.ReportMeta{InventoryName: hostileInventoryName})

	testutil.Golden(t, "testdata/report-hostile.golden", got)

	checkTableShape(t, got)
	checkHeadings(t, got, "# Migration assessment: ``Evil | `x` <b> [a](b) *s*``")

	spans := codeSpans(string(got))
	for _, value := range hostileValues() {
		if !slices.Contains(spans, value) {
			t.Errorf("the report does not show %q inside a code span, so a renderer can read it as syntax:\n%s",
				value, got)
		}
	}
}

// controlSnapshot is a snapshot for the assessments the escaping tests build by
// hand: the domain constructors reject the line endings, control characters and
// edge spaces those tests must reach the renderer with, so the guard, not the
// constructor, is what is under test.
var controlSnapshot = catalog.Snapshot{
	SourceURL:   "https://example.com/catalog",
	RetrievedOn: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
}

// TestReportSourceURLIsVerbatimCodeSpan pins that the catalog's source URL is a
// code span of the stored bytes: nothing is decoded, so a stored "%0A",
// "%E2%80%AE" or "&#8238;" stays the text the provider stored, and nothing is
// percent-encoded or wrapped in an autolink, both of which a renderer would
// decode into a line break or a right-to-left override. The one conversion left
// is CommonMark's own: a line ending inside a code span is a space.
func TestReportSourceURLIsVerbatimCodeSpan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		// want is the source URL as the report shows it: a code span.
		want string
	}{
		{name: "parentheses", url: "https://example.com/a(b)", want: "`https://example.com/a(b)`"},
		{name: "square brackets", url: "https://example.com/a[b]", want: "`https://example.com/a[b]`"},
		{name: "angle brackets", url: "https://example.com/a<b>", want: "`https://example.com/a<b>`"},
		{name: "pipe", url: "https://example.com/a|b", want: "`https://example.com/a|b`"},
		{name: "backtick takes a longer fence", url: "https://example.com/a`b", want: "``https://example.com/a`b``"},
		{name: "space", url: "https://example.com/a b", want: "`https://example.com/a b`"},
		{name: "line feed becomes a space", url: "https://example.com/a\nb", want: "`https://example.com/a b`"},
		{name: "carriage return becomes a space", url: "https://example.com/a\rb", want: "`https://example.com/a b`"},
		{
			name: "stored line feed escape stays literal",
			url:  "https://example.com/a%0Ab",
			want: "`https://example.com/a%0Ab`",
		},
		{
			name: "stored override escape stays literal",
			url:  "https://example.com/%E2%80%AE",
			want: "`https://example.com/%E2%80%AE`",
		},
		{
			name: "character reference stays literal",
			url:  "https://example.com/&#8238;x",
			want: "`https://example.com/&#8238;x`",
		},
		{name: "every hostile form at once", url: hostileSourceURL, want: "`" + hostileSourceURL + "`"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			a := assessment.Assessment{
				Target:   "ovhcloud",
				Snapshot: catalog.Snapshot{SourceURL: test.url, RetrievedOn: controlSnapshot.RetrievedOn},
				Source:   sourceName,
			}

			out := render(t, a, analysis.ReportMeta{})

			want := "- Catalog: `ovhcloud` snapshot from " + test.want + ", retrieved 2026-11-02"
			if got := lineWithPrefix(t, out, "- Catalog: "); got != want {
				t.Errorf("catalog line = %q, want %q", got, want)
			}
		})
	}
}

// TestReportEscapesLineEndings pins that a line ending inside a value never
// reaches the report as one: a code span shows it as a space, as CommonMark
// would, and running text, which only a literal reason part is, shows it as a
// character reference. No value can end a line, a heading or a table row.
func TestReportEscapesLineEndings(t *testing.T) {
	t.Parallel()

	a := assessment.Assessment{
		Target:   "tar\nget",
		Snapshot: catalog.Snapshot{SourceURL: "https://example.com/a%0Ab", RetrievedOn: controlSnapshot.RetrievedOn},
		Source:   "sou\rce",
		Verdicts: []assessment.Verdict{
			{
				Resource: inventory.Resource{
					ID:       "i\nd",
					RawType:  "AWS::X\rY",
					Category: taxonomy.Category("cat\nx"),
				},
				Fit:          mapping.None,
				BestOffering: "off\rering",
				Reasons:      []mapping.Reason{{Code: mapping.NotClassified}},
			},
			{
				Resource: inventory.Resource{
					ID:       "x\ny",
					Category: taxonomy.Unclassified,
				},
				Fit:     mapping.None,
				Reasons: []mapping.Reason{{Code: "lit\neral"}},
			},
		},
		Blockers: []assessment.Blocker{
			{ResourceID: "i\nd", Reason: mapping.Reason{Code: mapping.NotClassified}},
			{ResourceID: "x\ny", Reason: mapping.Reason{Code: "lit\neral"}},
		},
		Summary: assessment.Summary{Total: 2, None: 2},
	}

	out := string(render(t, a, analysis.ReportMeta{InventoryName: "i\nnv.json"}))

	escaped := []string{
		"# Migration assessment: `tar get`",
		"- Inventory file: `i nv.json`",
		"- Source: `sou ce`",
		"| `i d` | `AWS::X Y` | `cat x` | none | `off ering` | not classified |",
		"| `x y` |  | `unclassified` | none |  | lit&#10;eral |",
		"- `i d`: not classified",
		"- `x y`: lit&#10;eral",
	}
	for _, want := range escaped {
		if !strings.Contains(out, want) {
			t.Errorf("report does not contain %q:\n%s", want, out)
		}
	}

	lines := []string{
		"tar\nget", "i\nnv.json", "sou\rce", "i\nd", "AWS::X\rY", "cat\nx", "off\rering", "x\ny", "lit\neral",
	}
	for _, line := range lines {
		if strings.Contains(out, line) {
			t.Errorf("report contains the raw value %q, so a line ending was not escaped:\n%s", line, out)
		}
	}

	checkTableShape(t, []byte(out))
}

// TestReportEscapesCodeSpanEdges pins the fence and padding rule of a code span:
// the fence is one backtick longer than the longest run in the value, a value
// that starts or ends with a space keeps it because CommonMark strips one
// surrounding space, a value of spaces only is never stripped, a pipe is escaped
// so it cannot end a table cell and keeps a backslash already before it, and an
// empty value renders as an empty cell.
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
		{name: "pipe is escaped in a table cell", category: "a|b", want: "`a\\|b`"},
		{name: "backslash before a pipe keeps its backslash", category: "a\\|b", want: "`a\\\\|b`"},
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

			cells := tableRow(t, render(t, a, analysis.ReportMeta{}), "`i-1`")
			if len(cells) != 6 {
				t.Fatalf("the resource row has %d cells, want 6: %q", len(cells), cells)
			}

			if got := cells[2]; got != test.want {
				t.Errorf("Category cell = %q, want %q", got, test.want)
			}
		})
	}
}

// TestReportBlockersStartWithResourceID pins that a blocker is one list item
// whose text starts with the resource ID as a code span: a dash or a plus
// cannot start a second list item, a star or the parenthesis of an ordered
// marker cannot close one, and "[ ]" or "[x]" cannot turn the line into a task
// list, because nothing inside a code span is syntax in any renderer.
func TestReportBlockersStartWithResourceID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
	}{
		{name: "dash that would start a bullet", id: "- x"},
		{name: "plus that would start a bullet", id: "+ x"},
		{name: "star that would start a bullet", id: "* x"},
		{name: "digit and dot that would start an ordered item", id: "1. x"},
		{name: "digits and parenthesis that would start an ordered item", id: "12) x"},
		{name: "dot inside the ID", id: "x 1. y"},
		{name: "unchecked task list marker", id: "[ ] x"},
		{name: "checked task list marker", id: "[x] x"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// None of these IDs holds a backtick, so a one-backtick fence is the
			// code span the report must use.
			want := "`" + test.id + "`"

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

			wantBlockers := []string{"- " + want + ": not classified"}
			if got := blockerLines(t, out); !slices.Equal(got, wantBlockers) {
				t.Errorf("blockers = %q, want the one list item %q", got, wantBlockers)
			}

			if cells := tableRow(t, out, want); len(cells) != 6 {
				t.Errorf("the resource row %q has %d cells, want 6: %q", want, len(cells), cells)
			}
		})
	}
}

// verdictOf is a verdict for the layout tests that pin one part of the report
// without mapping a resource against a catalog.
func verdictOf(id string, fit mapping.Fit, best string, changes []taxonomy.Capability) assessment.Verdict {
	return assessment.Verdict{
		Resource: inventory.Resource{
			ID:       id,
			RawType:  "AWS::EC2::Instance",
			Category: taxonomy.ComputeVM,
		},
		Fit:             fit,
		BestOffering:    best,
		RequiredChanges: changes,
	}
}

// TestReportRequiredChanges pins the section that tells a reader what to change
// to move a resource: one item per partial verdict, in verdict order, naming the
// best offering's missing capabilities in one code span each and the offering
// after them; and "None." when nothing is partial, whatever the other fits are.
func TestReportRequiredChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		verdicts []assessment.Verdict
		want     []string
	}{
		{
			name: "no partial verdict renders None",
			verdicts: []assessment.Verdict{
				verdictOf("a-direct", mapping.Direct, "Elastic Metal", nil),
				verdictOf("b-none", mapping.None, "", nil),
			},
			want: []string{"None."},
		},
		{
			name: "one item per partial verdict in verdict order",
			verdicts: []assessment.Verdict{
				verdictOf("a-direct", mapping.Direct, "Elastic Metal", nil),
				verdictOf("b-partial", mapping.Partial, "Elastic Metal",
					[]taxonomy.Capability{taxonomy.HighAvailability, taxonomy.Snapshots}),
				verdictOf("c-partial", mapping.Partial, "Instances",
					[]taxonomy.Capability{taxonomy.IPv6}),
				verdictOf("d-none", mapping.None, "", nil),
			},
			want: []string{
				"- `b-partial`: `high-availability`, `snapshots` (not offered by `Elastic Metal`)",
				"- `c-partial`: `ipv6` (not offered by `Instances`)",
			},
		},
		{
			name: "a partial verdict without a candidate renders None",
			verdicts: []assessment.Verdict{
				verdictOf("a-partial", mapping.Partial, "", nil),
			},
			want: []string{"None."},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var summary assessment.Summary
			for _, v := range test.verdicts {
				summary.Total++

				switch v.Fit {
				case mapping.Direct:
					summary.Direct++
				case mapping.Partial:
					summary.Partial++
				case mapping.None:
					summary.None++
				}
			}

			a := assessment.Assessment{
				Target:   "Scaleway",
				Snapshot: controlSnapshot,
				Source:   sourceName,
				Verdicts: test.verdicts,
				Summary:  summary,
			}

			if got := requiredChangeLines(t, render(t, a, analysis.ReportMeta{})); !slices.Equal(got, test.want) {
				t.Errorf("required changes = %q, want %q", got, test.want)
			}
		})
	}
}

// TestReportBestOfferingTie pins the best-offering cell: the best candidate's
// name in a code span, followed by the other candidates that a reader could
// mistake for it, that is the ones with the same fit and as many missing
// capabilities, in candidate order. A candidate that fits worse, or misses more,
// is not shown beside the best one.
func TestReportBestOfferingTie(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		offerings    []catalog.Offering
		capabilities []taxonomy.Capability
		want         string
	}{
		{
			name:         "the only candidate has no tie",
			offerings:    []catalog.Offering{computeOffering("Instances", taxonomy.IPv6)},
			capabilities: []taxonomy.Capability{taxonomy.IPv6},
			want:         "`Instances`",
		},
		{
			name: "direct candidates with nothing missing tie in candidate order",
			offerings: []catalog.Offering{
				computeOffering("Zulu Compute", taxonomy.IPv6),
				computeOffering("Alpha Compute", taxonomy.IPv6),
			},
			capabilities: []taxonomy.Capability{taxonomy.IPv6},
			want:         "`Alpha Compute` (tie: `Zulu Compute`)",
		},
		{
			name: "three tied candidates list the others in candidate order",
			offerings: []catalog.Offering{
				computeOffering("Charlie Compute", taxonomy.IPv6),
				computeOffering("Alpha Compute", taxonomy.IPv6),
				computeOffering("Bravo Compute", taxonomy.IPv6),
			},
			capabilities: []taxonomy.Capability{taxonomy.IPv6},
			want:         "`Alpha Compute` (tie: `Bravo Compute`, `Charlie Compute`)",
		},
		{
			name: "a direct best takes no tie with a partial candidate",
			offerings: []catalog.Offering{
				computeOffering("Alpha Compute", taxonomy.IPv6),
				computeOffering("Bravo Compute", taxonomy.Autoscaling, taxonomy.IPv6),
			},
			capabilities: []taxonomy.Capability{taxonomy.Autoscaling, taxonomy.IPv6},
			want:         "`Bravo Compute`",
		},
		{
			name: "partial candidates missing one capability each tie",
			offerings: []catalog.Offering{
				computeOffering("Alpha Compute", taxonomy.IPv6),
				computeOffering("Bravo Compute", taxonomy.Autoscaling),
			},
			capabilities: []taxonomy.Capability{taxonomy.Autoscaling, taxonomy.IPv6},
			want:         "`Alpha Compute` (tie: `Bravo Compute`)",
		},
		{
			name: "partial candidates missing the same number tie",
			offerings: []catalog.Offering{
				computeOffering("Alpha Compute", taxonomy.IPv6),
				computeOffering("Bravo Compute", taxonomy.IPv6),
			},
			capabilities: []taxonomy.Capability{
				taxonomy.Autoscaling,
				taxonomy.IPv6,
				taxonomy.Snapshots,
			},
			want: "`Alpha Compute` (tie: `Bravo Compute`)",
		},
		{
			name: "a candidate missing more is no tie",
			offerings: []catalog.Offering{
				computeOffering("Alpha Compute", taxonomy.IPv6),
				computeOffering("Bravo Compute", taxonomy.Autoscaling, taxonomy.IPv6),
			},
			capabilities: []taxonomy.Capability{
				taxonomy.Autoscaling,
				taxonomy.IPv6,
				taxonomy.Snapshots,
			},
			want: "`Bravo Compute`",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			inv := newInventory(t, sourceName, []inventory.Resource{{
				ID:           "i-1",
				RawType:      "AWS::EC2::Instance",
				Category:     taxonomy.ComputeVM,
				Region:       "eu-west-1",
				Capabilities: test.capabilities,
			}})

			a := assessment.Assess(inv, newCatalog(t, test.offerings))

			cells := tableRow(t, render(t, a, analysis.ReportMeta{}), "`i-1`")
			if len(cells) != 6 {
				t.Fatalf("the resource row has %d cells, want 6: %q", len(cells), cells)
			}

			if got := cells[4]; got != test.want {
				t.Errorf("Best offering cell = %q, want %q", got, test.want)
			}
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
// title it is given and the four section headings, in layout order, so no value
// can forge one or move a section.
func checkHeadings(t *testing.T, out []byte, title string) {
	t.Helper()

	var headings []string

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "#") {
			headings = append(headings, line)
		}
	}

	want := []string{
		title,
		"## Summary",
		"## Resources",
		"## Required changes",
		"## Blockers",
	}

	if !slices.Equal(headings, want) {
		t.Errorf("headings = %q, want the title and the four sections %q", headings, want)
	}
}

// tableCells splits a Markdown table row on the pipes that are not escaped and
// drops the row's outer pipes. A pipe is escaped, as the table extension of
// CommonMark has it, when the character just before it is a backslash, whatever
// escapes that backslash; the escapes stay in the cell text because only the
// count matters here.
func tableCells(line string) []string {
	inner := strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")

	var (
		cells []string
		cell  strings.Builder
	)

	for i, r := range inner {
		if r == '|' && (i == 0 || inner[i-1] != '\\') {
			cells = append(cells, cell.String())
			cell.Reset()

			continue
		}

		cell.WriteRune(r)
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

// sectionLines returns the non-empty lines of the section headed by heading, in
// the order they appear.
func sectionLines(t *testing.T, out []byte, heading string) []string {
	t.Helper()

	lines := strings.Split(string(out), "\n")

	start := slices.Index(lines, heading)
	if start < 0 {
		t.Fatalf("report has no %q section:\n%s", heading, out)
	}

	var body []string

	for _, line := range lines[start+1:] {
		if strings.HasPrefix(line, "## ") {
			break
		}

		if line != "" {
			body = append(body, line)
		}
	}

	return body
}

// blockerLines returns the lines of the report's Blockers section, in the order
// they appear.
func blockerLines(t *testing.T, out []byte) []string {
	t.Helper()

	return sectionLines(t, out, "## Blockers")
}

// requiredChangeLines returns the lines of the report's Required changes section,
// in the order they appear.
func requiredChangeLines(t *testing.T, out []byte) []string {
	t.Helper()

	return sectionLines(t, out, "## Required changes")
}

// lineWithPrefix returns the one line of the report that starts with prefix.
func lineWithPrefix(t *testing.T, out []byte, prefix string) string {
	t.Helper()

	var found []string

	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, prefix) {
			found = append(found, line)
		}
	}

	if len(found) != 1 {
		t.Fatalf("report has %d lines starting with %q, want 1:\n%s", len(found), prefix, out)
	}

	return found[0]
}

// codeSpans returns the content of every inline code span of a report, as
// CommonMark renders it: the fence is stripped, one surrounding space is removed
// from content that has one on each side, and the "\|" a table cell needs is a
// pipe again. It is how the test sees what a renderer will show literally.
func codeSpans(out string) []string {
	var spans []string

	for _, line := range strings.Split(out, "\n") {
		i := 0
		for i < len(line) {
			if line[i] != '`' {
				i++

				continue
			}

			start := i
			for i < len(line) && line[i] == '`' {
				i++
			}

			fence := i - start

			end := closingFence(line, i, fence)
			if end < 0 {
				continue
			}

			content := line[i:end]
			if len(content) > 2 && strings.HasPrefix(content, " ") && strings.HasSuffix(content, " ") &&
				strings.Trim(content, " ") != "" {
				content = content[1 : len(content)-1]
			}

			spans = append(spans, strings.ReplaceAll(content, `\|`, "|"))

			i = end + fence
		}
	}

	return spans
}

// closingFence returns the index of the first run of exactly n backticks at or
// after from, or -1 when there is none.
func closingFence(line string, from, n int) int {
	for j := from; j < len(line); {
		if line[j] != '`' {
			j++

			continue
		}

		start := j
		for j < len(line) && line[j] == '`' {
			j++
		}

		if j-start == n {
			return start
		}
	}

	return -1
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
// a literal part is the only part that is not a code span, so it is escaped for
// running text.
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

// TestReportCancelledContext pins that a cancelled context stops Report before
// anything is rendered: the caller gets the cancellation, and the writer never
// sees a report the caller has stopped waiting for.
func TestReportCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	a := assessment.Assess(newInventory(t, sourceName, awsResources()), testutil.ScalewayCatalog(t))

	var out bytes.Buffer

	err := markdown.New().Report(ctx, &out, a, analysis.ReportMeta{InventoryName: inventoryFileName})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Report() error = %v, want an error wrapping context.Canceled", err)
	}

	if out.Len() != 0 {
		t.Errorf("Report() wrote %d bytes to the writer, want none:\n%s", out.Len(), out.String())
	}
}

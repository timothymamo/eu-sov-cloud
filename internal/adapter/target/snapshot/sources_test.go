// The documentation rule every embedded snapshot follows is pinned here: each
// offering in a snapshot has its own section in the SOURCES.md beside it, and
// that section names the https pages the offering comes from, the fact behind
// each capability and region, and the licence of each source. The test globs the
// snapshots beside this package, so it runs for every target with an embedded
// snapshot, adding one as each target arrives.

package snapshot_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/internal/adapter/target/snapshot"
	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/text"
)

// The files a snapshot directory holds, and the headings its SOURCES.md must
// carry. Paths are relative to this package's directory, where go test runs.
const (
	catalogFileName  = "catalog.json"
	sourcesFileName  = "SOURCES.md"
	licencesHeading  = "## Licences of the sources"
	offeringsHeading = "## Offerings"
	// defaultRegionHeading is the section that names the region estimates use
	// when the user names none, and why it is that region (ADR 0024).
	defaultRegionHeading = "## Default region"
)

// requiredSnapshots are the snapshots the rule is known to apply to today: every
// target adapter that embeds one. Finding none of them would make the glob below
// pass while checking nothing, so the absence of any one is a failure, and a new
// target's snapshot is added here so the check cannot silently skip it.
var requiredSnapshots = []string{"../scaleway/catalog.json", "../ovhcloud/catalog.json", "../upcloud/catalog.json"}

// TestEveryOfferingHasASourcesSection pins that every offering in an embedded
// snapshot has exactly one section of its own in the SOURCES.md beside it, that
// no section names an offering the snapshot does not have, that the section
// names at least one https page the offering comes from, and that the section
// names each of the offering's categories, capabilities and regions as a
// Markdown code span, so the fact behind each one can be traced to a page.
func TestEveryOfferingHasASourcesSection(t *testing.T) {
	t.Parallel()

	// Every target adapter's embedded snapshot is a sibling directory of this
	// package, so one glob covers every target, today and as more arrive.
	paths, err := filepath.Glob("../*/" + catalogFileName)
	if err != nil {
		t.Fatalf("globbing the snapshots beside this package: %v", err)
	}

	if len(paths) == 0 {
		t.Fatalf("found no embedded snapshot: no %s matches ../*/%s", catalogFileName, catalogFileName)
	}

	for _, want := range requiredSnapshots {
		if !slices.Contains(paths, want) {
			t.Fatalf("the snapshots found are %v, want them to include %s", paths, want)
		}
	}

	for _, path := range paths {
		t.Run(filepath.Base(filepath.Dir(path)), func(t *testing.T) {
			t.Parallel()

			checkSnapshotSources(t, path)
		})
	}
}

// mdSection is one "### <name>" subsection of a SOURCES.md: the heading's text
// and the lines under it, up to the next third-level heading.
type mdSection struct {
	name string
	body []string
}

// checkSnapshotSources checks one snapshot and the SOURCES.md beside it, and
// reports every problem it finds rather than the first, so one run shows all the
// gaps of a snapshot.
func checkSnapshotSources(t *testing.T, catalogPath string) {
	t.Helper()

	dir := filepath.Dir(catalogPath)

	data, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("%s: reading the snapshot: %v", dir, err)
	}

	document, err := snapshot.Parse(data)
	if err != nil {
		t.Fatalf("%s: the snapshot does not parse through the shared reader: %v", dir, err)
	}

	sourcesPath := filepath.Join(dir, sourcesFileName)

	sources, err := os.ReadFile(sourcesPath)
	if err != nil {
		t.Fatalf("%s: no %s beside the snapshot: %v", dir, sourcesFileName, err)
	}

	lines := strings.Split(string(sources), "\n")

	checkDefaultRegionSection(t, sourcesPath, lines, document.Catalog.Snapshot().DefaultRegion)

	if licences, ok := section(lines, licencesHeading); !ok {
		t.Errorf("%s: no %q section: every source must state its licence", sourcesPath, licencesHeading)
	} else if !hasContent(licences) {
		t.Errorf("%s: the %q section is blank: every source must state its licence", sourcesPath, licencesHeading)
	}

	offerings, ok := section(lines, offeringsHeading)
	if !ok {
		t.Errorf("%s: no %q section: every offering needs a section of its own", sourcesPath, offeringsHeading)

		return
	}

	sections := subsections(offerings)
	offeringSections := make(map[string]int, len(sections))

	for _, s := range sections {
		offeringSections[s.name]++
	}

	byName := make(map[string]catalog.Offering, len(sections))

	for _, offering := range document.Catalog.Offerings() {
		byName[offering.Name] = offering

		switch count := offeringSections[offering.Name]; {
		case count == 0:
			t.Errorf("%s: offering %s has no %s section",
				sourcesPath, text.Quote(offering.Name), text.Quote("### "+offering.Name))
		case count > 1:
			t.Errorf("%s: offering %s has %d %s sections, want exactly one",
				sourcesPath, text.Quote(offering.Name), count, text.Quote("### "+offering.Name))
		}
	}

	for _, s := range sections {
		offering, ok := byName[s.name]
		if !ok {
			t.Errorf("%s: section %s matches no offering in %s",
				sourcesPath, text.Quote("### "+s.name), catalogPath)

			continue
		}

		spans := codeSpans(s.body)

		// A section that names no page states a fact no reader can check: the PRD
		// requires it to name the pages the offering comes from.
		if !hasLink(s.body) {
			t.Errorf("%s: offering %s: its section links no %s page, so where the offering comes from is untraceable",
				sourcesPath, text.Quote(offering.Name), linkScheme)
		}

		for _, category := range offering.Categories {
			if _, ok := spans[string(category)]; !ok {
				t.Errorf("%s: offering %s: category %s has no code span in its section, so the fact behind it is untraceable",
					sourcesPath, text.Quote(offering.Name), text.Quote(string(category)))
			}
		}

		for _, capability := range offering.Capabilities {
			if _, ok := spans[string(capability)]; !ok {
				t.Errorf("%s: offering %s: capability %s has no code span in its section, so the fact behind it is untraceable",
					sourcesPath, text.Quote(offering.Name), text.Quote(string(capability)))
			}
		}

		for _, region := range offering.Regions {
			if _, ok := spans[region]; !ok {
				t.Errorf("%s: offering %s: region %s has no code span in its section, so the fact behind it is untraceable",
					sourcesPath, text.Quote(offering.Name), text.Quote(region))
			}
		}

		// Every price the snapshot stores is cited in the offering's own section:
		// the plan by its name exactly as the catalog holds it, and the amount as
		// Amount.String renders it, so a price in SOURCES.md and a price in
		// catalog.json can be compared as text (ADR 0024).
		var prices []catalog.Price

		for _, plan := range offering.Plans {
			if _, ok := spans[plan.Name]; !ok {
				t.Errorf("%s: offering %s: plan %s has no code span in its section, so its price is untraceable",
					sourcesPath, text.Quote(offering.Name), text.Quote(plan.Name))
			}

			prices = append(prices, plan.Prices...)
		}

		prices = append(prices, offering.StoragePrices...)

		for _, price := range prices {
			amount := price.Amount.String()

			if _, ok := spans[amount]; !ok {
				t.Errorf("%s: offering %s: price %s has no code span in its section, so the amount is untraceable",
					sourcesPath, text.Quote(offering.Name), text.Quote(amount))
			}
		}
	}
}

// checkDefaultRegionSection checks the snapshot's "## Default region" section:
// it exists, is not blank, and names the default region as a code span, so a
// reader can see which region estimates use and, from the prose around it, why
// (ADR 0024).
func checkDefaultRegionSection(t *testing.T, sourcesPath string, lines []string, defaultRegion string) {
	t.Helper()

	body, ok := section(lines, defaultRegionHeading)
	if !ok {
		t.Errorf("%s: no %q section: estimates need to say which region they use by default",
			sourcesPath, defaultRegionHeading)

		return
	}

	if !hasContent(body) {
		t.Errorf("%s: the %q section is blank: it must name the default region and why it is the default",
			sourcesPath, defaultRegionHeading)

		return
	}

	if _, ok := codeSpans(body)[defaultRegion]; !ok {
		t.Errorf("%s: the %q section does not name the default region %s as a code span",
			sourcesPath, defaultRegionHeading, text.Quote(defaultRegion))
	}
}

// section returns the lines under the heading whose text is exactly heading, up
// to the next second-level heading, and whether the document has such a heading.
func section(lines []string, heading string) ([]string, bool) {
	start := slices.Index(lines, heading)
	if start < 0 {
		return nil, false
	}

	end := len(lines)

	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i

			break
		}
	}

	return lines[start+1 : end], true
}

// subsections splits lines into the "### <name>" sections under them, each body
// running to the next third-level heading or to the end.
func subsections(lines []string) []mdSection {
	var (
		sections []mdSection
		current  *mdSection
	)

	for _, line := range lines {
		if name, ok := strings.CutPrefix(line, "### "); ok {
			if current != nil {
				sections = append(sections, *current)
			}

			current = &mdSection{name: strings.TrimSpace(name)}

			continue
		}

		if current != nil {
			current.body = append(current.body, line)
		}
	}

	if current != nil {
		sections = append(sections, *current)
	}

	return sections
}

// codeSpanPattern matches a Markdown code span: text between single backticks on
// one line.
var codeSpanPattern = regexp.MustCompile("`([^`\n]+)`")

// linkScheme is what a section's page links start with: an absolute link to the
// page the offering's facts come from.
const linkScheme = "https://"

// hasLink reports whether lines hold a link to a page: the pages a section names
// as where its offering comes from.
func hasLink(lines []string) bool {
	return slices.ContainsFunc(lines, func(line string) bool {
		return strings.Contains(line, linkScheme)
	})
}

// codeSpans returns the code spans in lines as a set.
func codeSpans(lines []string) map[string]struct{} {
	spans := make(map[string]struct{})

	for _, match := range codeSpanPattern.FindAllStringSubmatch(strings.Join(lines, "\n"), -1) {
		spans[match[1]] = struct{}{}
	}

	return spans
}

// hasContent reports whether lines hold a line that is not blank.
func hasContent(lines []string) bool {
	return slices.ContainsFunc(lines, func(line string) bool {
		return strings.TrimSpace(line) != ""
	})
}

// TestSourcesHelpers pins the two pure rules the SOURCES.md check is built from,
// on invented Markdown that no snapshot holds: a section is the body under a
// second-level heading, up to the next one, and a code span is the text between
// single backticks on one line. They are what makes a plan name and a price
// amount citable, and the default region nameable (ADR 0024), so a change to
// either helper is a change to the rule every snapshot's sources follow.
func TestSourcesHelpers(t *testing.T) {
	t.Parallel()

	invented := []string{
		"# Sources",
		"",
		defaultRegionHeading,
		"",
		"Estimates use `fr-par`, because it is the target's first-listed region.",
		"",
		offeringsHeading,
		"",
		"### Example Compute",
		"",
		"Plan `EC-2-4` costs `12.50` EUR a month; a GiB of storage costs `0.04`.",
		"",
		licencesHeading,
		"",
		"MIT.",
	}

	t.Run("default region section", func(t *testing.T) {
		t.Parallel()

		body, ok := section(invented, defaultRegionHeading)
		if !ok {
			t.Fatalf("section(%q) found no section in the invented sources", defaultRegionHeading)
		}

		if !hasContent(body) {
			t.Errorf("section(%q) is blank, want the region and why it is the default", defaultRegionHeading)
		}

		if _, ok := codeSpans(body)["fr-par"]; !ok {
			t.Errorf("section(%q) has no `fr-par` code span, want the default region named in one", defaultRegionHeading)
		}

		// A section ends at the next second-level heading, so the offering's code
		// spans are not mistaken for the default region section's.
		if _, ok := codeSpans(body)["EC-2-4"]; ok {
			t.Errorf("section(%q) reaches into the next section, want it to stop at the next `## ` heading",
				defaultRegionHeading)
		}

		if _, ok := section(invented, "## No such heading"); ok {
			t.Error("section() found a heading the invented sources do not have")
		}
	})

	t.Run("code spans", func(t *testing.T) {
		t.Parallel()

		spans := codeSpans([]string{"Plan `EC-2-4` costs `12.50`, or `13.0125` where prices differ; `0.04` per GiB."})

		for _, want := range []string{"EC-2-4", "12.50", "13.0125", "0.04"} {
			if _, ok := spans[want]; !ok {
				t.Errorf("codeSpans() has no %s span, want every code span on the line", text.Quote(want))
			}
		}

		// An amount is cited as Amount.String renders it, trailing zeros kept:
		// the span is 12.50, never the 12.5 the document writes.
		if _, ok := spans["12.5"]; ok {
			t.Error("codeSpans() found a `12.5` span, want the amount cited as Amount.String renders it")
		}
	})
}

// The documentation rule every embedded snapshot follows is pinned here: each
// offering in a snapshot has its own section in the SOURCES.md beside it, and
// that section names the pages the offering comes from, the fact behind each
// capability and region, and the licence of each source. The test globs the
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

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/snapshot"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// The files a snapshot directory holds, and the headings its SOURCES.md must
// carry. Paths are relative to this package's directory, where go test runs.
const (
	catalogFileName  = "catalog.json"
	sourcesFileName  = "SOURCES.md"
	licencesHeading  = "## Licences of the sources"
	offeringsHeading = "## Offerings"
)

// scalewaySnapshot is the snapshot the rule is known to apply to today: the one
// target adapter with an embedded snapshot. Finding no snapshot at all would
// make the glob below pass while checking nothing, so its absence is a failure.
const scalewaySnapshot = "../scaleway/catalog.json"

// TestEveryOfferingHasASourcesSection pins that every offering in an embedded
// snapshot has exactly one section of its own in the SOURCES.md beside it, that
// no section names an offering the snapshot does not have, and that the section
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

	if !slices.Contains(paths, scalewaySnapshot) {
		t.Fatalf("the snapshots found are %v, want them to include %s", paths, scalewaySnapshot)
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

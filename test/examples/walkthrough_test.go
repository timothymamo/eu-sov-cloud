package examples_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	// walkthroughPath is the guide page that quotes the reports, from the module
	// root, where this package finds examples/ too.
	walkthroughPath = "docs/guide/try-it.md"

	// goldensDir is where TestSampleReports keeps the reports the walkthrough
	// quotes, from this package's directory.
	goldensDir = "testdata"

	// truncationMarker ends an excerpt the walkthrough cuts short. A trailing
	// line that is exactly this marker (or ellipsis) is the prose's "and so
	// on", not part of the report, so the matcher drops it.
	truncationMarker = "..."

	// ellipsis is the single-character form of truncationMarker, which typists
	// and editors produce as readily as three dots.
	ellipsis = "…"
)

// quote is one fenced excerpt of the walkthrough: the fence's info string, the
// 1-based line number of its opening fence in the page, and the lines between
// the fences.
type quote struct {
	info  string
	line  int
	lines []string
}

// firstLine is the excerpt's first line, or the empty string, for an error
// message that names the block a reader would search for.
func (q quote) firstLine() string {
	if len(q.lines) == 0 {
		return ""
	}

	return q.lines[0]
}

// TestWalkthroughQuotesTheGoldens pins the promise docs/guide/try-it.md makes in
// prose: every excerpt it shows is quoted from a golden report in
// test/examples/testdata, so `go test -update` makes a catalog snapshot change
// show up in the walkthrough's diff, and the page cannot go stale on its own.
//
// A block matches a golden when its lines appear in that golden in the block's
// order (excerptMatches). Excerpts are quoted for prose and may leave out the
// parts a reader does not need, so they are not required to be contiguous, but
// every quoted line must be a line the report actually prints, in the order it
// prints it: a block may not paraphrase, reorder or invent a line, and a change
// to a report or to a block fails the test until both agree again. A trailing
// line of exactly "..." or "…" marks where the excerpt was cut short and is not
// part of the report.
func TestWalkthroughQuotesTheGoldens(t *testing.T) {
	t.Parallel()

	quotes := fencedQuotes(t, readFile(t, filepath.Join(repoRoot, walkthroughPath)), walkthroughPath)
	goldens := goldenReports(t)

	for _, q := range quotes {
		t.Run(fmt.Sprintf("line %d", q.line), func(t *testing.T) {
			t.Parallel()

			var missing []string

			for _, golden := range goldens {
				line, ok := excerptMatches(golden.content, q.lines)
				if ok {
					return
				}

				missing = append(missing, fmt.Sprintf("%s: no line %q", golden.path, line))
			}

			t.Errorf("%s:%d: the ```%s excerpt starting %q is not quoted from any golden report; "+
				"regenerate the goldens with `go test ./test/examples -update` and update the page, or fix the page\n%s",
				walkthroughPath, q.line, q.info, q.firstLine(), strings.Join(missing, "\n"))
		})
	}
}

// TestQuotedExcerptsMatchAGolden pins the matcher on altered copies of an
// excerpt, so TestWalkthroughQuotesTheGoldens is known to fail on a stale page
// and not only to pass on the current one. The corpus is a report of the shape
// the walkthrough quotes; no case reads the page or a golden file.
func TestQuotedExcerptsMatchAGolden(t *testing.T) {
	t.Parallel()

	const golden = "# Migration assessment: `Scaleway`\n" +
		"\n" +
		"- Source: `aws`\n" +
		"\n" +
		"## Summary\n" +
		"\n" +
		"| Fit | Resources |\n" +
		"|---|---|\n" +
		"| direct | 5 |\n" +
		"| partial | 6 |\n" +
		"| none | 2 |\n" +
		"| total | 13 |\n" +
		"\n" +
		"## Required changes\n"

	head := []string{
		"# Migration assessment: `Scaleway`",
		"",
		"- Source: `aws`",
	}

	tests := []struct {
		name    string
		lines   []string
		want    bool
		missing string
	}{
		{
			name:  "a copy of an excerpt",
			lines: append(slices.Clone(head), "## Summary"),
			want:  true,
		},
		{
			name:  "an excerpt cut short after a whole line",
			lines: head,
			want:  true,
		},
		{
			name:  "an excerpt ending in the truncation marker",
			lines: append(slices.Clone(head), truncationMarker),
			want:  true,
		},
		{
			name:  "an excerpt ending in an ellipsis",
			lines: append(slices.Clone(head), ellipsis),
			want:  true,
		},
		{
			name: "an excerpt that leaves table rows out",
			lines: []string{
				"| Fit | Resources |",
				"|---|---|",
				"| direct | 5 |",
				"| total | 13 |",
			},
			want: true,
		},
		{
			name:    "an excerpt with a line the report does not print",
			lines:   []string{"# Migration assessment: `Scaleway`", "| direct | 6 |"},
			want:    false,
			missing: "| direct | 6 |",
		},
		{
			name:    "an excerpt whose lines are out of order",
			lines:   []string{"| Fit | Resources |", "## Summary", "| total | 13 |"},
			want:    false,
			missing: "## Summary",
		},
		{
			name:    "an excerpt of another report",
			lines:   []string{"# Migration assessment: `OVHcloud`"},
			want:    false,
			missing: "# Migration assessment: `OVHcloud`",
		},
		{
			name:    "an excerpt that is only the truncation marker",
			lines:   []string{truncationMarker},
			want:    false,
			missing: "an excerpt needs a line of its own",
		},
		{
			name:    "an empty excerpt",
			lines:   nil,
			want:    false,
			missing: "an excerpt needs a line of its own",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			missing, ok := excerptMatches(golden, test.lines)
			if ok != test.want {
				t.Fatalf("excerptMatches(%q) matched = %t, want %t", test.lines, ok, test.want)
			}

			if missing != test.missing {
				t.Errorf("excerptMatches(%q) missing line = %q, want %q", test.lines, missing, test.missing)
			}
		})
	}
}

// goldenReport is one golden report the walkthrough may quote: the path to read
// for a test and its content.
type goldenReport struct {
	path    string
	content string
}

// goldenReports returns every golden report in testdata, sorted by path. It
// fails the test when there is none, so a matcher test never passes by
// comparing against nothing.
func goldenReports(t *testing.T) []goldenReport {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(goldensDir, "*.golden"))
	if err != nil {
		t.Fatalf("list the golden reports in %s: %v", goldensDir, err)
	}

	if len(paths) == 0 {
		t.Fatalf("no golden report in %s: want the reports the walkthrough quotes", goldensDir)
	}

	slices.Sort(paths)

	reports := make([]goldenReport, len(paths))
	for i, path := range paths {
		reports[i] = goldenReport{path: path, content: readFile(t, path)}
	}

	return reports
}

// fencedQuotes returns the fenced code blocks of the page whose info string is
// markdown or json, the blocks that quote a report, in the order they appear. It
// fails the test when the page has no such block, so the check cannot pass
// vacuously, and when a fence is never closed.
func fencedQuotes(t *testing.T, page, name string) []quote {
	t.Helper()

	lines := strings.Split(page, "\n")
	var quotes []quote

	for i := 0; i < len(lines); i++ {
		info, fenced := strings.CutPrefix(strings.TrimRight(lines[i], " \t"), "```")
		if !fenced {
			continue
		}

		info = strings.TrimSpace(info)

		// Find the closing fence; the opening line is the one just read.
		closing := i + 1
		for closing < len(lines) && strings.TrimRight(lines[closing], " \t") != "```" {
			closing++
		}

		if closing == len(lines) {
			t.Fatalf("%s:%d: the ```%s fence is never closed", name, i+1, info)
		}

		if info == "markdown" || info == "json" {
			quotes = append(quotes, quote{info: info, line: i + 1, lines: slices.Clone(lines[i+1 : closing])})
		}

		i = closing
	}

	if len(quotes) == 0 {
		t.Fatalf("%s: no ```markdown or ```json block: want the page to quote the golden reports", name)
	}

	return quotes
}

// excerptMatches reports whether the excerpt's lines appear in the golden in
// the excerpt's order, and the first excerpt line that does not. The lines need
// not be contiguous: an excerpt may leave out what the prose does not need, so
// each line is looked for after the line matched before it, which still refuses
// a line the report never prints and lines in the wrong order. A trailing line
// that is exactly the truncation marker or an ellipsis is dropped first, since
// it marks a cut rather than quoting one. An excerpt of no lines matches
// nothing, so an empty block or a bare marker is not a quote of anything.
func excerptMatches(golden string, excerpt []string) (missing string, ok bool) {
	lines := excerpt

	if len(lines) > 0 && slices.Contains([]string{truncationMarker, ellipsis}, lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}

	if len(lines) == 0 {
		return "an excerpt needs a line of its own", false
	}

	goldenLines := strings.Split(strings.TrimSuffix(golden, "\n"), "\n")

	next := 0

	for _, line := range lines {
		found := slices.Index(goldenLines[next:], line)
		if found < 0 {
			return line, false
		}

		next += found + 1
	}

	return "", true
}

// readFile returns the content of the file at path, which is a constant of this
// test or a golden file's path, never input a program reads at run time.
func readFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(content)
}

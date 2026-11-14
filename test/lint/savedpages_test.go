package lint_test

import (
	"strings"
	"testing"
)

// The saved-page rule and its allow list live in savedpages_rule_test.go
// (issue #212): a saved price page, a browser copy of a target's pricing page
// kept because the site refuses automated clients, belongs in a scratch
// directory outside the repository and is never committed (each target's
// doc.go and SOURCES.md say so). Committing one would put a large third-party
// page, tracking scripts and session data included, next to a catalog snapshot.

// TestSavedPageProblems pins the checker the rule runs, on a probe tree held as
// a list of slash-separated paths relative to the module root. A path whose
// extension is .html or .htm, in any case, and that the allow list does not
// name is refused: one problem per refused path, in input order, each naming
// the path and pointing to the SOURCES.md convention.
func TestSavedPageProblems(t *testing.T) {
	t.Parallel()

	allowedPricingPage := map[string]bool{
		"internal/adapter/target/upcloud/pricing.html": true,
	}

	tests := []struct {
		name    string
		paths   []string
		allowed map[string]bool
		want    []string // the path each problem must name, in order
	}{
		{
			name:  "no path in the tree is an html page",
			paths: []string{"README.md", "internal/adapter/target/upcloud/catalog.json", "main.go"},
			want:  nil,
		},
		{
			name:  "empty tree",
			paths: nil,
			want:  nil,
		},
		{
			name:  "saved price page next to the catalog snapshot",
			paths: []string{"internal/adapter/target/upcloud/pricing.html"},
			want:  []string{"internal/adapter/target/upcloud/pricing.html"},
		},
		{
			name:  "htm extension in capitals",
			paths: []string{"docs/x.HTM"},
			want:  []string{"docs/x.HTM"},
		},
		{
			name:  "mixed-case html extension",
			paths: []string{"a/b.Html"},
			want:  []string{"a/b.Html"},
		},
		{
			name:  "html in the middle of a name",
			paths: []string{"a.html.txt"},
			want:  nil,
		},
		{
			name:  "go file whose name starts with html",
			paths: []string{"htmlfile.go"},
			want:  nil,
		},
		{
			name:  "go file in a directory called html",
			paths: []string{"a/html/b.go"},
			want:  nil,
		},
		{
			name:  "xhtml extension",
			paths: []string{"x.xhtml"},
			want:  nil,
		},
		{
			name:    "allowed page beside a refused one",
			paths:   []string{"internal/adapter/target/upcloud/pricing.html", "docs/saved.html"},
			allowed: allowedPricingPage,
			want:    []string{"docs/saved.html"},
		},
		{
			name:    "only an allowed page",
			paths:   []string{"internal/adapter/target/upcloud/pricing.html"},
			allowed: allowedPricingPage,
			want:    nil,
		},
		{
			name: "several refused pages, each named in input order",
			paths: []string{
				"internal/adapter/target/ovhcloud/pricing.html",
				"internal/adapter/target/upcloud/pricing.htm",
				"docs/saved/scaleWAY.HTML",
			},
			want: []string{
				"internal/adapter/target/ovhcloud/pricing.html",
				"internal/adapter/target/upcloud/pricing.htm",
				"docs/saved/scaleWAY.HTML",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := savedPageProblems(tc.paths, tc.allowed)
			if len(got) != len(tc.want) {
				t.Fatalf("savedPageProblems(%q, %v) returned %d problems, want %d: %q",
					tc.paths, tc.allowed, len(got), len(tc.want), got)
			}
			for i, want := range tc.want {
				if !strings.Contains(got[i], want) {
					t.Errorf("problem %d = %q, want it to name %s", i, got[i], want)
				}
				if !strings.Contains(got[i], "SOURCES.md") {
					t.Errorf("problem %d = %q, want it to point to the SOURCES.md convention", i, got[i])
				}
			}
		})
	}
}

// TestNoTrackedSavedPages is the rule: git tracks no saved price page. A saved
// page is read from outside the repository and never committed, so this test
// passes on the module as it is, with an empty allow list, and the branch that
// commits one fails here.
func TestNoTrackedSavedPages(t *testing.T) {
	t.Parallel()

	for _, problem := range savedPageProblems(trackedFiles(t), allowedSavedPages) {
		t.Error(problem)
	}
}

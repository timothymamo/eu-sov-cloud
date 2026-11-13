package execenv_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/timothymamo/shift2eu/test/internal/execenv"
)

// The matcher of the environ rule has a file of its own: it pins paths as
// values, and a bare path literal in a Go file under test/ is what
// TestTestTreeNamesNoProcEnviron refuses. Its cases therefore live in
// testdata/environment_paths.json, a file of values that no scan of Go files
// meets, and the test reads them at run time: no file under test/ is left out
// of that scan, and nothing added to this one goes unchecked. The rule's
// probes are .go.txt files, for the same reason.

// environmentPathCase is one value the matcher is pinned on, and what it must
// find in it, as testdata/environment_paths.json states the case.
type environmentPathCase struct {
	// Name is the subtest's name.
	Name string `json:"name"`
	// Value is the literal a rule would read.
	Value string `json:"value"`
	// Segment is the path segment the matcher must name, "" for none.
	Segment string `json:"segment"`
	// Found is whether the matcher must name a segment at all.
	Found bool `json:"found"`
}

// readEnvironmentPathCases returns the cases testdata/environment_paths.json
// holds, one per value the matcher is pinned on.
func readEnvironmentPathCases(t *testing.T) []environmentPathCase {
	t.Helper()

	document, err := os.ReadFile(filepath.Join("testdata", "environment_paths.json"))
	if err != nil {
		t.Fatalf("read the case file: %v", err)
	}

	var cases []environmentPathCase
	if err := json.Unmarshal(document, &cases); err != nil {
		t.Fatalf("parse the case file: %v", err)
	}
	// Guard the file itself: a table that lost its cases would pass whatever
	// the matcher says.
	if len(cases) == 0 {
		t.Fatal("the case file holds no case, want the values the matcher is pinned on")
	}
	return cases
}

// TestEnvironmentPathIn pins the shared segment matcher on values, the cases
// test/lint's rule pinned on source before issue #195 round 2 moved it here:
// its own entries, "/dev/fd", and the same ones without a leading slash, which
// os.DirFS("/") opens; a directory named without a trailing slash; and the two
// names that only look like one.
func TestEnvironmentPathIn(t *testing.T) {
	t.Parallel()

	for _, tc := range readEnvironmentPathCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			got, ok := execenv.EnvironmentPathIn(tc.Value)
			if got != tc.Segment || ok != tc.Found {
				t.Errorf("EnvironmentPathIn(%q) = (%q, %t), want (%q, %t)", tc.Value, got, ok, tc.Segment, tc.Found)
			}
		})
	}
}

package testutil

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// update is the -update flag every golden test in the repository shares, so one
// go test invocation with -update rewrites the golden files of the package under
// test and nothing else.
var update = flag.Bool("update", false, "rewrite golden files with the rendered output instead of comparing")

// Golden compares got with the golden file at path and fails tb when the two
// differ. With -update it writes got to path and passes instead.
//
// A golden file is the specification of an adapter's output, so it is written
// and reviewed by hand; -update is only a convenience for the reviewer, never
// something a CI run does. The file is written with mode 0600 because a golden
// file holds test data only, and the directory it lives in is created when
// missing so a new test can be added with its golden path alone.
//
// A failure reports the first line that differs, with both versions quoted, so
// the message names the one change; the line-by-line diff of the whole file is
// left to git. Golden keeps no state between calls, so tests that use it may run
// in parallel, and it compares bytes: a missing or extra trailing newline is a
// difference like any other.
func Golden(tb testing.TB, path string, got []byte) {
	tb.Helper()

	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			tb.Fatalf("golden: create the directory for %s: %v", path, err)
		}

		if err := os.WriteFile(path, got, 0o600); err != nil {
			tb.Fatalf("golden: write %s: %v", path, err)
		}

		return
	}

	// The path is the calling test's own golden file, a constant in testdata, never
	// anything a program reads at run time.
	want, err := os.ReadFile(path) //nolint:gosec // a test golden file, not user input
	if err != nil {
		tb.Fatalf("golden: read %s: %v", path, err)
	}

	if bytes.Equal(want, got) {
		return
	}

	tb.Errorf("golden: %s does not match the rendered output; run the test with -update to accept it\n%s",
		path, firstDifference(want, got))
}

// firstDifference reports the first line where want and got differ, with the
// line number as a reader of both files counts it. It returns a fixed message
// when the two are equal, which cannot happen for a caller that only asks after
// a byte comparison failed.
func firstDifference(want, got []byte) string {
	wantLines := strings.Split(string(want), "\n")
	gotLines := strings.Split(string(got), "\n")

	for i := range max(len(wantLines), len(gotLines)) {
		var wantLine, gotLine string
		if i < len(wantLines) {
			wantLine = wantLines[i]
		}

		if i < len(gotLines) {
			gotLine = gotLines[i]
		}

		if wantLine != gotLine {
			return fmt.Sprintf("line %d:\n  want: %q\n   got: %q", i+1, wantLine, gotLine)
		}
	}

	return "the files differ only in bytes that do not start a line"
}

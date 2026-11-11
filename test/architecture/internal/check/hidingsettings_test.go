package check_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// TestHidingSettings pins check.HidingSettings, the strict reader of
// .golangci.yml that refuses the settings which hide issues from `make lint`
// without touching a rule (issue #81). It reads the real .golangci.yml of this
// repository and applies one setting at a time to a copy of it in memory, so
// every case below is a change a reviewer could make to the real file.
//
// The contract:
//
//   - HidingSettings(golangci []byte) ([]string, error) returns one message per
//     setting that hides issues, and no message for a document that hides
//     nothing. The error is for a document the reader cannot read, which it
//     refuses rather than reads as a weaker one.
//   - A message names the setting by its dotted key as written in
//     .golangci.yml (issues.max-same-issues, linters.exclusions.paths,
//     linters.exclusions.paths-except, linters.enable, linters.disable,
//     linters.default, linters.exclusions.rules, run.tests, and the key of an
//     unknown issues entry). When the setting turns a linter off or keeps it
//     from a file, the message also names that linter.
//   - The settings refused are: a non-empty linters.exclusions.paths or
//     linters.exclusions.paths-except; every issues setting that reports fewer
//     issues than lint finds, meaning a non-zero max-issues-per-linter or
//     max-same-issues (0 is unlimited, the default, and is allowed), new: true,
//     any new-from-rev, new-from-patch or new-from-merge-base, whole-files:
//     true, and any key the check does not know, which fails closed; and for
//     the two linters the architecture depends on, depguard and forbidigo, that
//     the linter is in linters.enable and not in linters.disable,
//     linters.default is not none, and run.tests is not false.
//   - A linters.exclusions.rules entry that names depguard is refused: it drops
//     depguard's issues for the files its path matches. An entry that names
//     forbidigo is refused when it has no text, which drops every forbidigo
//     issue in its path; with a text it only narrows, and test/lint pins the
//     texts of the forbidigo group rules (TestForbidigoExclusionsDropOneGroup).
//   - issues.fix and issues.uniq-by-line: false hide nothing and are allowed.
func TestHidingSettings(t *testing.T) {
	t.Parallel()

	real := repositoryGolangci(t)

	tests := []struct {
		name string
		// mutate applies the setting to the real .golangci.yml. nil keeps the
		// document as it is, for the case that today's config passes.
		mutate func(t *testing.T, doc string) string
		// want are substrings the returned messages must hold between them:
		// the setting, and the linter the setting turns off. Empty want means
		// the document must report no problem at all.
		want []string
		// wantErr is set for a document the check must refuse, rather than
		// read less of it.
		wantErr bool
	}{
		{
			name: "today's .golangci.yml hides nothing",
		},

		// Exclusions: a path list drops every issue in the paths it names, and
		// a path-except list drops every issue everywhere else.
		{
			name:   "linters.exclusions.paths hides every issue in its paths",
			mutate: insertsUnderLintersExclusions("    paths:\n      - internal/"),
			want:   []string{"linters.exclusions.paths"},
		},
		{
			name:   "linters.exclusions.paths-except hides every issue elsewhere",
			mutate: insertsUnderLintersExclusions("    paths-except:\n      - internal/"),
			want:   []string{"linters.exclusions.paths-except"},
		},

		// The issues caps and filters: 0 is unlimited, which is the default,
		// and every other value reports a subset of what lint finds.
		{
			name:   "issues.max-issues-per-linter caps how many issues a linter reports",
			mutate: appendsBlock("issues:\n  max-issues-per-linter: 50\n"),
			want:   []string{"issues.max-issues-per-linter"},
		},
		{
			name:   "issues.max-same-issues caps how many issues of one text are reported",
			mutate: appendsBlock("issues:\n  max-same-issues: 3\n"),
			want:   []string{"issues.max-same-issues"},
		},
		{
			name:   "issues.new reports only the issues of new code",
			mutate: appendsBlock("issues:\n  new: true\n"),
			want:   []string{"issues.new"},
		},
		{
			name:   "issues.new-from-rev reports only the issues after a revision",
			mutate: appendsBlock("issues:\n  new-from-rev: main\n"),
			want:   []string{"issues.new-from-rev"},
		},
		{
			name:   "issues.new-from-patch reports only the issues of a patch",
			mutate: appendsBlock("issues:\n  new-from-patch: fix.patch\n"),
			want:   []string{"issues.new-from-patch"},
		},
		{
			name:   "issues.new-from-merge-base reports only the issues after a merge base",
			mutate: appendsBlock("issues:\n  new-from-merge-base: main\n"),
			want:   []string{"issues.new-from-merge-base"},
		},
		{
			name:   "issues.whole-files reports only the issues a change rewrote a file for",
			mutate: appendsBlock("issues:\n  whole-files: true\n"),
			want:   []string{"issues.whole-files"},
		},
		{
			// The check knows every issues key golangci-lint v2 has, so a key
			// it does not know is one a later version added: it may cap or
			// filter, and refusing it is the only way to be sure it does not.
			name:   "an issues key the check does not know fails closed",
			mutate: appendsBlock("issues:\n  max-issues-per-line: 5\n"),
			want:   []string{"issues.max-issues-per-line"},
		},
		{
			name:   "issues caps set to 0 report every issue, as the default does",
			mutate: appendsBlock("issues:\n  max-issues-per-linter: 0\n  max-same-issues: 0\n"),
		},
		{
			name:   "issues settings that report every issue hide nothing",
			mutate: appendsBlock("issues:\n  new: false\n  whole-files: false\n  fix: false\n  uniq-by-line: false\n"),
		},

		// depguard and forbidigo: every way of turning one off.
		{
			name:   "depguard missing from linters.enable never runs",
			mutate: deletesLine("    - depguard"),
			want:   []string{"linters.enable", "depguard"},
		},
		{
			name:   "forbidigo missing from linters.enable never runs",
			mutate: deletesLine("    - forbidigo"),
			want:   []string{"linters.enable", "forbidigo"},
		},
		{
			name:   "depguard in linters.disable never runs",
			mutate: insertsAfter("linters:", "  disable:\n    - depguard"),
			want:   []string{"linters.disable", "depguard"},
		},
		{
			name:   "forbidigo in linters.disable never runs",
			mutate: insertsAfter("linters:", "  disable:\n    - forbidigo"),
			want:   []string{"linters.disable", "forbidigo"},
		},
		{
			// linters.default: none drops the linters the standard set names,
			// and a config where it is set is one where every linter has to be
			// enabled by hand: refuse it, whether or not the two are enabled.
			name:   "linters.default: none turns the standard linters off",
			mutate: replacesLine("  default: standard", "  default: none"),
			want:   []string{"linters.default", "none"},
		},
		{
			name:   "an exclusion rule naming depguard drops its issues on a path",
			mutate: insertsAfter("    rules:", "      - path: internal/domain/\n        linters:\n          - depguard"),
			want:   []string{"linters.exclusions.rules", "depguard"},
		},
		{
			name:   "an exclusion rule naming forbidigo with no text drops all of its issues",
			mutate: insertsAfter("    rules:", "      - path: internal/domain/\n        linters:\n          - forbidigo"),
			want:   []string{"linters.exclusions.rules", "forbidigo"},
		},
		{
			// run.tests: false stops every linter seeing _test.go files, and
			// depguard's and forbidigo's architecture rules hold for the tests
			// of the layers too. Today's config sets only run.build-tags.
			name:   "run.tests: false keeps the linters from every test file",
			mutate: insertsAfter("run:", "  tests: false"),
			want:   []string{"run.tests"},
		},
		{
			name:   "run.tests: true lints every test file",
			mutate: insertsAfter("run:", "  tests: true"),
		},

		// A document the reader cannot read is an error, not a pass: a check
		// that skips what it does not understand cannot say the file hides
		// nothing.
		{
			name:    "a document with a tab the reader refuses is an error",
			mutate:  appendsBlock("issues:\n\tmax-same-issues: 3\n"),
			wantErr: true,
		},
		{
			name:    "a repeated top-level key the reader refuses is an error",
			mutate:  appendsBlock("issues:\n  max-same-issues: 3\nissues:\n  max-issues-per-linter: 3\n"),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := real
			if tc.mutate != nil {
				doc = tc.mutate(t, doc)
			}

			problems, err := check.HidingSettings([]byte(doc))

			if tc.wantErr {
				if err == nil {
					t.Fatalf("HidingSettings() = %q, nil; want an error for a document the reader cannot read", problems)
				}

				return
			}

			if err != nil {
				t.Fatalf("HidingSettings() error: %v; want the document read", err)
			}

			if len(tc.want) == 0 {
				if len(problems) != 0 {
					t.Errorf("HidingSettings() = %q; want no problem, the document hides nothing", problems)
				}

				return
			}

			joined := strings.Join(problems, "\n")
			for _, want := range tc.want {
				if !strings.Contains(joined, want) {
					t.Errorf("HidingSettings() = %q; want a problem naming %q", problems, want)
				}
			}
		})
	}
}

// repositoryGolangci reads the .golangci.yml of this repository, the document
// every case of TestHidingSettings changes, so the cases test the file
// `make lint` reads and not a copy that could drift from it.
func repositoryGolangci(t *testing.T) string {
	t.Helper()

	path := filepath.Join("..", "..", "..", "..", ".golangci.yml")

	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(doc)
}

// mutation writes one setting into the real .golangci.yml.
type mutation func(t *testing.T, doc string) string

// insertsAfter returns a mutation that inserts block directly after the only
// line of the document equal to anchor.
func insertsAfter(anchor, block string) mutation {
	return func(t *testing.T, doc string) string {
		t.Helper()

		return edit(t, doc, func(lines []string) []string {
			i := onlyLine(t, lines, anchor)

			return slices.Concat(lines[:i+1], strings.Split(block, "\n"), lines[i+1:])
		})
	}
}

// insertsUnderLintersExclusions returns a mutation that inserts block as the
// first child of linters.exclusions. exclusions sits under linters and under
// formatters, so the mutation finds the one between the linters: and the
// formatters: line rather than the first exclusions: in the file.
func insertsUnderLintersExclusions(block string) mutation {
	return func(t *testing.T, doc string) string {
		t.Helper()

		return edit(t, doc, func(lines []string) []string {
			start := onlyLine(t, lines, "linters:")
			end := onlyLine(t, lines, "formatters:")

			for i := start + 1; i < end; i++ {
				if lines[i] == "  exclusions:" {
					return slices.Concat(lines[:i+1], strings.Split(block, "\n"), lines[i+1:])
				}
			}

			t.Fatal(".golangci.yml holds no linters.exclusions: line the mutation could put a setting in")

			return nil
		})
	}
}

// deletesLine returns a mutation that deletes the only line of the document
// equal to text.
func deletesLine(text string) mutation {
	return func(t *testing.T, doc string) string {
		t.Helper()

		return edit(t, doc, func(lines []string) []string {
			i := onlyLine(t, lines, text)

			return slices.Concat(lines[:i], lines[i+1:])
		})
	}
}

// replacesLine returns a mutation that rewrites the only line of the document
// equal to old as new.
func replacesLine(old, new string) mutation {
	return func(t *testing.T, doc string) string {
		t.Helper()

		return edit(t, doc, func(lines []string) []string {
			i := onlyLine(t, lines, old)
			out := slices.Clone(lines)
			out[i] = new

			return out
		})
	}
}

// appendsBlock returns a mutation that appends block, a top-level key and its
// children, to the end of the document.
func appendsBlock(block string) mutation {
	return func(t *testing.T, doc string) string {
		t.Helper()

		return edit(t, doc, func(lines []string) []string {
			return slices.Concat(lines, strings.Split(block, "\n"))
		})
	}
}

// edit rewrites the lines of doc with rewrite and returns the document. It
// fails when the rewrite left the document unchanged, so no case of
// TestHidingSettings passes on nothing.
func edit(t *testing.T, doc string, rewrite func(lines []string) []string) string {
	t.Helper()

	lines := strings.Split(strings.TrimSuffix(doc, "\n"), "\n")

	mutated := strings.Join(rewrite(lines), "\n") + "\n"
	if mutated == doc {
		t.Fatal("the mutation left .golangci.yml unchanged")
	}

	return mutated
}

// onlyLine returns the index of the only line of lines equal to text, and fails
// when there is none, or more than one, so a mutation cannot change a line it
// did not mean.
func onlyLine(t *testing.T, lines []string, text string) int {
	t.Helper()

	found := -1
	for i, line := range lines {
		if line != text {
			continue
		}

		if found >= 0 {
			t.Fatalf(".golangci.yml: %q is on more than one line, so the mutation is ambiguous", text)
		}

		found = i
	}

	if found < 0 {
		t.Fatalf(".golangci.yml: no line is %q, so the mutation has nothing to change", text)
	}

	return found
}

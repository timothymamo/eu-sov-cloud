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
//     linters.exclusions.paths-except; a linters.exclusions.generated that is
//     not disable (the key must be there: golangci-lint v2 reads it as lax when
//     it is absent, and a generated file is then never linted, depguard and
//     forbidigo included); every issues setting that reports fewer issues than
//     lint finds, meaning a non-zero max-issues-per-linter or max-same-issues
//     (0 means unlimited; golangci-lint v2's defaults are 50 and 3), new: true,
//     any new-from-rev, new-from-patch or new-from-merge-base, whole-files:
//     true, and any key the check does not know, which fails closed; and for
//     the two linters the architecture depends on, depguard and forbidigo, that
//     the linter is in linters.enable and not in linters.disable,
//     linters.default is not none or fast, and run.tests is not false.
//   - The document's shape is an allow list too, because a key the check does
//     not read is one a later golangci-lint may act on: version must be "2";
//     run may hold only the keys that do not hide an issue, so
//     run.issues-exit-code: 0 is refused and 1, or timeout and the other keys
//     that only bound how lint runs, are allowed; linters may hold only default,
//     enable, disable, settings and exclusions; linters.default may be standard
//     or all, not fast, none or a value the check does not know; and
//     linters.exclusions.presets may hold only comments, std-error-handling,
//     common-false-positives and legacy.
//   - A linters.exclusions.rules entry that names depguard is refused: it drops
//     depguard's issues for the files its path matches. An entry that names
//     forbidigo is refused when it has no text, or an empty one, which drops
//     every forbidigo issue in its path; with a text it only narrows, and
//     test/lint pins the texts of the forbidigo group rules
//     (TestForbidigoExclusionsDropOneGroup).
//     An entry that names no linter drops every linter's issues on its path and
//     is refused, and so is one that holds a key the check does not know: a key
//     a later golangci-lint adds may select files the entry then hides issues
//     in. linters.default may be standard or all; a value the check does not
//     know fails closed like an unknown issues key does.
//   - issues.fix: false and issues.uniq-by-line, either value, hide nothing and
//     are allowed: uniq-by-line merges the issues of one line, it drops none.
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

		// The document's shape: golangci-lint v2 reads version, run, linters,
		// formatters, issues, output and severity at the top level, so a key it
		// does not know is one a later version added, which may hide issues.
		{
			name:   "a top-level key the check does not know fails closed",
			mutate: appendsBlock("service:\n  golangci-lint-version: 2.6.0\n"),
			want:   []string{"service"},
		},
		{
			// version "2" is the document shape this check reads; a document
			// without one, or with a v1, may hold settings that hide issues in
			// a place the check does not look.
			name:   "a document whose version is not \"2\" names settings the check does not read",
			mutate: replacesLine(`version: "2"`, `version: "1"`),
			want:   []string{"version"},
		},
		{
			name:   "a document with no version names settings the check does not read",
			mutate: deletesLine(`version: "2"`),
			want:   []string{"version"},
		},

		// run: every key but the one that makes lint exit 0 while it prints
		// issues only bounds how lint runs, so the key list is an allow list and
		// an unknown key fails closed.
		{
			name:   "run.issues-exit-code: 0 exits 0 with the issues it found",
			mutate: insertsAfter("run:", "  issues-exit-code: 0"),
			want:   []string{"run.issues-exit-code"},
		},
		{
			name:   "run.issues-exit-code: 1 fails when it finds an issue",
			mutate: insertsAfter("run:", "  issues-exit-code: 1"),
		},
		{
			name:   "a run key the check does not know fails closed",
			mutate: insertsAfter("run:", "  skip-everything: true"),
			want:   []string{"run.skip-everything"},
		},
		{
			name:   "run.timeout only bounds how long lint runs",
			mutate: insertsAfter("run:", "  timeout: 5m"),
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
		{
			// The check knows every linters.exclusions key golangci-lint v2
			// reads, so a key it does not know is one a later version added or
			// one v1 had: either way it may leave out files, and refusing it is
			// the only way to be sure it hides nothing.
			name:   "a linters.exclusions key the check does not know fails closed",
			mutate: insertsUnderLintersExclusions("    exclude-dirs:\n      - internal/"),
			want:   []string{"linters.exclusions.exclude-dirs"},
		},

		{
			// A generated file is never linted, so every issue in it, depguard's
			// and forbidigo's included, goes unreported. golangci-lint v2 reads
			// generated as lax when the key is absent, and lax drops every file
			// with a "code generated" comment above its package clause, which a
			// hand-written file can carry too.
			name:   "linters.exclusions.generated: lax drops every file that looks generated",
			mutate: replacesUnderLintersExclusions("    generated: disable", "    generated: lax"),
			want:   []string{"linters.exclusions.generated", "depguard", "forbidigo"},
		},
		{
			name:   "linters.exclusions.generated: strict drops every file that looks generated",
			mutate: replacesUnderLintersExclusions("    generated: disable", "    generated: strict"),
			want:   []string{"linters.exclusions.generated", "depguard", "forbidigo"},
		},
		{
			// The key has to be there: absent, golangci-lint reads it as lax.
			name:   "a missing linters.exclusions.generated is lax, which drops every file that looks generated",
			mutate: deletesUnderLintersExclusions("    generated: disable"),
			want:   []string{"linters.exclusions.generated", "depguard", "forbidigo"},
		},
		{
			// golangci-lint v2 has four presets, and each drops the known false
			// positives it is named for. A preset it does not have is one a later
			// version added, which may drop more, so it fails closed.
			name:   "a linters.exclusions preset the check does not know fails closed",
			mutate: insertsAfter("    presets:", "      - everything"),
			want:   []string{"linters.exclusions.presets"},
		},
		{
			name:   "a linters.exclusions preset golangci-lint has drops only its known false positives",
			mutate: insertsAfter("    presets:", "      - comments"),
		},

		// The issues caps and filters: 0 means unlimited, where golangci-lint
		// v2's defaults cap the report at 50 issues a linter and 3 of one text,
		// so any other value reports a subset of what lint finds.
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
			name:   "issues caps set to 0 lift the default caps and report every issue",
			mutate: appendsBlock("issues:\n  max-issues-per-linter: 0\n  max-same-issues: 0\n"),
		},
		{
			// uniq-by-line merges the issues of one line into one, which changes
			// how many lines the report has, not which issues it holds.
			name:   "issues.uniq-by-line: true merges the issues of one line only",
			mutate: appendsBlock("issues:\n  uniq-by-line: true\n"),
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
			// The check knows every linters key golangci-lint v2 reads, so a key
			// it does not know is one a later version added or one v1 had: either
			// way it may turn a linter off, and refusing it is the only way to be
			// sure it does not.
			name:   "a linters key the check does not know fails closed",
			mutate: insertsAfter("linters:", "  enable-all: true"),
			want:   []string{"linters.enable-all"},
		},
		{
			name:   "linters.fast is a v1 setting the check does not know",
			mutate: insertsAfter("linters:", "  fast: true"),
			want:   []string{"linters.fast"},
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
			// fast leaves out the linters that need the whole program, so it
			// reports fewer issues than the standard set does.
			name:   "linters.default: fast leaves the slow linters out",
			mutate: replacesLine("  default: standard", "  default: fast"),
			want:   []string{"linters.default", "fast"},
		},
		{
			name:   "linters.default: all runs every linter",
			mutate: replacesLine("  default: standard", "  default: all"),
		},
		{
			// standard and all report every issue of every linter they turn on. A
			// default the check does not know may leave a linter out, so it fails
			// closed rather than pass.
			name:   "a linters.default the check does not know fails closed",
			mutate: replacesLine("  default: standard", "  default: everything"),
			want:   []string{"linters.default", "everything"},
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
			// An empty text matches every forbidigo issue, as no text does, so
			// the two forms are refused alike: only a text scoped to one group
			// leaves another group's issues reported.
			name:   "an exclusion rule naming forbidigo with an empty text drops all of its issues",
			mutate: insertsAfter("    rules:", "      - path: internal/domain/\n        linters:\n          - forbidigo\n        text: \"\""),
			want:   []string{"linters.exclusions.rules", "forbidigo"},
		},
		{
			name:   "an exclusion rule naming forbidigo with a single-quoted empty text drops all of its issues",
			mutate: insertsAfter("    rules:", "      - path: internal/domain/\n        linters:\n          - forbidigo\n        text: ''"),
			want:   []string{"linters.exclusions.rules", "forbidigo"},
		},
		{
			// An entry with no linters: key, and one with an empty list, drops
			// the issues of every linter on its path, depguard's and
			// forbidigo's included; naming the linters it is for is the only
			// form the check can read as one group's exclusion.
			name:   "an exclusion rule naming no linter drops every linter's issues",
			mutate: insertsAfter("    rules:", "      - path: internal/domain/"),
			want:   []string{"linters.exclusions.rules", "names no linter"},
		},
		{
			// The check knows path, path-except, text, source and linters. A key
			// a later version adds may select files the entry then drops every
			// issue in, so it fails closed.
			name:   "an exclusion rule with a key the check does not know fails closed",
			mutate: insertsAfter("    rules:", "      - path: internal/domain/\n        linters:\n          - gosec\n        severity: warning"),
			want:   []string{"linters.exclusions.rules", "severity"},
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
// first child of linters.exclusions.
func insertsUnderLintersExclusions(block string) mutation {
	return func(t *testing.T, doc string) string {
		t.Helper()

		return edit(t, doc, func(lines []string) []string {
			i := lintersExclusions(t, lines)

			return slices.Concat(lines[:i+1], strings.Split(block, "\n"), lines[i+1:])
		})
	}
}

// replacesUnderLintersExclusions returns a mutation that rewrites the only line
// of linters.exclusions equal to old as new.
func replacesUnderLintersExclusions(old, new string) mutation {
	return func(t *testing.T, doc string) string {
		t.Helper()

		return edit(t, doc, func(lines []string) []string {
			i := onlyLintersExclusionsLine(t, lines, old)
			out := slices.Clone(lines)
			out[i] = new

			return out
		})
	}
}

// deletesUnderLintersExclusions returns a mutation that deletes the only line
// of linters.exclusions equal to text.
func deletesUnderLintersExclusions(text string) mutation {
	return func(t *testing.T, doc string) string {
		t.Helper()

		return edit(t, doc, func(lines []string) []string {
			i := onlyLintersExclusionsLine(t, lines, text)

			return slices.Concat(lines[:i], lines[i+1:])
		})
	}
}

// lintersExclusions returns the index of the linters.exclusions: line of the
// document. exclusions sits under linters and under formatters, so the
// mutations find the one between the linters: and the formatters: line rather
// than the first exclusions: in the file.
func lintersExclusions(t *testing.T, lines []string) int {
	t.Helper()

	start := onlyLine(t, lines, "linters:")
	end := onlyLine(t, lines, "formatters:")

	for i := start + 1; i < end; i++ {
		if lines[i] == "  exclusions:" {
			return i
		}
	}

	t.Fatal(".golangci.yml holds no linters.exclusions: line the mutation could put a setting in")

	return 0
}

// onlyLintersExclusionsLine returns the index of the only line of
// linters.exclusions equal to text, and fails when there is none, or more than
// one, so a mutation cannot change a line it did not mean. formatters has an
// exclusions block of its own, with a generated: line equal to the linters'
// one, so the search stops at formatters:.
func onlyLintersExclusionsLine(t *testing.T, lines []string, text string) int {
	t.Helper()

	start := lintersExclusions(t, lines)
	end := onlyLine(t, lines, "formatters:")

	found := -1
	for i := start + 1; i < end; i++ {
		if lines[i] != text {
			continue
		}

		if found >= 0 {
			t.Fatalf(".golangci.yml: %q is on more than one line of linters.exclusions, so the mutation is ambiguous", text)
		}

		found = i
	}

	if found < 0 {
		t.Fatalf(".golangci.yml: no line of linters.exclusions is %q, so the mutation has nothing to change", text)
	}

	return found
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

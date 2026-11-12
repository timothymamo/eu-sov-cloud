package check_test

import (
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// The identifiers both documents record, as they read today: the
// release-please App's ID and slug, and the placeholders the maintainer
// replaces with the release-tag App's own before the first release cut with
// the split (#157, ADR 0021).
const (
	recordedReleasePleaseID   = "5729776"
	recordedReleasePleaseSlug = "shift2eu-release"
	tagAppIDPlaceholder       = "<RELEASE_TAG_APP_ID: set by the maintainer before merge>"
	tagAppSlugPlaceholder     = "<RELEASE_TAG_APP_SLUG: set by the maintainer before merge>"
)

// releaseTagSkillDoc returns the identifier table of
// .claude/skills/release-tag/SKILL.md as the skill writes it: one row per App,
// the App's name and then its ID, its slug and its bot login, each value in
// backticks.
func releaseTagSkillDoc(releasePleaseID, releasePleaseSlug, tagID, tagSlug string) []byte {
	return []byte("   | App | App ID | Slug | Bot login |\n" +
		"   |---|---|---|---|\n" +
		"   | release-please App | `" + releasePleaseID + "` | `" + releasePleaseSlug + "` | `app/" + releasePleaseSlug + "` (`gh`), `" + releasePleaseSlug + "[bot]` (REST API) |\n" +
		"   | release-tag App | `" + tagID + "` | `" + tagSlug + "` | `app/<slug>` |\n")
}

// releaseTagADRDoc returns the identifier table of ADR 0021 as the amendment
// writes it: one row per identifier, the release-please App's value and then
// the release-tag App's, each in backticks.
func releaseTagADRDoc(releasePleaseID, releasePleaseSlug, tagID, tagSlug string) []byte {
	return []byte("| | release-please App | release-tag App |\n" +
		"|---|---|---|\n" +
		"| App ID | `" + releasePleaseID + "` | `" + tagID + "` |\n" +
		"| Slug (bot login) | `" + releasePleaseSlug + "` (`" + releasePleaseSlug + "[bot]`, `app/" + releasePleaseSlug + "` in `gh`) | `" + tagSlug + "` |\n")
}

// rewriteTableRow returns doc with every Markdown table row whose first cell is
// label passed through rewrite, keeping the order of the document: what a case
// that writes one row twice, or leaves it out, needs.
func rewriteTableRow(t *testing.T, doc []byte, label string, rewrite func(row string) []string) []byte {
	t.Helper()

	lines := strings.Split(string(doc), "\n")
	out := make([]string, 0, len(lines))
	found := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "| "+label+" |") && strings.HasSuffix(trimmed, "|") {
			found = true
			out = append(out, rewrite(line)...)

			continue
		}

		out = append(out, line)
	}

	if !found {
		t.Fatalf("%q holds no table row labelled %q", doc, label)
	}

	return []byte(strings.Join(out, "\n"))
}

// byteMutate returns doc with old replaced by new, failing unless doc holds
// old: a case that breaks a cell names the cell.
func byteMutate(t *testing.T, doc []byte, old, new string) []byte {
	t.Helper()

	if !strings.Contains(string(doc), old) {
		t.Fatalf("the fixture holds no %q", old)
	}

	return []byte(strings.ReplaceAll(string(doc), old, new))
}

// TestReleaseAppIdentifiers pins the two documents that record the Apps'
// identifiers: the release-tag skill and ADR 0021. The release-tag skill fails
// closed on the recorded release-tag App's ID, and both documents compare the
// rulesets and the release PR's author against the recorded slugs, so the two
// records must agree, the release-tag App's ID must be a number or the
// maintainer's placeholder and nothing else, and neither of the release-tag
// App's values may be the release-please App's: a release-tag App that is
// really the release-please App would let the App that opens and merges the
// release PR bypass the tag ruleset, which is the split #157 undoes. The
// placeholder is accepted in both files, so CI stays green until the
// maintainer records the real identifiers.
//
// A case says whether the two documents it renders are a violation; the
// message itself is the check's to word.
func TestReleaseAppIdentifiers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// skill and adr are the two documents, each rendered by the fixture
		// above with the four values the case records.
		skill, adr []byte
		wantClean  bool
	}{
		{
			// The state of the repository until the maintainer records the
			// release-tag App: both documents hold the placeholders, which is
			// not a violation.
			name:      "both placeholders",
			skill:     releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, tagAppIDPlaceholder, tagAppSlugPlaceholder),
			adr:       releaseTagADRDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, tagAppIDPlaceholder, tagAppSlugPlaceholder),
			wantClean: true,
		},
		{
			name:      "both identifiers recorded",
			skill:     releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"),
			adr:       releaseTagADRDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"),
			wantClean: true,
		},
		{
			// One identifier written down and the other not: the two records
			// disagree about what the release-tag App is.
			name:  "the same ID in one file and not the other",
			skill: releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"),
			adr:   releaseTagADRDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "654321", "shift2eu-release-tag"),
		},
		{
			name:  "the same slug in one file and not the other",
			skill: releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"),
			adr:   releaseTagADRDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-tag"),
		},
		{
			// The skill fails closed on anything that is not a number, so an ID
			// it cannot read as one is one no release is cut against.
			name:  "an ID that is not a number",
			skill: releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "12a", "shift2eu-release-tag"),
			adr:   releaseTagADRDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "12a", "shift2eu-release-tag"),
		},
		{
			// The release-please App's own ID: the App that opens and merges
			// the release PR would be the App that bypasses the tag ruleset.
			name:  "the release-please App's ID as the release-tag App's",
			skill: releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, recordedReleasePleaseID, "shift2eu-release-tag"),
			adr:   releaseTagADRDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, recordedReleasePleaseID, "shift2eu-release-tag"),
		},
		{
			// The same App under the same slug, twice: the two rows would name
			// one App.
			name:  "the release-please App's slug as the release-tag App's",
			skill: releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", recordedReleasePleaseSlug),
			adr:   releaseTagADRDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", recordedReleasePleaseSlug),
		},
		{
			// The release-please App's own record, changed in one document: a
			// reader of one file checks the rulesets and the release PR's
			// author against a different App than a reader of the other.
			name:  "the release-please App's ID changed in one file",
			skill: releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"),
			adr:   releaseTagADRDoc("5729777", recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"),
		},
		{
			name:  "the release-please App's slug changed in one file",
			skill: releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"),
			adr:   releaseTagADRDoc(recordedReleasePleaseID, "shift2eu-release-please", "123456", "shift2eu-release-tag"),
		},
		{
			// Neither document recorded the release-please App's identifiers at
			// all: the checks that compare against them would compare nothing.
			name:  "no release-please App ID in either file",
			skill: releaseTagSkillDoc("", recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"),
			adr:   releaseTagADRDoc("", recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"),
		},
		{
			// One of the release-tag App's two values recorded and the other
			// still the maintainer's placeholder, in both files: the row would
			// name an App whose slug nobody knows.
			name:  "one placeholder beside one recorded value",
			skill: releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", tagAppSlugPlaceholder),
			adr:   releaseTagADRDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", tagAppSlugPlaceholder),
		},
		{
			// A slug no App may hold: both documents compare it against the
			// release PR's author, so a slug that is not one is a value no
			// reader matches.
			name:  "a release-tag slug that is not a slug",
			skill: releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "Shift2EU_Tag"),
			adr:   releaseTagADRDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "Shift2EU_Tag"),
		},
		{
			// The skill without the release-tag App's row: it records the
			// release-tag App not at all, while the ADR records it, and a reader
			// of the skill checks the rulesets against no App.
			name:  "the release-tag App's row missing from the skill",
			skill: rewriteTableRow(t, releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"), "release-tag App", func(string) []string { return nil }),
			adr:   releaseTagADRDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"),
		},
		{
			// The release-please App's row twice: the skill holds two records of
			// one App, and which of them is the document's is nobody's to say.
			name:  "the release-please App's row twice in the skill",
			skill: rewriteTableRow(t, releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"), "release-please App", func(row string) []string { return []string{row, row} }),
			adr:   releaseTagADRDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"),
		},
		{
			// Each cell is read as the first backticked text it holds, so a
			// value written without backticks records nothing, and the two
			// records no longer agree.
			name:  "a cell with no backticked value",
			skill: byteMutate(t, releaseTagSkillDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"), "| `123456` |", "| 123456 |"),
			adr:   releaseTagADRDoc(recordedReleasePleaseID, recordedReleasePleaseSlug, "123456", "shift2eu-release-tag"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := check.ReleaseAppIdentifiers(tc.skill, tc.adr)

			if tc.wantClean {
				if len(got) != 0 {
					t.Errorf("ReleaseAppIdentifiers() = %q, want no message", got)
				}

				return
			}

			if len(got) == 0 {
				t.Errorf("ReleaseAppIdentifiers() = no message, want one naming the disagreement between the two documents")
			}
		})
	}
}

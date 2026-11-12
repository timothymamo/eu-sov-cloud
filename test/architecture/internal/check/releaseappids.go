package check

import (
	"fmt"
	"regexp"
	"strings"
)

// The documents that record the two release Apps' identifiers (#157, ADR
// 0021), and what they record.
const (
	releaseTagSkillName = ".claude/skills/release-tag/SKILL.md"
	releaseTagADRName   = "docs/adr/0021-propose-releases-with-release-please-and-tag-with-a-github-app.md"

	// releasePleaseAppID is the release-please App's ID, recorded in both
	// documents; its slug is releasePleaseSlug.
	releasePleaseAppID = "5729776"

	// The placeholders both documents hold for the release-tag App until the
	// maintainer records its ID and slug.
	tagAppIDPlaceholder   = "<RELEASE_TAG_APP_ID: set by the maintainer before merge>"
	tagAppSlugPlaceholder = "<RELEASE_TAG_APP_SLUG: set by the maintainer before merge>"
)

// appIDPattern is a GitHub App ID, appSlugPattern an App's slug.
var (
	appIDPattern   = regexp.MustCompile(`^[1-9][0-9]*$`)
	appSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	backticked     = regexp.MustCompile("`([^`]*)`")
)

// appIdentifiers are one App's recorded ID and slug.
type appIdentifiers struct {
	id, slug string
}

// recordedApps are the two Apps' identifiers as one document records them.
type recordedApps struct {
	releasePlease, releaseTag appIdentifiers
}

// ReleaseAppIdentifiers holds the two documents that record the release Apps'
// identifiers, the release-tag skill (skill) and ADR 0021 (adr), to each
// other, and returns one message per problem. The skill's table holds one row
// per App (| release-please App | `<id>` | `<slug>` | ... | and
// | release-tag App | ... |), the ADR's one row per identifier (| App ID |
// `<release-please>` | `<release-tag>` | and | Slug (bot login) | ... |), each
// value the first backticked text of its cell.
//
// Both documents must record the same identifiers. The release-please App's
// are releasePleaseAppID and releasePleaseSlug. The release-tag App's are
// either both the maintainer's placeholders, or an App ID that is a number
// and a slug, neither of them the release-please App's: a release-tag App that
// is the release-please App would let the App that opens and merges the
// release PR bypass the tag ruleset, which is the split of #157 undone.
func ReleaseAppIdentifiers(skill, adr []byte) []string {
	var messages []string

	fromSkill, skillMessages := skillAppIdentifiers(skill)
	fromADR, adrMessages := adrAppIdentifiers(adr)

	messages = append(messages, skillMessages...)
	messages = append(messages, adrMessages...)

	if len(messages) != 0 {
		return messages
	}

	for _, doc := range []struct {
		name string
		apps recordedApps
	}{{releaseTagSkillName, fromSkill}, {releaseTagADRName, fromADR}} {
		messages = append(messages, recordedAppProblems(doc.name, doc.apps)...)
	}

	if fromSkill != fromADR {
		messages = append(messages, fmt.Sprintf(
			"%s records the release-please App as ID %q, slug %q and the release-tag App as ID %q, slug %q, "+
				"but %s records them as ID %q, slug %q and ID %q, slug %q: the two must agree",
			releaseTagSkillName, fromSkill.releasePlease.id, fromSkill.releasePlease.slug, fromSkill.releaseTag.id, fromSkill.releaseTag.slug,
			releaseTagADRName, fromADR.releasePlease.id, fromADR.releasePlease.slug, fromADR.releaseTag.id, fromADR.releaseTag.slug))
	}

	return messages
}

// recordedAppProblems returns the problems of one document's record, name.
func recordedAppProblems(name string, apps recordedApps) []string {
	var messages []string

	if apps.releasePlease.id != releasePleaseAppID {
		messages = append(messages, fmt.Sprintf("%s records the release-please App's ID as %q, want %s", name, apps.releasePlease.id, releasePleaseAppID))
	}

	if apps.releasePlease.slug != releasePleaseSlug {
		messages = append(messages, fmt.Sprintf("%s records the release-please App's slug as %q, want %s", name, apps.releasePlease.slug, releasePleaseSlug))
	}

	tag := apps.releaseTag
	idPlaceholder, slugPlaceholder := tag.id == tagAppIDPlaceholder, tag.slug == tagAppSlugPlaceholder

	if idPlaceholder != slugPlaceholder {
		messages = append(messages, fmt.Sprintf("%s records the release-tag App's ID %q and slug %q: record both or neither, never one placeholder", name, tag.id, tag.slug))
	}

	if !idPlaceholder {
		switch {
		case !appIDPattern.MatchString(tag.id):
			messages = append(messages, fmt.Sprintf("%s records the release-tag App's ID as %q, want a number or %s", name, tag.id, tagAppIDPlaceholder))
		case tag.id == releasePleaseAppID:
			messages = append(messages, fmt.Sprintf("%s records the release-tag App's ID as %s, the release-please App's: the App that opens the release PR must not bypass the tag ruleset", name, tag.id))
		}
	}

	if !slugPlaceholder {
		switch {
		case !appSlugPattern.MatchString(tag.slug):
			messages = append(messages, fmt.Sprintf("%s records the release-tag App's slug as %q, want a slug or %s", name, tag.slug, tagAppSlugPlaceholder))
		case tag.slug == releasePleaseSlug:
			messages = append(messages, fmt.Sprintf("%s records the release-tag App's slug as %s, the release-please App's: the two rows must name two Apps", name, tag.slug))
		}
	}

	return messages
}

// skillAppIdentifiers reads the release-tag skill's table: one row per App,
// its ID in the second column and its slug in the third.
func skillAppIdentifiers(doc []byte) (recordedApps, []string) {
	var (
		apps     recordedApps
		messages []string
	)

	rows := map[string]*appIdentifiers{"release-please App": &apps.releasePlease, "release-tag App": &apps.releaseTag}

	for _, label := range []string{"release-please App", "release-tag App"} {
		cells, problem := tableRow(releaseTagSkillName, doc, label, 4)
		if problem != "" {
			messages = append(messages, problem)

			continue
		}

		rows[label].id, rows[label].slug = cells[1], cells[2]
	}

	return apps, messages
}

// adrAppIdentifiers reads ADR 0021's table: one row per identifier, the
// release-please App's in the second column and the release-tag App's in the
// third.
func adrAppIdentifiers(doc []byte) (recordedApps, []string) {
	var (
		apps     recordedApps
		messages []string
	)

	if cells, problem := tableRow(releaseTagADRName, doc, "App ID", 3); problem != "" {
		messages = append(messages, problem)
	} else {
		apps.releasePlease.id, apps.releaseTag.id = cells[1], cells[2]
	}

	if cells, problem := tableRow(releaseTagADRName, doc, "Slug (bot login)", 3); problem != "" {
		messages = append(messages, problem)
	} else {
		apps.releasePlease.slug, apps.releaseTag.slug = cells[1], cells[2]
	}

	return apps, messages
}

// tableRow finds the one Markdown table row of doc, name, whose first cell is
// label, and returns its cells, every one after the first reduced to its first
// backticked text; or a problem when there is not exactly one such row of
// width cells, or a cell after the first holds no backticked text.
func tableRow(name string, doc []byte, label string, width int) ([]string, string) {
	var found [][]string

	for line := range strings.SplitSeq(string(doc), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") || len(trimmed) < 2 {
			continue
		}

		cells := strings.Split(trimmed[1:len(trimmed)-1], "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}

		if cells[0] == label {
			found = append(found, cells)
		}
	}

	if len(found) != 1 {
		return nil, fmt.Sprintf("%s holds %d table rows labelled %q, want exactly 1 recording the release Apps' identifiers", name, len(found), label)
	}

	cells := found[0]
	if len(cells) != width {
		return nil, fmt.Sprintf("%s: the row %q holds %d cells, want %d", name, label, len(cells), width)
	}

	for i := 1; i < len(cells); i++ {
		m := backticked.FindStringSubmatch(cells[i])
		if m == nil {
			return nil, fmt.Sprintf("%s: the row %q holds %q, want the value in backticks", name, label, cells[i])
		}

		cells[i] = m[1]
	}

	return cells, ""
}

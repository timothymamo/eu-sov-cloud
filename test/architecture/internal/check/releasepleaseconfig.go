package check

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const releasePleaseConfigName = "release-please-config.json"

// releasePleaseSettings are the settings that decide the version and the tag,
// with the value each must have, at the top level and, when a package entry
// overrides one, there too:
//
//   - bump-minor-pre-major: a breaking change is minor on v0.x;
//   - bump-patch-for-minor-pre-major: a feat stays minor on v0.x;
//   - include-v-in-tag and include-component-in-tag: every tag is exactly
//     vX.Y.Z, which the Release tags rulesets, release.yml's signing identity
//     and the #99 verification job name;
//   - skip-github-release: GoReleaser creates the GitHub Release.
var releasePleaseSettings = map[string]bool{
	"bump-minor-pre-major":           true,
	"bump-patch-for-minor-pre-major": false,
	"include-v-in-tag":               true,
	"include-component-in-tag":       false,
	"skip-github-release":            true,
}

// releasePleaseFreeKeys are the other keys the configuration may hold, at the
// top level or in the package entry: they name the document's schema, the
// package and its strategy, and the changelog's sections. Any other key is
// refused, so that a setting that moves the version (versioning, release-as,
// prerelease, ...) or the tag (tag-separator, component, ...) is reviewed
// here first.
var releasePleaseFreeKeys = []string{"$schema", "release-type", "package-name", "changelog-sections", "changelog-path", "packages"}

// hiddenChangelogTypes are the Conventional Commit types that alone produce no
// release (the release-tag skill's rules): release-please opens no release PR
// when the changelog it would write is empty, so a changelog-sections entry of
// one of these types must be hidden.
var hiddenChangelogTypes = []string{"docs", "test", "ci", "chore", "build", "style", "refactor"}

// ReleasePleaseConfig returns one message per problem in src,
// release-please-config.json (issue #142), and none for a configuration that
// holds every setting of releasePleaseSettings at the top level with its
// value, no package entry that overrides one with another, no key outside
// those and releasePleaseFreeKeys, no visible changelog section for a type
// that alone produces no release, and exactly one package, at the repository
// root (".").
func ReleasePleaseConfig(src []byte) []string {
	name := releasePleaseConfigName

	if len(bytes.TrimSpace(src)) == 0 {
		return []string{name + ": the document is empty, so this check cannot tell how a version is computed"}
	}

	var top map[string]json.RawMessage

	if err := json.Unmarshal(src, &top); err != nil {
		return []string{fmt.Sprintf("%s: read the document: %v", name, err)}
	}

	var messages []string

	fail := func(format string, args ...any) {
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	for _, key := range sortedKeys(releasePleaseSettings) {
		raw, ok := top[key]
		if !ok {
			fail("%s has no %s; want %q: %t", name, key, key, releasePleaseSettings[key])

			continue
		}

		checkReleasePleaseSetting(fail, name, key, raw)
	}

	checkReleasePleaseKeys(fail, name, top)

	raw, ok := top["packages"]
	if !ok {
		fail("%s has no packages; want exactly one, \".\", the module at the repository root", name)

		return messages
	}

	var packages map[string]map[string]json.RawMessage

	if err := json.Unmarshal(raw, &packages); err != nil {
		fail("%s: read packages: %v", name, err)

		return messages
	}

	paths := sortedKeys(packages)
	if !slices.Equal(paths, []string{"."}) {
		fail("%s: packages holds %q; want exactly one, \".\", the module at the repository root", name, paths)
	}

	for _, path := range paths {
		entry := packages[path]
		where := fmt.Sprintf("%s: packages: %q", name, path)

		for _, key := range sortedKeys(entry) {
			if _, ok := releasePleaseSettings[key]; ok {
				checkReleasePleaseSetting(fail, where, key, entry[key])
			}
		}

		if _, ok := entry["packages"]; ok {
			fail("%s holds packages; a package entry holds no packages", where)
		}

		checkReleasePleaseKeys(fail, where, entry)
	}

	return messages
}

// checkReleasePleaseSetting reports key, at where, when raw is not the
// boolean releasePleaseSettings wants.
func checkReleasePleaseSetting(fail func(string, ...any), where, key string, raw json.RawMessage) {
	want := releasePleaseSettings[key]

	var got bool
	if err := json.Unmarshal(raw, &got); err != nil || got != want {
		fail("%s: %s is %s, want %t", where, key, strings.TrimSpace(string(raw)), want)
	}
}

// checkReleasePleaseKeys reports every key of m, at where, that is neither a
// setting nor a free key, and every visible changelog section for a type that
// alone produces no release.
func checkReleasePleaseKeys(fail func(string, ...any), where string, m map[string]json.RawMessage) {
	for _, key := range sortedKeys(m) {
		_, setting := releasePleaseSettings[key]
		if !setting && !slices.Contains(releasePleaseFreeKeys, key) {
			fail("%s holds %s, which this check does not read and which may change the version or the tag; want only %s and %s", where, key, strings.Join(sortedKeys(releasePleaseSettings), ", "), strings.Join(releasePleaseFreeKeys, ", "))
		}
	}

	raw, ok := m["changelog-sections"]
	if !ok {
		return
	}

	var sections []struct {
		Type   string `json:"type"`
		Hidden bool   `json:"hidden"`
	}

	if err := json.Unmarshal(raw, &sections); err != nil {
		fail("%s: read changelog-sections: %v", where, err)

		return
	}

	for _, s := range sections {
		if slices.Contains(hiddenChangelogTypes, s.Type) && !s.Hidden {
			fail("%s: changelog-sections shows %s, so a %s commit alone would open a release PR; want hidden: true", where, s.Type, s.Type)
		}
	}
}

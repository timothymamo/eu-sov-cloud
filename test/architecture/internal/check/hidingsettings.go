package check

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// architectureLinters are the linters the architecture rules depend on:
// depguard holds the dependency rule, forbidigo the cloud-read-only and
// domain-is-pure groups. HidingSettings refuses every setting that turns one of
// them off or keeps it from a file.
var architectureLinters = []string{"depguard", "forbidigo"}

// topKeys are the top-level keys golangci-lint v2 reads. A key not in the list
// is one a later version added, which may hide issues, so it fails closed.
var topKeys = []string{"version", "run", "linters", "formatters", "issues", "output", "severity"}

// runKeys are the run keys that hide no issue: tests and issues-exit-code,
// whose values the check reads, and keys that only bound how lint runs.
// build-tags is accepted unread, because test/lint pins its value.
// relative-path-mode is accepted unread too: it changes only the base that
// linters.exclusions.rules path regexes match against, and that base is the
// same directory while the config, the working directory and the git root
// are all the repository root, as they are for make lint and CI. depguard's
// files: globs match absolute paths, and the exclusion rules that could hide a
// depguard or forbidigo issue are refused or pinned whatever their path
// matches. A key not in the list fails closed.
var runKeys = []string{
	"tests", "build-tags", "issues-exit-code", "timeout", "concurrency", "go",
	"modules-download-mode", "allow-parallel-runners", "allow-serial-runners", "relative-path-mode",
}

// lintersKeys are the linters keys golangci-lint v2 reads. A key not in the
// list, a v1 one such as fast or enable-all among them, fails closed.
var lintersKeys = []string{"default", "enable", "disable", "settings", "exclusions"}

// lintersDefaults are the linters.default values that turn on at least the
// standard linters. none and fast leave some out, and a value the check does
// not know may, so every other value is refused.
var lintersDefaults = []string{"standard", "all"}

// issuesAllowed maps every issues key golangci-lint v2 reads to the values that
// report every issue lint finds, or to nil when no value does: the key must
// then be absent. A key not in the map fails closed.
//
// The two caps default to 50 (max-issues-per-linter) and 3 (max-same-issues)
// in golangci-lint v2, and 0 means unlimited. The check accepts them absent or
// 0: absent, a cap trims only the report of a run that fails anyway, since
// lint exits non-zero on its first issue.
var issuesAllowed = map[string][]string{
	"max-issues-per-linter": {"0"},
	"max-same-issues":       {"0"},
	"new":                   {"false"},
	"new-from-rev":          nil,
	"new-from-patch":        nil,
	"new-from-merge-base":   nil,
	"whole-files":           {"false"},
	"fix":                   {"false"},
	"uniq-by-line":          {"true", "false"},
}

// exclusionsKeys are the keys of linters.exclusions golangci-lint v2 reads. A
// key not in the list fails closed.
var exclusionsKeys = []string{"generated", "warn-unused", "presets", "rules", "paths", "paths-except"}

// generatedMissing is the state of a linters.exclusions.generated that is not
// there, for generatedMessage.
const generatedMissing = "is missing, which golangci-lint reads as lax"

// exclusionsPresets are the presets of linters.exclusions golangci-lint v2 has,
// each of which drops only the known false positives it is named for. A preset
// not in the list is one a later version added, which may drop more, so it
// fails closed.
var exclusionsPresets = []string{"comments", "std-error-handling", "common-false-positives", "legacy"}

// HidingSettings returns one message per setting of a .golangci.yml document
// that hides issues from make lint without touching a rule (issue #81), each
// naming the setting by its dotted key and, when the setting turns depguard or
// forbidigo off or keeps it from a file, that linter. It returns nil for a
// document that hides nothing.
//
// The shape of the document is an allow list, because a key the check does
// not read is one a later golangci-lint may act on: a top-level key other than
// version, run, linters, formatters, issues, output and severity, a run,
// linters, linters.exclusions or issues key the check does not know, and a
// linters.exclusions.presets entry other than comments, std-error-handling,
// common-false-positives and legacy are refused. version must be "2".
//
// Of the keys it knows, it refuses: run.tests set to anything but true;
// run.issues-exit-code outside 1..125, Go's portable os.Exit range, since
// os.Exit truncates the code to 8 bits and 0, 256 or -256 lets lint exit 0
// while it prints issues; linters.default set to anything but standard or all; depguard or
// forbidigo missing from linters.enable or listed in linters.disable; a
// linters.exclusions.generated that is missing or not disable (golangci-lint
// reads it as lax when it is absent, which drops every issue, depguard's and
// forbidigo's included, in a file with a "code generated" comment); a
// non-empty linters.exclusions.paths or paths-except; every issues key set to
// anything but a value that reports every issue (0 for the caps, false for
// new, whole-files and fix, either value for uniq-by-line), and any
// new-from-rev, new-from-patch or new-from-merge-base; and a
// linters.exclusions.rules entry that names depguard, names forbidigo without
// a text or with an empty one, names no linter (it then drops every linter's
// issues on its path) or holds a key it does not know.
//
// It fails closed: an error wherever the line reader refuses the document, and
// wherever a setting it reads is not the shape it expects.
func HidingSettings(golangci []byte) ([]string, error) {
	lines, err := splitLines(golangci)
	if err != nil {
		return nil, fmt.Errorf("read .golangci.yml: %w", err)
	}

	top, err := mapping(lines)
	if err != nil {
		return nil, fmt.Errorf("read .golangci.yml: %w", err)
	}

	var messages []string

	for _, read := range []func([]entry) ([]string, error){topSettings, runSettings, issuesSettings, lintersSettings} {
		found, err := read(top)
		if err != nil {
			return nil, err
		}

		messages = append(messages, found...)
	}

	return messages, nil
}

// child returns the entry of entries with key, if any.
func child(entries []entry, key string) (entry, bool) {
	i := slices.IndexFunc(entries, func(e entry) bool { return e.key == key })
	if i < 0 {
		return entry{}, false
	}

	return entries[i], true
}

// childMapping returns the block mapping below the entry of entries with key,
// with false when there is no such entry.
func childMapping(entries []entry, key, path string) ([]entry, bool, error) {
	e, ok := child(entries, key)
	if !ok {
		return nil, false, nil
	}

	block, err := blockValue(e, path)
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}

	m, err := mapping(block)
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}

	return m, true, nil
}

// unknownKeys returns a message for every entry whose key is not in known,
// naming it as prefix followed by the key.
func unknownKeys(entries []entry, known []string, prefix string) []string {
	var messages []string

	for _, e := range entries {
		if !slices.Contains(known, e.key) {
			messages = append(messages, fmt.Sprintf("line %d: %s%s is a setting this check does not know, so it may hide issues: remove it, or teach the check what it does", e.num, prefix, e.key))
		}
	}

	return messages
}

// topSettings refuses a top-level key the check does not know and a version
// that is missing or not "2".
func topSettings(top []entry) ([]string, error) {
	messages := unknownKeys(top, topKeys, "")

	e, ok := child(top, "version")
	if !ok {
		return append(messages, `version is missing, so the document may hold settings in a shape this check does not read: set it to "2"`), nil
	}

	value, err := single(e)
	if err != nil {
		return nil, fmt.Errorf("read version: %w", err)
	}

	if value != "2" {
		messages = append(messages, fmt.Sprintf(`line %d: version is %s, so the document may hold settings in a shape this check does not read: set it to "2"`, e.num, value))
	}

	return messages, nil
}

// runSettings refuses a run key the check does not know, a run.tests that is
// not true, which keeps every linter, depguard and forbidigo among them, from
// every _test.go file, and a run.issues-exit-code outside 1..125.
func runSettings(top []entry) ([]string, error) {
	run, ok, err := childMapping(top, "run", "run")
	if err != nil || !ok {
		return nil, err
	}

	messages := unknownKeys(run, runKeys, "run.")

	if e, ok := child(run, "tests"); ok {
		value, err := single(e)
		if err != nil {
			return nil, fmt.Errorf("read run.tests: %w", err)
		}

		if value != "true" {
			messages = append(messages, fmt.Sprintf("line %d: run.tests is %s, which keeps %s from every test file: remove it or set it to true", e.num, value, linterNames()))
		}
	}

	if e, ok := child(run, "issues-exit-code"); ok {
		value, err := single(e)
		if err != nil {
			return nil, fmt.Errorf("read run.issues-exit-code: %w", err)
		}

		if code, err := strconv.Atoi(value); err != nil || code < 1 || code > 125 {
			messages = append(messages, fmt.Sprintf("line %d: run.issues-exit-code is %s, outside Go's portable exit range 1..125, and a value that wraps to 0 lets lint exit 0 while it prints issues: remove it or set it to 1", e.num, value))
		}
	}

	return messages, nil
}

// issuesSettings refuses every issues key that is not set to a value that
// reports every issue, and every key it does not know.
func issuesSettings(top []entry) ([]string, error) {
	issues, ok, err := childMapping(top, "issues", "issues")
	if err != nil || !ok {
		return nil, err
	}

	var messages []string

	for _, e := range issues {
		allowed, known := issuesAllowed[e.key]
		if !known {
			messages = append(messages, unknownKeys([]entry{e}, nil, "issues.")...)

			continue
		}

		if allowed == nil {
			messages = append(messages, fmt.Sprintf("line %d: issues.%s reports only some of the issues lint finds: remove it", e.num, e.key))

			continue
		}

		value, err := single(e)
		if err != nil {
			return nil, fmt.Errorf("read issues.%s: %w", e.key, err)
		}

		if !slices.Contains(allowed, value) {
			messages = append(messages, fmt.Sprintf("line %d: issues.%s is %s, which reports only some of the issues lint finds: remove it or set it to %s", e.num, e.key, value, allowed[0]))
		}
	}

	return messages, nil
}

// lintersSettings refuses a linters key the check does not know, a
// linters.default other than standard or all, depguard or forbidigo missing
// from linters.enable or in linters.disable, and the exclusions that hide
// issues.
func lintersSettings(top []entry) ([]string, error) {
	lintersEntry, ok := child(top, "linters")
	if !ok {
		return []string{
			"linters.enable is missing, so " + linterNames() + " never run",
			"linters.exclusions.generated " + generatedMissing + generatedHides(),
		}, nil
	}

	linters, _, err := childMapping(top, "linters", "linters")
	if err != nil {
		return nil, err
	}

	messages := unknownKeys(linters, lintersKeys, "linters.")

	if e, ok := child(linters, "default"); ok {
		value, err := single(e)
		if err != nil {
			return nil, fmt.Errorf("read linters.default: %w", err)
		}

		if !slices.Contains(lintersDefaults, value) {
			messages = append(messages, fmt.Sprintf("line %d: linters.default is %s, which may leave out linters the standard set runs: use standard or all", e.num, value))
		}
	}

	enabled, err := optionalList(linters, "enable", "linters.enable")
	if err != nil {
		return nil, err
	}

	disabled, err := optionalList(linters, "disable", "linters.disable")
	if err != nil {
		return nil, err
	}

	enableEntry, hasEnable := child(linters, "enable")
	disableEntry, _ := child(linters, "disable")

	for _, linter := range architectureLinters {
		switch {
		case !hasEnable:
			messages = append(messages, fmt.Sprintf("line %d: linters.enable is missing, so %s never runs", lintersEntry.num, linter))
		case !slices.Contains(enabled, linter):
			messages = append(messages, fmt.Sprintf("line %d: linters.enable does not list %s, so %s never runs", enableEntry.num, linter, linter))
		}

		if slices.Contains(disabled, linter) {
			messages = append(messages, fmt.Sprintf("line %d: linters.disable lists %s, so %s never runs", disableEntry.num, linter, linter))
		}
	}

	exclusions, err := exclusionsSettings(lintersEntry.num, linters)
	if err != nil {
		return nil, err
	}

	return append(messages, exclusions...), nil
}

// optionalList reads the list at key of entries, nil when there is none.
func optionalList(entries []entry, key, path string) ([]string, error) {
	e, ok := child(entries, key)
	if !ok {
		return nil, nil
	}

	values, err := list(e)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return values, nil
}

// exclusionsSettings refuses a linters.exclusions.generated that is missing or
// not disable, a non-empty linters.exclusions.paths or paths-except, a preset
// or a key of linters.exclusions it does not know, and the rules that hide
// issues of depguard or forbidigo. lintersLine is the line of the linters key,
// for the message when linters.exclusions is missing.
func exclusionsSettings(lintersLine int, linters []entry) ([]string, error) {
	exclusionsEntry, ok := child(linters, "exclusions")
	if !ok {
		return []string{generatedMessage(lintersLine, generatedMissing)}, nil
	}

	exclusions, _, err := childMapping(linters, "exclusions", "linters.exclusions")
	if err != nil {
		return nil, err
	}

	messages := unknownKeys(exclusions, exclusionsKeys, "linters.exclusions.")

	if _, ok := child(exclusions, "generated"); !ok {
		messages = append(messages, generatedMessage(exclusionsEntry.num, generatedMissing))
	}

	for _, e := range exclusions {
		path := "linters.exclusions." + e.key

		switch e.key {
		case "generated":
			value, err := single(e)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}

			if value != "disable" {
				messages = append(messages, generatedMessage(e.num, "is "+value))
			}
		case "presets":
			values, err := list(e)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}

			for _, preset := range values {
				if !slices.Contains(exclusionsPresets, preset) {
					messages = append(messages, fmt.Sprintf("line %d: %s holds %s, a preset this check does not know, so it may hide issues: remove it", e.num, path, preset))
				}
			}
		case "paths", "paths-except":
			values, err := list(e)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}

			if len(values) != 0 {
				messages = append(messages, fmt.Sprintf("line %d: %s drops every issue of every linter, %s included, in the files it selects: exclude by rule, for named linters", e.num, path, linterNames()))
			}
		case "rules":
			found, err := exclusionRules(e)
			if err != nil {
				return nil, err
			}

			messages = append(messages, found...)
		}
	}

	return messages, nil
}

// generatedMessage refuses a linters.exclusions.generated on line num in
// state: missing, or set to a value other than disable.
func generatedMessage(num int, state string) string {
	return fmt.Sprintf("line %d: linters.exclusions.generated %s", num, state) + generatedHides()
}

// generatedHides says what a linters.exclusions.generated other than disable
// hides, for a message.
func generatedHides() string {
	return ": it drops every issue, " + linterNames() + "'s included, in a file with a \"code generated\" comment, which a hand-written file can carry too: set it to disable"
}

// exclusionRules refuses every linters.exclusions.rules entry that drops
// depguard's issues or every forbidigo issue on its path.
func exclusionRules(e entry) ([]string, error) {
	const path = "linters.exclusions.rules"

	// blockValue would refuse an empty block too, where an empty rules key is
	// an empty list and hides nothing, so only its first check is repeated.
	if e.value != "" {
		return nil, fmt.Errorf("line %d: %s has %q on its own line, want a block below it", e.num, path, e.value)
	}

	items, err := sequence(e.block)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var messages []string

	for _, it := range items {
		fields, err := mapping(it.lines)
		if err != nil {
			return nil, fmt.Errorf("line %d: read an entry of %s: %w", it.num, path, err)
		}

		found, err := exclusionRule(it.num, fields)
		if err != nil {
			return nil, err
		}

		messages = append(messages, found...)
	}

	return messages, nil
}

// exclusionRule refuses one linters.exclusions.rules entry, starting on line
// num, when it names depguard, names forbidigo without a text or with an empty
// one, names no linter or holds a key this check does not know. It refuses
// only a missing or empty text: test/lint (TestForbidigoExclusionsDropOneGroup)
// pins the exact form a forbidigo text must have.
func exclusionRule(num int, fields []entry) ([]string, error) {
	const path = "linters.exclusions.rules"

	var messages []string

	for _, f := range fields {
		switch f.key {
		case "path", "path-except", "text", "source", "linters":
		default:
			messages = append(messages, fmt.Sprintf("line %d: an entry of %s has key %s, which this check does not know, so it may hide issues of %s", f.num, path, f.key, linterNames()))
		}
	}

	linters, err := optionalList(fields, "linters", path+" linters")
	if err != nil {
		return nil, fmt.Errorf("line %d: %w", num, err)
	}

	text := ""

	if e, ok := child(fields, "text"); ok {
		// scalar reads a quoted empty text as "", the form refused below.
		text, err = single(e)
		if err != nil {
			return nil, fmt.Errorf("line %d: read the text of an entry of %s: %w", num, path, err)
		}
	}

	if len(linters) == 0 {
		return append(messages, fmt.Sprintf("line %d: an entry of %s names no linter, so it drops the issues of every linter, %s included: name the linters it is for", num, path, linterNames())), nil
	}

	if slices.Contains(linters, "depguard") {
		messages = append(messages, fmt.Sprintf("line %d: an entry of %s names depguard, which drops its issues in the files the entry selects: change the depguard rule instead", num, path))
	}

	if slices.Contains(linters, "forbidigo") && text == "" {
		messages = append(messages, fmt.Sprintf("line %d: an entry of %s names forbidigo with no text, or an empty one, which drops every forbidigo issue in the files the entry selects: scope it to one group with a text", num, path))
	}

	return messages, nil
}

// linterNames returns the architecture linters for a message.
func linterNames() string {
	return strings.Join(architectureLinters, " and ")
}

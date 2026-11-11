package check

import (
	"fmt"
	"slices"
	"strings"
)

// architectureLinters are the linters the architecture rules depend on:
// depguard holds the dependency rule, forbidigo the cloud-read-only and
// domain-is-pure groups. HidingSettings refuses every setting that turns one of
// them off or keeps it from a file.
var architectureLinters = []string{"depguard", "forbidigo"}

// issuesAllowed maps every issues key golangci-lint v2 reads to the only value
// that reports every issue lint finds, or to "" when no value does: the key
// must then be absent. A key not in the map fails closed.
var issuesAllowed = map[string]string{
	"max-issues-per-linter": "0",
	"max-same-issues":       "0",
	"new":                   "false",
	"new-from-rev":          "",
	"new-from-patch":        "",
	"new-from-merge-base":   "",
	"whole-files":           "false",
	"fix":                   "false",
	"uniq-by-line":          "false",
}

// exclusionsKeys are the keys of linters.exclusions golangci-lint v2 reads. A
// key not in the list fails closed.
var exclusionsKeys = []string{"generated", "warn-unused", "presets", "rules", "paths", "paths-except"}

// HidingSettings returns one message per setting of a .golangci.yml document
// that hides issues from make lint without touching a rule (issue #81), each
// naming the setting by its dotted key and, when the setting turns depguard or
// forbidigo off or keeps it from a file, that linter. It returns nil for a
// document that hides nothing.
//
// It refuses: a non-empty linters.exclusions.paths or paths-except, and a
// linters.exclusions key it does not know; every issues key set to anything but
// the value that reports every issue (0 for the caps, false for new,
// whole-files, fix and uniq-by-line), any new-from-rev, new-from-patch or
// new-from-merge-base, and an issues key it does not know; linters.default:
// none, or a default it does not know; run.tests set to anything but true;
// depguard or forbidigo missing from linters.enable or listed in
// linters.disable; and a linters.exclusions.rules entry that names depguard,
// names forbidigo without a text, names no linter (it then drops every
// linter's issues on its path) or holds a key it does not know.
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

	for _, read := range []func([]entry) ([]string, error){runSettings, issuesSettings, lintersSettings} {
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

// runSettings refuses a run.tests that is not true: false keeps every linter,
// depguard and forbidigo among them, from every _test.go file.
func runSettings(top []entry) ([]string, error) {
	run, ok, err := childMapping(top, "run", "run")
	if err != nil || !ok {
		return nil, err
	}

	e, ok := child(run, "tests")
	if !ok {
		return nil, nil
	}

	value, err := single(e)
	if err != nil {
		return nil, fmt.Errorf("read run.tests: %w", err)
	}

	if value == "true" {
		return nil, nil
	}

	return []string{fmt.Sprintf("line %d: run.tests is %s, which keeps %s from every test file: remove it or set it to true", e.num, value, linterNames())}, nil
}

// issuesSettings refuses every issues key that is not set to the value that
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
			messages = append(messages, fmt.Sprintf("line %d: issues.%s is a setting this check does not know, so it may hide issues: remove it, or teach the check what it does", e.num, e.key))

			continue
		}

		if allowed == "" {
			messages = append(messages, fmt.Sprintf("line %d: issues.%s reports only some of the issues lint finds: remove it", e.num, e.key))

			continue
		}

		value, err := single(e)
		if err != nil {
			return nil, fmt.Errorf("read issues.%s: %w", e.key, err)
		}

		if value != allowed {
			messages = append(messages, fmt.Sprintf("line %d: issues.%s is %s, which reports only some of the issues lint finds: remove it or set it to %s", e.num, e.key, value, allowed))
		}
	}

	return messages, nil
}

// lintersSettings refuses linters.default: none, depguard or forbidigo missing
// from linters.enable or in linters.disable, and the exclusions that hide
// issues.
func lintersSettings(top []entry) ([]string, error) {
	linters, ok, err := childMapping(top, "linters", "linters")
	if err != nil {
		return nil, err
	}

	if !ok {
		return []string{"linters.enable is missing, so " + linterNames() + " never run"}, nil
	}

	var messages []string

	if e, ok := child(linters, "default"); ok {
		value, err := single(e)
		if err != nil {
			return nil, fmt.Errorf("read linters.default: %w", err)
		}

		switch value {
		case "standard", "all", "fast":
		case "none":
			messages = append(messages, fmt.Sprintf("line %d: linters.default is none, which turns the standard linters off: use standard", e.num))
		default:
			messages = append(messages, fmt.Sprintf("line %d: linters.default is %s, a value this check does not know: use standard", e.num, value))
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

	for _, linter := range architectureLinters {
		if !slices.Contains(enabled, linter) {
			messages = append(messages, fmt.Sprintf("linters.enable does not list %s, so %s never runs", linter, linter))
		}

		if slices.Contains(disabled, linter) {
			messages = append(messages, fmt.Sprintf("linters.disable lists %s, so %s never runs", linter, linter))
		}
	}

	exclusions, err := exclusionsSettings(linters)
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

// exclusionsSettings refuses a non-empty linters.exclusions.paths or
// paths-except, a key of linters.exclusions it does not know, and the rules
// that hide issues of depguard or forbidigo.
func exclusionsSettings(linters []entry) ([]string, error) {
	exclusions, ok, err := childMapping(linters, "exclusions", "linters.exclusions")
	if err != nil || !ok {
		return nil, err
	}

	var messages []string

	for _, e := range exclusions {
		path := "linters.exclusions." + e.key

		switch {
		case !slices.Contains(exclusionsKeys, e.key):
			messages = append(messages, fmt.Sprintf("line %d: %s is a setting this check does not know, so it may hide issues: remove it, or teach the check what it does", e.num, path))
		case e.key == "paths" || e.key == "paths-except":
			values, err := list(e)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}

			if len(values) != 0 {
				messages = append(messages, fmt.Sprintf("line %d: %s drops every issue of every linter, %s included, in the files it selects: exclude by rule, for named linters", e.num, path, linterNames()))
			}
		case e.key == "rules":
			found, err := exclusionRules(e)
			if err != nil {
				return nil, err
			}

			messages = append(messages, found...)
		}
	}

	return messages, nil
}

// exclusionRules refuses every linters.exclusions.rules entry that drops
// depguard's issues or every forbidigo issue on its path.
func exclusionRules(e entry) ([]string, error) {
	const path = "linters.exclusions.rules"

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
// num, when it names depguard, names forbidigo without a text, names no linter
// or holds a key this check does not know.
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

	_, hasText := child(fields, "text")

	if len(linters) == 0 {
		return append(messages, fmt.Sprintf("line %d: an entry of %s names no linter, so it drops the issues of every linter, %s included: name the linters it is for", num, path, linterNames())), nil
	}

	if slices.Contains(linters, "depguard") {
		messages = append(messages, fmt.Sprintf("line %d: an entry of %s names depguard, which drops its issues in the files the entry selects: change the depguard rule instead", num, path))
	}

	if slices.Contains(linters, "forbidigo") && !hasText {
		messages = append(messages, fmt.Sprintf("line %d: an entry of %s names forbidigo with no text, which drops every forbidigo issue in the files the entry selects: scope it to one group with a text", num, path))
	}

	return messages, nil
}

// linterNames returns the architecture linters for a message.
func linterNames() string {
	return strings.Join(architectureLinters, " and ")
}

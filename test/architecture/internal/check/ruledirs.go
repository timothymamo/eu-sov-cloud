package check

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
)

// testFiles is the files: entry that takes test files out of a depguard rule.
const testFiles = "!$test"

// RuleFiles is what a depguard rule's files: list says about the packages it
// holds, in the shapes the package check can hold a package to.
type RuleFiles struct {
	// Dirs are the directories the rule holds, each written **/<dir>/**, in
	// file order, slash-separated and relative to the module root. Every
	// directory below one is held too.
	Dirs []string
	// Excludes are the directories the rule's negated globs take out of it,
	// each written !**/<dir>/**, in file order: a package in one, or below
	// one, is not held by the rule.
	Excludes []string
	// Tests is set when the list names no !$test, so the rule holds the
	// _test.go files of its packages too, integration-tagged ones included.
	Tests bool
}

// RuleDirs returns what the files: list of the depguard rule named rule in a
// .golangci.yml document holds: the entries under linters: settings: depguard:
// rules: <rule>: files:, read as RuleFiles.
//
// It reads only the shapes the package check can hold a package to: a
// directory written **/<dir>/**, a whole directory taken out written
// !**/<dir>/**, and !$test, which takes the test files out; without !$test
// the rule holds test files, whose imports the check reads from go list's
// TestImports and XTestImports. It fails closed on every other shape: an
// error when the rule or its files: list is missing, when the list holds no
// directory (an empty list, or one that only takes directories out), and for
// any other entry, such as $all, $test, a file glob, a negated file glob, a
// negated $all or a bare path, and for an exclusion that equals one of the
// rule's directories or lies under none of them. The document is held to the
// same characters AllowList holds it to.
//
// The directories are anchored at the module root, while depguard matches
// **/<dir>/** at any depth of a file's path: the two differ only for a path
// that repeats a directory below another, such as
// internal/adapter/target/x/internal/adapter/source/y.
func RuleDirs(golangci []byte, rule string) (RuleFiles, error) {
	lines, err := splitLines(golangci)
	if err != nil {
		return RuleFiles{}, fmt.Errorf("read .golangci.yml: %w", err)
	}

	body, err := walk(lines, "linters", "settings", "depguard", "rules", rule)
	if err != nil {
		return RuleFiles{}, fmt.Errorf("read depguard rule %s: %w", rule, err)
	}

	files, err := readFiles(body)
	if err != nil {
		return RuleFiles{}, fmt.Errorf("read depguard rule %s: %w", rule, err)
	}

	return files, nil
}

// readFiles reads a rule body's files: list.
func readFiles(body []line) (RuleFiles, error) {
	entries, err := mapping(body)
	if err != nil {
		return RuleFiles{}, err
	}

	var files *entry

	for i := range entries {
		if entries[i].key == "files" {
			files = &entries[i]
		}
	}

	if files == nil {
		return RuleFiles{}, errors.New("no files list")
	}

	values, err := list(*files)
	if err != nil {
		return RuleFiles{}, err
	}

	out := RuleFiles{Tests: true}

	for _, value := range values {
		if value == testFiles {
			out.Tests = false

			continue
		}

		if negated, ok := strings.CutPrefix(value, "!"); ok {
			dir, ok := globDir(negated)
			if !ok {
				return RuleFiles{}, fmt.Errorf("line %d: files entry %s is not %s or a directory taken out written !**/<dir>/**, the negated shapes the package check holds", files.num, value, testFiles)
			}

			out.Excludes = append(out.Excludes, dir)

			continue
		}

		dir, ok := globDir(value)
		if !ok {
			return RuleFiles{}, fmt.Errorf("line %d: files entry %s is not a directory written **/<dir>/**, the shape the package check holds", files.num, value)
		}

		out.Dirs = append(out.Dirs, dir)
	}

	if len(out.Dirs) == 0 {
		return RuleFiles{}, fmt.Errorf("line %d: the files list names no directory", files.num)
	}

	for _, ex := range out.Excludes {
		if slices.Contains(out.Dirs, ex) {
			return RuleFiles{}, fmt.Errorf("line %d: the files list takes out %s, which it also names as a directory, so it holds nothing there", files.num, ex)
		}

		if !slices.ContainsFunc(out.Dirs, func(dir string) bool { return strings.HasPrefix(ex, dir+"/") }) {
			return RuleFiles{}, fmt.Errorf("line %d: the files list takes out %s, which lies under none of its directories, so it narrows nothing", files.num, ex)
		}
	}

	return out, nil
}

// globDir returns dir for a files entry written **/<dir>/**, where dir is a
// clean relative path holding no glob character, and whether entry has that
// shape.
func globDir(entry string) (string, bool) {
	dir, ok := strings.CutPrefix(entry, "**/")
	if !ok {
		return "", false
	}

	dir, ok = strings.CutSuffix(dir, "/**")
	if !ok || dir == "" || path.Clean(dir) != dir || strings.HasPrefix(dir, "/") || strings.HasPrefix(dir, ".") {
		return "", false
	}

	if strings.ContainsAny(dir, `*?[]{}\!`) {
		return "", false
	}

	return dir, true
}

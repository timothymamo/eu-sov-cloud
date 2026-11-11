package check

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// testFiles is the files: entry that takes test files out of a depguard rule.
const testFiles = "!$test"

// RuleDirs returns the directories the depguard rule named rule in a
// .golangci.yml document holds: the entries under linters: settings: depguard:
// rules: <rule>: files:, each written "**/<dir>/**", in file order, as <dir>,
// slash-separated and relative to the module root.
//
// It reads only the one shape the package check can hold a directory to, and
// fails closed on every other: an error when the rule or its files: list is
// missing, when the list holds no directory, when it does not hold !$test (the
// check lists packages without their test files, so a rule that also holds
// test files says more than the check can see), and for any other entry, such
// as $all, a file glob, a negated directory or a bare path. The document is
// held to the same characters AllowList holds it to.
func RuleDirs(golangci []byte, rule string) ([]string, error) {
	lines, err := splitLines(golangci)
	if err != nil {
		return nil, fmt.Errorf("read .golangci.yml: %w", err)
	}

	body, err := walk(lines, "linters", "settings", "depguard", "rules", rule)
	if err != nil {
		return nil, fmt.Errorf("read depguard rule %s: %w", rule, err)
	}

	dirs, err := readFiles(body)
	if err != nil {
		return nil, fmt.Errorf("read depguard rule %s: %w", rule, err)
	}

	return dirs, nil
}

// readFiles returns the directories of a rule body's files: list.
func readFiles(body []line) ([]string, error) {
	entries, err := mapping(body)
	if err != nil {
		return nil, err
	}

	var files *entry

	for i := range entries {
		if entries[i].key == "files" {
			files = &entries[i]
		}
	}

	if files == nil {
		return nil, errors.New("no files list")
	}

	values, err := list(*files)
	if err != nil {
		return nil, err
	}

	var (
		dirs      []string
		skipsTest bool
	)

	for _, value := range values {
		if value == testFiles {
			skipsTest = true

			continue
		}

		dir, ok := globDir(value)
		if !ok {
			return nil, fmt.Errorf("line %d: files entry %s is not %s or a directory written **/<dir>/**, the shapes the package check holds", files.num, value, testFiles)
		}

		dirs = append(dirs, dir)
	}

	if !skipsTest {
		return nil, fmt.Errorf("line %d: the files list has no %s, so the rule holds test files, which the package check does not list", files.num, testFiles)
	}

	if len(dirs) == 0 {
		return nil, fmt.Errorf("line %d: the files list names no directory", files.num)
	}

	return dirs, nil
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

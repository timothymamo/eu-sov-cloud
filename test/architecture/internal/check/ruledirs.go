package check

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/timothymamo/shift2eu/test/internal/yamlread"
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
	// TestsOnly is set when the list names the test files of its
	// directories alone, each directory written as the pair
	// **/<dir>/**/*_test.go and **/<dir>/*_test.go (issue #144): the rule
	// holds the _test.go files of every package in or below the directory,
	// and none of their production files. Tests is set too.
	TestsOnly bool
}

// RuleDirs returns what the files: list of the depguard rule named rule in a
// .golangci.yml document holds: the entries under linters: settings: depguard:
// rules: <rule>: files:, read as RuleFiles.
//
// It reads only the shapes the package check can hold a package to: a
// directory written **/<dir>/**, a whole directory taken out written
// !**/<dir>/**, and !$test, which takes the test files out; without !$test
// the rule holds test files, whose imports the check reads from go list's
// TestImports and XTestImports. A list may instead name the test files of
// its directories alone (TestsOnly), each directory as the pair
// **/<dir>/**/*_test.go, the files below it, and **/<dir>/*_test.go, the
// files in it: a list of that shape holds no other entry, no directory and
// no exclusion, and names both globs of each directory, since either one
// alone holds only part of the directory's test files, which the check
// cannot model. It fails closed on every other shape: an
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
	lines, err := yamlread.SplitLines(golangci)
	if err != nil {
		return RuleFiles{}, fmt.Errorf("read .golangci.yml: %w", err)
	}

	body, err := yamlread.Walk(lines, "linters", "settings", "depguard", "rules", rule)
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
func readFiles(body []yamlread.Line) (RuleFiles, error) {
	entries, err := yamlread.Mapping(body)
	if err != nil {
		return RuleFiles{}, err
	}

	var files *yamlread.Entry

	for i := range entries {
		if entries[i].Key == "files" {
			files = &entries[i]
		}
	}

	if files == nil {
		return RuleFiles{}, errors.New("no files list")
	}

	values, err := yamlread.List(*files)
	if err != nil {
		return RuleFiles{}, err
	}

	out := RuleFiles{Tests: true}

	// below and in are the directories whose test files below and in them a
	// test-file glob names, in file order.
	var below, in []string

	for _, value := range values {
		if dir, ok := testGlobDir(value, "/**/*_test.go"); ok {
			below = append(below, dir)

			continue
		}

		if dir, ok := testGlobDir(value, "/*_test.go"); ok {
			in = append(in, dir)

			continue
		}

		if value == testFiles {
			out.Tests = false

			continue
		}

		if negated, ok := strings.CutPrefix(value, "!"); ok {
			dir, ok := globDir(negated)
			if !ok {
				return RuleFiles{}, fmt.Errorf("line %d: files entry %s is not %s or a directory taken out written !**/<dir>/**, the negated shapes the package check holds", files.Num, value, testFiles)
			}

			out.Excludes = append(out.Excludes, dir)

			continue
		}

		dir, ok := globDir(value)
		if !ok {
			return RuleFiles{}, fmt.Errorf("line %d: files entry %s is not a directory written **/<dir>/**, the shape the package check holds", files.Num, value)
		}

		out.Dirs = append(out.Dirs, dir)
	}

	if len(below) != 0 || len(in) != 0 {
		return testFilesOnly(files.Num, out, below, in)
	}

	if len(out.Dirs) == 0 {
		return RuleFiles{}, fmt.Errorf("line %d: the files list names no directory", files.Num)
	}

	for _, ex := range out.Excludes {
		if slices.Contains(out.Dirs, ex) {
			return RuleFiles{}, fmt.Errorf("line %d: the files list takes out %s, which it also names as a directory, so it holds nothing there", files.Num, ex)
		}

		if !slices.ContainsFunc(out.Dirs, func(dir string) bool { return strings.HasPrefix(ex, dir+"/") }) {
			return RuleFiles{}, fmt.Errorf("line %d: the files list takes out %s, which lies under none of its directories, so it narrows nothing", files.Num, ex)
		}
	}

	return out, nil
}

// testFilesOnly returns the RuleFiles of a files list that names test-file
// globs, read so far as out with the directories whose test files below them
// (below) and in them (in) the globs name. It refuses a list that also names
// a directory, an exclusion or !$test, and a directory that does not have
// both globs, or has one twice.
func testFilesOnly(line int, out RuleFiles, below, in []string) (RuleFiles, error) {
	switch {
	case len(out.Dirs) != 0:
		return RuleFiles{}, fmt.Errorf("line %d: the files list names test-file globs and the directory %s, which would hold its production files too: hold the test files of a directory alone", line, out.Dirs[0])
	case len(out.Excludes) != 0:
		return RuleFiles{}, fmt.Errorf("line %d: the files list names test-file globs and takes out %s, a shape the package check does not hold", line, out.Excludes[0])
	case !out.Tests:
		return RuleFiles{}, fmt.Errorf("line %d: the files list names test-file globs and %s, so it holds no file", line, testFiles)
	}

	for _, dirs := range [][]string{below, in} {
		seen := make(map[string]bool, len(dirs))

		for _, dir := range dirs {
			if seen[dir] {
				return RuleFiles{}, fmt.Errorf("line %d: the files list names a test-file glob of %s twice", line, dir)
			}

			seen[dir] = true
		}
	}

	for _, dir := range below {
		if !slices.Contains(in, dir) {
			return RuleFiles{}, fmt.Errorf("line %d: the files list names **/%s/**/*_test.go without **/%s/*_test.go, so it leaves out the test files directly in %s", line, dir, dir, dir)
		}
	}

	for _, dir := range in {
		if !slices.Contains(below, dir) {
			return RuleFiles{}, fmt.Errorf("line %d: the files list names **/%s/*_test.go without **/%s/**/*_test.go, so it leaves out the test files below %s", line, dir, dir, dir)
		}
	}

	return RuleFiles{Dirs: below, Tests: true, TestsOnly: true}, nil
}

// testGlobDir returns dir for a files entry written **/<dir><suffix>, where
// suffix is /**/*_test.go or /*_test.go and dir is a directory as globDir
// reads one, and whether entry has that shape.
func testGlobDir(entry, suffix string) (string, bool) {
	dir, ok := strings.CutSuffix(entry, suffix)
	if !ok {
		return "", false
	}

	return globDir(dir + "/**")
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

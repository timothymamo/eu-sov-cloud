package lint_test

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/timothymamo/shift2eu/test/internal/yamlread"
)

// docTrees reads the files: list of the depguard rule test-docs-are-pure from
// doc, a whole .golangci.yml, with the shared line reader of
// test/internal/yamlread, and returns the directories it holds and the ones it
// takes out, in the order the rule writes them. It reads only the shapes the
// rule writes: a held directory as **/<dir>/**, one taken out as
// !**/<dir>/**, and !$test, which takes the _test.go files out and names no
// directory. A glob **/<dir>/** is read as <dir> at the module root, though
// depguard matches it at any depth, so a nested x/test/e2e is not covered;
// none exists today. Any other entry, a rule .golangci.yml does not hold, a rule with
// no files: list and a list that holds no directory are errors, so a list
// written another way is never read as no tree at all (issue #194).
func docTrees(doc []byte) (held, takenOut []string, err error) {
	lines, err := yamlread.SplitLines(doc)
	if err != nil {
		return nil, nil, err
	}

	block, err := yamlread.Walk(lines, "linters", "settings", "depguard", "rules", docRule)
	if err != nil {
		return nil, nil, err
	}

	entries, err := yamlread.Mapping(block)
	if err != nil {
		return nil, nil, fmt.Errorf("depguard rule %s: %w", docRule, err)
	}

	i := slices.IndexFunc(entries, func(e yamlread.Entry) bool { return e.Key == "files" })
	if i < 0 {
		return nil, nil, fmt.Errorf("depguard rule %s has no files: list", docRule)
	}

	files, err := yamlread.List(entries[i])
	if err != nil {
		return nil, nil, fmt.Errorf("depguard rule %s: %w", docRule, err)
	}

	for _, f := range files {
		if f == "!$test" {
			continue
		}

		negated := strings.HasPrefix(f, "!")
		dir, ok := globDir(strings.TrimPrefix(f, "!"))
		if !ok {
			return nil, nil, fmt.Errorf("depguard rule %s: files entry %q is neither **/<dir>/**, "+
				"!**/<dir>/** nor !$test", docRule, f)
		}

		if negated {
			takenOut = append(takenOut, dir)
		} else {
			held = append(held, dir)
		}
	}

	if len(held) == 0 {
		return nil, nil, errors.New("depguard rule " + docRule + " holds no directory")
	}

	return held, takenOut, nil
}

// globDir returns dir for a glob **/<dir>/**, where dir is a clean relative
// path with no glob character, and false for any other glob.
func globDir(glob string) (string, bool) {
	dir, ok := strings.CutPrefix(glob, "**/")
	if !ok {
		return "", false
	}

	dir, ok = strings.CutSuffix(dir, "/**")
	if !ok || dir == "" || strings.ContainsAny(dir, `*?[]{}!\`) || path.Clean(dir) != dir ||
		strings.HasPrefix(dir, "/") || dir == ".." || strings.HasPrefix(dir, "../") {
		return "", false
	}

	return dir, true
}

// docPath is one entry of the walk docEntries performs.
type docPath struct {
	rel     string // slash-separated path relative to the walked root
	symlink bool   // fs.DirEntry.Type()&fs.ModeSymlink != 0
}

// docSourceProblems returns the problems of entries, in their order, that lie
// under a tree of held and under no tree of takenOut. A tree holds a path by
// whole segments, so test/e2e2 is not under test/e2e.
//
// A symlink there is refused (docSymlinkLine) whatever its name, and under a
// testdata directory too: the walk reads the link, not what it points at, yet
// an import of its path follows it and the toolchain builds the source it finds
// there. Any other entry is refused (docSourceLine) when it is a non-Go source
// (nonGoSourceExtensions) under no testdata directory. A non-Go file under
// testdata is no source of the held package: the go command leaves testdata, _
// and . directories out of ./..., and a package there is built only when it is
// imported by path, which this check does not cover (a follow-up to #194).
func docSourceProblems(entries []docPath, held, takenOut []string) []string {
	var problems []string

	for _, e := range entries {
		if slices.ContainsFunc(takenOut, func(dir string) bool { return underDir(e.rel, dir) }) {
			continue
		}

		i := slices.IndexFunc(held, func(dir string) bool { return underDir(e.rel, dir) })
		if i < 0 {
			continue
		}

		if e.symlink {
			problems = append(problems, docSymlinkLine(e.rel, held[i]))

			continue
		}

		if !slices.Contains(nonGoSourceExtensions, path.Ext(e.rel)) {
			continue
		}

		if slices.Contains(strings.Split(path.Dir(e.rel), "/"), "testdata") {
			continue
		}

		problems = append(problems, docSourceLine(e.rel, held[i]))
	}

	return problems
}

// underDir reports whether rel lies under dir, by whole path segments.
func underDir(rel, dir string) bool {
	return strings.HasPrefix(rel, dir+"/")
}

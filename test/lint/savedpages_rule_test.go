package lint_test

import (
	"fmt"
	"path"
	"strings"
)

// allowedSavedPages names the .html and .htm files git may track, by their
// slash-separated path relative to the module root. Today git ls-files lists
// none, so it is empty. A saved price page never belongs here: it is read from
// a scratch directory outside the repository (issue #212). Add an entry only
// for an HTML file the project itself writes and needs in git, such as a
// fixture a renderer test reads, with a comment on the entry saying why it is
// no saved page.
var allowedSavedPages = map[string]bool{}

// savedPageProblems returns one problem per path whose final extension is .html
// or .htm, in any case, and that allowed does not name, in input order. paths
// are slash-separated and relative to the module root.
func savedPageProblems(paths []string, allowed map[string]bool) []string {
	var problems []string
	for _, p := range paths {
		switch strings.ToLower(path.Ext(p)) {
		case ".html", ".htm":
		default:
			continue
		}
		if allowed[p] {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"%s: git tracks an HTML page; a saved price page is read from a scratch directory "+
				"outside the repository and never committed (internal/adapter/target/upcloud/SOURCES.md, "+
				"issue #212): remove it from git, or add it to allowedSavedPages in "+
				"test/lint/savedpages_rule_test.go with the reason it is no saved page", p))
	}
	return problems
}

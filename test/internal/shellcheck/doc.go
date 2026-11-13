// Package shellcheck holds the shellcheck directive rule of issue #164 with the
// standard library only: CheckDirectives refuses a shellcheck disable directive
// that names no SC code, or names all, or gives no reason after a second "#" on
// the same line, and IsShellScript picks the tracked files that are shell
// scripts, by a .sh name or a sh or bash shebang. test/lint runs the rule over
// every tracked shell script and workflow, and test/architecture holds make
// lint's shellcheck run to the scripts IsShellScript picks.
//
// It lived in test/lint until issue #177 moved it here: test/lint holds doc.go
// and _test.go files only, under the strict test-docs-are-pure depguard rule,
// while Go's internal rule keeps this package out of reach of cmd/ and
// internal/.
package shellcheck

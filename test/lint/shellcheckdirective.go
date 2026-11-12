package lint

import (
	"bytes"
	"fmt"
	"path"
	"strings"
)

// CheckShellcheckDirectives returns what in src, the content of the file
// at path, breaks the rule that a shellcheck disable directive names the
// codes it silences and gives its reason, one line each and in line
// order, each of the form "path:line: <why>"; nil when nothing does.
//
// A directive is a comment whose text, after its "#" and the spaces that
// follow it, starts with the word shellcheck, as ShellCheck reads it. Only its
// disable field is checked: every code must be SC and four digits, and the
// reason is the text after the next "#" on the same line.
func CheckShellcheckDirectives(path string, src []byte) []string {
	var problems []string
	for i, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if why := checkDirectiveLine(line); why != "" {
			problems = append(problems, fmt.Sprintf("%s:%d: %s", path, i+1, why))
		}
	}
	return problems
}

// checkDirectiveLine returns why the shellcheck directive on line breaks the
// rule, or "" when line holds no directive or a good one.
func checkDirectiveLine(line string) string {
	comment, ok := commentText(line)
	if !ok {
		return ""
	}
	directive, reason, hasReason := strings.Cut(comment, "#")
	fields := strings.Fields(directive)
	if len(fields) == 0 || fields[0] != "shellcheck" {
		return ""
	}
	return checkDirective(fields[1:], strings.TrimSpace(reason), hasReason)
}

// commentText returns the text after the "#" that starts the comment on line,
// the first "#" at the start of the line or after a blank, as the shell reads
// it (so the "#" of $# starts none), and whether line has a comment.
func commentText(line string) (string, bool) {
	for i := 0; i < len(line); i++ {
		if line[i] == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return line[i+1:], true
		}
	}
	return "", false
}

// checkDirective returns why a directive with the key=value fields, and the
// reason after its second "#", breaks the rule, or "" when it does not.
func checkDirective(fields []string, reason string, hasReason bool) string {
	for _, field := range fields {
		value, ok := strings.CutPrefix(field, "disable=")
		if !ok {
			continue
		}
		for code := range strings.SplitSeq(value, ",") {
			if !isShellcheckCode(code) {
				return fmt.Sprintf("shellcheck disable=%s names %q, want codes of the form SC1234, never all or none", value, code)
			}
		}
		if !hasReason || reason == "" {
			return fmt.Sprintf("shellcheck disable=%s gives no reason, want \"# shellcheck disable=%s # why\" on one line", value, value)
		}
	}
	return ""
}

// isShellcheckCode reports whether code is exactly SC and four digits.
func isShellcheckCode(code string) bool {
	digits, ok := strings.CutPrefix(code, "SC")
	if !ok || len(digits) != 4 {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// IsShellScript reports whether the tracked file at name, with content
// src, is a shell script: its name ends in .sh, or its first line is a
// #! shebang that runs sh or bash, directly or through env, whose
// options and VAR=value assignments are skipped.
func IsShellScript(name string, src []byte) bool {
	if strings.HasSuffix(name, ".sh") {
		return true
	}
	first, _, _ := bytes.Cut(src, []byte("\n"))
	rest, ok := strings.CutPrefix(strings.TrimSuffix(string(first), "\r"), "#!")
	if !ok {
		return false
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return false
	}
	interpreter := path.Base(fields[0])
	if interpreter != "env" {
		return isShell(interpreter)
	}
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "-") || strings.Contains(field, "=") {
			continue
		}
		return isShell(path.Base(field))
	}
	return false
}

// isShell reports whether name is a shell this rule covers.
func isShell(name string) bool {
	return name == "sh" || name == "bash"
}

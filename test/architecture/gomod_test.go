package architecture_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// replaceDirective is a replace directive a go.mod file holds.
type replaceDirective struct {
	// Line is the line of go.mod the directive is on, counted from 1.
	Line int
	// Text is the directive without its comment. A replace block's entries
	// are reported one by one, as the directives they are.
	Text string
}

// TestRootModuleHasNoReplace pins the auditor's M1 finding: a replace directive
// in the root go.mod lets a fork or a local copy stand in for an import the
// depguard rules allow by prefix, and neither depguard nor the third-party
// exemption of the package check sees the difference, so the release would
// carry code no rule vetted. The module holds none today, and none may be
// added: a fork belongs in an allow entry the rule names, reviewed with it.
func TestRootModuleHasNoReplace(t *testing.T) {
	t.Parallel()

	goMod := readRepositoryFile(t, "go.mod")

	for _, directive := range replaceDirectives(goMod) {
		t.Errorf("go.mod:%d: %s: a replace directive puts code no rule vetted where an allow entry allows an import by prefix",
			directive.Line, directive.Text)
	}
}

// TestReplaceDirectives pins the parser TestRootModuleHasNoReplace trusts, over
// go.mod text written here: a single-line replace directive, a block of them,
// and the three lines that are none - a comment, a word a comment holds, and a
// module path that merely holds the word.
func TestReplaceDirectives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		goMod string
		want  []replaceDirective
	}{
		{
			name:  "a go.mod holding no replace directive holds none",
			goMod: "module example.com/m\n\ngo 1.27.4\n\nrequire (\n\texample.com/lib v1.0.0\n)\n",
		},
		{
			name:  "a single-line replace directive is found with its line",
			goMod: "module example.com/m\n\nreplace example.com/lib => example.com/fork v1.2.3\n",
			want:  []replaceDirective{{Line: 3, Text: "replace example.com/lib => example.com/fork v1.2.3"}},
		},
		{
			name:  "a replace directive's trailing comment is not part of it",
			goMod: "module example.com/m\n\nreplace example.com/lib => ./vendor/lib // pinned by hand\n",
			want:  []replaceDirective{{Line: 3, Text: "replace example.com/lib => ./vendor/lib"}},
		},
		{
			name: "a replace block's directives are found one by one",
			goMod: "module example.com/m\n\nreplace (\n\texample.com/lib => example.com/fork v1.2.3\n" +
				"\texample.com/other => ../other\n)\n",
			want: []replaceDirective{
				{Line: 4, Text: "example.com/lib => example.com/fork v1.2.3"},
				{Line: 5, Text: "example.com/other => ../other"},
			},
		},
		{
			name: "a replace block's comment is no directive and does not open a second block",
			goMod: "module example.com/m\n\nreplace (\n\t// nothing is pinned by hand yet\n)\n\n" +
				"require example.com/lib v1.0.0\n",
		},
		{
			name: "a replace in a comment is no directive",
			goMod: "module example.com/m\n\n// replace example.com/lib => example.com/fork v1.2.3\n" +
				"//replace (\nrequire example.com/lib v1.0.0 // replace is only mentioned here\n",
		},
		{
			name:  "a module path that merely holds the word is no directive",
			goMod: "module example.com/m\n\nrequire example.com/replace v1.0.0\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(tc.want, replaceDirectives([]byte(tc.goMod))); diff != "" {
				t.Errorf("replaceDirectives(-want +got):\n%s", diff)
			}
		})
	}
}

// replaceDirectives returns the replace directives goMod holds: a single-line
// one, and every directive of a replace (...) block. It reads go.mod's syntax
// closely enough for a guard that fails on any replace directive: the first
// word of a line names the directive, so require example.com/replace v1.0.0 is
// no directive; // starts a comment, which runs to the end of its line, so
// neither the line one starts nor the rest of a line one trails is read; and a
// block ends at the line holding its closing parenthesis. A block's entries are
// reported as the directives they are.
//
// Two forms it does not handle, neither of which a go command writes: a //
// inside a quoted module path, which no real path holds, and a block whose
// closing parenthesis shares a line with the last entry.
func replaceDirectives(goMod []byte) []replaceDirective {
	var (
		found   []replaceDirective
		inBlock bool
	)

	for i, raw := range strings.Split(string(goMod), "\n") {
		line := strings.TrimSpace(commentless(raw))
		if line == "" {
			continue
		}

		switch {
		case inBlock && line == ")":
			inBlock = false
		case inBlock:
			found = append(found, replaceDirective{Line: i + 1, Text: line})
		case !isReplaceDirective(line):
		default:
			if rest := strings.TrimSpace(line[len("replace"):]); rest == "(" {
				inBlock = true

				continue
			}

			found = append(found, replaceDirective{Line: i + 1, Text: line})
		}
	}

	return found
}

// isReplaceDirective reports whether line, trimmed and without its comment,
// starts with the replace directive: the word alone, or the word followed by
// the space, tab or parenthesis that separates it from what it holds. A word
// the word only starts, as replacement, is no directive.
func isReplaceDirective(line string) bool {
	rest, ok := strings.CutPrefix(line, "replace")
	if !ok {
		return false
	}

	if rest == "" {
		return true
	}

	return rest[0] == ' ' || rest[0] == '\t' || rest[0] == '('
}

// commentless returns line without the comment it trails, and empty for a line
// that is nothing but a comment.
func commentless(line string) string {
	text, _, _ := strings.Cut(line, "//")

	return text
}

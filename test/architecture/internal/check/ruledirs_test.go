package check_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// filesDoc writes a .golangci.yml holding one strict rule named r whose files:
// list is files, with comment as a comment line at the head of the rule: the
// character cases put the character where today's reader skips it.
func filesDoc(comment string, files ...string) string {
	var b strings.Builder

	b.WriteString("        r:\n          list-mode: strict\n")

	if comment != "" {
		b.WriteString("          " + comment + "\n")
	}

	b.WriteString("          files:\n")

	for _, file := range files {
		b.WriteString("            - " + strconv.Quote(file) + "\n")
	}

	b.WriteString("          allow:\n            - errors$\n")

	return golangciDoc(b.String())
}

// TestRuleDirs pins the directories the check reads back from a depguard rule's
// files: list: the ones it holds to the rule, in file order, and an error for
// every shape it cannot read a directory from. A rule must also name the test
// files away with !$test, since the check lists packages without them.
func TestRuleDirs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// golangci is the .golangci.yml to read; empty means filesDoc of files.
		golangci string
		// files are the entries of the rule's files: list when golangci is
		// empty.
		files []string
		rule  string
		want  []string
		// wantErr is set for every document the check must refuse.
		wantErr bool
		// wantErrFragment is the entry a refusal message must name.
		wantErrFragment string
	}{
		{
			name:  "one directory comes back without its globs",
			files: []string{"**/internal/domain/**", "!$test"},
			want:  []string{"internal/domain"},
		},
		{
			// The shape embedded-snapshot-targets-stay-offline has: every
			// target that embeds a snapshot is listed by its own entry.
			name: "two directories come back in file order",
			files: []string{
				"**/internal/adapter/target/ovhcloud/**",
				"**/internal/adapter/target/scaleway/**",
				"!$test",
			},
			want: []string{"internal/adapter/target/ovhcloud", "internal/adapter/target/scaleway"},
		},
		{
			// $all names every file of the module, not a directory the check
			// could hold packages to.
			name:            "$all is refused",
			files:           []string{"$all", "!$test"},
			wantErr:         true,
			wantErrFragment: "$all",
		},
		{
			name:            "a file glob is refused",
			files:           []string{"**/internal/domain/*.go", "!$test"},
			wantErr:         true,
			wantErrFragment: "**/internal/domain/*.go",
		},
		{
			name:            "a negated directory is refused",
			files:           []string{"!**/internal/domain/**", "!$test"},
			wantErr:         true,
			wantErrFragment: "!**/internal/domain/**",
		},
		{
			name:            "a bare path is refused",
			files:           []string{"internal/domain", "!$test"},
			wantErr:         true,
			wantErrFragment: "internal/domain",
		},
		{
			// The check lists packages without their test files, so a rule
			// that also covers test files is not one it can hold.
			name:    "a rule without !$test is an error",
			files:   []string{"**/internal/domain/**"},
			wantErr: true,
		},
		{
			name:    "a rule that names no directory is an error",
			files:   []string{"!$test"},
			wantErr: true,
		},
		{
			name:    "a rule that is not there is an error",
			files:   []string{"**/internal/domain/**", "!$test"},
			rule:    "no-such-rule",
			wantErr: true,
		},
		{
			// The character rules are the same as the ones AllowList holds the
			// document to.
			name:            "a character the reader refuses is an error",
			golangci:        filesDoc("# a lone\rCR", "**/internal/domain/**", "!$test"),
			wantErr:         true,
			wantErrFragment: "U+000D",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := tc.golangci
			if doc == "" {
				doc = filesDoc("", tc.files...)
			}

			rule := tc.rule
			if rule == "" {
				rule = "r"
			}

			got, err := check.RuleDirs([]byte(doc), rule)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("RuleDirs(%q) = %v, nil; want an error", rule, got)
				}

				if tc.wantErrFragment != "" && !strings.Contains(err.Error(), tc.wantErrFragment) {
					t.Errorf("RuleDirs(%q) error does not name %q: %v", rule, tc.wantErrFragment, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("RuleDirs(%q): %v", rule, err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("RuleDirs(%q) mismatch (-want +got):\n%s", rule, diff)
			}
		})
	}
}

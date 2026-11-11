package check_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// strictRulesYAML holds the list-modes StrictRules sorts the rules by: two
// strict rules with a lax one between them, and rules whose list-mode says
// nothing about what depguard allows.
const strictRulesYAML = `version: "2"

linters:
  settings:
    depguard:
      rules:
        domain-is-pure:
          list-mode: strict
          files:
            - "**/internal/domain/**"
          allow:
            - errors$
        app-tests-use-fakes-not-adapters:
          list-mode: lax
          files:
            - "**/internal/app/**/*_test.go"
          allow:
            - errors$
        jsondoc-parses-bytes:
          list-mode: strict
          allow:
            - bytes$
`

// TestStrictRules pins the rules the package check must know about: every rule
// whose list-mode is strict, in file order, since in any other mode depguard
// allows what no list names.
func TestStrictRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		golangci string
		want     []string
		wantErr  bool
	}{
		{
			name:     "the strict rules come back in file order, without the lax one",
			golangci: strictRulesYAML,
			want:     []string{"domain-is-pure", "jsondoc-parses-bytes"},
		},
		{
			name:     "a rule without a list-mode is not strict",
			golangci: golangciDoc("        domain-is-pure:\n          list-mode: strict\n          allow:\n            - errors$\n        no-mode:\n          allow:\n            - os$\n"),
			want:     []string{"domain-is-pure"},
		},
		{
			name:     "no depguard section is an error",
			golangci: "version: \"2\"\n\nlinters:\n  settings:\n    gosec:\n      excludes:\n        - G104\n",
			wantErr:  true,
		},
		{
			name:     "a character the reader refuses is an error",
			golangci: "# a lone\rCR\n" + strictRulesYAML,
			wantErr:  true,
		},
		{
			// golangci-lint reads its configuration keys without regard to
			// case, so Strict may well be strict to depguard; the check cannot
			// tell, so it refuses the mode rather than count it as not strict.
			name:     "a list-mode in the wrong case is an error",
			golangci: golangciDoc("        domain-is-pure:\n          list-mode: Strict\n          allow:\n            - errors$\n"),
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := check.StrictRules([]byte(tc.golangci))

			if tc.wantErr {
				if err == nil {
					t.Fatalf("StrictRules() = %v, nil; want an error", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("StrictRules(): %v", err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("StrictRules() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

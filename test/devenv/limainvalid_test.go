package devenv_test

import (
	"flag"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var update = flag.Bool("update", false, "write dev/testdata/lima-invalid from dev/lima.yaml and the cases instead of comparing")

const (
	limaTemplate   = "../../dev/lima.yaml"
	invalidLimaDir = "../../dev/testdata/lima-invalid"
)

// limaBreakage is one broken template: dev/lima.yaml with the one line old
// replaced by new. old must occur exactly once in dev/lima.yaml.
type limaBreakage struct {
	name string // the template's file name in dev/testdata/lima-invalid, without .yaml
	why  string // what Lima refuses, for the reader
	// refusal is the substring of the message limactl validate must print for
	// this template: the job requires a refusal, this pins which one, so a
	// template refused for an unintended reason fails here rather than passing
	// for the wrong reason. One line; pinned Lima is the source of the wording.
	refusal string
	old     string
	new     string
}

// limaBreakages are the templates the validate-template job must see Lima
// refuse. Add a case here, run this test with -update, and the job picks the
// new template up by itself.
var limaBreakages = []limaBreakage{
	{
		name:    "param-read-indirectly",
		why:     "#120: param ghSha256Amd64 is read only through ${!name}, so no script names it",
		refusal: "key `ghSha256Amd64` is not used in any provision",
		old:     `            "${PARAM_ghSha256Amd64}" "${PARAM_ghSha256Arm64}"`,
		new:     `            "$(name=PARAM_ghSha256; name+=Amd64; echo "${!name}")" "${PARAM_ghSha256Arm64}"`,
	},
	{
		name:    "unknown-field",
		why:     "containerd has no field systemwide: Lima only warns (Non-strict YAML), which the job counts as a refusal",
		refusal: `unknown field "systemwide"`,
		old:     `  system: false`,
		new:     `  systemwide: false`,
	},
	{
		name: "mistyped-value",
		// Anchored on containerd's user flag, not on cpus: a CPU count is something people tune in
		// dev/lima.yaml, and re-anchoring the case with every such change is noise. maybe is no
		// boolean in any YAML version (unlike yes/no/on/off), so Lima cannot unmarshal it as a bool.
		why:     "containerd's user is not a boolean",
		refusal: "of type bool",
		old:     `  user: false`,
		new:     `  user: maybe`,
	},
}

func TestInvalidLimaTemplatesDifferOnlyByTheirBreakage(t *testing.T) {
	t.Parallel()

	valid, err := os.ReadFile(limaTemplate)
	if err != nil {
		t.Fatalf("read %s: %v", limaTemplate, err)
	}

	lines := strings.Split(string(valid), "\n")

	for _, tc := range limaBreakages {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if strings.Contains(tc.refusal, "\n") || tc.refusal == "" {
				t.Fatalf("case %s must name one line of Lima's refusal, not %q", tc.name, tc.refusal)
			}

			if strings.Contains(tc.old, "\n") || strings.Contains(tc.new, "\n") || tc.old == tc.new {
				t.Fatalf("case %s must replace one line with a different line (%s)", tc.name, tc.why)
			}

			at := -1
			for i, line := range lines {
				if line != tc.old {
					continue
				}

				if at >= 0 {
					t.Fatalf("%s has the line %q more than once, so case %s does not name one breakage", limaTemplate, tc.old, tc.name)
				}

				at = i
			}

			if at < 0 {
				t.Fatalf("%s has no line %q: update case %s to the current template", limaTemplate, tc.old, tc.name)
			}

			broken := slices.Clone(lines)
			broken[at] = tc.new
			want := strings.Join(broken, "\n")
			name := tc.name + ".yaml"
			path := invalidLimaDir + "/" + name
			// A .expect holds the case's refusal substring and one trailing newline, so the wording
			// pinned against the Lima in CI is readable in the repository, beside the template.
			expectName := tc.name + ".expect"
			expectPath := invalidLimaDir + "/" + expectName
			wantExpect := tc.refusal + "\n"

			if *update {
				if err := os.MkdirAll(invalidLimaDir, 0o750); err != nil {
					t.Fatalf("create %s: %v", invalidLimaDir, err)
				}
			}

			// Through an os.Root, so a case name cannot reach a file outside the directory.
			dir, err := os.OpenRoot(invalidLimaDir)
			if err != nil {
				t.Fatalf("open %s: %v", invalidLimaDir, err)
			}
			defer dir.Close()

			if *update {
				if err := dir.WriteFile(name, []byte(want), 0o600); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}

				if err := dir.WriteFile(expectName, []byte(wantExpect), 0o600); err != nil {
					t.Fatalf("write %s: %v", expectPath, err)
				}

				return
			}

			got, err := dir.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v (run go test ./test/devenv -update to write it)", path, err)
			}

			if diff := cmp.Diff(strings.Split(want, "\n"), strings.Split(string(got), "\n")); diff != "" {
				t.Errorf("%s is not %s with only its breakage (%s); run go test ./test/devenv -update to rewrite it (-want +got):\n%s",
					path, limaTemplate, tc.why, diff)
			}

			gotExpect, err := dir.ReadFile(expectName)
			if err != nil {
				t.Fatalf("read %s: %v (run go test ./test/devenv -update to write it)", expectPath, err)
			}

			if string(gotExpect) != wantExpect {
				t.Errorf("%s does not name %s's refusal (%s); it must hold the expected substring of limactl validate's message and one newline (-want +got):\n%s",
					expectPath, name, tc.why, cmp.Diff(wantExpect, string(gotExpect)))
			}
		})
	}
}

// TestInvalidLimaTemplatesHaveACase fails on a file in dev/testdata/lima-invalid
// that no case writes, or a file a case writes that is missing: the job validates
// every .yaml template there, and one no case pins could differ from dev/lima.yaml
// in anything, while a .expect no case has, or a case without one, names a refusal
// nothing checks.
func TestInvalidLimaTemplatesHaveACase(t *testing.T) {
	t.Parallel()

	if *update {
		// The other test is writing the templates in parallel; run again without -update.
		t.Skip("-update writes the templates; run the test again without it to check the directory")
	}

	entries, err := os.ReadDir(invalidLimaDir)
	if err != nil {
		t.Fatalf("read %s: %v", invalidLimaDir, err)
	}

	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}

	var want []string
	for _, tc := range limaBreakages {
		want = append(want, tc.name+".yaml", tc.name+".expect")
	}

	slices.Sort(want)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("%s does not hold exactly one template and one .expect per case of limaBreakages (-want +got):\n%s", invalidLimaDir, diff)
	}
}

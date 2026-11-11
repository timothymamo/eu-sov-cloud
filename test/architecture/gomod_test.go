package architecture_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestRootModuleHasNoReplace pins the auditor's M1 finding: a replace directive
// in the root go.mod lets a fork or a local copy stand in for an import the
// depguard rules allow by prefix, and neither depguard nor the third-party
// exemption of the package check sees the difference, so the release would
// carry code no rule vetted. The module holds none today, and none may be
// added: a fork belongs in an allow entry the rule names, reviewed with it.
func TestRootModuleHasNoReplace(t *testing.T) {
	t.Parallel()

	for _, replace := range goModReplaces(t, repositoryRoot(t)) {
		t.Errorf("go.mod holds the replace directive %s: it puts code no rule vetted where an allow entry allows an import by prefix",
			replace)
	}
}

// TestGoModReplaces pins the check TestRootModuleHasNoReplace trusts, over
// go.mod files written here: a single-line replace directive, a block of them,
// a local directory in place of a module, and the three spellings a hand
// scanner misses - a bare carriage return after the word, with and without a
// block, and a CRLF file - all of which the go command reads as the directives
// they are. The two lines that are none, a module path that merely holds the
// word and a go.mod with no replace at all, hold none.
func TestGoModReplaces(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		goMod string
		want  []string
	}{
		{
			name:  "a go.mod holding no replace directive holds none",
			goMod: "module example.com/m\n\ngo 1.27.4\n\nrequire (\n\texample.com/lib v1.0.0\n)\n",
		},
		{
			name:  "a module path that merely holds the word is no directive",
			goMod: "module example.com/m\n\nrequire example.com/replace v1.0.0\n",
		},
		{
			name:  "a single-line replace directive is found",
			goMod: "module example.com/m\n\nreplace example.com/lib => example.com/fork v1.2.3\n",
			want:  []string{"example.com/lib => example.com/fork v1.2.3"},
		},
		{
			name:  "a local directory in place of a module is found",
			goMod: "module example.com/m\n\nreplace example.com/lib => ../fork\n",
			want:  []string{"example.com/lib => ../fork"},
		},
		{
			name: "a replace block's directives are found one by one",
			goMod: "module example.com/m\n\nreplace (\n\texample.com/lib => example.com/fork v1.2.3\n" +
				"\texample.com/other => ../other\n)\n",
			want: []string{
				"example.com/lib => example.com/fork v1.2.3",
				"example.com/other => ../other",
			},
		},
		{
			// The go.mod lexer reads a bare carriage return as whitespace,
			// so the go command applies this directive.
			name:  "a bare carriage return after the word does not hide a single-line replace",
			goMod: "module example.com/m\n\nreplace\rexample.com/lib => example.com/fork v1.2.3\n",
			want:  []string{"example.com/lib => example.com/fork v1.2.3"},
		},
		{
			// The same carriage return also separates the word from the
			// parenthesis that opens the block.
			name: "a bare carriage return after the word does not hide a replace block",
			goMod: "module example.com/m\n\nreplace\r(\n\texample.com/lib => example.com/fork v1.2.3\n" +
				"\texample.com/other => ../other\n)\n",
			want: []string{
				"example.com/lib => example.com/fork v1.2.3",
				"example.com/other => ../other",
			},
		},
		{
			name:  "a CRLF file's replace directive is found",
			goMod: "module example.com/m\r\n\r\nreplace example.com/lib => example.com/fork v1.2.3\r\n",
			want:  []string{"example.com/lib => example.com/fork v1.2.3"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(tc.goMod), 0o600); err != nil {
				t.Fatalf("write go.mod: %v", err)
			}

			if diff := cmp.Diff(tc.want, goModReplaces(t, dir)); diff != "" {
				t.Errorf("goModReplaces(-want +got):\n%s", diff)
			}
		})
	}
}

// TestRepositoryHasNoWorkspaceOrVendor pins the reviewer's N2 finding and the
// auditor's LOW one: a replace directive is not the only way to put unvetted
// code where an allow entry allows an import by prefix. A committed vendor
// directory wins over the module cache, and a go.work or go.work.sum file,
// which .gitignore keeps out of git, makes a directory outside the module stand
// in for a path under it. The release builds the main module alone, so none of
// them may exist, tracked or untracked: the check reads the file system, not
// git ls-files, because go.work is ignored and a force-added vendor directory
// is what a look at git ls-files would not have to catch.
func TestRepositoryHasNoWorkspaceOrVendor(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)

	// vendor/ is a directory, so any one of them counts: a vendor directory
	// holding no modules.txt is what a half-finished go mod vendor leaves
	// behind, and a single go build -mod=vendor would read it.
	for _, name := range []string{"go.work", "go.work.sum", "vendor"} {
		_, err := os.Stat(filepath.Join(root, name))
		switch {
		case err == nil:
			t.Errorf("%s exists at the repository root: it puts code no rule vetted where an allow entry allows an import by prefix, and the release builds the main module alone",
				name)
		case !errors.Is(err, fs.ErrNotExist):
			t.Errorf("stat %s: %v", name, err)
		}
	}
}

// goModReplaces returns the replace directives the go command reads from the
// go.mod in dir, each as "old => new" with the version of a module
// replacement, in the order go.mod holds them. It asks the go command rather
// than parsing the file: go.mod's lexer reads a bare carriage return as
// whitespace, so replace\rexample.com/lib => ... and replace\r( + entries +
// ) are directives the go command applies, and a hand scanner that reads only
// a space, a tab or a parenthesis after the word reports neither.
func goModReplaces(t *testing.T, dir string) []string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "go", "mod", "edit", "-json")
	cmd.Dir = dir
	// No toolchain, proxy or workspace of the developer's may decide what
	// the go command reads: the guard is about the file, and the check runs
	// offline.
	cmd.Env = makeEnv(map[string]string{
		"GOFLAGS":     "-mod=readonly",
		"GOWORK":      "off",
		"GOPROXY":     "off",
		"GOTOOLCHAIN": "local",
	})

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go mod edit -json in %s: %v", dir, err)
	}

	var parsed struct {
		Replace []struct {
			Old moduleRef
			New moduleRef
		}
	}

	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("decode go mod edit -json in %s: %v", dir, err)
	}

	replaces := make([]string, 0, len(parsed.Replace))

	for _, replace := range parsed.Replace {
		replaces = append(replaces, replace.Old.String()+" => "+replace.New.String())
	}

	if len(replaces) == 0 {
		return nil
	}

	return replaces
}

// moduleRef is a module path and, for a module replacement, the version it
// names. A directory replacement names a path alone.
type moduleRef struct {
	Path    string
	Version string
}

// String renders the reference as go.mod writes it.
func (ref moduleRef) String() string {
	if ref.Version == "" {
		return ref.Path
	}

	return ref.Path + " " + ref.Version
}

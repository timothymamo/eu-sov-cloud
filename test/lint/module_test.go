package lint_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// lintTags are the build tags golangci-lint loads files with, as the go
// command's -tags flag takes them: negatableTags, the one list of them, which
// TestNegatableTagsMatchGolangci keeps equal to run.build-tags in .golangci.yml.
var lintTags = strings.Join(slices.Sorted(maps.Keys(negatableTags)), ",")

// lintEnv is one environment golangci-lint loads packages in on CI, the
// linux/amd64 entry of the lint-target matrix, pinned so that the go command
// answers the same on any host: linux on amd64 with cgo,
// the tags given on the command line only, no go.work, and no network. The
// module cache is already filled by the build that runs these tests.
var lintEnv = []string{
	"GOOS=linux",
	"GOARCH=amd64",
	"CGO_ENABLED=1",
	"GOFLAGS=-mod=readonly",
	"GOWORK=off",
	"GOPROXY=off",
}

// passedEnv names the variables of this process a child (go or golangci-lint)
// is given: where the go toolchain, its caches and a C compiler are, and the
// home and temp directories they fall back on, on Linux and macOS (Windows is
// neither a CI nor a dev host). Nothing else crosses over, so the credentials
// and API keys in this environment never reach a child or an error message
// that prints its output.
var passedEnv = []string{
	"PATH", "HOME", "TMPDIR", "GOTMPDIR",
	"GOROOT", "GOPATH", "GOCACHE", "GOMODCACHE", "GOENV", "GOTOOLCHAIN",
	"CC", "CXX",
	"XDG_CACHE_HOME", "XDG_CONFIG_HOME",
}

// childEnv returns the environment of a child process: each variable of
// passedEnv that is set here, then extra, which wins on a repeated key.
func childEnv(extra ...string) []string {
	env := make([]string, 0, len(passedEnv)+len(extra))
	for _, key := range passedEnv {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return append(env, extra...)
}

// moduleRoot returns the root of the module. go test runs a test in its
// package's directory, test/lint.
func moduleRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root: %v", err)
	}
	return root
}

// goFile is a Go file of the module.
type goFile struct {
	rel string // relative to the walked directory, slash-separated
	src []byte // content
}

// moduleGoFiles returns every .go file under root, tests included, outside the
// directories the go command's ./... pattern leaves out, which golangci-lint
// never visits (skipDir). It reads them with walkGoFiles, the walk
// TestSourcesHideNothingFromDepguard reads, and fails the test on each .go
// file that is not a regular file.
func moduleGoFiles(t *testing.T, root string) []goFile {
	t.Helper()

	var files []goFile
	refused, err := walkGoFiles(root, skipDir, func(rel string, src []byte) error {
		files = append(files, goFile{rel: rel, src: src})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	for _, problem := range refused {
		t.Error(problem)
	}
	if len(files) == 0 {
		t.Fatalf("no Go files found under %s", root)
	}
	return files
}

// listedPackage holds the fields of go list -json that these tests read.
type listedPackage struct {
	ImportPath     string
	Name           string
	Dir            string
	GoFiles        []string
	CgoFiles       []string
	TestGoFiles    []string
	XTestGoFiles   []string
	IgnoredGoFiles []string
	InvalidGoFiles []string
	Module         *struct {
		Path string
		Main bool
	}
}

// goList runs go list -e -json=<fields> with the build tags lint uses, in
// lint's environment, from the module root, and returns the packages it lists.
func goList(t *testing.T, root, fields string, args ...string) []listedPackage {
	t.Helper()

	args = append([]string{"list", "-e", "-tags", lintTags, "-json=" + fields}, args...)
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = root
	cmd.Env = childEnv(lintEnv...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go %s: %v\n%s\nThe module cache must hold every module go.mod requires; "+
			"run go mod download if it does not.", strings.Join(args, " "), err, stderr.String())
	}

	var pkgs []listedPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p listedPackage
		err := dec.Decode(&p)
		if errors.Is(err, io.EOF) {
			return pkgs
		}
		if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		pkgs = append(pkgs, p)
	}
}

// relToRoot returns dir relative to the module root, slash-separated, and
// whether dir is inside the module root at all. Both are compared with their
// symbolic links resolved, as the go command resolves them.
func relToRoot(t *testing.T, root, dir string) (string, bool) {
	t.Helper()

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// TestChildEnvPassesOnlyItsList pins that a child process gets the variables
// of passedEnv and the extras, and no other variable of this process.
func TestChildEnvPassesOnlyItsList(t *testing.T) {
	t.Setenv("EUSOV_CHILD_ENV_SENTINEL", "must-not-cross")
	t.Setenv("GOCACHE", "/cache")

	env := childEnv("GOPROXY=off")

	keys := map[string]bool{}
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		keys[key] = true
	}
	if keys["EUSOV_CHILD_ENV_SENTINEL"] {
		t.Error("childEnv passed a variable that is not in passedEnv")
	}
	for _, key := range []string{"GOCACHE", "GOPROXY"} {
		if !keys[key] {
			t.Errorf("childEnv dropped %s", key)
		}
	}
	for key := range keys {
		if key != "GOPROXY" && !slices.Contains(passedEnv, key) {
			t.Errorf("childEnv passed %s, which is not in passedEnv", key)
		}
	}
}

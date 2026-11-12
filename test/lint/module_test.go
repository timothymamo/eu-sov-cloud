package lint_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/test/internal/releasetarget"
)

// lintTags are the build tags golangci-lint loads files with, as the go
// command's -tags flag takes them: negatableTags, the one list of them, which
// TestNegatableTagsMatchGolangci keeps equal to run.build-tags in .golangci.yml.
var lintTags = strings.Join(slices.Sorted(maps.Keys(negatableTags)), ",")

// offlineEnv pins the go command to answer the same on any host: the tags
// given on the command line only, no go.work, no network, and the baseline
// amd64 and arm64 levels the release builds at, as make lint and CI set them,
// so a file such as one built only at amd64.v3 is never counted as loaded
// because of a level this process happens to have. The module cache is already
// filled by the build that runs these tests.
var offlineEnv = []string{
	"GOFLAGS=-mod=readonly",
	"GOWORK=off",
	"GOPROXY=off",
	"GOAMD64=v1",
	"GOARM64=v8.0",
}

// lintEnv is one environment golangci-lint loads packages in on CI, the native
// linux/amd64 entry of the lint-target matrix: linux on amd64 with cgo, offline.
var lintEnv = append([]string{"GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=1"}, offlineEnv...)

// targetEnv is the environment of the lint run for one release target: its
// platform with cgo off, as the release builds it, offline.
func targetEnv(target releasetarget.Target) []string {
	return append([]string{"GOOS=" + target.GOOS, "GOARCH=" + target.GOARCH, "CGO_ENABLED=0"}, offlineEnv...)
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
// lint's environment (lintEnv), from the module root, and returns the packages
// it lists.
func goList(t *testing.T, root, fields string, args ...string) []listedPackage {
	t.Helper()

	return goListIn(t, root, lintEnv, fields, args...)
}

// goListIn is goList in the environment env, which childEnv completes.
func goListIn(t *testing.T, root string, env []string, fields string, args ...string) []listedPackage {
	t.Helper()

	args = append([]string{"list", "-e", "-tags", lintTags, "-json=" + fields}, args...)
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = root
	cmd.Env = childEnv(env...)
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

// unloadedGoFiles returns the sorted, slash-separated paths, relative to root,
// of the .go files under root (moduleGoFiles) that the go command loads for
// none of targets: for each it lists ./... with the tags lint uses, in that
// target's environment (targetEnv), and a file it lists for any target,
// as a Go, cgo, test or invalid file, counts as loaded. An invalid file is one
// lint loads and reports, so it is not hidden from lint.
func unloadedGoFiles(t *testing.T, root string, targets []releasetarget.Target) []string {
	t.Helper()

	loaded := map[string]bool{}
	for _, target := range targets {
		for _, p := range goListIn(t, root, targetEnv(target),
			"Dir,GoFiles,CgoFiles,TestGoFiles,XTestGoFiles,InvalidGoFiles", "./...") {
			if p.Dir == "" {
				continue
			}
			dir, ok := relToRoot(t, root, p.Dir)
			if !ok {
				continue
			}
			for _, names := range [][]string{p.GoFiles, p.CgoFiles, p.TestGoFiles, p.XTestGoFiles, p.InvalidGoFiles} {
				for _, name := range names {
					loaded[path.Join(dir, name)] = true
				}
			}
		}
	}

	var unloaded []string
	for _, f := range moduleGoFiles(t, root) {
		if !loaded[f.rel] {
			unloaded = append(unloaded, f.rel)
		}
	}
	slices.Sort(unloaded)
	return unloaded
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

// TestTargetEnvPinsTheReleaseBuild pins the whole environment of a release
// target's lint run. The platform bounds what the target loads and the levels
// GOAMD64=v1 and GOARM64=v8.0 pin the amd64 and arm64 build to the release
// baseline: without the pin, a file built only at a higher level, such as
// testdata/platformprobe/amd64v3.go, is loaded on a host whose go env sets
// that level, and the test that only a release target loads it passes for the
// wrong reason. cgo is off as the release builds it, and the run is offline.
func TestTargetEnvPinsTheReleaseBuild(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target releasetarget.Target
	}{
		{name: "linux amd64", target: releasetarget.Target{GOOS: "linux", GOARCH: "amd64"}},
		{name: "windows arm64", target: releasetarget.Target{GOOS: "windows", GOARCH: "arm64"}},
		{name: "darwin arm64", target: releasetarget.Target{GOOS: "darwin", GOARCH: "arm64"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want := []string{
				"CGO_ENABLED=0",
				"GOAMD64=v1",
				"GOARCH=" + tc.target.GOARCH,
				"GOARM64=v8.0",
				"GOFLAGS=-mod=readonly",
				"GOOS=" + tc.target.GOOS,
				"GOPROXY=off",
				"GOWORK=off",
			}
			got := slices.Clone(targetEnv(tc.target))
			slices.Sort(got)

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("targetEnv(%s/%s) mismatch (-want +got):\n%s", tc.target.GOOS, tc.target.GOARCH, diff)
			}
		})
	}
}

// TestChildEnvPassesOnlyItsList pins that a child process gets the variables
// of passedEnv and the extras, and no other variable of this process.
func TestChildEnvPassesOnlyItsList(t *testing.T) {
	t.Setenv("SHIFT2EU_CHILD_ENV_SENTINEL", "must-not-cross")
	t.Setenv("GOCACHE", "/cache")

	env := childEnv("GOPROXY=off")

	keys := map[string]bool{}
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		keys[key] = true
	}
	if keys["SHIFT2EU_CHILD_ENV_SENTINEL"] {
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

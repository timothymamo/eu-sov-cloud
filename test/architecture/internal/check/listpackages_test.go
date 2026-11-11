package check_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/internal/releasetarget"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// The fixture modules below live under testdata, so `go list ./...` and lint
// never read them: each has a go.mod of its own, and its own way to change a
// listing that reads anything but the settings ListPackages passes.
const (
	// taggedModule builds its a package from two files, one of them behind the
	// sneaky build tag and importing os: a listing that picks up a tag from
	// anywhere but its arguments compiles it in. Its b package holds a file
	// behind goexperiment.fieldtrack, which imports os in the same way.
	taggedModule = "testdata/tagged"

	// privateModule requires a module that does not resolve. A listing that
	// inherits GOPRIVATE or GONOPROXY leaves GOPROXY=off behind and dials the
	// name instead.
	privateModule = "testdata/private"

	// brokenModule imports a package no module provides: the listing must fail
	// rather than report a package it could not resolve.
	brokenModule = "testdata/broken"
)

// fixtureDir returns the fixture module named from the package directory, as an
// absolute path: ListPackages runs go list in it, and go list runs in the
// module there, not in this one.
func fixtureDir(t *testing.T, name string) string {
	t.Helper()

	dir, err := filepath.Abs(filepath.FromSlash(name))
	if err != nil {
		t.Fatalf("resolve %s: %v", name, err)
	}

	return dir
}

// TestListPackagesIgnoresTheEnvironment pins the environment go list runs with:
// ListPackages builds it from an allow list of the inherited variables it needs
// plus the settings the check passes, so neither a developer's environment nor
// their go env file can add a build tag or switch an experiment on in the
// listing.
func TestListPackagesIgnoresTheEnvironment(t *testing.T) {
	// Not parallel: every case sets a process-wide environment variable.
	dir := fixtureDir(t, taggedModule)

	t.Run("GOFLAGS in the environment adds no build tag", func(t *testing.T) {
		t.Setenv("GOFLAGS", "-tags=sneaky")

		assertNoSneakyImport(t, dir)
	})

	t.Run("a go env file adds no build tag", func(t *testing.T) {
		withGoEnvFile(t, "GOFLAGS=-tags=sneaky\n")

		assertNoSneakyImport(t, dir)

		// The twin: the same go list with the same HOME lists the file's tag,
		// so the fixture really would show a leak. GOENV is dropped because the
		// listing has to fix it itself; GOFLAGS too, because an environment
		// GOFLAGS wins over the file's and the listing sets one of its own.
		if imports := goListImports(t, dir, "./a", commandEnv("GOENV", "GOFLAGS")); !slices.Contains(imports, "os") {
			t.Errorf("go list ./a with the go env file listed no import os, so the env file proves nothing: imports: %v",
				imports)
		}
	})

	t.Run("GOENV=off stops the go command reading the go env file", func(t *testing.T) {
		// The listing sets GOFLAGS itself, and an environment GOFLAGS wins over
		// the file's, so only a setting it leaves unset shows what GOENV=off
		// does: GOEXPERIMENT selects the b fixture's experiment file.
		withGoEnvFile(t, "GOEXPERIMENT=fieldtrack\n")

		assertNoExperimentImport(t, dir)

		// The twin: go list with the same HOME and without GOENV=off reads the
		// file and compiles the experiment file. GOENV and GOEXPERIMENT are
		// dropped because an environment value would win over the file's.
		if imports := goListImports(t, dir, "./b", commandEnv("GOENV", "GOEXPERIMENT")); !slices.Contains(imports, "os") {
			t.Errorf("go list ./b with the go env file listed no import os, so the env file proves nothing: imports: %v",
				imports)
		}
	})

	t.Run("GOEXPERIMENT in the environment does not reach go list", func(t *testing.T) {
		t.Setenv("GOEXPERIMENT", "fieldtrack")

		assertNoExperimentImport(t, dir)

		// The twin: go list with the same GOEXPERIMENT compiles the experiment
		// file, so the listing above really would show a leak. GOENV and GOFLAGS
		// are dropped because an environment value of theirs would only hide
		// what GOEXPERIMENT does.
		if imports := goListImports(t, dir, "./b", commandEnv("GOENV", "GOFLAGS")); !slices.Contains(imports, "os") {
			t.Errorf("go list ./b with GOEXPERIMENT=%s listed no import os, so the fixture proves nothing: imports: %v",
				os.Getenv("GOEXPERIMENT"), imports)
		}
	})
}

// assertNoSneakyImport lists the tagged fixture's a package for linux/amd64
// with no build tags, and fails when its imports hold os, which only the
// sneaky-tagged file imports.
func assertNoSneakyImport(t *testing.T, dir string) {
	t.Helper()

	pkgs, err := check.ListPackages(t.Context(), dir, releasetarget.Target{GOOS: "linux", GOARCH: "amd64"}, false, nil, "./a")
	if err != nil {
		t.Fatalf("ListPackages(%s, ./a): %v", dir, err)
	}

	pkg, ok := packageByPath(pkgs, "example.com/tagged/a")
	if !ok {
		t.Fatalf("ListPackages(%s, ./a) did not list example.com/tagged/a: %v", dir, importPaths(pkgs))
	}

	if slices.Contains(pkg.Imports, "os") {
		t.Errorf("ListPackages(%s, ./a) listed example.com/tagged/a with import os: a build tag from outside the check's arguments reached go list; imports: %v",
			dir, pkg.Imports)
	}
}

// assertNoExperimentImport lists the tagged fixture's b package for linux/amd64
// with no build tags, and fails when its imports hold os, which only the
// experiment-tagged file imports.
func assertNoExperimentImport(t *testing.T, dir string) {
	t.Helper()

	pkgs, err := check.ListPackages(t.Context(), dir, releasetarget.Target{GOOS: "linux", GOARCH: "amd64"}, false, nil, "./b")
	if err != nil {
		t.Fatalf("ListPackages(%s, ./b): %v", dir, err)
	}

	pkg, ok := packageByPath(pkgs, "example.com/tagged/b")
	if !ok {
		t.Fatalf("ListPackages(%s, ./b) did not list example.com/tagged/b: %v", dir, importPaths(pkgs))
	}

	if slices.Contains(pkg.Imports, "os") {
		t.Errorf("ListPackages(%s, ./b) listed example.com/tagged/b with import os: GOEXPERIMENT reached go list; imports: %v",
			dir, pkg.Imports)
	}
}

// withGoEnvFile points HOME at a temp dir that holds go's default environment
// file with content, so the go command reads the file the way it reads a
// developer's.
func withGoEnvFile(t *testing.T, content string) {
	t.Helper()

	// The caches stay the ones the test's own go uses: moving HOME moves the
	// default build and module caches under the new home, and a go whose module
	// cache holds no toolchain downloads one.
	gocache := goEnvSetting(t, "GOCACHE")
	gomodcache := goEnvSetting(t, "GOMODCACHE")

	home := t.TempDir()

	// The go command reads its env file from the user config directory: on
	// Linux and the BSDs $XDG_CONFIG_HOME, or $HOME/.config when that is empty,
	// which the go command reads as unset; on macOS a directory under Library.
	// GOENV is emptied so a developer's GOENV cannot point the go command at
	// their own file instead.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GOCACHE", gocache)
	t.Setenv("GOMODCACHE", gomodcache)
	t.Setenv("HOME", home)
	t.Setenv("GOENV", "")

	envFile := defaultGoEnvFile(t, home)
	if err := os.MkdirAll(filepath.Dir(envFile), 0o700); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(envFile), err)
	}

	if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", envFile, err)
	}
}

// goListImports runs `go list -json` for pattern in dir with env as the
// command's whole environment, and returns the imports of the one package it
// lists: the listing a developer's go list makes, which the cases above use to
// prove their fixture would show a leak.
func goListImports(t *testing.T, dir, pattern string, env []string) []string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "go", "list", "-json=ImportPath,Imports", "--", pattern)
	cmd.Dir = dir
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("go list %s in %s: %v: %s", pattern, dir, err, strings.TrimSpace(stderr.String()))
	}

	var listed struct {
		Imports []string
	}

	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatalf("decode go list %s output: %v: %s", pattern, err, stdout.String())
	}

	return listed.Imports
}

// commandEnv returns the environment the test runs in with the named variables
// removed: what go list gets when the case does not fix them itself.
func commandEnv(unset ...string) []string {
	env := os.Environ()
	out := make([]string, 0, len(env))

	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(unset, name) {
			out = append(out, kv)
		}
	}

	return out
}

// goEnvSetting returns the value `go env` reports for name, read before a case
// moves HOME: the real build and module caches stay the ones the test's go uses,
// so no case downloads anything.
func goEnvSetting(t *testing.T, name string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "go", "env", name)
	cmd.Dir = fixtureDir(t, taggedModule)

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go env %s: %v", name, err)
	}

	return strings.TrimSpace(string(out))
}

// defaultGoEnvFile returns the environment file the go command reads when GOENV
// names none: the file `go env -w` writes under home.
func defaultGoEnvFile(t *testing.T, home string) string {
	t.Helper()

	var file string

	switch runtime.GOOS {
	case "windows":
		t.Skip("the default go env file lives in %APPDATA% on Windows, not under HOME")
	case "darwin":
		file = filepath.Join(home, "Library", "Application Support", "go", "env")
	default:
		file = filepath.Join(home, ".config", "go", "env")
	}

	return file
}

// TestListPackagesUsesTheTagsItIsGiven is the twin of the test above: passed
// the tag, the same fixture compiles the sneaky file, so the fixture really
// would show a leak.
func TestListPackagesUsesTheTagsItIsGiven(t *testing.T) {
	t.Parallel()

	dir := fixtureDir(t, taggedModule)

	pkgs, err := check.ListPackages(t.Context(), dir, releasetarget.Target{GOOS: "linux", GOARCH: "amd64"}, false, []string{"sneaky"}, "./a")
	if err != nil {
		t.Fatalf("ListPackages(%s, ./a, tags=sneaky): %v", dir, err)
	}

	pkg, ok := packageByPath(pkgs, "example.com/tagged/a")
	if !ok {
		t.Fatalf("ListPackages(%s, ./a, tags=sneaky) did not list example.com/tagged/a: %v", dir, importPaths(pkgs))
	}

	if !slices.Contains(pkg.Imports, "os") {
		t.Errorf("ListPackages(%s, ./a, tags=sneaky) listed example.com/tagged/a without import os, so the fixture no longer proves a leak would show; imports: %v",
			dir, pkg.Imports)
	}
}

// TestListPackagesFetchesNothing pins that no inherited variable can make go
// list reach the network: the private fixture's one dependency is a name that
// cannot resolve, and the listing must fail offline, saying so, rather than
// dial it.
func TestListPackagesFetchesNothing(t *testing.T) {
	// Not parallel: the cases set process-wide environment variables.
	dir := fixtureDir(t, privateModule)

	for _, setting := range []struct {
		name  string
		value string
	}{
		{name: "GOPRIVATE", value: "example.invalid"},
		{name: "GONOPROXY", value: "example.invalid"},
	} {
		t.Run(setting.name+" does not reach go list", func(t *testing.T) {
			t.Setenv(setting.name, setting.value)

			_, err := check.ListPackages(t.Context(), dir, releasetarget.Target{GOOS: "linux", GOARCH: "amd64"}, false, nil, "./p")
			if err == nil {
				t.Fatalf("ListPackages(%s, ./p) = nil error; want an error: no module provides example.invalid/dep", dir)
			}

			// The error must be the offline one. "lookup" alone is no sign of
			// a fetch: the offline message is "module lookup disabled by
			// GOPROXY=off". A direct fetch shows as a dial, a DNS lookup of
			// the name or an https request.
			offline := false

			for _, message := range []string{"GOPROXY=off", "missing go.sum entry"} {
				if strings.Contains(err.Error(), message) {
					offline = true
				}
			}

			if !offline {
				t.Errorf("ListPackages(%s, ./p) failed without saying it stopped offline, want GOPROXY=off or a missing go.sum entry to stop the listing; error: %v",
					dir, err)
			}

			for _, attempt := range []string{"dial tcp", "no such host", "https://", "git ls-remote", "connection refused", "i/o timeout"} {
				if strings.Contains(err.Error(), attempt) {
					t.Errorf("ListPackages(%s, ./p) tried to fetch the module (%q), so %s reached go list; error: %v",
						dir, attempt, setting.name, err)
				}
			}
		})
	}
}

// TestListPackagesRefusesABrokenDependency pins the fail-closed decode: a
// package whose import no module provides fails the listing, so no check runs
// over a package set go list could not resolve.
func TestListPackagesRefusesABrokenDependency(t *testing.T) {
	t.Parallel()

	dir := fixtureDir(t, brokenModule)

	pkgs, err := check.ListPackages(t.Context(), dir, releasetarget.Target{GOOS: "linux", GOARCH: "amd64"}, false, nil, "./b")
	if err == nil {
		t.Fatalf("ListPackages(%s, ./b) = %v, nil; want an error for an import no module provides", dir, importPaths(pkgs))
	}
}

// packageByPath returns the package with the import path from pkgs.
func packageByPath(pkgs []check.Package, importPath string) (check.Package, bool) {
	for _, pkg := range pkgs {
		if pkg.ImportPath == importPath {
			return pkg, true
		}
	}

	return check.Package{}, false
}

// importPaths returns the import path of every package in pkgs, for a failure
// message.
func importPaths(pkgs []check.Package) []string {
	paths := make([]string, 0, len(pkgs))

	for _, pkg := range pkgs {
		paths = append(paths, pkg.ImportPath)
	}

	return paths
}

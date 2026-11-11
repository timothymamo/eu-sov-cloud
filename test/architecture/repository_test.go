package architecture_test

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

const (
	// repoRoot is the module root, as the package directory go test runs in
	// reaches it.
	repoRoot = "../.."

	// modulePath is the module the check reads import paths under.
	modulePath = "github.com/timothymamo/eu-sov-cloud"
)

// skippedStrictRules are the strict rules of .golangci.yml the package check
// cannot hold, each with the reason it cannot. Every other strict rule is one
// the check reads a layer from: TestEveryStrictRuleIsChecked requires that, and
// TestReleaseTargetDependencies lists the packages of its directories. A strict
// rule in neither fails TestEveryStrictRuleIsChecked.
var skippedStrictRules = map[string]string{
	"cli-gets-adapters-from-cmd":                     "it allows $gostd, which depguard expands to one prefix entry per top-level directory of GOROOT/src, so no allow list of package-by-package imports can say what it means",
	"e2e-is-black-box":                               "it holds test files, and go list -deps lists no test file and no package only a test imports",
	"cloud-adapters-reach-the-cloud-through-the-sdk": "it covers every directory under internal/adapter/source but two it excludes, so a sibling helper package is held too, which no list of **/<dir>/** directories can say; its allow list names the cloud SDK roots as prefixes, every package of the SDK, which no package-by-package list can spell out; and it holds test files, integration tests included, which go list -deps does not list",
}

// TestRepositoryHasNoLineDirectives pins that no Go file of this repository
// carries a line directive, hand-written or generated: the linter matches
// depguard's file globs against the position a directive names, so one hides a
// file from the architecture rules (issue #40). A symlink among the files git
// tracks or would add fails this test too: it is reported rather than read
// through. What git ignores and does not track is not looked at.
func TestRepositoryHasNoLineDirectives(t *testing.T) {
	t.Parallel()

	directives, err := check.FindLineDirectives(t.Context(), repoRoot)
	if err != nil {
		t.Fatalf("FindLineDirectives(%q): %v", repoRoot, err)
	}

	if len(directives) != 0 {
		t.Errorf("the repository's Go files hold %d line directives or symlinks, which hide a file from the linter:\n%s",
			len(directives), strings.Join(directives, "\n"))
	}
}

// TestReleaseTargetDependencies pins the checks against the repository: for
// every platform .goreleaser.yaml builds, with cgo off and on and with the e2e
// and integration files off and on, the domain, the application layer and the
// shared readers import nothing the depguard rules refuse, and the release
// binary is built from packages of the main module alone, each in the directory
// its import path names and holding no source but Go files.
func TestReleaseTargetDependencies(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)

	golangci := readRepositoryFile(t, ".golangci.yml")
	goreleaser := readRepositoryFile(t, ".goreleaser.yaml")

	// .goreleaser.yaml is the one source of the release platforms: the check
	// reads the list from it rather than pinning a copy here.
	targets, err := check.ReleaseTargets(goreleaser)
	if err != nil {
		t.Fatalf("ReleaseTargets(.goreleaser.yaml): %v", err)
	}

	if len(targets) == 0 {
		t.Fatal("ReleaseTargets(.goreleaser.yaml) returned no target, so the check below would run on nothing")
	}

	if !slices.Contains(targets, check.Target{GOOS: "linux", GOARCH: "amd64"}) {
		t.Errorf("ReleaseTargets(.goreleaser.yaml) = %v, want it to include linux/amd64", targets)
	}

	rules, patterns := depguardRules(t, golangci)

	cgoModes := []struct {
		name    string
		enabled bool
	}{
		{name: "cgo=0", enabled: false},
		{name: "cgo=1", enabled: true},
	}

	// The e2e and integration files are behind build tags; make lint loads
	// them, so the check reads the dependencies they bring in as well, and the
	// build the release makes as well as a developer's test build.
	tagSets := []struct {
		name string
		tags []string
	}{
		{name: "tags=none", tags: nil},
		{name: "tags=e2e", tags: []string{"e2e"}},
		{name: "tags=integration", tags: []string{"integration"}},
		{name: "tags=e2e+integration", tags: []string{"e2e", "integration"}},
	}

	for _, target := range targets {
		for _, cgo := range cgoModes {
			for _, tags := range tagSets {
				t.Run(fmt.Sprintf("%s/%s/%s/%s", target.GOOS, target.GOARCH, cgo.name, tags.name), func(t *testing.T) {
					t.Parallel()

					checkLayers(t, root, target, cgo.enabled, tags.tags, rules, patterns)
					checkReleaseBinary(t, root, target, cgo.enabled, tags.tags)
				})
			}
		}
	}
}

// depguardRules returns the rules the package check holds, read from the
// .golangci.yml document: for every strict rule the check can hold, one
// check.Rule per directory its files: list names, with that rule's allow list,
// and the go list pattern of that directory. Nothing is copied by hand, so a
// directory or a rule added to .golangci.yml is checked without an edit here.
func depguardRules(t *testing.T, golangci []byte) ([]check.Rule, []string) {
	t.Helper()

	strict, err := check.StrictRules(golangci)
	if err != nil {
		t.Fatalf("StrictRules(.golangci.yml): %v", err)
	}

	if len(strict) == 0 {
		t.Fatal("StrictRules(.golangci.yml) returned no rule, so the check below would run on nothing")
	}

	var (
		rules    []check.Rule
		patterns []string
	)

	for _, name := range strict {
		if _, skipped := skippedStrictRules[name]; skipped {
			continue
		}

		dirs, err := check.RuleDirs(golangci, name)
		if err != nil {
			t.Fatalf("RuleDirs(.golangci.yml, %q): %v", name, err)
		}

		allow, err := check.AllowList(golangci, name)
		if err != nil {
			t.Fatalf("AllowList(.golangci.yml, %q): %v", name, err)
		}

		if len(allow) == 0 {
			t.Fatalf("AllowList(.golangci.yml, %q) returned no entry, so its directories would be held to nothing", name)
		}

		for _, dir := range dirs {
			rules = append(rules, check.Rule{Name: name, Dir: dir, Allow: allow})
			patterns = append(patterns, "./"+dir+"/...")
		}
	}

	if len(rules) == 0 {
		t.Fatal("no strict rule gave the check a layer, so it would hold nothing")
	}

	return rules, patterns
}

// checkLayers lists the layer patterns for one build and fails when the check
// flags a package, when the listing fails, or when the listing did not hold the
// layers it was asked for.
func checkLayers(
	t *testing.T,
	root string,
	target check.Target,
	cgo bool,
	tags []string,
	rules []check.Rule,
	patterns []string,
) {
	t.Helper()

	pkgs, err := check.ListPackages(t.Context(), root, target, cgo, tags, patterns...)
	if err != nil {
		t.Fatalf("ListPackages(%v, cgo=%t, tags=%v): %v", target, cgo, tags, err)
	}

	if messages := check.Packages(root, modulePath, rules, pkgs); len(messages) != 0 {
		t.Errorf("Packages(%v, cgo=%t, tags=%v) flagged %d package(s):\n%s",
			target, cgo, tags, len(messages), strings.Join(messages, "\n"))
	}

	var listed []string

	for _, pkg := range pkgs {
		if !pkg.Standard {
			listed = append(listed, pkg.ImportPath)
		}
	}

	// A layer `go list` never read would pass the check above without being
	// looked at, so the directory of every rule must hold a listed package. The
	// four below are named as well: the rules are read from .golangci.yml, and
	// a rule quietly narrowed would otherwise narrow this guard with it.
	required := make([]string, 0, len(rules)+4)
	for _, rule := range rules {
		required = append(required, rule.Dir)
	}

	required = append(required,
		"internal/adapter/target/ovhcloud",
		"internal/adapter/target/scaleway",
		"internal/adapter/target/upcloud",
		"internal/adapter/internal/jsondoc",
	)

	seen := make(map[string]bool, len(required))

	for _, dir := range required {
		if seen[dir] {
			continue
		}

		seen[dir] = true

		prefix := modulePath + "/" + dir

		if !slices.ContainsFunc(listed, func(path string) bool {
			return path == prefix || strings.HasPrefix(path, prefix+"/")
		}) {
			t.Errorf("go list did not list a package of %s, so the check above read nothing of that layer; non-standard packages listed: %s",
				dir, strings.Join(listed, ", "))
		}
	}
}

// checkReleaseBinary lists what the release binary, check.ReleaseMain, is built
// from for one build and fails when a package of the module breaks the source
// rules: no package outside the main module, no package whose directory is not
// the one its import path names, and no cgo, C or assembly source anywhere in
// the binary.
func checkReleaseBinary(t *testing.T, root string, target check.Target, cgo bool, tags []string) {
	t.Helper()

	pkgs, err := check.ListPackages(t.Context(), root, target, cgo, tags, check.ReleaseMain)
	if err != nil {
		t.Fatalf("ListPackages(%s, %v, cgo=%t, tags=%v): %v", check.ReleaseMain, target, cgo, tags, err)
	}

	if messages := check.Sources(root, modulePath, pkgs); len(messages) != 0 {
		t.Errorf("Sources(%s, %v, cgo=%t, tags=%v) flagged %d package(s):\n%s",
			check.ReleaseMain, target, cgo, tags, len(messages), strings.Join(messages, "\n"))
	}

	// Without them the listing would pass over a binary that carries none of
	// the packages the check exists for.
	for _, want := range []string{
		modulePath + "/internal/adapter/target/ovhcloud",
		modulePath + "/internal/adapter/target/scaleway",
		modulePath + "/internal/cli",
	} {
		if !slices.ContainsFunc(pkgs, func(pkg check.Package) bool { return pkg.ImportPath == want }) {
			t.Errorf("go list did not list %s, so the check above read nothing of it", want)
		}
	}
}

// TestEveryStrictRuleIsChecked pins that every strict depguard rule of
// .golangci.yml is either one the package check reads a layer from (RuleDirs for
// its directories, AllowList for its imports) or a skip with a reason: a new
// strict rule in neither would refuse imports no test looks at, or the reverse,
// a rule quietly turned lax would leave its layer unchecked. A skip of a rule
// that is gone, or no longer strict, is an error too.
func TestEveryStrictRuleIsChecked(t *testing.T) {
	t.Parallel()

	golangci := readRepositoryFile(t, ".golangci.yml")

	strict, err := check.StrictRules(golangci)
	if err != nil {
		t.Fatalf("StrictRules(.golangci.yml): %v", err)
	}

	if len(strict) == 0 {
		t.Fatal("StrictRules(.golangci.yml) returned no rule, so this test would check nothing")
	}

	for _, name := range strict {
		reason, skipped := skippedStrictRules[name]
		if skipped {
			if reason == "" {
				t.Errorf("%s is skipped without a reason: say why the package check cannot hold it", name)
			}

			continue
		}

		if dirs, err := check.RuleDirs(golangci, name); err != nil {
			t.Errorf("the strict depguard rule %s is not one the package check can hold: %v; give its files: list the shape RuleDirs reads, or add it to skippedStrictRules with the reason the check cannot hold it", name, err)
		} else if len(dirs) == 0 {
			t.Errorf("RuleDirs(.golangci.yml, %s) returned no directory, so the rule holds no layer", name)
		}

		if _, err := check.AllowList(golangci, name); err != nil {
			t.Errorf("the strict depguard rule %s has no allow list the package check can read: %v", name, err)
		}
	}

	for _, name := range slices.Sorted(maps.Keys(skippedStrictRules)) {
		if !slices.Contains(strict, name) {
			t.Errorf("skippedStrictRules holds %s, which .golangci.yml no longer holds with list-mode: strict, so the skip is stale: remove it, or restore the rule", name)
		}
	}
}

// TestReleaseWorkflowIsClosed pins that the release is closed to what the
// check lists: the workflow holds exactly the five jobs the check knows, test,
// goreleaser, image, sbom and sign, with exactly their steps, so goreleaser
// runs with exactly release --clean after no repository code and with no
// environment of its own for the go command, only a commit on main is released,
// and the publishing privileges stay split: the image job publishes what it
// builds, the SBOM job only reads the registry and signs nothing, and the sign
// job runs sigstore's and GitHub's actions alone (check.ReleaseWorkflow). Nor
// may any other goreleaser document, go.work, go.work.sum or vendor sit at the
// root that git could commit (check.StrayReleaseFiles).
func TestReleaseWorkflowIsClosed(t *testing.T) {
	t.Parallel()

	stray, err := check.StrayReleaseFiles(t.Context(), repositoryRoot(t))
	if err != nil {
		t.Fatalf("StrayReleaseFiles(): %v", err)
	}

	for _, message := range stray {
		t.Error(message)
	}

	for _, message := range check.ReleaseWorkflow(readRepositoryFile(t, ".github/workflows/release.yml")) {
		t.Error(message)
	}
}

// TestWorkflowPrivilegesAreClosed pins the audit's finding of issue #72: no
// workflow but .github/workflows/release.yml may hold a packages: write,
// id-token: write or attestations: write permission, a write-all, or an
// on: push: tags: trigger. A workflow that pushes to a registry, mints an OIDC
// identity or runs on a tag publishes releases, and the release workflow is the
// one place that is reviewed for it.
func TestWorkflowPrivilegesAreClosed(t *testing.T) {
	t.Parallel()

	const dir = ".github/workflows"

	entries, err := os.ReadDir(filepath.Join(repositoryRoot(t), filepath.FromSlash(dir)))
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	if len(entries) == 0 {
		t.Fatalf("%s holds no workflow, so this test would check nothing", dir)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := dir + "/" + entry.Name()

		for _, message := range check.WorkflowPrivileges(name, readRepositoryFile(t, name)) {
			t.Error(message)
		}
	}
}

// TestCIImageJobIsTheReleaseDryRun pins the review's finding of issue #72:
// ci.yml's image job is the dry run of release.yml's image and sbom jobs, so a
// pull request fails when the release's platforms, annotations, provenance and
// SBOM settings, tag rules, syft version, action pins or BuildKit image drift
// from what the dry run builds and checks (check.DryRunMirrorsRelease).
func TestCIImageJobIsTheReleaseDryRun(t *testing.T) {
	t.Parallel()

	ci := readRepositoryFile(t, ".github/workflows/ci.yml")
	release := readRepositoryFile(t, ".github/workflows/release.yml")

	for _, message := range check.DryRunMirrorsRelease(ci, release) {
		t.Error(message)
	}
}

// repositoryRoot returns the repository root as an absolute path, for the calls
// that run go list in it.
func repositoryRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("resolve %q: %v", repoRoot, err)
	}

	return root
}

// readRepositoryFile reads the file name under the repository root, and fails
// the test when it cannot: every test here reads what the release reads.
func readRepositoryFile(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	return data
}

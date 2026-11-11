package architecture_test

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

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
	"cli-gets-adapters-from-cmd": "it allows $gostd, which depguard expands to one prefix entry per top-level directory of GOROOT/src, so no allow list of package-by-package imports can say what it means",
	"e2e-is-black-box":           "test/e2e holds test files only, so go list -deps reaches no package of it: it is no layer of the production packages the check holds, and a package only a test file imports is never listed at all",
}

// sdkRule is the strict depguard rule that holds every cloud source adapter, and
// every helper directory under internal/adapter/source, to its cloud's SDK: its
// allow list names the SDK roots as import prefixes because depguard reads an
// entry without $ as one, and its files: list takes two directories out with
// negated globs and holds test files too (issue #79).
const sdkRule = "cloud-adapters-reach-the-cloud-through-the-sdk"

// prefixEntryRules are the strict rules whose allow list may hold a prefix
// entry, each named on purpose: the SDK-only rule names the roots of the cloud
// SDKs, whose every package an adapter may reach its cloud through. Every other
// strict rule names the packages it allows one by one, where an entry that lost
// its trailing $ is a widening the reviewer must see (os$ -> os allows
// os/exec) rather than a prefix.
var prefixEntryRules = []string{sdkRule}

// sdkRuleExcludes are the directories the SDK-only rule's files: list takes out
// of it with negated globs: the inventory-file reader holds a rule of its own,
// and so does the contract suite the adapters share.
var sdkRuleExcludes = []string{
	"internal/adapter/source/inventoryfile",
	"internal/adapter/source/sourcetest",
}

// sdkRulePrefixes are the rule's allow entries written as an import prefix: the
// three SDK roots, and go-cmp's package directory, whose packages the adapters'
// test files import. Every other entry ends in $ and allows one package alone.
var sdkRulePrefixes = []string{
	"cloud.google.com/go/",
	"github.com/Azure/azure-sdk-for-go/sdk/",
	"github.com/aws/aws-sdk-go-v2/",
	"github.com/google/go-cmp/cmp/",
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

		for _, rule := range rulesOf(t, golangci, name) {
			rules = append(rules, rule)
			patterns = append(patterns, "./"+rule.Dir+"/...")
		}
	}

	if len(rules) == 0 {
		t.Fatal("no strict rule gave the check a layer, so it would hold nothing")
	}

	return rules, patterns
}

// rulesOf returns what the package check holds for one strict depguard rule: one
// check.Rule per directory its files: list names, with the directories its
// negated globs take out of the rule, whether it holds test files, and that
// rule's allow list. Nothing is copied by hand, so a directory, an exclusion, a
// prefix allow entry or a rule added to .golangci.yml is checked without an edit
// here.
func rulesOf(t *testing.T, golangci []byte, name string) []check.Rule {
	t.Helper()

	files, err := check.RuleDirs(golangci, name)
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

	rules := make([]check.Rule, 0, len(files.Dirs))

	for _, dir := range files.Dirs {
		rules = append(rules, check.Rule{
			Name:     name,
			Dir:      dir,
			Excludes: files.Excludes,
			Tests:    files.Tests,
			Allow:    allow,
		})
	}

	return rules
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
// rules: Sources checks every package whose import path is the module path or
// under it and requires that it belongs to the main module, so a nested module
// under that path is flagged, that its directory with symlinks resolved is the
// one its import path names, and that it holds no source but Go files: no cgo,
// C, C++, header, Fortran, Objective-C, assembly, SWIG or syso file. Imports are
// not checked here, and the standard library and packages outside the module
// path (third-party code) are skipped. It fails too when go list did not list
// internal/cli or one of the three target adapters, since the checks above
// would then read nothing of them.
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
		modulePath + "/internal/adapter/target/upcloud",
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

		if files, err := check.RuleDirs(golangci, name); err != nil {
			t.Errorf("the strict depguard rule %s is not one the package check can hold: %v; give its files: list the shape RuleDirs reads, or add it to skippedStrictRules with the reason the check cannot hold it", name, err)
		} else if len(files.Dirs) == 0 {
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

// TestCloudAdapterSDKRuleIsChecked pins that the SDK-only rule is no longer a
// skip (issue #79): the package check reads its files: list, its exclusions and
// its allow list, so TestEveryStrictRuleIsChecked and
// TestReleaseTargetDependencies hold every cloud source adapter to it, package
// by package, on every release target. The rule covers internal/adapter/source
// but the two directories it takes out with negated globs, holds test files,
// and allows the three SDK roots as import prefixes.
//
// The directories, the exclusions and the prefix entries are compared as whole
// sets: an exclusion or an exact entry that turned into a prefix without a
// reviewer seeing it is how a rule widens.
func TestCloudAdapterSDKRuleIsChecked(t *testing.T) {
	t.Parallel()

	golangci := readRepositoryFile(t, ".golangci.yml")

	if reason, skipped := skippedStrictRules[sdkRule]; skipped {
		t.Errorf("%s is still skipped (%s); the package check reads prefix allow entries, negated files globs and test-file imports now: remove the skip", sdkRule, reason)
	}

	strict, err := check.StrictRules(golangci)
	if err != nil {
		t.Fatalf("StrictRules(.golangci.yml): %v", err)
	}

	if !slices.Contains(strict, sdkRule) {
		t.Fatalf("StrictRules(.golangci.yml) = %v, want it to hold %s", strict, sdkRule)
	}

	files, err := check.RuleDirs(golangci, sdkRule)
	if err != nil {
		t.Fatalf("RuleDirs(.golangci.yml, %q): %v", sdkRule, err)
	}

	if diff := cmp.Diff(sortedCopy([]string{"internal/adapter/source"}), sortedCopy(files.Dirs)); diff != "" {
		t.Errorf("RuleDirs(.golangci.yml, %q).Dirs mismatch (-want +got):\n%s", sdkRule, diff)
	}

	if diff := cmp.Diff(sortedCopy(sdkRuleExcludes), sortedCopy(files.Excludes)); diff != "" {
		t.Errorf("RuleDirs(.golangci.yml, %q).Excludes mismatch (-want +got):\n%s: the rule takes these two directories out, and no other", sdkRule, diff)
	}

	if !files.Tests {
		t.Errorf("RuleDirs(.golangci.yml, %q).Tests = false, want true: the rule names no !$test, so it holds test files, the integration ones included", sdkRule)
	}

	allow, err := check.AllowList(golangci, sdkRule)
	if err != nil {
		t.Fatalf("AllowList(.golangci.yml, %q): %v", sdkRule, err)
	}

	var prefixes []string

	for _, entry := range allow {
		if entry.Prefix {
			prefixes = append(prefixes, entry.Path)
		}
	}

	if diff := cmp.Diff(sortedCopy(sdkRulePrefixes), sortedCopy(prefixes)); diff != "" {
		t.Errorf("AllowList(.golangci.yml, %q) prefix entries mismatch (-want +got):\n%s: the SDK roots are the entries the rule allows whole trees through", sdkRule, diff)
	}

	// The entries that end in $ stay exact: depguard allows that package alone
	// through one.
	if !slices.ContainsFunc(allow, func(e check.AllowEntry) bool { return !e.Prefix && e.Path == "errors" }) {
		t.Errorf("AllowList(.golangci.yml, %q) = %v, want the exact entry errors$", sdkRule, allow)
	}
}

// TestOnlyTheSDKRuleHoldsPrefixEntries is the second tripwire for a dropped $:
// before issue #79 a strict allow entry without one was refused, so `os$` that
// lost its dollar was caught twice over, and now a prefix entry is a shape the
// check reads. Only the rules named in prefixEntryRules may hold one; in every
// other strict rule a prefix entry is a widening, and this fails, naming the
// rule and the entry.
//
// A rule the package check skips is not read here: AllowList refuses its $gostd,
// and TestDepguardRulesArePinned pins that list entry by entry.
func TestOnlyTheSDKRuleHoldsPrefixEntries(t *testing.T) {
	t.Parallel()

	golangci := readRepositoryFile(t, ".golangci.yml")

	strict, err := check.StrictRules(golangci)
	if err != nil {
		t.Fatalf("StrictRules(.golangci.yml): %v", err)
	}

	checked := 0

	for _, name := range strict {
		if _, skipped := skippedStrictRules[name]; skipped {
			continue
		}

		checked++

		allow, err := check.AllowList(golangci, name)
		if err != nil {
			t.Errorf("AllowList(.golangci.yml, %q): %v", name, err)

			continue
		}

		for _, entry := range allow {
			if !entry.Prefix || slices.Contains(prefixEntryRules, name) {
				continue
			}

			t.Errorf("the strict depguard rule %s allows %s by prefix, and only %s may hold a prefix entry: write the package exact (%s$) or, for a whole tree, end the entry in /",
				name, entry.Path, strings.Join(prefixEntryRules, " and "), entry.Path)
		}
	}

	if checked == 0 {
		t.Fatal("no strict rule was read, so this test would check nothing")
	}

	for _, name := range prefixEntryRules {
		if !slices.Contains(strict, name) {
			t.Errorf("prefixEntryRules holds %s, which .golangci.yml no longer holds with list-mode: strict, so the exception is stale: remove it, or restore the rule", name)
		}
	}
}

// TestCloudAdapterSDKRuleHoldsItsProbeForEveryReleaseTarget is the issue's probe
// (issue #79), one per platform the release builds: a fixture adapter under
// internal/adapter/source that imports net/http directly fails the package check
// with the rule read from .golangci.yml, on every GOOS/GOARCH the release
// targets list, exactly as depguard refuses it. The adapter package is listed
// for the platform first, so a run that read nothing of it cannot pass.
func TestCloudAdapterSDKRuleHoldsItsProbeForEveryReleaseTarget(t *testing.T) {
	t.Parallel()

	golangci := readRepositoryFile(t, ".golangci.yml")

	targets, err := check.ReleaseTargets(readRepositoryFile(t, ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("ReleaseTargets(.goreleaser.yaml): %v", err)
	}

	if len(targets) == 0 {
		t.Fatal("ReleaseTargets(.goreleaser.yaml) returned no target, so the probe below would run on nothing")
	}

	rules := rulesOf(t, golangci, sdkRule)

	root, err := filepath.Abs(filepath.FromSlash("testdata/adapterprobe"))
	if err != nil {
		t.Fatalf("resolve the probe module: %v", err)
	}

	const (
		probeModule = "example.com/adapterprobe"
		probePkg    = probeModule + "/internal/adapter/source/probe"
	)

	for _, target := range targets {
		t.Run(target.GOOS+"/"+target.GOARCH, func(t *testing.T) {
			t.Parallel()

			pkgs, err := check.ListPackages(t.Context(), root, target, false, nil, "./internal/adapter/source/...")
			if err != nil {
				t.Fatalf("ListPackages(adapterprobe, %v): %v", target, err)
			}

			if !slices.ContainsFunc(pkgs, func(pkg check.Package) bool { return pkg.ImportPath == probePkg }) {
				t.Fatalf("ListPackages(adapterprobe, %v) did not list %s, so the check below would read nothing of the probe", target, probePkg)
			}

			messages := check.Packages(root, probeModule, rules, pkgs)

			if len(messages) != 1 {
				t.Fatalf("Packages(adapterprobe, %v) flagged %d package(s), want exactly the probe's net/http import:\n%s",
					target, len(messages), strings.Join(messages, "\n"))
			}

			for _, want := range []string{probePkg + ": ", "net/http", sdkRule} {
				if !strings.Contains(messages[0], want) {
					t.Errorf("message %q does not name %q", messages[0], want)
				}
			}
		})
	}
}

// TestCloudAdapterSDKRuleHoldsItsSDKProbeForEveryReleaseTarget pins the other
// half of issue #79's SDK rule: an adapter that reaches its cloud through the
// SDK, as the rule requires, must pass, because depguard lints the adapter's
// files and never the SDK's. go list -deps lists the SDK package the adapter
// imports and every package that package imports, so the package check sees a
// third-party package under the rule's prefix entry and one that no entry names,
// and must report neither: otherwise every adapter fails the rule the day it
// imports its cloud's SDK.
//
// The fixture's SDK is two stand-in modules the go.mod replaces with local
// directories, so the listing needs no network and no go.sum entry.
func TestCloudAdapterSDKRuleHoldsItsSDKProbeForEveryReleaseTarget(t *testing.T) {
	t.Parallel()

	golangci := readRepositoryFile(t, ".golangci.yml")

	targets, err := check.ReleaseTargets(readRepositoryFile(t, ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("ReleaseTargets(.goreleaser.yaml): %v", err)
	}

	if len(targets) == 0 {
		t.Fatal("ReleaseTargets(.goreleaser.yaml) returned no target, so the probe below would run on nothing")
	}

	rules := rulesOf(t, golangci, sdkRule)

	root, err := filepath.Abs(filepath.FromSlash("testdata/sdkprobe"))
	if err != nil {
		t.Fatalf("resolve the probe module: %v", err)
	}

	const (
		probeModule = "example.com/sdkprobe"
		probePkg    = probeModule + "/internal/adapter/source/aws"
		sdkPkg      = "github.com/aws/aws-sdk-go-v2/service/ec2"
		sdkDepPkg   = "github.com/aws/smithy-go"
	)

	for _, target := range targets {
		t.Run(target.GOOS+"/"+target.GOARCH, func(t *testing.T) {
			t.Parallel()

			pkgs, err := check.ListPackages(t.Context(), root, target, false, nil, "./internal/adapter/source/...")
			if err != nil {
				t.Fatalf("ListPackages(sdkprobe, %v): %v", target, err)
			}

			// The probe, its SDK package and the package that one imports are
			// all listed: a run that read nothing of them could not pass this.
			for _, want := range []string{probePkg, sdkPkg, sdkDepPkg} {
				if !slices.ContainsFunc(pkgs, func(pkg check.Package) bool { return pkg.ImportPath == want }) {
					t.Fatalf("ListPackages(sdkprobe, %v) did not list %s, so the check below would read nothing of it", target, want)
				}
			}

			if messages := check.Packages(root, probeModule, rules, pkgs); len(messages) != 0 {
				t.Errorf("Packages(sdkprobe, %v) flagged %d package(s), want none: the probe reaches its cloud through the SDK the rule allows, and depguard lints no SDK file:\n%s",
					target, len(messages), strings.Join(messages, "\n"))
			}
		})
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

// sortedCopy returns a sorted copy of values, so a comparison of two sets from
// .golangci.yml does not depend on the order its lists are written in.
func sortedCopy(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)

	return out
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

package architecture_test

import (
	"testing"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// TestGolangciSettingsHideNoIssue runs check.HidingSettings against the real
// .golangci.yml, so a setting that hides issues without touching a rule fails
// here, in the repository, and not only in the unit tests of the check (issue
// #81). check.HidingSettings' doc comment is the contract; in short it refuses
// a document that is not version "2", a key of the shape it does not know
// (which fails closed), a run.tests that is not true, a run.issues-exit-code
// outside Go's portable 1..125, a linters.default other than standard or all,
// an issues cap, filter or fix that reports fewer issues than lint finds, a
// linters.exclusions.generated that is missing or not disable, a non-empty
// linters.exclusions.paths or paths-except, and every way of turning depguard
// or forbidigo off or keeping it from a file: missing from linters.enable,
// listed in linters.disable, or covered by an exclusion rule.
//
// Today's config must pass. Change it and this test says which setting now
// hides what.
func TestGolangciSettingsHideNoIssue(t *testing.T) {
	t.Parallel()

	problems, err := check.HidingSettings(readRepositoryFile(t, ".golangci.yml"))
	if err != nil {
		t.Fatalf("HidingSettings(.golangci.yml): %v", err)
	}

	for _, problem := range problems {
		t.Error(problem)
	}
}

// TestRepositoryReadsOnlyGolangciYML pins that `make lint` reads the
// .golangci.yml the check above reads. golangci-lint looks its config up
// through viper in the order .golangci.json, .golangci.toml, .golangci.yaml,
// .golangci.yml, and takes the first it finds, so another of those documents at
// the root, which git could commit, would run in place of this one: forbidigo
// off, and an exit code 0 with every issue hidden (issue #81).
func TestRepositoryReadsOnlyGolangciYML(t *testing.T) {
	t.Parallel()

	stray, err := check.StrayLintConfigs(t.Context(), repositoryRoot(t))
	if err != nil {
		t.Fatalf("StrayLintConfigs(): %v", err)
	}

	for _, message := range stray {
		t.Error(message)
	}
}

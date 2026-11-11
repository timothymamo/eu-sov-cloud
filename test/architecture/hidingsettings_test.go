package architecture_test

import (
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// TestGolangciSettingsHideNoIssue runs check.HidingSettings against the real
// .golangci.yml, so a setting that hides issues without touching a rule fails
// here, in the repository, and not only in the unit tests of the check (issue
// #81): linters.exclusions.paths and paths-except, the issues caps and filters
// (max-issues-per-linter, max-same-issues, new, new-from-rev, new-from-patch,
// new-from-merge-base, whole-files and any issues key the check does not know),
// linters.default: none, run.tests: false, and every way of turning off
// depguard or forbidigo, the two linters the architecture depends on: missing
// from linters.enable, listed in linters.disable, or kept from the files it
// must see by an exclusion rule.
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

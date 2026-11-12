package architecture_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// shellcheckPin matches one line of a shellcheck pin, `<key>: "<value>"` whose
// key starts with shellcheck in any case, at any indent: dev/lima.yaml's params
// and ci.yml's lint-shell env: alike.
var shellcheckPin = regexp.MustCompile(`(?m)^\s*((?i:shellcheck)[A-Za-z0-9_]*):\s*"([^"]*)"\s*$`)

// shellcheckDigestVersion matches the digestVersion of a shellcheck digest's
// renovate comment.
var shellcheckDigestVersion = regexp.MustCompile(`(?m)^\s*# renovate: datasource=github-release-attachments depName=koalaman/shellcheck digestVersion=(\S+)\s*$`)

// TestShellcheckPinsAgree holds the shellcheck the Lima VM installs and the one
// CI's lint-shell job installs to one release (issue #164, ADR 0022): the same
// version, CI's digest the VM's amd64 digest, as CI runs on x86_64, and every
// digest's renovate digestVersion the version itself. make lint must find the
// same findings in the VM as the Lint check does in CI.
func TestShellcheckPinsAgree(t *testing.T) {
	t.Parallel()

	read := func(rel string) string {
		t.Helper()

		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}

		return string(data)
	}

	pins := func(rel, doc string) map[string]string {
		t.Helper()

		found := map[string]string{}

		for _, m := range shellcheckPin.FindAllStringSubmatch(doc, -1) {
			if _, ok := found[m[1]]; ok {
				t.Errorf("%s pins %s twice", rel, m[1])
			}

			found[m[1]] = m[2]
		}

		return found
	}

	lima := read("dev/lima.yaml")
	ci := read(".github/workflows/ci.yml")
	limaPins := pins("dev/lima.yaml", lima)
	ciPins := pins(".github/workflows/ci.yml", ci)

	want := func(rel string, found map[string]string, key string) string {
		t.Helper()

		value, ok := found[key]
		if !ok || value == "" {
			t.Fatalf("%s pins no %s", rel, key)
		}

		return value
	}

	version := want("dev/lima.yaml", limaPins, "shellcheckVersion")
	amd64 := want("dev/lima.yaml", limaPins, "shellcheckSha256Amd64")
	want("dev/lima.yaml", limaPins, "shellcheckSha256Arm64")

	if got := want(".github/workflows/ci.yml", ciPins, "SHELLCHECK_VERSION"); got != version {
		t.Errorf("ci.yml's SHELLCHECK_VERSION is %q, dev/lima.yaml's shellcheckVersion %q: CI and the VM must run one shellcheck", got, version)
	}

	if got := want(".github/workflows/ci.yml", ciPins, "SHELLCHECK_SHA256"); got != amd64 {
		t.Errorf("ci.yml's SHELLCHECK_SHA256 is %q, dev/lima.yaml's shellcheckSha256Amd64 %q: CI runs on x86_64, so they must be one digest", got, amd64)
	}

	for _, file := range []struct {
		rel, doc string
		digests  int
	}{
		{"dev/lima.yaml", lima, 2},
		{".github/workflows/ci.yml", ci, 1},
	} {
		matches := shellcheckDigestVersion.FindAllStringSubmatch(file.doc, -1)
		if len(matches) != file.digests {
			t.Errorf("%s holds %d shellcheck digest renovate comments, want %d, one above each digest", file.rel, len(matches), file.digests)
		}

		for _, m := range matches {
			if m[1] != version {
				t.Errorf("%s: a shellcheck digest's renovate comment has digestVersion=%s, want the pinned version %s", file.rel, m[1], version)
			}
		}
	}
}

package check_test

import (
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// releasePleaseConfig is release-please-config.json as
// check.ReleasePleaseConfig wants it (issue #142): the module's one Go package,
// at the repository root, whose version is computed with the release-tag
// skill's rules. A breaking change is minor while the module is v0.x (Godoc
// still calls it unstable), a feat is the minor it always was, and every tag is
// exactly vX.Y.Z: the schema's keys, the release-type and the package-name are
// free, but no component prefix may reach the tag, because the Release tags
// ruleset, release.yml's identity and the #99 verification job all name
// vX.Y.Z. release-please creates the tag and the release PR alone: the GitHub
// Release, its artifacts, the images, their signatures and their attestations
// stay GoReleaser's, so skip-github-release is on.
const releasePleaseConfig = `{
  "$schema": "https://raw.githubusercontent.com/googleapis/release-please/main/schemas/config.json",
  "release-type": "go",
  "bump-minor-pre-major": true,
  "bump-patch-for-minor-pre-major": false,
  "include-v-in-tag": true,
  "include-component-in-tag": false,
  "skip-github-release": true,
  "packages": {
    ".": {
      "package-name": "shift2eu"
    }
  }
}
`

// TestReleasePleaseConfig pins issue #142's release-please configuration: the
// rules the release-tag skill applied by hand, now read by release-please from
// a committed document. Each case mutates releasePleaseConfig once and names
// the setting a message must report.
func TestReleasePleaseConfig(t *testing.T) {
	t.Parallel()

	// inPackage inserts line into the "." package entry, after its
	// package-name.
	inPackage := func(line string) string {
		return mutate(t, releasePleaseConfig, `      "package-name": "shift2eu"`, `      "package-name": "shift2eu",`+"\n      "+line)
	}

	tests := []struct {
		name   string
		config string
		// wantIn is a part of the message the check must return; "" means none
		// at all.
		wantIn string
	}{
		{name: "the accepted config", config: releasePleaseConfig},

		// A breaking change is minor on v0.x, which bump-minor-pre-major
		// alone decides: without it, a breaking change waits for v1 and no
		// minor is ever released from v0.x.
		{
			name:   "no bump-minor-pre-major",
			config: mutate(t, releasePleaseConfig, "  \"bump-minor-pre-major\": true,\n", ""),
			wantIn: "bump-minor-pre-major",
		},
		{
			name:   "bump-minor-pre-major: false",
			config: mutate(t, releasePleaseConfig, `"bump-minor-pre-major": true`, `"bump-minor-pre-major": false`),
			wantIn: "bump-minor-pre-major",
		},

		// A feat is the minor it always was: bump-patch-for-minor-pre-major
		// would make it a patch.
		{
			name:   "no bump-patch-for-minor-pre-major",
			config: mutate(t, releasePleaseConfig, "  \"bump-patch-for-minor-pre-major\": false,\n", ""),
			wantIn: "bump-patch-for-minor-pre-major",
		},
		{
			name:   "bump-patch-for-minor-pre-major: true",
			config: mutate(t, releasePleaseConfig, `"bump-patch-for-minor-pre-major": false`, `"bump-patch-for-minor-pre-major": true`),
			wantIn: "bump-patch-for-minor-pre-major",
		},

		// Every tag is exactly vX.Y.Z: the v is included and no component
		// prefix is.
		{
			name:   "no include-v-in-tag",
			config: mutate(t, releasePleaseConfig, "  \"include-v-in-tag\": true,\n", ""),
			wantIn: "include-v-in-tag",
		},
		{
			name:   "include-v-in-tag: false",
			config: mutate(t, releasePleaseConfig, `"include-v-in-tag": true`, `"include-v-in-tag": false`),
			wantIn: "include-v-in-tag",
		},
		{
			name:   "no include-component-in-tag",
			config: mutate(t, releasePleaseConfig, "  \"include-component-in-tag\": false,\n", ""),
			wantIn: "include-component-in-tag",
		},
		{
			name:   "include-component-in-tag: true",
			config: mutate(t, releasePleaseConfig, `"include-component-in-tag": false`, `"include-component-in-tag": true`),
			wantIn: "include-component-in-tag",
		},
		{
			// A package entry's own settings override the top level's, so the
			// entry may not put a component back into the tag.
			name:   "a package entry that includes the component in the tag",
			config: inPackage(`"include-component-in-tag": true`),
			wantIn: "include-component-in-tag",
		},

		// GoReleaser creates the GitHub Release, so release-please must not.
		{
			name:   "no skip-github-release",
			config: mutate(t, releasePleaseConfig, "  \"skip-github-release\": true,\n", ""),
			wantIn: "skip-github-release",
		},
		{
			name:   "skip-github-release: false",
			config: mutate(t, releasePleaseConfig, `"skip-github-release": true`, `"skip-github-release": false`),
			wantIn: "skip-github-release",
		},
		{
			name:   "a package entry that creates the GitHub Release",
			config: inPackage(`"skip-github-release": false`),
			wantIn: "skip-github-release",
		},
		{
			// A package entry that makes a breaking change a patch overrides
			// the top level's bump-minor-pre-major.
			name:   "a package entry that bumps a breaking change as a patch",
			config: inPackage(`"bump-minor-pre-major": false`),
			wantIn: "bump-minor-pre-major",
		},

		// One package, at the repository root: the version and the tag belong
		// to the module, and there is no second component to tag.
		{
			name:   "no packages",
			config: mutate(t, releasePleaseConfig, "  \"skip-github-release\": true,\n  \"packages\": {\n    \".\": {\n      \"package-name\": \"shift2eu\"\n    }\n  }\n", "  \"skip-github-release\": true\n"),
			wantIn: "packages",
		},
		{
			name:   "two packages",
			config: mutate(t, releasePleaseConfig, "    \".\": {\n      \"package-name\": \"shift2eu\"\n    }", "    \".\": {\n      \"package-name\": \"shift2eu\"\n    },\n    \"server\": {\n      \"package-name\": \"shift2eu-server\"\n    }"),
			wantIn: "packages",
		},
		{
			name:   "a package that is not the root",
			config: mutate(t, releasePleaseConfig, "    \".\": {", "    \"cmd\": {"),
			wantIn: "packages",
		},

		// A document the check cannot read tells it nothing, so it is refused
		// rather than passed.
		{name: "an empty document", config: "", wantIn: "empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := check.ReleasePleaseConfig([]byte(tc.config))
			joined := strings.Join(got, "\n")

			if tc.wantIn == "" {
				if len(got) != 0 {
					t.Errorf("ReleasePleaseConfig() = %q, want no message", got)
				}

				return
			}

			if !strings.Contains(joined, tc.wantIn) {
				t.Errorf("ReleasePleaseConfig() = %q, want a message containing %q", got, tc.wantIn)
			}
		})
	}
}

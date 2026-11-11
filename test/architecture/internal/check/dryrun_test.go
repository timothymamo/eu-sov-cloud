package check_test

import (
	"os"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// ciDryRun is the image job of .github/workflows/ci.yml as far as the dry run
// checks it against release.yml: the same BuildKit, the same action pins, the
// release's tag rules with a sample tag in place of the pushed one, the
// release's build settings written to an OCI archive, and the SBOM of the built
// image.
const ciDryRun = `name: CI

on:
  pull_request:

jobs:
  image:
    name: Container image
    runs-on: ubuntu-latest
    env:
      BUILDKIT_VERSION: v0.34.2@sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe
    steps:
      - uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069 # v4.4.1
        with:
          driver-opts: image=moby/buildkit:${{ env.BUILDKIT_VERSION }}
      - name: Image tags for a sample release tag, the highest
        uses: docker/metadata-action@dc802804100637a589fabce1cb79ff13a1411302 # v6.2.0
        with:
          images: ghcr.io/timothymamo/eusov
          flavor: latest=${{ steps.latest.outputs.highest }}
          tags: |
            type=semver,pattern={{raw}},value=v1.2.3
            type=semver,pattern={{major}}.{{minor}},value=v1.2.3,enable=${{ !contains('v1.2.3', '-') }}
      - name: Build linux/amd64 and linux/arm64
        uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: false
          outputs: type=oci,dest=${{ runner.temp }}/eusov.oci.tar
          annotations: index:org.opencontainers.image.source=https://github.com/timothymamo/eu-sov-cloud
          provenance: false
          sbom: false
      - name: Load linux/amd64 for the smoke test
        uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        with:
          context: .
          platforms: linux/amd64
          load: true
      - name: SBOM of the built image
        uses: anchore/sbom-action@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c # v0.24.3
        with:
          image: oci-archive:${{ runner.temp }}/eusov.oci.tar
          format: spdx-json
          output-file: ${{ runner.temp }}/eusov.spdx.json
          syft-version: v1.57.1
`

// releaseDryRun is the image and sbom jobs of .github/workflows/release.yml as
// far as the dry run checks them: the BuildKit image by exact release and index
// digest, the pins, the tag rules on github.ref_name, the build settings, and
// the SPDX SBOM with an exact syft.
const releaseDryRun = `name: Release

on:
  push:
    tags: ["v*.*.*"]

jobs:
  image:
    name: Container image
    runs-on: ubuntu-latest
    steps:
      - uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069 # v4.4.1
        with:
          driver-opts: image=moby/buildkit:v0.34.2@sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe
      - name: Image tags
        uses: docker/metadata-action@dc802804100637a589fabce1cb79ff13a1411302 # v6.2.0
        with:
          images: ghcr.io/timothymamo/eusov
          flavor: latest=${{ steps.stamp.outputs.latest }}
          tags: |
            type=semver,pattern={{raw}}
            type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}
      - name: Build and push linux/amd64 and linux/arm64
        uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: true
          annotations: index:org.opencontainers.image.source=https://github.com/timothymamo/eu-sov-cloud
          provenance: false
          sbom: false
  sbom:
    name: SBOM
    runs-on: ubuntu-latest
    steps:
      - uses: anchore/sbom-action@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c # v0.24.3
        with:
          image: registry:ghcr.io/timothymamo/eusov@${{ needs.image.outputs.digest }}
          format: spdx-json
          output-file: eusov.spdx.json
          syft-version: v1.57.1
`

// mutate returns doc with old replaced by new, failing unless doc holds old
// exactly once, so that a case that mistypes the fixture cannot pass for the
// wrong reason.
func mutate(t *testing.T, doc, old, new string) string {
	t.Helper()

	if got := strings.Count(doc, old); got != 1 {
		t.Fatalf("the fixture holds %q %d times, want exactly once", old, got)
	}

	return strings.Replace(doc, old, new, 1)
}

// TestDryRunMirrorsRelease pins the review's finding of issue #72: ci.yml's
// image job is the dry run of release.yml's image and sbom jobs, so it must
// fail the pull request when the release's build settings drift from it. What
// the two documents publish differ by design (push: true against an OCI
// archive, a real tag against a sample one, a registry image against an
// archive), so the check compares the fields the release's build and SBOM rest
// on.
func TestDryRunMirrorsRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ci      string
		release string
		// wantIn is a part of the message the check must return; "" means
		// none at all.
		wantIn string
	}{
		{name: "the accepted pair", ci: ciDryRun, release: releaseDryRun},
		{
			// The same pair with CRLF line ends, which the reader tolerates.
			name:    "the accepted pair with CRLF line ends",
			ci:      strings.ReplaceAll(ciDryRun, "\n", "\r\n"),
			release: strings.ReplaceAll(releaseDryRun, "\n", "\r\n"),
		},

		// The build: the release's platforms, annotations and BuildKit's own
		// provenance and SBOM settings, compared on the step that builds what
		// the release publishes (not the ci's second, single-platform load for
		// the smoke test).
		{name: "a drifted platform", ci: mutate(t, ciDryRun, "          platforms: linux/amd64,linux/arm64\n", "          platforms: linux/amd64\n"), release: releaseDryRun, wantIn: `with: platforms is "linux/amd64", release.yml's`},
		{name: "a drifted annotation", ci: mutate(t, ciDryRun, "https://github.com/timothymamo/eu-sov-cloud", "https://example.com"), release: releaseDryRun, wantIn: "with: annotations is"},
		{name: "a drifted provenance", ci: mutate(t, ciDryRun, "          provenance: false\n", "          provenance: true\n"), release: releaseDryRun, wantIn: `with: provenance is "true"`},
		{name: "a drifted sbom setting", ci: mutate(t, ciDryRun, "          sbom: false\n", "          sbom: true\n"), release: releaseDryRun, wantIn: `with: sbom is "true"`},

		// The tags: the release's rules with a sample tag in place of
		// github.ref_name, and the same image name.
		{
			name:    "a drifted tag rule",
			ci:      mutate(t, ciDryRun, "type=semver,pattern={{major}}.{{minor}},value=v1.2.3", "type=semver,pattern={{major}},value=v1.2.3"),
			release: releaseDryRun,
			wantIn:  "with: tags are",
		},
		{
			name:    "a tag rule the release does not have",
			ci:      mutate(t, ciDryRun, "            type=semver,pattern={{raw}},value=v1.2.3\n", "            type=semver,pattern={{raw}},value=v1.2.3\n            type=ref,event=branch\n"),
			release: releaseDryRun,
			wantIn:  "with: tags are",
		},
		{name: "a drifted image name", ci: mutate(t, ciDryRun, "          images: ghcr.io/timothymamo/eusov\n", "          images: ghcr.io/example/eusov\n"), release: releaseDryRun, wantIn: `with: images is "ghcr.io/example/eusov"`},

		// The SBOM: the release's format and exact syft.
		{name: "a drifted syft version", ci: mutate(t, ciDryRun, "syft-version: v1.57.1", "syft-version: v1.58.0"), release: releaseDryRun, wantIn: `with: syft-version is "v1.58.0"`},
		{name: "a drifted SBOM format", ci: mutate(t, ciDryRun, "          format: spdx-json\n", "          format: cyclonedx-json\n"), release: releaseDryRun, wantIn: `with: format is "cyclonedx-json"`},

		// The pins: the same commit for every action the two share, and the
		// same BuildKit image by tag and digest.
		{name: "a drifted metadata-action pin", ci: mutate(t, ciDryRun, "docker/metadata-action@dc802804100637a589fabce1cb79ff13a1411302", "docker/metadata-action@dc802804100637a589fabce1cb79ff13a1411303"), release: releaseDryRun, wantIn: "docker/metadata-action runs at"},
		{name: "a drifted build-push-action pin", ci: ciDryRun, release: mutate(t, releaseDryRun, "docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc", "docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dd"), wantIn: "docker/build-push-action runs at c3c9e263c25d99ce0380d002d59b67737d91b0dc (line 24) and at c3c9e263c25d99ce0380d002d59b67737d91b0dd"},
		{name: "a drifted sbom-action pin", ci: mutate(t, ciDryRun, "anchore/sbom-action@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c", "anchore/sbom-action@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6d"), release: releaseDryRun, wantIn: "anchore/sbom-action runs at"},
		{name: "a drifted setup-buildx-action pin", ci: ciDryRun, release: mutate(t, releaseDryRun, "docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069", "docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497070"), wantIn: "docker/setup-buildx-action runs at"},
		{name: "a drifted BuildKit image", ci: mutate(t, ciDryRun, "v0.34.2@sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe", "v0.35.0@sha256:1111111111111111111111111111111111111111111111111111111111111111"), release: releaseDryRun, wantIn: "the dry run must build with the release's moby/buildkit tag and digest"},

		// The one build the dry run compares is the one that does not load the
		// image: none left, or a second one, is refused rather than guessed at.
		{
			name:    "a multi-platform build that loads the image too",
			ci:      mutate(t, ciDryRun, "          outputs: type=oci,dest=${{ runner.temp }}/eusov.oci.tar\n", "          load: true\n          outputs: type=oci,dest=${{ runner.temp }}/eusov.oci.tar\n"),
			release: releaseDryRun,
			wantIn:  "load: true left out, holds 0 docker/build-push-action steps, want exactly 1",
		},
		{
			name:    "a second build that does not load the image",
			ci:      mutate(t, ciDryRun, "      - name: Load linux/amd64 for the smoke test\n", "      - name: Build once more\n        uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0\n        with:\n          context: .\n          platforms: linux/amd64,linux/arm64\n          push: false\n          outputs: type=oci,dest=${{ runner.temp }}/eusov-2.oci.tar\n      - name: Load linux/amd64 for the smoke test\n"),
			release: releaseDryRun,
			wantIn:  "load: true left out, holds 2 docker/build-push-action steps, want exactly 1",
		},
		{
			// The load: true step is the dry run's own smoke test, not part of
			// what the release publishes: leaving it out leaves the one build.
			name:    "the dry run without its smoke-test load",
			ci:      mutate(t, ciDryRun, "      - name: Load linux/amd64 for the smoke test\n        uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0\n        with:\n          context: .\n          platforms: linux/amd64\n          load: true\n", ""),
			release: releaseDryRun,
		},

		// The sample tag the dry run's tag rules must carry for the release's
		// github.ref_name rules to be compared at all.
		{
			name:    "a metadata step without a sample tag",
			ci:      mutate(t, ciDryRun, "            type=semver,pattern={{raw}},value=v1.2.3\n            type=semver,pattern={{major}}.{{minor}},value=v1.2.3,enable=${{ !contains('v1.2.3', '-') }}\n", "            type=semver,pattern={{raw}}\n            type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}\n"),
			release: releaseDryRun,
			wantIn:  "holds no ,value=<sample>",
		},
		{
			name:    "a metadata step with an empty sample tag",
			ci:      mutate(t, ciDryRun, "            type=semver,pattern={{raw}},value=v1.2.3\n", "            type=semver,pattern={{raw}},value=\n"),
			release: releaseDryRun,
			wantIn:  "holds an empty ,value=",
		},
		{
			name:    "a dry run without the metadata-action step",
			ci:      mutate(t, ciDryRun, "      - name: Image tags for a sample release tag, the highest\n        uses: docker/metadata-action@dc802804100637a589fabce1cb79ff13a1411302 # v6.2.0\n        with:\n          images: ghcr.io/timothymamo/eusov\n          flavor: latest=${{ steps.latest.outputs.highest }}\n          tags: |\n            type=semver,pattern={{raw}},value=v1.2.3\n            type=semver,pattern={{major}}.{{minor}},value=v1.2.3,enable=${{ !contains('v1.2.3', '-') }}\n", ""),
			release: releaseDryRun,
			wantIn:  "so the dry run checks none of the release's tags",
		},

		// The with: keys the build and the SBOM are compared on, and the steps
		// themselves: a key or a step the dry run does not have is named.
		{name: "a build step without platforms", ci: mutate(t, ciDryRun, "          platforms: linux/amd64,linux/arm64\n          push: false\n", "          push: false\n"), release: releaseDryRun, wantIn: "holds no with: platforms"},
		{name: "a metadata step without images", ci: mutate(t, ciDryRun, "          images: ghcr.io/timothymamo/eusov\n", ""), release: releaseDryRun, wantIn: "holds no with: images"},

		// The BuildKit image comes from the job's env:, which the check resolves
		// and refuses when it cannot.
		{
			name:    "a BuildKit expression that is not an env variable",
			ci:      mutate(t, ciDryRun, "          driver-opts: image=moby/buildkit:${{ env.BUILDKIT_VERSION }}\n", "          driver-opts: image=moby/buildkit:${{ github.sha }}\n"),
			release: releaseDryRun,
			wantIn:  `the expression "github.sha" is not an env. variable`,
		},
		{
			name:    "a BuildKit expression the job env does not hold",
			ci:      mutate(t, ciDryRun, "${{ env.BUILDKIT_VERSION }}", "${{ env.BUILDKIT }}"),
			release: releaseDryRun,
			wantIn:  "the job's env: holds no BUILDKIT",
		},
		{
			name:    "a job without the env the BuildKit image is in",
			ci:      mutate(t, ciDryRun, "    env:\n      BUILDKIT_VERSION: v0.34.2@sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe\n", ""),
			release: releaseDryRun,
			wantIn:  "the job's env: holds no BUILDKIT_VERSION",
		},
		{name: "an env variable whose name the reader does not follow", ci: mutate(t, ciDryRun, "      BUILDKIT_VERSION:", "      buildkit_version:"), release: releaseDryRun, wantIn: "is not one variable this reader understands"},

		// The jobs the two documents must hold to be compared at all.
		{name: "a ci.yml without an image job", ci: mutate(t, ciDryRun, "jobs:\n  image:\n", "jobs:\n  images:\n"), release: releaseDryRun, wantIn: "ci.yml: no jobs: image"},
		{name: "a release.yml without an sbom job", ci: ciDryRun, release: mutate(t, releaseDryRun, "  sbom:\n", "  provenance:\n"), wantIn: "release.yml: no jobs: sbom"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := check.DryRunMirrorsRelease([]byte(tc.ci), []byte(tc.release))

			if tc.wantIn == "" {
				if len(got) != 0 {
					t.Errorf("DryRunMirrorsRelease() = %q, want no message", got)
				}

				return
			}

			if !strings.Contains(strings.Join(got, "\n"), tc.wantIn) {
				t.Errorf("DryRunMirrorsRelease() = %q, want a message containing %q", got, tc.wantIn)
			}
		})
	}
}

// TestDryRunMirrorsTheRepositoryWorkflows pins the same pair the repository
// runs: the real ci.yml and release.yml must not have drifted apart.
func TestDryRunMirrorsTheRepositoryWorkflows(t *testing.T) {
	t.Parallel()

	ci, err := os.ReadFile("../../../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}

	release, err := os.ReadFile("../../../../.github/workflows/release.yml")
	if err != nil {
		t.Fatalf("read release.yml: %v", err)
	}

	for _, message := range check.DryRunMirrorsRelease(ci, release) {
		t.Error(message)
	}
}

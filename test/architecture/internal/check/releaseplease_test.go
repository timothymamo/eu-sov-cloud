package check_test

import (
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// releasePleaseWorkflowTop is .github/workflows/release-please.yml as
// check.ReleasePleaseWorkflow wants it (issue #142), up to the two steps that
// create the tag: one job, on a push to main alone, in the release-please
// environment, which holds the release App's ID and key and is limited to
// main. GITHUB_TOKEN holds nothing that writes: the writing is done by the
// App's short-lived token, minted by actions/create-github-app-token with
// exactly Contents and Pull requests write, and handed to the release-please
// step, which keeps one release PR open on main. The job's own permissions are
// reads: contents to read the repository, actions to read release.yml's runs.
// The concurrency group is this workflow's own, never the one release.yml
// uses, so that a later push to main can never cancel a release that is
// waiting to run. Every action is pinned by commit, and nothing here
// publishes: release.yml still owns GoReleaser, the two images, their
// signatures and their attestations.
const releasePleaseWorkflowTop = `name: Release please

# A push to main alone proposes a version: the tag is created after the release
# PR merges, never on a push of a tag.
on:
  push:
    branches: [main]

# Nothing through GITHUB_TOKEN writes: the App's token does the writing.
permissions: {}

# A group of its own, never the group release.yml uses. GitHub keeps one
# pending run per group and cancels the older pending one when a new run
# queues, so sharing that group would let a later push to main cancel a release
# that is waiting to run, and the release would be lost. A run in progress is
# never cancelled (ADR 0015).
concurrency:
  group: release-please
  cancel-in-progress: false

jobs:
  release-please:
    name: Release please
    runs-on: ubuntu-latest
    # The App's ID and key live in this environment, limited to main.
    environment: release-please
    permissions:
      contents: read
      actions: read
    steps:
      - name: Mint the release App's token
        id: app
        uses: actions/create-github-app-token@a1b2c3d4e5f60718293a4b5c6d7e8f9012345678 # v1.11.3
        with:
          app-id: ${{ vars.RELEASE_APP_ID }}
          private-key: ${{ secrets.RELEASE_APP_PRIVATE_KEY }}
          permission-contents: write
          permission-pull-requests: write
      - uses: googleapis/release-please-action@0123456789abcdef0123456789abcdef01234567 # v4.1.3
        with:
          token: ${{ steps.app.outputs.token }}
`

// releasePleaseGuardStep refuses to create the tag while release.yml has a run
// queued or in progress. With a group of its own this workflow is never
// cancelled by a release, so it waits instead of losing one: a tag pushed at a
// release.yml that GitHub then cancels publishes nothing. It reads the runs
// with GITHUB_TOKEN, whose actions: read the job grants.
const releasePleaseGuardStep = `      - name: Wait for no release to be queued or in progress
        id: guard
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          for status in queued in_progress; do
            runs=$(gh api "repos/${{ github.repository }}/actions/workflows/release.yml/runs?status=${status}" --jq .total_count)
            if [ "${runs}" != "0" ]; then
              echo "a release run is ${status}: nothing is tagged" >&2
              exit 1
            fi
          done
`

// releasePleaseTagStep creates the vX.Y.Z tag, which release-please does not:
// with skip-github-release it creates no GitHub Release, and it creates a tag
// only by creating one. So the tag is created here, by the same App, through
// the API, on the merge commit of the merged release PR still labelled
// autorelease: pending, and that PR is relabelled autorelease: tagged so it is
// never tagged twice. A tag created with GITHUB_TOKEN starts no workflow, and
// the Release tags ruleset refuses it; a tag created with git push is not the
// App's either (ADR 0015).
const releasePleaseTagStep = `      - name: Tag the merged release PR
        id: tag
        env:
          GH_TOKEN: ${{ steps.app.outputs.token }}
        run: |
          label="autorelease: pending"
          pr=$(gh api "repos/${{ github.repository }}/pulls?state=closed&per_page=20" --jq ".[] | select(.merged_at != null and any(.labels[]; .name == \"${label}\")) | .number")
          test -n "${pr}" || exit 0
          merge=$(gh api "repos/${{ github.repository }}/pulls/${pr}" --jq .merge_commit_sha)
          tag=$(gh api "repos/${{ github.repository }}/pulls/${pr}" --jq .title | sed -n "s/^chore(main): release \(v[0-9][0-9.]*\)$/\1/p")
          gh api --method POST "repos/${{ github.repository }}/git/refs" -f ref="refs/tags/${tag}" -f sha="${merge}"
          gh api --method DELETE "repos/${{ github.repository }}/issues/${pr}/labels/${label}"
          gh api --method POST "repos/${{ github.repository }}/issues/${pr}/labels" -f "labels[]=autorelease: tagged"
`

// releasePleaseWorkflow is the whole file: the release-please step, then the
// wait for a release in flight, then the step that creates the tag.
const releasePleaseWorkflow = releasePleaseWorkflowTop + releasePleaseGuardStep + releasePleaseTagStep

// TestReleasePleaseWorkflow pins issue #142's workflow: the one that proposes
// the next version in a release PR on main and, when that PR merges, creates
// the vX.Y.Z tag with a GitHub App token, so that release.yml publishes exactly
// as it does today. The tag is created by a step of its own through the API,
// and only while release.yml has no run queued or in progress, in a concurrency
// group of its own. The check returns one message per problem and none for
// releasePleaseWorkflow, whose every part each case mutates once.
func TestReleasePleaseWorkflow(t *testing.T) {
	t.Parallel()

	// beforeReleasePlease returns the workflow with step inserted before the
	// release-please step, as a caller writes a step in the job.
	beforeReleasePlease := func(step string) string {
		return mutate(t, releasePleaseWorkflow, "      - uses: googleapis/release-please-action@", "      - "+step+"\n      - uses: googleapis/release-please-action@")
	}

	tests := []struct {
		name     string
		workflow string
		// wantIn is a part of the message the check must return; "" means
		// none at all.
		wantIn string
	}{
		// The accepted shape, and the reading that must not depend on the
		// line ends.
		{name: "the accepted shape", workflow: releasePleaseWorkflow},
		{name: "CRLF line ends", workflow: strings.ReplaceAll(releasePleaseWorkflow, "\n", "\r\n")},
		{
			// The job's own permissions are reads: the App's token, not
			// GITHUB_TOKEN, writes, and with permissions: {} at the top level a
			// job that declares none holds none, so the wait for a release in
			// flight fails closed and no tag is created. That is not a privilege
			// this check refuses.
			name:     "no job-level permissions",
			workflow: mutate(t, releasePleaseWorkflow, "    permissions:\n      contents: read\n      actions: read\n", ""),
		},

		// The trigger: a push to main alone. Any other trigger, any other
		// branch and any tag runs the workflow where no release PR belongs.
		{
			name:     "a pull_request trigger",
			workflow: mutate(t, releasePleaseWorkflow, "    branches: [main]\n", "    branches: [main]\n  pull_request:\n"),
			wantIn:   "pull_request",
		},
		{
			name:     "a workflow_dispatch trigger",
			workflow: mutate(t, releasePleaseWorkflow, "  push:\n    branches: [main]\n", "  workflow_dispatch:\n  push:\n    branches: [main]\n"),
			wantIn:   "workflow_dispatch",
		},
		{
			name:     "a second branch",
			workflow: mutate(t, releasePleaseWorkflow, "    branches: [main]", "    branches: [main, next]"),
			wantIn:   "branches",
		},
		{
			name:     "a tags trigger",
			workflow: mutate(t, releasePleaseWorkflow, "    branches: [main]\n", "    branches: [main]\n    tags: [\"v*.*.*\"]\n"),
			wantIn:   "tags",
		},
		{
			// on: push with nothing below it runs on a push of any branch and
			// any tag.
			name:     "a bare on: push",
			workflow: mutate(t, releasePleaseWorkflow, "on:\n  push:\n    branches: [main]\n", "on: push\n"),
			wantIn:   "push",
		},

		// Nothing through GITHUB_TOKEN: the App's token writes, so a
		// workflow-level permission is a privilege the workflow does not need.
		{
			name:     "no top-level permissions",
			workflow: mutate(t, releasePleaseWorkflow, "permissions: {}\n", ""),
			wantIn:   "no top-level permissions",
		},
		{
			name:     "a top-level permissions with a scope",
			workflow: mutate(t, releasePleaseWorkflow, "permissions: {}", "permissions:\n  contents: write"),
			wantIn:   "permissions",
		},
		{
			name:     "write-all at the job level",
			workflow: mutate(t, releasePleaseWorkflow, "    permissions:\n      contents: read\n      actions: read\n", "    permissions: write-all\n"),
			wantIn:   "write-all",
		},
		{
			name:     "contents: write at the job level",
			workflow: mutate(t, releasePleaseWorkflow, "      contents: read\n", "      contents: write\n"),
			wantIn:   "contents",
		},
		{
			name:     "actions: write at the job level",
			workflow: mutate(t, releasePleaseWorkflow, "      actions: read\n", "      actions: write\n"),
			wantIn:   "actions",
		},

		// The App's ID and key are held by an environment of its own, limited
		// to main.
		{
			name:     "no environment",
			workflow: mutate(t, releasePleaseWorkflow, "    environment: release-please\n", ""),
			wantIn:   "environment",
		},
		{
			name:     "another environment",
			workflow: mutate(t, releasePleaseWorkflow, "environment: release-please", "environment: production"),
			wantIn:   "environment",
		},

		// The App token: minted by actions/create-github-app-token from the
		// environment's ID and key, asking for exactly the two permissions the
		// workflow uses.
		{
			name:     "no app-id",
			workflow: mutate(t, releasePleaseWorkflow, "          app-id: ${{ vars.RELEASE_APP_ID }}\n", ""),
			wantIn:   "app-id",
		},
		{
			name:     "an app-id from a secret",
			workflow: mutate(t, releasePleaseWorkflow, "app-id: ${{ vars.RELEASE_APP_ID }}", "app-id: ${{ secrets.RELEASE_APP_ID }}"),
			wantIn:   "app-id",
		},
		{
			name:     "a private key from a variable",
			workflow: mutate(t, releasePleaseWorkflow, "private-key: ${{ secrets.RELEASE_APP_PRIVATE_KEY }}", "private-key: ${{ vars.RELEASE_APP_PRIVATE_KEY }}"),
			wantIn:   "private-key",
		},
		{
			name:     "no private-key",
			workflow: mutate(t, releasePleaseWorkflow, "          private-key: ${{ secrets.RELEASE_APP_PRIVATE_KEY }}\n", ""),
			wantIn:   "private-key",
		},
		{
			name:     "permission-contents: read",
			workflow: mutate(t, releasePleaseWorkflow, "permission-contents: write", "permission-contents: read"),
			wantIn:   "permission-contents",
		},
		{
			name:     "permission-pull-requests: read",
			workflow: mutate(t, releasePleaseWorkflow, "permission-pull-requests: write", "permission-pull-requests: read"),
			wantIn:   "permission-pull-requests",
		},
		{
			name:     "no permission-pull-requests",
			workflow: mutate(t, releasePleaseWorkflow, "          permission-pull-requests: write\n", ""),
			wantIn:   "permission-pull-requests",
		},
		{
			name:     "a third App permission",
			workflow: mutate(t, releasePleaseWorkflow, "          permission-pull-requests: write\n", "          permission-pull-requests: write\n          permission-issues: write\n"),
			wantIn:   "permission-issues",
		},

		// The release-please step reads the App's token, never GITHUB_TOKEN or
		// github.token: a tag pushed with either does not start release.yml,
		// and the ruleset refuses it.
		{
			name:     "the release-please step reading secrets.GITHUB_TOKEN",
			workflow: mutate(t, releasePleaseWorkflow, "token: ${{ steps.app.outputs.token }}", "token: ${{ secrets.GITHUB_TOKEN }}"),
			wantIn:   "GITHUB_TOKEN",
		},
		{
			name:     "the release-please step reading github.token",
			workflow: mutate(t, releasePleaseWorkflow, "token: ${{ steps.app.outputs.token }}", "token: ${{ github.token }}"),
			wantIn:   "github.token",
		},
		{
			name:     "the release-please step reading another step's token",
			workflow: mutate(t, releasePleaseWorkflow, "          token: ${{ steps.app.outputs.token }}", "          token: ${{ steps.mint.outputs.token }}"),
			wantIn:   "steps.mint.outputs.token",
		},
		{
			name:     "no token on the release-please step",
			workflow: mutate(t, releasePleaseWorkflow, "        with:\n          token: ${{ steps.app.outputs.token }}\n", ""),
			wantIn:   "token",
		},

		// The tag itself: with skip-github-release release-please creates no
		// GitHub Release, and it creates a tag only by creating one, so the
		// workflow creates the tag in a step of its own, through the API, with
		// the same App token. A tag created with GITHUB_TOKEN or with git push
		// starts no release run and the ruleset refuses it (ADR 0015).
		{
			name:     "no tagging step",
			workflow: mutate(t, releasePleaseWorkflow, releasePleaseTagStep, ""),
			wantIn:   "tag",
		},
		{
			name:     "the tagging step reading secrets.GITHUB_TOKEN",
			workflow: mutate(t, releasePleaseWorkflow, "          GH_TOKEN: ${{ steps.app.outputs.token }}\n", "          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n"),
			wantIn:   "GITHUB_TOKEN",
		},
		{
			name:     "the tagging step reading github.token",
			workflow: mutate(t, releasePleaseWorkflow, "          GH_TOKEN: ${{ steps.app.outputs.token }}\n", "          GH_TOKEN: ${{ github.token }}\n"),
			wantIn:   "github.token",
		},
		{
			name:     "no GH_TOKEN on the tagging step",
			workflow: mutate(t, releasePleaseWorkflow, "        env:\n          GH_TOKEN: ${{ steps.app.outputs.token }}\n", ""),
			wantIn:   "GH_TOKEN",
		},
		{
			name:     "a step pushing a tag with git push",
			workflow: beforeReleasePlease("name: Push the tag\n        run: git push origin v1.2.3"),
			wantIn:   "git push",
		},

		// One release at a time, and never a tag at a release that is already
		// queued or running: with a group of its own this workflow is never
		// cancelled, so the step that tags waits until release.yml holds no run.
		{
			name:     "no wait for a release in flight",
			workflow: mutate(t, releasePleaseWorkflow, releasePleaseGuardStep, ""),
			wantIn:   "release.yml",
		},
		{
			name:     "a wait that reads another workflow's runs",
			workflow: mutate(t, releasePleaseWorkflow, "actions/workflows/release.yml/runs", "actions/workflows/ci.yml/runs"),
			wantIn:   "release.yml",
		},

		// Every action is pinned by a 40-hex commit digest, which a Renovate
		// update moves and which a tag or branch never is.
		{
			name:     "the App token action pinned by tag",
			workflow: mutate(t, releasePleaseWorkflow, "create-github-app-token@a1b2c3d4e5f60718293a4b5c6d7e8f9012345678", "create-github-app-token@v1"),
			wantIn:   "create-github-app-token@v1",
		},
		{
			name:     "the release-please action pinned by branch",
			workflow: mutate(t, releasePleaseWorkflow, "release-please-action@0123456789abcdef0123456789abcdef01234567", "release-please-action@main"),
			wantIn:   "release-please-action@main",
		},
		{
			name:     "the release-please action pinned by a short commit",
			workflow: mutate(t, releasePleaseWorkflow, "release-please-action@0123456789abcdef0123456789abcdef01234567", "release-please-action@0123456"),
			wantIn:   "release-please-action@0123456",
		},

		// Nothing publishes here: publishing, signing and attesting stay in
		// release.yml, so the signing identity keeps naming that workflow. Each
		// case adds the step to the job and wants it named: the message must say
		// what publishes, not how many steps the job holds.
		{
			name:     "a goreleaser step",
			workflow: beforeReleasePlease("uses: goreleaser/goreleaser-action@a4f02ab5355a589de60d917a659ce617ed19b3de"),
			wantIn:   "goreleaser",
		},
		{
			name:     "a goreleaser run",
			workflow: beforeReleasePlease("name: Release\n        run: goreleaser release --clean"),
			wantIn:   "goreleaser",
		},
		{
			name:     "a cosign step",
			workflow: beforeReleasePlease("uses: sigstore/cosign-installer@1e69f48acb82d1966a394da916b4c1698aa569d6"),
			wantIn:   "cosign",
		},
		{
			name:     "a docker login step",
			workflow: beforeReleasePlease("uses: docker/login-action@8aa5831fb8d48d8f76594c7ddde61ae58019f550"),
			wantIn:   "docker/login-action",
		},
		{
			name:     "a docker build and push step",
			workflow: beforeReleasePlease("uses: docker/build-push-action@dc802804100637a589fabce1cb79ff13a1411302"),
			wantIn:   "docker/build-push-action",
		},
		{
			name:     "an attest step",
			workflow: beforeReleasePlease("uses: actions/attest@9000827ccba6bdab643e8b6fd33ac0654aef8333"),
			wantIn:   "actions/attest",
		},
		{
			name:     "a docker push run",
			workflow: beforeReleasePlease("name: Push the image\n        run: docker push ghcr.io/timothymamo/shift2eu:latest"),
			wantIn:   "docker push",
		},

		// The concurrency group is this workflow's own: GitHub keeps one pending
		// run per group and cancels the older pending one when a new run queues,
		// so sharing the group release.yml uses would let a later push to main
		// cancel a release that is waiting to run, and the release would be
		// lost. A run in progress is never cancelled (ADR 0015).
		{
			name:     "no concurrency",
			workflow: mutate(t, releasePleaseWorkflow, "concurrency:\n  group: release-please\n  cancel-in-progress: false\n", ""),
			wantIn:   "concurrency",
		},
		{
			name:     "no concurrency group",
			workflow: mutate(t, releasePleaseWorkflow, "  group: release-please\n", ""),
			wantIn:   "group",
		},
		{
			// release.yml's own group: a release run waiting behind this one
			// would be cancelled by the next push to main.
			name:     "the concurrency group release.yml uses",
			workflow: mutate(t, releasePleaseWorkflow, "group: release-please", "group: release"),
			wantIn:   "release.yml",
		},
		{
			name:     "a concurrency that cancels the run in progress",
			workflow: mutate(t, releasePleaseWorkflow, "cancel-in-progress: false", "cancel-in-progress: true"),
			wantIn:   "cancel-in-progress",
		},

		// A document the check cannot read tells it nothing, so it is refused
		// rather than passed.
		{name: "an empty document", workflow: "", wantIn: "empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := check.ReleasePleaseWorkflow([]byte(tc.workflow))
			joined := strings.Join(got, "\n")

			if tc.wantIn == "" {
				if len(got) != 0 {
					t.Errorf("ReleasePleaseWorkflow() = %q, want no message", got)
				}

				return
			}

			if !strings.Contains(joined, tc.wantIn) {
				t.Errorf("ReleasePleaseWorkflow() = %q, want a message containing %q", got, tc.wantIn)
			}
		})
	}
}

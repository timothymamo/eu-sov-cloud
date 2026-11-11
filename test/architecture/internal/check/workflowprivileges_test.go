package check_test

import (
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/test/architecture/internal/check"
)

// workflowDoc renders a small workflow with the trigger lines below on:, a
// workflow-level permissions block ("" for none) and a job-level permissions
// block ("" for none), each as a caller writes it in .github/workflows.
func workflowDoc(trigger, workflowPermissions, jobPermissions string) string {
	var b strings.Builder

	b.WriteString("name: Example\n\non:\n")
	b.WriteString(trigger)
	b.WriteString("\n")
	b.WriteString(workflowPermissions)
	b.WriteString("jobs:\n")
	b.WriteString("  build:\n")
	b.WriteString("    name: Build\n")
	b.WriteString("    runs-on: ubuntu-latest\n")
	b.WriteString(jobPermissions)
	b.WriteString("    steps:\n")
	b.WriteString("      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1\n")
	b.WriteString("        with:\n")
	b.WriteString("          persist-credentials: false\n")
	b.WriteString("      - name: Build\n")
	b.WriteString("        run: make build\n")

	return b.String()
}

// TestWorkflowPrivileges pins the audit's rule of issue #72: only
// .github/workflows/release.yml may hold what publishes to a registry, mints an
// OIDC identity or stores attestations, and only it may run on a tag. Every
// other workflow is refused a packages: write, id-token: write or
// attestations: write permission, at workflow or job level, a write-all, and an
// on: push: tags: trigger.
func TestWorkflowPrivileges(t *testing.T) {
	t.Parallel()

	const (
		ciName      = ".github/workflows/ci.yml"
		releaseName = ".github/workflows/release.yml"

		onBranches = "  push:\n    branches: [main]\n  pull_request:\n"
		onTags     = "  push:\n    tags: [\"v*.*.*\"]\n"

		workflowRead = "permissions:\n  contents: read\n"
		noPermission = "permissions: {}\n"
	)

	tests := []struct {
		name     string
		workflow string
		src      string
		// wantIn is a part of the message the check must return; "" means
		// none at all.
		wantIn string
	}{
		// The workflows the repository runs on every pull request ask for
		// nothing a release needs.
		{
			name:     "a ci.yml-like workflow",
			workflow: ciName,
			src:      workflowDoc(onBranches, workflowRead, ""),
		},
		{
			name:     "a security.yml-like workflow",
			workflow: ciName,
			src: workflowDoc(
				"  push:\n    branches: [main]\n  pull_request:\n  schedule:\n    - cron: \"0 6 * * 1\"\n",
				workflowRead,
				"    permissions:\n      contents: read\n      security-events: write\n",
			),
		},
		{
			name:     "packages: read",
			workflow: ciName,
			src:      workflowDoc(onBranches, workflowRead, "    permissions:\n      packages: read\n"),
		},
		{
			name:     "permissions: {}",
			workflow: ciName,
			src:      workflowDoc(onBranches, noPermission, ""),
		},
		{
			name:     "a workflow_dispatch trigger",
			workflow: ciName,
			src:      workflowDoc("  workflow_dispatch:\n  pull_request:\n", workflowRead, ""),
		},

		// release.yml is the one workflow that publishes: it holds all three
		// and runs on a tag.
		{
			name:     "release.yml",
			workflow: releaseName,
			src: workflowDoc(
				onTags,
				noPermission,
				"    permissions:\n      packages: write\n      id-token: write\n      attestations: write\n",
			),
		},

		// Every publishing privilege, at job level and at workflow level, in a
		// workflow that is not the release.
		{name: "packages: write at job level", workflow: ciName, src: workflowDoc(onBranches, workflowRead, "    permissions:\n      packages: write\n"), wantIn: "packages"},
		{name: "id-token: write at job level", workflow: ciName, src: workflowDoc(onBranches, workflowRead, "    permissions:\n      id-token: write\n"), wantIn: "id-token"},
		{name: "attestations: write at job level", workflow: ciName, src: workflowDoc(onBranches, workflowRead, "    permissions:\n      attestations: write\n"), wantIn: "attestations"},
		{name: "packages: write at workflow level", workflow: ciName, src: workflowDoc(onBranches, "permissions:\n  contents: read\n  packages: write\n", ""), wantIn: "packages"},
		{name: "id-token: write at workflow level", workflow: ciName, src: workflowDoc(onBranches, "permissions:\n  contents: read\n  id-token: write\n", ""), wantIn: "id-token"},
		{name: "attestations: write at workflow level", workflow: ciName, src: workflowDoc(onBranches, "permissions:\n  contents: read\n  attestations: write\n", ""), wantIn: "attestations"},
		{name: "write-all at workflow level", workflow: ciName, src: workflowDoc(onBranches, "permissions: write-all\n", ""), wantIn: "write-all"},
		{name: "write-all at job level", workflow: ciName, src: workflowDoc(onBranches, workflowRead, "    permissions: write-all\n"), wantIn: "write-all"},
		{
			name:     "two privileges at once",
			workflow: ciName,
			src:      workflowDoc(onBranches, workflowRead, "    permissions:\n      packages: write\n      id-token: write\n"),
			wantIn:   "packages",
		},

		// A tag trigger is what a release runs on, so no other workflow may
		// have one.
		{name: "a tags trigger", workflow: ciName, src: workflowDoc(onTags, workflowRead, ""), wantIn: "tags"},
		{
			name:     "a tags trigger next to a branch trigger",
			workflow: ciName,
			src:      workflowDoc("  push:\n    branches: [main]\n    tags: [\"v*\"]\n  pull_request:\n", workflowRead, ""),
			wantIn:   "tags",
		},
		{name: "an empty document", workflow: ciName, src: "", wantIn: "workflow"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := check.WorkflowPrivileges(tc.workflow, []byte(tc.src))

			if tc.wantIn == "" {
				if len(got) != 0 {
					t.Errorf("WorkflowPrivileges(%q) = %q, want no message", tc.workflow, got)
				}

				return
			}

			if !strings.Contains(strings.Join(got, "\n"), tc.wantIn) {
				t.Errorf("WorkflowPrivileges(%q) = %q, want a message containing %q", tc.workflow, got, tc.wantIn)
			}
		})
	}
}

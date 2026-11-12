package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/timothymamo/shift2eu/test/internal/yamlread"
)

const (
	ciWorkflowName = ".github/workflows/ci.yml"

	// releasePleaseDryRunScript is the dry run of release-please.yml's guard
	// and tag steps, which a job of ci.yml runs.
	releasePleaseDryRunScript = ".github/scripts/release-please-dry-run.sh"
)

// dryRunCredentials are the expressions a job of the dry run may not hold:
// any secret, and GITHUB_TOKEN by either name.
var dryRunCredentials = []string{"secrets.", "github.token"}

// ReleasePleaseDryRun returns one message per problem in src, ci.yml, and none
// for one that runs .github/scripts/release-please-dry-run.sh on every pull
// request and every push to main (issue #142), in a job that:
//
//   - holds no write permission, its own or, when it declares none, the
//     workflow's: no write scope, no write-all, and no workflow with no
//     permissions: at all, which leaves GITHUB_TOKEN the repository's default;
//   - holds no secret and no GITHUB_TOKEN anywhere in it.
//
// The dry run reaches no network, writes nothing and needs no credential, so a
// pull request that changes how the tag's version is read, or when the tag
// step refuses to tag, fails before it merges, and the dry run cannot reach the
// repository with anything but a read.
func ReleasePleaseDryRun(src []byte) []string {
	name := ciWorkflowName

	lines, err := yamlread.SplitLines(src)
	if err != nil {
		return []string{fmt.Sprintf("%s: read the workflow: %v", name, err)}
	}

	top, err := yamlread.Mapping(lines)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", name, err)}
	}

	r := &schemaReader{raw: rawLines(src)}
	byKey := map[string]yamlread.Entry{}

	for _, e := range top {
		byKey[e.Key] = e
	}

	r.dryRunTriggers(byKey["on"])

	jobs, ok := byKey["jobs"]
	if !ok {
		r.fail("%s has no jobs:, so nothing runs %s", name, releasePleaseDryRunScript)

		return r.messages
	}

	entries, err := yamlread.Mapping(r.block(jobs, name+": jobs"))
	if err != nil {
		r.fail("%s: jobs: %v", name, err)

		return r.messages
	}

	found := false

	for _, job := range entries {
		path := name + ": jobs: " + job.Key
		if !r.runsDryRun(job, path) {
			continue
		}

		found = true

		r.dryRunPermissions(job, byKey, path)
		r.dryRunCredentials(job, path)
	}

	if !found {
		r.fail("%s holds no job that runs %s, so a change to release-please.yml's guard or tag step is never run before it merges", name, releasePleaseDryRunScript)
	}

	return r.messages
}

// dryRunTriggers checks that on: holds pull_request and a push whose branches
// include main.
func (r *schemaReader) dryRunTriggers(e yamlread.Entry) {
	path := ciWorkflowName + ": on"

	if e.Num == 0 {
		r.fail("%s has no on:; want pull_request and push: branches: [main]", ciWorkflowName)

		return
	}

	if e.Value != "" {
		list, err := yamlread.List(e)
		if err != nil || !slices.Contains(list, "pull_request") || !slices.Contains(list, "push") {
			r.fail("line %d: %s is %q; want pull_request and push: branches: [main]", e.Num, path, e.Value)
		}

		return
	}

	triggers, err := yamlread.Mapping(e.Block)
	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	pullRequest, push := false, false

	for _, t := range triggers {
		switch t.Key {
		case "pull_request":
			pullRequest = true
		case "push":
			push = r.pushesMain(t, path)
		}
	}

	if !pullRequest {
		r.fail("line %d: %s has no pull_request trigger, so the dry run of release-please.yml never runs before a change to it merges", e.Num, path)
	}

	if !push {
		r.fail("line %d: %s has no push trigger with branches: [main], so the dry run does not run on main", e.Num, path)
	}
}

// pushesMain reports whether t, the push trigger, runs on a push to main.
func (r *schemaReader) pushesMain(t yamlread.Entry, path string) bool {
	if t.Value == "" && len(t.Block) == 0 {
		return true
	}

	entries, err := yamlread.Mapping(t.Block)
	if err != nil {
		r.fail("%s: push: %v", path, err)

		return false
	}

	for _, p := range entries {
		if p.Key != "branches" {
			continue
		}

		list, err := yamlread.List(p)
		if err != nil {
			r.fail("%s: push: %v", path, err)

			return false
		}

		return slices.Contains(list, "main")
	}

	return false
}

// runsDryRun reports whether one of job's steps runs the dry run's script.
func (r *schemaReader) runsDryRun(job yamlread.Entry, path string) bool {
	keys, err := yamlread.Mapping(job.Block)
	if err != nil {
		r.fail("%s: %v", path, err)

		return false
	}

	for _, k := range keys {
		if k.Key != "steps" {
			continue
		}

		steps, ok := r.workflowSteps(k, path+": steps")
		if !ok {
			return false
		}

		for _, s := range steps {
			if !s.hasRun {
				continue
			}

			script, err := r.value(s.run)
			if err == nil && strings.Contains(script, releasePleaseDryRunScript) {
				return true
			}
		}
	}

	return false
}

// dryRunPermissions checks that the job holds no write: its own permissions:,
// or the workflow's when it declares none.
func (r *schemaReader) dryRunPermissions(job yamlread.Entry, top map[string]yamlread.Entry, path string) {
	keys, _ := yamlread.Mapping(job.Block)

	for _, k := range keys {
		if k.Key == "permissions" {
			r.noWrite(k, path+": permissions")

			return
		}
	}

	perm, ok := top["permissions"]
	if !ok {
		r.fail("line %d: %s declares no permissions: and neither does %s, so GITHUB_TOKEN holds the repository's default, which may write; want permissions: contents: read", job.Num, path, ciWorkflowName)

		return
	}

	r.noWrite(perm, path+": the workflow's permissions, which it holds")
}

// noWrite checks that e, a permissions:, grants no write.
func (r *schemaReader) noWrite(e yamlread.Entry, path string) {
	if e.Value != "" {
		if e.Value != "{}" && e.Value != "read-all" {
			r.fail("line %d: %s is %s; the dry run of release-please.yml writes nothing, so it holds no write", e.Num, path, e.Value)
		}

		return
	}

	scopes, err := yamlread.Mapping(e.Block)
	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	for _, s := range scopes {
		value, err := yamlread.Single(s)
		if err != nil || (value != "read" && value != "none") {
			r.fail("line %d: %s: %s is %s; the dry run of release-please.yml writes nothing, so it holds no write", s.Num, path, s.Key, s.Value)
		}
	}
}

// dryRunCredentials refuses a secret or GITHUB_TOKEN anywhere in the job: the
// lines from its key to the next line indented no further, comments included.
func (r *schemaReader) dryRunCredentials(job yamlread.Entry, path string) {
	first := job.Num - 1
	if first < 0 || first >= len(r.raw) {
		return
	}

	indent := len(r.raw[first]) - len(strings.TrimLeft(r.raw[first], " "))

	for i := first; i < len(r.raw); i++ {
		line := r.raw[i]
		trimmed := strings.TrimLeft(line, " ")

		if i > first && trimmed != "" && len(line)-len(trimmed) <= indent {
			break
		}

		for _, credential := range dryRunCredentials {
			if strings.Contains(line, credential) {
				r.fail("line %d: %s holds %s: the dry run needs no secret and no token, so it holds none", i+1, path, strings.TrimSpace(line))
			}
		}
	}
}

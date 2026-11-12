package check

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/timothymamo/shift2eu/test/internal/yamlread"
)

// The release-please workflow of issue #142 (ADR 0021): what it is read for.
const (
	releasePleaseWorkflowName = ".github/workflows/release-please.yml"
	// releasePleaseGroup is the workflow's own concurrency group; releaseGroup
	// the one release.yml uses, which it must never share.
	releasePleaseGroup = "release-please"
	releaseGroup       = "release"
	// releasePleaseEnvironment holds the release App's ID and key, and is
	// limited to main.
	releasePleaseEnvironment = "release-please"

	appTokenAction      = "actions/create-github-app-token"
	releasePleaseAction = "googleapis/release-please-action"

	// The ids of the two steps that create the tag: the wait for no release in
	// flight, and the tagging step itself.
	guardStepID = "guard"
	tagStepID   = "tag"

	// releaseRuns is the API path of release.yml's runs, which the guard
	// reads.
	releaseRuns = "actions/workflows/release.yml/runs"

	// releasePleaseRunner is the one runner the job may run on: GitHub's,
	// never a self-hosted one, since the job holds the release App's token.
	releasePleaseRunner = "ubuntu-latest"
)

// releaseRunStatuses are every status GitHub may hold a run of release.yml in
// before it finishes: the guard waits for no run in any of them, since a run
// requested, waiting or pending may still start and publish.
var releaseRunStatuses = []string{"requested", "waiting", "pending", "queued", "in_progress"}

// guardStatusLoop matches the guard's loop over the statuses it waits for.
var guardStatusLoop = regexp.MustCompile(`(?m)^\s*for\s+status\s+in\s+([A-Za-z_ ]+?)\s*;\s*do\s*$`)

// automaticTokens are the two spellings of GITHUB_TOKEN the guard may read
// release.yml's runs with.
var automaticTokens = []string{"${{ github.token }}", "${{ secrets.GITHUB_TOKEN }}"}

// tagStepEnv are the only variables the tagging step's env: holds.
var tagStepEnv = []string{"APP_SLUG", "GH_REPO", "GH_TOKEN"}

// appTokenOutput matches a step output named token, the form in which the
// App's token reaches another step.
var appTokenOutput = regexp.MustCompile(`outputs\s*\.\s*token\b`)

// fetchingRuns are commands that create or edit a GitHub Release, or reach a
// host of their own, none of which a run: step of this workflow may hold.
var fetchingRuns = []string{"gh release", "curl", "wget"}

// repoEndpoint matches the repository's API path in a script, through GH_REPO
// or github.repository, and captures what follows it.
var repoEndpoint = regexp.MustCompile(`repos/(?:\$\{GH_REPO\}|\$GH_REPO|\$\{\{\s*github\.repository\s*\}\})/([^"'\s]*)`)

// tagEndpoints are the API endpoints the tagging step may call, its query
// removed: the release PRs (issues, pulls, a pull request, its files and its
// labels), the manifest at the merge commit (contents), the merge commit's
// ancestry (compare), and the tag (git/ref, git/refs, git/matching-refs,
// git/tags). Nothing creates or edits a GitHub Release.
var tagEndpoints = []*regexp.Regexp{
	regexp.MustCompile(`^issues$`),
	regexp.MustCompile(`^issues/[^/]+/labels(/[^/]+)?$`),
	regexp.MustCompile(`^pulls$`),
	regexp.MustCompile(`^pulls/[^/]+$`),
	regexp.MustCompile(`^pulls/[^/]+/files$`),
	regexp.MustCompile(`^contents/.+$`),
	regexp.MustCompile(`^compare/[^/]+$`),
	regexp.MustCompile(`^git/ref/.+$`),
	regexp.MustCompile(`^git/refs$`),
	regexp.MustCompile(`^git/matching-refs/.+$`),
	regexp.MustCompile(`^git/tags(/[^/]+)?$`),
}

// commitPin matches the 40-hex commit an action is pinned to.
var commitPin = regexp.MustCompile(`^[0-9a-f]{40}$`)

// appTokenWith is the whole with: of the step that mints the App token: the
// environment's ID and key, and exactly the two permissions the workflow uses.
var appTokenWith = map[string]string{
	"app-id":                   "${{ vars.RELEASE_APP_ID }}",
	"private-key":              "${{ secrets.RELEASE_APP_PRIVATE_KEY }}",
	"permission-contents":      "write",
	"permission-pull-requests": "write",
}

// publishingRuns are commands that publish, sign or attest, or push a tag,
// none of which a run: step of this workflow may hold.
var publishingRuns = []string{"goreleaser", "cosign", "docker login", "docker push", "docker buildx", "git push", "git tag"}

// allowedExpression matches the one expression a run: script may hold. Any
// other ${{ }} in a script is refused, so that no pull request title, body,
// label or branch name is ever pasted into the shell: those reach a script
// through env: alone, quoted.
var allowedExpression = regexp.MustCompile(`\$\{\{\s*github\.repository\s*\}\}`)

// ReleasePleaseWorkflow returns one message per problem in src, the
// release-please workflow of issue #142, and none for one that:
//
//   - runs on a push to main alone, with permissions: {} at the top level and
//     a concurrency group of its own, release-please, never release.yml's
//     release, with cancel-in-progress: false: GitHub keeps one pending run
//     per group and cancels the older one, so sharing release.yml's group
//     would let a later push to main cancel a release waiting to run;
//   - holds one job, in the release-please environment, whose permissions
//     only read (or that declares none), and whose steps are run: steps, or
//     actions/create-github-app-token and googleapis/release-please-action
//     pinned to a 40-hex commit, with no if:, continue-on-error: or other key
//     that could skip a step or let one fail;
//   - mints the App token once, from vars.RELEASE_APP_ID and
//     secrets.RELEASE_APP_PRIVATE_KEY, with exactly permission-contents and
//     permission-pull-requests: write, before the release-please step, which
//     reads that token and nothing else;
//   - runs on ubuntu-latest alone, never a self-hosted runner;
//   - creates the tag in a run: step with id: tag after the release-please
//     step, whose env: is exactly GH_TOKEN, the App's token, GH_REPO, the
//     repository, and APP_SLUG, the App's own slug, and whose gh api calls
//     reach only the endpoints in tagEndpoints, behind a run: step with id:
//     guard that reads release.yml's requested, waiting, pending, queued and
//     in_progress runs with GITHUB_TOKEN;
//   - hands the App's token to release-please's with: token and the tagging
//     step's GH_TOKEN, and nowhere else;
//   - publishes nothing (no goreleaser, cosign, docker login, push or build,
//     attest), pushes no tag with git, creates no GitHub Release and calls no
//     host of its own (no gh release, curl or wget);
//   - pastes no expression but ${{ github.repository }} into a script.
//
// It reads the document with the strict line reader and refuses what it does
// not expect rather than guess at it.
func ReleasePleaseWorkflow(src []byte) []string {
	name := releasePleaseWorkflowName

	lines, err := yamlread.SplitLines(src)
	if err != nil {
		return []string{fmt.Sprintf("%s: read the workflow: %v", name, err)}
	}

	if len(lines) == 0 {
		return []string{name + ": the workflow is empty, so this check cannot tell what it does"}
	}

	r := &schemaReader{raw: rawLines(src)}

	top := r.exactKeysAllowing(lines, name, []string{"name", "on", "permissions", "concurrency", "jobs"})
	if top == nil {
		return r.messages
	}

	r.releasePleaseTrigger(top["on"])

	if perm, ok := top["permissions"]; !ok {
		r.fail("%s has no top-level permissions:, so GITHUB_TOKEN takes the repository's default permissions; want permissions: {}", name)
	} else if perm.Value != "{}" || len(perm.Block) != 0 {
		r.fail("line %d: %s: the top-level permissions grant GITHUB_TOKEN a scope; want permissions: {}, the App's token does the writing", perm.Num, name)
	}

	if c, ok := top["concurrency"]; !ok {
		r.fail("%s has no concurrency:, so two runs could tag at once; want group: %s and cancel-in-progress: false", name, releasePleaseGroup)
	} else {
		r.releasePleaseConcurrency(c)
	}

	if jobs, ok := top["jobs"]; !ok {
		r.fail("%s has no jobs:", name)
	} else {
		r.releasePleaseJobs(jobs)
	}

	return r.messages
}

// exactKeysAllowing reads lines as a mapping whose keys are all among allowed,
// none of them required, and returns its entries by key, or nil when it cannot
// read them.
func (r *schemaReader) exactKeysAllowing(lines []yamlread.Line, path string, allowed []string) map[string]yamlread.Entry {
	if lines == nil {
		return nil
	}

	entries, err := yamlread.Mapping(lines)
	if err != nil {
		r.fail("%s: %v", path, err)

		return nil
	}

	got := make(map[string]yamlread.Entry, len(entries))

	for _, e := range entries {
		got[e.Key] = e

		if !slices.Contains(allowed, e.Key) {
			r.fail("line %d: %s holds the key %q, want only %s", e.Num, path, e.Key, strings.Join(allowed, ", "))
		}
	}

	return got
}

// releasePleaseTrigger checks on: a push to the branch main alone.
func (r *schemaReader) releasePleaseTrigger(e yamlread.Entry) {
	path := releasePleaseWorkflowName + ": on"

	if e.Num == 0 {
		r.fail("%s has no on:; want a push to main alone", releasePleaseWorkflowName)

		return
	}

	if e.Value != "" {
		r.fail("line %d: %s is %q, which runs on a push of any branch or tag; want push: branches: [main] alone", e.Num, path, e.Value)

		return
	}

	triggers, err := yamlread.Mapping(e.Block)
	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	for _, t := range triggers {
		if t.Key != "push" {
			r.fail("line %d: %s: %s runs the workflow outside a push to main; want push: branches: [main] alone", t.Num, path, t.Key)

			continue
		}

		if t.Value != "" || len(t.Block) == 0 {
			r.fail("line %d: %s: push runs on a push of any branch or tag; want push: branches: [main] alone", t.Num, path)

			continue
		}

		push, err := yamlread.Mapping(t.Block)
		if err != nil {
			r.fail("%s: push: %v", path, err)

			continue
		}

		branches := false

		for _, p := range push {
			if p.Key != "branches" {
				r.fail("line %d: %s: push: %s is not a push to main alone; want branches: [main] alone", p.Num, path, p.Key)

				continue
			}

			branches = true

			list, err := yamlread.List(p)
			if err != nil || !slices.Equal(list, []string{"main"}) {
				r.fail("line %d: %s: push: branches is %q, want [main]", p.Num, path, p.Value)
			}
		}

		if !branches {
			r.fail("line %d: %s: push has no branches:, so it runs on a push of any branch; want branches: [main]", t.Num, path)
		}
	}

	if !slices.ContainsFunc(triggers, func(t yamlread.Entry) bool { return t.Key == "push" }) {
		r.fail("line %d: %s has no push:; want push: branches: [main]", e.Num, path)
	}
}

// releasePleaseConcurrency checks concurrency: the workflow's own group, a run
// in progress never cancelled.
func (r *schemaReader) releasePleaseConcurrency(e yamlread.Entry) {
	path := releasePleaseWorkflowName + ": concurrency"

	c := r.exactKeysAllowing(r.block(e, path), path, []string{"group", "cancel-in-progress"})
	if c == nil {
		return
	}

	if g, ok := c["group"]; !ok {
		r.fail("line %d: %s has no group:; want group: %s", e.Num, path, releasePleaseGroup)
	} else if value, err := yamlread.Single(g); err != nil {
		r.fail("%s: %v", path, err)
	} else if value == releaseGroup {
		r.fail("line %d: %s: group is %s, the group .github/workflows/release.yml uses: GitHub keeps one pending run per group, so a later push to main would cancel a release waiting to run; want group: %s", g.Num, path, value, releasePleaseGroup)
	} else if value != releasePleaseGroup {
		r.fail("line %d: %s: group is %q, want %s", g.Num, path, value, releasePleaseGroup)
	}

	if cancel, ok := c["cancel-in-progress"]; !ok {
		r.fail("line %d: %s has no cancel-in-progress:; want cancel-in-progress: false", e.Num, path)
	} else if value, err := yamlread.Single(cancel); err != nil || value != "false" {
		r.fail("line %d: %s: cancel-in-progress is %q, want false: a run in progress may be tagging", cancel.Num, path, cancel.Value)
	}
}

// releasePleaseJobs checks jobs: one job, its environment, its permissions and
// its steps.
func (r *schemaReader) releasePleaseJobs(e yamlread.Entry) {
	path := releasePleaseWorkflowName + ": jobs"

	jobs, err := yamlread.Mapping(r.block(e, path))
	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	if len(jobs) != 1 {
		r.fail("line %d: %s holds %d jobs, want exactly 1", e.Num, path, len(jobs))
	}

	for _, job := range jobs {
		jobPath := path + ": " + job.Key

		keys := r.exactKeysAllowing(r.block(job, jobPath), jobPath, []string{"name", "runs-on", "environment", "permissions", "timeout-minutes", "steps"})
		if keys == nil {
			continue
		}

		if env, ok := keys["environment"]; !ok {
			r.fail("line %d: %s has no environment:, so the release App's key is not limited to main; want environment: %s", job.Num, jobPath, releasePleaseEnvironment)
		} else if value, err := yamlread.Single(env); err != nil || value != releasePleaseEnvironment {
			r.fail("line %d: %s: environment is %q, want %s, which holds the release App's ID and key and is limited to main", env.Num, jobPath, env.Value, releasePleaseEnvironment)
		}

		if runner, ok := keys["runs-on"]; !ok {
			r.fail("line %d: %s has no runs-on:; want runs-on: %s", job.Num, jobPath, releasePleaseRunner)
		} else if value, err := yamlread.Single(runner); err != nil || value != releasePleaseRunner {
			r.fail("line %d: %s: runs-on is %q, want exactly %s: the job holds the release App's token, so it runs on a runner GitHub provides, never a self-hosted one", runner.Num, jobPath, runner.Value, releasePleaseRunner)
		}

		if perm, ok := keys["permissions"]; ok {
			r.readOnlyPermissions(perm, jobPath+": permissions")
		}

		if steps, ok := keys["steps"]; !ok {
			r.fail("line %d: %s has no steps:", job.Num, jobPath)
		} else {
			r.releasePleaseSteps(steps, jobPath+": steps")
		}
	}
}

// readOnlyPermissions checks that e, a job's permissions:, grants GITHUB_TOKEN
// no write: the App's token does the writing.
func (r *schemaReader) readOnlyPermissions(e yamlread.Entry, path string) {
	if e.Value != "" {
		if e.Value != "{}" && e.Value != "read-all" {
			r.fail("line %d: %s is %s; want read scopes only, the App's token does the writing", e.Num, path, e.Value)
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
		if err != nil {
			r.fail("%s: %v", path, err)

			continue
		}

		if value != "read" && value != "none" {
			r.fail("line %d: %s: %s is %s; want read or none, the App's token does the writing", s.Num, path, s.Key, value)
		}
	}
}

// pleaseStep is one step of the release-please job.
type pleaseStep struct {
	num            int
	id, action     string
	commit         string
	with           map[string]yamlread.Entry
	env            map[string]string
	envNums        map[string]int
	script         string
	hasRun, hasEnv bool
}

// releasePleaseSteps reads and checks the job's steps.
func (r *schemaReader) releasePleaseSteps(e yamlread.Entry, path string) {
	steps, ok := r.readPleaseSteps(e, path)
	if !ok {
		return
	}

	index := func(match func(pleaseStep) bool) []int {
		var out []int

		for i, s := range steps {
			if match(s) {
				out = append(out, i)
			}
		}

		return out
	}

	apps := index(func(s pleaseStep) bool { return s.action == appTokenAction })
	pleases := index(func(s pleaseStep) bool { return s.action == releasePleaseAction })
	guards := index(func(s pleaseStep) bool { return s.id == guardStepID })
	tags := index(func(s pleaseStep) bool { return s.id == tagStepID })

	token, slug := "", ""

	if len(apps) != 1 {
		r.fail("%s holds %d %s steps, want exactly 1, which mints the release App's token", path, len(apps), appTokenAction)
	} else {
		app := steps[apps[0]]
		r.appTokenStep(app, path)

		if app.id == "" {
			r.fail("line %d: %s: the %s step has no id:, so no step can read its token", app.num, path, appTokenAction)
		} else {
			token = "${{ steps." + app.id + ".outputs.token }}"
			slug = "${{ steps." + app.id + ".outputs.app-slug }}"
		}
	}

	if len(pleases) != 1 {
		r.fail("%s holds %d %s steps, want exactly 1", path, len(pleases), releasePleaseAction)
	} else {
		please := steps[pleases[0]]
		r.releasePleaseStep(please, path, token)

		if len(apps) == 1 && apps[0] > pleases[0] {
			r.fail("line %d: %s: the %s step runs before the App's token is minted", please.num, path, releasePleaseAction)
		}
	}

	if len(guards) != 1 {
		r.fail("%s holds %d steps with id: %s, want exactly 1, which waits for no run of .github/workflows/release.yml to be queued or in progress before the tag is created", path, len(guards), guardStepID)
	} else {
		r.guardStep(steps[guards[0]], path, token)
	}

	if len(tags) != 1 {
		r.fail("%s holds %d steps with id: %s, want exactly 1, which creates the vX.Y.Z tag with the release App's token", path, len(tags), tagStepID)
	} else {
		tag := steps[tags[0]]
		r.tagStep(tag, path, token, slug)

		if len(guards) == 1 && guards[0] > tags[0] {
			r.fail("line %d: %s: the tagging step runs before the wait for no release.yml run in flight", tag.num, path)
		}

		if len(pleases) == 1 && pleases[0] > tags[0] {
			r.fail("line %d: %s: the tagging step runs before the %s step", tag.num, path, releasePleaseAction)
		}
	}

	allowed := map[int]bool{}

	if len(pleases) == 1 {
		if t, ok := steps[pleases[0]].with["token"]; ok {
			allowed[t.Num] = true
		}
	}

	if len(tags) == 1 {
		if num, ok := steps[tags[0]].envNums["GH_TOKEN"]; ok {
			allowed[num] = true
		}
	}

	r.appTokenReach(allowed)
}

// appTokenReach refuses a step output named token anywhere in the workflow
// but on the lines in allowed: release-please's with: token and the tagging
// step's GH_TOKEN. Handing the App's token to any other step widens the
// reach of a token that can create release tags. Comments are skipped.
func (r *schemaReader) appTokenReach(allowed map[int]bool) {
	for i, line := range r.raw {
		num := i + 1
		if strings.HasPrefix(strings.TrimSpace(line), "#") || allowed[num] {
			continue
		}

		if appTokenOutput.MatchString(line) {
			r.fail("line %d: %s hands a step's outputs.token to more than the %s step's with: token and the tagging step's GH_TOKEN: the release App's token reaches those two alone", num, releasePleaseWorkflowName, releasePleaseAction)
		}
	}
}

// entryNums returns the line of each variable of e, an env: block read as
// jobEnv reads it, one NAME: value per line; jobEnv reports what it cannot
// read.
func entryNums(e yamlread.Entry) map[string]int {
	out := map[string]int{}

	for _, l := range e.Block {
		if name, _, found := strings.Cut(l.Text, ": "); found {
			out[name] = l.Num
		}
	}

	return out
}

// readPleaseSteps reads e, the job's steps:, refusing a key that could skip a
// step or let it fail, an action that is not one of the two the workflow runs,
// a run: that publishes, and an expression pasted into a script.
func (r *schemaReader) readPleaseSteps(e yamlread.Entry, path string) ([]pleaseStep, bool) {
	items, err := yamlread.Sequence(r.block(e, path))
	if err != nil {
		r.fail("%s: %v", path, err)

		return nil, false
	}

	steps := make([]pleaseStep, 0, len(items))

	for i, it := range items {
		stepPath := fmt.Sprintf("%s: step %d (line %d)", path, i+1, it.Num)

		keys := r.exactKeysAllowing(it.Lines, stepPath, []string{"name", "id", "uses", "with", "env", "run"})
		if keys == nil {
			return nil, false
		}

		step := pleaseStep{num: it.Num}

		if id, ok := keys["id"]; ok {
			step.id, _ = yamlread.Single(id)
		}

		if uses, ok := keys["uses"]; ok {
			value, err := yamlread.Single(uses)
			if err != nil {
				r.fail("%s: %v", stepPath, err)
			}

			step.action, step.commit, _ = strings.Cut(value, "@")
			r.pinnedAction(step, stepPath, value)
		}

		step.with = r.stepMapping(keys, "with", stepPath)
		if env, ok := keys["env"]; ok {
			step.hasEnv = true
			step.env, _ = r.jobEnv(env, stepPath+": env")
			step.envNums = entryNums(env)
		}

		if run, ok := keys["run"]; ok {
			step.hasRun = true
			step.script = r.stepScript(run, stepPath)
			r.safeScript(step, stepPath)
		}

		steps = append(steps, step)
	}

	return steps, true
}

// pinnedAction checks that the step runs one of the two actions the workflow
// may run, pinned to a commit.
func (r *schemaReader) pinnedAction(s pleaseStep, path, uses string) {
	if s.action != appTokenAction && s.action != releasePleaseAction {
		r.fail("line %d: %s runs %s; want only %s and %s: nothing here publishes, release.yml does", s.num, path, s.action, appTokenAction, releasePleaseAction)
	}

	if !commitPin.MatchString(s.commit) {
		r.fail("line %d: %s runs %s, which is not pinned to a 40-hex commit", s.num, path, uses)
	}
}

// stepMapping reads the step's key, a block mapping, by key.
func (r *schemaReader) stepMapping(keys map[string]yamlread.Entry, key, path string) map[string]yamlread.Entry {
	out := map[string]yamlread.Entry{}

	e, ok := keys[key]
	if !ok {
		return out
	}

	entries, err := yamlread.Mapping(r.block(e, path+": "+key))
	if err != nil {
		r.fail("%s: %s: %v", path, key, err)

		return out
	}

	for _, m := range entries {
		out[m.Key] = m
	}

	return out
}

// stepScript reads a run:, a literal block or one scalar.
func (r *schemaReader) stepScript(e yamlread.Entry, path string) string {
	value, err := r.value(e)
	if err != nil {
		r.fail("%s: run: %v", path, err)
	}

	return value
}

// safeScript refuses a script that publishes or pushes a tag with git, and
// one that pastes an expression other than ${{ github.repository }} into the
// shell.
func (r *schemaReader) safeScript(s pleaseStep, path string) {
	for _, command := range publishingRuns {
		if strings.Contains(s.script, command) {
			r.fail("line %d: %s runs %s; nothing here publishes or pushes a tag with git: release.yml publishes, and the tag is created through the API with the App's token", s.num, path, command)
		}
	}

	for _, command := range fetchingRuns {
		if strings.Contains(s.script, command) {
			r.fail("line %d: %s runs %s; nothing here creates or edits a GitHub Release, which is GoReleaser's in release.yml, or calls a host of its own", s.num, path, command)
		}
	}

	if strings.Contains(allowedExpression.ReplaceAllString(s.script, ""), "${{") {
		r.fail("line %d: %s pastes an expression into its script; pass it through env: and quote it", s.num, path)
	}
}

// singleValue returns the scalar of e, "" when it cannot be read.
func singleValue(e yamlread.Entry) string {
	value, err := yamlread.Single(e)
	if err != nil {
		return e.Value
	}

	return value
}

// appTokenStep checks the with: of the step that mints the App token.
func (r *schemaReader) appTokenStep(s pleaseStep, path string) {
	for _, key := range sortedKeys(appTokenWith) {
		e, ok := s.with[key]
		if !ok {
			r.fail("line %d: %s: the %s step has no %s; want %s: %s", s.num, path, appTokenAction, key, key, appTokenWith[key])

			continue
		}

		if value := singleValue(e); value != appTokenWith[key] {
			r.fail("line %d: %s: %s is %s, want %s", e.Num, path, key, value, appTokenWith[key])
		}
	}

	for _, key := range sortedKeys(s.with) {
		if _, ok := appTokenWith[key]; !ok {
			r.fail("line %d: %s: the %s step holds %s; want only %s", s.with[key].Num, path, appTokenAction, key, strings.Join(sortedKeys(appTokenWith), ", "))
		}
	}
}

// releasePleaseStep checks that the release-please step reads the App's token
// and sets nothing else: every other input overrides the committed
// configuration.
func (r *schemaReader) releasePleaseStep(s pleaseStep, path, token string) {
	t, ok := s.with["token"]
	if !ok {
		r.fail("line %d: %s: the %s step has no with: token, so it runs with GITHUB_TOKEN; want token: %s", s.num, path, releasePleaseAction, token)
	} else if value := singleValue(t); token != "" && value != token {
		r.fail("line %d: %s: the %s step's token is %s, want the release App's token, %s", t.Num, path, releasePleaseAction, value, token)
	}

	for _, key := range sortedKeys(s.with) {
		if key != "token" {
			r.fail("line %d: %s: the %s step holds %s, which overrides release-please-config.json; want only token", s.with[key].Num, path, releasePleaseAction, key)
		}
	}

	if s.hasEnv {
		r.fail("line %d: %s: the %s step holds env:; want only with: token", s.num, path, releasePleaseAction)
	}
}

// guardStep checks that the guard is a run: step that reads release.yml's
// runs in every status in releaseRunStatuses, with GITHUB_TOKEN: the App's
// token is for release-please and the tag alone.
func (r *schemaReader) guardStep(s pleaseStep, path, token string) {
	if !s.hasRun {
		r.fail("line %d: %s: the step with id: %s has no run:, so nothing waits for .github/workflows/release.yml", s.num, path, guardStepID)

		return
	}

	if !strings.Contains(s.script, releaseRuns) {
		r.fail("line %d: %s: the step with id: %s does not read %s, the runs of .github/workflows/release.yml", s.num, path, guardStepID, releaseRuns)
	}

	var waited []string
	for _, m := range guardStatusLoop.FindAllStringSubmatch(s.script, -1) {
		waited = append(waited, strings.Fields(m[1])...)
	}

	for _, status := range releaseRunStatuses {
		if !slices.Contains(waited, status) {
			r.fail("line %d: %s: the step with id: %s does not wait for release.yml's %s runs in its for status in ...; do loop: a run %s may still publish, so a tag created now could be lost", s.num, path, guardStepID, status, status)
		}
	}

	value, ok := s.env["GH_TOKEN"]

	switch {
	case !ok:
		r.fail("line %d: %s: the step with id: %s has no env: GH_TOKEN, so gh cannot read release.yml's runs; want GH_TOKEN: %s", s.num, path, guardStepID, automaticTokens[0])
	case !slices.Contains(automaticTokens, value):
		r.fail("line %d: %s: the step with id: %s reads release.yml's runs with GH_TOKEN %s; want GITHUB_TOKEN, %s or %s: the release App's token %s is for release-please and the tag alone", s.num, path, guardStepID, value, automaticTokens[0], automaticTokens[1], token)
	}
}

// tagStep checks that the tagging step is a run: step whose env: is exactly
// the App's token, the repository and the App's slug, and whose script calls
// only the API endpoints in tagEndpoints.
func (r *schemaReader) tagStep(s pleaseStep, path, token, slug string) {
	if !s.hasRun {
		r.fail("line %d: %s: the step with id: %s has no run:, so no tag is created", s.num, path, tagStepID)
	}

	for _, key := range sortedKeys(s.env) {
		if !slices.Contains(tagStepEnv, key) {
			r.fail("line %d: %s: the tagging step's env: holds %s; want only %s", s.num, path, key, strings.Join(tagStepEnv, ", "))
		}
	}

	if value, ok := s.env["GH_REPO"]; !ok || value != "${{ github.repository }}" {
		r.fail("line %d: %s: the tagging step's GH_REPO is %q, want ${{ github.repository }}", s.num, path, value)
	}

	if value, ok := s.env["APP_SLUG"]; !ok {
		r.fail("line %d: %s: the tagging step has no env: APP_SLUG, so it cannot tell a release PR the App opened from one a person labelled; want APP_SLUG: %s", s.num, path, slug)
	} else if slug != "" && value != slug {
		r.fail("line %d: %s: the tagging step's APP_SLUG is %s, want the release App's own slug, %s", s.num, path, value, slug)
	}

	r.tagEndpoints(s, path)

	value, ok := s.env["GH_TOKEN"]
	if !ok {
		r.fail("line %d: %s: the tagging step has no env: GH_TOKEN, so gh has no token to create the tag with; want GH_TOKEN: %s", s.num, path, token)

		return
	}

	if token != "" && value != token {
		r.fail("line %d: %s: the tagging step's GH_TOKEN is %s, want the release App's token, %s: a tag created with GITHUB_TOKEN starts no release run and the ruleset refuses it", s.num, path, value, token)
	}
}

// tagEndpoints refuses a gh api call of the tagging step whose path is not the
// repository's, and a repository endpoint outside tagEndpoints.
func (r *schemaReader) tagEndpoints(s pleaseStep, path string) {
	for _, line := range strings.Split(s.script, "\n") {
		if strings.Contains(line, "gh api") && !repoEndpoint.MatchString(line) {
			r.fail("line %d: %s: the tagging step calls gh api with no repos/${GH_REPO}/ path on its line: %q; want only the repository's endpoints", s.num, path, strings.TrimSpace(line))
		}
	}

	for _, m := range repoEndpoint.FindAllStringSubmatch(s.script, -1) {
		endpoint, _, _ := strings.Cut(m[1], "?")
		if !slices.ContainsFunc(tagEndpoints, func(re *regexp.Regexp) bool { return re.MatchString(endpoint) }) {
			r.fail("line %d: %s: the tagging step calls the API endpoint %s; want only the release PR's (issues, pulls, pulls/<n>, pulls/<n>/files, issues/<n>/labels), the manifest's (contents), the merge commit's ancestry (compare) and the tag's (git/ref, git/refs, git/matching-refs, git/tags)", s.num, path, endpoint)
		}
	}
}

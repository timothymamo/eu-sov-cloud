package check

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/timothymamo/shift2eu/test/internal/yamlread"
)

// The release-please workflow of issues #142, #157 and #159 (ADR 0021): what
// it is read for.
const (
	releasePleaseWorkflowName = ".github/workflows/release-please.yml"
	// releasePleaseGroup is the workflow's own concurrency group; releaseGroup
	// the one release.yml uses, which it must never share.
	releasePleaseGroup = "release-please"
	releaseGroup       = "release"
	// releasePleaseEnvironment holds the release-please App's client ID and
	// private key, releaseTagEnvironment the release-tag App's; both are
	// limited to main.
	releasePleaseEnvironment = "release-please"
	releaseTagEnvironment    = "release-tag"

	// The workflow's three jobs: the release PR, the tag, the relabelling of
	// the tagged release PR.
	releasePleaseJob = "release-please"
	tagJob           = "tag"
	relabelJob       = "relabel"

	appTokenAction      = "actions/create-github-app-token"
	releasePleaseAction = "googleapis/release-please-action"

	// The ids of the two steps that create the tag: the wait for no release in
	// flight, and the tagging step itself.
	guardStepID = "guard"
	tagStepID   = "tag"

	// releaseRuns is the API path of release.yml's runs, which the guard
	// reads.
	releaseRuns = "actions/workflows/release.yml/runs"

	// releasePleaseRunner is the one runner the jobs may run on: GitHub's,
	// never a self-hosted one, since each job holds an App's token.
	releasePleaseRunner = "ubuntu-latest"

	// releasePleaseSlug is the release-please App's public slug, whose bot
	// shift2eu-release[bot] opens the release PR: the tag job's guard and
	// tagging steps hold it as a constant, never as an output of the job that
	// runs the third-party release-please-action, which that action could set.
	releasePleaseSlug = "shift2eu-release"
	// The expressions the relabel job reads the tag job through: the PR and
	// the tag it created.
	taggedPR   = "${{ needs.tag.outputs.pr }}"
	taggedTag  = "${{ needs.tag.outputs.tag }}"
	repository = "${{ github.repository }}"
	// relabelCondition is the relabel job's if:, so that it runs only when the
	// tag job tagged a PR.
	relabelCondition = "needs.tag.outputs.pr != ''"
)

// releaseRunStatuses are every status GitHub may hold a run of release.yml in
// before it finishes: the guard waits for no run in any of them, since a run
// requested, waiting or pending may still start and publish.
var releaseRunStatuses = []string{"requested", "waiting", "pending", "queued", "in_progress"}

// allowedNeeds are the only needs expressions the workflow may hold outside
// comments: the tag job's two outputs, which the relabel job reads. Every other
// reach for a job's outputs, the release-please job's above all, which runs the
// third-party release-please-action, is a value a job would trust.
var allowedNeeds = []string{"needs.tag.outputs.pr", "needs.tag.outputs.tag"}

// needsRef matches a needs context as a word, in any case as GitHub Actions
// reads expression contexts, and the property path that follows it, dotted
// or in brackets, and captures the whole expression: bare (toJSON(needs)),
// needs.<job>.outputs.<name>, NEEDS.<job> or Needs['<job>'].
var needsRef = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_-])(needs(?:\s*\.\s*[A-Za-z0-9_*-]+|\s*\[[^\]]*\])*)`)

// guardStatusLoop matches the guard's loop over the statuses it waits for.
var guardStatusLoop = regexp.MustCompile(`(?m)^\s*for\s+status\s+in\s+([A-Za-z_ ]+?)\s*;\s*do\s*$`)

// automaticTokens are the two spellings of GITHUB_TOKEN the guard may read
// release.yml's runs with.
var automaticTokens = []string{"${{ github.token }}", "${{ secrets.GITHUB_TOKEN }}"}

// appTokenOutput matches a step output named token, the form in which an
// App's token reaches another step.
var appTokenOutput = regexp.MustCompile(`outputs\s*\.\s*token\b`)

// credentialRef matches a variable or a secret an expression reads, and
// captures its name.
var credentialRef = regexp.MustCompile(`\b(?:vars|secrets)\s*\.\s*([A-Za-z0-9_-]+)`)

// fetchingRuns are commands that create or edit a GitHub Release, or reach a
// host of their own, none of which a run: step of this workflow may hold.
var fetchingRuns = []string{"gh release", "curl", "wget"}

// repoEndpoint matches the repository's API path in a script, through GH_REPO
// or github.repository, and captures what follows it.
var repoEndpoint = regexp.MustCompile(`repos/(?:\$\{GH_REPO\}|\$GH_REPO|\$\{\{\s*github\.repository\s*\}\})/([^"'\s]*)`)

// tagEndpoints are the API endpoints the tagging step may call, its query
// removed: the release PRs (pulls, a pull request and its files), the
// manifest at the merge commit (contents), the merge commit's ancestry
// (compare), and the tag (git/ref, git/refs, git/matching-refs, git/tags).
// Nothing creates or edits a GitHub Release, and nothing labels: the
// release-tag App's token reads pull requests only, and the relabel job
// relabels. The issues listing is not among them either: neither App's token
// holds an Issues permission, and GitHub answers that listing with 403
// without it.
var tagEndpoints = []*regexp.Regexp{
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

// tagEndpointsWant names tagEndpoints in a message.
const tagEndpointsWant = "the release PR's (pulls, pulls/<n>, pulls/<n>/files), the manifest's (contents), the merge commit's ancestry (compare) and the tag's (git/ref, git/refs, git/matching-refs, git/tags)"

// relabelEndpoints are the API endpoints the relabelling step may call: the
// tagged PR (pulls/<n>), the tag it re-reads (git/ref, git/tags/<sha>) and the
// PR's labels.
var relabelEndpoints = []*regexp.Regexp{
	regexp.MustCompile(`^pulls/[^/]+$`),
	regexp.MustCompile(`^git/ref/.+$`),
	regexp.MustCompile(`^git/tags/[^/]+$`),
	regexp.MustCompile(`^issues/[^/]+/labels(/[^/]+)?$`),
}

// relabelEndpointsWant names relabelEndpoints in a message.
const relabelEndpointsWant = "the tagged PR's (pulls/<n>, issues/<n>/labels) and the tag's (git/ref, git/tags/<sha>)"

// publishingRuns are commands that publish, sign or attest, or push a tag,
// none of which a run: step of this workflow may hold.
var publishingRuns = []string{"goreleaser", "cosign", "docker login", "docker push", "docker buildx", "git push", "git tag"}

// allowedExpression matches the one expression a run: script may hold. Any
// other ${{ }} in a script is refused, so that no pull request title, body,
// label or branch name, and no job output, is ever pasted into the shell:
// those reach a script through env: alone, quoted.
var allowedExpression = regexp.MustCompile(`\$\{\{\s*github\.repository\s*\}\}`)

// commitPin matches the 40-hex commit an action is pinned to.
var commitPin = regexp.MustCompile(`^[0-9a-f]{40}$`)

// releaseApp is one of the two Apps of issue #157 and the whole with: of the
// step that mints its token: its environment's client ID (never the
// deprecated app-id, issue #159) and private key, and exactly the permissions
// its jobs need.
type releaseApp struct {
	name string
	with map[string]string
}

// releasePleaseApp opens the release PR, labels it and relabels it once it is
// tagged. It bypasses no v* tag ruleset; its token can still merge a PR into
// main once the required checks pass (ADR 0021, residual risk).
var releasePleaseApp = releaseApp{
	name: "the release-please App",
	with: map[string]string{
		"client-id":                "${{ vars.RELEASE_APP_CLIENT_ID }}",
		"private-key":              "${{ secrets.RELEASE_APP_PRIVATE_KEY }}",
		"permission-contents":      "write",
		"permission-pull-requests": "write",
	},
}

// releaseTagApp creates the tag, and is the only App that may bypass the
// Release tag creation ruleset. It reads pull requests, to check the merged
// release PR, and writes nothing but contents, which is still enough to merge
// a PR once the required checks pass (ADR 0021, residual risk).
var releaseTagApp = releaseApp{
	name: "the release-tag App",
	with: map[string]string{
		"client-id":                "${{ vars.RELEASE_TAG_APP_CLIENT_ID }}",
		"private-key":              "${{ secrets.RELEASE_TAG_APP_PRIVATE_KEY }}",
		"permission-contents":      "write",
		"permission-pull-requests": "read",
	},
}

// stepKind is what a step of the workflow is for.
type stepKind int

const (
	appTokenStepKind stepKind = iota
	pleaseStepKind
	guardStepKind
	tagStepKind
	runStepKind
	otherStepKind
)

// String names the kind in a message.
func (k stepKind) String() string {
	switch k {
	case appTokenStepKind:
		return "the " + appTokenAction + " step"
	case pleaseStepKind:
		return "the " + releasePleaseAction + " step"
	case guardStepKind:
		return "the wait for no release.yml run in flight (id: guard)"
	case tagStepKind:
		return "the tagging step (id: tag)"
	case runStepKind:
		return "a run: step"
	default:
		return "another action"
	}
}

// kindOf tells what s is for.
func kindOf(s pleaseStep) stepKind {
	switch {
	case s.action == appTokenAction:
		return appTokenStepKind
	case s.action == releasePleaseAction:
		return pleaseStepKind
	case s.id == guardStepID:
		return guardStepKind
	case s.id == tagStepID:
		return tagStepKind
	case s.action == "" && s.hasRun:
		return runStepKind
	default:
		return otherStepKind
	}
}

// describeKinds names kinds in a message.
func describeKinds(kinds []stepKind) string {
	if len(kinds) == 0 {
		return "no step"
	}

	names := make([]string, 0, len(kinds))
	for _, k := range kinds {
		names = append(names, k.String())
	}

	return "[" + strings.Join(names, ", ") + "]"
}

// releaseJobSpec is what one of the workflow's three jobs must be.
type releaseJobSpec struct {
	key         string
	does        string
	environment string
	app         releaseApp
	// needs is the jobs it waits for, sorted; none for the first job.
	needs []string
	// condition is its if:, "" for none: a job that must run holds none.
	condition string
	// outputs are its outputs: by name, given the id of its token step.
	outputs func(appID string) map[string]string
	// steps are its steps, in order.
	steps []stepKind
}

// keys are the keys the job may hold.
func (s releaseJobSpec) keys() []string {
	keys := []string{"name", "runs-on", "environment", "permissions", "timeout-minutes", "steps"}

	if len(s.needs) != 0 {
		keys = append(keys, "needs")
	}

	if s.condition != "" {
		keys = append(keys, "if")
	}

	if s.outputs != nil {
		keys = append(keys, "outputs")
	}

	return keys
}

// releaseJobSpecs are the workflow's three jobs, in the order they run.
var releaseJobSpecs = []releaseJobSpec{
	{
		key:         releasePleaseJob,
		does:        "opens the release PR with the release-please App's token",
		environment: releasePleaseEnvironment,
		app:         releasePleaseApp,
		steps:       []stepKind{appTokenStepKind, pleaseStepKind},
	},
	{
		key:         tagJob,
		does:        "creates the tag with the release-tag App's token",
		environment: releaseTagEnvironment,
		app:         releaseTagApp,
		needs:       []string{releasePleaseJob},
		outputs: func(string) map[string]string {
			return map[string]string{"pr": "${{ steps.tag.outputs.pr }}", "tag": "${{ steps.tag.outputs.tag }}"}
		},
		steps: []stepKind{appTokenStepKind, guardStepKind, tagStepKind},
	},
	{
		key:         relabelJob,
		does:        "relabels the tagged release PR with the release-please App's token",
		environment: releasePleaseEnvironment,
		app:         releasePleaseApp,
		needs:       []string{releasePleaseJob, tagJob},
		condition:   relabelCondition,
		steps:       []stepKind{appTokenStepKind, runStepKind},
	},
}

// ReleasePleaseWorkflow returns one message per problem in src, the
// release-please workflow of issues #142, #157 and #159, and none for one
// that:
//
//   - runs on a push to main alone, with permissions: {} at the top level and
//     a concurrency group of its own, release-please, never release.yml's
//     release, with cancel-in-progress: false: GitHub keeps one pending run
//     per group and cancels the older one, so sharing release.yml's group
//     would let a later push to main cancel a release waiting to run;
//   - holds exactly three jobs, each on ubuntu-latest alone, whose
//     permissions only read (or that declare none), and whose steps are run:
//     steps, or actions/create-github-app-token and
//     googleapis/release-please-action pinned to a 40-hex commit, with no
//     if:, continue-on-error: or other key that could skip a step or let one
//     fail:
//   - release-please, in the release-please environment, mints the
//     release-please App's token and runs release-please with it, and
//     declares no outputs:, so nothing after it trusts what the third-party
//     action could write;
//   - tag, which needs release-please, in the release-tag environment, mints
//     the release-tag App's token, waits in a run: step with id: guard that
//     reads release.yml's requested, waiting, pending, queued and in_progress
//     runs, and whose env: is exactly GH_TOKEN (the automatic token, either
//     spelling), GH_REPO and APP_SLUG: shift2eu-release, then creates the tag in a run: step with
//     id: tag whose env: is exactly GH_TOKEN, the release-tag App's token,
//     GH_REPO and APP_SLUG, the release-please App's slug shift2eu-release,
//     whose gh api calls reach only tagEndpoints, and which outputs the PR and
//     the tag it created;
//   - relabel, which needs both and runs only when the tag job tagged a PR,
//     in the release-please environment, mints the release-please App's
//     token and relabels the PR in a run: step whose env: is exactly GH_TOKEN,
//     that token, GH_REPO, PR and TAG, the tag job's outputs, and whose gh api
//     calls reach only relabelEndpoints;
//   - mints each App's token from client-id and private-key, never app-id,
//     read from that App's own variable and secret, with exactly the
//     permission-* inputs releasePleaseApp and releaseTagApp hold, and reads
//     no other variable or secret anywhere but GITHUB_TOKEN;
//   - hands each App's token to release-please's with: token, the tagging
//     step's GH_TOKEN and the relabelling step's GH_TOKEN, and nowhere else;
//   - publishes nothing (no goreleaser, cosign, docker login, push or build,
//     attest), pushes no tag with git, creates no GitHub Release and calls no
//     host of its own (no gh release, curl or wget);
//   - holds no needs expression outside comments but needs.tag.outputs.pr and
//     needs.tag.outputs.tag, each as the whole of its ${{ }} or as the
//     relabel job's exact if:: no job reads the release-please job's outputs,
//     in any spelling or case, nor every job's at once (toJSON(needs));
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
		r.fail("line %d: %s: the top-level permissions grant GITHUB_TOKEN a scope; want permissions: {}, the Apps' tokens do the writing", perm.Num, name)
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

// releasePleaseJobs checks jobs: the three jobs, each by its spec, then the
// reach of the Apps' tokens and credentials over the whole document.
func (r *schemaReader) releasePleaseJobs(e yamlread.Entry) {
	path := releasePleaseWorkflowName + ": jobs"

	jobs, err := yamlread.Mapping(r.block(e, path))
	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	byKey := make(map[string]yamlread.Entry, len(jobs))

	for _, job := range jobs {
		byKey[job.Key] = job

		if !slices.ContainsFunc(releaseJobSpecs, func(s releaseJobSpec) bool { return s.key == job.Key }) {
			r.fail("line %d: %s holds the job %s; want only %s, %s and %s", job.Num, path, job.Key, releasePleaseJob, tagJob, relabelJob)
		}
	}

	reach := map[int]bool{}
	credentials := map[int]bool{}

	for _, spec := range releaseJobSpecs {
		job, ok := byKey[spec.key]
		if !ok {
			r.fail("line %d: %s has no job %s, which %s", e.Num, path, spec.key, spec.does)

			continue
		}

		r.releasePleaseJob(job, spec, path+": "+spec.key, reach, credentials)
	}

	r.appTokenReach(reach)
	r.credentialReach(credentials)
	r.needsReach()
}

// needsReach refuses every needs expression outside comments but
// allowedNeeds, quoting it: the release-please job runs the third-party
// release-please-action, so no job trusts its outputs, in any spelling or
// case, and toJSON(needs) would hand a step every output of every job before
// it. An allowed expression is accepted only as the whole of its ${{ }}
// (whitespace inside the braces aside), or bare as the relabel job's exact
// if:, relabelCondition: inside a larger expression, such as a fallback
// (|| 'x') or fromJSON(...), it is refused with that expression quoted. A
// job's needs: key is not an expression and is skipped, as are comment lines.
// Any other word needs, in a script's text too, is refused: the check fails
// closed rather than tell an expression from prose.
func (r *schemaReader) needsReach() {
	for i, line := range r.raw {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if rest, ok := strings.CutPrefix(trimmed, "needs:"); ok {
			line = rest
		}

		for _, m := range needsRef.FindAllStringSubmatchIndex(line, -1) {
			expr := line[m[2]:m[3]]
			if m[3] < len(line) && isNameByte(line[m[3]]) {
				continue // a longer word, such as needsx
			}

			if !slices.Contains(allowedNeeds, expr) {
				r.fail("line %d: %s holds the needs expression %q (a bare word needs counts too: the check does not tell prose from an expression); want only %s: the release-please job runs %s, so no job trusts its outputs, and the tag job holds the release-please App's slug as the constant %s", i+1, releasePleaseWorkflowName, expr, strings.Join(allowedNeeds, " and "), releasePleaseAction, releasePleaseSlug)

				continue
			}

			if whole, ok := wholeNeedsExpression(line, trimmed, m[2], m[3]); !ok {
				r.fail("line %d: %s holds %s inside the expression %q; want it only as the whole of ${{ %s }}, or as the relabel job's if: %s: a larger expression around it may change what the step reads", i+1, releasePleaseWorkflowName, expr, whole, expr, relabelCondition)
			}
		}
	}
}

// wholeNeedsExpression reports whether the allowed needs expression at
// line[start:end] is the whole of the ${{ }} around it, or, outside any, the
// whole relabel job's if: (trimmed is the line without its indentation). It
// returns the expression around it, to quote when it is not.
func wholeNeedsExpression(line, trimmed string, start, end int) (string, bool) {
	open := strings.LastIndex(line[:start], "${{")
	if open >= 0 && !strings.Contains(line[open:start], "}}") {
		closing := strings.Index(line[end:], "}}")
		if closing < 0 {
			return line[open:], false
		}

		whole := line[open : end+closing+2]

		return whole, strings.TrimSpace(line[open+3:end+closing]) == line[start:end]
	}

	return trimmed, trimmed == "if: "+relabelCondition
}

// isNameByte reports whether b may continue a name in an expression.
func isNameByte(b byte) bool {
	return b == '_' || b == '-' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// releasePleaseJob checks one job against its spec: its keys, environment,
// runner, permissions, needs, if:, steps and outputs. It records in reach the
// lines that may read its App's token, and in credentials the lines that may
// read its App's variable and secret.
func (r *schemaReader) releasePleaseJob(job yamlread.Entry, spec releaseJobSpec, path string, reach, credentials map[int]bool) {
	keys := r.exactKeysAllowing(r.block(job, path), path, spec.keys())
	if keys == nil {
		return
	}

	if env, ok := keys["environment"]; !ok {
		r.fail("line %d: %s has no environment:, so %s's private key is not limited to main; want environment: %s", job.Num, path, spec.app.name, spec.environment)
	} else if value, err := yamlread.Single(env); err != nil || value != spec.environment {
		r.fail("line %d: %s: environment is %q, want %s, which holds %s's client ID and private key and is limited to main", env.Num, path, env.Value, spec.environment, spec.app.name)
	}

	if runner, ok := keys["runs-on"]; !ok {
		r.fail("line %d: %s has no runs-on:; want runs-on: %s", job.Num, path, releasePleaseRunner)
	} else if value, err := yamlread.Single(runner); err != nil || value != releasePleaseRunner {
		r.fail("line %d: %s: runs-on is %q, want exactly %s: the job holds %s's token, so it runs on a runner GitHub provides, never a self-hosted one", runner.Num, path, runner.Value, releasePleaseRunner, spec.app.name)
	}

	if perm, ok := keys["permissions"]; ok {
		r.readOnlyPermissions(perm, path+": permissions")
	}

	r.releasePleaseNeeds(job, keys, spec, path)
	r.releasePleaseCondition(job, keys, spec, path)

	appID := ""

	if steps, ok := keys["steps"]; !ok {
		r.fail("line %d: %s has no steps:", job.Num, path)
	} else {
		appID = r.releasePleaseSteps(steps, spec, path+": steps", reach, credentials)
	}

	r.releasePleaseOutputs(job, keys, spec, path, appID)
}

// releasePleaseNeeds checks the job's needs: the jobs before it, and none for
// the first.
func (r *schemaReader) releasePleaseNeeds(job yamlread.Entry, keys map[string]yamlread.Entry, spec releaseJobSpec, path string) {
	if len(spec.needs) == 0 {
		return
	}

	want := strings.Join(spec.needs, ", ")

	n, ok := keys["needs"]
	if !ok {
		r.fail("line %d: %s has no needs:, so it may run before the job it reads; want needs: [%s]", job.Num, path, want)

		return
	}

	var got []string

	var err error

	if strings.HasPrefix(n.Value, "[") || n.Value == "" {
		got, err = yamlread.List(n)
	} else {
		var value string
		value, err = yamlread.Single(n)
		got = []string{value}
	}

	if err != nil || !slices.Equal(slices.Sorted(slices.Values(got)), spec.needs) {
		r.fail("line %d: %s: needs is %q, want [%s]: the job runs after the jobs whose outputs it reads", n.Num, path, n.Value, want)
	}
}

// releasePleaseCondition checks the job's if:: none on a job that must run,
// and on the relabel job exactly relabelCondition.
func (r *schemaReader) releasePleaseCondition(job yamlread.Entry, keys map[string]yamlread.Entry, spec releaseJobSpec, path string) {
	if spec.condition == "" {
		return
	}

	c, ok := keys["if"]
	if !ok {
		r.fail("line %d: %s has no if:, so it runs when the tag job tagged nothing; want if: %s", job.Num, path, spec.condition)

		return
	}

	if value, err := yamlread.Single(c); err != nil || value != spec.condition {
		r.fail("line %d: %s: if is %q, want %s", c.Num, path, c.Value, spec.condition)
	}
}

// releasePleaseOutputs checks the job's outputs: exactly those the jobs after
// it read.
func (r *schemaReader) releasePleaseOutputs(job yamlread.Entry, keys map[string]yamlread.Entry, spec releaseJobSpec, path, appID string) {
	if spec.outputs == nil {
		return
	}

	want := spec.outputs(appID)
	outPath := path + ": outputs"

	o, ok := keys["outputs"]
	if !ok {
		r.fail("line %d: %s has no outputs:; want %s, which the jobs after it read", job.Num, path, strings.Join(sortedKeys(want), ", "))

		return
	}

	got := r.exactKeysAllowing(r.block(o, outPath), outPath, sortedKeys(want))
	if got == nil {
		return
	}

	for _, key := range sortedKeys(want) {
		e, ok := got[key]
		if !ok {
			r.fail("line %d: %s has no %s; want %s: %s", o.Num, outPath, key, key, want[key])

			continue
		}

		if value := singleValue(e); appID != "" && value != want[key] {
			r.fail("line %d: %s: %s is %s, want %s", e.Num, outPath, key, value, want[key])
		}
	}
}

// readOnlyPermissions checks that e, a job's permissions:, grants GITHUB_TOKEN
// no write: the Apps' tokens do the writing.
func (r *schemaReader) readOnlyPermissions(e yamlread.Entry, path string) {
	if e.Value != "" {
		if e.Value != "{}" && e.Value != "read-all" {
			r.fail("line %d: %s is %s; want read scopes only, the Apps' tokens do the writing", e.Num, path, e.Value)
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
			r.fail("line %d: %s: %s is %s; want read or none, the Apps' tokens do the writing", s.Num, path, s.Key, value)
		}
	}
}

// pleaseStep is one step of the workflow.
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

// releasePleaseSteps reads and checks one job's steps against its spec, and
// returns the id of its token step, "" when it has none to read.
func (r *schemaReader) releasePleaseSteps(e yamlread.Entry, spec releaseJobSpec, path string, reach, credentials map[int]bool) string {
	steps, ok := r.readPleaseSteps(e, path)
	if !ok {
		return ""
	}

	kinds := make([]stepKind, len(steps))

	for i, s := range steps {
		kinds[i] = kindOf(s)

		switch {
		case kinds[i] == pleaseStepKind && spec.key != releasePleaseJob:
			r.fail("line %d: %s runs %s; want it in the release-please job alone, which mints the release-please App's token", s.num, path, releasePleaseAction)
		case (kinds[i] == guardStepKind || kinds[i] == tagStepKind) && spec.key != tagJob:
			r.fail("line %d: %s holds %s; want it in the tag job alone, which mints the release-tag App's token, the only App that may bypass the Release tag creation ruleset", s.num, path, kinds[i])
		}
	}

	if !slices.Equal(kinds, spec.steps) {
		r.fail("%s holds %s, want exactly %s, in this order", path, describeKinds(kinds), describeKinds(spec.steps))
	}

	appID, token := "", ""

	for i, s := range steps {
		if kinds[i] != appTokenStepKind {
			continue
		}

		r.appTokenStep(s, path, spec.app)

		for _, key := range []string{"client-id", "private-key"} {
			if e, ok := s.with[key]; ok {
				credentials[e.Num] = true
			}
		}

		if appID != "" {
			continue
		}

		if s.id == "" {
			r.fail("line %d: %s: the %s step has no id:, so no step can read its token", s.num, path, appTokenAction)

			continue
		}

		appID = s.id
		token = "${{ steps." + s.id + ".outputs.token }}"
	}

	for i, s := range steps {
		switch {
		case kinds[i] == pleaseStepKind && spec.key == releasePleaseJob:
			r.releasePleaseStep(s, path, token)

			if t, ok := s.with["token"]; ok {
				reach[t.Num] = true
			}
		case kinds[i] == guardStepKind && spec.key == tagJob:
			r.guardStep(s, path, token)
		case kinds[i] == tagStepKind && spec.key == tagJob:
			r.tagStep(s, path, token)
			allowEnvLine(reach, s, "GH_TOKEN")
		case kinds[i] == runStepKind && spec.key == relabelJob:
			r.relabelStep(s, path, token)
			allowEnvLine(reach, s, "GH_TOKEN")
		}
	}

	return appID
}

// allowEnvLine records the line of s's env: key in lines.
func allowEnvLine(lines map[int]bool, s pleaseStep, key string) {
	if num, ok := s.envNums[key]; ok {
		lines[num] = true
	}
}

// appTokenReach refuses a step output named token anywhere in the workflow
// but on the lines in allowed: release-please's with: token, the tagging
// step's GH_TOKEN and the relabelling step's GH_TOKEN. Handing an App's token
// to any other step widens the reach of a token that can merge a PR or create
// a release tag. Comments are skipped.
func (r *schemaReader) appTokenReach(allowed map[int]bool) {
	for i, line := range r.raw {
		num := i + 1
		if strings.HasPrefix(strings.TrimSpace(line), "#") || allowed[num] {
			continue
		}

		if appTokenOutput.MatchString(line) {
			r.fail("line %d: %s hands a step's outputs.token to more than the %s step's with: token, the tagging step's GH_TOKEN and the relabelling step's GH_TOKEN: each App's token reaches those alone", num, releasePleaseWorkflowName, releasePleaseAction)
		}
	}
}

// credentialReach refuses a variable or a secret read anywhere in the
// workflow but on the lines in allowed, the client-id and private-key of the
// token steps, which appTokenStep holds to their own App's: each App's
// credentials reach the step that mints its token and nothing else, so no job
// reads the other App's. GITHUB_TOKEN is not a credential of either App.
// Comments are skipped.
func (r *schemaReader) credentialReach(allowed map[int]bool) {
	for i, line := range r.raw {
		num := i + 1
		if strings.HasPrefix(strings.TrimSpace(line), "#") || allowed[num] {
			continue
		}

		for _, m := range credentialRef.FindAllStringSubmatch(line, -1) {
			if m[1] != "GITHUB_TOKEN" {
				r.fail("line %d: %s reads %s outside a token step's client-id and private-key: each App's variable and secret reach the step that mints its token alone", num, releasePleaseWorkflowName, m[0])
			}
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

// appTokenStep checks the with: of a step that mints app's token: exactly
// app.with, so client-id and never the deprecated app-id, that App's own
// variable and secret, and its permissions alone. It records the lines of
// client-id and private-key in credentials.
func (r *schemaReader) appTokenStep(s pleaseStep, path string, app releaseApp) {
	for _, key := range sortedKeys(app.with) {
		e, ok := s.with[key]
		if !ok {
			r.fail("line %d: %s: the %s step has no %s; want %s: %s, %s's", s.num, path, appTokenAction, key, key, app.with[key], app.name)

			continue
		}

		if value := singleValue(e); value != app.with[key] {
			r.fail("line %d: %s: %s is %s, want %s, %s's", e.Num, path, key, value, app.with[key], app.name)
		}
	}

	for _, key := range sortedKeys(s.with) {
		if _, ok := app.with[key]; ok {
			continue
		}

		if key == "app-id" {
			r.fail("line %d: %s: the %s step holds app-id, which the action deprecates; want client-id: %s alone (issue #159)", s.with[key].Num, path, appTokenAction, app.with["client-id"])

			continue
		}

		r.fail("line %d: %s: the %s step holds %s; want only %s, the token %s's job needs", s.with[key].Num, path, appTokenAction, key, strings.Join(sortedKeys(app.with), ", "), app.name)
	}
}

// releasePleaseStep checks that the release-please step reads the
// release-please App's token and sets nothing else: every other input
// overrides the committed configuration.
func (r *schemaReader) releasePleaseStep(s pleaseStep, path, token string) {
	t, ok := s.with["token"]
	if !ok {
		r.fail("line %d: %s: the %s step has no with: token, so it runs with GITHUB_TOKEN; want token: %s", s.num, path, releasePleaseAction, token)
	} else if value := singleValue(t); token != "" && value != token {
		r.fail("line %d: %s: the %s step's token is %s, want the release-please App's token, %s", t.Num, path, releasePleaseAction, value, token)
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
// runs in every status in releaseRunStatuses, with GITHUB_TOKEN: the
// release-tag App's token is for the tag alone. Its env: is exactly GH_TOKEN,
// GH_REPO and APP_SLUG, the release-please App's slug as a constant.
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
		r.fail("line %d: %s: the step with id: %s reads release.yml's runs with GH_TOKEN %s; want GITHUB_TOKEN, %s or %s: the release-tag App's token %s is for the tag alone", s.num, path, guardStepID, value, automaticTokens[0], automaticTokens[1], token)
	}

	switch slug, ok := s.env["APP_SLUG"]; {
	case !ok:
		r.fail("line %d: %s: the step with id: %s has no env: APP_SLUG, so it cannot tell a release PR the release-please App opened; want APP_SLUG: %s", s.num, path, guardStepID, releasePleaseSlug)
	case slug != releasePleaseSlug:
		r.fail("line %d: %s: the step with id: %s's APP_SLUG is %s, want %s, the release-please App's slug", s.num, path, guardStepID, slug, releasePleaseSlug)
	}

	switch repo, ok := s.env["GH_REPO"]; {
	case !ok:
		r.fail("line %d: %s: the step with id: %s has no env: GH_REPO; want GH_REPO: %s", s.num, path, guardStepID, repository)
	case repo != repository:
		r.fail("line %d: %s: the step with id: %s's GH_REPO is %s, want %s", s.num, path, guardStepID, repo, repository)
	}

	for _, key := range sortedKeys(s.env) {
		if !slices.Contains([]string{"GH_TOKEN", "GH_REPO", "APP_SLUG"}, key) {
			r.fail("line %d: %s: the step with id: %s's env: holds %s; want only APP_SLUG, GH_REPO, GH_TOKEN", s.num, path, guardStepID, key)
		}
	}
}

// tagStep checks that the tagging step is a run: step whose env: is exactly
// the release-tag App's token, the repository and the release-please App's
// slug, whose script calls only tagEndpoints, and that writes the PR and the
// tag it created to $GITHUB_OUTPUT for the relabel job.
func (r *schemaReader) tagStep(s pleaseStep, path, token string) {
	if !s.hasRun {
		r.fail("line %d: %s: the step with id: %s has no run:, so no tag is created", s.num, path, tagStepID)
	}

	r.stepEnv(s, path, "the tagging step", map[string]string{
		"GH_TOKEN": token,
		"GH_REPO":  repository,
		"APP_SLUG": releasePleaseSlug,
	})

	if s.hasRun && !strings.Contains(s.script, "GITHUB_OUTPUT") {
		r.fail("line %d: %s: the tagging step writes nothing to $GITHUB_OUTPUT, so the relabel job cannot tell which PR it tagged; want pr= and tag=", s.num, path)
	}

	r.stepEndpoints(s, path, "the tagging step", tagEndpoints, tagEndpointsWant)
}

// relabelStep checks that the relabelling step is a run: step whose env: is
// exactly the release-please App's token, the repository and the tag job's
// two outputs, and whose script calls only relabelEndpoints.
func (r *schemaReader) relabelStep(s pleaseStep, path, token string) {
	r.stepEnv(s, path, "the relabelling step", map[string]string{
		"GH_TOKEN": token,
		"GH_REPO":  repository,
		"PR":       taggedPR,
		"TAG":      taggedTag,
	})

	r.stepEndpoints(s, path, "the relabelling step", relabelEndpoints, relabelEndpointsWant)
}

// stepEnv checks that s's env: is exactly want; a wanted value "" is not yet
// known (a token step with no id) and is not compared.
func (r *schemaReader) stepEnv(s pleaseStep, path, step string, want map[string]string) {
	for _, key := range sortedKeys(want) {
		value, ok := s.env[key]

		switch {
		case !ok:
			r.fail("line %d: %s: %s has no env: %s; want %s: %s", s.num, path, step, key, key, want[key])
		case want[key] != "" && value != want[key]:
			r.fail("line %d: %s: %s's %s is %s, want %s", s.num, path, step, key, value, want[key])
		}
	}

	for _, key := range sortedKeys(s.env) {
		if _, ok := want[key]; !ok {
			r.fail("line %d: %s: %s's env: holds %s; want only %s", s.num, path, step, key, strings.Join(sortedKeys(want), ", "))
		}
	}
}

// stepEndpoints refuses a gh api call of s whose path is not the
// repository's, and a repository endpoint outside allowed.
func (r *schemaReader) stepEndpoints(s pleaseStep, path, step string, allowed []*regexp.Regexp, want string) {
	for line := range strings.SplitSeq(s.script, "\n") {
		if strings.Contains(line, "gh api") && !repoEndpoint.MatchString(line) {
			r.fail("line %d: %s: %s calls gh api with no repos/${GH_REPO}/ path on its line: %q; want only the repository's endpoints", s.num, path, step, strings.TrimSpace(line))
		}
	}

	for _, m := range repoEndpoint.FindAllStringSubmatch(s.script, -1) {
		endpoint, _, _ := strings.Cut(m[1], "?")
		if !slices.ContainsFunc(allowed, func(re *regexp.Regexp) bool { return re.MatchString(endpoint) }) {
			r.fail("line %d: %s: %s calls the API endpoint %s; want only %s", s.num, path, step, endpoint, want)
		}
	}
}

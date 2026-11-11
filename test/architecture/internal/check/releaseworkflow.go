package check

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The release workflow is read as a closed schema: every key, job and step it
// may hold is listed below, with the value it must have. A pin a Renovate
// update changes (an action's commit, goreleaser's version) is matched by
// form, everything else exactly.
var (
	// actionPin matches the commit an action is pinned to.
	actionPin = `@[0-9a-f]{40}$`

	checkoutAction   = regexp.MustCompile(`^actions/checkout` + actionPin)
	setupGoAction    = regexp.MustCompile(`^actions/setup-go` + actionPin)
	goreleaserAction = regexp.MustCompile(`^goreleaser/goreleaser-action` + actionPin)

	// goreleaserVersion matches one exact goreleaser release.
	goreleaserVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
)

// releaseStep is one step a job of the release workflow must run.
type releaseStep struct {
	// what names the step in a message.
	what string
	// uses matches the action the step runs, nil for a run: step.
	uses *regexp.Regexp
	// name and run are a run: step's name and command.
	name, run string
	// with is the step's whole with: mapping, each key with what its value
	// must match.
	with map[string]*regexp.Regexp
	// env is the step's whole env: mapping, line by line; nil for none.
	env []string
}

// exactly matches value and nothing else.
func exactly(value string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(value) + `$`)
}

var (
	// setupGoStep is setup-go with its Actions cache off: the cache would
	// restore GOMODCACHE and GOCACHE, which the go command trusts without
	// verifying them again, from an earlier job.
	setupGoStep = releaseStep{
		what: "actions/setup-go", uses: setupGoAction,
		with: map[string]*regexp.Regexp{"go-version-file": exactly("go.mod"), "cache": exactly("false")},
	}

	// testSteps are the test job's steps: the tests, and nothing else, on a
	// runner of their own.
	testSteps = []releaseStep{
		{
			what: "actions/checkout", uses: checkoutAction,
			with: map[string]*regexp.Regexp{"persist-credentials": exactly("false")},
		},
		setupGoStep,
		{what: "make test", name: "Test", run: "make test"},
	}

	// goreleaserSteps are the goreleaser job's steps: no repository code runs
	// before goreleaser, which is asked for exactly release --clean.
	goreleaserSteps = []releaseStep{
		{
			what: "actions/checkout", uses: checkoutAction,
			with: map[string]*regexp.Regexp{"fetch-depth": exactly("0"), "persist-credentials": exactly("false")},
		},
		setupGoStep,
		{
			what: "goreleaser/goreleaser-action", uses: goreleaserAction,
			with: map[string]*regexp.Regexp{
				"distribution": exactly("goreleaser"),
				"version":      goreleaserVersion,
				"args":         exactly("release --clean"),
			},
			env: []string{"GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}"},
		},
	}
)

// ReleaseWorkflow returns a message for everything in the release workflow
// document src that could make goreleaser build something other than what
// the check lists. It reads the document as a closed schema:
//
//   - the top level holds exactly name, on (a push of v*.*.* tags),
//     permissions ({}) and jobs, so no workflow-level env: or defaults:;
//   - jobs holds exactly test and goreleaser, each with exactly name, runs-on
//     (ubuntu-latest), permissions (contents: read for test, contents: write
//     for goreleaser) and steps, and goreleaser needs: test, so no job-level
//     env:, container:, if: or other key;
//   - the test job's steps are exactly actions/checkout, actions/setup-go and
//     make test; the goreleaser job's exactly actions/checkout,
//     actions/setup-go and goreleaser/goreleaser-action, in that order, each
//     action pinned by commit and each with exactly the with: keys and values
//     listed, setup-go with cache: false in both jobs, goreleaser asked for
//     exactly release --clean at an exact version, with an env: of exactly
//     GITHUB_TOKEN from secrets.GITHUB_TOKEN.
//
// The goreleaser job therefore runs no repository code before goreleaser, and
// make test, which does, runs on another runner: neither the go env file nor
// GITHUB_ENV or GITHUB_PATH carries over from it. The one other channel
// between jobs, setup-go's Actions cache of GOMODCACHE and GOCACHE (shared by
// every job with the same runner OS, Go version and go.sum, CI on main
// included), is closed by cache: false: the go command trusts what GOCACHE
// holds and does not verify extracted modules again. The variables of the go
// command can come only from the runner image or from inside a pinned action.
//
// As a backstop, no line, comments included, may name github.env or
// github.path in any case, nor GITHUB_ENV, GITHUB_PATH or a variable of the
// go command or of cgo (a word starting GO or CGO in upper case: GOFLAGS,
// GOEXPERIMENT, GOAMD64, CGO_ENABLED, ...).
func ReleaseWorkflow(src []byte) []string {
	var messages []string

	for _, raw := range strings.Split(string(src), "\n") {
		messages = append(messages, forbiddenNames(strings.TrimSpace(raw))...)
	}

	lines, err := splitLines(src)
	if err != nil {
		return append(messages, fmt.Sprintf("read the release workflow: %v", err))
	}

	return append(messages, releaseSchema(lines)...)
}

// releaseSchema returns a message for every way lines, the release workflow,
// departs from the schema ReleaseWorkflow describes.
func releaseSchema(lines []line) []string {
	r := &schemaReader{}

	if len(lines) == 0 {
		return []string{"the release workflow is empty, so this check cannot tell what goreleaser is asked to do"}
	}

	top := r.exactKeys(lines, "the release workflow", "name", "on", "permissions", "jobs")
	if top == nil {
		return r.messages
	}

	r.scalar(top["name"], "the release workflow's name", exactly("Release"))
	r.rawValue(top["permissions"], "the release workflow's permissions", "{}")

	if on := r.exactKeys(r.block(top["on"], "on"), "on", "push"); on != nil {
		if push := r.exactKeys(r.block(on["push"], "on: push"), "on: push", "tags"); push != nil {
			r.list(push["tags"], "on: push: tags", []string{"v*.*.*"})
		}
	}

	jobs := r.exactKeys(r.block(top["jobs"], "jobs"), "jobs", "test", "goreleaser")
	if jobs == nil {
		return r.messages
	}

	r.job(jobs["test"], "jobs: test", "Test", "", "read", testSteps)
	r.job(jobs["goreleaser"], "jobs: goreleaser", "GoReleaser", "test", "write", goreleaserSteps)

	return r.messages
}

// schemaReader collects the messages of a schema check.
type schemaReader struct {
	messages []string
}

func (r *schemaReader) fail(format string, args ...any) {
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

// job checks one job: its name, needs (none when needs is empty), runner,
// contents permission and steps.
func (r *schemaReader) job(e entry, path, name, needs, contents string, steps []releaseStep) {
	keys := []string{"name", "runs-on", "permissions", "steps"}
	if needs != "" {
		keys = append(keys, "needs")
	}

	job := r.exactKeys(r.block(e, path), path, keys...)
	if job == nil {
		return
	}

	r.scalar(job["name"], path+": name", exactly(name))
	r.scalar(job["runs-on"], path+": runs-on", exactly("ubuntu-latest"))

	if needs != "" {
		r.scalar(job["needs"], path+": needs", exactly(needs))
	}

	if perms := r.exactKeys(r.block(job["permissions"], path+": permissions"), path+": permissions", "contents"); perms != nil {
		r.scalar(perms["contents"], path+": permissions: contents", exactly(contents))
	}

	stepsPath := path + ": steps"

	block := r.block(job["steps"], stepsPath)
	if block == nil {
		return
	}

	items, err := sequence(block)
	if err != nil {
		r.fail("%s: %v", stepsPath, err)

		return
	}

	if len(items) != len(steps) {
		whats := make([]string, 0, len(steps))
		for _, s := range steps {
			whats = append(whats, s.what)
		}

		r.fail("%s holds %d steps, want exactly %d: %s, so that nothing the check never lists runs in this job",
			stepsPath, len(items), len(steps), strings.Join(whats, ", "))

		return
	}

	for i, it := range items {
		r.step(it, fmt.Sprintf("%s: step %d", stepsPath, i+1), steps[i])
	}
}

// step checks one step against want.
func (r *schemaReader) step(it item, path string, want releaseStep) {
	path = fmt.Sprintf("%s (line %d), want %s", path, it.num, want.what)

	if want.uses == nil {
		step := r.exactKeys(it.lines, path, "name", "run")
		if step == nil {
			return
		}

		r.scalar(step["name"], path+": name", exactly(want.name))
		r.scalar(step["run"], path+": run", exactly(want.run))

		return
	}

	keys := []string{"uses", "with"}
	if want.env != nil {
		keys = append(keys, "env")
	}

	step := r.exactKeys(it.lines, path, keys...)
	if step == nil {
		return
	}

	r.scalar(step["uses"], path+": uses", want.uses)

	withKeys := make([]string, 0, len(want.with))
	for key := range want.with {
		withKeys = append(withKeys, key)
	}

	slices.Sort(withKeys)

	if with := r.exactKeys(r.block(step["with"], path+": with"), path+": with", withKeys...); with != nil {
		for _, key := range withKeys {
			r.scalar(with[key], path+": with: "+key, want.with[key])
		}
	}

	if want.env == nil {
		return
	}

	// The env: keys are upper case, which the mapping reader refuses, so the
	// block is compared line by line.
	env := r.block(step["env"], path+": env")
	if env == nil {
		return
	}

	got := make([]string, 0, len(env))
	for _, l := range env {
		got = append(got, l.text)
	}

	if !slices.Equal(got, want.env) {
		r.fail("%s: env holds %q, want exactly %q, so goreleaser could build with an environment the check never lists with", path, got, want.env)
	}
}

// exactKeys reads lines as a block mapping holding exactly want, and returns
// its entries by key; nil when it cannot, with the reasons in the messages.
func (r *schemaReader) exactKeys(lines []line, path string, want ...string) map[string]entry {
	// A nil block was reported by block already.
	if lines == nil {
		return nil
	}

	entries, err := mapping(lines)
	if err != nil {
		r.fail("%s: %v", path, err)

		return nil
	}

	got := make(map[string]entry, len(entries))
	ok := true

	for _, e := range entries {
		got[e.key] = e

		if !slices.Contains(want, e.key) {
			r.fail("%s holds the key %q (line %d), want only %s", path, e.key, e.num, strings.Join(want, ", "))

			ok = false
		}
	}

	for _, key := range want {
		if _, found := got[key]; !found {
			r.fail("%s holds no %q, want exactly %s", path, key, strings.Join(want, ", "))

			ok = false
		}
	}

	if !ok {
		return nil
	}

	return got
}

// block returns the lines of e's value, a block below its key; nil when it is
// not one, with the reason in the messages.
func (r *schemaReader) block(e entry, path string) []line {
	lines, err := blockValue(e, path)
	if err != nil {
		r.fail("%v", err)

		return nil
	}

	return lines
}

// scalar checks that e's value is one scalar that want matches.
func (r *schemaReader) scalar(e entry, path string, want *regexp.Regexp) {
	value, err := single(e)
	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	if !want.MatchString(value) {
		r.fail("line %d: %s is %q, want one matching %s", e.num, path, value, want)
	}
}

// rawValue checks that e's value is exactly want, as written on its line.
func (r *schemaReader) rawValue(e entry, path, want string) {
	if e.value != want || len(e.block) != 0 {
		r.fail("line %d: %s is %q, want exactly %q", e.num, path, e.value, want)
	}
}

// list checks that e's value is exactly the list want.
func (r *schemaReader) list(e entry, path string, want []string) {
	got, err := list(e)
	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	if !slices.Equal(got, want) {
		r.fail("line %d: %s is %q, want exactly %q", e.num, path, got, want)
	}
}

// forbiddenNames returns a message for every word of line that names a
// variable of the go command or of cgo, or GITHUB_ENV or GITHUB_PATH, and for
// github.env or github.path in any case.
func forbiddenNames(line string) []string {
	var messages []string

	lower := strings.ToLower(line)

	for _, token := range []string{"github.env", "github.path"} {
		if strings.Contains(lower, token) {
			messages = append(messages, fmt.Sprintf("the release workflow's line %q names %s, through which a step sets the environment or the PATH of the steps after it", line, token))
		}
	}

	words := strings.FieldsFunc(line, func(r rune) bool {
		return r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})

	for _, word := range words {
		switch {
		case word == "GITHUB_ENV" || word == "GITHUB_PATH":
			messages = append(messages, fmt.Sprintf("the release workflow's line %q names %s, through which a step sets the environment of the steps after it, goreleaser's included", line, word))
		case (strings.HasPrefix(word, "GO") || strings.HasPrefix(word, "CGO")) && strings.ToUpper(word) == word:
			messages = append(messages, fmt.Sprintf("the release workflow's line %q names %s, a variable of the go command or of cgo, which would change what goreleaser builds and which the check never lists with", line, word))
		}
	}

	return messages
}

// strayReleaseFiles are the names at the repository root, slash-separated,
// that change what goreleaser builds without the check reading them, each
// with the reason.
var strayReleaseFiles = []struct {
	name   string
	reason string
}{
	{name: ".config/goreleaser.yml", reason: "it is one of the other file names goreleaser looks for, so the release could build from a document the check never reads"},
	{name: ".config/goreleaser.yaml", reason: "it is one of the other file names goreleaser looks for, so the release could build from a document the check never reads"},
	{name: ".goreleaser.yml", reason: "it is one of the other file names goreleaser looks for, so the release could build from a document the check never reads"},
	{name: "goreleaser.yml", reason: "it is one of the other file names goreleaser looks for, so the release could build from a document the check never reads"},
	{name: "goreleaser.yaml", reason: "it is one of the other file names goreleaser looks for, so the release could build from a document the check never reads"},
	{name: "go.work", reason: "the go command goreleaser runs reads it and the check lists with GOWORK=off, so the release could build modules the check never lists"},
	{name: "go.work.sum", reason: "it comes with a go.work, which the go command goreleaser runs reads and the check lists with GOWORK=off"},
	{name: "vendor", reason: "the go command goreleaser runs builds from it and the check lists with -mod=readonly, so the release could build sources the check never lists"},
}

// StrayReleaseFiles returns a message for every file under root that git
// could commit and that changes what goreleaser builds without the check
// reading it: a goreleaser document other than .goreleaser.yaml, go.work,
// go.work.sum, or anything in vendor. It decides by what `git ls-files
// --cached --others --exclude-standard` lists (gitFiles): a tracked file, even
// one gone from the working tree or added with --force over .gitignore, and an
// untracked file git does not ignore. A developer's ignored go.work never
// reaches the release, which builds from a fresh checkout, so it is not
// refused. Symlinks are listed by git as files and count too.
//
// It fails closed: when git cannot run or cannot list root, it returns an
// error.
func StrayReleaseFiles(ctx context.Context, root string) ([]string, error) {
	names, err := gitFiles(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("find stray release files under %s: %w", root, err)
	}

	var messages []string

	for _, stray := range strayReleaseFiles {
		if slices.ContainsFunc(names, func(name string) bool {
			return name == stray.name || strings.HasPrefix(name, stray.name+"/")
		}) {
			messages = append(messages, stray.name+" is one git could commit: "+stray.reason)
		}
	}

	return messages, nil
}

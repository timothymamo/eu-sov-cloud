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
// update changes (an action's commit, goreleaser's, cosign's and syft's
// versions, the BuildKit image) is matched by form, everything else exactly.
var (
	// actionPin matches the commit an action is pinned to.
	actionPin = `@[0-9a-f]{40}$`

	checkoutAction         = regexp.MustCompile(`^actions/checkout` + actionPin)
	setupGoAction          = regexp.MustCompile(`^actions/setup-go` + actionPin)
	goreleaserAction       = regexp.MustCompile(`^goreleaser/goreleaser-action` + actionPin)
	setupBuildxAction      = regexp.MustCompile(`^docker/setup-buildx-action` + actionPin)
	loginAction            = regexp.MustCompile(`^docker/login-action` + actionPin)
	metadataAction         = regexp.MustCompile(`^docker/metadata-action` + actionPin)
	buildPushAction        = regexp.MustCompile(`^docker/build-push-action` + actionPin)
	cosignInstallerAction  = regexp.MustCompile(`^sigstore/cosign-installer` + actionPin)
	attestProvenanceAction = regexp.MustCompile(`^actions/attest-build-provenance` + actionPin)
	sbomAction             = regexp.MustCompile(`^anchore/sbom-action` + actionPin)
	attestSBOMAction       = regexp.MustCompile(`^actions/attest-sbom` + actionPin)

	// exactRelease matches one exact release of a tool: goreleaser, cosign,
	// syft.
	exactRelease = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

	// buildkitImage matches the BuildKit image setup-buildx-action runs,
	// pinned by an exact release and the digest of its multi-arch index.
	buildkitImage = regexp.MustCompile(`^image=moby/buildkit:v[0-9]+\.[0-9]+\.[0-9]+@sha256:[0-9a-f]{64}$`)
)

// releaseStep is one step a job of the release workflow must run. The keys
// the step holds are exactly those whose field is set.
type releaseStep struct {
	// what names the step in a message.
	what string
	// name and id are the step's name: and id:, "" for none.
	name, id string
	// uses matches the action the step runs, nil for a run: step.
	uses *regexp.Regexp
	// with is the step's whole with: mapping, each key with what its value
	// must match; nil for none. A literal block value (|) is matched as its
	// lines joined by newlines.
	with map[string]*regexp.Regexp
	// env is the step's whole env: mapping, line by line; nil for none.
	env []string
	// run is the step's command, a literal block's lines joined by newlines;
	// "" for none.
	run string
}

// exactly matches value and nothing else.
func exactly(value string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(value) + `$`)
}

// The image the image job publishes, and the expression of the digest it
// built, which every step after the build signs, attests and reads by.
const (
	imageName   = "ghcr.io/timothymamo/eusov"
	imageDigest = "${{ steps.build.outputs.digest }}"
)

var (
	// setupGoStep is setup-go with its Actions cache off: the cache would
	// restore GOMODCACHE and GOCACHE, which the go command trusts without
	// verifying them again, from an earlier job.
	setupGoStep = releaseStep{
		what: "actions/setup-go", uses: setupGoAction,
		with: map[string]*regexp.Regexp{"go-version-file": exactly("go.mod"), "cache": exactly("false")},
	}

	// fullCheckoutStep checks out the whole history without keeping the token.
	fullCheckoutStep = releaseStep{
		what: "actions/checkout", uses: checkoutAction,
		with: map[string]*regexp.Regexp{"fetch-depth": exactly("0"), "persist-credentials": exactly("false")},
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
		fullCheckoutStep,
		setupGoStep,
		{
			what: "goreleaser/goreleaser-action", uses: goreleaserAction,
			with: map[string]*regexp.Regexp{
				"distribution": exactly("goreleaser"),
				"version":      exactRelease,
				"args":         exactly("release --clean"),
			},
			env: []string{"GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}"},
		},
	}

	// imageSteps are the image job's steps: the build arguments GoReleaser
	// stamps, read from git with the tag passed through env:; a build of two
	// platforms from an empty BuildKit, pushed, without BuildKit's own
	// provenance or SBOM; then a keyless signature, a provenance attestation,
	// an SBOM and its attestation, each of the image by the digest built, and
	// nothing uploaded anywhere else.
	imageSteps = []releaseStep{
		fullCheckoutStep,
		{
			what: "the stamp of the build arguments", name: "Build arguments, as GoReleaser stamps them", id: "stamp",
			env: []string{"TAG: ${{ github.ref_name }}"},
			run: strings.Join([]string{
				`{`,
				`  echo "version=${TAG#v}"`,
				`  echo "commit=$(git rev-parse --short HEAD)"`,
				`  echo "date=$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ)"`,
				`} >> "$GITHUB_OUTPUT"`,
			}, "\n"),
		},
		{
			what: "docker/setup-buildx-action", uses: setupBuildxAction,
			with: map[string]*regexp.Regexp{"driver-opts": buildkitImage},
		},
		{
			what: "docker/login-action", uses: loginAction,
			with: map[string]*regexp.Regexp{
				"registry": exactly("ghcr.io"),
				"username": exactly("${{ github.actor }}"),
				"password": exactly("${{ secrets.GITHUB_TOKEN }}"),
			},
		},
		{
			what: "docker/metadata-action", name: "Image tags", id: "tags", uses: metadataAction,
			with: map[string]*regexp.Regexp{
				"images": exactly(imageName),
				"flavor": exactly("latest=auto"),
				"tags": exactly(strings.Join([]string{
					"type=semver,pattern={{raw}}",
					"type=semver,pattern={{major}}.{{minor}},enable=${{ !contains(github.ref_name, '-') }}",
				}, "\n")),
			},
		},
		{
			what: "docker/build-push-action", name: "Build and push linux/amd64 and linux/arm64", id: "build", uses: buildPushAction,
			with: map[string]*regexp.Regexp{
				"context":     exactly("."),
				"platforms":   exactly("linux/amd64,linux/arm64"),
				"push":        exactly("true"),
				"tags":        exactly("${{ steps.tags.outputs.tags }}"),
				"annotations": exactly("index:org.opencontainers.image.source=https://github.com/timothymamo/eu-sov-cloud"),
				"provenance":  exactly("false"),
				"sbom":        exactly("false"),
				"build-args": exactly(strings.Join([]string{
					"VERSION=${{ steps.stamp.outputs.version }}",
					"COMMIT=${{ steps.stamp.outputs.commit }}",
					"DATE=${{ steps.stamp.outputs.date }}",
				}, "\n")),
			},
		},
		{
			what: "sigstore/cosign-installer", uses: cosignInstallerAction,
			with: map[string]*regexp.Regexp{"cosign-release": exactRelease},
		},
		{
			what: "the signature", name: "Sign the image",
			env: []string{"DIGEST: " + imageDigest},
			run: `cosign sign --yes "` + imageName + `@${DIGEST}"`,
		},
		{
			what: "actions/attest-build-provenance", uses: attestProvenanceAction,
			with: map[string]*regexp.Regexp{
				"subject-name":          exactly(imageName),
				"subject-digest":        exactly(imageDigest),
				"push-to-registry":      exactly("true"),
				"create-storage-record": exactly("false"),
			},
		},
		{
			what: "anchore/sbom-action", uses: sbomAction,
			with: map[string]*regexp.Regexp{
				"image":                 exactly("registry:" + imageName + "@" + imageDigest),
				"format":                exactly("spdx-json"),
				"output-file":           exactly("eusov.spdx.json"),
				"upload-artifact":       exactly("false"),
				"upload-release-assets": exactly("false"),
				"syft-version":          exactRelease,
			},
		},
		{
			what: "actions/attest-sbom", uses: attestSBOMAction,
			with: map[string]*regexp.Regexp{
				"subject-name":     exactly(imageName),
				"subject-digest":   exactly(imageDigest),
				"sbom-path":        exactly("eusov.spdx.json"),
				"push-to-registry": exactly("true"),
			},
		},
	}
)

// releaseJob is one job of the release workflow.
type releaseJob struct {
	key, name string
	// needs is the one job it needs, "" for none.
	needs string
	// permissions is the job's whole permissions: mapping.
	permissions map[string]string
	steps       []releaseStep
}

// releaseJobs are the release workflow's jobs, in the order a message lists
// them. The write permissions stay split: the goreleaser job alone writes the
// repository's contents, the image job alone packages, an OIDC token and
// attestations.
var releaseJobs = []releaseJob{
	{key: "test", name: "Test", permissions: map[string]string{"contents": "read"}, steps: testSteps},
	{
		key: "goreleaser", name: "GoReleaser", needs: "test",
		permissions: map[string]string{"contents": "write"}, steps: goreleaserSteps,
	},
	{
		key: "image", name: "Container image", needs: "test",
		permissions: map[string]string{
			"contents": "read", "packages": "write", "id-token": "write", "attestations": "write",
		},
		steps: imageSteps,
	},
}

// ReleaseWorkflow returns a message for everything in the release workflow
// document src that could make goreleaser build, or the image job publish,
// something other than what the check lists. It reads the document as a
// closed schema:
//
//   - the top level holds exactly name, on (a push of v*.*.* tags),
//     permissions ({}) and jobs, so no workflow-level env: or defaults:;
//   - jobs holds exactly test, goreleaser and image, each with exactly name,
//     runs-on (ubuntu-latest), permissions and steps, and goreleaser and image
//     each needs: test, so no job-level env:, container:, if: or other key;
//   - the permissions are exactly contents: read for test, contents: write
//     for goreleaser, and contents: read, packages: write, id-token: write and
//     attestations: write for image;
//   - the test job's steps are exactly actions/checkout, actions/setup-go and
//     make test; the goreleaser job's exactly actions/checkout,
//     actions/setup-go and goreleaser/goreleaser-action, setup-go with
//     cache: false in both jobs, goreleaser asked for exactly release --clean
//     at an exact version, with an env: of exactly GITHUB_TOKEN from
//     secrets.GITHUB_TOKEN;
//   - the image job's steps are exactly actions/checkout, the stamp of the
//     build arguments (a run: script, with the tag through env:),
//     docker/setup-buildx-action (BuildKit by exact release and index digest),
//     docker/login-action, docker/metadata-action, docker/build-push-action
//     (linux/amd64 and linux/arm64, pushed, no cache, no BuildKit provenance
//     or SBOM), sigstore/cosign-installer (an exact release), cosign sign by
//     digest, actions/attest-build-provenance, anchore/sbom-action (an exact
//     syft, uploading nothing) and actions/attest-sbom, all by the digest
//     built and pushed to the registry;
//   - every step holds exactly the keys listed (name, id, uses, with, env,
//     run), in that order of steps, each action pinned by commit, each with:
//     key and value, env: line and run: command as listed. A run: command is
//     one line or a literal block (|), compared line by line, and holds no
//     ${{ }} expression: what a step takes from the workflow comes in through
//     env:.
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

	raw := strings.Split(string(src), "\n")
	for i := range raw {
		raw[i] = strings.TrimSuffix(raw[i], "\r")
		messages = append(messages, forbiddenNames(strings.TrimSpace(raw[i]))...)
	}

	lines, err := splitLines(src)
	if err != nil {
		return append(messages, fmt.Sprintf("read the release workflow: %v", err))
	}

	return append(messages, releaseSchema(raw, lines)...)
}

// releaseSchema returns a message for every way lines, the content lines of
// the release workflow whose every line is raw, departs from the schema
// ReleaseWorkflow describes.
func releaseSchema(raw []string, lines []line) []string {
	r := &schemaReader{raw: raw}

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

	keys := make([]string, 0, len(releaseJobs))
	for _, j := range releaseJobs {
		keys = append(keys, j.key)
	}

	jobs := r.exactKeys(r.block(top["jobs"], "jobs"), "jobs", keys...)
	if jobs == nil {
		return r.messages
	}

	for _, j := range releaseJobs {
		r.job(jobs[j.key], "jobs: "+j.key, j)
	}

	return r.messages
}

// schemaReader collects the messages of a schema check.
type schemaReader struct {
	// raw are the document's lines, line ends removed, from which a literal
	// block is read.
	raw      []string
	messages []string
}

func (r *schemaReader) fail(format string, args ...any) {
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

// job checks one job against want: its name, needs, runner, permissions and
// steps.
func (r *schemaReader) job(e entry, path string, want releaseJob) {
	keys := []string{"name", "runs-on", "permissions", "steps"}
	if want.needs != "" {
		keys = append(keys, "needs")
	}

	job := r.exactKeys(r.block(e, path), path, keys...)
	if job == nil {
		return
	}

	r.scalar(job["name"], path+": name", exactly(want.name))
	r.scalar(job["runs-on"], path+": runs-on", exactly("ubuntu-latest"))

	if want.needs != "" {
		r.scalar(job["needs"], path+": needs", exactly(want.needs))
	}

	permsPath := path + ": permissions"

	scopes := sortedKeys(want.permissions)
	if perms := r.exactKeys(r.block(job["permissions"], permsPath), permsPath, scopes...); perms != nil {
		for _, scope := range scopes {
			r.scalar(perms[scope], permsPath+": "+scope, exactly(want.permissions[scope]))
		}
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

	if len(items) != len(want.steps) {
		whats := make([]string, 0, len(want.steps))
		for _, s := range want.steps {
			whats = append(whats, s.what)
		}

		r.fail("%s holds %d steps, want exactly %d: %s, so that nothing the check never lists runs in this job",
			stepsPath, len(items), len(want.steps), strings.Join(whats, ", "))

		return
	}

	for i, it := range items {
		r.step(it, fmt.Sprintf("%s: step %d", stepsPath, i+1), want.steps[i])
	}
}

// step checks one step against want.
func (r *schemaReader) step(it item, path string, want releaseStep) {
	path = fmt.Sprintf("%s (line %d), want %s", path, it.num, want.what)

	var keys []string

	for _, k := range []struct {
		key string
		set bool
	}{
		{"name", want.name != ""},
		{"id", want.id != ""},
		{"uses", want.uses != nil},
		{"with", want.with != nil},
		{"env", want.env != nil},
		{"run", want.run != ""},
	} {
		if k.set {
			keys = append(keys, k.key)
		}
	}

	step := r.exactKeys(it.lines, path, keys...)
	if step == nil {
		return
	}

	if want.name != "" {
		r.scalar(step["name"], path+": name", exactly(want.name))
	}

	if want.id != "" {
		r.scalar(step["id"], path+": id", exactly(want.id))
	}

	if want.uses != nil {
		r.scalar(step["uses"], path+": uses", want.uses)
	}

	if want.with != nil {
		withKeys := sortedKeys(want.with)
		if with := r.exactKeys(r.block(step["with"], path+": with"), path+": with", withKeys...); with != nil {
			for _, key := range withKeys {
				r.scalar(with[key], path+": with: "+key, want.with[key])
			}
		}
	}

	if want.env != nil {
		r.env(step["env"], path+": env", want.env)
	}

	if want.run != "" {
		r.run(step["run"], path+": run", want.run)
	}
}

// env checks that e's value is exactly the env: mapping want, line by line:
// its keys are upper case, which the mapping reader refuses.
func (r *schemaReader) env(e entry, path string, want []string) {
	env := r.block(e, path)
	if env == nil {
		return
	}

	got := make([]string, 0, len(env))
	for _, l := range env {
		got = append(got, l.text)
	}

	if !slices.Equal(got, want) {
		r.fail("%s holds %q, want exactly %q, so the step could run with an environment the check never lists", path, got, want)
	}
}

// run checks that e's value is exactly the command want, and refuses a ${{ }}
// expression in it: the runner pastes the expression's value into the script
// before the shell reads it, so a tag or a branch name could run as a command.
func (r *schemaReader) run(e entry, path, want string) {
	value, err := r.value(e)
	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	if strings.Contains(value, "${{") {
		r.fail("line %d: %s holds a ${{ }} expression, whose value the runner pastes into the script as code; pass it through env: instead", e.num, path)
	}

	if value != want {
		r.fail("line %d: %s is %q, want exactly %q", e.num, path, value, want)
	}
}

// sortedKeys returns the keys of m in order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
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

// scalar checks that e's value, one scalar or a literal block, is one that
// want matches.
func (r *schemaReader) scalar(e entry, path string, want *regexp.Regexp) {
	value, err := r.value(e)
	if err != nil {
		r.fail("%s: %v", path, err)

		return
	}

	if !want.MatchString(value) {
		r.fail("line %d: %s is %q, want one matching %s", e.num, path, value, want)
	}
}

// value reads e's value: a literal block (|) as its lines joined by newlines,
// anything else as one scalar on the key's own line.
func (r *schemaReader) value(e entry) (string, error) {
	if e.value == "|" {
		return literalBlock(r.raw, e)
	}

	return single(e)
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

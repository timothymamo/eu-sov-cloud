package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/timothymamo/eu-sov-cloud/test/internal/yamlread"
)

// The actions the dry run and the release share, which must run at the same
// commit in both.
const (
	setupBuildxName = "docker/setup-buildx-action"
	metadataName    = "docker/metadata-action"
	buildPushName   = "docker/build-push-action"
	sbomActionName  = "anchore/sbom-action"
)

// sharedActions are the actions the dry run and the release share.
var sharedActions = []string{setupBuildxName, metadataName, buildPushName, sbomActionName}

// buildSettings are the with: keys of the multi-platform build that the
// release's image rests on, compared between the dry run and the release.
var buildSettings = []string{"platforms", "annotations", "provenance", "sbom"}

// sbomSettings are the with: keys of anchore/sbom-action compared between the
// dry run and the release.
var sbomSettings = []string{"format", "syft-version"}

// The latest decision: the id of the step that makes it in each document, the
// flavor: through which metadata-action reads it, the line of its run: block
// that picks the highest release tag, and the source of the tags that line
// reads in each document.
const (
	releaseLatestID  = "stamp"
	ciLatestID       = "latest"
	releaseFlavor    = "latest=${{ steps." + releaseLatestID + ".outputs.latest }}"
	ciFlavorPrefix   = "latest=${{ steps." + ciLatestID + ".outputs."
	flavorSuffix     = " }}"
	latestFlavor     = "latest=<the latest decision>"
	highestLine      = `highest="$(`
	releaseTagSource = `git tag -l 'v*.*.*' --merged refs/remotes/origin/main`
	ciTagSource      = `printf '%s\n' "$@"`
)

// workflowStep is one step of a job, as the dry run check reads it.
type workflowStep struct {
	num int
	// action is the action the step runs, without its commit; "" for a run:
	// step.
	action string
	// commit is what follows the @ of uses:.
	commit string
	// with are the step's with: entries by key.
	with map[string]yamlread.Entry
	// id is the step's id:, "" for none.
	id string
	// run is the step's run: entry, read only when hasRun.
	run    yamlread.Entry
	hasRun bool
}

// workflowJob is one job, as the dry run check reads it.
type workflowJob struct {
	// env is the job's env: mapping.
	env   map[string]string
	steps []workflowStep
}

// DryRunMirrorsRelease returns a message for every way the image job of
// ci.yml, the dry run, has drifted from the image and sbom jobs of
// release.yml, documents ci and release. What the two publish differs by
// design (an OCI archive against a push, a sample tag against the pushed one),
// so it compares the fields the release's build and SBOM rest on:
//
//   - docker/setup-buildx-action, docker/metadata-action,
//     docker/build-push-action and anchore/sbom-action run at the same
//     commit in both;
//   - the BuildKit image of driver-opts is the same tag and digest, with each
//     ${{ env.NAME }} of the dry run read from its job's env:;
//   - the dry run's multi-platform build (its build-push-action step without
//     load: true, of which there must be exactly one) has the release build's
//     platforms, annotations, provenance and sbom;
//   - every metadata-action step of the dry run has the release's images, and
//     its tags, once the sample tag of their ,value=<sample> is dropped and
//     the sample, quoted or not, is replaced by github.ref_name, are exactly
//     the release's tags;
//   - anchore/sbom-action has the release's format and syft-version;
//   - the release's metadata-action reads latest from its stamp step alone
//     (flavor: latest=${{ steps.stamp.outputs.latest }}), and every one of the
//     dry run's from an output of its own step id: latest;
//   - the line of the stamp's run: block that picks the highest tag, the one
//     starting highest="$(, is the dry run's line in its latest step once the
//     release's tag source, git tag -l 'v*.*.*' --merged
//     refs/remotes/origin/main, is replaced by the dry run's printf '%s\n'
//     "$@": the same filter, sort and pick, on sample tags.
func DryRunMirrorsRelease(ci, release []byte) []string {
	r := &schemaReader{}

	ciJob, ok := r.readJob(ci, "ci.yml", "image")
	if !ok {
		return r.messages
	}

	imageJob, ok := r.readJob(release, "release.yml", "image")
	if !ok {
		return r.messages
	}

	sbomJob, ok := r.readJob(release, "release.yml", "sbom")
	if !ok {
		return r.messages
	}

	releaseSteps := slices.Concat(imageJob.steps, sbomJob.steps)

	d := dryRun{r: r, ciRaw: rawLines(ci), releaseRaw: rawLines(release)}

	d.pins(ciJob.steps, releaseSteps)
	d.buildkit(ciJob, releaseSteps)
	d.build(ciJob.steps, releaseSteps)
	d.tags(ciJob.steps, releaseSteps)
	d.flavor(ciJob.steps, releaseSteps)
	d.highest(ciJob.steps, releaseSteps)
	d.sbom(ciJob.steps, releaseSteps)

	return r.messages
}

// dryRun compares the dry run with the release.
type dryRun struct {
	r                 *schemaReader
	ciRaw, releaseRaw []string
}

// rawLines returns the lines of doc with their line ends removed.
func rawLines(doc []byte) []string {
	raw := strings.Split(string(doc), "\n")
	for i := range raw {
		raw[i] = strings.TrimSuffix(raw[i], "\r")
	}

	return raw
}

// readJob reads the job key of the workflow doc, called file in a message.
func (r *schemaReader) readJob(doc []byte, file, key string) (workflowJob, bool) {
	lines, err := yamlread.SplitLines(doc)
	if err != nil {
		r.fail("%s: %v", file, err)

		return workflowJob{}, false
	}

	block, err := yamlread.Walk(lines, "jobs", key)
	if err != nil {
		r.fail("%s: %v", file, err)

		return workflowJob{}, false
	}

	entries, err := yamlread.Mapping(block)
	if err != nil {
		r.fail("%s: jobs: %s: %v", file, key, err)

		return workflowJob{}, false
	}

	job := workflowJob{env: map[string]string{}}
	path := file + ": jobs: " + key

	for _, e := range entries {
		switch e.Key {
		case "env":
			env, ok := r.jobEnv(e, path+": env")
			if !ok {
				return workflowJob{}, false
			}

			job.env = env
		case "steps":
			steps, ok := r.workflowSteps(e, path+": steps")
			if !ok {
				return workflowJob{}, false
			}

			job.steps = steps
		}
	}

	return job, true
}

// jobEnv reads e, a job's env:, as one line per variable: an upper-case name
// and a scalar.
func (r *schemaReader) jobEnv(e yamlread.Entry, path string) (map[string]string, bool) {
	lines := r.block(e, path)
	if lines == nil {
		return nil, false
	}

	env := map[string]string{}

	for _, l := range lines {
		name, rest, found := strings.Cut(l.Text, ": ")
		if l.Indent != lines[0].Indent || !found || name == "" || strings.Trim(name, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_") != "" {
			r.fail("line %d: %s: %q is not one variable this reader understands", l.Num, path, l.Text)

			return nil, false
		}

		value, err := yamlread.Scalar(strings.TrimSpace(rest))
		if err != nil {
			r.fail("line %d: %s: %s: %v", l.Num, path, name, err)

			return nil, false
		}

		if _, repeated := env[name]; repeated {
			r.fail("line %d: %s: %s is repeated", l.Num, path, name)

			return nil, false
		}

		env[name] = value
	}

	return env, true
}

// workflowSteps reads e, a job's steps:, keeping each step's id:, uses:,
// with: and run:.
func (r *schemaReader) workflowSteps(e yamlread.Entry, path string) ([]workflowStep, bool) {
	block := r.block(e, path)
	if block == nil {
		return nil, false
	}

	items, err := yamlread.Sequence(block)
	if err != nil {
		r.fail("%s: %v", path, err)

		return nil, false
	}

	steps := make([]workflowStep, 0, len(items))

	for i, it := range items {
		stepPath := fmt.Sprintf("%s: step %d (line %d)", path, i+1, it.Num)

		entries, err := yamlread.Mapping(it.Lines)
		if err != nil {
			r.fail("%s: %v", stepPath, err)

			return nil, false
		}

		step := workflowStep{num: it.Num, with: map[string]yamlread.Entry{}}

		for _, se := range entries {
			switch se.Key {
			case "id":
				id, err := yamlread.Single(se)
				if err != nil {
					r.fail("%s: %v", stepPath, err)

					return nil, false
				}

				step.id = id
			case "run":
				step.run, step.hasRun = se, true
			case "uses":
				uses, err := yamlread.Single(se)
				if err != nil {
					r.fail("%s: %v", stepPath, err)

					return nil, false
				}

				step.action, step.commit, _ = strings.Cut(uses, "@")
			case "with":
				with, err := yamlread.Mapping(r.block(se, stepPath+": with"))
				if err != nil {
					r.fail("%s: with: %v", stepPath, err)

					return nil, false
				}

				for _, w := range with {
					step.with[w.Key] = w
				}
			}
		}

		steps = append(steps, step)
	}

	return steps, true
}

// stepsOf returns the steps of steps that run action.
func stepsOf(steps []workflowStep, action string) []workflowStep {
	var out []workflowStep

	for _, s := range steps {
		if s.action == action {
			out = append(out, s)
		}
	}

	return out
}

// one returns the one step of steps that runs action in file's job.
func (d dryRun) one(steps []workflowStep, file, action string) (workflowStep, bool) {
	found := stepsOf(steps, action)
	if len(found) != 1 {
		d.r.fail("%s holds %d %s steps, want exactly 1", file, len(found), action)

		return workflowStep{}, false
	}

	return found[0], true
}

// withValue reads key of the with: of s, a step of the document raw, called
// path in a message.
func (d dryRun) withValue(raw []string, s workflowStep, path, key string) (string, bool) {
	e, found := s.with[key]
	if !found {
		d.r.fail("%s (line %d) holds no with: %s", path, s.num, key)

		return "", false
	}

	value, err := (&schemaReader{raw: raw}).value(e)
	if err != nil {
		d.r.fail("%s (line %d): with: %s: %v", path, s.num, key, err)

		return "", false
	}

	return value, true
}

// pins checks that every shared action runs at one commit in both documents.
func (d dryRun) pins(ci, release []workflowStep) {
	for _, action := range sharedActions {
		ciSteps, releaseSteps := stepsOf(ci, action), stepsOf(release, action)
		if len(ciSteps) == 0 || len(releaseSteps) == 0 {
			d.r.fail("%s runs in %d steps of ci.yml's image job and %d of release.yml's image and sbom jobs, want both", action, len(ciSteps), len(releaseSteps))

			continue
		}

		want := releaseSteps[0].commit

		for _, s := range slices.Concat(ciSteps, releaseSteps) {
			if s.commit != want {
				d.r.fail("%s runs at %s (line %d) and at %s (line %d), want one commit in ci.yml and release.yml", action, s.commit, s.num, want, releaseSteps[0].num)
			}
		}
	}
}

// buildkit checks that the dry run's BuildKit image, its env: read, is the
// release's.
func (d dryRun) buildkit(ci workflowJob, release []workflowStep) {
	ciStep, ok := d.one(ci.steps, "ci.yml's image job", setupBuildxName)
	if !ok {
		return
	}

	releaseStep, ok := d.one(release, "release.yml's image job", setupBuildxName)
	if !ok {
		return
	}

	got, ok := d.withValue(d.ciRaw, ciStep, "ci.yml: "+setupBuildxName, "driver-opts")
	if !ok {
		return
	}

	want, ok := d.withValue(d.releaseRaw, releaseStep, "release.yml: "+setupBuildxName, "driver-opts")
	if !ok {
		return
	}

	resolved, err := resolveEnv(got, ci.env)
	if err != nil {
		d.r.fail("ci.yml: %s (line %d): driver-opts: %v", setupBuildxName, ciStep.num, err)

		return
	}

	if resolved != want {
		d.r.fail("ci.yml runs BuildKit %q (line %d), release.yml %q (line %d): the dry run must build with the release's moby/buildkit tag and digest", resolved, ciStep.num, want, releaseStep.num)
	}
}

// resolveEnv returns value with every ${{ env.NAME }} replaced by NAME's value
// in env. It refuses any other expression and a name env does not hold.
func resolveEnv(value string, env map[string]string) (string, error) {
	var b strings.Builder

	for {
		start := strings.Index(value, "${{")
		if start < 0 {
			b.WriteString(value)

			return b.String(), nil
		}

		end := strings.Index(value[start:], "}}")
		if end < 0 {
			return "", fmt.Errorf("%q does not close its expression", value)
		}

		expr := strings.TrimSpace(value[start+3 : start+end])

		name, found := strings.CutPrefix(expr, "env.")
		if !found {
			return "", fmt.Errorf("the expression %q is not an env. variable", expr)
		}

		resolved, found := env[name]
		if !found {
			return "", fmt.Errorf("the job's env: holds no %s", name)
		}

		b.WriteString(value[:start])
		b.WriteString(resolved)
		value = value[start+end+2:]
	}
}

// build checks the dry run's multi-platform build against the release's.
func (d dryRun) build(ci, release []workflowStep) {
	var oci []workflowStep

	for _, s := range stepsOf(ci, buildPushName) {
		load, found := s.with["load"]
		if found {
			if value, err := yamlread.Single(load); err == nil && value == "true" {
				continue
			}
		}

		oci = append(oci, s)
	}

	ciStep, ok := d.one(oci, "ci.yml's image job, load: true left out,", buildPushName)
	if !ok {
		return
	}

	releaseStep, ok := d.one(release, "release.yml's image job", buildPushName)
	if !ok {
		return
	}

	for _, key := range buildSettings {
		d.same(ciStep, releaseStep, buildPushName, key)
	}
}

// same checks that key of the with: of the two steps is the same.
func (d dryRun) same(ciStep, releaseStep workflowStep, action, key string) {
	got, ok := d.withValue(d.ciRaw, ciStep, "ci.yml: "+action, key)
	if !ok {
		return
	}

	want, ok := d.withValue(d.releaseRaw, releaseStep, "release.yml: "+action, key)
	if !ok {
		return
	}

	if got != want {
		d.r.fail("ci.yml: %s (line %d): with: %s is %q, release.yml's (line %d) %q: the dry run must build what the release builds", action, ciStep.num, key, got, releaseStep.num, want)
	}
}

// tags checks every metadata-action step of the dry run against the
// release's: the same images, and the same tags for its sample tag.
func (d dryRun) tags(ci, release []workflowStep) {
	releaseStep, ok := d.one(release, "release.yml's image job", metadataName)
	if !ok {
		return
	}

	want, ok := d.withValue(d.releaseRaw, releaseStep, "release.yml: "+metadataName, "tags")
	if !ok {
		return
	}

	ciSteps := stepsOf(ci, metadataName)
	if len(ciSteps) == 0 {
		d.r.fail("ci.yml's image job holds no %s step, so the dry run checks none of the release's tags", metadataName)
	}

	for _, s := range ciSteps {
		d.same(s, releaseStep, metadataName, "images")

		got, ok := d.withValue(d.ciRaw, s, "ci.yml: "+metadataName, "tags")
		if !ok {
			continue
		}

		general, err := generalTags(got)
		if err != nil {
			d.r.fail("ci.yml: %s (line %d): with: tags: %v", metadataName, s.num, err)

			continue
		}

		if general != want {
			d.r.fail("ci.yml: %s (line %d): with: tags are %q for github.ref_name, release.yml's (line %d) %q: the dry run must check the release's tag rules", metadataName, s.num, general, releaseStep.num, want)
		}
	}
}

// generalTags returns tags, the tag rules of a dry run for one sample tag,
// as the release writes them: ,value=<sample> dropped from every rule, then
// the sample, quoted or not, replaced by github.ref_name.
func generalTags(tags string) (string, error) {
	_, after, found := strings.Cut(tags, ",value=")
	if !found {
		return "", fmt.Errorf("%q holds no ,value=<sample>", tags)
	}

	sample, _, _ := strings.Cut(after, ",")
	sample, _, _ = strings.Cut(sample, "\n")

	if sample == "" {
		return "", fmt.Errorf("%q holds an empty ,value=", tags)
	}

	tags = strings.ReplaceAll(tags, ",value="+sample, "")
	tags = strings.ReplaceAll(tags, "'"+sample+"'", "github.ref_name")

	return strings.ReplaceAll(tags, sample, "github.ref_name"), nil
}

// sbom checks the dry run's SBOM settings against the release's.
func (d dryRun) sbom(ci, release []workflowStep) {
	ciStep, ok := d.one(ci, "ci.yml's image job", sbomActionName)
	if !ok {
		return
	}

	releaseStep, ok := d.one(release, "release.yml's sbom job", sbomActionName)
	if !ok {
		return
	}

	for _, key := range sbomSettings {
		d.same(ciStep, releaseStep, sbomActionName, key)
	}
}

// flavor checks that the release's metadata-action reads latest from its stamp
// step and every one of the dry run's from an output of its latest step.
func (d dryRun) flavor(ci, release []workflowStep) {
	releaseStep, ok := d.one(release, "release.yml's image job", metadataName)
	if !ok {
		return
	}

	want, ok := d.withValue(d.releaseRaw, releaseStep, "release.yml: "+metadataName, "flavor")
	if !ok {
		return
	}

	if want != releaseFlavor {
		d.r.fail("release.yml: %s (line %d): with: flavor is %q, want exactly %q: the dry run must tag latest as the release does, so the release must read latest from its stamp", metadataName, releaseStep.num, want, releaseFlavor)

		return
	}

	for _, s := range stepsOf(ci, metadataName) {
		got, ok := d.withValue(d.ciRaw, s, "ci.yml: "+metadataName, "flavor")
		if !ok {
			continue
		}

		if normaliseCIFlavor(got) != latestFlavor {
			d.r.fail("ci.yml: %s (line %d): with: flavor is %q, want %s<name>%s: the dry run must tag latest as the release does, from its step id: %s", metadataName, s.num, got, ciFlavorPrefix, flavorSuffix, ciLatestID)
		}
	}
}

// normaliseCIFlavor returns latestFlavor for a flavor: that reads latest from
// a named output of the dry run's latest step, and flavor unchanged otherwise.
func normaliseCIFlavor(flavor string) string {
	name, found := strings.CutPrefix(flavor, ciFlavorPrefix)
	if !found {
		return flavor
	}

	name, found = strings.CutSuffix(name, flavorSuffix)
	if !found || name == "" || strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789_-") != "" {
		return flavor
	}

	return latestFlavor
}

// highest checks that the dry run picks the highest tag with the release's
// pipeline, on its sample tags in place of the release's tags on main.
func (d dryRun) highest(ci, release []workflowStep) {
	want, wantNum, ok := d.highestLine(d.releaseRaw, release, "release.yml's image job", releaseLatestID)
	if !ok {
		return
	}

	got, gotNum, ok := d.highestLine(d.ciRaw, ci, "ci.yml's image job", ciLatestID)
	if !ok {
		return
	}

	if strings.ReplaceAll(want, releaseTagSource, ciTagSource) != got {
		d.r.fail("ci.yml (line %d) picks the highest tag with %q, release.yml (line %d) with %q: the dry run must pick the highest tag the release picks, with %s in place of %s", gotNum, got, wantNum, want, ciTagSource, releaseTagSource)
	}
}

// highestLine returns the line, trimmed, of the run: block (|) of the step id
// of steps, in the document raw, that starts highest="$(, and the step's line.
func (d dryRun) highestLine(raw []string, steps []workflowStep, file, id string) (string, int, bool) {
	var found []workflowStep

	for _, s := range steps {
		if s.id == id {
			found = append(found, s)
		}
	}

	if len(found) != 1 || !found[0].hasRun {
		d.r.fail("%s holds %d run: steps with id: %s, want exactly 1: the dry run must pick the highest tag the release picks", file, len(found), id)

		return "", 0, false
	}

	s := found[0]

	// The dry run's script opens with a comment line inside its block.
	script, err := yamlread.LiteralBlockLeadingHashes(raw, s.run)
	if err != nil {
		d.r.fail("%s: step id: %s (line %d): run: %v", file, id, s.num, err)

		return "", 0, false
	}

	var lines []string

	for l := range strings.SplitSeq(script, "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, highestLine) {
			lines = append(lines, l)
		}
	}

	if len(lines) != 1 {
		d.r.fail("%s: step id: %s (line %d) holds %d lines starting %s, want exactly 1: the dry run must pick the highest tag the release picks", file, id, s.num, len(lines), highestLine)

		return "", 0, false
	}

	return lines[0], s.num, true
}

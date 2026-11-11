package container_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const (
	dockerfileName     = "Dockerfile"
	dockerignoreName   = ".dockerignore"
	goModName          = "go.mod"
	goreleaserName     = ".goreleaser.yaml"
	versionPackagePath = "github.com/timothymamo/eu-sov-cloud/internal/version"
)

// TestBuilderGoVersionMatchesGoMod pins that the builder stage is the golang
// image of the Go version go.mod declares, so the image and a local build
// compile with the same toolchain. A tag that only starts with the same major
// and minor version, such as golang:1.27, is not enough: it moves to another
// patch release without a change here.
func TestBuilderGoVersionMatchesGoMod(t *testing.T) {
	t.Parallel()

	want := goDirectiveVersion(t, readRepoFile(t, goModName))
	dockerfile := readRepoFile(t, dockerfileName)

	var builders []baseImage
	for _, image := range fromImages(dockerfileInstructions(t, dockerfile)) {
		if strings.HasPrefix(image.image, "golang:") {
			builders = append(builders, image)
		}
	}

	if len(builders) == 0 {
		t.Fatalf("%s: no stage starts FROM golang:<version>, so there is no builder stage to compare with %s", dockerfileName, goModName)
	}

	for _, builder := range builders {
		tag := imageTag(builder.image)

		if tag != want && !strings.HasPrefix(tag, want+"-") {
			t.Errorf("%s:%d: the builder stage uses the image %q, whose tag %q is not the Go version %s declares (go %s): want golang:%s or golang:%s-<variant>, so the image is built with the toolchain the module requires",
				dockerfileName, builder.line, builder.image, tag, goModName, want, want, want)
		}
	}
}

// TestEveryBaseImageIsDigestPinned pins that every FROM instruction names its
// image by digest, so a rebuild uses the same base image and Renovate updates
// the pin (CLAUDE.md rule 7). A reference to a stage declared earlier, and
// scratch, name no registry image and are left out.
func TestEveryBaseImageIsDigestPinned(t *testing.T) {
	t.Parallel()

	dockerfile := readRepoFile(t, dockerfileName)
	images := fromImages(dockerfileInstructions(t, dockerfile))

	if len(images) == 0 {
		t.Fatalf("%s: no FROM instruction, so there is no base image to pin", dockerfileName)
	}

	digest := regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
	declared := make(map[string]struct{}, len(images))

	for _, image := range images {
		_, earlierStage := declared[image.image]

		switch {
		case earlierStage, strings.EqualFold(image.image, "scratch"):
		case digest.MatchString(image.image):
		default:
			t.Errorf("%s:%d: the base image %q is not pinned by digest: want <image>@sha256:<64 hex characters>, so a rebuild uses the same base image",
				dockerfileName, image.line, image.image)
		}

		if image.stage != "" {
			declared[image.stage] = struct{}{}
		}
	}
}

// TestBuildMatchesGoReleaser pins that the image's build carries the settings
// .goreleaser.yaml gives the release binary: cgo off, -trimpath, the -s -w
// linker flags and the internal/version -X flags, the same main package, and a
// cross build from TARGETOS and TARGETARCH. The test reads the settings from
// .goreleaser.yaml rather than pinning a copy of them here.
func TestBuildMatchesGoReleaser(t *testing.T) {
	t.Parallel()

	settings := parseBuildSettings(t, readRepoFile(t, goreleaserName))
	dockerfile := joinedInstructions(dockerfileInstructions(t, readRepoFile(t, dockerfileName)))

	settingsToMatch := []struct {
		setting string
		list    string
		in      []string
	}{
		{setting: "CGO_ENABLED=0", list: "env", in: settings.env},
		{setting: "-trimpath", list: "flags", in: settings.flags},
		{setting: "-s -w", list: "ldflags", in: settings.ldflags},
	}

	for _, tc := range settingsToMatch {
		if !slices.Contains(tc.in, tc.setting) {
			t.Errorf("%s: the builds entry no longer sets %q in its %s list, so this test has nothing of the release build to match",
				goreleaserName, tc.setting, tc.list)

			continue
		}

		if !strings.Contains(dockerfile, tc.setting) {
			t.Errorf("%s: the build does not set %q, which %s sets for the release binary, so the image and the release are built differently",
				dockerfileName, tc.setting, goreleaserName)
		}
	}

	prefixes := xFlagPrefixes(settings.ldflags)
	for _, name := range []string{"version", "commit", "date"} {
		want := versionPackagePath + "." + name + "="
		if !slices.Contains(prefixes, want) {
			t.Errorf("%s: ldflags no longer carry -X %s, so this test cannot check that the image stamps %s",
				goreleaserName, want, name)
		}
	}

	for _, prefix := range prefixes {
		flag := "-X " + prefix
		if !strings.Contains(dockerfile, flag) {
			t.Errorf("%s: the build does not pass the linker flag %q, which %s sets for the release binary; the image must take the value from a build arg, so only the part up to the = is compared",
				dockerfileName, flag, goreleaserName)
		}
	}

	if !strings.Contains(dockerfile, "./cmd/eusov") {
		t.Errorf("%s: the build does not compile ./cmd/eusov, the main package %s builds the release binary from",
			dockerfileName, goreleaserName)
	}

	for _, tc := range []struct{ env, buildArg string }{
		{env: "GOOS", buildArg: "TARGETOS"},
		{env: "GOARCH", buildArg: "TARGETARCH"},
	} {
		if !usesBuildArg(dockerfile, tc.env, tc.buildArg) {
			t.Errorf("%s: the build does not set %s from %s (as $%s or ${%s}), so docker buildx --platform builds every platform for the builder's own",
				dockerfileName, tc.env, tc.buildArg, tc.buildArg, tc.buildArg)
		}
	}
}

// TestFinalStageIsMinimalAndNonRoot pins the last stage of the Dockerfile: a
// base image with nothing to install, patch or run but the binary and the CA
// certificates; a non-root user; the binary as the exec-form entrypoint; and the
// OCI labels that say where the image comes from and under which license.
func TestFinalStageIsMinimalAndNonRoot(t *testing.T) {
	t.Parallel()

	instructions := dockerfileInstructions(t, readRepoFile(t, dockerfileName))
	final := finalStage(t, instructions)

	base := fromImages(final)[0]
	if !strings.EqualFold(base.image, "scratch") && !strings.HasPrefix(base.image, "gcr.io/distroless/static") {
		t.Errorf("%s:%d: the final stage starts FROM %q: want scratch or gcr.io/distroless/static, which hold no shell, package manager or Go toolchain",
			dockerfileName, base.line, base.image)
	}

	user, ok := lastInstruction(final, "USER")
	switch {
	case !ok:
		t.Errorf("%s: the final stage has no USER instruction: want a non-root user such as USER 65532:65532, so the process does not run as root",
			dockerfileName)
	default:
		// Docker reads the user as <name>[:<group>]; a name that is root, or
		// the root uid, runs the process as root whatever the group is.
		name, _, _ := strings.Cut(user.arguments(), ":")
		if name == "" || name == "root" || name == "0" {
			t.Errorf("%s:%d: the final stage runs as USER %q: want a non-root user such as nonroot or 65532:65532",
				dockerfileName, user.line, user.arguments())
		}
	}

	entrypoint, ok := lastInstruction(final, "ENTRYPOINT")
	switch {
	case !ok:
		t.Errorf("%s: the final stage has no ENTRYPOINT: want ENTRYPOINT [\"/eusov\"] in exec form, so the binary runs without a shell",
			dockerfileName)
	default:
		var argv []string
		if err := json.Unmarshal([]byte(entrypoint.arguments()), &argv); err != nil {
			t.Errorf("%s:%d: ENTRYPOINT %s is not in exec (JSON array) form, so Docker wraps it in /bin/sh, which the image does not hold: %v",
				dockerfileName, entrypoint.line, entrypoint.arguments(), err)
		} else if diff := cmp.Diff([]string{"/eusov"}, argv); diff != "" {
			t.Errorf("%s:%d: ENTRYPOINT mismatch (-want +got):\n%s",
				dockerfileName, entrypoint.line, diff)
		}
	}

	labels := finalStageLabels(final)
	for _, key := range []string{
		"org.opencontainers.image.source",
		"org.opencontainers.image.version",
		"org.opencontainers.image.revision",
		"org.opencontainers.image.licenses",
	} {
		value, ok := labels[key]
		switch {
		case !ok:
			t.Errorf("%s: the final stage has no LABEL %s: want the four OCI labels, source, version, revision and licenses",
				dockerfileName, key)
		case value == "":
			t.Errorf("%s: LABEL %s is empty: want a value, set from a build arg where it changes per release",
				dockerfileName, key)
		}
	}
}

// TestDockerignoreKeepsDataOutOfContext pins that .dockerignore keeps the git
// data, the worktrees, the build output and the local inventories and reports
// out of the build context, so no layer can hold real infrastructure data
// (CLAUDE.md rule 2). The inventory and report patterns are matched loosely: any
// entry mentioning them counts, as only some globs are needed.
func TestDockerignoreKeepsDataOutOfContext(t *testing.T) {
	t.Parallel()

	entries := dockerignoreEntries(t, readRepoFile(t, dockerignoreName))

	for _, want := range []string{".git", ".worktrees", "bin", "coverage.out"} {
		if !slices.Contains(entries, want) {
			t.Errorf("%s: no entry %q: the build context carries it, and no layer of the image may hold git data, a worktree, a local build or a coverage report",
				dockerignoreName, want)
		}
	}

	for _, want := range []string{"inventory", "report"} {
		mention := func(entry string) bool {
			return strings.Contains(strings.ToLower(entry), want)
		}
		if !slices.ContainsFunc(entries, mention) {
			t.Errorf("%s: no entry mentions %q: a local inventory or report would be sent to the daemon and could land in a layer, which CLAUDE.md rule 2 forbids",
				dockerignoreName, want)
		}
	}
}

// baseImage is one FROM instruction: the image it starts the stage from and the
// stage name it declares, if any.
type baseImage struct {
	line  int
	image string
	stage string
}

// instruction is one Dockerfile instruction, its line continuations joined and
// its whitespace collapsed, with the line it starts on for failure messages.
type instruction struct {
	line int
	text string
}

// keyword returns the instruction's upper-case keyword, such as FROM or LABEL.
func (in instruction) keyword() string {
	keyword, _, _ := strings.Cut(in.text, " ")
	return strings.ToUpper(keyword)
}

// arguments returns everything after the keyword.
func (in instruction) arguments() string {
	_, args, _ := strings.Cut(in.text, " ")
	return args
}

// buildSettings is the part of .goreleaser.yaml's builds entry the image's
// build must match.
type buildSettings struct {
	env     []string
	flags   []string
	ldflags []string
}

// repoRoot returns the module root: the first directory above the directory the
// test runs in that holds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get the working directory: %v", err)
	}

	for start := dir; ; {
		_, err := os.Stat(filepath.Join(dir, goModName))
		switch {
		case err == nil:
			return dir
		case !errors.Is(err, fs.ErrNotExist):
			t.Fatalf("stat %s: %v", filepath.Join(dir, goModName), err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no %s above %s, so the repository root is not found", goModName, start)
		}

		dir = parent
	}
}

// readRepoFile reads the file name under the repository root, and fails the
// test naming it when it is missing or unreadable: every test here pins one of
// the build files.
func readRepoFile(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("%s: read from the repository root: %v; the container build needs it at the root", name, err)
	}

	return string(data)
}

// goDirectiveVersion returns the version of go.mod's go directive, "1.27.4"
// for "go 1.27.4".
func goDirectiveVersion(t *testing.T, goMod string) string {
	t.Helper()

	for _, line := range strings.Split(goMod, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "go" {
			return fields[1]
		}
	}

	t.Fatalf("%s: no go directive: want a line such as \"go 1.27.4\", which the builder image's tag must match", goModName)

	return ""
}

// dockerfileInstructions parses the Dockerfile into its instructions. Blank
// lines and comments are dropped, and a line ending in a backslash is joined to
// the next, so a LABEL continued over several lines reads as one instruction.
func dockerfileInstructions(t *testing.T, dockerfile string) []instruction {
	t.Helper()

	var (
		instructions []instruction
		current      strings.Builder
		start        int
	)

	for i, raw := range strings.Split(dockerfile, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))

		// Docker removes comment lines before it joins continuations, so a
		// comment inside a continued instruction is dropped too.
		if strings.HasPrefix(line, "#") {
			continue
		}

		if line == "" && current.Len() == 0 {
			continue
		}

		if current.Len() == 0 {
			start = i + 1
		}

		continued := strings.HasSuffix(line, `\`)
		current.WriteString(" ")
		current.WriteString(strings.TrimSuffix(line, `\`))

		if continued {
			continue
		}

		instructions = append(instructions, instruction{
			line: start,
			text: strings.Join(strings.Fields(current.String()), " "),
		})
		current.Reset()
	}

	if current.Len() != 0 {
		t.Errorf("%s:%d: the instruction ends in a line continuation with no next line", dockerfileName, start)
	}

	if len(instructions) == 0 {
		t.Fatalf("%s: no instruction, so there is nothing to build", dockerfileName)
	}

	return instructions
}

// fromImages returns the FROM instruction of every stage, in file order. The
// --platform flag a FROM may carry is skipped.
func fromImages(instructions []instruction) []baseImage {
	var images []baseImage

	for _, in := range instructions {
		if in.keyword() != "FROM" {
			continue
		}

		fields := strings.Fields(in.arguments())
		for len(fields) > 0 && strings.HasPrefix(fields[0], "--") {
			fields = fields[1:]
		}

		if len(fields) == 0 {
			continue
		}

		image := baseImage{line: in.line, image: fields[0]}
		if len(fields) >= 3 && strings.EqualFold(fields[1], "AS") {
			image.stage = strings.ToLower(fields[2])
		}

		images = append(images, image)
	}

	return images
}

// finalStage returns the instructions of the last stage, the ones after the
// last FROM, and fails when the Dockerfile has no FROM at all.
func finalStage(t *testing.T, instructions []instruction) []instruction {
	t.Helper()

	last := -1
	for i, in := range instructions {
		if in.keyword() == "FROM" {
			last = i
		}
	}

	if last < 0 {
		t.Fatalf("%s: no FROM instruction, so there is no final stage", dockerfileName)
	}

	return instructions[last:]
}

// lastInstruction returns the last instruction of the given keyword, which is
// the one Docker applies when the same instruction appears twice.
func lastInstruction(instructions []instruction, keyword string) (instruction, bool) {
	found := false
	var last instruction

	for _, in := range instructions {
		if in.keyword() == keyword {
			last, found = in, true
		}
	}

	return last, found
}

// joinedInstructions returns the instructions as text, one per line, with the
// continuations already joined: what the build settings of the Dockerfile are
// read from.
func joinedInstructions(instructions []instruction) string {
	lines := make([]string, 0, len(instructions))
	for _, in := range instructions {
		lines = append(lines, in.text)
	}

	return strings.Join(lines, "\n")
}

// imageTag returns the tag of a golang image reference, "1.27.4-bookworm" for
// "golang:1.27.4-bookworm@sha256:...".
func imageTag(image string) string {
	tag := strings.TrimPrefix(image, "golang:")
	tag, _, _ = strings.Cut(tag, "@")

	return tag
}

// usesBuildArg reports whether the build reads env (GOOS, GOARCH) from the
// buildx platform argument, as GOOS=$TARGETOS or GOOS=${TARGETOS}.
func usesBuildArg(dockerfile, env, buildArg string) bool {
	return strings.Contains(dockerfile, env+"=$"+buildArg) ||
		strings.Contains(dockerfile, env+"=${"+buildArg+"}")
}

// parseBuildSettings reads the env, flags and ldflags lists of .goreleaser.yaml
// line by line: a key ending in a colon, then the "- value" items indented under
// it. A YAML library would be a dependency for one file (CLAUDE.md rule 6).
func parseBuildSettings(t *testing.T, goreleaser string) buildSettings {
	t.Helper()

	var settings buildSettings
	var list *[]string

	for _, raw := range strings.Split(goreleaser, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))

		switch {
		case line == "", strings.HasPrefix(line, "#"), line == "---":
			continue

		case strings.HasPrefix(line, "- "):
			if list != nil {
				*list = append(*list, unquote(strings.TrimSpace(line[2:])))
			}

		case strings.HasSuffix(line, ":"):
			list = nil

			switch key := strings.TrimSuffix(line, ":"); key {
			case "env":
				list = &settings.env
			case "flags":
				list = &settings.flags
			case "ldflags":
				list = &settings.ldflags
			}

		default:
			list = nil
		}
	}

	if len(settings.env) == 0 || len(settings.flags) == 0 || len(settings.ldflags) == 0 {
		t.Fatalf("%s: the builds entry has env=%v, flags=%v and ldflags=%v: the test reads the image's build settings from all three lists, so each must hold an item",
			goreleaserName, settings.env, settings.flags, settings.ldflags)
	}

	return settings
}

// unquote removes the matching quotes YAML allows around a scalar.
func unquote(value string) string {
	if len(value) >= 2 {
		if first, last := value[0], value[len(value)-1]; (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}

	return value
}

// xFlagPrefixes returns the path prefix of every linker -X flag in ldflags, the
// part up to and including the =: GoReleaser fills the value from a template,
// the Dockerfile from a build arg, so only the prefix can match.
func xFlagPrefixes(ldflags []string) []string {
	var prefixes []string

	for _, flag := range ldflags {
		fields := strings.Fields(flag)

		for i := 0; i+1 < len(fields); i++ {
			if fields[i] != "-X" {
				continue
			}

			if path, _, found := strings.Cut(fields[i+1], "="); found {
				prefixes = append(prefixes, path+"=")
			}
		}
	}

	return prefixes
}

// labelPair matches one key=value pair of a LABEL instruction, the value quoted
// with either quote character or bare.
var labelPair = regexp.MustCompile(`([A-Za-z0-9][A-Za-z0-9._-]*)=(?:"([^"]*)"|'([^']*)'|(\S+))`)

// finalStageLabels returns the labels the final stage sets, its last value
// winning when a key appears twice.
func finalStageLabels(instructions []instruction) map[string]string {
	labels := make(map[string]string)

	for _, in := range instructions {
		if in.keyword() != "LABEL" {
			continue
		}

		for _, pair := range labelPair.FindAllStringSubmatch(in.arguments(), -1) {
			for _, value := range pair[2:] {
				if value != "" {
					labels[pair[1]] = value

					break
				}
			}

			if _, set := labels[pair[1]]; !set {
				labels[pair[1]] = ""
			}
		}
	}

	return labels
}

// dockerignoreEntries returns the normalised lines of .dockerignore: comments and
// blank lines dropped, a leading or trailing slash removed, so "/bin/" reads as
// the entry "bin".
func dockerignoreEntries(t *testing.T, dockerignore string) []string {
	t.Helper()

	var entries []string

	for _, raw := range strings.Split(dockerignore, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		entries = append(entries, strings.Trim(strings.TrimPrefix(line, "/"), "/"))
	}

	if len(entries) == 0 {
		t.Fatalf("%s: no entry, so the build context holds the git data, the worktrees, the build output and any local inventory or report",
			dockerignoreName)
	}

	return entries
}

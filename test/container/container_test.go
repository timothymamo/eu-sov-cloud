package container_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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

// TestBuilderStageCrossCompilesOnBuildPlatform pins that the stage running
// `go build` starts FROM --platform=$BUILDPLATFORM, so `docker buildx build
// --platform linux/arm64` compiles the binary on the build host and
// cross-compiles it for the target, instead of running the whole builder stage
// under QEMU emulation (docs/adr/0013-ship-eusov-as-a-distroless-container-image.md).
// The builder is found by the go build it runs rather than by its name, so a
// renamed or reordered stage is still the one checked.
func TestBuilderStageCrossCompilesOnBuildPlatform(t *testing.T) {
	t.Parallel()

	assertBuilderCrossCompiles(t, readRepoFile(t, dockerfileName))
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

	if isDebugBaseImage(base.image) {
		t.Errorf("%s:%d: the final stage starts FROM %q, the debug variant of a distroless image: :debug and :debug-nonroot ship a busybox shell and a package manager's leftovers, contradicting the image's promise of no shell; want the same image under its non-debug tag, such as :nonroot",
			dockerfileName, base.line, base.image)
	}

	// The builder cross-compiles for $BUILDPLATFORM, so CI needs no QEMU
	// emulation. A RUN in the final stage would need it, and a shell distroless
	// does not have.
	for _, in := range final {
		if in.keyword() == "RUN" {
			t.Errorf("%s:%d: the final stage has a RUN instruction, %q: want none, because the builder cross-compiles on $BUILDPLATFORM, so CI needs no QEMU emulation, and a RUN in the final stage would need it as well as a shell, which distroless does not have; do the work in the builder and COPY its result",
				dockerfileName, in.line, in.text)
		}
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

// TestDebugBaseImageClassification exercises the check that keeps the final
// stage off the debug variant of a distroless image. Distroless publishes
// :debug and :debug-nonroot builds of every image, which hold a busybox shell
// (and a package manager) for troubleshooting: they look like the right image
// and contradict the reason the final stage is distroless at all. The table is
// black-box: it goes through example image references only.
func TestDebugBaseImageClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		image string
		want  bool
	}{
		{
			name:  "distroless nonroot tag",
			image: "gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329",
		},
		{
			name:  "distroless untagged with digest",
			image: "gcr.io/distroless/static-debian13@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329",
		},
		{
			name:  "distroless latest tag",
			image: "gcr.io/distroless/static-debian13:latest",
		},
		{
			name:  "scratch",
			image: "scratch",
		},
		{
			name:  "registry with a port and no tag",
			image: "localhost:5000/distroless/static-debian13",
		},
		{
			name:  "distroless debug tag",
			image: "gcr.io/distroless/static-debian13:debug@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329",
			want:  true,
		},
		{
			name:  "distroless debug-nonroot tag",
			image: "gcr.io/distroless/static-debian13:debug-nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329",
			want:  true,
		},
		{
			name:  "registry with a port and a debug tag",
			image: "localhost:5000/distroless/static-debian13:debug",
			want:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isDebugBaseImage(tc.image); got != tc.want {
				t.Errorf("isDebugBaseImage(%q) = %t, want %t", tc.image, got, tc.want)
			}
		})
	}
}

// TestDockerignoreKeepsEmbeddedFiles pins that .dockerignore keeps every file a
// //go:embed directive embeds in the build context. The image builds from cmd/
// and internal/ only, and go build fails outright when an embed pattern matches
// no file, so an over-loose ignore pattern breaks the image build rather than
// shrinking it. Today the embedded files are the target catalog snapshots,
// internal/adapter/target/{ovhcloud,scaleway}/catalog.json, which are not test
// data and must stay in.
func TestDockerignoreKeepsEmbeddedFiles(t *testing.T) {
	t.Parallel()

	patterns := dockerignorePatterns(t, readRepoFile(t, dockerignoreName))
	embedded := embeddedFiles(t)

	if len(embedded) == 0 {
		t.Fatalf("no //go:embed directive in a non-test .go file under cmd/ or internal/ names a file, so this test would pass whatever %s holds: the target catalog snapshots are embedded and must be found",
			dockerignoreName)
	}

	for _, file := range embedded {
		excluded, rule := dockerignoreExcludes(patterns, file.path)
		if !excluded {
			continue
		}

		t.Errorf("%s:%d: the pattern %q excludes the file %q, which %s:%d embeds with //go:embed: the build context leaves it out, so `go build` fails in the image with \"pattern ...: no matching files found\"; the embedded catalog snapshots are not test data and must stay in",
			dockerignoreName, rule.line, rule.raw, file.path, file.source, file.line)
	}
}

// TestDockerignoreKeepsDataOutOfContext pins that .dockerignore keeps the git
// data, the worktrees, the build output, the credentials and the local
// inventories and reports out of the build context, so no layer can hold real
// infrastructure data (CLAUDE.md rule 2). The sample paths are evaluated with
// the same matcher TestDockerignoreKeepsEmbeddedFiles uses, last matching
// pattern winning, so the test pins where a path ends up rather than which lines
// the file happens to hold: the context is an allow list (`*`, then
// `!internal/`), and a pattern anchored to the context root, such as `.env`,
// never matches the .env a package keeps under internal/. The literal entries
// are pinned as well, because they name the data in the file itself.
func TestDockerignoreKeepsDataOutOfContext(t *testing.T) {
	t.Parallel()

	dockerignore := readRepoFile(t, dockerignoreName)
	entries := dockerignoreEntries(t, dockerignore)
	patterns := dockerignorePatterns(t, dockerignore)

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

	tests := []struct {
		name string
		path string
		why  string
	}{
		{name: "env file at the context root", path: ".env", why: "holds credentials"},
		{name: "env file in a package", path: "internal/x/.env", why: "holds credentials"},
		{name: "env variant in a package", path: "internal/x/.env.local", why: "holds credentials"},
		{name: "envrc in a package", path: "cmd/eusov/.envrc", why: "holds credentials"},
		{name: "private key in a package", path: "internal/x/key.pem", why: "is a private key"},
		{name: "key file in a package", path: "internal/x/id.key", why: "is a private key"},
		{name: "aws credentials", path: "internal/x/.aws/credentials", why: "holds cloud credentials"},
		{name: "azure configuration", path: "internal/x/.azure/config", why: "holds cloud credentials"},
		{name: "gcloud credentials", path: "internal/x/.config/gcloud/creds.json", why: "holds cloud credentials"},
		{name: "inventory in a package", path: "internal/x/inventory.json", why: "describes real infrastructure"},
		{name: "report in a package", path: "internal/x/report.md", why: "describes real infrastructure"},
		{name: "git data", path: ".git/config", why: "holds the repository's history and remotes"},
		{name: "local build", path: "bin/eusov", why: "is a locally built binary"},
		{name: "coverage report", path: "coverage.out", why: "is a local build artefact"},
		{name: "worktree module", path: ".worktrees/x/go.mod", why: "belongs to a worktree, not the build"},
		{name: "test fixture", path: "internal/x/testdata/inventory.json", why: "is test data the binary does not build"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			excluded, rule := dockerignoreExcludes(patterns, tc.path)
			if excluded {
				return
			}

			matched := "no pattern matches it at all"
			if rule.pattern != "" {
				matched = fmt.Sprintf("the last pattern matching it is %q, on line %d", rule.raw, rule.line)
			}

			t.Errorf("%s: the path %q would be sent to the daemon (%s) because it is in the build context: %s. Patterns are matched against the whole path relative to the context root, so an entry without a **/ prefix only covers the root, and `!internal/` re-includes the directory after it; want a pattern such as `**/%s` that excludes it at any depth",
				dockerignoreName, tc.path, tc.why, matched, tc.path)
		})
	}
}

// TestEmbedPatternTokens pins how the patterns of a //go:embed directive are
// split: whitespace separates them, and a double-quoted or back-quoted literal is
// one pattern however many spaces it holds, unquoted the way the go command
// unquotes it. A pattern that cannot be read fails rather than being skipped, so
// the test never silently stops looking at an embedded file.
func TestEmbedPatternTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		patterns string
		want     []string
		wantErr  bool
	}{
		{
			name:     "bare patterns",
			patterns: "catalog.json other.json",
			want:     []string{"catalog.json", "other.json"},
		},
		{
			name:     "tabs separate patterns too",
			patterns: "catalog.json\tother.json",
			want:     []string{"catalog.json", "other.json"},
		},
		{
			name:     "leading and trailing whitespace",
			patterns: "  catalog.json  ",
			want:     []string{"catalog.json"},
		},
		{
			name:     "double-quoted pattern with a space",
			patterns: `catalog.json "catalog 2.json"`,
			want:     []string{"catalog.json", "catalog 2.json"},
		},
		{
			name:     "back-quoted pattern with a space",
			patterns: "`catalog 3.json` other.json",
			want:     []string{"catalog 3.json", "other.json"},
		},
		{
			name:     "escape in a double-quoted pattern",
			patterns: `"say \"hi\".json"`,
			want:     []string{`say "hi".json`},
		},
		{
			name:     "escaped tab in a double-quoted pattern",
			patterns: `"catalog\t2.json"`,
			want:     []string{"catalog\t2.json"},
		},
		{
			name:     "glob pattern",
			patterns: "snapshots/*.json",
			want:     []string{"snapshots/*.json"},
		},
		{
			name:     "all prefix is kept on the token",
			patterns: "all:web",
			want:     []string{"all:web"},
		},
		{name: "no pattern", patterns: "", want: nil},
		{name: "unterminated quote", patterns: `"catalog.json`, wantErr: true},
		{name: "invalid escape", patterns: `"catalog\q.json"`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := embedPatternTokens(tc.patterns)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("embedPatternTokens(%q) = %q, want an error", tc.patterns, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("embedPatternTokens(%q): %v", tc.patterns, err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("embedPatternTokens(%q) mismatch (-want +got):\n%s", tc.patterns, diff)
			}
		})
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

		fields := fromFields(in)
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

// baseImageTag returns the tag of any image reference: the part between the
// last colon and the @ digest, and "" for an untagged reference. A colon in a
// registry port, as in localhost:5000/distroless/static, is not a tag.
func baseImageTag(image string) string {
	reference, _, _ := strings.Cut(image, "@")

	colon := strings.LastIndex(reference, ":")
	if colon <= strings.LastIndex(reference, "/") {
		return ""
	}

	return reference[colon+1:]
}

// isDebugBaseImage reports whether an image reference asks for the debug
// variant of a distroless image. Distroless publishes :debug and
// :debug-nonroot builds that hold a busybox shell and a package manager for
// troubleshooting, which is exactly what the minimal final stage rules out.
func isDebugBaseImage(image string) bool {
	return strings.Contains(strings.ToLower(baseImageTag(image)), "debug")
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

// stageInstructions returns the instructions after the FROM at index i, up to
// the next FROM: the body of the stage that FROM starts.
func stageInstructions(instructions []instruction, i int) []instruction {
	for j := i + 1; j < len(instructions); j++ {
		if instructions[j].keyword() == "FROM" {
			return instructions[i+1 : j]
		}
	}

	return instructions[i+1:]
}

// builderStage returns the FROM instruction of the stage that runs `go build`,
// and fails the test when no stage builds anything: the builder is found by what
// it does, so it is checked whatever the stage is called.
func builderStage(t *testing.T, dockerfile string) instruction {
	t.Helper()

	instructions := dockerfileInstructions(t, dockerfile)

	for i, in := range instructions {
		if in.keyword() != "FROM" {
			continue
		}

		for _, stageIn := range stageInstructions(instructions, i) {
			if stageIn.keyword() == "RUN" && strings.Contains(stageIn.arguments(), "go build") {
				return in
			}
		}
	}

	t.Fatalf("%s: no stage runs `go build`, so there is no builder stage whose platform can be checked",
		dockerfileName)

	return instruction{}
}

// assertBuilderCrossCompiles fails unless the stage that runs `go build` starts
// FROM --platform=$BUILDPLATFORM. It reads the Dockerfile text it is given, so a
// mutated copy can be run through it to show the check discriminates.
func assertBuilderCrossCompiles(t *testing.T, dockerfile string) {
	t.Helper()

	builder := builderStage(t, dockerfile)

	switch platform := fromPlatform(builder); platform {
	case "$BUILDPLATFORM", "${BUILDPLATFORM}":
		return
	case "":
		t.Errorf("%s:%d: the stage that runs `go build` starts FROM %q and names no platform: want FROM --platform=$BUILDPLATFORM, so the builder runs on the build host and cross-compiles for TARGETOS and TARGETARCH; without it a --platform linux/arm64 build emulates the whole builder stage under QEMU",
			dockerfileName, builder.line, fromImage(builder))
	default:
		t.Errorf("%s:%d: the stage that runs `go build` starts FROM --platform=%s %q: want --platform=$BUILDPLATFORM, so the builder runs on the build host and cross-compiles for TARGETOS and TARGETARCH instead of being emulated under QEMU",
			dockerfileName, builder.line, platform, fromImage(builder))
	}
}

// fromFields returns the fields of a FROM instruction after its flags: the
// image reference, then AS and the stage name when the stage has one.
func fromFields(in instruction) []string {
	fields := strings.Fields(in.arguments())
	for len(fields) > 0 && strings.HasPrefix(fields[0], "--") {
		fields = fields[1:]
	}

	return fields
}

// fromImage returns the image reference of a FROM instruction, and "" when the
// instruction names no image.
func fromImage(in instruction) string {
	if fields := fromFields(in); len(fields) > 0 {
		return fields[0]
	}

	return ""
}

// fromPlatform returns the value of the --platform flag of a FROM instruction,
// "$BUILDPLATFORM" for "--platform=$BUILDPLATFORM", and "" when it carries
// none. Only the flags before the image reference are read.
func fromPlatform(in instruction) string {
	for _, field := range strings.Fields(in.arguments()) {
		if !strings.HasPrefix(field, "--") {
			break
		}

		if value, found := strings.CutPrefix(field, "--platform="); found {
			return value
		}
	}

	return ""
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

// dockerignorePattern is one .dockerignore line: the pattern with its `!` and
// surrounding slashes removed, whether it re-includes what an earlier line
// excluded, and the line number and text for failure messages.
type dockerignorePattern struct {
	line    int
	raw     string
	pattern string
	negate  bool
}

// embeddedFile is one file a //go:embed directive embeds, named by its slash
// path relative to the repository root, with the directive's place for failure
// messages.
type embeddedFile struct {
	path   string
	source string
	line   int
}

// embedDirective matches a //go:embed line and captures its patterns.
var embedDirective = regexp.MustCompile(`^//go:embed\s+(.+)$`)

// dockerignorePatterns parses .dockerignore into its patterns: comments and
// blank lines are dropped, a leading `!` marks a re-include, and the slashes
// that only anchor a pattern to the context root are removed. A line that
// cannot be evaluated fails the test instead of being skipped.
func dockerignorePatterns(t *testing.T, dockerignore string) []dockerignorePattern {
	t.Helper()

	var patterns []dockerignorePattern

	for i, raw := range strings.Split(dockerignore, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		pattern := strings.Trim(strings.TrimPrefix(line, "!"), "/")
		if pattern == "" {
			t.Errorf("%s:%d: the entry %q names no path, so this test cannot tell what the build context holds",
				dockerignoreName, i+1, line)

			continue
		}

		for _, segment := range strings.Split(pattern, "/") {
			if _, err := path.Match(segment, "x"); err != nil {
				t.Errorf("%s:%d: the pattern %q is not a valid glob: %v",
					dockerignoreName, i+1, line, err)
			}
		}

		patterns = append(patterns, dockerignorePattern{
			line:    i + 1,
			raw:     line,
			pattern: pattern,
			negate:  strings.HasPrefix(line, "!"),
		})
	}

	if len(patterns) == 0 {
		t.Fatalf("%s: no pattern, so the build context holds every file in the repository", dockerignoreName)
	}

	return patterns
}

// dockerignoreExcludes reports whether the patterns leave the slash path,
// relative to the repository root, out of the build context, and returns the
// pattern that decided it: the last pattern that matches wins, and a `!`
// pattern puts the path back in.
func dockerignoreExcludes(patterns []dockerignorePattern, filePath string) (bool, dockerignorePattern) {
	excluded := false
	var last dockerignorePattern

	for _, pattern := range patterns {
		if pattern.matches(filePath) {
			excluded, last = !pattern.negate, pattern
		}
	}

	return excluded, last
}

// matches reports whether the pattern matches the slash path relative to the
// repository root. Docker evaluates a pattern against the path and against each
// of its parent directories, so the directory pattern `cmd` covers what is under
// cmd, and a `**` segment matches zero or more directories.
func (p dockerignorePattern) matches(filePath string) bool {
	if matchDockerignorePath(p.pattern, filePath) {
		return true
	}

	for dir := path.Dir(filePath); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if matchDockerignorePath(p.pattern, dir) {
			return true
		}
	}

	return false
}

// matchDockerignorePath reports whether one pattern matches the slash path
// segment by segment, with `**` standing for zero or more segments and every
// other segment matched by path.Match.
func matchDockerignorePath(pattern, filePath string) bool {
	return matchDockerignoreSegments(strings.Split(pattern, "/"), strings.Split(filePath, "/"))
}

// matchDockerignoreSegments reports whether the pattern segments match the path
// segments: `**` consumes any number of them, and each other segment is one
// path.Match, which does not cross a slash.
func matchDockerignoreSegments(pattern, segments []string) bool {
	if len(pattern) == 0 {
		return len(segments) == 0
	}

	if pattern[0] == "**" {
		for skip := 0; skip <= len(segments); skip++ {
			if matchDockerignoreSegments(pattern[1:], segments[skip:]) {
				return true
			}
		}

		return false
	}

	if len(segments) == 0 {
		return false
	}

	matched, err := path.Match(pattern[0], segments[0])
	if err != nil || !matched {
		return false
	}

	return matchDockerignoreSegments(pattern[1:], segments[1:])
}

// embeddedFiles walks the non-test .go files under cmd/ and internal/, the two
// directories the Dockerfile copies into the builder, and returns every file
// their embed directives name. The patterns are tokenised and unquoted as the go
// command reads them, a glob is expanded, and a directory pattern is walked
// recursively, leaving out the names starting with . or _ that it leaves out
// unless the pattern carries the all: prefix.
func embeddedFiles(t *testing.T) []embeddedFile {
	t.Helper()

	repository, err := os.OpenRoot(repoRoot(t))
	if err != nil {
		t.Fatalf("open the repository root: %v", err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Errorf("close the repository root: %v", err)
		}
	})

	tree := repository.FS()
	var files []embeddedFile

	for _, top := range []string{"cmd", "internal"} {
		err := fs.WalkDir(tree, top, func(filePath string, entry fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case entry.IsDir(), !strings.HasSuffix(filePath, ".go"), strings.HasSuffix(filePath, "_test.go"):
				return nil
			}

			data, err := fs.ReadFile(tree, filePath)
			if err != nil {
				return err
			}

			for i, line := range strings.Split(string(data), "\n") {
				match := embedDirective.FindStringSubmatch(strings.TrimSpace(line))
				if match == nil {
					continue
				}

				patterns, parseErr := embedPatternTokens(match[1])
				if parseErr != nil {
					t.Fatalf("%s:%d: //go:embed %s: %v", filePath, i+1, match[1], parseErr)
				}

				for _, pattern := range patterns {
					files = append(files, embedTargets(t, tree, filePath, i+1, pattern)...)
				}
			}

			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", top, err)
		}
	}

	slices.SortFunc(files, func(a, b embeddedFile) int { return strings.Compare(a.path, b.path) })

	return files
}

// embedAllPrefix is the prefix an embed pattern carries to include the files a
// directory pattern otherwise leaves out, the ones whose name starts with . or
// _. The go command strips it before it resolves the pattern.
const embedAllPrefix = "all:"

// embedTargets resolves one embed pattern, declared on the given line of the
// .go file source, to the files it embeds, relative to its package directory.
// The pattern is one token as embedPatternTokens returns it: already unquoted,
// with the all: prefix still on it. A pattern matching nothing fails the test: go
// build fails the same way, so the repository is broken rather than this test.
func embedTargets(t *testing.T, tree fs.FS, source string, line int, pattern string) []embeddedFile {
	t.Helper()

	name := pattern
	all := false
	if stripped, found := strings.CutPrefix(name, embedAllPrefix); found {
		name, all = stripped, true
	}

	target := path.Join(path.Dir(source), name)

	matches := []string{target}
	if strings.ContainsAny(name, `*?[`) {
		found, err := fs.Glob(tree, target)
		if err != nil {
			t.Fatalf("%s:%d: the embed glob %q cannot be expanded: %v", source, line, pattern, err)
		}

		if len(found) == 0 {
			t.Fatalf("%s:%d: the embed glob %q matches no file, so go build fails on the package",
				source, line, pattern)
		}

		matches = found
	}

	var files []embeddedFile

	for _, match := range matches {
		info, err := fs.Stat(tree, match)
		if err != nil {
			t.Fatalf("%s:%d: the embed pattern %q names %s, which cannot be read: %v; go build fails on the package",
				source, line, pattern, match, err)
		}

		if !info.IsDir() {
			files = append(files, embeddedFile{path: match, source: source, line: line})

			continue
		}

		err = fs.WalkDir(tree, match, func(embeddedPath string, entry fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case embeddedPath == match:
				return nil
			case !all && excludedByEmbed(entry.Name()):
				if entry.IsDir() {
					return fs.SkipDir
				}

				return nil
			case entry.IsDir():
				return nil
			}

			files = append(files, embeddedFile{path: embeddedPath, source: source, line: line})

			return nil
		})
		if err != nil {
			t.Fatalf("%s:%d: walk the embed directory %s: %v", source, line, match, err)
		}
	}

	return files
}

// excludedByEmbed reports whether go:embed leaves the name out of a directory
// pattern: it skips the files and directories whose name starts with . or _,
// unless the pattern carries the all: prefix.
func excludedByEmbed(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// embedPatternTokens splits the patterns of a //go:embed directive the way the
// go command does: spaces and tabs separate them, and a pattern may be written
// as a double-quoted or back-quoted Go string literal, which may hold spaces and
// escapes. Every quoted token is unquoted, so the caller works with the file
// names as the go command reads them.
func embedPatternTokens(patterns string) ([]string, error) {
	var tokens []string

	for rest := strings.TrimLeft(patterns, " \t"); rest != ""; rest = strings.TrimLeft(rest, " \t") {
		if rest[0] == '"' || rest[0] == '`' {
			token, after, err := cutQuotedEmbedPattern(rest)
			if err != nil {
				return nil, err
			}

			tokens = append(tokens, token)
			rest = after

			continue
		}

		end := strings.IndexAny(rest, " \t")
		if end < 0 {
			end = len(rest)
		}

		tokens = append(tokens, rest[:end])
		rest = rest[end:]
	}

	return tokens, nil
}

// cutQuotedEmbedPattern reads the Go string literal at the start of rest, which
// begins with a double or back quote, and returns its value and the text after
// it.
func cutQuotedEmbedPattern(rest string) (string, string, error) {
	quote := rest[0]

	for i := 1; i < len(rest); i++ {
		switch {
		case quote == '"' && rest[i] == '\\':
			i++
		case rest[i] == quote:
			value, err := strconv.Unquote(rest[:i+1])
			if err != nil {
				return "", "", fmt.Errorf("the embed pattern %s is not a valid Go string literal: %w", rest[:i+1], err)
			}

			return value, rest[i+1:], nil
		}
	}

	return "", "", fmt.Errorf("the embed pattern %s has no closing %c", rest, quote)
}

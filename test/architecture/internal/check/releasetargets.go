package check

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ReleaseMain is the main package the release builds: ReleaseTargets refuses a
// .goreleaser.yaml that builds another, and the repository test lists the
// packages of the release binary from it.
const ReleaseMain = "./cmd/eusov"

// Target is one platform the release builds.
type Target struct {
	GOOS   string
	GOARCH string
}

// ReleaseTargets returns the platforms a .goreleaser.yaml document builds: the
// goos x goarch product of its one build, sorted by GOOS then GOARCH.
//
// It fails closed, and holds the document to the keys the check knows cannot
// change what the release compiles:
//
//   - The top level may hold only version, project_name, before, builds,
//     archives, checksum, changelog and release. Any other key is refused: a
//     top-level env applies to every build, gomod picks the Go binary that
//     builds, and a key the check does not know may do either.
//   - before may hold only hooks, and they must be exactly [go mod tidy]: a
//     hook runs before the build and could rewrite what it compiles.
//   - builds must hold exactly one entry. It may hold only id, main, binary,
//     env, flags, ldflags, goos and goarch: ignore and targets (which change
//     the platform list), tags, overrides, goamd64 and goarm64 (which change
//     the build tags a file is selected by) and every other key are refused.
//     main must be ./cmd/eusov, the package the repository test lists as the
//     release binary. env must be exactly CGO_ENABLED=0: goreleaser renders
//     each entry as a template, its name included, so no rule on names alone
//     tells which variable an entry sets. flags may hold only -trimpath. goos
//     and goarch must be non-empty lists of GOOS and GOARCH names, in flow
//     ([a, b]) or block (- a) form. A template ({{) in goos, goarch or flags
//     is refused.
func ReleaseTargets(goreleaser []byte) ([]Target, error) {
	lines, err := splitLines(goreleaser)
	if err != nil {
		return nil, fmt.Errorf("read .goreleaser.yaml: %w", err)
	}

	block, err := topLevel(lines)
	if err != nil {
		return nil, fmt.Errorf("read .goreleaser.yaml: %w", err)
	}

	builds, err := sequence(block)
	if err != nil {
		return nil, fmt.Errorf("read .goreleaser.yaml: builds: %w", err)
	}

	if len(builds) != 1 {
		return nil, fmt.Errorf("read .goreleaser.yaml: %d builds, want exactly one, so its platforms are the one list the check reads", len(builds))
	}

	targets, err := buildTargets(builds[0])
	if err != nil {
		return nil, fmt.Errorf("read .goreleaser.yaml: the build at line %d: %w", builds[0].num, err)
	}

	return targets, nil
}

// topLevel checks every top-level key of the document and returns the lines
// of builds.
func topLevel(lines []line) ([]line, error) {
	entries, err := mapping(lines)
	if err != nil {
		return nil, err
	}

	var builds []line

	for _, e := range entries {
		switch e.key {
		case "version", "project_name", "archives", "checksum", "changelog", "release":
			// The format version, the project's name, and how the built
			// binaries are packed, summed, announced and published: none
			// changes what is compiled. GitHubRelease holds release to its
			// one setting.
		case "before":
			err = checkBefore(e)
		case "builds":
			builds, err = blockValue(e, "builds")
		default:
			err = fmt.Errorf("line %d: top-level key %s may change what the release compiles, and the check does not read it", e.num, e.key)
		}

		if err != nil {
			return nil, err
		}
	}

	if builds == nil {
		return nil, errors.New("no builds")
	}

	return builds, nil
}

// checkBefore refuses a before section that holds anything but the hook
// go mod tidy.
func checkBefore(before entry) error {
	block, err := blockValue(before, "before")
	if err != nil {
		return err
	}

	entries, err := mapping(block)
	if err != nil {
		return fmt.Errorf("before: %w", err)
	}

	for _, e := range entries {
		if e.key != "hooks" {
			return fmt.Errorf("line %d: before: key %s may change what the release compiles, and the check does not read it", e.num, e.key)
		}

		hooks, err := list(e)
		if err != nil {
			return fmt.Errorf("before: %w", err)
		}

		if !slices.Equal(hooks, []string{"go mod tidy"}) {
			return fmt.Errorf("line %d: before: hooks are %q, want exactly [go mod tidy]: a hook runs before the build and could rewrite what it compiles", e.num, hooks)
		}
	}

	return nil
}

// buildTargets returns the goos x goarch product of one build, sorted, after
// checking every key the build holds.
func buildTargets(build item) ([]Target, error) {
	entries, err := mapping(build.lines)
	if err != nil {
		return nil, err
	}

	var (
		mainPkg                  string
		env, flags, goos, goarch []string
	)

	for _, e := range entries {
		switch e.key {
		case "id", "binary", "ldflags":
			// They name the build and its binary, and set link-time values:
			// none selects the files a package compiles.
		case "main":
			mainPkg, err = single(e)
		case "env":
			env, err = list(e)
		case "flags":
			flags, err = list(e)
		case "goos":
			goos, err = list(e)
		case "goarch":
			goarch, err = list(e)
		default:
			return nil, fmt.Errorf("line %d: key %s may change what the release compiles, and the check does not read it", e.num, e.key)
		}

		if err != nil {
			return nil, err
		}
	}

	if mainPkg != ReleaseMain {
		return nil, fmt.Errorf("main is %q, want %s, the package the check lists as the release binary", mainPkg, ReleaseMain)
	}

	if !slices.Equal(env, []string{"CGO_ENABLED=0"}) {
		return nil, fmt.Errorf("env is %q, want exactly [CGO_ENABLED=0]: goreleaser renders each entry, its name included, as a template, so any other entry may set a variable the go command reads", env)
	}

	for _, kv := range []struct {
		key    string
		values []string
	}{
		{key: "flags", values: flags},
		{key: "goos", values: goos},
		{key: "goarch", values: goarch},
	} {
		if err := checkNoTemplate(kv.key, kv.values); err != nil {
			return nil, err
		}
	}

	for _, flag := range flags {
		if flag != "-trimpath" {
			return nil, fmt.Errorf("flag %s may change what the release compiles, and the check does not apply it", flag)
		}
	}

	if err := checkPlatformNames("goos", goos); err != nil {
		return nil, err
	}

	if err := checkPlatformNames("goarch", goarch); err != nil {
		return nil, err
	}

	targets := make([]Target, 0, len(goos)*len(goarch))

	for _, system := range goos {
		for _, arch := range goarch {
			targets = append(targets, Target{GOOS: system, GOARCH: arch})
		}
	}

	slices.SortFunc(targets, func(a, b Target) int {
		if c := strings.Compare(a.GOOS, b.GOOS); c != 0 {
			return c
		}

		return strings.Compare(a.GOARCH, b.GOARCH)
	})

	return targets, nil
}

// checkNoTemplate refuses an entry that holds a template, which goreleaser
// renders and the check does not.
func checkNoTemplate(key string, values []string) error {
	for _, value := range values {
		if strings.Contains(value, "{{") {
			return fmt.Errorf("%s entry %q is a template, which goreleaser renders and the check does not", key, value)
		}
	}

	return nil
}

// checkPlatformNames refuses an empty list, a name that is not lower-case
// ASCII letters and digits, and a name listed twice.
func checkPlatformNames(key string, names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("%s is missing or empty", key)
	}

	for i, name := range names {
		if name == "" || strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789") != "" {
			return fmt.Errorf("%s entry %q is not a GOOS or GOARCH name", key, name)
		}

		if slices.Contains(names[:i], name) {
			return fmt.Errorf("%s lists %s twice", key, name)
		}
	}

	return nil
}

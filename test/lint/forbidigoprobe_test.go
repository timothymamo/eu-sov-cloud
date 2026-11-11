package lint_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// probeEnv names the variable that turns a missing golangci-lint from a skip
// into a failure. make lint and every entry of CI's lint-target matrix, which
// have golangci-lint, set it to require.
const probeEnv = "EUSOV_LINT_PROBES"

// astPrefix starts the message of a rule test/lint checks itself, on the AST,
// rather than golangci-lint. A probe's want comment with this prefix is for
// that test, and runForbidigoProbe leaves it out.
const astPrefix = "test/lint: "

// forbidigoProbe is a probe of one forbidigo group: a module under
// testdata/<dir>/mod in which every line that ends in a want comment must be
// refused, with the messages the comment names, and no other line may be.
type forbidigoProbe struct {
	dir string // the directory under testdata
	tag string // the group's tag, which starts each of its msgs
	// stubs maps each directory under testdata/<dir>/stub to the module path
	// its stub stands in for.
	stubs map[string]string
	// depguard also runs depguard, for a group that an import rule backs.
	depguard bool
}

// runForbidigoProbe runs golangci-lint, with the repository's .golangci.yml and
// forbidigo alone enabled (depguard too if the probe asks), on a scratch copy of
// the probe's module, offline and with no credentials. A line that ends in
//
//	// want "<message prefix>" ["<message prefix>" ...]
//
// must get, for each prefix, an issue whose message starts with it, and every
// issue must match a want on its line. A forbidigo issue's message is its msg,
// a depguard issue's its text. Lines are not merged (--uniq-by-line=false), so
// two entries refusing one line are told apart. Every entry of the group must
// refuse at least one line, so an entry that silently stops matching fails the
// probe. Without golangci-lint on PATH it skips, unless EUSOV_LINT_PROBES=require.
func runForbidigoProbe(t *testing.T, p forbidigoProbe) {
	t.Helper()

	bin, err := exec.LookPath("golangci-lint")
	if err != nil {
		if os.Getenv(probeEnv) == "require" {
			t.Fatalf("golangci-lint is not on PATH, and %s=require: %v", probeEnv, err)
		}
		t.Skipf("golangci-lint is not on PATH; make lint and every entry of CI's lint-target matrix run this probe (%s=require)", probeEnv)
	}
	root := moduleRoot(t)
	if got, want := golangciLintVersion(t, bin), pinnedGolangciLintVersion(t, root); got != want {
		// CI must probe the version it pins. Elsewhere the probe still runs,
		// as forbidigo's matching rarely moves between releases, but its
		// verdict holds for this version only.
		if os.Getenv("CI") == "true" {
			t.Fatalf("golangci-lint is %s, but CI pins %s (.github/workflows/ci.yml); probe with the pinned version", got, want)
		}
		t.Logf("WARNING: golangci-lint is %s, but CI pins %s (.github/workflows/ci.yml), so this probe proves the "+
			"rules for %s only. Run make lint inside the dev VM (docs/dev-environment.md), which has the pinned version.",
			got, want, got)
	}

	// The scratch copy: the probe module and its stubs, each given a go.mod
	// here, and the repository's .golangci.yml beside the probe.
	scratch := t.TempDir()
	if err := os.CopyFS(scratch, os.DirFS(filepath.Join(root, "test", "lint", "testdata", p.dir))); err != nil {
		t.Fatalf("copy the probe: %v", err)
	}
	dst, err := os.OpenRoot(scratch)
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	config, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dst, "mod/.golangci.yml", string(config))

	goMod := "module github.com/timothymamo/eu-sov-cloud\n\ngo " + goDirective(t, root) + "\n"
	if len(p.stubs) > 0 {
		var requires, replaces strings.Builder
		for _, dir := range slices.Sorted(maps.Keys(p.stubs)) {
			path := p.stubs[dir]
			writeFile(t, dst, "stub/"+dir+"/go.mod", "module "+path+"\n\ngo 1.24\n")
			version := "v0.0.0"
			if m := majorVersion.FindString(path[strings.LastIndex(path, "/")+1:]); m != "" {
				version = m + ".0.0" // a /vN module path needs a vN version
			}
			fmt.Fprintf(&requires, "\t%s %s\n", path, version)
			fmt.Fprintf(&replaces, "replace %s => ../stub/%s\n", path, dir)
		}
		goMod += "\nrequire (\n" + requires.String() + ")\n\n" + replaces.String()
	}
	writeFile(t, dst, "mod/go.mod", goMod)
	mod := filepath.Join(scratch, "mod")

	linters := "forbidigo"
	if p.depguard {
		linters += ",depguard"
	}
	// Probes run in parallel, each with a cache of its own in its scratch
	// directory, so the lock that refuses a second run is not needed.
	cmd := exec.CommandContext(t.Context(), bin, "run", "--enable-only="+linters, "--allow-parallel-runners",
		"--uniq-by-line=false", "--max-issues-per-linter=0", "--max-same-issues=0",
		"--output.json.path=stdout", "--output.text.path=", "--show-stats=false", "./...")
	cmd.Dir = mod
	cmd.Env = childEnv(
		// cgo on, as lint loads the module (lintEnv), so a probe of a cgo
		// file sees it.
		"CGO_ENABLED=1", "GOFLAGS=-mod=mod", "GOWORK=off", "GOPROXY=off", "GOTOOLCHAIN=local",
		"GOLANGCI_LINT_CACHE="+filepath.Join(scratch, "cache"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	// golangci-lint exits 1 when it reports issues, which the probe expects.
	var exitErr *exec.ExitError
	if err != nil && (!errors.As(err, &exitErr) || exitErr.ExitCode() != 1) {
		t.Fatalf("golangci-lint run: %v\n%s\n%s", err, out, stderr.String())
	}

	var report struct {
		Issues []struct {
			FromLinter string
			Text       string
			Pos        struct {
				Filename string
				Line     int
			}
		}
	}
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode golangci-lint output: %v\n%s\n%s", err, out, stderr.String())
	}

	want := wantMessages(t, mod, func(prefix string) bool { return !strings.HasPrefix(prefix, astPrefix) })
	if len(want) == 0 {
		t.Fatal("the probe holds no want comment")
	}
	matched := map[string]map[string]bool{} // line to the want prefixes an issue matched
	refusedBy := map[string]bool{}          // the msgs of the group's entries that refused a line
	for _, issue := range report.Issues {
		line := fmt.Sprintf("%s:%d", filepath.ToSlash(issue.Pos.Filename), issue.Pos.Line)
		var msg string
		switch issue.FromLinter {
		case "forbidigo":
			m := forbidigoMsg.FindStringSubmatch(issue.Text)
			if m == nil || !strings.HasPrefix(m[1], p.tag+": ") {
				t.Errorf("%s: forbidigo issue outside group %s: %s", line, p.tag, issue.Text)
				continue
			}
			msg = m[1]
			refusedBy[msg] = true
		case "depguard":
			if !p.depguard {
				t.Errorf("%s: unexpected depguard issue: %s", line, issue.Text)
				continue
			}
			msg = issue.Text
		default:
			t.Errorf("%s: unexpected issue from %s: %s", line, issue.FromLinter, issue.Text)
			continue
		}
		ok := false
		for _, prefix := range want[line] {
			if strings.HasPrefix(msg, prefix) {
				ok = true
				if matched[line] == nil {
					matched[line] = map[string]bool{}
				}
				matched[line][prefix] = true
			}
		}
		if !ok {
			t.Errorf("%s: %s refuses this line, which the probe does not expect: %s", line, issue.FromLinter, msg)
		}
	}
	for _, line := range slices.Sorted(maps.Keys(want)) {
		for _, prefix := range want[line] {
			if !matched[line][prefix] {
				t.Errorf("%s: no issue starts with %q; the entry that should refuse this line no longer does", line, prefix)
			}
		}
	}
	for _, e := range forbidigoEntries(t, root) {
		if strings.HasPrefix(e.msg, p.tag+": ") && !refusedBy[e.msg] {
			t.Errorf(".golangci.yml:%d: no probe line in testdata/%s is refused by the entry with msg %q; "+
				"add a line only it refuses", e.line, p.dir, e.msg)
		}
	}
}

// forbidigoMsg captures the msg at the end of a forbidigo issue's text:
// use of `<expression>` forbidden because "<msg>".
var forbidigoMsg = regexp.MustCompile(`because "([^"]*)"$`)

var (
	// wantComment matches a line that ends in a want comment.
	wantComment = regexp.MustCompile(`// want((?: "[^"]*")+)\s*$`)
	// wantPrefix matches each quoted prefix of a want comment.
	wantPrefix = regexp.MustCompile(`"([^"]*)"`)
)

// wantMessages maps file:line, relative to dir and slash-separated, of every
// line in a Go file under dir that ends in a want comment to the message
// prefixes the comment names that keep reports true.
func wantMessages(t *testing.T, dir string, keep func(prefix string) bool) map[string][]string {
	t.Helper()

	want := map[string][]string{}
	refused, err := walkGoFiles(dir, nil, func(rel string, src []byte) error {
		sc := bufio.NewScanner(bytes.NewReader(src))
		for n := 1; sc.Scan(); n++ {
			m := wantComment.FindStringSubmatch(sc.Text())
			if m == nil {
				continue
			}
			for _, q := range wantPrefix.FindAllStringSubmatch(m[1], -1) {
				if keep(q[1]) {
					line := fmt.Sprintf("%s:%d", rel, n)
					want[line] = append(want[line], q[1])
				}
			}
		}
		return sc.Err()
	})
	if err != nil {
		t.Fatalf("read probe: %v", err)
	}
	for _, problem := range refused {
		t.Error(problem)
	}
	return want
}

// forbidigoEntry is one entry of forbid in .golangci.yml.
type forbidigoEntry struct {
	pattern, pkg, msg string
	line              int // of its pattern
}

// forbidigoEntries reads every forbidigo entry from .golangci.yml. It reads the
// file line by line, as every entry there is written: a "- pattern:" line, then
// pkg and msg, each a single-quoted YAML scalar on one line.
func forbidigoEntries(t *testing.T, root string) []forbidigoEntry {
	t.Helper()

	config, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []forbidigoEntry
	lines := strings.Split(string(config), "\n")
	for i, line := range lines {
		pattern, ok := yamlValue(line, "- pattern:")
		if !ok {
			continue
		}
		e := forbidigoEntry{pattern: pattern, line: i + 1}
		for _, next := range lines[i+1 : min(i+3, len(lines))] {
			if v, ok := yamlValue(next, "pkg:"); ok {
				e.pkg = v
			}
			if v, ok := yamlValue(next, "msg:"); ok {
				e.msg = v
			}
		}
		if e.msg == "" {
			t.Fatalf(".golangci.yml:%d: a forbidigo entry with no msg on the two lines after its pattern", e.line)
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		t.Fatal(".golangci.yml holds no forbidigo entry")
	}
	return entries
}

// yamlValue returns the single-quoted scalar after key on line.
func yamlValue(line, key string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), key+" ")
	if !ok || len(rest) < 2 || rest[0] != '\'' || rest[len(rest)-1] != '\'' {
		return "", false
	}
	return strings.ReplaceAll(rest[1:len(rest)-1], "''", "'"), true
}

// golangciLintVersion returns the version of the golangci-lint at bin, without
// a leading v.
func golangciLintVersion(t *testing.T, bin string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), bin, "version", "--short")
	cmd.Env = childEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("golangci-lint version: %v", err)
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
}

// pinnedStep matches the list item of the golangci-lint action step, and
// captures the indentation of its "- ".
var pinnedStep = regexp.MustCompile(`(?m)^([ \t]*)- uses: golangci/golangci-lint-action@[^\n]*\n`)

// pinnedVersion returns the version input of the golangci-lint action step in
// ci, without a leading v. Every line it reads after "- uses:" must be indented
// further than that "- ", so it stays inside the step: a line that opens a new
// list item ("- ") or is indented less, a later step or job, ends the step, and
// a version of theirs is never read. The step's other keys (its env: block)
// may stand between uses: and with:, and between with: and version:.
func pinnedVersion(ci []byte) (string, bool) {
	loc := pinnedStep.FindSubmatchIndex(ci)
	if loc == nil {
		return "", false
	}
	in := regexp.QuoteMeta(string(ci[loc[2]:loc[3]])) + `[ \t]+`
	key := in + `[^-\s][^\n]*\n`
	step := regexp.MustCompile(`^(?:` + key + `)*?` + in + `with:\n(?:` + key + `)*?` + in + `version: v?([0-9]\S*)`)
	m := step.FindSubmatch(ci[loc[1]:])
	if m == nil {
		return "", false
	}
	return string(m[1]), true
}

// TestPinnedVersion pins that pinnedVersion reads the version input of the
// golangci-lint step only, never one of a later step or job.
func TestPinnedVersion(t *testing.T) {
	t.Parallel()

	const step = "    steps:\n      - uses: golangci/golangci-lint-action@abc # v9\n"
	tests := []struct {
		name   string
		ci     string
		want   string
		wantOK bool
	}{
		{
			name:   "env before with",
			ci:     step + "        env:\n          GOOS: linux\n        with:\n          skip-cache: true\n          version: v2.15.0 # pin\n",
			want:   "2.15.0",
			wantOK: true,
		},
		{
			name: "a later step's version",
			ci:   step + "        with:\n          skip-cache: true\n      - uses: other@def\n        with:\n          version: v1.0.0\n",
		},
		{
			name: "a later job's version",
			ci:   step + "  next:\n    uses: ./.github/workflows/reusable.yml\n    with:\n      version: v1.0.0\n",
		},
		{
			name: "a later job's version, indented like the step's keys",
			ci:   step + "    next:\n        with:\n          version: v1.0.0\n",
			// "    next:" is indented less than the step's "- ", so it ends the step.
		},
		{
			name: "no golangci-lint step",
			ci:   "      - uses: other@def\n        with:\n          version: v1.0.0\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := pinnedVersion([]byte(tt.ci))
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("pinnedVersion() = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// pinnedGolangciLintVersion returns the golangci-lint version that every entry
// of CI's lint-target matrix pins, without a leading v.
func pinnedGolangciLintVersion(t *testing.T, root string) string {
	t.Helper()

	ci, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	v, ok := pinnedVersion(ci)
	if !ok {
		t.Fatal(".github/workflows/ci.yml: no version input on the golangci-lint action step")
	}
	return v
}

// goDirective returns the go version the module's go.mod declares.
func goDirective(t *testing.T, root string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(string(data)) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "go "); ok {
			return v
		}
	}
	t.Fatal("go.mod has no go directive")
	return ""
}

// writeFile writes content to the file at name, a slash-separated path in
// dst.
func writeFile(t *testing.T, dst *os.Root, name, content string) {
	t.Helper()

	if err := dst.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

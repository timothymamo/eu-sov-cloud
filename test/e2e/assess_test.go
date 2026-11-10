//go:build e2e

package e2e_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/internal/testutil"
)

const (
	// targetKey is the one target the walking skeleton offers.
	targetKey = "scaleway"
	// errPrefix is how the binary writes an error to stderr.
	errPrefix = "eusov: "
	// maxErrorLine is the longest error line the binary may write: the cap on
	// the message, the prefix, a short truncation marker and the newline.
	maxErrorLine = 1024 + len(errPrefix) + 64 + 1
)

// binaryPath is the eusov binary every test runs, built once by TestMain.
var binaryPath string

// emptyEnv runs the binary with no environment at all. The assess command needs
// no credentials, no HOME and no TMPDIR: the inventory path comes from the
// argument, the catalog is embedded and --output writes next to its destination.
func emptyEnv() []string {
	return []string{}
}

// TestMain builds the binary once, into a directory of its own, and removes it
// afterwards. The go tool itself runs in the test's environment; only the
// binary runs without one, in emptyEnv.
func TestMain(m *testing.M) {
	code, err := buildAndRun(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(code)
}

// buildAndRun builds the binary and runs the tests, returning the exit code.
func buildAndRun(m *testing.M) (int, error) {
	dir, err := os.MkdirTemp("", "eusov-e2e-")
	if err != nil {
		return 0, fmt.Errorf("create the directory for the binary: %w", err)
	}

	defer os.RemoveAll(dir)

	binaryPath = filepath.Join(dir, "eusov")

	build := exec.Command("go", "build", "-trimpath", "-o", binaryPath, filepath.Join("..", "..", "cmd", "eusov"))
	if out, err := build.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("build eusov: %w\n%s", err, out)
	}

	return m.Run(), nil
}

// result is one run of the binary.
type result struct {
	code   int
	stdout string
	stderr string
}

// run runs the binary with env and args and returns what it printed and the
// exit code it chose. A run the test cannot start at all fails the test.
func run(t *testing.T, env []string, args ...string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer

	cmd := exec.Command(binaryPath, args...)
	cmd.Env = env
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError

	switch {
	case err == nil:
		return result{code: 0, stdout: stdout.String(), stderr: stderr.String()}
	case errors.As(err, &exitErr):
		return result{code: exitErr.ExitCode(), stdout: stdout.String(), stderr: stderr.String()}
	default:
		t.Fatalf("running eusov %q: %v", args, err)

		return result{}
	}
}

// assess runs `eusov assess --inventory inventory --target scaleway` with no
// environment and returns the result.
func assess(t *testing.T, inventory string, extra ...string) result {
	t.Helper()

	args := append([]string{"assess", "--inventory", inventory, "--target", targetKey}, extra...)

	return run(t, emptyEnv(), args...)
}

// goldenPath names a golden file of this package.
func goldenPath(name string) string {
	return filepath.Join("testdata", name)
}

// TestAssessReport pins the whole path end to end: the built binary, run with no
// environment, turns the fixture inventory into the report in assess.golden. The
// golden is the specification of the walking skeleton: it shows the target, the
// catalog snapshot, the inventory's base name, the summary, one row per resource
// with its fit and reasons, the required changes and the blockers. The fixture
// lists its resources in an order of its own, not in ID order, because the
// report orders the rows itself and must not depend on the file's order.
func TestAssessReport(t *testing.T) {
	t.Parallel()

	got := assess(t, filepath.Join("testdata", "inventory.json"))

	if got.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", got.code, got.stderr)
	}

	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}

	testutil.Golden(t, goldenPath("assess.golden"), []byte(got.stdout))
}

// TestAssessOutput pins --output: the report file holds exactly what stdout
// would have held, and stdout stays empty.
func TestAssessOutput(t *testing.T) {
	t.Parallel()

	dest := filepath.Join(t.TempDir(), "report.md")

	got := assess(t, filepath.Join("testdata", "inventory.json"), "--output", dest)

	if got.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", got.code, got.stderr)
	}

	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty with --output", got.stdout)
	}

	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}

	written, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("the report file was not written: %v", err)
	}

	testutil.Golden(t, goldenPath("assess.golden"), written)
}

// TestAssessHostileInventory pins that values from an inventory file reach the
// report as text: read by the real adapter, rendered by the real reporter, every
// value appears verbatim inside a code span, so no renderer can read it as a
// heading, a link, an image, math, a mention, a reference, an emoji, a task list
// or a table cell boundary.
func TestAssessHostileInventory(t *testing.T) {
	t.Parallel()

	got := assess(t, filepath.Join("testdata", "inventory-hostile.json"))

	if got.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", got.code, got.stderr)
	}

	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}

	testutil.Golden(t, goldenPath("assess-hostile.golden"), []byte(got.stdout))

	for _, value := range hostileValues() {
		if !hasCodeSpan(got.stdout, value) {
			t.Errorf("the report does not show %q inside a code span:\n%s", value, got.stdout)
		}
	}
}

// hostileValues returns every value of the hostile fixture that a renderer would
// otherwise rewrite.
func hostileValues() []string {
	return []string{
		"#1",
		"$\\Huge X$",
		"[ ] x",
		"@octocat",
		"a|b",
		"<b>",
		"tick`x",
		":x:",
	}
}

// TestAssessErrors pins the input errors: exit code 1, one error line that names
// the file, and never a panic or a stack trace.
func TestAssessErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		inventory string
		// wantStderr lists the values stderr must hold besides the quoted file
		// name: the problem with the file, which tells the user what to fix.
		wantStderr []string
	}{
		{
			name:       "missing file",
			inventory:  filepath.Join("testdata", "no-such-file.json"),
			wantStderr: []string{"cannot read"},
		},
		{
			name:       "not JSON",
			inventory:  filepath.Join("testdata", "not-json.json"),
			wantStderr: []string{"malformed JSON"},
		},
		{
			name:       "unsupported schema version",
			inventory:  filepath.Join("testdata", "schema-version-2.json"),
			wantStderr: []string{"unsupported schema version"},
		},
		{
			name:       "unknown category",
			inventory:  filepath.Join("testdata", "unknown-category.json"),
			wantStderr: []string{"i-0123456789abcdef0", "compute.mv"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := assess(t, tc.inventory)

			if got.code != 1 {
				t.Errorf("exit code = %d, want 1; stderr = %q", got.code, got.stderr)
			}

			if got.stdout != "" {
				t.Errorf("stdout = %q, want empty", got.stdout)
			}

			if !strings.HasPrefix(got.stderr, errPrefix) {
				t.Errorf("stderr = %q, want it to start with %q", got.stderr, errPrefix)
			}

			if !strings.HasSuffix(got.stderr, "\n") || strings.Count(got.stderr, "\n") != 1 {
				t.Errorf("stderr = %q, want exactly one line ending in a newline", got.stderr)
			}

			if len(got.stderr) > maxErrorLine {
				t.Errorf("stderr is %d bytes, want at most %d", len(got.stderr), maxErrorLine)
			}

			// The error names the file it failed on, quoted, as text.Quote shows it.
			if want := strconv.Quote(tc.inventory); !strings.Contains(got.stderr, want) {
				t.Errorf("stderr = %q, want it to name the file as %s", got.stderr, want)
			}

			for _, want := range tc.wantStderr {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want it to name %q", got.stderr, want)
				}
			}

			if lower := strings.ToLower(got.stdout + got.stderr); strings.Contains(lower, "panic") ||
				strings.Contains(lower, "goroutine") {
				t.Errorf("the output holds a panic or a stack trace:\n%s\n%s", got.stdout, got.stderr)
			}
		})
	}
}

// TestAssessUsage pins the usage errors of the binary: exit code 2, usage text on
// stderr, nothing on stdout, and the supported targets named for an unknown one.
func TestAssessUsage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		// wantStderr lists values stderr must hold besides the usage text.
		wantStderr []string
		// wantFirstLine lists values the first line, the error itself, must hold.
		wantFirstLine []string
	}{
		{
			name: "missing target",
			args: []string{
				"assess", "--inventory", filepath.Join("testdata", "inventory.json"),
			},
		},
		{
			name: "unknown target",
			args: []string{
				"assess", "--inventory", filepath.Join("testdata", "inventory.json"),
				"--target", "nope",
			},
			wantStderr:    []string{targetKey},
			wantFirstLine: []string{targetKey},
		},
		{
			name: "unknown flag",
			args: []string{
				"assess", "--inventory", filepath.Join("testdata", "inventory.json"),
				"--target", targetKey, "--bogus",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := run(t, emptyEnv(), tc.args...)

			if got.code != 2 {
				t.Errorf("exit code = %d, want 2; stderr = %q", got.code, got.stderr)
			}

			if got.stdout != "" {
				t.Errorf("stdout = %q, want empty", got.stdout)
			}

			if !strings.Contains(strings.ToLower(got.stderr), "usage") {
				t.Errorf("stderr = %q, want usage text", got.stderr)
			}

			for _, want := range tc.wantStderr {
				if !strings.Contains(got.stderr, want) {
					t.Errorf("stderr = %q, want it to name %q", got.stderr, want)
				}
			}

			// The error is the first line; the usage text follows it. A user who
			// mistypes a target learns the supported ones from that line.
			firstLine, _, _ := strings.Cut(got.stderr, "\n")
			for _, want := range tc.wantFirstLine {
				if !strings.Contains(firstLine, want) {
					t.Errorf("the first line %q, want it to name %q", firstLine, want)
				}
			}
		})
	}
}

// TestVersion pins that `eusov version` still works: one non-empty line, exit
// code 0, nothing on stderr.
func TestVersion(t *testing.T) {
	t.Parallel()

	got := run(t, emptyEnv(), "version")

	if got.code != 0 {
		t.Errorf("exit code = %d, want 0; stderr = %q", got.code, got.stderr)
	}

	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}

	if strings.TrimSpace(got.stdout) == "" {
		t.Errorf("stdout = %q, want a version line", got.stdout)
	}

	if !strings.HasSuffix(got.stdout, "\n") || strings.Count(got.stdout, "\n") != 1 {
		t.Errorf("stdout = %q, want exactly one line ending in a newline", got.stdout)
	}
}

// TestAssessIsDeterministic pins that two runs on the same inventory give the
// same bytes: nothing in the report depends on map order or the clock.
func TestAssessIsDeterministic(t *testing.T) {
	t.Parallel()

	inventory := filepath.Join("testdata", "inventory.json")

	first := assess(t, inventory)
	second := assess(t, inventory)

	if first.code != 0 || second.code != 0 {
		t.Fatalf("exit codes = %d and %d, want 0; stderr = %q and %q",
			first.code, second.code, first.stderr, second.stderr)
	}

	if first.stdout != second.stdout {
		t.Errorf("two runs differ:\nfirst:\n%s\nsecond:\n%s", first.stdout, second.stdout)
	}
}

// TestAssessPathHasNoNetworkOrCloudSDK pins the CLI criterion of the PRD: no
// package on the assess path imports net/http, os/exec or a cloud SDK, so the
// command cannot call a provider or start a process even by accident.
func TestAssessPathHasNoNetworkOrCloudSDK(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving the module root: %v", err)
	}

	cmd := exec.Command("go", "list", "-deps", "./cmd/eusov")
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("go list -deps ./cmd/eusov: %v\n%s", err, exitErr.Stderr)
		}

		t.Fatalf("go list -deps ./cmd/eusov: %v", err)
	}

	for _, pkg := range strings.Fields(string(out)) {
		if forbiddenDependency(pkg) {
			t.Errorf("the assess path depends on %s; no package on it may resolve a name, open a "+
				"connection, start a process or use a cloud SDK", pkg)
		}
	}
}

// forbiddenDependency reports whether pkg is a network, process or cloud SDK
// package the assess path must not depend on.
//
// net itself resolves names and opens connections, and every net subpackage that
// does I/O is refused with it. The two exceptions are parsers and value types:
// net/url turns a string into a URL and net/netip holds an address, and neither
// reaches the network, so the domain catalog and text packages may use them. A
// TLS handshake, a Google API client and the Scaleway, OVHcloud and UpCloud SDKs
// belong to an adapter, never to the assess path.
func forbiddenDependency(pkg string) bool {
	if pureNetPackage(pkg) {
		return false
	}

	for _, denied := range []string{"net", "os/exec", "crypto/tls"} {
		if pkg == denied || strings.HasPrefix(pkg, denied+"/") {
			return true
		}
	}

	for _, denied := range []string{
		"github.com/aws/",
		"cloud.google.com/",
		"github.com/Azure/",
		"google.golang.org/api/",
		"github.com/scaleway/",
		"github.com/ovh/",
		"github.com/UpCloudLtd/",
	} {
		if strings.HasPrefix(pkg, denied) {
			return true
		}
	}

	return false
}

// pureNetPackage reports whether pkg is a net subpackage that parses text or
// holds a value and never opens a connection or resolves a name.
func pureNetPackage(pkg string) bool {
	return pkg == "net/url" || pkg == "net/netip"
}

// hasCodeSpan reports whether s holds a Markdown code span whose content is
// exactly value.
func hasCodeSpan(s, value string) bool {
	for _, content := range codeSpans(s) {
		if content == value {
			return true
		}
	}

	return false
}

// codeSpans returns the content of every Markdown code span in s, with the
// padding CommonMark strips and the table-cell pipe escape undone, so a test can
// check what a renderer would show.
func codeSpans(s string) []string {
	var spans []string

	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++

			continue
		}

		start := i
		for i < len(s) && s[i] == '`' {
			i++
		}

		fence := i - start
		content, next, ok := spanContent(s, i, fence)

		if !ok {
			return spans
		}

		spans = append(spans, content)
		i = next
	}

	return spans
}

// spanContent returns the content of the code span whose fence is fence
// backticks long and whose content starts at byte from, the byte after the
// closing fence, and whether there is such a span.
func spanContent(s string, from, fence int) (content string, next int, ok bool) {
	for j := from; j < len(s); {
		if s[j] != '`' {
			j++

			continue
		}

		k := j
		for k < len(s) && s[k] == '`' {
			k++
		}

		// A closing fence is exactly as long as the opening one.
		if k-j == fence {
			content = strings.TrimSuffix(strings.TrimPrefix(s[from:j], " "), " ")
			content = strings.ReplaceAll(content, `\|`, "|")

			return content, k, true
		}

		j = k
	}

	return "", 0, false
}

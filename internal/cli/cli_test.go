package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"

	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/cli"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/testutil"
	"github.com/timothymamo/eu-sov-cloud/internal/version"
)

const (
	// reportBody is what the fake reporter writes, so a test can tell the
	// reporter's output from anything the CLI writes itself.
	reportBody = "# eusov report\n"
	// inventoryPath is the --inventory value the tests below use: a path with a
	// directory, so the test can see that a report shows the base name only.
	inventoryPath = "dir/inv.json"
	// inventoryName is the base name of inventoryPath, the only part of it a
	// report may show.
	inventoryName = "inv.json"
	// targetKey is the one target the fake deps support, as the CLI spells it.
	targetKey = "scaleway"
	// errPrefix is how the CLI writes an error to stderr.
	errPrefix = "eusov: "
	// maxErrorLine is the longest error line the CLI may write: the cap on the
	// message, the prefix, a short truncation marker and the newline.
	maxErrorLine = 1024 + len(errPrefix) + 64 + 1
)

// fakeInventorySource is an analysis.InventorySource that returns a fixed
// inventory or a fixed error and records how often it was called.
type fakeInventorySource struct {
	inv   inventory.Inventory
	err   error
	calls int
}

func (s *fakeInventorySource) Inventory(ctx context.Context) (inventory.Inventory, error) {
	s.calls++

	if err := ctx.Err(); err != nil {
		return inventory.Inventory{}, err
	}

	return s.inv, s.err
}

// fakeCatalogSource is an analysis.CatalogSource that returns a fixed catalog
// and records how often it was called.
type fakeCatalogSource struct {
	cat   catalog.Catalog
	calls int
}

func (s *fakeCatalogSource) Catalog(ctx context.Context) (catalog.Catalog, error) {
	s.calls++

	if err := ctx.Err(); err != nil {
		return catalog.Catalog{}, err
	}

	return s.cat, nil
}

// fakeReporter is an analysis.Reporter that writes a fixed body and then
// returns a fixed error. It records the metadata it was given, so a test can
// check what the CLI told the reporter about the run.
type fakeReporter struct {
	body []byte
	err  error
	meta analysis.ReportMeta
	// calls is the number of times Report was called.
	calls int
}

func (r *fakeReporter) Report(ctx context.Context, w io.Writer, _ assessment.Assessment, meta analysis.ReportMeta) error {
	r.calls++
	r.meta = meta

	if err := ctx.Err(); err != nil {
		return err
	}

	if len(r.body) > 0 {
		if _, err := w.Write(r.body); err != nil {
			return err
		}
	}

	return r.err
}

// harness holds the three fakes the CLI is run with and what they were asked
// for, so each test says what it expects of the run without building deps
// itself.
type harness struct {
	inventory *fakeInventorySource
	catalog   *fakeCatalogSource
	reporter  *fakeReporter
	// paths is every --inventory value the CLI passed to NewInventorySource.
	paths []string
}

// newHarness returns a harness whose fakes succeed.
func newHarness(t *testing.T) *harness {
	t.Helper()

	inv, err := inventory.New("aws", []inventory.Resource{{
		ID:       "i-0123456789abcdef0",
		RawType:  "AWS::EC2::Instance",
		Category: taxonomy.ComputeVM,
		Region:   "eu-west-1",
	}})
	if err != nil {
		t.Fatalf("inventory.New() returned unexpected error: %v", err)
	}

	return &harness{
		inventory: &fakeInventorySource{inv: inv},
		catalog:   &fakeCatalogSource{cat: testutil.ScalewayCatalog(t)},
		reporter:  &fakeReporter{body: []byte(reportBody)},
	}
}

// deps returns the Deps the CLI is run with.
func (h *harness) deps() cli.Deps {
	return cli.Deps{
		NewInventorySource: func(path string) analysis.InventorySource {
			h.paths = append(h.paths, path)

			return h.inventory
		},
		Targets: map[string]func() analysis.CatalogSource{
			targetKey: func() analysis.CatalogSource { return h.catalog },
		},
		NewReporter: func() analysis.Reporter { return h.reporter },
	}
}

// run runs the CLI with args and returns the exit code, stdout and stderr.
func (h *harness) run(ctx context.Context, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer

	code = cli.Run(ctx, args, &out, &errOut, h.deps())

	return code, out.String(), errOut.String()
}

// checkErrorLine fails the test unless stderr is exactly one line that starts
// with the prefix and holds no character a terminal or a log reader would act
// on: no raw control character (the final newline aside), no format character
// and no line or paragraph separator.
func checkErrorLine(t *testing.T, stderr string) {
	t.Helper()

	if !strings.HasPrefix(stderr, errPrefix) {
		t.Errorf("stderr = %q, want it to start with %q", stderr, errPrefix)
	}

	if !strings.HasSuffix(stderr, "\n") || strings.Count(stderr, "\n") != 1 {
		t.Errorf("stderr = %q, want exactly one line ending in a newline", stderr)
	}

	if len(stderr) <= len(errPrefix)+1 {
		t.Errorf("stderr = %q, want a message after the prefix", stderr)
	}

	if len(stderr) > maxErrorLine {
		t.Errorf("stderr is %d bytes, want at most %d", len(stderr), maxErrorLine)
	}

	for _, r := range stderr {
		if r == '\n' {
			continue
		}

		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			t.Errorf("stderr = %q holds the raw character %U; it must be escaped", stderr, r)

			break
		}
	}
}

// TestRunAssessToStdout pins the happy path: the assessment runs, the report
// goes to stdout, and the inventory file is named by its base name.
func TestRunAssessToStdout(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	code, stdout, stderr := h.run(t.Context(), "assess", "--inventory", inventoryPath, "--target", targetKey)

	if code != 0 {
		t.Errorf("Run() = %d, want 0; stderr = %q", code, stderr)
	}

	if stdout != reportBody {
		t.Errorf("stdout = %q, want the reporter's output %q", stdout, reportBody)
	}

	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}

	if len(h.paths) != 1 || h.paths[0] != inventoryPath {
		t.Errorf("NewInventorySource() was called with %q, want [%q]", h.paths, inventoryPath)
	}

	if h.reporter.calls != 1 {
		t.Errorf("the reporter was called %d times, want 1", h.reporter.calls)
	}

	if got := h.reporter.meta.InventoryName; got != inventoryName {
		t.Errorf("ReportMeta.InventoryName = %q, want the base name %q", got, inventoryName)
	}
}

// TestRunAssessToFile pins --output: the report is written to the file, nothing
// goes to stdout, and a failure leaves an existing destination untouched.
func TestRunAssessToFile(t *testing.T) {
	t.Parallel()

	t.Run("writes a new file with mode 0600", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		dest := filepath.Join(dir, "report.md")
		h := newHarness(t)

		code, stdout, stderr := h.run(t.Context(), "assess", "--inventory", inventoryPath,
			"--target", targetKey, "--output", dest)

		if code != 0 {
			t.Errorf("Run() = %d, want 0; stderr = %q", code, stderr)
		}

		if stdout != "" {
			t.Errorf("stdout = %q, want empty with --output", stdout)
		}

		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}

		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("the report file was not written: %v", err)
		}

		if string(got) != reportBody {
			t.Errorf("report file = %q, want %q", got, reportBody)
		}

		checkFileMode(t, dest)
		checkOnlyFile(t, dir, "report.md")
	})

	t.Run("replaces an existing file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		dest := filepath.Join(dir, "report.md")

		if err := os.WriteFile(dest, []byte("old report\n"), 0o600); err != nil {
			t.Fatalf("writing the existing destination: %v", err)
		}

		h := newHarness(t)

		code, stdout, stderr := h.run(t.Context(), "assess", "--inventory", inventoryPath,
			"--target", targetKey, "--output", dest)

		if code != 0 {
			t.Errorf("Run() = %d, want 0; stderr = %q", code, stderr)
		}

		if stdout != "" {
			t.Errorf("stdout = %q, want empty with --output", stdout)
		}

		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("reading the report file: %v", err)
		}

		if string(got) != reportBody {
			t.Errorf("report file = %q, want %q", got, reportBody)
		}

		checkFileMode(t, dest)
	})

	t.Run("keeps the destination when the reporter fails", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		dest := filepath.Join(dir, "report.md")
		old := []byte("old report\n")

		if err := os.WriteFile(dest, old, 0o600); err != nil {
			t.Fatalf("writing the existing destination: %v", err)
		}

		h := newHarness(t)
		// A reporter may write part of its output before it fails; the CLI must
		// not let that reach the destination.
		h.reporter.body = []byte("# partial report\n")
		h.reporter.err = errors.New("the reporter failed")

		code, stdout, stderr := h.run(t.Context(), "assess", "--inventory", inventoryPath,
			"--target", targetKey, "--output", dest)

		if code != 1 {
			t.Errorf("Run() = %d, want 1", code)
		}

		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}

		checkErrorLine(t, stderr)

		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("reading the destination after a failed run: %v", err)
		}

		if !bytes.Equal(got, old) {
			t.Errorf("destination = %q, want it unchanged as %q", got, old)
		}

		checkOnlyFile(t, dir, "report.md")
	})
}

// TestRunAssessRuntimeError pins the error line of a failed run: one line,
// prefixed, sanitised and capped, naming what failed.
func TestRunAssessRuntimeError(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.inventory.err = errors.New(hostileMessage())

	code, stdout, stderr := h.run(t.Context(), "assess", "--inventory", inventoryPath, "--target", targetKey)

	if code != 1 {
		t.Errorf("Run() = %d, want 1", code)
	}

	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}

	checkErrorLine(t, stderr)

	// The message says which stage failed, which is how a user tells a bad
	// inventory from a bad catalog.
	if !strings.Contains(stderr, "inventory") {
		t.Errorf("stderr = %q, want it to name the inventory stage", stderr)
	}
}

// TestRunAssessRuntimeErrorLeavesNoFile pins that a failed run with --output
// creates no destination at all when there was none.
func TestRunAssessRuntimeErrorLeavesNoFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dest := filepath.Join(dir, "report.md")
	h := newHarness(t)
	h.inventory.err = errors.New("the inventory file is not a valid inventory document")

	code, stdout, stderr := h.run(t.Context(), "assess", "--inventory", inventoryPath,
		"--target", targetKey, "--output", dest)

	if code != 1 {
		t.Errorf("Run() = %d, want 1", code)
	}

	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}

	checkErrorLine(t, stderr)

	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the destination exists after a failed run; stat error = %v", err)
	}

	checkOnlyFile(t, dir, "")
}

// TestRunUsageErrors pins every usage error: exit code 2, usage text on stderr,
// nothing on stdout and no adapter built.
func TestRunUsageErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		// wantStderr lists values stderr must hold besides the usage text.
		wantStderr []string
	}{
		{name: "no arguments", args: nil},
		{name: "unknown command", args: []string{"discover"}},
		{name: "assess without flags", args: []string{"assess"}},
		{name: "missing inventory", args: []string{"assess", "--target", targetKey}},
		{name: "missing target", args: []string{"assess", "--inventory", inventoryPath}},
		{
			name: "unknown flag",
			args: []string{"assess", "--inventory", inventoryPath, "--target", targetKey, "--bogus"},
		},
		{
			name: "extra positional argument",
			args: []string{"assess", "--inventory", inventoryPath, "--target", targetKey, "extra"},
		},
		{
			name:       "unknown target",
			args:       []string{"assess", "--inventory", inventoryPath, "--target", "nope"},
			wantStderr: []string{targetKey},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			code, stdout, stderr := h.run(t.Context(), tc.args...)

			if code != 2 {
				t.Errorf("Run() = %d, want 2; stderr = %q", code, stderr)
			}

			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}

			if !strings.Contains(strings.ToLower(stderr), "usage") {
				t.Errorf("stderr = %q, want usage text", stderr)
			}

			for _, want := range tc.wantStderr {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr = %q, want it to name %q", stderr, want)
				}
			}

			if len(h.paths) != 0 || h.reporter.calls != 0 {
				t.Errorf("a usage error built adapters: inventory paths %q, reporter calls %d",
					h.paths, h.reporter.calls)
			}
		})
	}
}

// TestRunVersion pins that the version command still works: the version line,
// exit code 0 and nothing on stderr.
func TestRunVersion(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{"version", "--version"} {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			code, stdout, stderr := h.run(t.Context(), arg)

			if code != 0 {
				t.Errorf("Run(%q) = %d, want 0; stderr = %q", arg, code, stderr)
			}

			if want := version.String() + "\n"; stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}

			if stderr != "" {
				t.Errorf("stderr = %q, want empty", stderr)
			}
		})
	}
}

// TestRunCancelled pins that a cancelled run, as SIGINT gives it, stops with
// exit code 1 and one error line.
func TestRunCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	h := newHarness(t)
	code, stdout, stderr := h.run(ctx, "assess", "--inventory", inventoryPath, "--target", targetKey)

	if code != 1 {
		t.Errorf("Run() = %d, want 1", code)
	}

	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}

	checkErrorLine(t, stderr)

	if h.reporter.calls != 0 {
		t.Errorf("the reporter was called %d times for a cancelled run, want 0", h.reporter.calls)
	}
}

// hostileMessage is an error text no error line may reproduce: a line feed that
// would end the line, a terminal escape that would clear the screen, a
// right-to-left override that would reverse it, and enough length to grow
// without a cap.
func hostileMessage() string {
	var b strings.Builder

	b.WriteString("inventory ")
	b.WriteString("\n")
	b.WriteString("\x1b[2J")
	b.WriteString("\u202e")

	for b.Len() < 5000 {
		b.WriteString("x")
	}

	return b.String()
}

// checkFileMode fails the test unless path was written with mode 0600. Modes
// are not modelled the same way on every operating system, so the check runs
// where they are.
func checkFileMode(t *testing.T, path string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}

	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("%s has mode %04o, want 0600", path, got)
	}
}

// checkOnlyFile fails the test unless dir holds exactly one entry, want, or no
// entry at all when want is empty. A temporary file left behind by a failed
// --output is a second entry.
func checkOnlyFile(t *testing.T, dir, want string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the output directory: %v", err)
	}

	if want == "" {
		if len(entries) != 0 {
			t.Errorf("the output directory holds %d entries, want none", len(entries))
		}

		return
	}

	if len(entries) != 1 || entries[0].Name() != want {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}

		t.Errorf("the output directory holds %q, want only %q", names, want)
	}
}

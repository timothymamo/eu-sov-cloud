//go:build unix

package cli_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestRunAssessOutputIsAFIFO pins that --output naming a FIFO, which is not a
// report file, fails the run and leaves the FIFO as it was: a rename must never
// replace it with a regular file, and no temporary file is left behind.
func TestRunAssessOutputIsAFIFO(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dest := filepath.Join(dir, "report.fifo")

	if err := syscall.Mkfifo(dest, 0o600); err != nil {
		t.Skipf("creating a FIFO: %v", err)
	}

	h := newHarness(t)

	code, stdout, stderr := h.run(t.Context(), "assess", "--inventory", inventoryPath,
		"--target", targetKey, "--output", dest)

	if code != 1 {
		t.Errorf("Run() = %d, want 1", code)
	}

	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}

	checkErrorLine(t, stderr)

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("the FIFO is gone after the run: %v", err)
	}

	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Errorf("the destination has mode %s, want it still a FIFO", info.Mode())
	}

	if got := tempReportFiles(t, dir); len(got) != 0 {
		t.Errorf("the output directory holds the temporary files %q, want none", got)
	}

	checkOnlyFile(t, dir, "report.fifo")
}

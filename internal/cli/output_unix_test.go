//go:build unix

package cli_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestRunAssessOutputIsAFIFO pins that --output naming a FIFO, or a symbolic
// link to one, fails the run and leaves both as they were: a rename must never
// replace a FIFO, or the link to it, with a regular file. The destination is
// refused before the inventory is read, and no temporary file is left behind.
func TestRunAssessOutputIsAFIFO(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// link makes the destination a symbolic link to the FIFO instead of
		// being that FIFO.
		link bool
		// wantEntries lists the output directory after the run, sorted.
		wantEntries []string
	}{
		{name: "a FIFO", wantEntries: []string{"real.fifo"}},
		{
			name:        "a symbolic link to a FIFO",
			link:        true,
			wantEntries: []string{"real.fifo", "report.fifo"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			real := filepath.Join(dir, "real.fifo")

			if err := syscall.Mkfifo(real, 0o600); err != nil {
				t.Skipf("creating a FIFO: %v", err)
			}

			dest := real
			if tc.link {
				dest = filepath.Join(dir, "report.fifo")

				if err := os.Symlink("real.fifo", dest); err != nil {
					t.Skipf("symbolic links are not supported here: %v", err)
				}
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

			info, err := os.Stat(real)
			if err != nil {
				t.Fatalf("the FIFO is gone after the run: %v", err)
			}

			if info.Mode()&os.ModeNamedPipe == 0 {
				t.Errorf("the FIFO has mode %s, want it still a FIFO", info.Mode())
			}

			// A refused destination is refused as it stands, so a link to the
			// FIFO is still that link.
			if tc.link {
				checkSymlink(t, dest, "real.fifo")
			}

			if got := tempReportFiles(t, dir); len(got) != 0 {
				t.Errorf("the output directory holds the temporary files %q, want none", got)
			}

			checkEntries(t, dir, tc.wantEntries...)

			// The destination is refused before anything is read.
			if h.inventory.calls != 0 || h.reporter.calls != 0 {
				t.Errorf("a refused output read the inventory %d times and reported %d, want 0 and 0",
					h.inventory.calls, h.reporter.calls)
			}
		})
	}
}

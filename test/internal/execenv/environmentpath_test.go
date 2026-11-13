package execenv_test

import (
	"testing"

	"github.com/timothymamo/shift2eu/test/internal/execenv"
)

// The matcher of the environ rule has a file of its own: it pins paths as
// values, and a bare path literal in a Go file under test/ is what
// TestTestTreeNamesNoProcEnviron refuses, so that scan leaves this one file out
// by name (test/lint's environPathsPinnedBy). The rule's probes are .go.txt
// files, which no scan of Go files meets.

// TestEnvironmentPathIn pins the shared segment matcher on values, the cases
// test/lint's rule pinned on source before issue #195 round 2 moved it here:
// its own entries, "/dev/fd", and the same ones without a leading slash, which
// os.DirFS("/") opens; a directory named without a trailing slash; and the two
// names that only look like one.
func TestEnvironmentPathIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		value  string
		want   string
		wantOK bool
	}{
		{
			name:   "the environ file of this process",
			value:  "/proc/self/environ",
			want:   "proc",
			wantOK: true,
		},
		{
			name:   "the environ file of another process",
			value:  "/proc/1/environ",
			want:   "proc",
			wantOK: true,
		},
		{
			name:   "a descriptor of this process",
			value:  "/dev/fd/3",
			want:   "dev/fd",
			wantOK: true,
		},
		{
			name:   "the environ file, without the leading slash",
			value:  "proc/self/environ",
			want:   "proc",
			wantOK: true,
		},
		{
			name:   "a descriptor, without the leading slash",
			value:  "dev/fd/3",
			want:   "dev/fd",
			wantOK: true,
		},
		{
			name:   "the directory itself",
			value:  "/proc",
			want:   "proc",
			wantOK: true,
		},
		{
			name:   "the directory itself, without the leading slash",
			value:  "proc",
			want:   "proc",
			wantOK: true,
		},
		{
			name:   "a descriptor directory, named without a trailing slash",
			value:  "/dev/fd",
			want:   "dev/fd",
			wantOK: true,
		},
		{
			name:   "the directory with a trailing slash only",
			value:  "/proc/",
			want:   "proc",
			wantOK: true,
		},
		{
			// The e2e tier's rule is the segment, not the whole value: any
			// path into /proc names something a test there has no business
			// naming.
			name:   "another file of the process, deeper in /proc",
			value:  "/proc/self/status",
			want:   "proc",
			wantOK: true,
		},
		{
			name:   "a nested entry of the process",
			value:  "/proc/self/task/1/environ",
			want:   "proc",
			wantOK: true,
		},
		{
			name:  "a path of the root's own",
			value: "/etc/shift2eu/config.json",
		},
		{
			name:  "a name that only ends in one",
			value: "myproc/self/environ",
		},
		{
			name:  "a directory that only starts with one",
			value: "/procfs/self/environ",
		},
		{
			name:  "no path at all",
			value: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := execenv.EnvironmentPathIn(tc.value)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("EnvironmentPathIn(%q) = (%q, %t), want (%q, %t)", tc.value, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

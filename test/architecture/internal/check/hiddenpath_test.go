package check_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/timothymamo/shift2eu/test/architecture/internal/check"
)

// TestHiddenPathPackages pins check.HiddenPathPackages (issue #206). The go
// command skips a directory named testdata, and a directory whose name starts
// with _ or ., when it walks ./..., but it still builds a package in one of
// them when another package imports it by explicit path. depguard and
// test/lint walk the tree the way the go command does, so a package imported
// that way reaches a release binary with no architecture or lint rule applied
// to it.
//
// The contract:
//
//   - HiddenPathPackages(modulePath string, importPaths []string) []string
//     returns the import paths that are modulePath or under it and that hold,
//     below modulePath, an element that is exactly "testdata" or starts with _
//     or ., sorted, each once.
//   - Only the elements below the module path count: a module path that itself
//     holds such an element (example.com/_mod) reports none of its own
//     packages for it, and neither does a path that merely shares the module
//     path's prefix as a string (example.com/mod2 is not under
//     example.com/mod).
//   - A package outside the module is never reported, and neither is the
//     module root: the check looks at what the module builds, and a path of
//     another module is that module's business.
//   - A name that only looks hidden is no element of the go command's set:
//     a_b, x.y and testdatax are ordinary directory names.
func TestHiddenPathPackages(t *testing.T) {
	t.Parallel()

	const mod = "example.com/mod"

	tests := []struct {
		name        string
		modulePath  string
		importPaths []string
		want        []string
	}{
		{
			name:        "nothing to check reports nothing",
			modulePath:  mod,
			importPaths: nil,
			want:        nil,
		},
		{
			name:        "a testdata element below the module path is reported",
			modulePath:  mod,
			importPaths: []string{mod + "/testdata/x"},
			want:        []string{mod + "/testdata/x"},
		},
		{
			name:        "a testdata element as the last element is reported",
			modulePath:  mod,
			importPaths: []string{mod + "/a/testdata"},
			want:        []string{mod + "/a/testdata"},
		},
		{
			name:        "the testdata package beside the module root is reported",
			modulePath:  mod,
			importPaths: []string{mod + "/testdata"},
			want:        []string{mod + "/testdata"},
		},
		{
			name:        "an underscore-prefixed element below the module root is reported",
			modulePath:  mod,
			importPaths: []string{mod + "/_x"},
			want:        []string{mod + "/_x"},
		},
		{
			name:        "an underscore-prefixed element in the middle is reported",
			modulePath:  mod,
			importPaths: []string{mod + "/a/_x/b"},
			want:        []string{mod + "/a/_x/b"},
		},
		{
			name:        "a dot-prefixed element below the module root is reported",
			modulePath:  mod,
			importPaths: []string{mod + "/.x"},
			want:        []string{mod + "/.x"},
		},
		{
			name:        "a dot-prefixed element in the middle is reported",
			modulePath:  mod,
			importPaths: []string{mod + "/a/.x/y"},
			want:        []string{mod + "/a/.x/y"},
		},
		{
			name:       "names that only look hidden are no element the go command skips",
			modulePath: mod,
			importPaths: []string{
				mod + "/a_b",
				mod + "/x.y",
				mod + "/testdatax",
				mod + "/internal/x",
			},
			want: nil,
		},
		{
			name:       "a package outside the module is not reported",
			modulePath: mod,
			importPaths: []string{
				"example.org/other/_x",
				"example.org/other/testdata/x",
				"internal/_x",
			},
			want: nil,
		},
		{
			name:       "a path that only shares the module path's prefix is no package under it",
			modulePath: mod,
			importPaths: []string{
				mod + "2/testdata/x",
				mod + "x/_x",
			},
			want: nil,
		},
		{
			name:        "the module root itself is not reported",
			modulePath:  mod,
			importPaths: []string{mod},
			want:        nil,
		},
		{
			// The go command skips such a directory inside the module, not one
			// the module path already names: the module's own packages are
			// built and listed whatever its path holds.
			name:       "only elements below the module path count",
			modulePath: "example.com/_mod",
			importPaths: []string{
				"example.com/_mod/x",
				"example.com/_mod/.x",
			},
			want: []string{"example.com/_mod/.x"},
		},
		{
			name:        "the module path holding testdata still reports the ones below it",
			modulePath:  "example.com/testdata/mod",
			importPaths: []string{"example.com/testdata/mod/x", "example.com/testdata/mod/_x"},
			want:        []string{"example.com/testdata/mod/_x"},
		},
		{
			name:        "an import path reported twice is reported once",
			modulePath:  mod,
			importPaths: []string{mod + "/testdata/x", mod + "/testdata/x"},
			want:        []string{mod + "/testdata/x"},
		},
		{
			name:       "the result is sorted",
			modulePath: mod,
			importPaths: []string{
				mod + "/testdata/x",
				mod + "/a/normal",
				mod + "/_x",
			},
			want: []string{mod + "/_x", mod + "/testdata/x"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := check.HiddenPathPackages(tc.modulePath, tc.importPaths)

			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("HiddenPathPackages(%q, %q) mismatch (-want +got):\n%s", tc.modulePath, tc.importPaths, diff)
			}
		})
	}
}

// TestHiddenPathMessages pins check.HiddenPathMessages (issue #206), the finding
// the composition roots are held to: one message per package
// check.HiddenPathPackages reports among the listing, sorted, each naming the
// composition root's directory and the package, and saying why the package
// escaped every rule that walks the tree.
//
// The contract:
//
//   - HiddenPathMessages(modulePath, rootDir string, importPaths []string)
//     []string returns no message for a listing without a hidden-path package.
//   - Each message reads "<rootDir> reaches <import path>, which ./... never
//     walks but go builds when a package imports it by explicit path: no lint
//     or architecture rule sees it", with rootDir as the composition root's
//     directory relative to the module root (cmd/shift2eu) and the import path
//     as go list reports it. The verb is reaches, not imports: go list -deps
//     lists the root's transitive dependencies, not only the packages it
//     imports itself.
//   - A package outside the module earns no message, as in HiddenPathPackages.
func TestHiddenPathMessages(t *testing.T) {
	t.Parallel()

	const (
		mod     = "example.com/hiddenpathprobe"
		rootDir = "cmd/shift2eu"
	)

	tests := []struct {
		name        string
		modulePath  string
		rootDir     string
		importPaths []string
		want        []string
	}{
		{
			name:        "a listing without a hidden-path package earns no message",
			modulePath:  mod,
			rootDir:     rootDir,
			importPaths: []string{mod + "/cmd/shift2eu", mod + "/internal/x", "os"},
			want:        nil,
		},
		{
			name:        "each hidden-path package earns a message naming the root and the package",
			modulePath:  mod,
			rootDir:     rootDir,
			importPaths: []string{mod + "/testdata/x", mod + "/cmd/shift2eu", mod + "/_x", mod + "/.x", mod + "/internal/x"},
			want: []string{
				"cmd/shift2eu reaches example.com/hiddenpathprobe/.x, which ./... never walks but go builds when a package imports it by explicit path: no lint or architecture rule sees it",
				"cmd/shift2eu reaches example.com/hiddenpathprobe/_x, which ./... never walks but go builds when a package imports it by explicit path: no lint or architecture rule sees it",
				"cmd/shift2eu reaches example.com/hiddenpathprobe/testdata/x, which ./... never walks but go builds when a package imports it by explicit path: no lint or architecture rule sees it",
			},
		},
		{
			name:        "a package outside the module earns no message",
			modulePath:  mod,
			rootDir:     rootDir,
			importPaths: []string{"example.org/other/_x"},
			want:        nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := check.HiddenPathMessages(tc.modulePath, tc.rootDir, tc.importPaths)

			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("HiddenPathMessages(%q, %q, %q) mismatch (-want +got):\n%s", tc.modulePath, tc.rootDir, tc.importPaths, diff)
			}
		})
	}
}

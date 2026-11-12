package docs_test

import (
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/test/internal/mdlink"
	"github.com/timothymamo/shift2eu/test/internal/yamlread"
)

// archiveFiles reads the files each archive in .goreleaser.yaml ships beside
// its binary: one list per archives: entry, in the order the document writes
// them, since issue #147 gives each binary an archive of its own.
func archiveFiles(t *testing.T, fsys fs.FS) [][]string {
	t.Helper()

	data, err := fs.ReadFile(fsys, ".goreleaser.yaml")
	if err != nil {
		t.Fatalf("reading .goreleaser.yaml: %v", err)
	}
	lines, err := yamlread.SplitLines(data)
	if err != nil {
		t.Fatalf(".goreleaser.yaml: %v", err)
	}
	block, err := yamlread.Walk(lines, "archives")
	if err != nil {
		t.Fatalf(".goreleaser.yaml: %v", err)
	}
	archives, err := yamlread.Sequence(block)
	if err != nil {
		t.Fatalf(".goreleaser.yaml: archives: %v", err)
	}
	if len(archives) == 0 {
		t.Fatal(".goreleaser.yaml: archives: holds no entry, so the release ships nothing this test could read")
	}
	lists := make([][]string, 0, len(archives))
	for i, archive := range archives {
		entries, err := yamlread.Mapping(archive.Lines)
		if err != nil {
			t.Fatalf(".goreleaser.yaml: archive %d: %v", i+1, err)
		}
		files := []string(nil)
		for _, e := range entries {
			if e.Key == "files" {
				files, err = yamlread.List(e)
				if err != nil {
					t.Fatalf(".goreleaser.yaml: archive %d: files: %v", i+1, err)
				}
			}
		}
		if files == nil {
			t.Fatalf(".goreleaser.yaml: archive %d lists no files", i+1)
		}
		lists = append(lists, files)
	}
	return lists
}

// shipped reports whether name is in an archive whose files are files, read
// as GoReleaser v2 reads them: D/**/* matches every file in a directory below
// D, but not a file directly in D, and any other entry is a path.Match
// pattern. A directory is shipped when a file in it is.
func shipped(name string, files []string) bool {
	for _, f := range files {
		if dir, ok := strings.CutSuffix(f, "/**/*"); ok {
			if rest, ok := strings.CutPrefix(name, dir+"/"); ok && strings.Contains(rest, "/") {
				return true
			}
			continue
		}
		if ok, err := path.Match(f, name); err == nil && ok {
			return true
		}
	}
	return false
}

// TestReadmeLinksStayInTheArchive fails on a relative link in README.md to a
// file or directory a release archive does not ship: every archive ships
// README.md, so its links must work in an unpacked archive too, whichever
// binary's archive it is. Deep links into the source belong in the pages under
// docs/.
func TestReadmeLinksStayInTheArchive(t *testing.T) {
	t.Parallel()

	fsys := os.DirFS(repoRoot)
	archives := archiveFiles(t, fsys)
	for i, files := range archives {
		if !shipped("README.md", files) {
			t.Fatalf(".goreleaser.yaml: archive %d does not ship README.md; files: %q", i+1, files)
		}
	}

	data, err := fs.ReadFile(fsys, "README.md")
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}
	for _, l := range mdlink.Links(data) {
		target, _, _ := strings.Cut(l.Dest, "#")
		if target == "" || strings.Contains(target, ":") {
			continue
		}
		name := path.Clean(target)
		for i, files := range archives {
			if !shipped(name, files) {
				t.Errorf("README.md:%d: link %q: archive %d of .goreleaser.yaml does not ship %s (archives: files: %q)", l.Line, l.Dest, i+1, name, files)
			}
		}
	}
}

// TestIndexListsEveryPage fails on a Markdown page under docs/ that
// docs/README.md, the documentation's index, does not link to.
func TestIndexListsEveryPage(t *testing.T) {
	t.Parallel()

	fsys := os.DirFS(repoRoot)
	const index = "docs/README.md"
	data, err := fs.ReadFile(fsys, index)
	if err != nil {
		t.Fatalf("reading %s: %v", index, err)
	}
	linked := map[string]bool{}
	for _, l := range mdlink.Links(data) {
		target, _, _ := strings.Cut(l.Dest, "#")
		if target != "" && !strings.Contains(target, ":") {
			linked[path.Join("docs", target)] = true
		}
	}

	for _, page := range markdownFiles(t, fsys, "docs")[1:] { // [0] is README.md
		if page != index && !linked[page] {
			t.Errorf("%s: not listed in %s, the documentation's index", page, index)
		}
	}
}

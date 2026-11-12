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

// archiveFiles reads the files every archive in .goreleaser.yaml ships beside
// the binaries.
func archiveFiles(t *testing.T, fsys fs.FS) []string {
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
	if len(archives) != 1 {
		t.Fatalf(".goreleaser.yaml: %d archives, want 1: this test reads the files of one", len(archives))
	}
	entries, err := yamlread.Mapping(archives[0].Lines)
	if err != nil {
		t.Fatalf(".goreleaser.yaml: archives: %v", err)
	}
	for _, e := range entries {
		if e.Key == "files" {
			files, err := yamlread.List(e)
			if err != nil {
				t.Fatalf(".goreleaser.yaml: archives: %v", err)
			}
			return files
		}
	}
	t.Fatal(".goreleaser.yaml: the archive lists no files")
	return nil
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
// file or directory the release archives do not ship: the archives ship
// README.md, so its links must work in an unpacked archive too. Deep links
// into the source belong in the pages under docs/.
func TestReadmeLinksStayInTheArchive(t *testing.T) {
	t.Parallel()

	fsys := os.DirFS(repoRoot)
	files := archiveFiles(t, fsys)
	if !shipped("README.md", files) {
		t.Fatalf(".goreleaser.yaml: the archives do not ship README.md; files: %q", files)
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
		if name := path.Clean(target); !shipped(name, files) {
			t.Errorf("README.md:%d: link %q: the release archives do not ship %s (.goreleaser.yaml, archives: files: %q)", l.Line, l.Dest, name, files)
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

package examples_test

import (
	"path/filepath"
	"slices"
	"testing"
)

// TestSampleInventoriesAreValid pins that every file a reader can find under
// examples/ is a sample of the source its directory names: the inventory-file
// adapter, not a test-only decoder, accepts it, and its declared source is the
// directory. It also fails when a source has no sample at all, so the
// walkthrough can point every command at examples/<source>/inventory.json.
func TestSampleInventoriesAreValid(t *testing.T) {
	t.Parallel()

	files := sampleFiles(t)

	for _, file := range files {
		t.Run(file.name, func(t *testing.T) {
			t.Parallel()

			if !slices.Contains(sources, file.source) {
				t.Errorf("%s is in %q, which is not a source directory; a sample lives in one of %s",
					file.name, file.source, sources)
			}

			inv := loadSample(t, file.path)
			if got := inv.Source(); got != file.source {
				t.Errorf("%s: source = %q, want the directory's name %q", file.name, got, file.source)
			}
		})
	}

	t.Run("every source has a sample", func(t *testing.T) {
		t.Parallel()

		for _, source := range sources {
			if !slices.ContainsFunc(files, func(file sample) bool { return file.source == source }) {
				t.Errorf("no sample inventory for source %s: want at least one *.json in %s",
					source, filepath.ToSlash(filepath.Join(examplesDir, source)))
			}
		}
	})
}

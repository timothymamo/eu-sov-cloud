package examples_test

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/internal/adapter/source/inventoryfile"
	"github.com/timothymamo/shift2eu/internal/adapter/target/ovhcloud"
	"github.com/timothymamo/shift2eu/internal/adapter/target/scaleway"
	"github.com/timothymamo/shift2eu/internal/adapter/target/upcloud"
	"github.com/timothymamo/shift2eu/internal/app/analysis"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
)

const (
	// repoRoot is the module root, from this package's directory.
	repoRoot = "../.."
	// examplesDir is the top-level directory the samples live in.
	examplesDir = "examples"
	// inventoryFileName is the name of the sample inventory in each source
	// directory. The walkthrough and the report goldens name it, so the samples
	// use exactly this name (the demo commands pass examples/<source>/inventory.json).
	inventoryFileName = "inventory.json"
)

// sources are the source directories under examples/, one per hyperscaler. An
// inventory has one source, so a sample in examples/aws says "aws".
var sources = []string{"aws", "azure", "gcp"}

// sample is one sample inventory file: the path to hand the inventory-file
// adapter, its name below examples/ for a test and error message, and the
// source its directory names.
type sample struct {
	path   string
	name   string
	source string
}

// sampleFiles returns every *.json file under examples/, sorted by path, with
// the source its directory names. It fails the test when the examples
// directory is missing or holds no sample, so a test never passes by checking
// nothing.
func sampleFiles(t *testing.T) []sample {
	t.Helper()

	return filesUnder(t, "sample inventory", func(path string) bool {
		return strings.EqualFold(filepath.Ext(path), ".json")
	})
}

// allFiles returns every regular file under examples/, the READMEs included,
// sorted by path. TestSamplesAreSanitised scans all of them: prose and a
// command leak an identifier as easily as a field does, so the documentation
// values rule (CLAUDE.md rule 2) holds for the whole directory.
func allFiles(t *testing.T) []sample {
	t.Helper()

	return filesUnder(t, "file", func(string) bool { return true })
}

// filesUnder returns every regular file under examples/ that keep selects,
// sorted by path, with the source its directory names.
func filesUnder(t *testing.T, kind string, keep func(path string) bool) []sample {
	t.Helper()

	root := filepath.Join(repoRoot, examplesDir)
	var files []sample

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || !entry.Type().IsRegular() || !keep(path) {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		name := filepath.ToSlash(rel)
		source, _, _ := strings.Cut(name, "/")

		files = append(files, sample{path: path, name: name, source: source})

		return nil
	})
	if err != nil {
		t.Fatalf("walk the files under %s: %v", root, err)
	}

	if len(files) == 0 {
		t.Fatalf("no %s under %s: want at least one, for example %s",
			kind, root, filepath.ToSlash(filepath.Join(examplesDir, "aws", inventoryFileName)))
	}

	slices.SortFunc(files, func(a, b sample) int { return strings.Compare(a.path, b.path) })

	return files
}

// samplePath returns the path of one source's sample inventory, relative to
// this package's directory.
func samplePath(source string) string {
	return filepath.Join(repoRoot, examplesDir, source, inventoryFileName)
}

// loadSample decodes one sample through the real inventory-file adapter, the
// one cmd/shift2eu wires.
func loadSample(t *testing.T, path string) inventory.Inventory {
	t.Helper()

	inv, err := inventoryfile.New(path).Inventory(t.Context())
	if err != nil {
		t.Fatalf("inventoryfile.New(%q).Inventory: %v: the sample is missing or is not a valid inventory", path, err)
	}

	return inv
}

// targetCatalog is one target a sample must assess against: the key the CLI
// selects it by and the constructor of its catalog source, as cmd/shift2eu
// wires it.
type targetCatalog struct {
	key     string
	catalog func() analysis.CatalogSource
}

// targets are the targets cmd/shift2eu wires, in the key order the CLI sorts
// a comparison into.
var targets = []targetCatalog{
	{key: "ovhcloud", catalog: func() analysis.CatalogSource { return ovhcloud.New() }},
	{key: "scaleway", catalog: func() analysis.CatalogSource { return scaleway.New() }},
	{key: "upcloud", catalog: func() analysis.CatalogSource { return upcloud.New() }},
}

// TestSanitisedFilesCoverEverySampleFile pins that the sanitisation check reads
// every file a reader finds under examples/, the READMEs included, and not only
// the JSON inventories (the audit's L-2 finding on issue #179): a README quotes
// values and commands, and leaks an identifier as easily as a field does.
func TestSanitisedFilesCoverEverySampleFile(t *testing.T) {
	t.Parallel()

	all := allFiles(t)

	names := make([]string, len(all))
	for i, file := range all {
		names[i] = file.name
	}

	for _, file := range sampleFiles(t) {
		if !slices.Contains(names, file.name) {
			t.Errorf("%s is not among the files the sanitisation check reads: %v", file.name, names)
		}
	}

	want := []string{"README.md"}
	for _, source := range sources {
		want = append(want, source+"/README.md")
	}

	for _, name := range want {
		if !slices.Contains(names, name) {
			t.Errorf("%s is not among the files the sanitisation check reads: %v", name, names)
		}
	}
}

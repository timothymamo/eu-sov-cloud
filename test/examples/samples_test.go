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

	root := filepath.Join(repoRoot, examplesDir)
	var files []sample

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".json") {
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
		t.Fatalf("walk the sample inventories under %s: %v: want a *%s per source, for example %s",
			root, err, ".json", filepath.ToSlash(filepath.Join(examplesDir, "aws", inventoryFileName)))
	}

	if len(files) == 0 {
		t.Fatalf("no sample inventory under %s: want at least one *.json per source (%s), for example %s",
			root, strings.Join(sources, ", "), filepath.ToSlash(filepath.Join(examplesDir, "aws", inventoryFileName)))
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

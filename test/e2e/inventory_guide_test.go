//go:build e2e

package e2e_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/internal/testutil"
)

// This file pins the jq programs docs/guide/inventory.md documents (issue #178):
// each program turns the recorded output of the guide's read-only CLI calls into
// a schema version 1 inventory the binary accepts. The programs are part of what
// the guide promises, so they are run as the guide runs them, against the
// recorded fixtures, not just read.

const (
	// inventoryGuideDir holds the recorded, sanitised CLI output of the guide,
	// one directory per source.
	inventoryGuideDir = "testdata/inventory-guide"
	// jqProgramDir holds the programs the guide runs.
	jqProgramDir = "../../docs/guide/inventory"
	// inventoryGuideGoldenDir holds the inventories the programs write. It is a
	// directory of its own because the gcp and azure programs read every
	// <source>/*.json file: a golden beside them would be an input too.
	inventoryGuideGoldenDir = "inventory-guide/golden"
	// schemaVersion is the only schema version the binary reads (ADR 0006).
	schemaVersion = 1
)

// inventoryGuideCase is one source the guide covers: the recorded CLI output its
// program reads, and the inventory the program writes for it.
type inventoryGuideCase struct {
	// source is the inventory's source and the program's name without .jq.
	source string
	// pattern is the glob, relative to this package, of the recorded CLI output.
	// The AWS calls are per region, so its pattern has one more level.
	pattern string
	// golden is the inventory the program writes for the recorded output. It is
	// named .golden rather than .json because .gitignore excludes
	// *.inventory.json: a real inventory describes someone's infrastructure,
	// while this one is a sanitised fixture and has to be committed.
	golden string
}

// inventoryGuides returns the sources the guide covers. A source added to the
// guide gets a row here, its fixtures and a golden file.
func inventoryGuides() []inventoryGuideCase {
	return []inventoryGuideCase{
		{source: "aws", pattern: inventoryGuideDir + "/aws/*/*.json", golden: "aws.inventory.golden"},
		{source: "gcp", pattern: inventoryGuideDir + "/gcp/*.json", golden: "gcp.inventory.golden"},
		{source: "azure", pattern: inventoryGuideDir + "/azure/*.json", golden: "azure.inventory.golden"},
	}
}

// taxonomyCategories are the taxonomy categories the recorded fixtures cover.
// They are written out because this package is black box and imports no domain
// package (the e2e-is-black-box depguard rule), and because a -update that
// dropped a mapping must not be able to hide itself: the inventory is checked
// against this list, not against the golden file it was written from.
func taxonomyCategories() []string {
	return []string{
		"compute.vm",
		"storage.block",
		"storage.object",
		"database.postgresql",
		"database.mysql",
		"kubernetes.managed",
		"serverless.function",
		"messaging.queue",
	}
}

// inventoryDoc is the part of a schema version 1 inventory this file checks.
type inventoryDoc struct {
	SchemaVersion int    `json:"schemaVersion"`
	Source        string `json:"source"`
	Resources     []struct {
		ID       string `json:"id"`
		RawType  string `json:"rawType"`
		Category string `json:"category"`
		Region   string `json:"region"`
	} `json:"resources"`
}

// jqPath returns the path of the jq every release target has installed. A test
// that cannot run the programs fails instead of skipping: the guide's promise
// would otherwise go unchecked wherever jq is missing.
func jqPath(t *testing.T) string {
	t.Helper()

	path, err := exec.LookPath("jq")
	if err != nil {
		t.Fatalf("jq is not on PATH; the inventory guide's programs need it (install jq): %v", err)
	}

	return path
}

// TestInventoryGuidePrograms pins docs/guide/inventory/<source>.jq end to end:
// the program, run as the guide runs it on the recorded CLI output, writes the
// golden inventory, and the built binary assesses that inventory against every
// target without an error. The assertions beyond the golden file are the
// guide's promises that a careless -update must not be able to accept away: the
// document's schema version and source, every taxonomy category the fixtures
// cover, an unclassified resource that keeps its rawType, and short IDs only.
func TestInventoryGuidePrograms(t *testing.T) {
	t.Parallel()

	jq := jqPath(t)

	for _, tc := range inventoryGuides() {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()

			inputs, err := filepath.Glob(tc.pattern)
			if err != nil {
				t.Fatalf("glob %s: %v", tc.pattern, err)
			}

			if len(inputs) == 0 {
				t.Fatalf("%s matched no recorded CLI output; the fixtures are missing", tc.pattern)
			}

			slices.Sort(inputs)

			stdout := runJQ(t, jq, tc.source, inputs)

			testutil.Golden(t, filepath.Join("testdata", inventoryGuideGoldenDir, tc.golden), stdout)

			doc := decodeInventory(t, stdout)

			if doc.SchemaVersion != schemaVersion {
				t.Errorf("schemaVersion = %d, want %d", doc.SchemaVersion, schemaVersion)
			}

			if doc.Source != tc.source {
				t.Errorf("source = %q, want %q", doc.Source, tc.source)
			}

			if len(doc.Resources) == 0 {
				t.Fatalf("the inventory holds no resource; the program classified nothing\n%s", stdout)
			}

			assertCategories(t, tc.source, doc)
			assertUnclassifiedKeepsRawType(t, tc.source, doc)
			assertShortIDs(t, tc.source, doc)

			assertAssessAccepts(t, tc, stdout)
		})
	}
}

// runJQ runs jq -n -f the source's program on the recorded CLI output, as the
// guide does, and returns what it wrote to stdout. The program must exit 0 with
// nothing on stderr, so a warning a reader would not notice fails the test.
func runJQ(t *testing.T, jq, source string, inputs []string) []byte {
	t.Helper()

	args := append([]string{"-n", "-f", filepath.Join(jqProgramDir, source+".jq")}, inputs...)

	cmd := exec.Command(jq, args...)

	var stdout, stderr strings.Builder

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// The program needs no environment: it reads the files the arguments name.
	cmd.Env = emptyEnv()

	err := cmd.Run()

	var exitErr *exec.ExitError

	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		t.Fatalf("jq -n -f %s %v exited %d, want 0; stderr = %q",
			source+".jq", inputs, exitErr.ExitCode(), stderr.String())
	default:
		t.Fatalf("running jq on %s's recorded CLI output: %v", source, err)
	}

	if stderr.Len() != 0 {
		t.Errorf("jq wrote to stderr, want nothing: %q", stderr.String())
	}

	return []byte(stdout.String())
}

// decodeInventory decodes the document the program wrote. The binary has to
// decode it too, so a document that does not decode fails here first.
func decodeInventory(t *testing.T, document []byte) inventoryDoc {
	t.Helper()

	var doc inventoryDoc
	if err := json.Unmarshal(document, &doc); err != nil {
		t.Fatalf("the program's output is not a JSON inventory: %v\n%s", err, document)
	}

	return doc
}

// assertCategories fails unless every taxonomy category the fixtures cover
// appears in the inventory. A category that no longer appears is a mapping the
// program lost, or a fixture it no longer reads.
func assertCategories(t *testing.T, source string, doc inventoryDoc) {
	t.Helper()

	got := make([]string, 0, len(doc.Resources))
	for _, resource := range doc.Resources {
		got = append(got, resource.Category)
	}

	slices.Sort(got)
	got = slices.Compact(got)

	want := append(taxonomyCategories(), "unclassified")
	slices.Sort(want)
	want = slices.Compact(want)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("%s: the inventory's categories do not cover the fixture (-want +got):\n%s", source, diff)
	}
}

// assertUnclassifiedKeepsRawType fails unless the inventory holds an
// unclassified resource with a rawType. An unclassified resource without one
// tells a reader what blocks the move even less than the category already does.
func assertUnclassifiedKeepsRawType(t *testing.T, source string, doc inventoryDoc) {
	t.Helper()

	var unclassified []string

	for _, resource := range doc.Resources {
		if resource.Category != "unclassified" {
			continue
		}

		if resource.RawType == "" {
			t.Errorf("%s: unclassified resource %q has no rawType", source, resource.ID)

			continue
		}

		unclassified = append(unclassified, resource.ID)
	}

	if len(unclassified) == 0 {
		t.Errorf("%s: the inventory holds no unclassified resource with a rawType, want at least one", source)
	}
}

// assertShortIDs fails when a resource ID still carries an ARN, an Azure
// resource ID or a GCP resource name: those hold the account, subscription or
// project, which must not reach an inventory or a report.
func assertShortIDs(t *testing.T, source string, doc inventoryDoc) {
	t.Helper()

	long := []string{"arn:", "/subscriptions/", "projects/"}

	for _, resource := range doc.Resources {
		for _, marker := range long {
			if strings.Contains(resource.ID, marker) {
				t.Errorf("%s: resource id %q holds %q; the guide's programs write short ids only",
					source, resource.ID, marker)
			}
		}
	}
}

// assertAssessAccepts runs the built binary on the inventory the program wrote,
// against every target, and fails unless it exits 0 with nothing on stderr. The
// output goes to a temporary directory, so no fixture is written by a test.
func assertAssessAccepts(t *testing.T, tc inventoryGuideCase, document []byte) {
	t.Helper()

	inventory := filepath.Join(t.TempDir(), tc.golden)
	if err := os.WriteFile(inventory, document, 0o600); err != nil {
		t.Fatalf("write %s: %v", inventory, err)
	}

	got := assess(t, allKey, inventory)

	if got.code != 0 {
		t.Fatalf("shift2eu assess --target %s on the %s inventory exited %d, want 0; stderr = %q",
			allKey, tc.source, got.code, got.stderr)
	}

	if got.stderr != "" {
		t.Errorf("assess wrote to stderr, want nothing: %q", got.stderr)
	}

	// The report names the source the program wrote, so a document that
	// decodes but says nothing usable still fails here.
	want := "- Source: `" + tc.source + "`"

	if !strings.Contains(got.stdout, want) {
		t.Errorf("the report does not hold %q:\n%s", want, got.stdout)
	}
}

package apiparity_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/report/jsonreport"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/report/markdown"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/request/assessmentrequest"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/inventoryfile"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/ovhcloud"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/scaleway"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/upcloud"
	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/cli"
	"github.com/timothymamo/eu-sov-cloud/internal/httpapi"
	"github.com/timothymamo/eu-sov-cloud/internal/testutil"
)

// The two sanitised inventories the parity cases read: one with several
// categories and fits, and the hostile one the report tests also use.
const (
	inventoryFixture = "../e2e/testdata/inventory.json"
	hostileFixture   = "../e2e/testdata/inventory-hostile.json"
)

// The report schemas jsonreport publishes, relative to this package.
const (
	assessmentSchemaPath = "../../internal/adapter/report/jsonreport/schema/assessment-v1.schema.json"
	comparisonSchemaPath = "../../internal/adapter/report/jsonreport/schema/comparison-v1.schema.json"
)

// The paths, formats and inventory name both sides share.
const (
	assessmentsPath = "/v1/assessments"
	jsonFormat      = "json"
	markdownFormat  = "markdown"
	// inventoryName is the name both reports must carry: the API names every
	// body inventory.json because a body has no file name, so the CLI is given a
	// file of that name.
	inventoryName = "inventory.json"
)

// parityCase is one inventory, target list and format both driving adapters
// render.
type parityCase struct {
	name    string
	fixture string
	// targets is the --target keys and the API's targets, in order; "all" alone
	// selects every target on both sides.
	targets []string
	// format is the CLI's --format value and accept the API's Accept header.
	format string
	accept string
}

// TestTheAPIAndTheCLIAgreeOnTheReport pins the parity PRD #103 promises: for the
// same inventory and targets, POST /v1/assessments answers the bytes
// `eusov assess` writes, for the JSON report and the Markdown report, for one
// target, a list and every target, and for a hostile inventory too. Both sides
// are wired with the real adapters, as their composition roots wire them, so a
// difference in decoding, target rules, ordering or rendering shows up as a
// difference in the bytes.
func TestTheAPIAndTheCLIAgreeOnTheReport(t *testing.T) {
	t.Parallel()

	schemas := reportSchemas(t)

	tests := []parityCase{
		{
			name:    "one target, JSON",
			fixture: inventoryFixture,
			targets: []string{"scaleway"},
			format:  jsonFormat,
			accept:  "application/json",
		},
		{
			name:    "a list of targets, JSON",
			fixture: inventoryFixture,
			targets: []string{"ovhcloud", "upcloud"},
			format:  jsonFormat,
			accept:  "application/json",
		},
		{
			name:    "every target, JSON",
			fixture: inventoryFixture,
			targets: []string{"all"},
			format:  jsonFormat,
			accept:  "application/json",
		},
		{
			name:    "one target, Markdown",
			fixture: inventoryFixture,
			targets: []string{"scaleway"},
			format:  markdownFormat,
			accept:  "text/markdown",
		},
		{
			name:    "every target, Markdown",
			fixture: inventoryFixture,
			targets: []string{"all"},
			format:  markdownFormat,
			accept:  "text/markdown",
		},
		{
			name:    "a hostile inventory, JSON",
			fixture: hostileFixture,
			targets: []string{"scaleway"},
			format:  jsonFormat,
			accept:  "application/json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			document := readFixture(t, tc.fixture)
			inventoryPath := writeInventory(t, document)

			cliReport := runAssess(t, inventoryPath, tc)
			apiReport := postAssessment(t, document, tc.targets, tc.accept)

			if !bytes.Equal(cliReport, apiReport) {
				t.Errorf("the API's body differs from `eusov assess --target %s --format %s` (-CLI +API):\n%s",
					strings.Join(tc.targets, ","), tc.format, cmp.Diff(string(cliReport), string(apiReport)))
			}

			if tc.format == jsonFormat {
				validateReport(t, schemas, apiReport)
			}
		})
	}
}

// runAssess runs the real CLI over inventoryPath and returns its report, failing
// the test when the run does not succeed cleanly.
func runAssess(t *testing.T, inventoryPath string, tc parityCase) []byte {
	t.Helper()

	args := []string{
		"assess",
		"--inventory", inventoryPath,
		"--target", strings.Join(tc.targets, ","),
		"--format", tc.format,
	}

	var stdout, stderr bytes.Buffer

	if code := cli.Run(t.Context(), args, &stdout, &stderr, cliDeps()); code != 0 {
		t.Fatalf("eusov %s = exit %d, want 0; stderr: %q", strings.Join(args, " "), code, stderr.String())
	}

	if stderr.Len() != 0 {
		t.Errorf("eusov %s wrote to stderr on success: %q", strings.Join(args, " "), stderr.String())
	}

	return stdout.Bytes()
}

// postAssessment posts document and targets to the real API handler and returns
// its report body, failing the test when the request is not answered with 200.
func postAssessment(t *testing.T, document []byte, targets []string, accept string) []byte {
	t.Helper()

	body, err := json.Marshal(struct {
		Inventory json.RawMessage `json:"inventory"`
		Targets   []string        `json:"targets"`
	}{Inventory: document, Targets: targets})
	if err != nil {
		t.Fatalf("encoding the request body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, assessmentsPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	rec := httptest.NewRecorder()
	apiHandler(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s = %d, want %d; body: %q",
			assessmentsPath, rec.Code, http.StatusOK, rec.Body.String())
	}

	return rec.Body.Bytes()
}

// writeInventory copies document to a fresh temporary directory as
// inventory.json and returns its path, so the CLI reports the name the API
// reports.
func writeInventory(t *testing.T, document []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), inventoryName)

	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatalf("writing the inventory %s: %v", path, err)
	}

	return path
}

// readFixture returns the bytes of a fixture file.
func readFixture(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the fixture %s: %v", path, err)
	}

	return data
}

// reportSchemas parses jsonreport's two schemas, keyed by the document kind each
// describes.
func reportSchemas(t *testing.T) map[string]*testutil.Schema {
	t.Helper()

	files := []struct {
		kind string
		path string
	}{
		{kind: "assessment", path: assessmentSchemaPath},
		{kind: "comparison", path: comparisonSchemaPath},
	}

	schemas := make(map[string]*testutil.Schema, len(files))

	for _, file := range files {
		_, schema, err := testutil.ParseSchema(readFixture(t, file.path))
		if err != nil {
			t.Fatalf("parsing the %s schema %s: %v", file.kind, file.path, err)
		}

		schemas[file.kind] = schema
	}

	return schemas
}

// validateReport holds a JSON report to the schema of the kind it declares.
func validateReport(t *testing.T, schemas map[string]*testutil.Schema, report []byte) {
	t.Helper()

	var envelope struct {
		Kind string `json:"kind"`
	}

	if err := json.Unmarshal(report, &envelope); err != nil {
		t.Fatalf("decoding the report's kind: %v; body: %s", err, report)
	}

	schema, ok := schemas[envelope.Kind]
	if !ok {
		t.Fatalf("the report's kind %q is neither assessment nor comparison", envelope.Kind)
	}

	schema.ValidateJSON(t, report)
}

// cliDeps wires the assess path of the CLI as cmd/eusov does: the inventory
// file, the embedded OVHcloud, Scaleway and UpCloud catalogs and the two report
// formats, each rendering a single report and a comparison.
func cliDeps() cli.Deps {
	return cli.Deps{
		NewInventorySource: func(path string) analysis.InventorySource {
			return inventoryfile.New(path)
		},
		Targets: targetSources(),
		Formats: map[string]cli.Format{
			jsonFormat: {
				NewReporter:           func() analysis.Reporter { return jsonreport.New() },
				NewComparisonReporter: func() analysis.ComparisonReporter { return jsonreport.New() },
			},
			markdownFormat: {
				NewReporter:           func() analysis.Reporter { return markdown.New() },
				NewComparisonReporter: func() analysis.ComparisonReporter { return markdown.New() },
			},
		},
	}
}

// apiHandler wires the HTTP API as its composition root does: the strict request
// decoder, the in-memory inventory source, the same targets and reporters as the
// CLI, and the body cap assessmentrequest declares.
func apiHandler(t *testing.T) http.Handler {
	t.Helper()

	handler, err := httpapi.New(httpapi.Deps{
		DecodeRequest: assessmentrequest.Decode,
		NewInventorySource: func(document []byte) analysis.InventorySource {
			return inventoryfile.FromBytes(document)
		},
		Targets: targetSources(),
		JSON: httpapi.Format{
			NewReporter:           func() analysis.Reporter { return jsonreport.New() },
			NewComparisonReporter: func() analysis.ComparisonReporter { return jsonreport.New() },
		},
		Markdown: httpapi.Format{
			NewReporter:           func() analysis.Reporter { return markdown.New() },
			NewComparisonReporter: func() analysis.ComparisonReporter { return markdown.New() },
		},
	}, httpapi.WithMaxBodyBytes(assessmentrequest.MaxBodySize))
	if err != nil {
		t.Fatalf("httpapi.New() returned unexpected error: %v", err)
	}

	return handler
}

// targetSources wires the embedded target catalogs, as both composition roots
// do.
func targetSources() map[string]func() analysis.CatalogSource {
	return map[string]func() analysis.CatalogSource{
		"ovhcloud": func() analysis.CatalogSource { return ovhcloud.New() },
		"scaleway": func() analysis.CatalogSource { return scaleway.New() },
		"upcloud":  func() analysis.CatalogSource { return upcloud.New() },
	}
}

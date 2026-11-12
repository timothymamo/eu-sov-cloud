package openapibundle_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/test/internal/openapibundle"
)

// update rewrites the committed bundle instead of comparing with it:
// go test ./test/openapibundle -update.
var update = flag.Bool("update", false, "rewrite api/openapi.bundled.json")

// repoRoot is the module root, as the package directory go test runs in is
// test/openapibundle.
const repoRoot = "../.."

// The document the bundle is generated from, and the bundle every release
// publishes as shift2eu-server_openapi.json (issue #139), both relative to the
// root.
const (
	source  = "api/openapi.json"
	bundled = "api/openapi.bundled.json"
)

// updateCommand is what a stale or missing bundle tells the reader to run.
const updateCommand = "go test ./test/openapibundle -update"

// reportSchemas are the schemas api/openapi.json refers to by a
// repository-relative path, and the name each one must have in the bundle:
// the base name of the file, its kebab parts title-cased, Report inserted
// before the version.
var reportSchemas = map[string]string{
	"AssessmentReportV1": "assessment-v1.schema.json",
	"ComparisonReportV1": "comparison-v1.schema.json",
}

// TestBundleIsCommitted pins the self-contained OpenAPI document of issue
// #139: api/openapi.bundled.json is exactly what openapibundle.Bundle makes of
// api/openapi.json with the report schemas, so it cannot drift from the
// document or the schemas it is generated from. A missing or stale file fails
// with the command that writes it.
func TestBundleIsCommitted(t *testing.T) {
	// Not parallel: -update writes the file the other test reads.
	got, err := openapibundle.Bundle(os.DirFS(repoRoot), source)
	if err != nil {
		t.Fatalf("bundle %s: %v", source, err)
	}

	path := filepath.Join(repoRoot, filepath.FromSlash(bundled))

	if *update {
		// 0o600: the golden file internal/testutil/golden.go writes as well; git
		// stores the mode as 644 anyway.
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("write %s: %v", bundled, err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run %s to write it)", bundled, err, updateCommand)
	}

	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Errorf("%s is not %s bundled: run %s and commit the result (-want +got):\n%s",
			bundled, source, updateCommand, diff)
	}
}

// TestBundledDocumentIsSelfContained pins what the committed bundle is for: it
// resolves outside the repository. Every $ref it holds is a pointer into the
// document itself, #/..., that resolves as a JSON pointer, so no ref is
// repository-relative (../...) any more, and components.schemas holds the two
// report schemas the published document must carry.
func TestBundledDocumentIsSelfContained(t *testing.T) {
	t.Parallel()

	doc := readBundled(t)

	found := refs(doc)
	if len(found) == 0 {
		t.Fatalf("%s holds no $ref, want the document's refs", bundled)
	}

	for _, ref := range found {
		if strings.Contains(ref, "../") {
			t.Errorf("%s: the ref %q is relative to the repository, want a pointer into the document: the bundle is published on its own", bundled, ref)

			continue
		}

		if !strings.HasPrefix(ref, "#/") {
			t.Errorf("%s: the ref %q is not a pointer into the document, want one starting with #/", bundled, ref)

			continue
		}

		if _, err := resolve(doc, ref); err != nil {
			t.Errorf("%s: the ref %q does not resolve: %v", bundled, ref, err)
		}
	}

	schemas := schemasOf(t, doc)

	for name, file := range reportSchemas {
		if _, ok := schemas[name]; !ok {
			t.Errorf("%s: components.schemas holds no %s, want the %s schema, the one api/openapi.json refers to by a path", bundled, name, file)
		}
	}
}

// readBundled returns the committed bundle, decoded as one JSON document.
func readBundled(t *testing.T) any {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(bundled)))
	if err != nil {
		t.Fatalf("read %s: %v (run %s to write it)", bundled, err, updateCommand)
	}

	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s is not one JSON document: %v (run %s to write it)", bundled, err, updateCommand)
	}

	return doc
}

// schemasOf returns components.schemas of doc.
func schemasOf(t *testing.T, doc any) map[string]any {
	t.Helper()

	root, ok := doc.(map[string]any)
	if !ok {
		t.Fatalf("%s is not a JSON object", bundled)
	}

	components, ok := root["components"].(map[string]any)
	if !ok {
		t.Fatalf("%s holds no components object", bundled)
	}

	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatalf("%s holds no components.schemas object", bundled)
	}

	return schemas
}

// refs returns every $ref value doc holds, in a stable order: the keys of an
// object are walked sorted, so a failure names the same ref every run.
func refs(doc any) []string {
	var found []string

	var walk func(v any)

	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}

			sort.Strings(keys)

			for _, key := range keys {
				if key == "$ref" {
					if ref, ok := v[key].(string); ok {
						found = append(found, ref)
					}

					continue
				}

				walk(v[key])
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}

	walk(doc)

	return found
}

// resolve returns the value of doc that the JSON pointer ref names, and an
// error when it names none: every step is an object key or an array index, and
// ~1 and ~0 stand for / and ~.
func resolve(doc any, ref string) (any, error) {
	cur := doc

	for _, step := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		step = strings.ReplaceAll(strings.ReplaceAll(step, "~1", "/"), "~0", "~")

		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[step]
			if !ok {
				return nil, fmt.Errorf("no key %q", step)
			}

			cur = next
		case []any:
			i, err := strconv.Atoi(step)
			if err != nil || i < 0 || i >= len(node) {
				return nil, fmt.Errorf("no entry %q, want one of the %d the array holds", step, len(node))
			}

			cur = node[i]
		default:
			return nil, fmt.Errorf("%q is below a %T", step, cur)
		}
	}

	return cur, nil
}

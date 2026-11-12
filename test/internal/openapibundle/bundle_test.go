package openapibundle_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/test/internal/openapibundle"
)

// TestBundle pins the bundling of an OpenAPI document into one self-contained
// document (issue #139). openapibundle.Bundle reads the document at path from
// fsys, a file system rooted at the repository, and adds every schema that a
// $ref outside the document points at to components.schemas, as the case
// below pins:
//
//   - a $ref that is not a local #... pointer is replaced by
//     #/components/schemas/<Name>, and the file it names is added under
//     components.schemas.<Name>. <Name> is the file's base name with the
//     kebab parts title-cased, Report inserted before the version and the
//     -vN suffix written VN: assessment-v1.schema.json is
//     AssessmentReportV1, as is widget-v1.schema.json WidgetReportV1;
//   - the refs inside an added schema that point into itself (#/$defs/X and #)
//     are rewritten to absolute pointers in the bundle
//     (#/components/schemas/<Name>/$defs/X and #/components/schemas/<Name>), so
//     two schemas that both define $defs.count do not collide, and the
//     schema's $id and $schema are removed, because an $id rebases exactly
//     those pointers. Everything else of the schema is kept;
//   - a file the document refers to twice is added once;
//   - the document's own local pointers (#/components/responses/Error) and the
//     components it already holds are left as they are.
//
// A document it cannot bundle is refused, naming what it could not read: a
// ref to a file fsys does not hold, a ref that escapes the root (../../x), a
// schema whose <Name> the document already has in components.schemas, and
// invalid JSON.
//
// The fixture tree under testdata is bundled whole; the single rules are
// pinned one at a time on a minimal document held in the case, which is a
// fstest.MapFS rather than a file, because each one is one line of JSON.
func TestBundle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fsys fs.FS
		path string
		// want is the bundle the case expects, as JSON text. It is decoded
		// before it is compared with the decoded bundle, so the order and the
		// indentation of the JSON here mean nothing: TestBundleWritesOneCanonicalDocument
		// pins the output's form.
		want string
		// wantErr says whether the document is refused, and wantErrIn a part
		// of the message that must name what could not be bundled; "" accepts
		// any message.
		wantErr   bool
		wantErrIn string
	}{
		{
			name: "the fixture tree is bundled whole",
			fsys: os.DirFS("testdata"),
			path: "api/openapi.json",
			want: bundledFixture,
		},

		{
			name: "a ref to a file the root does not hold is refused",
			fsys: document(`{
				"schemas": {"widget": {"$ref": "../schemas/widget-v1.schema.json"}}
			}`),
			path:      "api/openapi.json",
			wantErr:   true,
			wantErrIn: "widget-v1.schema.json",
		},
		{
			name: "a ref that escapes the root is refused",
			fsys: document(`{
				"schemas": {"widget": {"$ref": "../../x.schema.json"}}
			}`),
			path:      "api/openapi.json",
			wantErr:   true,
			wantErrIn: "../x.schema.json",
		},
		{
			name: "a name components.schemas already holds is refused",
			fsys: fstest.MapFS{
				"api/openapi.json": file(`{
					"schemas": {"widget": {"$ref": "../schemas/widget-v1.schema.json"}},
					"components": {"schemas": {"WidgetReportV1": {"type": "object"}}}
				}`),
				"schemas/widget-v1.schema.json": file(`{"type": "object"}`),
			},
			path:      "api/openapi.json",
			wantErr:   true,
			wantErrIn: "WidgetReportV1",
		},
		{
			name:    "a document that is not JSON is refused",
			fsys:    document(`{"openapi": "3.1.0",`),
			path:    "api/openapi.json",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := openapibundle.Bundle(tc.fsys, tc.path)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("Bundle(%q) = %s, want an error", tc.path, got)
				}

				if tc.wantErrIn != "" && !strings.Contains(err.Error(), tc.wantErrIn) {
					t.Fatalf("Bundle(%q) error = %q, want a message containing %q", tc.path, err, tc.wantErrIn)
				}

				return
			}

			if err != nil {
				t.Fatalf("Bundle(%q): %v", tc.path, err)
			}

			if diff := cmp.Diff(decode(t, tc.want), decode(t, string(got))); diff != "" {
				t.Errorf("Bundle(%q) (-want +got):\n%s", tc.path, diff)
			}
		})
	}
}

// TestBundleWritesOneCanonicalDocument pins the form of the bundle, so that the
// document committed under api/ does not change with the map iteration order or
// a formatter: Bundle writes 2-space indented JSON with its keys sorted and one
// trailing newline, and two calls on the same document give the same bytes.
func TestBundleWritesOneCanonicalDocument(t *testing.T) {
	t.Parallel()

	doc := os.DirFS("testdata")

	got, err := openapibundle.Bundle(doc, "api/openapi.json")
	if err != nil {
		t.Fatalf("Bundle(%q): %v", "api/openapi.json", err)
	}

	// The output is the canonical one: what the standard library writes for
	// the document the output itself holds, every object with its keys
	// sorted, indented by 2 spaces, plus the one newline a text file ends
	// with.
	canonical, err := json.MarshalIndent(decode(t, string(got)), "", "  ")
	if err != nil {
		t.Fatalf("write the bundle's document back: %v", err)
	}

	if diff := cmp.Diff(string(canonical)+"\n", string(got)); diff != "" {
		t.Errorf("Bundle(%q) is not 2-space indented JSON with sorted keys and one trailing newline (-want +got):\n%s", "api/openapi.json", diff)
	}

	again, err := openapibundle.Bundle(doc, "api/openapi.json")
	if err != nil {
		t.Fatalf("Bundle(%q) a second time: %v", "api/openapi.json", err)
	}

	if diff := cmp.Diff(string(got), string(again)); diff != "" {
		t.Errorf("Bundle(%q) is not deterministic (-first +second):\n%s", "api/openapi.json", diff)
	}
}

// bundledFixture is what testdata/api/openapi.json must be bundled into:
// components.responses.Error, components.schemas.Error and the document's own
// local pointers stay as they are, the two schemas the document refers to are
// added once each with the $schema and $id they carry removed, and every ref
// into them, a self ref included, is an absolute pointer into the bundle.
const bundledFixture = `
{
  "openapi": "3.1.0",
  "info": {
    "title": "Widgets",
    "version": "1"
  },
  "paths": {
    "/gadgets": {
      "get": {
        "responses": {
          "200": {
            "description": "A gadget.",
            "content": {
              "application/json": {
                "schema": {"$ref": "#/components/schemas/GadgetReportV1"}
              }
            }
          }
        }
      }
    },
    "/widgets": {
      "get": {
        "responses": {
          "200": {
            "description": "A widget.",
            "content": {
              "application/json": {
                "schema": {"$ref": "#/components/schemas/WidgetReportV1"}
              }
            }
          },
          "default": {"$ref": "#/components/responses/Error"}
        }
      }
    },
    "/widgets/count": {
      "get": {
        "responses": {
          "200": {
            "description": "How many widgets.",
            "content": {
              "application/json": {
                "schema": {"$ref": "#/components/schemas/WidgetReportV1"}
              }
            }
          }
        }
      }
    }
  },
  "components": {
    "responses": {
      "Error": {
        "description": "An error."
      }
    },
    "schemas": {
      "Error": {
        "type": "object",
        "properties": {
          "code": {
            "type": "string"
          }
        }
      },
      "GadgetReportV1": {
        "title": "The gadget.",
        "type": "object",
        "properties": {
          "gadget": {"$ref": "#/components/schemas/GadgetReportV1"},
          "count": {"$ref": "#/components/schemas/GadgetReportV1/$defs/count"}
        },
        "$defs": {
          "count": {
            "type": "string"
          }
        }
      },
      "WidgetReportV1": {
        "title": "The widget.",
        "type": "object",
        "properties": {
          "widget": {"$ref": "#/components/schemas/WidgetReportV1/$defs/widget"},
          "count": {"$ref": "#/components/schemas/WidgetReportV1/$defs/count"}
        },
        "$defs": {
          "widget": {
            "type": "string"
          },
          "count": {
            "type": "integer"
          }
        }
      }
    }
  }
}
`

// document returns a file system holding the OpenAPI document doc at
// api/openapi.json and nothing else.
func document(doc string) fs.FS {
	return fstest.MapFS{"api/openapi.json": file(doc)}
}

// file returns doc as a MapFS file.
func file(doc string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(doc)}
}

// decode reads data as one JSON document, keeping every number as it is
// written, so that a comparison of two documents does not depend on how
// either was indented or ordered.
func decode(t *testing.T, data string) any {
	t.Helper()

	dec := json.NewDecoder(strings.NewReader(data))
	dec.UseNumber()

	var doc any
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("decode a JSON document: %v", err)
	}

	return doc
}

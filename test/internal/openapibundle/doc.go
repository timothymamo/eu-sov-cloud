// Package openapibundle bundles an OpenAPI document and the JSON Schemas it
// refers to by a path into one self-contained document (issue #139, ADR 0019).
//
// api/openapi.json refers to the report schemas under
// internal/adapter/report/jsonreport/schema by repository-relative $ref, so it
// resolves only inside the repository. Bundle reads the document at a path of
// a file system rooted at the repository and replaces every $ref that is not a
// local #... pointer by #/components/schemas/<Name>, resolving the ref against
// the document's directory and refusing one that leaves the root. The file is
// added once under components.schemas.<Name>: <Name> is its base name with the
// kebab parts title-cased, Report inserted before the version and the -vN
// suffix written VN (assessment-v1.schema.json is AssessmentReportV1). The
// added schema loses its $id and $schema, because an $id rebases its own
// pointers, and those pointers (#, #/$defs/X) are rewritten to absolute ones
// into the bundle (#/components/schemas/<Name>/$defs/X), so two schemas with
// the same $defs do not collide. The document's own pointers and components
// stay as they are. A missing or escaping file, a <Name> the document already
// holds and invalid JSON are refused, naming the file, ref or name.
//
// The output is canonical, so the committed bundle does not change with map
// order: 2-space indented JSON with sorted keys and one trailing newline.
//
// test/openapibundle pins api/openapi.bundled.json, the bundle every release
// publishes as shift2eu-server_openapi.json, to what Bundle makes of
// api/openapi.json (go test ./test/openapibundle -update, make openapi, writes
// it). It uses the standard library only.
//
// The package is internal to test: it is not an API of the module.
package openapibundle

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
// stay as they are. Every kebab part but the version must match
// ^[a-z][a-z0-9]*$, so <Name> is a valid OpenAPI component key and holds no ~
// or / a JSON pointer would have to escape. A missing or escaping file, a
// base name with any other kebab part, a <Name> the document already holds and
// invalid JSON are refused, naming the file, ref or name.
//
// Every string-valued $ref key anywhere in the document and the schemas is
// treated as a reference and rewritten, wherever it sits: also one inside an
// example, default or const value, which is data rather than a reference
// (api/openapi.json and the report schemas hold none today).
//
// The output is canonical, so the committed bundle does not change with map
// order: 2-space indented JSON with sorted keys and one trailing newline. It
// escapes no HTML: <, > and & stay as they are written (Bearer <token>), not
// \u003c, \u003e and \u0026, as a published document is no HTML.
//
// Path names the committed bundle, api/openapi.bundled.json: test/openapibundle
// writes it, and the release's .goreleaser.yaml glob, check.BundleGlob in
// test/architecture, is "./" + Path, so both name the same file.
//
// test/openapibundle pins api/openapi.bundled.json, the bundle every release
// publishes as shift2eu-server_openapi.json, to what Bundle makes of
// api/openapi.json (go test ./test/openapibundle -update, make openapi, writes
// it). It uses the standard library only.
//
// The package is internal to test: it is not an API of the module.
package openapibundle

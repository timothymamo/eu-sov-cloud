// Package testutil holds helpers shared by tests: golden-file comparison
// (with an -update flag), fixture loading from testdata, small builders for
// domain values and a JSON Schema validator for the tests that hold a document
// to a schema. It must only be imported from _test.go files.
//
// The application layer's tests may import it, so the golden files it reads
// and writes are the only I/O those tests reach. Its imports, its own tests'
// included, are a strict allow list (depguard rule testutil-imports in
// .golangci.yml): a new import needs an entry there and in test/architecture's
// pin. It reads no environment variable and starts no process (forbidigo
// group testutil-no-env, CLAUDE.md rule 3): a helper takes what it needs as a
// parameter.
package testutil

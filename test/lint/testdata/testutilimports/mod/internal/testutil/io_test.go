// The rule names the directory whole and no !$test, so it holds the helpers'
// _test.go files too (issue #144): a test of testutil is compiled into the same
// package and runs with the same privileges, so it may not import net/http
// either. The import is used, and nothing here runs.
package testutil

import (
	"net/http" // want "import 'net/http' is not allowed from list 'testutil-imports'"
	"testing"
)

var _ = http.Get

// TestFake keeps the package's own test file and its refused import used.
func TestFake(t *testing.T) {
	t.Parallel()

	Fake()
}

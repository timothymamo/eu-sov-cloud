package skip

import (
	"net/http"
	"testing"
)

// TestMethod pins nothing: the file exists to import net/http from a test file
// of an excluded directory, which the rule that holds a/source does not hold.
func TestMethod(t *testing.T) {
	t.Parallel()

	if http.MethodGet == "" {
		t.Fatal("net/http names no GET method")
	}
}

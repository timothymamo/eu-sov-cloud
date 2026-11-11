package xprobe_test

import (
	"net/http"
	"testing"
)

// TestMethod pins nothing: the file exists to import net/http from an external
// test package, which the rule that holds this directory refuses.
func TestMethod(t *testing.T) {
	t.Parallel()

	if http.MethodGet == "" {
		t.Fatal("net/http names no GET method")
	}
}

package version_test

import (
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/internal/version"
)

func TestString(t *testing.T) {
	t.Parallel()

	got := version.String()

	if !strings.HasPrefix(got, "shift2eu ") {
		t.Errorf("String() = %q, want prefix %q", got, "shift2eu ")
	}
}

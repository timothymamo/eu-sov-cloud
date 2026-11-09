package version_test

import (
	"strings"
	"testing"

	"github.com/timothymamo/eu-sov-cloud/internal/version"
)

func TestString(t *testing.T) {
	t.Parallel()

	got := version.String()

	if !strings.HasPrefix(got, "eusov ") {
		t.Errorf("String() = %q, want prefix %q", got, "eusov ")
	}
}

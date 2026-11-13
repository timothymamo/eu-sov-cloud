// The e2e tests build and run the binary, so os/exec stays theirs (issue #185):
// depguard's e2e-is-black-box holds them, and !$test takes them out of
// test-docs-are-pure, the strict allow list that holds the tier's production
// files. This import carries no want comment, so a rule that reached the test
// files, or a files: list that stopped excluding $test, fails the probe.
package e2e

import (
	"os/exec"
	"testing"
)

func TestRunsTheBuiltBinary(t *testing.T) {
	t.Parallel()

	if _, err := exec.Command("shift2eu", "assess").Output(); err != nil {
		t.Fatal(err)
	}
}

// A helper reaches no network and starts no program. Both imports are refused by
// testutil-imports (issue #144, the auditor's finding M-1 and the reviewer's S1):
// an application layer's test may import internal/testutil, and before the rule
// nothing bounded what a helper imported, so net/http and os/exec here would have
// given every test of the layer HTTP calls and a subprocess with lint green. The
// imports are used, so the file type-checks, and nothing here runs: the probe
// lints the file.
package testutil

import (
	"net/http" // want "import 'net/http' is not allowed from list 'testutil-imports'"
	"os/exec"  // want "import 'os/exec' is not allowed from list 'testutil-imports'"
)

var (
	_ = http.Get
	_ = exec.Command
)

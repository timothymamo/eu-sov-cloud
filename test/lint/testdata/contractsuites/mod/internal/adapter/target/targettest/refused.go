// The imports the target contract suite must not make (issue #152, the auditor's
// finding L-A): every _test.go file of a target adapter imports the suite, so a
// production file here that starts a program or reads the environment would hand
// every target adapter's test that I/O with lint green. The rule
// contract-suites-are-pure is a strict allow list that names none of them; each
// line below ends in a want comment.
package targettest

import (
	"os"      // want "import 'os' is not allowed"
	"os/exec" // want "import 'os/exec' is not allowed"
)

var (
	_ = exec.Command
	_ = os.Getenv
)

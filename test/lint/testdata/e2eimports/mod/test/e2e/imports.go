// The e2e tier's production files import nothing (issue #185). test/e2e/doc.go
// carries no build tag, so test/e2e is a package a composition root may import,
// and depguard's strict rule test-docs-are-pure holds every non-test file under it
// to an allow list of nothing: an untagged file there that imported os/exec,
// unsafe or os/user could run a program with the whole environment of the process
// that started it, take a pointer apart, or look up the home directory a
// credential file lives in, with lint green.
//
// Each import below ends in a want comment that names no rule, so the probe stays
// true if another rule refuses the import, and an allow list that named one of
// the three would fail it. The e2e tests, which run the binary, keep os/exec:
// runs_test.go imports it with no want comment.
package e2e

import (
	"os/exec" // want "import 'os/exec' is not allowed"
	"os/user" // want "import 'os/user' is not allowed"
	"unsafe"  // want "import 'unsafe' is not allowed"
)

// The refused packages are imported to be judged: one symbol of each keeps the
// import used, which the type checker requires.
var (
	_ = exec.Command
	_ = user.Current
	_ = unsafe.Sizeof(0)
)

// The imports the source contract suite must not make (issue #152, the auditor's
// finding L-A): any _test.go file of the module may import the suite, so a
// production file here that starts a program, reads the environment, opens a
// connection or takes a pointer apart would hand every adapter test that I/O with
// lint green. The rule contract-suites-are-pure is a strict allow list that names
// none of them; each line below ends in a want comment.
//
// The imported packages are used, which the type checker requires of every
// import.
package sourcetest

import (
	"net"     // want "import 'net' is not allowed"
	"os"      // want "import 'os' is not allowed"
	"os/exec" // want "import 'os/exec' is not allowed"
	"syscall" // want "import 'syscall' is not allowed"
	"unsafe"  // want "import 'unsafe' is not allowed"
)

var (
	_ = exec.Command
	_ = syscall.Exec
	_ = unsafe.Sizeof
	_ = os.Getenv
	_ net.Conn
)

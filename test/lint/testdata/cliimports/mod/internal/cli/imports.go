// The CLI's production code starts no program, makes no raw system call and
// takes no pointer apart (issue #152). Before the rule was a strict allow list,
// cli-gets-adapters-from-cmd allowed $gostd and then denied a list of packages
// one by one: os/exec, syscall and plugin were refused by a deny entry, and any
// standard package nobody thought to deny — unsafe among them — was allowed.
// The allow list names the packages the CLI imports today, so these three are
// refused because it names none of them.
//
// Each line that ends in a want comment is an import the rule must refuse. The
// want names no rule, so extending the rule, or adding a sibling rule with the
// same entries, keeps the probe true as long as the import is refused.
package cli

import (
	"os/exec" // want "import 'os/exec' is not allowed"
	"syscall" // want "import 'syscall' is not allowed"
	"unsafe"  // want "import 'unsafe' is not allowed"
)

// The refused packages are imported to be judged: one symbol of each keeps the
// import used, which the type checker requires.
var (
	_ = exec.Command
	_ = syscall.Exec
	_ = unsafe.Sizeof
)

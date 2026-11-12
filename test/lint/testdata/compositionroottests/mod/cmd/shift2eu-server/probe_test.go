// The server composition root's _test.go file, for
// TestCompositionRootTestsReachNoForeignCodeProbe: the same refused imports as
// the CLI's, because both roots' tests run in the server's process and a rule
// that held one root's tests and not the other's would leave the server open.
// The imports are used, so the file type-checks, and no line does anything at
// run time: the probe lints the file and never runs it.
package main

import (
	"go/build"     // want "import 'go/build' is not allowed from list 'composition-root-tests'"
	"net/http/cgi" // want "import 'net/http/cgi' is not allowed from list 'composition-root-tests'"
	"os/exec"      // want "import 'os/exec' is not allowed from list 'composition-root-tests'"
	"os/user" // want "import 'os/user' is not allowed from list 'composition-root-tests'"
	"plugin"       // want "import 'plugin' is not allowed from list 'composition-root-tests'"
	"runtime/cgo"  // want "import 'runtime/cgo' is not allowed from list 'composition-root-tests'"
	"testing"
	"unsafe" // want "import 'unsafe' is not allowed from list 'composition-root-tests'"
)

var (
	_ = build.Default
	_ = cgi.Handler{}
	_ = exec.CommandContext
	_ = user.Current
	_ = plugin.Open
	_ = cgo.NewHandle
	_ = unsafe.Sizeof(0)
)

// TestProbe keeps the refused imports used without calling any of them.
func TestProbe(t *testing.T) {
	t.Parallel()
}

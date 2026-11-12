// The driving adapter's _test.go file for
// TestCompositionRootTestsReachNoForeignCodeProbe (issue #152): the tests of
// internal/cli are held to the limits the composition roots' tests are, because
// a test of the CLI runs in the same process as the CLI and a root delegates to
// it. Each import below is one the composition-root-tests rule refuses for the
// roots' tests, and each stands for a way a test can run code the CLI never
// runs: start a program (os/exec), hand a path to a CGI handler (net/http/cgi),
// load a plugin (plugin), read a tree (go/build), call C (runtime/cgo), look a
// user up in the OS database (os/user, issue #152), or reach any package's symbol
// through unsafe. The imports are used, so the file
// type-checks, and no line does anything at run time: the probe lints the file
// and never runs it.
//
// Each want names no rule, so extending composition-root-tests to these files
// and giving them a sibling rule with the same entries both pass.
package cli_test

import (
	"go/build"     // want "import 'go/build' is not allowed"
	"net/http/cgi" // want "import 'net/http/cgi' is not allowed"
	"os/exec"      // want "import 'os/exec' is not allowed"
	"os/user" // want "import 'os/user' is not allowed"
	"plugin"       // want "import 'plugin' is not allowed"
	"runtime/cgo"  // want "import 'runtime/cgo' is not allowed"
	"testing"
	"unsafe" // want "import 'unsafe' is not allowed"
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

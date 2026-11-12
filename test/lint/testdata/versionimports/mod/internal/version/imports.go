// The build metadata imports the standard packages it needs to format the build
// line, and nothing else (issue #152, the security audit's M-3). Before the rule
// version-is-build-metadata, both driving adapters allowed internal/version and
// no rule held what it imports: a composition root that may not read the
// environment or start a process could hand either to this package, and the call
// happened one import deeper with lint green. os and os/exec are refused because
// the allow list names fmt alone.
//
// Each line that ends in a want comment is an import the rule must refuse. The
// want names no rule, so extending the rule, or adding a sibling rule with the
// same entries, keeps the probe true as long as the import is refused.
package version

import (
	"os"      // want "import 'os' is not allowed"
	"os/exec" // want "import 'os/exec' is not allowed"
)

// The refused packages are imported to be judged: one symbol of each keeps the
// import used, which the type checker requires.
var (
	_ = os.Getpagesize
	_ = exec.Command
)

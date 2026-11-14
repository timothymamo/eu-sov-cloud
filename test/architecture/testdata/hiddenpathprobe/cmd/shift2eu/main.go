// Command shift2eu stands in for a composition root that imports packages the
// go command skips when it walks ./...: one under a testdata directory, one
// under a directory whose name starts with _ and one under a directory whose
// name starts with . go list -deps of the root lists all three, so no depguard
// rule and no architecture check that walks the tree sees them (issue #206),
// and the probe exists so test/architecture requires the hidden-path check to
// report them.
package main

import (
	dot "example.com/hiddenpathprobe/.x"
	underscore "example.com/hiddenpathprobe/_x"
	testdata "example.com/hiddenpathprobe/testdata/x"
)

func main() {
	_ = dot.Name() + underscore.Name() + testdata.Name()
}

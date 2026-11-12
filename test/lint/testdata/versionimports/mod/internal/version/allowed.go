// The package the build metadata imports today, which version-is-build-metadata
// must keep allowing (issue #152): fmt formats the build line, and the three
// variables the build sets through -X are no import at all. A strict allow list
// says what internal/version may import, so an entry lost in a rewrite fails
// here: this file carries no want comment.
package version

import (
	_ "fmt"
)

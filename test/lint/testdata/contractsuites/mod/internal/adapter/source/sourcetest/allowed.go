// The packages the source contract suite imports today, which depguard's
// contract-suites-are-pure rule must keep allowing (issue #152, the auditor's
// finding L-A): the suite passes an analysis.InventorySource to an adapter's
// Inventory and compares the inventory it returns. No line here carries a want
// comment, so an allow entry lost in the rewrite fails the probe.
//
// The module's packages are named one by one in the rule, each ending in $, for
// the reason given at domain-is-pure in .golangci.yml: a prefix entry would also
// admit a package under testdata or a _ directory, which make lint never visits.
package sourcetest

import (
	_ "context"
	_ "errors"
	_ "fmt"
	_ "slices"
	_ "testing"

	_ "github.com/timothymamo/shift2eu/internal/app/analysis"
	_ "github.com/timothymamo/shift2eu/internal/domain/inventory"
	_ "github.com/timothymamo/shift2eu/internal/domain/text"
)

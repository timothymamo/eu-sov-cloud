// The packages the target contract suite imports today, which depguard's
// contract-suites-are-pure rule must keep allowing (issue #152, the auditor's
// finding L-A): the suite passes an analysis.Target to a target adapter's Catalog
// and compares the catalog it returns, and it takes a context with a timeout. No
// line here carries a want comment, so an allow entry lost in the rewrite fails
// the probe.
package targettest

import (
	_ "context"
	_ "errors"
	_ "fmt"
	_ "slices"
	_ "testing"
	_ "time"

	_ "github.com/timothymamo/shift2eu/internal/app/analysis"
	_ "github.com/timothymamo/shift2eu/internal/domain/catalog"
	_ "github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	_ "github.com/timothymamo/shift2eu/internal/domain/text"
)

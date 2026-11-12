// Package foo stands for a package outside the composition roots and the HTTP
// API that no strict rule holds: internal/config, or any other package of the
// module. adapters-are-built-in-cmd is the only rule that refuses an adapter
// import here, and it is lax, so this package shows that rule alone stops one:
// httpapi above is refused by httpapi-gets-adapters-from-cmd too, which would
// keep the probe green if the lax rule stopped firing.
package foo

import (
	"github.com/timothymamo/shift2eu/internal/adapter/report/markdown"           // want "import 'github.com/timothymamo/shift2eu/internal/adapter/report/markdown' is not allowed from list 'adapters-are-built-in-cmd': adapters are constructed only in cmd/shift2eu and cmd/shift2eu-server"
	"github.com/timothymamo/shift2eu/internal/adapter/request/assessmentrequest" // want "import 'github.com/timothymamo/shift2eu/internal/adapter/request/assessmentrequest' is not allowed from list 'adapters-are-built-in-cmd': adapters are constructed only in cmd/shift2eu and cmd/shift2eu-server"
	"github.com/timothymamo/shift2eu/internal/adapter/source/inventoryfile"      // want "import 'github.com/timothymamo/shift2eu/internal/adapter/source/inventoryfile' is not allowed from list 'adapters-are-built-in-cmd': adapters are constructed only in cmd/shift2eu and cmd/shift2eu-server"
	"github.com/timothymamo/shift2eu/internal/adapter/target/ovhcloud"           // want "import 'github.com/timothymamo/shift2eu/internal/adapter/target/ovhcloud' is not allowed from list 'adapters-are-built-in-cmd': adapters are constructed only in cmd/shift2eu and cmd/shift2eu-server"
)

// Names names the adapters a package outside the composition roots must not
// reach.
const Names = markdown.Name + assessmentrequest.Name + inventoryfile.Name + ovhcloud.Name

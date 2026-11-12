// Package httpapi stands for the HTTP API, a driving adapter that constructs
// no adapter: it gets them through httpapi.Deps from its composition root. An
// import of an adapter here must be reported by adapters-are-built-in-cmd,
// beside httpapi-gets-adapters-from-cmd, which already refuses it.
package httpapi

import (
	"github.com/timothymamo/shift2eu/internal/adapter/report/markdown"           // want "import 'github.com/timothymamo/shift2eu/internal/adapter/report/markdown' is not allowed from list 'httpapi-gets-adapters-from-cmd'" "import 'github.com/timothymamo/shift2eu/internal/adapter/report/markdown' is not allowed from list 'adapters-are-built-in-cmd': adapters are constructed only in cmd/shift2eu and cmd/shift2eu-server"
	"github.com/timothymamo/shift2eu/internal/adapter/request/assessmentrequest" // want "import 'github.com/timothymamo/shift2eu/internal/adapter/request/assessmentrequest' is not allowed from list 'httpapi-gets-adapters-from-cmd'" "import 'github.com/timothymamo/shift2eu/internal/adapter/request/assessmentrequest' is not allowed from list 'adapters-are-built-in-cmd': adapters are constructed only in cmd/shift2eu and cmd/shift2eu-server"
	"github.com/timothymamo/shift2eu/internal/adapter/source/inventoryfile"      // want "import 'github.com/timothymamo/shift2eu/internal/adapter/source/inventoryfile' is not allowed from list 'httpapi-gets-adapters-from-cmd'" "import 'github.com/timothymamo/shift2eu/internal/adapter/source/inventoryfile' is not allowed from list 'adapters-are-built-in-cmd': adapters are constructed only in cmd/shift2eu and cmd/shift2eu-server"
	"github.com/timothymamo/shift2eu/internal/adapter/target/ovhcloud"           // want "import 'github.com/timothymamo/shift2eu/internal/adapter/target/ovhcloud' is not allowed from list 'httpapi-gets-adapters-from-cmd'" "import 'github.com/timothymamo/shift2eu/internal/adapter/target/ovhcloud' is not allowed from list 'adapters-are-built-in-cmd': adapters are constructed only in cmd/shift2eu and cmd/shift2eu-server"
)

// Names names the adapters the HTTP API must not reach.
const Names = markdown.Name + assessmentrequest.Name + inventoryfile.Name + ovhcloud.Name

// The composition root: adapters are constructed here, so an adapter import is
// not reported by adapters-are-built-in-cmd.
package main

import (
	"github.com/timothymamo/shift2eu/internal/adapter/report/markdown"
	"github.com/timothymamo/shift2eu/internal/adapter/request/assessmentrequest"
	"github.com/timothymamo/shift2eu/internal/adapter/source/inventoryfile"
	"github.com/timothymamo/shift2eu/internal/adapter/target/ovhcloud"
)

func main() {
	_ = markdown.Name + assessmentrequest.Name + inventoryfile.Name + ovhcloud.Name
}

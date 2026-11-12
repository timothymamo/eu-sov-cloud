package main

import (
	"github.com/timothymamo/shift2eu/internal/adapter/report/jsonreport"
	"github.com/timothymamo/shift2eu/internal/adapter/report/markdown"
	"github.com/timothymamo/shift2eu/internal/adapter/request/assessmentrequest"
	"github.com/timothymamo/shift2eu/internal/adapter/source/inventoryfile"
	"github.com/timothymamo/shift2eu/internal/adapter/target/ovhcloud"
	"github.com/timothymamo/shift2eu/internal/adapter/target/scaleway"
	"github.com/timothymamo/shift2eu/internal/adapter/target/upcloud"
	"github.com/timothymamo/shift2eu/internal/app/analysis"
	"github.com/timothymamo/shift2eu/internal/httpapi"
)

// deps wires the adapters of the API: the strict request decoder, the inventory
// reader over the document in the request, the embedded OVHcloud, Scaleway and
// UpCloud catalogs, as cmd/shift2eu wires them, and the JSON and Markdown
// reporters, each rendering both the single-target report and the comparison.
// None of them uses the network, a file or credentials.
func deps() httpapi.Deps {
	return httpapi.Deps{
		DecodeRequest: assessmentrequest.Decode,
		NewInventorySource: func(doc []byte) analysis.InventorySource {
			return inventoryfile.FromBytes(doc)
		},
		Targets: map[string]func() analysis.CatalogSource{
			"ovhcloud": func() analysis.CatalogSource { return ovhcloud.New() },
			"scaleway": func() analysis.CatalogSource { return scaleway.New() },
			"upcloud":  func() analysis.CatalogSource { return upcloud.New() },
		},
		JSON: httpapi.Format{
			NewReporter:           func() analysis.Reporter { return jsonreport.New() },
			NewComparisonReporter: func() analysis.ComparisonReporter { return jsonreport.New() },
		},
		Markdown: httpapi.Format{
			NewReporter:           func() analysis.Reporter { return markdown.New() },
			NewComparisonReporter: func() analysis.ComparisonReporter { return markdown.New() },
		},
	}
}

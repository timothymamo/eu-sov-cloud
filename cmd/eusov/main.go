// Command eusov analyses infrastructure running in a hyperscaler (AWS, Azure,
// GCP) and reports what can move to an EU cloud provider (OVHcloud, Scaleway,
// UpCloud).
//
// main is the composition root: it is the only place that wires concrete
// adapters into the application use cases. The command line itself is handled
// by internal/cli.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/report/markdown"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/inventoryfile"
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/target/scaleway"
	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/cli"
)

func main() {
	// SIGINT and SIGTERM cancel ctx, which ends a run with exit code 1. Once
	// ctx is cancelled, stop restores the default behaviour, so a second signal
	// ends the process at once.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	context.AfterFunc(ctx, stop)

	// Exit as soon as Run returns, without waiting for adapter goroutines: after
	// a cancellation the inventory-file adapter may leave one blocked in open()
	// on a FIFO with no writer (ADR 0006).
	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, deps()))
}

// deps wires the adapters of the assess path: the inventory file, the embedded
// Scaleway catalog and the Markdown reporter. None of them uses the network or
// credentials.
func deps() cli.Deps {
	return cli.Deps{
		NewInventorySource: func(path string) analysis.InventorySource {
			return inventoryfile.New(path)
		},
		Targets: map[string]func() analysis.CatalogSource{
			"scaleway": func() analysis.CatalogSource { return scaleway.New() },
		},
		NewReporter: func() analysis.Reporter { return markdown.New() },
	}
}

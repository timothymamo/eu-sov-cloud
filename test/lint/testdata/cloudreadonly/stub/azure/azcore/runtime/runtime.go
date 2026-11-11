// Package runtime stands in for github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package runtime

type Pipeline struct{}

func NewPipeline(module, version string, plOpts, options any) Pipeline { return Pipeline{} }

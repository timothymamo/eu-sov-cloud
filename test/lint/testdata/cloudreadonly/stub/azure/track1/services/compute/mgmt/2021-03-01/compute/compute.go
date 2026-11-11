// Package compute stands in for github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2021-03-01/compute in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package compute

type VirtualMachinesClient struct{}

func NewVirtualMachinesClient(string) VirtualMachinesClient { return VirtualMachinesClient{} }

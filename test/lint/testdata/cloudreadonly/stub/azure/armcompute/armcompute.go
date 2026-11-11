// Package armcompute stands in for github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package armcompute

import "context"

type VirtualMachinesClient struct{}

type VirtualMachinesClientGetOptions struct{}

type VirtualMachinesClientListOptions struct{}

type VirtualMachinesClientBeginDeleteOptions struct{}

type VirtualMachine struct{}

type Pager struct{}

func (*Pager) More() bool { return false }

type Poller struct{}

func NewVirtualMachinesClient(string, any, any) (*VirtualMachinesClient, error) {
	return &VirtualMachinesClient{}, nil
}

func (*VirtualMachinesClient) Get(context.Context, string, string, *VirtualMachinesClientGetOptions) (VirtualMachine, error) {
	return VirtualMachine{}, nil
}

func (*VirtualMachinesClient) NewListPager(string, *VirtualMachinesClientListOptions) *Pager {
	return &Pager{}
}

func (*VirtualMachinesClient) BeginDelete(context.Context, string, string, *VirtualMachinesClientBeginDeleteOptions) (*Poller, error) {
	return &Poller{}, nil
}

// Package compute stands in for cloud.google.com/go/compute/apiv1 in the
// cloud-read-only probe: the same import path, package name and method
// shapes, with no code behind them.
package compute

import (
	"context"

	"cloud.google.com/go/compute/apiv1/computepb"
)

type InstancesClient struct{}

type Operation struct{}

func NewInstancesRESTClient(context.Context) (*InstancesClient, error) {
	return &InstancesClient{}, nil
}

func (*InstancesClient) Get(context.Context, *computepb.GetInstanceRequest) (*computepb.Instance, error) {
	return nil, nil
}

func (*InstancesClient) Delete(context.Context, *computepb.DeleteInstanceRequest) (*Operation, error) {
	return nil, nil
}

func (*InstancesClient) Close() error { return nil }

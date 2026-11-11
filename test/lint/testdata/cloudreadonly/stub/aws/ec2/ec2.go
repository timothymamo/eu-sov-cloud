// Package ec2 stands in for github.com/aws/aws-sdk-go-v2/service/ec2 in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package ec2

import "context"

// Options carries the client options a func(*Options) can set per call.
type Options struct {
	Region       string
	APIOptions   []func(any) error
	HTTPClient   any
	BaseEndpoint *string
}

func WithAPIOptions(...func(any) error) func(*Options) { return nil }

type Client struct{}

type DescribeInstancesInput struct{}

type DescribeInstancesOutput struct{}

type GetPasswordDataInput struct{}

type GetPasswordDataOutput struct{}

type TerminateInstancesInput struct{}

type TerminateInstancesOutput struct{}

type RunInstancesInput struct{}

type RunInstancesOutput struct{}

type RequestSpotInstancesInput struct{ InstanceCount *int32 }

type RequestSpotInstancesOutput struct{}

func NewFromConfig(any, ...func(*Options)) *Client { return &Client{} }

func (*Client) DescribeInstances(context.Context, *DescribeInstancesInput, ...func(*Options)) (*DescribeInstancesOutput, error) {
	return nil, nil
}

func (*Client) GetPasswordData(context.Context, *GetPasswordDataInput, ...func(*Options)) (*GetPasswordDataOutput, error) {
	return nil, nil
}

func (*Client) TerminateInstances(context.Context, *TerminateInstancesInput, ...func(*Options)) (*TerminateInstancesOutput, error) {
	return nil, nil
}

func (*Client) RunInstances(context.Context, *RunInstancesInput, ...func(*Options)) (*RunInstancesOutput, error) {
	return nil, nil
}

func (*Client) RequestSpotInstances(context.Context, *RequestSpotInstancesInput, ...func(*Options)) (*RequestSpotInstancesOutput, error) {
	return nil, nil
}

type DescribeInstancesAPIClient interface {
	DescribeInstances(context.Context, *DescribeInstancesInput, ...func(*Options)) (*DescribeInstancesOutput, error)
}

type TerminateInstancesAPIClient interface {
	TerminateInstances(context.Context, *TerminateInstancesInput, ...func(*Options)) (*TerminateInstancesOutput, error)
}

type DescribeInstancesPaginator struct{}

func NewDescribeInstancesPaginator(DescribeInstancesAPIClient, *DescribeInstancesInput) *DescribeInstancesPaginator {
	return &DescribeInstancesPaginator{}
}

func (*DescribeInstancesPaginator) HasMorePages() bool { return false }

func (*DescribeInstancesPaginator) NextPage(context.Context, ...func(*Options)) (*DescribeInstancesOutput, error) {
	return nil, nil
}

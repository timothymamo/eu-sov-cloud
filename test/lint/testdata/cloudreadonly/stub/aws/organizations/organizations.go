// Package organizations stands in for github.com/aws/aws-sdk-go-v2/service/organizations in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package organizations

import "context"

type Options struct{}

type Client struct{}

type LeaveOrganizationInput struct{}

type LeaveOrganizationOutput struct{}

func NewFromConfig(any, ...func(*Options)) *Client { return &Client{} }

func (*Client) LeaveOrganization(context.Context, *LeaveOrganizationInput, ...func(*Options)) (*LeaveOrganizationOutput, error) {
	return nil, nil
}

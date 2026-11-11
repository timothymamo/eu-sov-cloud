// Package azblob stands in for github.com/Azure/azure-sdk-for-go/sdk/storage/azblob in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package azblob

import "context"

type Client struct{}

type ClientOptions struct{}

type ListContainersOptions struct{}

type UploadBufferOptions struct{}

type Pager struct{}

func (*Client) NewListContainersPager(*ListContainersOptions) *Pager { return &Pager{} }

func (*Client) UploadBuffer(context.Context, string, string, []byte, *UploadBufferOptions) (any, error) {
	return nil, nil
}

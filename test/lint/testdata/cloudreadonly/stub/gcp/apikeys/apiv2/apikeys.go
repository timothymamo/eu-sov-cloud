// Package apikeys stands in for cloud.google.com/go/apikeys/apiv2 in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package apikeys

import "context"

type Client struct{}

func (*Client) GetKeyString(context.Context, any) (any, error) { return nil, nil }

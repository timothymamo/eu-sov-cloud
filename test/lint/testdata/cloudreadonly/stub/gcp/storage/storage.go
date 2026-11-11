// Package storage stands in for cloud.google.com/go/storage in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package storage

import "context"

type Client struct{}

func NewClient(context.Context) (*Client, error) { return &Client{}, nil }

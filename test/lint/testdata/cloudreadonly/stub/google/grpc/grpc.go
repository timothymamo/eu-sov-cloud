// Package grpc stands in for google.golang.org/grpc in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package grpc

import "context"

type ClientConn struct{}

func NewClient(string) (*ClientConn, error) { return &ClientConn{}, nil }

func (*ClientConn) Invoke(context.Context, string, any, any) error { return nil }

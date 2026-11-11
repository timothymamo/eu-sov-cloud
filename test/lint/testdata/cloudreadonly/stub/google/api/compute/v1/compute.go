// Package compute stands in for google.golang.org/api/compute/v1 in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package compute

import "context"

type Service struct{}

func NewService(context.Context) (*Service, error) { return &Service{}, nil }

// Package transport stands in for google.golang.org/api/transport in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package transport

import "context"

func Creds(context.Context) (any, error) { return nil, nil }

// Package codes stands in for google.golang.org/grpc/codes in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package codes

type Code uint32

const NotFound Code = 5

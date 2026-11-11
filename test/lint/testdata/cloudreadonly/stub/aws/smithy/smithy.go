// Package smithy stands in for github.com/aws/smithy-go in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package smithy

// APIError is how an adapter reads an API error code.
type APIError interface {
	error
	ErrorCode() string
}

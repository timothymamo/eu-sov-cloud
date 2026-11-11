// Package v4 stands in for github.com/aws/aws-sdk-go-v2/aws/signer/v4 in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package v4

type Signer struct{}

func NewSigner(...func(any)) *Signer { return &Signer{} }

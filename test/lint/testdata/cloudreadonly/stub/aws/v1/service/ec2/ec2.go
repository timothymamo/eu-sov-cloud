// Package ec2 stands in for github.com/aws/aws-sdk-go/service/ec2 in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package ec2

type EC2 struct{}

func New(any) *EC2 { return &EC2{} }

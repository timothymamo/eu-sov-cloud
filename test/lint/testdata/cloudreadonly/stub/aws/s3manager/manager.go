// Package manager stands in for github.com/aws/aws-sdk-go-v2/feature/s3/manager in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package manager

type Uploader struct{}

func NewUploader(any, ...func(*Uploader)) *Uploader { return &Uploader{} }

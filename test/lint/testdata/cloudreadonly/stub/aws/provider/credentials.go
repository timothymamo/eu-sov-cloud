// Package credentials stands in for github.com/aws/aws-sdk-go-v2/credentials in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package credentials

type StaticCredentialsProvider struct{}

func NewStaticCredentialsProvider(key, secret, session string) StaticCredentialsProvider {
	return StaticCredentialsProvider{}
}

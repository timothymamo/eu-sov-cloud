// Package aws stands in for github.com/aws/aws-sdk-go-v2/aws in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package aws

import "context"

type Credentials struct{ AccessKeyID, SecretAccessKey, SessionToken string }

type CredentialsProvider interface {
	Retrieve(context.Context) (Credentials, error)
}

// Config is what config.LoadDefaultConfig returns and NewFromConfig takes.
type Config struct {
	Region       string
	BaseEndpoint *string
	Credentials  CredentialsProvider
	HTTPClient   any
	APIOptions   []func(any) error
}

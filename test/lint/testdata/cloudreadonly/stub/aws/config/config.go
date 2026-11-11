// Package config stands in for github.com/aws/aws-sdk-go-v2/config in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package config

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
)

type LoadOptions struct{}

func LoadDefaultConfig(context.Context, ...func(*LoadOptions) error) (aws.Config, error) {
	return aws.Config{}, nil
}

func WithRegion(string) func(*LoadOptions) error { return nil }

func WithCredentialsProvider(aws.CredentialsProvider) func(*LoadOptions) error { return nil }

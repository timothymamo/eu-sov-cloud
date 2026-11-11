// Package aws stands in for the AWS source adapter: it reaches its cloud only
// through the SDK, as the SDK-only rule requires, and its own import of the SDK
// is what pulls the SDK's packages, and their dependencies, into the listing.
package aws

import "github.com/aws/aws-sdk-go-v2/service/ec2"

// New returns the SDK client the adapter would use.
func New() ec2.Client { return ec2.Client{} }

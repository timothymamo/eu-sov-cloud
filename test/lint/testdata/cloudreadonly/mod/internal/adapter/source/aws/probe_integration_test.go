//go:build integration

package aws_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// An integration test runs with real credentials, so it is held to the same
// rule.
func TestProbe(t *testing.T) {
	ctx := context.Background()
	client := ec2.NewFromConfig(nil)
	if _, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.TerminateInstances(ctx, nil); err != nil { // want "cloud-read-only: an AWS SDK client allows"
		t.Fatal(err)
	}
	var _ *ec2.TerminateInstancesInput // want "cloud-read-only: only the input, output or APIClient of an AWS"
}

// Package aws is a probe for forbidigo's cloud-read-only group and the depguard
// rules that back it (test/lint, TestCloudReadOnlyProbe): a line that ends in a
// want comment must be refused with each message it names, and no other line
// may be refused.
package aws

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware" // want "import 'github.com/aws/smithy-go/middleware' is not allowed from list 'cloud-adapters-reach-the-cloud-through-the-sdk'"
)

// describer is an adapter's own interface that declares a read only.
type describer interface {
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

// terminator is an adapter's own interface that an SDK client satisfies.
type terminator interface {
	TerminateInstances(context.Context, *ec2.TerminateInstancesInput, ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) // want "cloud-read-only: only the input, output or APIClient of an AWS" "test/lint: an interface of a cloud source adapter declares only read verbs"
}

// helper is unexported, so no SDK client can satisfy it.
type helper interface {
	terminate(context.Context) error
}

// Reads makes the calls an AWS adapter needs; none is refused.
func Reads(ctx context.Context, client *ec2.Client, api describer, h helper) error {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("eu-west-1"))
	if err != nil {
		return err
	}
	_ = ec2.NewFromConfig(cfg, func(o *ec2.Options) { o.Region = cfg.Region })
	_ = cfg.BaseEndpoint // an endpoint override is a reviewer check, not a lint rule
	if _, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{}); err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			return nil
		}
		return err
	}
	var _ *ec2.DescribeInstancesOutput
	var _ ec2.DescribeInstancesAPIClient = client
	pages := ec2.NewDescribeInstancesPaginator(client, &ec2.DescribeInstancesInput{})
	for pages.HasMorePages() {
		if _, err := pages.NextPage(ctx); err != nil {
			return err
		}
	}
	if _, err := api.DescribeInstances(ctx, nil); err != nil {
		return err
	}
	return h.terminate(ctx)
}

// Writes are refused, each by the entry its want comment names.
func Writes(ctx context.Context, client *ec2.Client, api terminator) {
	_, _ = client.TerminateInstances(ctx, nil)   // want "cloud-read-only: an AWS SDK client allows"
	_, _ = client.RunInstances(ctx, nil)         // want "cloud-read-only: an AWS SDK client allows"
	_, _ = client.RequestSpotInstances(ctx, nil) // want "cloud-read-only: an AWS SDK client allows"
	f := client.TerminateInstances               // want "cloud-read-only: an AWS SDK client allows"
	_ = f
	_ = (*ec2.Client).TerminateInstances    // want "cloud-read-only: an AWS SDK client allows"
	var _ *ec2.TerminateInstancesInput      // want "cloud-read-only: only the input, output or APIClient of an AWS"
	var _ *ec2.TerminateInstancesOutput     // want "cloud-read-only: only the input, output or APIClient of an AWS"
	var _ ec2.TerminateInstancesAPIClient   // want "cloud-read-only: only the input, output or APIClient of an AWS"
	_, _ = api.TerminateInstances(ctx, nil) // want "cloud-read-only: a source adapter calls no method"
	_ = manager.NewUploader(client)         // want "cloud-read-only: the S3 upload manager"
	_, _ = client.GetPasswordData(ctx, nil) // want "cloud-read-only: this AWS call mints"
	var _ *ec2.GetPasswordDataInput         // want "cloud-read-only: this AWS call mints"
}

// Hooks that rewrite a request or hand out a credential are refused.
func Hooks(cfg aws.Config) {
	_ = cfg.Credentials                                                     // want "cloud-read-only: the credentials, HTTP client, API options and interceptors of an AWS config"
	_ = aws.Config{Credentials: nil}                                        // want "cloud-read-only: the credentials, HTTP client, API options and interceptors of an AWS config"
	_ = config.WithCredentialsProvider(nil)                                 // want "cloud-read-only: the credentials, HTTP client, API options and interceptors of an AWS config"
	_ = ec2.Options{APIOptions: nil}                                        // want "cloud-read-only: the credentials, HTTP client, API options and interceptors of an AWS client"
	_ = ec2.NewFromConfig(cfg, func(o *ec2.Options) { o.HTTPClient = nil }) // want "cloud-read-only: the credentials, HTTP client, API options and interceptors of an AWS client"
	_ = ec2.WithAPIOptions()                                                // want "cloud-read-only: the credentials, HTTP client, API options and interceptors of an AWS client"
	_ = credentials.NewStaticCredentialsProvider("", "", "")                // want "cloud-read-only: an AWS signer"
	_ = v4.NewSigner()                                                      // want "cloud-read-only: an AWS signer"
	var _ middleware.Stack                                                  // want "cloud-read-only: an AWS signer"
}

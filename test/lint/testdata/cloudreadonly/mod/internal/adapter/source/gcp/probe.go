// Package gcp is a probe for forbidigo's cloud-read-only group and the depguard
// rules that back it (test/lint, TestCloudReadOnlyProbe): a line that ends in a
// want comment must be refused with each message it names, and no other line
// may be refused.
package gcp

import (
	"context"

	apikeys "cloud.google.com/go/apikeys/apiv2"
	"cloud.google.com/go/auth"
	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/storage"
	"golang.org/x/oauth2"                        // want "import 'golang.org/x/oauth2' is not allowed from list 'cloud-adapters-reach-the-cloud-through-the-sdk'"
	computev1 "google.golang.org/api/compute/v1" // want "import 'google.golang.org/api/compute/v1' is not allowed from list 'cloud-adapters-reach-the-cloud-through-the-sdk'"
	"google.golang.org/api/option"
	"google.golang.org/api/transport" // want "import 'google.golang.org/api/transport' is not allowed from list 'cloud-adapters-reach-the-cloud-through-the-sdk'"
	"google.golang.org/grpc"          // want "import 'google.golang.org/grpc' is not allowed from list 'cloud-adapters-reach-the-cloud-through-the-sdk'"
	"google.golang.org/grpc/codes"
)

// Reads makes the calls a GCP adapter needs; none is refused.
func Reads(ctx context.Context, client *compute.InstancesClient) error {
	defer client.Close()
	if _, err := client.Get(ctx, &computepb.GetInstanceRequest{}); err != nil {
		return err
	}
	var _ *computepb.ListInstancesResponse
	_ = option.WithQuotaProject("p")
	_ = option.WithEndpoint("https://example.com") // an endpoint override is a reviewer check, not a lint rule
	_ = codes.NotFound
	return nil
}

// Writes are refused, each by the entry its want comment names.
func Writes(ctx context.Context, client *compute.InstancesClient, keys *apikeys.Client) {
	_, _ = client.Delete(ctx, nil)          // want "cloud-read-only: a GCP SDK client allows"
	var _ *computepb.DeleteInstanceRequest  // want "cloud-read-only: only the request or response of a GCP"
	var _ *computepb.DeleteInstanceResponse // want "cloud-read-only: only the request or response of a GCP"
	_, _ = keys.GetKeyString(ctx, nil)      // want "cloud-read-only: this GCP call returns"
	_, _ = storage.NewClient(ctx)           // want "cloud-read-only: a hand-written GCP client"
	_, _ = computev1.NewService(ctx)        // want "cloud-read-only: use cloud.google.com/go"
}

// Hooks that put in a transport or a credential are refused.
func Hooks(ctx context.Context) {
	_ = oauth2.StaticTokenSource(nil)   // want "cloud-read-only: a GCP token source"
	_, _ = transport.Creds(ctx)         // want "cloud-read-only: a GCP token source"
	var _ *auth.Credentials             // want "cloud-read-only: a GCP token source"
	_ = option.WithHTTPClient(nil)      // want "cloud-read-only: this option puts"
	conn, _ := grpc.NewClient("x")      // want "cloud-read-only: a gRPC connection"
	_ = conn.Invoke(ctx, "m", nil, nil) // want "cloud-read-only: a gRPC connection"
}

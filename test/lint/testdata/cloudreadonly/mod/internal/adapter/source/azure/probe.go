// Package azure is a probe for forbidigo's cloud-read-only group and the depguard
// rules that back it (test/lint, TestCloudReadOnlyProbe): a line that ends in a
// want comment must be refused with each message it names, and no other line
// may be refused.
package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	trackone "github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2021-03-01/compute" // want "import 'github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2021-03-01/compute' is not allowed from list 'cloud-adapters-reach-the-cloud-through-the-sdk'"
)

// Reads makes the calls an Azure adapter needs; none is refused.
func Reads(ctx context.Context, client *armcompute.VirtualMachinesClient, blobs *azblob.Client) error {
	_ = &arm.ClientOptions{}
	_ = arm.ClientOptions{DisableRPRegistration: true}
	if _, err := client.Get(ctx, "rg", "vm", nil); err != nil {
		return err
	}
	for pager := client.NewListPager("rg", &armcompute.VirtualMachinesClientListOptions{}); pager.More(); {
		break
	}
	_ = blobs.NewListContainersPager(&azblob.ListContainersOptions{})
	return nil
}

// Writes are refused, each by the entry its want comment names.
func Writes(ctx context.Context, client *armcompute.VirtualMachinesClient, blobs *azblob.Client,
	accounts *armstorage.AccountsClient, secrets *armkeyvault.SecretsClient,
) {
	_, _ = client.BeginDelete(ctx, "rg", "vm", nil)           // want "cloud-read-only: an Azure SDK client allows"
	var _ *armcompute.VirtualMachinesClientBeginDeleteOptions // want "cloud-read-only: only the options of an Azure Get"
	_, _ = blobs.UploadBuffer(ctx, "c", "b", nil, nil)        // want "cloud-read-only: an Azure SDK client allows"
	var _ *azblob.UploadBufferOptions                         // want "cloud-read-only: only the options of an Azure data-plane"
	var _ *azblob.ClientOptions                               // want "cloud-read-only: only the options of an Azure data-plane"
	_, _ = accounts.ListKeys(ctx, "rg", "a", nil)             // want "cloud-read-only: this Azure call mints"
	_, _ = secrets.Get(ctx, "rg", "v", "s", nil)              // want "cloud-read-only: this Azure client holds"
	_ = trackone.NewVirtualMachinesClient("sub")              // want "cloud-read-only: use aws-sdk-go-v2 or the Azure SDK"
}

// Hooks that rewrite a request or hand out a credential are refused, also
// through the fields arm.ClientOptions promotes from the embedded azcore
// options, as the real SDK lays them out.
func Hooks(ctx context.Context, cred azcore.TokenCredential) {
	_, _ = cred.GetToken(ctx, policy.TokenRequestOptions{}) // want "cloud-read-only: an Azure token, request or pipeline"
	var _ *policy.Request                                   // want "cloud-read-only: an Azure token, request or pipeline"
	_ = runtime.NewPipeline("m", "v", nil, nil)             // want "cloud-read-only: an Azure token, request or pipeline"
	opts := &arm.ClientOptions{}
	opts.Transport = nil                                                              // want "cloud-read-only: an Azure transport or policy"
	opts.PerCallPolicies = nil                                                        // want "cloud-read-only: an Azure transport or policy"
	_ = arm.ClientOptions{ClientOptions: policy.ClientOptions{PerRetryPolicies: nil}} // want "cloud-read-only: an Azure transport or policy"
	_ = azcore.ClientOptions{Transport: nil}                                          // want "cloud-read-only: an Azure transport or policy"
}

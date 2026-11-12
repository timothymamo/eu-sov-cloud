package lint_test

import (
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"testing"
)

// TestCloudReadOnlyProbe runs forbidigo and depguard with the repository's
// .golangci.yml on probe source adapters (testdata/cloudreadonly/mod) over stub
// cloud SDKs. Each entry of forbidigo's cloud-read-only group has a probe line
// that only it refuses, so an entry that silently stops matching fails here,
// and so does one that refuses a read: the read calls, an AWS paginator, an
// Azure pager, a GCP Close and the allowed options sit beside the writes. The
// depguard lines prove that a source adapter, and a sibling package of one,
// imports nothing that calls a method by name or runs a program
// (cloud-adapters-reach-the-cloud-through-the-sdk), and that a cloud SDK is
// imported only in its provider's adapter (cloud-sdks-only-in-provider-adapters).
func TestCloudReadOnlyProbe(t *testing.T) {
	t.Parallel()

	runForbidigoProbe(t, forbidigoProbe{
		dir:      "cloudreadonly",
		tag:      "cloud-read-only",
		depguard: true,
		// Each stub has the real SDK's import path, package name and shapes,
		// so forbidigo matches it as it would the real SDK, with no network
		// and no module download. azure/azcore lays its ClientOptions out as
		// the real SDK does: arm.ClientOptions aliases the arm policy
		// package's ClientOptions, which embeds the azcore one.
		stubs: map[string]string{
			"aws/config":        "github.com/aws/aws-sdk-go-v2/config",
			"aws/ec2":           "github.com/aws/aws-sdk-go-v2/service/ec2",
			"aws/organizations": "github.com/aws/aws-sdk-go-v2/service/organizations",
			"aws/provider":      "github.com/aws/aws-sdk-go-v2/credentials", // a dir named for credentials trips gosec G101
			"aws/s3manager":     "github.com/aws/aws-sdk-go-v2/feature/s3/manager",
			"aws/sdk":           "github.com/aws/aws-sdk-go-v2",
			"aws/smithy":        "github.com/aws/smithy-go",
			"aws/v1":            "github.com/aws/aws-sdk-go",
			"azure/armcompute":  "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5",
			"azure/armkeyvault": "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault",
			"azure/armstorage":  "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage",
			"azure/azblob":      "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob",
			"azure/azcore":      "github.com/Azure/azure-sdk-for-go/sdk/azcore",
			"azure/track1":      "github.com/Azure/azure-sdk-for-go",
			"gcp/apikeys":       "cloud.google.com/go/apikeys",
			"gcp/auth":          "cloud.google.com/go/auth",
			"gcp/compute":       "cloud.google.com/go/compute",
			"gcp/storage":       "cloud.google.com/go/storage",
			"golang/oauth2":     "golang.org/x/oauth2",
			"google/api":        "google.golang.org/api",
			"google/grpc":       "google.golang.org/grpc",
		},
	})
}

// readOnlySDK describes one SDK's allowlist entries in forbidigo's
// cloud-read-only group, for TestCloudReadOnlyPatterns.
type readOnlySDK struct {
	name string
	pkg  string // an import path of a service package of the SDK
	// symbols returns how forbidigo renders the uses of an operation op, each
	// with the msg prefix of the entry that refuses it when op is a write.
	symbols func(op string) []symbol
	reads   []string // operations the SDK's entries must allow
}

type symbol struct{ name, entry string }

// cloudWrites are operation names that write, or start a write, on a cloud:
// every verb of issue #38's denylist and of the verbs the review of the rule
// found missing from it, each alone and with a resource. A first-letter class
// with a letter missing, or a trie branch cut short, lets one through.
var cloudWrites = func() []string {
	verbs := []string{
		// AWS (issue #38)
		"Create", "Delete", "Put", "Update", "Modify", "Tag", "Untag", "Terminate", "Run", "Start", "Stop",
		"Reboot", "Attach", "Detach", "Associate", "Disassociate", "Authorize", "Revoke", "Set", "Add",
		"Remove", "Enable", "Disable", "Register", "Deregister", "Import", "Copy", "Restore",
		// Azure and GCP (issue #38)
		"Begin", "BeginCreateOrUpdate", "BeginDelete", "CreateOrUpdate", "Insert", "Patch",
		// Found missing in review
		"Purchase", "Provision", "Deprovision", "Request", "Confirm", "Advertise", "Withdraw", "Monitor",
		"Unmonitor", "Swap", "Reimage", "Redeploy", "Reapply", "Power", "PowerOff", "Scale", "Merge",
		"Split", "Truncate", "Clear", "Flush", "Commit", "Rollback", "Sign", "Decrypt", "Leave", "Bundle",
		"Subscribe", "Unsubscribe", "Receive", "Backtrack", "Switchover", "Complete", "Initiate", "Signal",
		"Continue", "Generalize", "SimulateEviction", "Acquire", "Break", "Renew", "Abandon", "Recreate",
		"BulkInsert", "Deprecate", "Expand", "Switch", "Perform", "SimulateMaintenanceEvent", "Undelete",
	}
	var ops []string
	for _, v := range verbs {
		ops = append(ops, v, v+"Instances")
	}
	// One name for every first letter, so that no letter is missing from a
	// first-letter class or a branch.
	for c := 'A'; c <= 'Z'; c++ {
		ops = append(ops, string(c)+"ogus", string(c)+"zzz")
	}
	// Names that run a read verb on in lower case, or stop part-way into a
	// longer verb: none is a read.
	return append(ops, "Describes", "Describer", "Getaway", "Gets", "Listen", "Listing", "Lists",
		"Closer", "Closes", "CloseAndDelete", "AggregatedLists", "NewDeletePager",
		"NewListenPager", "Newsletter", "NewGetaway", "Nothing")
}()

// readOnlySDKs are the SDKs whose clients forbidigo's cloud-read-only group
// holds to an allowlist of read verbs.
var readOnlySDKs = []readOnlySDK{
	{
		name: "AWS",
		pkg:  "github.com/aws/aws-sdk-go-v2/service/ec2",
		symbols: func(op string) []symbol {
			const input = "cloud-read-only: only the input, output or APIClient of an AWS"
			return []symbol{
				{"ec2.Client." + op, "cloud-read-only: an AWS SDK client allows"},
				{"ec2." + op + "Input", input},
				{"ec2." + op + "Output", input},
				{"ec2." + op + "APIClient", input},
			}
		},
		reads: []string{
			"DescribeInstances", "DescribeInstanceStatus", "Describe", "GetObject", "GetBucketPolicy",
			"Get", "ListBuckets", "ListTagsForResource", "List", "Describe2", "Get2", "List2",
		},
	},
	{
		name: "Azure resource manager",
		pkg:  "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5",
		symbols: func(op string) []symbol {
			return []symbol{
				{"armcompute.VirtualMachinesClient." + op, "cloud-read-only: an Azure SDK client allows"},
				{"armcompute.VirtualMachinesClient" + op + "Options", "cloud-read-only: only the options of an Azure Get"},
			}
		},
		reads: []string{
			"Get", "GetInstanceView", "List", "ListAll", "ListByLocation", "ListAvailableSizes",
			"NewListPager", "NewListAllPager", "NewListByLocationPager", "NewGetPager", "NewGet", "NewList",
		},
	},
	{
		name: "Azure data plane",
		pkg:  "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob",
		symbols: func(op string) []symbol {
			ops := []symbol{
				{"azblob.Client." + op, "cloud-read-only: an Azure SDK client allows"},
			}
			// A data-plane option has no New form: NewListBlobsPager takes
			// ListBlobsOptions.
			if !strings.HasPrefix(op, "New") {
				ops = append(ops, symbol{"azblob." + op + "Options", "cloud-read-only: only the options of an Azure data-plane"})
			}
			return ops
		},
		reads: []string{"Get", "GetProperties", "List", "ListContainers", "NewListContainersPager", "NewGetPager"},
	},
	{
		name: "GCP",
		pkg:  "cloud.google.com/go/compute/apiv1",
		symbols: func(op string) []symbol {
			return []symbol{{"compute.InstancesClient." + op, "cloud-read-only: a GCP SDK client allows"}}
		},
		reads: []string{"Get", "GetGuestAttributes", "List", "ListReferrers", "AggregatedList", "AggregatedListInstances", "Close"},
	},
	{
		name: "GCP protobuf",
		pkg:  "cloud.google.com/go/compute/apiv1/computepb",
		symbols: func(op string) []symbol {
			const request = "cloud-read-only: only the request or response of a GCP"
			return []symbol{
				{"computepb." + op + "InstanceRequest", request},
				{"computepb." + op + "InstanceResponse", request},
			}
		},
		reads: []string{"Get", "GetGuestAttributes", "List", "ListReferrers", "AggregatedList"},
	},
}

// deniedNames are names that one denylist entry of forbidigo's cloud-read-only
// group refuses, all declared in one package and rendered as forbidigo renders
// them (docs/architecture.md, How forbidigo renders a name).
type deniedNames struct {
	entry string // the msg prefix of the entry that refuses them
	pkg   string // the import path of the package that declares them
	names []string
}

// cloudDenied holds, for every entry of the cloud-read-only group that is not
// an allowlist trie, names for every alternative and optional part (x?, x*,
// x{0,n}) of its pattern and pkg: TestCloudReadOnlyPatterns drops each part in
// turn and requires a name here that the entry then lets through. Most are
// real SDK names; where no real name needs a part, a name of the shape it is
// there for stands in (the example packages, sdx, a feature package or major
// version no SDK has yet). Every line that holds a stand-in is marked
// "// stand-in"; it is not an SDK name, and the part it pins is not dead.
var cloudDenied = []deniedNames{
	{
		entry: "cloud-read-only: the S3 upload manager",
		pkg:   "github.com/aws/aws-sdk-go-v2/feature/s3/manager",
		names: []string{"manager.NewUploader", "manager.Uploader", "manager.Uploader.Upload", "manager.UploadAPIClient", "manager.DeleteObjectsAPIClient"},
	},
	{entry: "cloud-read-only: the S3 upload manager", pkg: "github.com/aws/aws-sdk-go-v2/feature/s3/manager/v2", names: []string{"manager.Uploader.Upload"}}, // stand-in
	{entry: "cloud-read-only: use cloud.google.com/go", pkg: "google.golang.org/api/compute/v1", names: []string{"compute.InstancesService.Delete", "compute.InstancesDeleteCall.Do"}},
	{entry: "cloud-read-only: use cloud.google.com/go", pkg: "google.golang.org/api/merchantapi/products_v1beta", names: []string{"merchantapi.ProductsService.Get"}},

	// AWS calls that mint a credential or return a secret.
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/sts", names: []string{"sts.Client.GetSessionToken", "sts.GetFederationTokenInput"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/ecr", names: []string{"ecr.Client.GetAuthorizationToken"}},
	{
		entry: "cloud-read-only: this AWS call",
		pkg:   "github.com/aws/aws-sdk-go-v2/service/cognitoidentity",
		names: []string{"cognitoidentity.Client.GetOpenIdTokenForDeveloperIdentity", "cognitoidentity.Client.GetCredentialsForIdentity", "cognitoidentity.Client.GetId", "cognitoidentity.GetIdInput"},
	},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/sso", names: []string{"sso.Client.GetRoleCredentials"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/secretsmanager", names: []string{"secretsmanager.Client.GetSecretValue", "secretsmanager.GetSecretValueInput"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/lightsail", names: []string{"lightsail.Client.GetRelationalDatabaseMasterUserPassword", "lightsail.Client.GetInstanceAccessDetails"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/ec2", names: []string{"ec2.Client.GetPasswordData", "ec2.GetPasswordDataInput"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/gamelift", names: []string{"gamelift.Client.GetComputeAccess", "gamelift.Client.GetInstanceAccess"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/s3control", names: []string{"s3control.Client.GetDataAccess"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/apigateway", names: []string{"apigateway.Client.GetApiKey", "apigateway.Client.GetApiKeys", "apigateway.GetApiKeysAPIClient"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/wafv2", names: []string{"wafv2.Client.GetDecryptedAPIKey", "wafv2.Client.ListAPIKeys"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/appsync", names: []string{"appsync.Client.ListApiKeys"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/ivs", names: []string{"ivs.Client.GetStreamKey"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/quicksight", names: []string{"quicksight.Client.GetDashboardEmbedUrl", "quicksight.Client.GetSessionEmbedUrl"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/datazone", names: []string{"datazone.Client.GetIamPortalLoginUrl", "datazone.Client.GetConnection"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/m2", names: []string{"m2.Client.GetSignedBluinsightsUrl"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/example", names: []string{"example.Client.GetLoginURL", "example.Client.GetUploadUrl", "example.Client.GetPresignedUrl"}}, // stand-in
	{
		entry: "cloud-read-only: this AWS call",
		pkg:   "github.com/aws/aws-sdk-go-v2/service/ssm",
		names: []string{
			"ssm.Client.GetParameter", "ssm.Client.GetParametersByPath", "ssm.GetParameterInput", "ssm.GetParametersByPathAPIClient",
			// A call through a paginator's interface, which is not named Client.
			"ssm.GetParametersByPathAPIClient.GetParametersByPath",
		},
	},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/ssm/v2", names: []string{"ssm.Client.GetParameter"}}, // stand-in
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/feature/ssm/parameters", names: []string{"parameters.Client.GetParameter"}},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/storagegateway", names: []string{"storagegateway.Client.DescribeChapCredentials"}},
	{
		entry: "cloud-read-only: this AWS call",
		pkg:   "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider",
		names: []string{"cognitoidentityprovider.Client.DescribeUserPoolClient", "cognitoidentityprovider.Client.ListUserPoolClientSecrets"},
	},
	{entry: "cloud-read-only: this AWS call", pkg: "github.com/aws/aws-sdk-go-v2/service/glue", names: []string{"glue.Client.GetConnection", "glue.Client.GetConnections", "glue.GetConnectionsInput"}},

	// Azure calls that mint a credential or return a key, a secret or a URL
	// that carries one.
	{
		entry: "cloud-read-only: this Azure call",
		pkg:   "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage",
		names: []string{"armstorage.AccountsClient.ListKeys", "armstorage.AccountsClientListKeysOptions", "armstorage.AccountsClient.ListAccountSAS"},
	},
	{
		entry: "cloud-read-only: this Azure call",
		pkg:   "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v4",
		names: []string{
			"armappservice.WebAppsClient.ListFunctionSecrets", "armappservice.WebAppsClient.ListFunctionSecretsSlot",
			"armappservice.WebAppsClientListFunctionSecretsOptions", "armappservice.WebAppsClient.ListApplicationSettings",
			"armappservice.WebAppsClient.ListConnectionStrings", "armappservice.WebAppsClient.GetAuthSettings",
			"armappservice.WebAppsClient.GetAuthSettingsV2", "armappservice.StaticSitesClient.ListStaticSiteAppSettings",
			"armappservice.WebSiteManagementClient.GetPublishingUser", "armappservice.WebAppsClient.GetFunctionsAdminToken",
		},
	},
	{entry: "cloud-read-only: this Azure call", pkg: "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/maps/armmaps", names: []string{"armmaps.AccountsClient.ListSas"}},
	{
		entry: "cloud-read-only: this Azure call",
		pkg:   "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6",
		names: []string{"armcontainerservice.ManagedClustersClient.ListClusterAdminCredentials", "armcontainerservice.ManagedClustersClient.GetAccessProfile"},
	},
	{entry: "cloud-read-only: this Azure call", pkg: "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/logic/armlogic", names: []string{"armlogic.WorkflowTriggersClient.ListCallbackURL"}},
	{entry: "cloud-read-only: this Azure call", pkg: "github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets", names: []string{"azsecrets.Client.GetSecret", "azsecrets.GetSecretOptions"}},
	{
		entry: "cloud-read-only: this Azure call",
		pkg:   "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/example/armexample", // stand-in
		names: []string{
			"armexample.Client.GetKey", "armexample.Client.NewListKeysPager", "armexample.Client.ListKeysForKeyName",
			"armexample.Client.GetSASURL", "armexample.Client.GetSasURL", "armexample.Client.GetConnectionString",
			"armexample.Client.GetConnectionInfo", "armexample.Client.GetCredential", "armexample.Client.ListTokens",
			"armexample.Client.GetFullURL", "armexample.Client.GetDownloadURI", "armexample.Client.GetUploadURL",
			"armexample.Client.GetPassword", "armexample.Client.GetGatewaySettings", "armexample.Client.ListValue",
			"armexample.ClientListValueOptions",
		},
	},
	{
		entry: "cloud-read-only: this Azure client",
		pkg:   "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault",
		names: []string{"armkeyvault.SecretsClient.Get", "armkeyvault.SecretsClient.NewListPager", "armkeyvault.SecretsClientGetOptions"},
	},
	{
		entry: "cloud-read-only: this Azure client",
		pkg:   "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/search/armsearch",
		names: []string{"armsearch.AdminKeysClient.Get", "armsearch.QueryKeysClient.NewListBySearchServicePager"},
	},
	{entry: "cloud-read-only: this Azure client", pkg: "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/applicationinsights/armapplicationinsights", names: []string{"armapplicationinsights.APIKeysClient.NewListPager"}},
	{entry: "cloud-read-only: this Azure client", pkg: "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/operationalinsights/armoperationalinsights/v2", names: []string{"armoperationalinsights.SharedKeysClient.GetSharedKeys"}},
	{entry: "cloud-read-only: this Azure client", pkg: "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry", names: []string{"armcontainerregistry.TokensClient.Get"}},
	{entry: "cloud-read-only: this Azure client", pkg: "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/automation/armautomation", names: []string{"armautomation.KeysClient.ListByAutomationAccount", "armautomation.CredentialClient.Get"}},
	{
		entry: "cloud-read-only: this Azure client",
		pkg:   "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/example/armexample", // stand-in
		names: []string{
			"armexample.AuthKeysClient.Get", "armexample.EndpointKeysClient.Get", "armexample.MetricsKeysClient.Get",
			"armexample.BitLockerKeysClient.Get", "armexample.ActivationKeysClient.Get", "armexample.SecretClient.Get",
			"armexample.CredentialsClient.Get", "armexample.TokenClient.Get", "armexample.VaultSecretsClient.Get",
		},
	},

	// GCP calls that return a key, a token, a password or a secret.
	{entry: "cloud-read-only: this GCP call", pkg: "cloud.google.com/go/apikeys/apiv2", names: []string{"apikeys.Client.GetKeyString"}},
	{entry: "cloud-read-only: this GCP call", pkg: "cloud.google.com/go/apikeys/apiv2/apikeyspb", names: []string{"apikeyspb.GetKeyStringRequest"}},
	{entry: "cloud-read-only: this GCP call", pkg: "cloud.google.com/go/redis/apiv1", names: []string{"redis.CloudRedisClient.GetInstanceAuthString"}},
	{
		entry: "cloud-read-only: this GCP call",
		pkg:   "cloud.google.com/go/analytics/admin/apiv1alpha",
		names: []string{"admin.AnalyticsAdminClient.GetMeasurementProtocolSecret", "admin.AnalyticsAdminClient.ListMeasurementProtocolSecrets"},
	},
	{
		entry: "cloud-read-only: this GCP call",
		pkg:   "cloud.google.com/go/example/apiv1", // stand-in
		names: []string{
			"example.Client.GetToken", "example.Client.GetTokens", "example.Client.GetPassword", "example.Client.GetCredential",
			"example.Client.GetCredentials", "example.Client.GetActivationKey", "example.Client.GetActivationKeys",
			"example.Client.ListAuthTokens", "example.Client.ListActivationKeys", "example.Client.ListInstanceAuthTokens",
		},
	},

	// Clients no allowlist covers.
	{entry: "cloud-read-only: use aws-sdk-go-v2 or the Azure SDK", pkg: "github.com/aws/aws-sdk-go/service/ec2", names: []string{"ec2.EC2.TerminateInstances"}},
	{entry: "cloud-read-only: use aws-sdk-go-v2 or the Azure SDK", pkg: "github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute", names: []string{"compute.VirtualMachinesClient.Delete"}},
	{entry: "cloud-read-only: use aws-sdk-go-v2 or the Azure SDK", pkg: "github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2021-03-01/compute", names: []string{"compute.VirtualMachinesClient.Delete"}},
	{entry: "cloud-read-only: use aws-sdk-go-v2 or the Azure SDK", pkg: "github.com/Azure/azure-sdk-for-go/sdx", names: []string{"sdx.Client.Delete"}},   // stand-in
	{entry: "cloud-read-only: use aws-sdk-go-v2 or the Azure SDK", pkg: "github.com/Azure/azure-sdk-for-go/sdkx", names: []string{"sdkx.Client.Delete"}}, // stand-in
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/compute/metadata", names: []string{"metadata.Get"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/profiler", names: []string{"profiler.Start"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/datastore", names: []string{"datastore.Client.Put"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/firestore", names: []string{"firestore.Client.GetAll"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/spanner", names: []string{"spanner.Client.Apply"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/bigtable", names: []string{"bigtable.Table.Apply"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/storage", names: []string{"storage.BucketHandle.Delete"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/storage/transfermanager", names: []string{"transfermanager.Downloader.DownloadObject"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/storage/dataflux", names: []string{"dataflux.NewLister"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/storage/experimental", names: []string{"experimental.WithGRPCBidiReads"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/bigquery", names: []string{"bigquery.Client.Query"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/bigquery/v2", names: []string{"bigquery.Client.Query"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/bigquery/v2/query", names: []string{"query.Helper.StartQuery"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/bigquery/storage/managedwriter", names: []string{"managedwriter.ManagedStream.AppendRows"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/bigquery/storage/managedwriter/adapt", names: []string{"adapt.NormalizeDescriptor"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/pubsub", names: []string{"pubsub.Topic.Publish"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/pubsub/v2", names: []string{"pubsub.Publisher.Publish"}},
	{entry: "cloud-read-only: a hand-written GCP client", pkg: "cloud.google.com/go/pubsub/aliasshim", names: []string{"aliasshim.Topic"}},

	// Hooks that rewrite a request or hand out a credential.
	{entry: "cloud-read-only: an AWS signer", pkg: "github.com/aws/aws-sdk-go-v2/aws/signer/v4", names: []string{"v4.Signer.SignHTTP"}},
	{entry: "cloud-read-only: an AWS signer", pkg: "github.com/aws/aws-sdk-go-v2/credentials", names: []string{"credentials.NewStaticCredentialsProvider"}},
	{entry: "cloud-read-only: an AWS signer", pkg: "github.com/aws/aws-sdk-go-v2/credentials/stscreds", names: []string{"stscreds.NewAssumeRoleProvider"}},
	{entry: "cloud-read-only: an AWS signer", pkg: "github.com/aws/aws-sdk-go-v2/feature/ec2/imds", names: []string{"imds.Client.GetMetadata"}},
	{entry: "cloud-read-only: an AWS signer", pkg: "github.com/aws/aws-sdk-go-v2/feature/rds/auth", names: []string{"auth.BuildAuthToken"}},
	{entry: "cloud-read-only: an AWS signer", pkg: "github.com/aws/aws-sdk-go-v2/feature/dsql/auth", names: []string{"auth.GenerateDbConnectAuthToken"}},
	{entry: "cloud-read-only: an AWS signer", pkg: "github.com/aws/smithy-go/middleware", names: []string{"middleware.Stack.Build"}},
	{entry: "cloud-read-only: an AWS signer", pkg: "github.com/aws/smithy-go/transport/http", names: []string{"http.Request.SetStream"}},
	{entry: "cloud-read-only: an AWS signer", pkg: "github.com/aws/smithy-go/auth", names: []string{"auth.Option"}},
	{
		entry: "cloud-read-only: the credentials, HTTP client, API options and interceptors of an AWS config",
		pkg:   "github.com/aws/aws-sdk-go-v2/aws",
		names: []string{
			"aws.Config.Credentials", "aws.NewCredentialsCache", "aws.CredentialsCache.Retrieve", "aws.Config.BearerAuthTokenProvider",
			"aws.Config.HTTPClient", "aws.HTTPClient", "aws.Config.APIOptions", "aws.Config.Interceptors", "aws.Config.ServiceOptions",
		},
	},
	{
		entry: "cloud-read-only: the credentials, HTTP client, API options and interceptors of an AWS config",
		pkg:   "github.com/aws/aws-sdk-go-v2/config",
		names: []string{
			"config.WithBeforeAttempt", "config.WithAfterAttempt", "config.WithHTTPClient", "config.WithAPIOptions",
			"config.WithServiceOptions", "config.WithCredentialsProvider", "config.WithBearerAuthTokenProvider",
			"config.WithAssumeRoleCredentialOptions", "config.WithSSOTokenProviderOptions",
			"config.WithBearerAuthTokenCacheOptions", "config.WithSharedCredentialsFiles",
			"config.WithExampleCredentialsOptions", // stand-in
		},
	},
	{
		entry: "cloud-read-only: the credentials, HTTP client, API options and interceptors of an AWS client",
		pkg:   "github.com/aws/aws-sdk-go-v2/service/ec2",
		names: []string{
			"ec2.Options.APIOptions", "ec2.APIOptions", "ec2.Options.HTTPClient", "ec2.Options.Credentials",
			"ec2.Options.BearerAuthTokenProvider", "ec2.Options.HTTPSignerV4", "ec2.Options.Interceptors",
			"ec2.Options.AuthSchemes", "ec2.Options.AuthSchemeResolver", "ec2.WithAPIOptions",
		},
	},
	{entry: "cloud-read-only: the credentials, HTTP client, API options and interceptors of an AWS client", pkg: "github.com/aws/aws-sdk-go-v2/service/sqs/v2", names: []string{"sqs.Options.HTTPClient"}}, // stand-in
	{entry: "cloud-read-only: the credentials, HTTP client, API options and interceptors of an AWS client", pkg: "github.com/aws/aws-sdk-go-v2/feature/s3/manager", names: []string{"manager.Options.HTTPClient"}},
	{entry: "cloud-read-only: an Azure token, request or pipeline", pkg: "github.com/Azure/azure-sdk-for-go/sdk/azidentity", names: []string{"azidentity.DefaultAzureCredential.GetToken"}},
	{entry: "cloud-read-only: an Azure token, request or pipeline", pkg: "github.com/Azure/azure-sdk-for-go/sdk/azcore", names: []string{"azcore.TokenCredential.GetToken"}},
	{entry: "cloud-read-only: an Azure token, request or pipeline", pkg: "github.com/Azure/azure-sdk-for-go/sdk/azcore/internal/exported", names: []string{"exported.Request", "exported.Request.Raw"}},
	{
		entry: "cloud-read-only: an Azure token, request or pipeline",
		pkg:   "github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime",
		names: []string{"runtime.Pipeline.Do", "runtime.NewPipeline", "runtime.NewRequest", "runtime.NewRequestFromRequest", "runtime.NewRequestForPipeline"},
	},
	{
		entry: "cloud-read-only: an Azure transport or policy",
		pkg:   "github.com/Azure/azure-sdk-for-go/sdk/azcore/policy",
		names: []string{"policy.ClientOptions.Transport", "policy.Transport", "policy.ClientOptions.PerCallPolicies", "policy.PerRetryPolicies"},
	},
	{entry: "cloud-read-only: an Azure transport or policy", pkg: "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy", names: []string{"policy.ClientOptions.Transport"}},
	{entry: "cloud-read-only: a GCP token source", pkg: "golang.org/x/oauth2", names: []string{"oauth2.StaticTokenSource"}},
	{entry: "cloud-read-only: a GCP token source", pkg: "golang.org/x/oauth2/google", names: []string{"google.FindDefaultCredentials"}},
	{entry: "cloud-read-only: a GCP token source", pkg: "google.golang.org/api/transport", names: []string{"transport.Creds"}},
	{entry: "cloud-read-only: a GCP token source", pkg: "google.golang.org/api/transport/http", names: []string{"http.NewClient"}},
	{entry: "cloud-read-only: a GCP token source", pkg: "cloud.google.com/go/auth", names: []string{"auth.Credentials.Token"}},
	{entry: "cloud-read-only: a GCP token source", pkg: "cloud.google.com/go/auth/credentials", names: []string{"credentials.DetectDefault"}},
	{
		entry: "cloud-read-only: this option puts",
		pkg:   "google.golang.org/api/option",
		names: []string{
			"option.WithHTTPClient", "option.WithGRPCConn", "option.WithTokenSource", "option.WithCredentialsFile",
			"option.WithAuthCredentials", "option.WithAPIKey", "option.WithClientCertSource", "option.WithServiceAccountFile",
			"option.ImpersonateCredentials",
		},
	},
	{
		entry: "cloud-read-only: a gRPC connection",
		pkg:   "google.golang.org/grpc",
		names: []string{
			"grpc.Dial", "grpc.DialContext", "grpc.NewClient", "grpc.NewClientStream", "grpc.Invoke", "grpc.WithUnaryInterceptor",
			"grpc.UnaryClientInterceptor", "grpc.ClientConn.Invoke", "grpc.ClientConn.NewStream", "grpc.ClientConnInterface.Invoke",
		},
	},
}

// cloudDenylistReads are reads the denylist entries were narrowed to leave
// alone (see the comment above the secret entries in .golangci.yml): no
// cloud-read-only entry may refuse them.
var cloudDenylistReads = []struct {
	pkg   string
	names []string
}{
	{"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault", []string{"armkeyvault.KeysClient.Get", "armkeyvault.KeysClient.NewListPager"}},
	{"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/sql/armsql", []string{"armsql.ServerKeysClient.NewListByServerPager"}},
	{"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5", []string{"armcompute.SSHPublicKeysClient.Get"}},
	{"github.com/aws/aws-sdk-go-v2/service/redshiftserverless", []string{"redshiftserverless.Client.GetEndpointAccess"}},
	{"github.com/aws/aws-sdk-go-v2/service/codestarconnections", []string{"codestarconnections.Client.GetConnection"}},
	{"github.com/aws/aws-sdk-go-v2/config", []string{"config.LoadDefaultConfig", "config.WithRegion"}},
	{"github.com/aws/smithy-go", []string{"smithy.APIError"}},
	{"cloud.google.com/go/secretmanager/apiv1", []string{"secretmanager.Client.GetSecret"}},
	{"google.golang.org/api/option", []string{"option.WithQuotaProject"}},
	{"google.golang.org/grpc/codes", []string{"codes.NotFound"}},
}

// TestCloudReadOnlyPatterns checks the entries of forbidigo's cloud-read-only
// group, read from .golangci.yml, offline and without golangci-lint. Every
// write in long lists of real write and read operation names is refused by the
// allowlist entry that should refuse it, and every read by none. Every name in
// cloudDenied is refused by its denylist entry, and each alternative and
// optional part of a denylist entry is needed by one of them, so dropping one
// fails. The probe proves how forbidigo renders a name; this proves the
// patterns over far more names than a probe can hold, so a class-letter typo
// fails at once.
func TestCloudReadOnlyPatterns(t *testing.T) {
	t.Parallel()

	type compiled struct {
		pattern, pkg *regexp.Regexp
		msg          string
		raw          forbidigoEntry
	}
	var entries []compiled
	for _, e := range forbidigoEntries(t, moduleRoot(t)) {
		pattern, err := regexp.Compile(e.pattern)
		if err != nil {
			t.Fatalf(".golangci.yml:%d: pattern: %v", e.line, err)
		}
		pkg, err := regexp.Compile(e.pkg)
		if err != nil {
			t.Fatalf(".golangci.yml:%d: pkg: %v", e.line, err)
		}
		entries = append(entries, compiled{pattern, pkg, e.msg, e})
	}
	refusedBy := func(pkg, name string) []string {
		var msgs []string
		for _, e := range entries {
			if strings.HasPrefix(e.msg, "cloud-read-only: ") && e.pkg.MatchString(pkg) && e.pattern.MatchString(name) {
				msgs = append(msgs, e.msg)
			}
		}
		return msgs
	}

	for _, sdk := range readOnlySDKs {
		t.Run(sdk.name, func(t *testing.T) {
			t.Parallel()

			for _, op := range cloudWrites {
				for _, s := range sdk.symbols(op) {
					msgs := refusedBy(sdk.pkg, s.name)
					if !slices.ContainsFunc(msgs, func(m string) bool { return strings.HasPrefix(m, s.entry) }) {
						t.Errorf("%s (%s) is a write, but no entry with msg %q refuses it; refused by %q", s.name, sdk.pkg, s.entry, msgs)
					}
				}
			}
			for _, op := range sdk.reads {
				for _, s := range sdk.symbols(op) {
					if msgs := refusedBy(sdk.pkg, s.name); len(msgs) > 0 {
						t.Errorf("%s (%s) is a read, but %q refuses it", s.name, sdk.pkg, msgs)
					}
				}
			}
		})
	}

	t.Run("denylist", func(t *testing.T) {
		t.Parallel()

		// The allowlist entries are checked above, and the entry for an
		// adapter's own types by TestWriteVerbsMatchForbidigo.
		allowlist := []string{"cloud-read-only: a source adapter calls no method"}
		for _, sdk := range readOnlySDKs {
			for _, s := range sdk.symbols("Create") {
				allowlist = append(allowlist, s.entry)
			}
		}

		type pkgName struct{ pkg, name string }
		denied := map[int][]pkgName{} // by index into entries
		for _, d := range cloudDenied {
			i := slices.IndexFunc(entries, func(e compiled) bool { return strings.HasPrefix(e.msg, d.entry) })
			if i < 0 {
				t.Errorf("cloudDenied: .golangci.yml has no forbidigo entry whose msg starts with %q", d.entry)
				continue
			}
			for _, name := range d.names {
				if e := entries[i]; !e.pkg.MatchString(d.pkg) || !e.pattern.MatchString(name) {
					t.Errorf(".golangci.yml:%d: the entry with msg %q does not refuse %s (%s)", e.raw.line, d.entry, name, d.pkg)
				}
				denied[i] = append(denied[i], pkgName{d.pkg, name})
			}
		}

		for i, e := range entries {
			if !strings.HasPrefix(e.msg, "cloud-read-only: ") ||
				slices.ContainsFunc(allowlist, func(prefix string) bool { return strings.HasPrefix(e.msg, prefix) }) {
				continue
			}
			names := denied[i]
			if len(names) == 0 {
				t.Errorf(".golangci.yml:%d: the entry with msg %q has no names in cloudDenied", e.raw.line, e.msg)
				continue
			}
			for _, m := range regexpMutants(t, e.raw.pattern) {
				if !slices.ContainsFunc(names, func(n pkgName) bool { return !m.re.MatchString(n.name) }) {
					t.Errorf(".golangci.yml:%d: with %s dropped from pattern, the entry still refuses every name cloudDenied gives it; add a name that needs it\npattern: %s\nmutant:  %s", e.raw.line, m.dropped, e.raw.pattern, m.re)
				}
			}
			for _, m := range regexpMutants(t, e.raw.pkg) {
				if !slices.ContainsFunc(names, func(n pkgName) bool { return !m.re.MatchString(n.pkg) }) {
					t.Errorf(".golangci.yml:%d: with %s dropped from pkg, the entry still covers every package cloudDenied gives it; add a name from a package that needs it\npkg:    %s\nmutant: %s", e.raw.line, m.dropped, e.raw.pkg, m.re)
				}
			}
		}

		for _, r := range cloudDenylistReads {
			for _, name := range r.names {
				if msgs := refusedBy(r.pkg, name); len(msgs) > 0 {
					t.Errorf("%s (%s) is a read, but %q refuses it", name, r.pkg, msgs)
				}
			}
		}
	})
}

// regexpMutant is a regexp with one part of it dropped.
type regexpMutant struct {
	re      *regexp.Regexp
	dropped string // the part, as regexp/syntax prints it
}

// regexpMutants returns pattern once with each branch of each alternation
// dropped, and once with each optional part dropped. regexp/syntax factors a
// common prefix out of an alternation, so a branch is one of the parsed tree:
// in GetParameter\w*|GetId, the branches are Parameter\w* and Id after Get.
func regexpMutants(t *testing.T, pattern string) []regexpMutant {
	t.Helper()

	tree, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		t.Fatalf("%s: %v", pattern, err)
	}
	var mutants []regexpMutant
	for _, m := range dropParts(tree) {
		re, err := regexp.Compile(m.tree.String())
		if err != nil {
			t.Fatalf("%s with %s dropped: %v", pattern, m.dropped, err)
		}
		mutants = append(mutants, regexpMutant{re, m.dropped})
	}
	return mutants
}

type treeMutant struct {
	tree    *syntax.Regexp
	dropped string
}

// dropParts returns copies of re, each with one alternation branch or one
// optional part (x?, x*, x{0,n}) dropped. re itself is not changed.
func dropParts(re *syntax.Regexp) []treeMutant {
	var mutants []treeMutant
	switch re.Op {
	case syntax.OpAlternate:
		for i, sub := range re.Sub {
			alt := *re
			alt.Sub = slices.Delete(slices.Clone(re.Sub), i, i+1)
			mutants = append(mutants, treeMutant{&alt, sub.String()})
		}
	case syntax.OpQuest, syntax.OpStar:
		mutants = append(mutants, treeMutant{&syntax.Regexp{Op: syntax.OpEmptyMatch}, re.String()})
	case syntax.OpRepeat:
		if re.Min == 0 {
			mutants = append(mutants, treeMutant{&syntax.Regexp{Op: syntax.OpEmptyMatch}, re.String()})
		}
	default:
		// A literal or a class is not dropped: every name needs it.
		// syntax.Parse turns an alternation of single letters into a class,
		// (?:I|L) into [IL] and URI|URL into UR[IL], so such an alternation
		// is not mutated either: write each branch as more than one letter,
		// or pin the letters with names some other way.
	}
	for i, sub := range re.Sub {
		for _, m := range dropParts(sub) {
			parent := *re
			parent.Sub = slices.Clone(re.Sub)
			parent.Sub[i] = m.tree
			mutants = append(mutants, treeMutant{&parent, m.dropped})
		}
	}
	return mutants
}

// TestCloudReadOnlyPackages checks which import paths the allowlist entries of
// forbidigo's cloud-read-only group take: every package of an SDK whose clients
// they cover, and no types package beneath one, which holds data, not calls.
func TestCloudReadOnlyPackages(t *testing.T) {
	t.Parallel()

	byMsg := map[string]*regexp.Regexp{}
	for _, e := range forbidigoEntries(t, moduleRoot(t)) {
		byMsg[e.msg] = regexp.MustCompile(e.pkg)
	}
	pkgOf := func(prefix string) *regexp.Regexp {
		for msg, pkg := range byMsg {
			if strings.HasPrefix(msg, prefix) {
				return pkg
			}
		}
		t.Fatalf(".golangci.yml has no forbidigo entry whose msg starts with %q", prefix)
		return nil
	}

	tests := []struct {
		entry   string
		covers  []string
		ignores []string
	}{
		{
			entry: "cloud-read-only: an AWS SDK client allows",
			covers: []string{
				"github.com/aws/aws-sdk-go-v2/service/ec2", "github.com/aws/aws-sdk-go-v2/service/sqs/v2",
				"github.com/aws/aws-sdk-go-v2/feature/s3/manager", "github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue",
			},
			ignores: []string{"github.com/aws/aws-sdk-go-v2/service/ec2/types", "github.com/aws/aws-sdk-go-v2/aws"},
		},
		{
			entry:   "cloud-read-only: an Azure SDK client allows",
			covers:  []string{"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5", "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"},
			ignores: []string{"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2021-03-01/compute"},
		},
		{
			entry:  "cloud-read-only: a GCP SDK client allows",
			covers: []string{"cloud.google.com/go/compute/apiv1", "cloud.google.com/go/resourcemanager/apiv3"},
		},
		{
			entry:   "cloud-read-only: only the request or response of a GCP",
			covers:  []string{"cloud.google.com/go/compute/apiv1/computepb", "cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"},
			ignores: []string{"cloud.google.com/go/compute/apiv1"},
		},
		{
			entry:   "cloud-read-only: use cloud.google.com/go",
			covers:  []string{"google.golang.org/api/compute/v1", "google.golang.org/api/admin/directory/v1", "google.golang.org/api/merchantapi/products_v1beta"},
			ignores: []string{"google.golang.org/api/option", "google.golang.org/api/iterator", "google.golang.org/api/googleapi"},
		},
		{
			entry:   "cloud-read-only: use aws-sdk-go-v2 or the Azure SDK",
			covers:  []string{"github.com/aws/aws-sdk-go/service/ec2", "github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2021-03-01/compute", "github.com/Azure/azure-sdk-for-go/storage"},
			ignores: []string{"github.com/aws/aws-sdk-go-v2/service/ec2", "github.com/Azure/azure-sdk-for-go/sdk/azcore"},
		},
		{
			entry:   "cloud-read-only: a source adapter calls no method",
			covers:  []string{"github.com/timothymamo/shift2eu/internal/adapter/source/aws", "github.com/timothymamo/shift2eu/internal/adapter/source/aws_test", "github.com/timothymamo/shift2eu/internal/adapter/source/awsutil"},
			ignores: []string{"github.com/timothymamo/shift2eu/internal/adapter/target/ovhcloud"},
		},
	}
	for _, tc := range tests {
		pkg := pkgOf(tc.entry)
		for _, p := range tc.covers {
			if !pkg.MatchString(p) {
				t.Errorf("the entry with msg %q does not cover %s", tc.entry, p)
			}
		}
		for _, p := range tc.ignores {
			if pkg.MatchString(p) {
				t.Errorf("the entry with msg %q covers %s, which it should leave out", tc.entry, p)
			}
		}
	}
}

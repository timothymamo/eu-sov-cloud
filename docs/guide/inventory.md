# Build an inventory from your cloud

Turn your AWS, GCP or Azure estate into an `inventory.json` that `shift2eu assess` reads, using only
the provider's own command-line tool, read-only credentials and `jq`. You run a handful of list and
describe calls, save their output in one directory, and run one `jq` program from this repository
over it. This is an interim path: `shift2eu discover`, which will do the same from inside the tool,
does not exist yet.

## Before you start

You need:

- the provider's CLI (`aws` version 2, `gcloud`, or `az` with the `resource-graph` extension),
  signed in the way you normally sign in. Every command below uses the CLI's default credential
  chain; nothing here asks you to print, paste or store a credential;
- `jq` (the programs are tested with jq 1.8);
- `shift2eu` ([Install](install.md)), or a clone with `make build`;
- the programs from this repository, `docs/guide/inventory/aws.jq`, `gcp.jq` and `azure.jq`. A
  release archive ships them under `docs/guide/inventory/` too.

Every command on this page reads: it lists, describes, gets, searches or queries, and never creates,
updates, deletes or tags anything. Where a provider's inventory service must be switched on first
(AWS Config, the GCP Cloud Asset API), this page says so and leaves that to your cloud administrator.

## The inventory format in brief

An inventory is a JSON object with a version, the source and a list of resources.
[ADR 0006](../adr/0006-json-inventory-file-format-v1.md) records the format in full; the reader is
strict, so a misspelt key or an unknown value is an error, never ignored.

```json
{
  "schemaVersion": 1,
  "source": "aws",
  "resources": [
    {
      "id": "shop-db",
      "rawType": "AWS::RDS::DBInstance",
      "category": "database.postgresql",
      "region": "eu-central-1",
      "capabilities": ["point-in-time-restore", "zone-high-availability"]
    },
    { "id": "shop-cdn", "rawType": "AWS::CloudFront::Distribution", "category": "unclassified" }
  ]
}
```

| Field | Required | Meaning |
|---|---|---|
| `schemaVersion` | yes | Always `1` |
| `source` | yes | `aws`, `gcp` or `azure` here |
| `resources[].id` | yes, unique | A short name for the resource |
| `resources[].category` | yes | A taxonomy category, below |
| `resources[].rawType` | no | The source's own type name |
| `resources[].region` | no | The source region; absent for a global resource |
| `resources[].capabilities` | no | Capabilities the resource depends on, below |

The **taxonomy categories** are the only bridge between a source and a target: `compute.vm`,
`kubernetes.managed`, `storage.object`, `storage.block`, `database.postgresql`, `database.mysql`,
`serverless.function` and `messaging.queue`. A resource that fits none of them is `unclassified`
and keeps its `rawType`: it then shows up in the report as a blocker on every target, instead of
disappearing from the assessment.

The **capabilities** are features a resource depends on:

| Capability | Meaning |
|---|---|
| `ipv6` | Reachable over IPv6 and can reach IPv6 destinations |
| `point-in-time-restore` | Data can be restored to any moment in a retention window |
| `object-versioning` | Earlier versions of overwritten or deleted objects are kept |
| `node-high-availability` | Keeps serving through the loss of one node, without manual failover |
| `zone-high-availability` | Keeps serving through the loss of a zone; implies `node-high-availability` |
| `read-replicas` | Reads are served from replicas of a primary database |
| `autoscaling` | Capacity is added and removed with load |
| `encryption-at-rest` | Stored data is encrypted |
| `snapshots` | Point-in-time copies of a volume or machine are taken |
| `dead-letter-queue` | Messages that keep failing are set aside |

A capability changes the verdict: an offering that serves the category but lacks a capability the
resource lists is a partial fit, not a direct one, and the report names the missing capability. A
capability left out can therefore make a move look easier than it is, which is why the programs
derive them from the resources' settings rather than leaving them out.

## How the programs work

There is one program per source under `docs/guide/inventory/`: `aws.jq`, `gcp.jq` and `azure.jq`.
For each source you:

1. run the source's commands below, which write their JSON output into one directory
   (`aws-inventory/<region>/`, `gcp-inventory/` or `azure-inventory/`);
2. run the source's program once, with `jq -n`, over every file in that directory, and redirect the
   result to `inventory.json`.

A program recognises each input by its shape, not by its file name, so the file names below are only
a convention, and a file may hold several JSON documents one after the other (the loops below write
them that way). The program header lists the shapes it reads.

The programs classify by the tables in each source's section: a `rawType` becomes a taxonomy
category, and a setting becomes a capability. They state how a source's resources map to the
taxonomy, nothing else; which target offers what is the catalog's business, decided by `assess`.

The programs write short IDs (an instance ID, a bucket, database or queue name), never an ARN, a full
GCP resource name or an Azure resource ID, which carry the account, project or subscription. Two
resources with the same short name, such as a function and a queue both called `orders`, get their
type and region appended (`orders (AWS::SQS::Queue, eu-central-1)`) so every ID stays unique.

The commands are written for a POSIX shell (bash, zsh).

## AWS

### AWS permissions

The AWS managed policy `ReadOnlyAccess` covers every call. It also lets its holder read your data
(objects in buckets, for example), so prefer a policy, created by your administrator, that allows
only the calls this page makes:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeInstances",
        "ec2:DescribeVolumes",
        "ec2:DescribeSnapshots",
        "autoscaling:DescribeAutoScalingGroups",
        "s3:ListAllMyBuckets",
        "s3:GetBucketVersioning",
        "rds:DescribeDBInstances",
        "eks:ListClusters",
        "eks:DescribeCluster",
        "eks:ListNodegroups",
        "eks:DescribeNodegroup",
        "lambda:ListFunctions",
        "sqs:ListQueues",
        "sqs:GetQueueAttributes",
        "config:SelectResourceConfig"
      ],
      "Resource": "*"
    }
  ]
}
```

`ViewOnlyAccess` lists resources but is not meant to grant every `Get` call above; check it against
the list before you rely on it.

### Collect from AWS

Run these once per region you use, with the region in `AWS_REGION`. Each region's output goes to a
directory of its own:

```bash
export AWS_REGION=eu-central-1
out="aws-inventory/$AWS_REGION"
mkdir -p "$out"

aws ec2 describe-instances --output json > "$out/ec2-instances.json"
aws ec2 describe-volumes --output json > "$out/ec2-volumes.json"
aws ec2 describe-snapshots --owner-ids self --output json > "$out/ec2-snapshots.json"
aws autoscaling describe-auto-scaling-groups --output json > "$out/autoscaling-groups.json"

aws s3api list-buckets --output json > "$out/s3-buckets.json"
jq -r '.Buckets[].Name' "$out/s3-buckets.json" | while read -r bucket; do
  aws s3api get-bucket-versioning --bucket "$bucket" --output json |
    jq --arg bucket "$bucket" '{Bucket: $bucket, Versioning: .}'
done > "$out/s3-versioning.json"

aws rds describe-db-instances --output json > "$out/rds-instances.json"

aws eks list-clusters --output json | jq -r '.clusters[]' | while read -r cluster; do
  aws eks describe-cluster --name "$cluster" --output json
  aws eks list-nodegroups --cluster-name "$cluster" --output json | jq -r '.nodegroups[]' |
    while read -r nodegroup; do
      aws eks describe-nodegroup --cluster-name "$cluster" --nodegroup-name "$nodegroup" --output json
    done
done > "$out/eks.json"

aws lambda list-functions --output json > "$out/lambda-functions.json"

aws sqs list-queues --output json | jq -r '.QueueUrls[]?' | while read -r queue; do
  aws sqs get-queue-attributes --queue-url "$queue" --attribute-names All --output json
done > "$out/sqs-queues.json"

# Optional: everything else, from AWS Config (see "Everything else on AWS").
aws configservice select-resource-config \
  --expression "SELECT resourceId, resourceName, resourceType, awsRegion" \
  --output json > "$out/config.json"
```

S3 is global: `list-buckets` returns every bucket from any region, and the program lists each bucket
once. A bucket's region comes from the `BucketRegion` field, which recent versions of the AWS CLI
version 2 print; with an older CLI, buckets have no region.

### Build the AWS inventory

```bash
jq -n -f docs/guide/inventory/aws.jq aws-inventory/*/*.json > inventory.json
```

### AWS classification

The `rawType` is the resource's AWS CloudFormation type.

| `rawType` | Category | Read from |
|---|---|---|
| `AWS::EC2::Instance` | `compute.vm` | `ec2 describe-instances` |
| `AWS::AutoScaling::AutoScalingGroup` | `compute.vm` | `autoscaling describe-auto-scaling-groups` |
| `AWS::EC2::Volume` | `storage.block` | `ec2 describe-volumes` |
| `AWS::S3::Bucket` | `storage.object` | `s3api list-buckets` |
| `AWS::RDS::DBInstance`, engine `postgres` or `aurora-postgresql` | `database.postgresql` | `rds describe-db-instances` |
| `AWS::RDS::DBInstance`, engine `mysql` or `aurora-mysql` | `database.mysql` | `rds describe-db-instances` |
| `AWS::RDS::DBInstance`, any other engine (MariaDB, Oracle, SQL Server) | `unclassified` | `rds describe-db-instances` |
| `AWS::EKS::Cluster` | `kubernetes.managed` | `eks describe-cluster` |
| `AWS::Lambda::Function` | `serverless.function` | `lambda list-functions` |
| `AWS::SQS::Queue` | `messaging.queue` | `sqs get-queue-attributes` |
| any other type AWS Config records | `unclassified` | `configservice select-resource-config` |

Instances that are terminated or shutting down are left out, and so are instances an Auto Scaling
group launched (tag `aws:autoscaling:groupName`): the group stands for them.

| Category | Capability | When |
|---|---|---|
| `compute.vm` (instance) | `ipv6` | a network interface has an IPv6 address |
| `compute.vm` (instance) | `snapshots` | one of its volumes has a snapshot you own |
| `compute.vm` (Auto Scaling group) | `autoscaling` | `MaxSize` is greater than `MinSize` |
| `compute.vm` (Auto Scaling group) | `zone-high-availability` | it spans two or more availability zones |
| `storage.block` | `encryption-at-rest` | `Encrypted` is true |
| `storage.block` | `snapshots` | the volume has a snapshot you own |
| `storage.object` | `encryption-at-rest` | always: S3 encrypts every bucket at rest |
| `storage.object` | `object-versioning` | versioning status is `Enabled` |
| `database.*` | `point-in-time-restore` | `BackupRetentionPeriod` is above 0 |
| `database.*` | `read-replicas` | it has read replicas |
| `database.*` | `zone-high-availability` | `MultiAZ` is true |
| `database.*` | `encryption-at-rest` | `StorageEncrypted` is true |
| `kubernetes.managed` | `autoscaling` | a managed node group's `maxSize` is above its `minSize` |
| `kubernetes.managed` | `node-high-availability` | its managed node groups want two or more nodes in all |
| `messaging.queue` | `dead-letter-queue` | it has a `RedrivePolicy` |
| `messaging.queue` | `encryption-at-rest` | SQS-managed or KMS server-side encryption is on |

### Everything else on AWS

The service calls above cover what the taxonomy classifies. Everything else (CloudFront
distributions, Route 53 zones, load balancers, and also networking and IAM resources) should show up
as `unclassified`, so the report shows what has no place to go. Two AWS services list everything:

- **AWS Config** (`configservice select-resource-config`, an SQL-like read-only query). Its
  `resourceType` is the CloudFormation type, the same convention as the table above, so the program
  reads it: every type the table does not cover becomes `unclassified`, and the types it does cover
  are dropped, as the service calls already list them. Config only knows what its recorder records,
  per region, and the recorder must be switched on by your administrator. With a Config aggregator,
  `select-aggregate-resource-config` queries every account and region at once; its output has the
  same shape.
- **Resource Explorer** (`resource-explorer-2 search` or `list-resources`) lists the resources of
  every region from one aggregator index, but names types its own way (`cloudfront:distribution`),
  not as CloudFormation types, so the program does not read it. It is still a quick way to see what
  an account holds; its index must be created by your administrator.

Without `config.json`, the inventory holds only the resources the service calls list.

## GCP

### GCP permissions

On the project: `roles/cloudasset.viewer` (Cloud Asset Inventory) and `roles/viewer` (the
per-service lists). The Cloud Asset API (`cloudasset.googleapis.com`) must be enabled on the project;
if it is not, ask your administrator to enable it.

### Collect from GCP

```bash
project=example-project   # your project ID
out=gcp-inventory
mkdir -p "$out"

gcloud asset search-all-resources --scope="projects/$project" --format=json > "$out/assets.json"
gcloud compute instances list --project="$project" --format=json > "$out/compute-instances.json"
gcloud compute instance-groups managed list --project="$project" --format=json > "$out/compute-instance-groups.json"
gcloud compute snapshots list --project="$project" --format=json > "$out/compute-snapshots.json"
gcloud sql instances list --project="$project" --format=json > "$out/sql-instances.json"
gcloud storage buckets list --project="$project" --raw --format=json > "$out/storage-buckets.json"
gcloud container clusters list --project="$project" --format=json > "$out/container-clusters.json"
gcloud pubsub subscriptions list --project="$project" --format=json > "$out/pubsub-subscriptions.json"
```

The Cloud Asset Inventory listing is the list of resources; the other calls add the settings the
capabilities need. A resource the other calls do not cover still appears, as `unclassified` or
without capabilities. For several projects, collect each into a directory of its own and build one
inventory per project.

### Build the GCP inventory

```bash
jq -n -f docs/guide/inventory/gcp.jq gcp-inventory/*.json > inventory.json
```

### GCP classification

The `rawType` is the resource's Cloud Asset Inventory asset type.

| `rawType` | Category |
|---|---|
| `compute.googleapis.com/Instance` | `compute.vm` |
| `compute.googleapis.com/InstanceGroupManager` | `compute.vm` |
| `compute.googleapis.com/Disk` | `storage.block` |
| `storage.googleapis.com/Bucket` | `storage.object` |
| `sqladmin.googleapis.com/Instance`, `databaseVersion` `POSTGRES_*` | `database.postgresql` |
| `sqladmin.googleapis.com/Instance`, `databaseVersion` `MYSQL_*` | `database.mysql` |
| `sqladmin.googleapis.com/Instance`, any other engine, or not in `sql instances list` | `unclassified` |
| `container.googleapis.com/Cluster` | `kubernetes.managed` |
| `cloudfunctions.googleapis.com/Function`, `cloudfunctions.googleapis.com/CloudFunction` | `serverless.function` |
| `pubsub.googleapis.com/Topic` | `messaging.queue` |
| `pubsub.googleapis.com/Subscription` | `messaging.queue` |
| `run.googleapis.com/Service` | `unclassified` |
| any other asset type | `unclassified` |

A Pub/Sub topic (where publishers write) and a subscription (where consumers read) are both listed,
as both have to move. Left out: instances a managed instance group created (metadata `created-by`;
the group stands for them), the Cloud Run service behind a 2nd gen Cloud Function (label
`goog-managed-by: cloudfunctions`; the function stands for it), and snapshots and autoscalers, which
become capabilities of their disk or group. Any other Cloud Run service runs a container, not a
function, and stays `unclassified`.

| Category | Capability | When |
|---|---|---|
| `compute.vm` (instance) | `ipv6` | a network interface has stack type `IPV4_IPV6` or an IPv6 access config |
| `compute.vm` (managed instance group) | `autoscaling` | an autoscaler is attached (`status.autoscaler`) |
| `compute.vm` (managed instance group) | `zone-high-availability` | the group is regional |
| `storage.block` | `encryption-at-rest` | always: Google Cloud encrypts all data at rest |
| `storage.block` | `snapshots` | a snapshot's `sourceDisk` is the disk |
| `storage.object` | `encryption-at-rest` | always |
| `storage.object` | `object-versioning` | `versioning.enabled` is true |
| `database.*` | `encryption-at-rest` | always |
| `database.*` | `point-in-time-restore` | `pointInTimeRecoveryEnabled` is true, or for MySQL `binaryLogEnabled` |
| `database.*` | `zone-high-availability` | `availabilityType` is `REGIONAL` |
| `database.*` | `read-replicas` | `replicaNames` is not empty |
| `kubernetes.managed` | `zone-high-availability` | the cluster is regional |
| `kubernetes.managed` | `node-high-availability` | it runs two or more nodes |
| `kubernetes.managed` | `autoscaling` | Autopilot, or a node pool with autoscaling |
| `messaging.queue` (subscription) | `dead-letter-queue` | it has a dead-letter topic |

## Azure

### Azure permissions

The built-in `Reader` role on the subscription. It covers Resource Graph and the two management-plane
reads below; none of the commands reads a storage account key or any data.

### Collect from Azure

Resource Graph returns at most 1,000 rows per call, so the loop below fetches page after page until
no skip token is left. It reads the subscription the CLI is set to (`az account show`):

```bash
subscription="$(az account show --query id --output tsv)"
out=azure-inventory
mkdir -p "$out"

query="Resources | project id, name, type, kind, location, zones, properties"
page=1
token=""
while :; do
  if [ -z "$token" ]; then
    az graph query --graph-query "$query" --subscriptions "$subscription" --first 1000 \
      --output json > "$out/graph-$page.json"
  else
    az graph query --graph-query "$query" --subscriptions "$subscription" --first 1000 \
      --skip-token "$token" --output json > "$out/graph-$page.json"
  fi
  token="$(jq -r '.skip_token // empty' "$out/graph-$page.json")"
  [ -n "$token" ] || break
  page=$((page + 1))
done

az servicebus namespace list --query "[].[name, resourceGroup]" --output tsv |
  while read -r name group; do
    az servicebus queue list --namespace-name "$name" --resource-group "$group" --output json
  done > "$out/servicebus-queues.json"

az storage account list --query "[].[name, resourceGroup]" --output tsv |
  while read -r name group; do
    az storage account blob-service-properties show --account-name "$name" \
      --resource-group "$group" --output json
  done > "$out/blob-services.json"
```

Resource Graph holds most settings; Service Bus queues and a storage account's blob versioning are
not in it, hence the two loops. Storage queues are read through the data plane, which `Reader` cannot
do, so they are not collected: add them by hand if you use them.

### Build the Azure inventory

```bash
jq -n -f docs/guide/inventory/azure.jq azure-inventory/*.json > inventory.json
```

### Azure classification

The `rawType` is the resource's Azure Resource Manager type, taken from its resource ID (Resource
Graph writes types in lower case).

| `rawType` | Category |
|---|---|
| `Microsoft.Compute/virtualMachines` | `compute.vm` |
| `Microsoft.Compute/virtualMachineScaleSets` | `compute.vm` |
| `Microsoft.Compute/disks` | `storage.block` |
| `Microsoft.Storage/storageAccounts` (not kind `FileStorage`) | `storage.object` |
| `Microsoft.Storage/storageAccounts`, kind `FileStorage` | `unclassified` |
| `Microsoft.DBforPostgreSQL/flexibleServers` | `database.postgresql` |
| `Microsoft.DBforMySQL/flexibleServers` | `database.mysql` |
| `Microsoft.ContainerService/managedClusters` | `kubernetes.managed` |
| `Microsoft.Web/sites`, kind `functionapp` | `serverless.function` |
| `Microsoft.Web/sites`, any other kind (web app, Logic App) | `unclassified` |
| `Microsoft.ServiceBus/namespaces/queues` | `messaging.queue` |
| any other type | `unclassified` |

Left out: virtual machines of a scale set (the scale set stands for them), snapshots and autoscale
settings, which become capabilities of their disk or scale set, and a Service Bus namespace whose
queues are listed (a namespace without queues stays `unclassified`).

| Category | Capability | When |
|---|---|---|
| `compute.vm` (virtual machine) | `ipv6` | a network interface has an IPv6 IP configuration |
| `compute.vm` (virtual machine) | `snapshots` | one of its managed disks has a snapshot |
| `compute.vm` (scale set) | `autoscaling` | an enabled autoscale setting targets it |
| `compute.vm` (scale set) | `zone-high-availability` | it spans two or more zones |
| `storage.block` | `encryption-at-rest` | always: Azure encrypts every managed disk |
| `storage.block` | `snapshots` | a snapshot was taken from the disk |
| `storage.object` | `encryption-at-rest` | always: Azure encrypts every storage account |
| `storage.object` | `object-versioning` | blob versioning is enabled |
| `database.*` | `encryption-at-rest` | always |
| `database.*` | `point-in-time-restore` | `backupRetentionDays` is above 0 |
| `database.*` | `zone-high-availability` | high availability mode `ZoneRedundant` |
| `database.*` | `node-high-availability` | high availability mode `SameZone` |
| `database.*` | `read-replicas` | another server names it as its source |
| `kubernetes.managed` | `autoscaling` | a node pool has autoscaling on |
| `kubernetes.managed` | `zone-high-availability` | a node pool spans two or more zones |
| `kubernetes.managed` | `node-high-availability` | its node pools run two or more nodes in all |
| `messaging.queue` | `dead-letter-queue` | always: every Service Bus queue has a dead-letter subqueue |

## Trim what you do not want assessed

The full listings include supporting resources, such as networks, network interfaces, security
groups and IAM roles, and each becomes an `unclassified` blocker. To drop the unclassified resources
of a type, filter the inventory with `jq`; for example, every Azure networking resource:

```bash
jq 'del(.resources[] | select(.category == "unclassified" and (.rawType | startswith("Microsoft.Network/"))))' \
  inventory.json > inventory-trimmed.json
```

Edit the file by hand as well: it is plain JSON. Leave out only what really has nothing to move.

## Privacy

An inventory holds the names and IDs of resources in a real account. The programs keep it as small as
they can (short IDs, no account, project or subscription field), but names can still say a lot about
a business.

- Keep short IDs. Do not replace them with ARNs, full GCP resource names or Azure resource IDs, which
  carry the account, project or subscription ID.
- Review the file before you share it, and rename anything that should not leave your organisation.
- `shift2eu assess` runs offline: it reads the file and writes a report, and sends nothing anywhere.
- Delete the collected directory when you are done, and never commit an inventory or the raw CLI
  output from a real account to a repository (this repository's fixtures are sanitised by rule).

## Validate the result

Assess the inventory against every target:

```bash
shift2eu assess --inventory inventory.json --target all
```

`assess` refuses a file it cannot read exactly, with exit code 1 and one line that names the file
and the field by its path, for example:

```text
shift2eu: load inventory: inventory file "inventory.json": not a valid inventory document: unknown key "regoin" at "resources[0].regoin", line 1, column 82
shift2eu: load inventory: inventory file "inventory.json": resource "i-0123456789abcdef0": unknown taxonomy category "compute.mv"
shift2eu: load inventory: inventory file "inventory.json": resources[1]: duplicate resource ID "a"
```

It also refuses a file over 10 MiB, more than 50,000 resources and more than 32 capabilities on one
resource; split a larger estate into several inventories, by region or project. After a hand edit,
check the category and capability names against the lists above. [Assess an inventory](assess.md)
describes the report and the exit codes, and [Try it](try-it.md#read-the-report) how to read it.

# azure.jq turns the output of the read-only az calls in docs/guide/inventory.md
# into a shift2eu inventory file, schema version 1
# (docs/adr/0006-json-inventory-file-format-v1.md). Run it with -n on every file
# the guide's commands wrote:
#
#   jq -n -f docs/guide/inventory/azure.jq inventories/azure/*.json > inventories/azure.inventory.json
#
# The Resource Graph listing (az graph query, one file per page) is the list of
# resources and holds most details; four calls add what it lacks. Each element of
# each input is recognised by its shape, not by its file name. The guide's queries
# keep only the fields read here; the full output is read the same way:
#
#   {"data": [...]}                              az graph query, one page
#   {"type": "Microsoft.ServiceBus/Namespaces/Queues"}  az servicebus queue list
#   {"type": "Microsoft.ServiceBus/Namespaces/Topics"}  az servicebus topic list
#   {"type": "Microsoft.Storage/storageAccounts/queueServices/queues"}
#                                                az rest --method get .../queues
#   {"type": "Microsoft.Storage/storageAccounts/blobServices"}
#                                                az storage account
#                                                blob-service-properties show
#
# Resource IDs are short (the resource's name), never the full Azure resource ID,
# which carries the subscription. Anything it cannot classify is "unclassified"
# with its rawType (an Azure Resource Manager type), so it shows up as a blocker.
#
# Everything in an AKS cluster's node resource group (its scale sets, load
# balancers, network interfaces) is left out, the cluster stands for it; its
# managed disks, persistent volumes that hold data, stay.

# The Resource Manager type of a resource ID, in the casing of the ID (Resource
# Graph writes types in lower case):
# /subscriptions/s/resourceGroups/g/providers/Microsoft.ServiceBus/namespaces/n/queues/q
# -> Microsoft.ServiceBus/namespaces/queues.
def id_type:
  split("/providers/") | last | split("/") as $p
  | [$p[0]] + [range(1; $p | length; 2) as $i | $p[$i]] | join("/");

# The capability $name when the condition is true.
def cap($name; cond): if cond then $name else empty end;

# Gives every resource a distinct ID: two resources with the same short ID get
# their rawType and region appended, and any still equal a counter.
def unique_ids:
  def suffix: " (" + (.rawType // "unknown") + (if .region then ", " + .region else "" end) + ")";
  group_by(.id)
  | map(if length == 1 then . else map(.id += suffix) end)
  | add // []
  | group_by(.id)
  | map(if length == 1 then . else to_entries | map(.value.id += " #" + (.key + 1 | tostring) | .value) end)
  | add // [];

# Writes the inventory document: no empty region or capabilities, capabilities
# sorted, identical resources listed once.
def inventory($source):
  map(
    {id, rawType, category, region, capabilities}
    | if .region == null or .region == "" then del(.region) else . end
    | .capabilities = ((.capabilities // []) | unique)
    | if .capabilities == [] then del(.capabilities) else . end
  )
  | unique
  | unique_ids
  | sort_by(.id)
  | {schemaVersion: 1, source: $source, resources: .};

def lower_type: (.type // "") | ascii_downcase;
def lid: ascii_downcase;

# The resource group of a resource ID, in lower case.
def id_group: lid | split("/")[4] // "";

# The types the four calls next to Resource Graph return.
def child_types:
  "microsoft.servicebus/namespaces/queues", "microsoft.servicebus/namespaces/topics",
  "microsoft.storage/storageaccounts/queueservices/queues",
  "microsoft.storage/storageaccounts/blobservices";

[inputs | if type == "object" and (.data | type) == "array" then .data[]
          elif type == "object" and (.value | type) == "array" then .value[]
          elif type == "array" then .[] else . end
        | objects | select((.id? | type) == "string" and (.type? | type) == "string")] as $all
| [$all[] | select(lower_type == "microsoft.servicebus/namespaces/queues")] as $queues
| [$all[] | select(lower_type == "microsoft.servicebus/namespaces/topics")] as $topics
| [$all[] | select(lower_type == "microsoft.storage/storageaccounts/queueservices/queues")] as $storagequeues
| [$all[] | select(lower_type == "microsoft.storage/storageaccounts/blobservices")] as $blobs
| [$all[] | select(lower_type | IN(child_types) | not)] as $rows
| ($rows | INDEX(.id | lid)) as $byid
| ([$rows[] | select(lower_type == "microsoft.compute/snapshots")
    | .properties.creationData.sourceResourceId // empty | lid] | unique) as $snapshotted
| ([$rows[] | select(lower_type == "microsoft.insights/autoscalesettings")
    | select(.properties.enabled != false) | .properties.targetResourceUri // empty | lid] | unique) as $autoscaled
| ([$rows[] | .properties.sourceServerResourceId? // empty | lid] | unique) as $replicated
| ([$blobs[] | select(.isVersioningEnabled == true) | .id | lid | sub("/blobservices/.*$"; "")]) as $versioned
| ([$queues[], $topics[] | .id | lid | sub("/(queues|topics)/.*$"; "")] | unique) as $queuenamespaces
| ([$rows[] | select(lower_type == "microsoft.containerservice/managedclusters")
    | .properties.nodeResourceGroup // empty | ascii_downcase] | unique) as $nodegroups
| [
    # Service Bus queues. Every queue has a dead-letter subqueue.
    ($queues[]
      | (.id | lid | sub("/queues/.*$"; "")) as $ns
      | {
          id: .name,
          rawType: (.id | id_type),
          category: "messaging.queue",
          region: $byid[$ns].location,
          capabilities: ["dead-letter-queue"]
        }),

    # Service Bus topics: publish and subscribe, which no taxonomy category
    # covers, so unclassified.
    ($topics[]
      | (.id | lid | sub("/topics/.*$"; "")) as $ns
      | {
          id: .name,
          rawType: (.id | id_type),
          category: "unclassified",
          region: $byid[$ns].location
        }),

    # Storage queues. Azure encrypts every storage account at rest.
    ($storagequeues[]
      | (.id | lid | sub("/queueservices/.*$"; "")) as $account
      | {
          id: .name,
          rawType: (.id | id_type),
          category: "messaging.queue",
          region: $byid[$account].location,
          capabilities: ["encryption-at-rest"]
        }),

    ($rows[]
      | (.id | lid) as $id
      | lower_type as $t
      # Managed by AKS in a cluster's node resource group: the cluster stands
      # for it. A managed disk there is a persistent volume and stays.
      | select(($id | id_group | IN($nodegroups[])) and $t != "microsoft.compute/disks" | not)
      | {
          id: .name,
          rawType: (.id | id_type | if ascii_downcase == $t then . else $t end),
          region: (if .location == "global" then null else .location end)
        } as $base
      | if $t == "microsoft.compute/virtualmachines" then
          # A virtual machine of a scale set is left out: the scale set stands
          # for it.
          select(.properties.virtualMachineScaleSet.id? == null)
          | ([.properties.storageProfile.osDisk.managedDisk.id?,
              .properties.storageProfile.dataDisks[]?.managedDisk.id?] | map(select(.) | lid)) as $disks
          | $base + {category: "compute.vm", capabilities: [
              cap("ipv6"; any(.properties.networkProfile.networkInterfaces[]?.id | lid;
                any($byid[.].properties.ipConfigurations[]?; .properties.privateIPAddressVersion == "IPv6"))),
              cap("snapshots"; any($disks[]; IN($snapshotted[])))
            ]}
        elif $t == "microsoft.compute/virtualmachinescalesets" then
          $base + {category: "compute.vm", capabilities: [
            cap("autoscaling"; $id | IN($autoscaled[])),
            cap("zone-high-availability"; (.zones // []) | length >= 2)
          ]}
        elif $t == "microsoft.baremetalinfrastructure/baremetalinstances"
          or $t == "microsoft.azurelargeinstance/azurelargeinstances" then
          # A BareMetal Infrastructure instance or a SAP HANA Large Instance: a
          # dedicated physical server (ADR 0023).
          $base + {category: "compute.baremetal"}
        elif $t == "microsoft.compute/disks" then
          # Azure encrypts every managed disk at rest.
          $base + {category: "storage.block", capabilities: [
            "encryption-at-rest",
            cap("snapshots"; $id | IN($snapshotted[]))
          ]}
        elif $t == "microsoft.storage/storageaccounts" then
          # A FileStorage account holds file shares only. Azure encrypts every
          # storage account at rest.
          if .kind == "FileStorage" then $base + {category: "unclassified"}
          else $base + {category: "storage.object", capabilities: [
            "encryption-at-rest",
            cap("object-versioning"; $id | IN($versioned[]))
          ]} end
        elif $t == "microsoft.dbforpostgresql/flexibleservers" or $t == "microsoft.dbformysql/flexibleservers" then
          $base + {
            category: (if $t == "microsoft.dbforpostgresql/flexibleservers" then "database.postgresql" else "database.mysql" end),
            capabilities: [
              "encryption-at-rest",
              cap("point-in-time-restore"; (.properties.backup.backupRetentionDays // 0) > 0),
              cap("zone-high-availability"; .properties.highAvailability.mode? == "ZoneRedundant"),
              cap("node-high-availability"; .properties.highAvailability.mode? == "SameZone"),
              cap("read-replicas"; $id | IN($replicated[]))
            ]}
        elif $t == "microsoft.containerservice/managedclusters" then
          [.properties.agentPoolProfiles[]?] as $pools
          | $base + {category: "kubernetes.managed", capabilities: [
              cap("autoscaling"; any($pools[]; .enableAutoScaling == true)),
              cap("zone-high-availability"; any($pools[]; (.availabilityZones // []) | length >= 2)),
              cap("node-high-availability"; ([$pools[].count // 0] | add // 0) >= 2)
            ]}
        elif $t == "microsoft.web/sites" then
          # A function app; a web app or a Logic App is unclassified.
          (.kind // "" | ascii_downcase | split(",")) as $kind
          | $base + {category: (if ($kind | index("functionapp")) and ($kind | index("workflowapp") | not)
              then "serverless.function" else "unclassified" end)}
        elif $t == "microsoft.compute/snapshots" or $t == "microsoft.insights/autoscalesettings" then
          # Part of a disk's or a scale set's capabilities, not a resource to move.
          empty
        elif $t == "microsoft.servicebus/namespaces" and ($id | IN($queuenamespaces[])) then
          # Its queues and topics stand for it.
          empty
        else
          $base + {category: "unclassified"}
        end)
  ]
| inventory("azure")

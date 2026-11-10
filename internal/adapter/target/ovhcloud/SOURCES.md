# OVHcloud catalog snapshot: sources

`catalog.json` is a hand-compiled, dated snapshot of what OVHcloud **Public Cloud** offers in its EU regions,
in taxonomy terms. This file records where each fact comes from, so the snapshot can be checked and refreshed.

## How it was compiled

- **Retrieved on:** 2026-11-10. Every page cited below was fetched on that date, without credentials and
  without calling any OVHcloud API.
- **`sourceURL`** is OVHcloud's Public Cloud regions and availability page,
  <https://www.ovhcloud.com/en/public-cloud/regions-availability/>. It is the one page that lists, per
  product, the locations where it is offered, and it is where the declared regions and most offering regions
  come from. On it, a tick marks a product as available in a location, and a separate "Β" mark (legend
  "Beta version") marks a beta. No cell of the EMEA tables carried the beta mark on 2026-11-10, so every
  tick relied on below is a generally available product.
- **Product pages** on `www.ovhcloud.com` and **documentation** on `docs.ovhcloud.com` give the
  capabilities. Each was read live on the retrieval date.
- **Documentation source.** `docs.ovhcloud.com` is built from
  [`ovh/ovhcloud-docs`](https://github.com/ovh/ovhcloud-docs). Its files were read at commit
  `2df4cc152902b880880b77955ab9e52de4a2c2ff` (branch `develop`, 2026-11-10) to find the relevant pages, and
  every fact used was then checked on the published page cited. The repository named in PRD #11,
  [`ovh/docs`](https://github.com/ovh/docs), is archived: its README at commit
  `989d55b1b8f7ca0d8c39280cb469e4d40513f18d` (2026-09-01) says the documentation has moved to
  `ovh/ovhcloud-docs` and `docs.ovhcloud.com`. No fact was taken from `ovh/docs`.
- **Roadmap.** OVHcloud's public roadmap, [`ovh/public-cloud-roadmap`](https://github.com/ovh/public-cloud-roadmap),
  was read for the rollout state of one feature (Block Storage encryption in Warsaw).

## Licences of the sources

- **`www.ovhcloud.com`** (product and regions pages): © OVH SAS, under the site's legal notices. No open
  licence is granted.
- **`docs.ovhcloud.com`** and its source **`ovh/ovhcloud-docs`**: no licence is stated. The repository has no
  licence file at the commit above, its README states none, and GitHub detects none. So the content is
  under OVH SAS's copyright with all rights reserved.
- **`ovh/docs`** (archived, not used for facts) is licensed CC BY-NC-SA 4.0 (Creative Commons
  Attribution-NonCommercial-ShareAlike 4.0 International), as its `LICENSE` file and README state at the
  commit above.
- **`ovh/public-cloud-roadmap`** issues: no licence is stated. Only an issue's open or closed state is used.
- **Rule for this file and `catalog.json`:** take facts only (which product exists, in which locations, with
  which feature) and brief quotations, each in quotation marks and attributed to the page it comes from.
  Paraphrase everything else. Never copy documentation or web text in bulk, and keep it out of
  `catalog.json`, which holds only names, taxonomy terms and region codes. A refresh follows the same rule.

## Rules applied

- **Scope:** OVHcloud Public Cloud only (PRD #11). Bare Metal dedicated servers, Hosted Private Cloud, Web
  Cloud, Local Zones and the SecNumCloud platform are out of scope.
- **Regions:** one code per location, the code the regions page prints under each location (PRD #11,
  *one region name per location*). Only locations in EU member states are declared. Products name their
  regions differently; *Declared EU regions* below records which product-level names each code covers.
  An offering lists a region when the regions page ticks the product there, unless a product page narrows
  it, which is noted.
- **1-AZ and 3-AZ regions:** Paris and Milan are 3-AZ regions (three availability zones); the others are
  1-AZ regions. OVHcloud sets a product's deployment mode by the region: Object Storage, Managed Kubernetes
  and Public Cloud Databases deployed in Paris or Milan are 3-AZ deployments. Where a product is highly
  available across zones only in the 3-AZ regions, the snapshot lists a variant named "<offering> 3-AZ"
  in `MIL` and `PAR` only, beside the offering in the 1-AZ regions (PRD #11, *a capability limited to some
  regions makes a separate offering*). For the same reason, a capability missing from some of a product's
  regions makes a separate offering (Block Storage LUKS).
- **Capabilities:** only from the taxonomy vocabulary, each as the taxonomy defines it, and only when an
  OVHcloud page describes the feature as available. A capability that needs a particular plan or class is
  marked **Needs:** on its line. A feature with no vocabulary entry is listed under *Left out on purpose*.
- **`high-availability`:** the taxonomy defines it as surviving the loss of a node or zone without manual
  failover. Each offering's section says which, as **Scope: node-level** or **Scope: zone-level**. It is
  counted where OVHcloud documents a mechanism that keeps the service running through such a loss: standby
  nodes with automatic failover, or data and control plane spread across zones. Redundant copies of data
  inside one zone (replication or erasure coding), which every storage offering in a category has, are
  durability and are not counted, as in the Scaleway snapshot (Standard One Zone object storage, Block
  Storage). OVHcloud's own pages likewise reserve "high availability" for 3-AZ deployments of storage.

## Declared EU regions

`GRA` (Gravelines, France), `LIM` (Frankfurt/Limburg, Germany), `MIL` (Milan, Italy), `PAR` (Paris, France),
`RBX` (Roubaix, France), `SBG` (Strasbourg, France), `WAW` (Warsaw, Poland).

The regions page's EMEA tables list eight locations on 2026-11-10: Strasbourg `SBG`, Gravelines `GRA`,
Roubaix `RBX`, "3-AZ Paris" `PAR`, "3-AZ Milan" `MIL`, London `ERI`, Frankfurt `LIM` and Warsaw `WAW`, plus a
"Local Zones" column with no location named. The column for `LIM` is headed "Frankfurt" in most tables and
"Limburg" in the identity table. London (`ERI`, Erith) is in the United Kingdom, which is not an EU member
state, so it is not declared. The North America (Beauharnois, Toronto, Seattle, Washington DC) and Asia
Pacific (Singapore, Sydney, Mumbai) tabs list no EU location. Local Zones are excluded (PRD #11).

Product-level names each code covers:

| Code | Compute (OpenStack regions) | Object Storage endpoint | Databases | Managed Kubernetes |
|---|---|---|---|---|
| `GRA` | GRA1, GRA3, GRA5, GRA7, GRA9, GRA11 | `gra` | GRA | GRA5, GRA7, GRA9, GRA11 |
| `LIM` | DE1 | `de` | DE | DE1 |
| `MIL` | EU-SOUTH-MIL | `eu-south-mil` | EU-SOUTH-MIL | EU-SOUTH-MIL |
| `PAR` | EU-WEST-PAR | `eu-west-par` | EU-WEST-PAR | EU-WEST-PAR |
| `RBX` | RBX-A | `rbx` | RBX | EU-WEST-RBX |
| `SBG` | SBG5, SBG7 | `sbg` | SBG | SBG5 |
| `WAW` | WAW1 | `waw` | WAW | WAW1 |

Sources: the regions page footnotes (GRA and SBG OpenStack regions); the Object Storage endpoints page
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-location>; the database backups page
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/backups> (service locations); the Managed
Kubernetes regions page
<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/datacenters-nodes-storage-flavors>;
the Block Storage encryption page
<https://docs.ovhcloud.com/en/guides/public-cloud/compute/encrypting-a-block-storage-volume-with-luks>
(DE1, WAW1, RBX-A, EU-WEST-PAR and EU-SOUTH-MIL as OpenStack regions).

## Offerings

### Block Storage

Category `storage.block`: Block Storage volumes in the 1-AZ regions (the Classic, High Speed and High Speed
Gen2 classes, unencrypted). Pages: <https://www.ovhcloud.com/en/public-cloud/block-storage/>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/block-storage/block-storage-the-right-storage-class>,
<https://docs.ovhcloud.com/en/guides/public-cloud/compute/storage-create-volume-snapshot>.

- `snapshots`: volume snapshots, "a recovery point stored within the same storage cluster as the original
  volume", taken without detaching the volume (volume snapshot page). The regions page ticks "Volume
  Snapshot" in all five regions below.
- No `high-availability`: the classes in 1-AZ regions replicate data within one zone (storage class page,
  "Zonal" replication), which is durability under the rules above.
- Regions `GRA`, `LIM`, `RBX`, `SBG`, `WAW`: the regions page ticks the three Block Storage classes and
  "Volume Snapshot" in each. Volumes in `PAR` and `MIL` are under *Block Storage 3-AZ*.

### Block Storage 3-AZ

Category `storage.block`: the Regional Classic Volume class, offered only in 3-AZ regions. Pages:
<https://www.ovhcloud.com/en/public-cloud/block-storage/>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/block-storage/block-storage-the-right-storage-class>,
<https://docs.ovhcloud.com/en/guides/public-cloud/compute/storage-classic-multi-attach-3az>.

- `high-availability`. **Needs:** the Regional Classic Volume class (volume type `classic-multiattach`,
  "Classic 3AZ" in the console). **Scope: zone-level.** The class replicates data "across three
  availability zones within a region" with a 99.99% SLA, and can be attached at once to instances in
  different zones (storage class page; up to 16 instances, multi-attach page). The product page says high availability is
  guaranteed in 3-AZ regions with this class. The zonal High Speed volumes also offered in these regions do
  not have this capability.
- No `snapshots`: the regions page does not tick "Volume Snapshot" or "Volume Backup" in Paris or Milan.
- No `encryption-at-rest`: multi-attach volumes cannot be encrypted (multi-attach page, *Limitations*; storage
  class page: Regional Classic does not support LUKS). See *Left out on purpose*.
- Regions `MIL`, `PAR`: the 3-AZ regions; the product page offers Regional Classic Volume "in 3-AZ regions
  only", and the regions page ticks the Block Storage classes in both.

### Block Storage LUKS

Category `storage.block`: the encrypted variants of the Block Storage classes (volume types
`classic-luks`, `high-speed-luks`, `high-speed-gen2-luks`), listed apart because they are not offered in
every 1-AZ region. Pages: <https://docs.ovhcloud.com/en/guides/public-cloud/compute/encrypting-a-block-storage-volume-with-luks>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/block-storage/block-storage-the-right-storage-class>,
<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/datacenters-nodes-storage-flavors>,
<https://docs.ovhcloud.com/en/guides/public-cloud/compute/storage-create-volume-snapshot>.

- `encryption-at-rest`: volumes "encrypted at rest using LUKS", with an OVHcloud Managed Key by default or
  a Customer Managed Key in OKMS (encryption page). The storage class page says each class has an encrypted
  version "depending on the region".
- `snapshots`: volume snapshots as for *Block Storage*; the encryption page warns that deleting the key
  makes the volume "and all its snapshots" unrecoverable, so encrypted volumes have snapshots.
- Regions `GRA`, `LIM`, `RBX`, `SBG`: the Managed Kubernetes regions page lists the regions where LUKS
  encrypted storage "is currently available": RBX, SBG, GRA, EU-WEST-PAR, DE1 and EU-SOUTH-MIL in the EU.
  OVHcloud's roadmap closed the rollouts for GRA and DE1 on 2026-02-19
  (<https://github.com/ovh/public-cloud-roadmap/issues/951>,
  <https://github.com/ovh/public-cloud-roadmap/issues/952>). Warsaw is not listed, and its rollout is still
  open (<https://github.com/ovh/public-cloud-roadmap/issues/954>). Paris and Milan are left out because
  volume snapshots are not offered there (see *Block Storage 3-AZ*).

### Instances

Category `compute.vm`: Public Cloud virtual machine instances ("Virtual Machine Instances": General Purpose,
Compute and Memory optimized, Discovery, GPU and Storage optimized ranges). Pages:
<https://www.ovhcloud.com/en/public-cloud/virtual-instances/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/network-services/ipv6-configuration>,
<https://docs.ovhcloud.com/en/guides/public-cloud/compute/getting-started>,
<https://docs.ovhcloud.com/en/guides/public-cloud/compute/save-an-instance>.

- `ipv6`: each Public Cloud instance "is delivered with an IPv4 address and an IPv6 address" (IPv6 page),
  through a Basic IP. Caveats on the same page: Floating IP and Gateway do not support IPv6, and in 3-AZ
  regions the IPv6 address of a Basic IP "cannot be provisioned from the OVHcloud Console yet" (the API and
  OpenStack tools are not excluded).
- `snapshots`: manual and scheduled instance snapshots, local or copied to another region (snapshot page).
  The regions page ticks "Instance Backup" in all seven regions below.
- Regions `GRA`, `LIM`, `MIL`, `PAR`, `RBX`, `SBG`, `WAW`: the regions page ticks General Purpose, Compute
  optimized and Memory optimized instances in each. Some ranges are narrower (Discovery not in Paris or
  Milan; GPU only in Gravelines, Frankfurt and Warsaw); the offering lists a region when at least the General
  Purpose range is there.

### Managed Kubernetes Service

Category `kubernetes.managed`: Managed Kubernetes in the 1-AZ regions (Free plan, and the Standard plan in
Roubaix). Pages: <https://www.ovhcloud.com/en/public-cloud/kubernetes/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/mks-plans>,
<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/datacenters-nodes-storage-flavors>,
<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/using-cluster-autoscaler>.

- `autoscaling`: the cluster autoscaler adds and removes nodes of a node pool with the workload (cluster
  autoscaler page). No plan restriction is documented.
- No `high-availability`: see *Left out on purpose*. The plans page describes the Free plan's control plane
  as "single zone" with a 99.5% SLO.
- Regions `GRA`, `LIM`, `RBX`, `SBG`, `WAW`: the regions page ticks Managed Kubernetes Service in each. The
  Kubernetes regions page lists GRA5, GRA7, GRA9, GRA11, SBG5, DE1 and WAW1 (Free plan, 1-AZ) and EU-WEST-RBX
  (Standard plan, 1-AZ).

### Managed Kubernetes Service 3-AZ

Category `kubernetes.managed`: Managed Kubernetes on the Standard plan in the 3-AZ regions. Pages:
<https://www.ovhcloud.com/en/public-cloud/kubernetes/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/mks-plans>,
<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/understanding-mks-architecture>,
<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/datacenters-nodes-storage-flavors>,
<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/using-cluster-autoscaler>.

- `autoscaling`: the cluster autoscaler, as for *Managed Kubernetes Service*.
- `high-availability`. **Scope: zone-level.** The Standard plan replicates the control plane and a
  dedicated etcd across three availability zones (architecture page), and a Standard cluster in a 3-AZ
  region "can remain fully functional during an AZ failure" (plans page), with a 99.99% SLA in 3-AZ
  regions. Paris and Milan offer only the Standard plan (Kubernetes regions page), so every cluster there
  has this capability. Node pools are spread over the zones by the customer (product page FAQ).
- Regions `MIL`, `PAR`: the Kubernetes regions page lists EU-WEST-PAR and EU-SOUTH-MIL as Standard plan, 3-AZ.
- **Status:** the regions page ticks both without the beta mark, and the plans page gives the Standard
  plan's SLAs. One FAQ line on the product page still says the 99.99% SLA "will be offered after the beta
  phase"; it is taken as stale.

### Managed MySQL

Category `database.mysql`: Public Cloud Databases for MySQL ("Managed MySQL" on the product page) in the
1-AZ regions. Pages: <https://www.ovhcloud.com/en/public-cloud/mysql/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/mysql-capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/backups>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/concepts-security-overview>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/faq>.

- `encryption-at-rest`: nodes use full volume encryption with LUKS and backups are encrypted; the security
  page answers "Yes, on every plan".
- `high-availability`. **Needs:** a multi-node plan (Business/Production, 2 nodes, or Enterprise/Advanced,
  3 nodes); the Essential/Discovery plan runs one node. **Scope: node-level only.** Data is replicated
  across nodes and failover after a node failure is automatic (FAQ). In a 1-AZ region the nodes are on
  different hosts, which "may be" in the same data centre, so the service may be impacted by a data-centre
  outage (deployment modes page). It survives the loss of a node, not of the zone.
- `point-in-time-restore`: PITR for MySQL to any point within the plan's backup retention (backups page;
  feature table on the capabilities page).
- `read-replicas`: "Read-only nodes" for MySQL (capabilities page); replica nodes accept reads only (FAQ).
  **Needs:** a multi-node plan; the product page lists "Read replica included" on the Advanced plan.
- Regions `GRA`, `LIM`, `RBX`, `SBG`, `WAW`: the regions page ticks MySQL in each, and the backups page lists
  GRA, DE, RBX, SBG and WAW as service locations. The MySQL page says the offer is available "in all Public
  Cloud 1-AZ and 3-AZ regions".

### Managed MySQL 3-AZ

Category `database.mysql`: Public Cloud Databases for MySQL in the 3-AZ regions. Pages:
<https://www.ovhcloud.com/en/public-cloud/mysql/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/mysql-capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/migrate-from-1az-to-3az>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/backups>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/concepts-security-overview>.

- `encryption-at-rest`, `point-in-time-restore` and `read-replicas`: as for *Managed MySQL*; the pages
  state them per engine and plan, not per region.
- `high-availability`. **Needs:** a multi-node plan (Production or Advanced in "Multi AZ"). **Scope:
  zone-level.** A multi-node service is deployed across three independent availability zones and keeps
  running if one zone fails (deployment modes page); its data is replicated synchronously across the three
  zones (1-AZ to 3-AZ migration page). The capabilities page gives 99.95% and 99.99% SLAs for the Multi AZ
  plans.
- Regions `MIL`, `PAR`: the regions page ticks MySQL in "3-AZ Paris" and "3-AZ Milan"; the backups page lists
  EU-WEST-PAR and EU-SOUTH-MIL as service locations.

### Managed PostgreSQL

Category `database.postgresql`: Public Cloud Databases for PostgreSQL ("Managed PostgreSQL" on the product
page) in the 1-AZ regions. Pages: <https://www.ovhcloud.com/en/public-cloud/postgresql/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/postgresql-capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/backups>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/concepts-security-overview>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/faq>.

- `encryption-at-rest`: as for *Managed MySQL* (security page, every plan).
- `high-availability`. **Needs:** a multi-node plan (Business/Production, 2 nodes, or Enterprise/Advanced,
  3 nodes). **Scope: node-level only**, for the reasons given under *Managed MySQL*: automatic failover
  between nodes on different hosts, which may share a data centre.
- `point-in-time-restore`: PITR for PostgreSQL within the plan's backup retention (backups page; capabilities
  page).
- `read-replicas`: "Read-only nodes" for PostgreSQL (capabilities page). **Needs:** a multi-node plan; the
  product page lists "Read replica included" on the Advanced plan.
- Regions `GRA`, `LIM`, `RBX`, `SBG`, `WAW`: the regions page ticks PostgreSQL in each, and the backups page
  lists GRA, DE, RBX, SBG and WAW as service locations. The PostgreSQL page says the offer is available "in
  all Public Cloud 1-AZ and 3-AZ regions".

### Managed PostgreSQL 3-AZ

Category `database.postgresql`: Public Cloud Databases for PostgreSQL in the 3-AZ regions. Pages:
<https://www.ovhcloud.com/en/public-cloud/postgresql/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/postgresql-capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/migrate-from-1az-to-3az>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/backups>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/concepts-security-overview>.

- `encryption-at-rest`, `point-in-time-restore` and `read-replicas`: as for *Managed PostgreSQL*.
- `high-availability`. **Needs:** a multi-node plan (Production or Advanced in "Multi AZ"). **Scope:
  zone-level**, for the reasons given under *Managed MySQL 3-AZ*. The product page shows "High availability
  3-AZ" among the key features.
- Regions `MIL`, `PAR`: the regions page ticks PostgreSQL in "3-AZ Paris" and "3-AZ Milan"; the backups page
  lists EU-WEST-PAR and EU-SOUTH-MIL as service locations.

### Metal Instances

Category `compute.vm`: Public Cloud instances on dedicated physical servers with no virtualisation layer.
Pages: <https://www.ovhcloud.com/en/public-cloud/metal-instances/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/network-services/ipv6-configuration>,
<https://docs.ovhcloud.com/en/guides/public-cloud/compute/save-an-instance>.

- `ipv6`: the IPv6 page applies to "A Public Cloud instance (any model)", and every instance is delivered
  with an IPv6 address.
- `snapshots`: for Metal instances, snapshots are taken only as a "Cold Snapshot": the instance switches to
  rescue mode and reboots afterwards (snapshot page). The capability is there, with that downtime.
- Regions `GRA`, `LIM`, `SBG`, `WAW`: the regions page ticks Metal Instances in Strasbourg, Gravelines,
  London, Frankfurt and Warsaw, with the footnote "Metal Instances : SBG7, GRA11". London is not an EU member
  state.

### Object Storage

Category `storage.object`: S3-compatible Object Storage in the 1-AZ regions (High Performance, Standard and
Infrequent Access classes). Pages: <https://www.ovhcloud.com/en/public-cloud/object-storage/>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-location>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-regions-comparison>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-versioning>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-encrypt-your-objects-with-sse-c>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-s3-compliancy>.

- `encryption-at-rest`: server-side encryption with OVHcloud-managed keys (SSE-OMK) or customer keys (SSE-C)
  (encryption page). The compliance page marks both as supported in every region (its "Regions" column, as
  opposed to Local Zones).
- `object-versioning`: bucket versioning, enabled per bucket and then only suspendable (versioning page);
  marked supported in every region on the compliance page.
- No `high-availability`: in a 1-AZ region data is erasure-coded across servers of one zone and "may become
  unavailable" if the data centre hosting the service has an outage (deployment modes page). That is
  durability under the rules above.
- Regions `GRA`, `LIM`, `RBX`, `SBG`, `WAW`: the endpoints page lists `gra`, `de`, `rbx`, `sbg` and `waw` as
  "1-AZ region" endpoints, and the regions page ticks Object Storage there.

### Object Storage 3-AZ

Category `storage.object`: S3-compatible Object Storage in the 3-AZ regions (Standard, Infrequent Access,
Active Archive, and Cold Archive in Paris only). Pages: <https://www.ovhcloud.com/en/public-cloud/object-storage/>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-location>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-regions-comparison>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-choosing-the-right-storage-class-for-your-needs>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-versioning>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-encrypt-your-objects-with-sse-c>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-s3-compliancy>.

- `encryption-at-rest` and `object-versioning`: as for *Object Storage*; the compliance page states them for
  every region.
- `high-availability`. **Needs:** a class other than Cold Archive (Standard, Infrequent Access or Active
  Archive, each with a 99.99% SLA in 3-AZ regions on the storage class page). **Scope: zone-level.** In a
  3-AZ region, data "remains available for both read and write operations, even in the event of a zone
  failure" (deployment modes page). Cold Archive objects must be restored before they can be read (storage
  class page), so they do not have this capability.
- Regions `MIL`, `PAR`: the endpoints page lists `eu-west-par` and `eu-south-mil` as "3-AZ region" endpoints,
  and the regions page ticks Object Storage there.

## Left out on purpose

- **`serverless.function`: no offering.** Checked on 2026-11-10:
  - The Public Cloud menu on `www.ovhcloud.com` (for example on the regions page) lists "Serverless" under
    Compute as "Coming soon", with no page behind it; <https://www.ovhcloud.com/en/public-cloud/serverless/>
    answers 404. It is not generally available.
  - AI Endpoints (<https://www.ovhcloud.com/en/public-cloud/ai-endpoints/>) gives API access to a catalogue of
    hosted generative AI models with token-based billing: model inference, not user code (PRD #11 decision).
  - AI Deploy (<https://docs.ovhcloud.com/en/guides/public-cloud/ai-machine-learning/ai-deploy-capabilities>)
    runs a customer's application or model on resources it keeps "as long as your app is running": a
    long-running deployment, not code run per invocation.
  - The OpenFaaS guide
    (<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/install-openfaas>)
    shows customers how to install OpenFaaS on their own Kubernetes cluster; OVHcloud does not run it.
  - Workflow Management, ticked on the regions page, is not described as running customer code; the
    instance snapshot page uses it, and OpenStack Mistral workflows, to schedule instance snapshots.
- **`messaging.queue`: no offering.** The regions page lists no queue product in any Public Cloud table. The
  menu describes Kafka as a "Queuing solution for event-driven architectures", but the product is Apache
  Kafka (versions 3.9, 4.2 and 4.3:
  <https://docs.ovhcloud.com/en/guides/public-cloud/databases/kafka-capabilities>), a partitioned log whose
  consumers track offsets, with no per-message acknowledgement or dead-letter queue (PRD #11 decision).
  Apache Kafka 4.x also has share groups (KIP-932, "Queues for Kafka"), but OVHcloud's Kafka pages,
  including the advanced parameters reference
  (<https://docs.ovhcloud.com/en/guides/public-cloud/databases/kafka-advanced-parameters-references>), do not
  mention them, so they are not counted.
- **Managed Kubernetes Service (1-AZ) `high-availability`:** the product page FAQ says that in a Free cluster
  the control plane components "also have high availability, but within a single zone". The documentation
  does not describe how: the architecture page draws the Free plan's control plane as one shared component,
  and the plans page calls it a "single zone managed control plane" with a 99.5% SLO, and says a Free cluster
  would go down with its zone. The Standard plan in Roubaix (1-AZ) is not described as replicated either. Unlike the
  databases, no failover mechanism is documented, so node-level high availability is not claimed. Check
  this in review; issue #14 covers node versus zone granularity.
- **Block Storage `encryption-at-rest` in Warsaw, Paris and Milan:** not in Warsaw (rollout open, see
  *Block Storage LUKS*). In Paris and Milan, encrypted zonal volumes exist (Kubernetes regions page), but the
  only offering there is *Block Storage 3-AZ*, whose class cannot be encrypted; listing both capabilities on
  it would describe a volume that has both, which does not exist.
- **Block Storage `snapshots` in Paris and Milan:** the regions page ticks neither "Volume Snapshot" nor
  "Volume Backup" there.
- **Block Storage and Object Storage (1-AZ) `high-availability`:** replication or erasure coding within one
  zone is durability (see *Rules applied*).
- **Instances and Metal Instances `high-availability`, `autoscaling` and `encryption-at-rest`:** the pages
  read describe no automatic restart of an instance on another host, no autoscaling group for instances
  (autoscaling is documented for Kubernetes node pools), and no encryption of an instance's own disks
  (encrypted volumes are *Block Storage LUKS*).
- **Managed MySQL and Managed PostgreSQL `snapshots`, `autoscaling` and `ipv6`:** backups are automated
  (with PITR) and manual backups use the engines' own dump tools (FAQ), with no snapshot of the service's
  volume. Storage and node size are increased by hand
  (<https://docs.ovhcloud.com/en/guides/public-cloud/databases/update-cluster-plan>). IPv6 is not mentioned.
- **Managed Kubernetes `encryption-at-rest` and `ipv6`:** the architecture page lists etcd encryption at rest
  among OVHcloud's tasks, but OVHcloud's roadmap still has "etcd encrypted at rest" open
  (<https://github.com/ovh/public-cloud-roadmap/issues/1035>); IPv6 for clusters is not documented.
- **Object Storage `ipv6`:** not stated on the pages read.
- **Features with no vocabulary entry**, left out under PRD #11 (*the taxonomy is unchanged*): Object
  Storage Object Lock (immutability) and asynchronous cross-region replication; customer-managed keys
  (SSE-C, Block Storage CMK in OKMS); Block Storage multi-attach; database forking and private networking
  (vRack); Managed Kubernetes custom gateways and disabled public exposure; instance remote snapshots in
  another region.
- **Products not in the snapshot:** Swift Object Storage and Cloud Archive (older Swift-API storage; S3
  Object Storage already serves the category); File Storage; MongoDB, Valkey, ClickHouse, OpenSearch and
  Kafka (no matching taxonomy category); Managed Private Registry, Load Balancer, Gateway and the other
  network services (no matching category); Local Zones (PRD #11).

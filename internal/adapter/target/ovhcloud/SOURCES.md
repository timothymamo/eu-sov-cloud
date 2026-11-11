# OVHcloud catalog snapshot: sources

`catalog.json` is a hand-compiled, dated snapshot of what OVHcloud **Public Cloud** offers in its EU regions,
in taxonomy terms. This file records where each fact comes from, so the snapshot can be checked and refreshed.

## How it was compiled

- **Retrieved on:** 2026-11-11, the date of the PRD #14 re-verification below, which changed facts in the
  snapshot. The snapshot was first compiled on 2026-11-10, and every page cited below was fetched on
  one of those two dates, without credentials and without calling any OVHcloud API.
- **`sourceURL`** is OVHcloud's Public Cloud regions and availability page,
  <https://www.ovhcloud.com/en/public-cloud/regions-availability/>. It is the one page that lists, per
  product, the locations where it is offered, and it is where the declared regions and most offering regions
  come from. On it, a tick marks a product as available in a location, and a separate "Β" mark (legend
  "Beta version") marks a beta. No cell of the EMEA tables carried the beta mark on 2026-11-10, so every
  tick relied on below is a generally available product.
- **Product pages** on `www.ovhcloud.com` and **documentation** on `docs.ovhcloud.com` give the
  capabilities. Each was read live on the retrieval date.
- **Documentation source.** `docs.ovhcloud.com` is built from `ovh/ovhcloud-docs`. Its files were read at
  commit
  [`2df4cc152902b880880b77955ab9e52de4a2c2ff`](https://github.com/ovh/ovhcloud-docs/tree/2df4cc152902b880880b77955ab9e52de4a2c2ff)
  (branch `develop`, 2026-11-10) to find the relevant pages, and every fact used was then checked on the
  published page cited. The repository named in PRD #11, `ovh/docs`, is archived by a notice in its README
  at commit
  [`989d55b1b8f7ca0d8c39280cb469e4d40513f18d`](https://github.com/ovh/docs/tree/989d55b1b8f7ca0d8c39280cb469e4d40513f18d)
  (2026-09-01), which says the repository is no longer maintained and the documentation has moved to
  `ovh/ovhcloud-docs` and `docs.ovhcloud.com`. GitHub's archive flag is not set on the repository. No fact
  was taken from `ovh/docs`.
- **Roadmap.** OVHcloud's public roadmap, `ovh/public-cloud-roadmap`, was read for the state of a few
  features: the Block Storage encryption rollouts, the general availability of the Managed Kubernetes
  Standard plan and etcd encryption at rest. Each issue is linked where it is used and was read on the
  retrieval date.
- **Re-verified on 2026-11-11 for PRD #14 (capability granularity).** Every high-availability and
  read-replica claim was checked again, and replaced by its scoped capability (`node-high-availability`
  or `zone-high-availability`) or dropped, under the target rule and the tier-variant rule (*Rules
  applied*). `ovh/ovhcloud-docs` `develop` was still at the commit cited below, and the pages cited were
  read from it; the MySQL and PostgreSQL product pages were read live. Because it changed facts,
  `retrievedOn` in `catalog.json` is its date, 2026-11-11. No region changed; the regions page was
  read on 2026-11-10.

## Licences of the sources

- **`www.ovhcloud.com`** (product and regions pages): © OVH SAS, under the site's legal notices. No open
  licence is granted.
- **`docs.ovhcloud.com`** and its source **`ovh/ovhcloud-docs`**: no licence is stated. The repository has no
  licence file at the commit above, its README states none, and GitHub detects none. So the content is
  under OVH SAS's copyright with all rights reserved.
- **`ovh/docs`** (archived by its README, not used for facts) is licensed CC BY-NC-SA 4.0 (Creative
  Commons Attribution-NonCommercial-ShareAlike 4.0 International), as its `LICENSE` file states at the
  commit above. Its README (`readme.md`) says the content "remains under the terms provided in the
  LICENSE file".
- **`ovh/public-cloud-roadmap`** issues: no licence is stated. Only an issue's open or closed state, its
  dates and a brief quotation from a comment are used.
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
  and Public Cloud Databases deployed in Paris or Milan are 3-AZ deployments. Where a product reaches
  zone-level high availability only in the 3-AZ regions, the snapshot lists a variant whose name has
  "3-AZ" in `MIL` and `PAR` only, beside the offering in the 1-AZ regions (PRD #11, *a capability limited
  to some regions makes a separate offering*). For the same reason, a capability missing from some of a product's
  regions makes a separate offering (*Block Storage LUKS* in the 1-AZ regions, and *Block Storage LUKS
  Zonal* in the 3-AZ regions, which have no volume snapshots).
- **Capabilities:** only from the taxonomy vocabulary, each as the taxonomy defines it, and only when an
  OVHcloud page describes the feature as available. A capability that needs a particular plan or class is
  marked **Needs:** on its line. A feature with no vocabulary entry is listed under *Left out on purpose*.
- **Target rule** (ADR 0011): an offering lists a capability only where the documentation states it
  **in full**. An offering that meets only part of a capability's definition does not list it, and
  *Left out on purpose* records the omission with its reason.
- **Tier-variant rule** (ADR 0011): all of an offering's capabilities hold in **one** tier. The base
  offering is the product's default tier. A tier that adds a capability is a variant named
  `<base offering> <tier>`, with its own section below. A tier that only lacks capabilities gets no
  offering. Here the base tiers are the Essential/Discovery plan (Managed MySQL and Managed
  PostgreSQL), the Standard class (Object Storage 3-AZ) and the Standard plan (Managed Kubernetes
  Service 3-AZ, the only plan in Paris and Milan). The variants are *Managed MySQL Multi-node*,
  *Managed PostgreSQL Multi-node* and their *Multi-node 3-AZ* forms, and *Block Storage 3-AZ Regional
  Classic*, named after the volume class. A **Needs:** line names the tier its offering is named after,
  or the product's default tier. **Naming order:** a variant is `<base offering> <tier>`, and a
  region-group suffix such as `3-AZ` comes after the tier (*Managed PostgreSQL Multi-node 3-AZ*). The
  1-AZ variant's name is then a prefix of the 3-AZ one, so it wins ties by name, and a resource that
  needs only a capability both variants share is pointed at the variant offered in the most regions.
  This departs from PRD #11's `<product> 3-AZ` pattern only where a tier variant also has a 3-AZ form;
  *Block Storage 3-AZ Regional Classic* keeps it, as its class exists only in the 3-AZ regions.
- **Scope:** high availability is listed at the level the documentation states in full.
  `node-high-availability` is listed when the offering keeps serving through the loss of a node, server
  or host with no manual failover; `zone-high-availability` when it keeps serving through the loss of a
  whole availability zone or data centre. A **Scope:** line on the capability says which level, and why
  the other is not claimed. The catalog adds node level to every offering that lists zone level (ADR
  0011), so each zone-level section also names `node-high-availability`.
- **Copies of data are not availability.** Redundant copies of data inside one zone (replication or
  erasure coding) are durability, and are not counted as `node-high-availability` unless a page states
  that the service keeps **serving** through the loss of a server. OVHcloud's Object Storage deployment
  modes page states that for the 1-AZ setup, so *Object Storage* lists node level. The Block Storage
  pages state only copies of data for the zonal classes, so those offerings list neither level. The
  Block Storage product page also uses the phrase "high availability" for every class ("Thanks to its
  high availability, you can choose between One-Zone, Multi-Zone, or Local Zone configuration"). That
  use is not counted: for the one-zone classes the same page names only data "replicated in the same
  datacentre", and it says high availability "is guaranteed in our 3-AZ regions" with the Regional
  Classic Volume class.

## Declared EU regions

`GRA` (Gravelines, France), `LIM` (Frankfurt/Limburg, Germany), `MIL` (Milan, Italy), `PAR` (Paris, France),
`RBX` (Roubaix, France), `SBG` (Strasbourg, France), `WAW` (Warsaw, Poland).

The regions page's EMEA tables list eight locations on 2026-11-10: Strasbourg `SBG`, Gravelines `GRA`,
Roubaix `RBX`, "3-AZ Paris" `PAR`, "3-AZ Milan" `MIL`, London `ERI`, Frankfurt `LIM` and Warsaw `WAW`, plus a
"Local Zones" column with no location named. The column for `LIM` is headed "Frankfurt" in most tables and
"Limburg" in the identity table. London (`ERI`, Erith) is in the United Kingdom, which is not an EU member
state, so it is not declared. The North America (Montreal `BHS`, Toronto, Seattle, Washington DC) and Asia
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
- No `node-high-availability` and no `zone-high-availability`: the classes in 1-AZ regions replicate
  data within one zone (storage class page, "Zonal" replication). That is a copy of data, not a stated
  continuity of an attached volume, so neither level is claimed (*Rules applied*).
- Regions `GRA`, `LIM`, `RBX`, `SBG`, `WAW`: the regions page ticks the three Block Storage classes and
  "Volume Snapshot" in each. Volumes in `PAR` and `MIL` are under *Block Storage 3-AZ Regional Classic*
  and *Block Storage LUKS Zonal*.

### Block Storage 3-AZ Regional Classic

Category `storage.block`: the Regional Classic Volume class, offered only in 3-AZ regions. It is a tier
variant (ADR 0011), named after the class, because Paris and Milan also offer zonal High Speed Gen2
volumes, and no page names Regional Classic as the default class there. Pages:
<https://www.ovhcloud.com/en/public-cloud/block-storage/>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/block-storage/block-storage-the-right-storage-class>,
<https://docs.ovhcloud.com/en/guides/public-cloud/compute/storage-classic-multi-attach-3az>.

- `zone-high-availability`. **Needs:** the Regional Classic Volume class (volume type
  `classic-multiattach`, "Classic 3AZ" in the console), the class this offering is named after.
  **Scope: zone-level.** The zone-level evidence is continued service, stated on two pages. The storage
  class page says: "To ensure service continuity even in the event of an AZ failure, this class also
  supports Multi-Attach, allowing multiple instances located in different availability zones to
  simultaneously attach and use the same volume." The reference architecture page
  (<https://docs.ovhcloud.com/en/guides/public-cloud/cross-functional/deployments-modes-reference-architecture>,
  row "Block storage Classic Multi-Zone") says that in an AZ failure "Block Storage data remains
  available without impact or downtime, provided the conditions of the multi-attached resilient
  architecture are met". The condition is that the volume is attached from instances in more than one
  zone (up to 16 instances, multi-attach page); a volume attached from one zone only is not stated to
  keep serving. Supporting evidence only, since copies of data are not availability (*Rules applied*):
  the storage class page says the class "provides high availability by automatically replicating data
  across three availability zones", with a 99.99% SLA and "Multi-zone" replication, and the product page
  says high availability is guaranteed in 3-AZ regions with this class. The zonal High Speed volumes also offered in these regions do not have this
  capability, and get no offering of their own (*Block Storage LUKS Zonal* covers the encrypted ones).
- `node-high-availability`: implied by zone level (ADR 0011); a volume replicated across three zones
  also survives the loss of one storage server.
- No `snapshots`: the regions page does not tick "Volume Snapshot" or "Volume Backup" in Paris or Milan.
- No `encryption-at-rest`: multi-attach volumes cannot be encrypted (multi-attach page, *Limitations*; storage
  class page: Regional Classic does not support LUKS). Encrypted volumes in these regions are zonal, under
  *Block Storage LUKS Zonal*. See *Left out on purpose*.
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
- `snapshots`: volume snapshots as for *Block Storage*. The Managed Kubernetes volume snapshot page
  (<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/backup-restore-pv-volume-snapshot>)
  says "Volume snapshots work with all storage classes, including LUKS encrypted volumes".
- Regions `GRA`, `LIM`, `RBX`, `SBG`: the Managed Kubernetes regions page lists the regions where LUKS
  encrypted storage "is currently available": RBX, SBG, GRA, EU-WEST-PAR, DE1 and EU-SOUTH-MIL in the EU.
  OVHcloud's roadmap closed the rollouts for GRA and DE1 on 2026-02-19
  (<https://github.com/ovh/public-cloud-roadmap/issues/951>,
  <https://github.com/ovh/public-cloud-roadmap/issues/952>). Warsaw is not listed, and its rollout is still
  open (<https://github.com/ovh/public-cloud-roadmap/issues/954>). The same page lists EU-WEST-PAR and
  EU-SOUTH-MIL, but volume snapshots are not offered there, so encrypted volumes in Paris and Milan make an
  offering of their own, *Block Storage LUKS Zonal*.

### Block Storage LUKS Zonal

Category `storage.block`: encrypted zonal volumes in the 3-AZ regions, of the High Speed Gen2 class (volume
type `high-speed-gen2-luks`). Each volume sits in one availability zone of the region. Listed apart from
*Block Storage LUKS* because volume snapshots are not offered in these regions. Pages:
<https://docs.ovhcloud.com/en/guides/public-cloud/compute/encrypting-a-block-storage-volume-with-luks>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/block-storage/block-storage-the-right-storage-class>,
<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/datacenters-nodes-storage-flavors>,
<https://www.ovhcloud.com/en/public-cloud/regions-availability/>.

- `encryption-at-rest`: LUKS encryption at rest, as for *Block Storage LUKS* (encryption page). The
  encryption page says "classic-luks is not available in 3-AZ regions" and to "use high-speed-gen2-luks
  there"; its example creates a `high-speed-gen2-luks` volume in region `EU-WEST-PAR`, availability zone
  `eu-west-par-a`. The storage class page offers High Speed Volume Gen2 in "3-AZ, 1-AZ, Local Zones".
- No `snapshots`: the regions page ticks neither "Volume Snapshot" nor "Volume Backup" in Paris or Milan,
  as for *Block Storage 3-AZ Regional Classic*.
- No `node-high-availability` and no `zone-high-availability`: the storage class page gives High Speed
  Volume Gen2 "Zonal" replication, so a volume stays in its zone; copies within one zone are durability
  under the rules above.
- Regions `MIL`, `PAR`: the Managed Kubernetes regions page lists EU-WEST-PAR and EU-SOUTH-MIL among the
  regions where LUKS encrypted storage "is currently available", and the regions page ticks Block Storage
  High Speed Gen 2 in "3-AZ Paris" and "3-AZ Milan".
- The unencrypted zonal volumes in these regions have no offering of their own: they have no capability
  that this offering lacks.

### Instances

Category `compute.vm`: Public Cloud virtual machine instances (General Purpose, Compute and Memory
optimized, Discovery, GPU and Storage optimized ranges). "Instances" is the name the OVHcloud Console and
documentation use (the getting-started page's navigation path ends in "Compute > Instances"); the product
page is titled "Virtual Machine Instances". Pages:
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
- No `node-high-availability` and no `zone-high-availability`: no replica or failover mechanism is
  documented for the control plane of a 1-AZ cluster, so neither level is stated in full (target rule).
  The plans page describes the Free plan's control plane as "single zone" with a 99.5% SLO. See *Left
  out on purpose*.
- No tier variant for the Standard plan in Roubaix: it adds no capability in the vocabulary (Floating IPs
  per node and a custom router have no entry), so it gets no offering (tier-variant rule).
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
- `zone-high-availability`. **Needs:** the Standard plan, the only plan Paris and Milan offer
  (Kubernetes regions page), so it is this offering's default tier and every cluster there has the
  capability. **Scope: zone-level.** The Standard plan replicates the control plane and a dedicated etcd
  across three availability zones (architecture page), and a Standard cluster in a 3-AZ region "can
  remain fully functional during an AZ failure" (plans page), with a 99.99% SLA in 3-AZ regions. Node
  pools are spread over the zones by the customer (product page FAQ). The reference architecture page
  (<https://docs.ovhcloud.com/en/guides/public-cloud/cross-functional/deployments-modes-reference-architecture>,
  row "Managed Kubernetes Service") corroborates it: in an AZ failure "the Control Plane remains
  available and the customer's workload is rescheduled on the nodes on another available AZ".
  **Caveat:** zone level covers the control plane and node pools spread across the zones. A stateful
  workload survives a zone loss only when its persistent volumes use Regional Classic (*Block Storage
  3-AZ Regional Classic*): the same row says "workloads using persistent volumes of single-zone classes
  cannot be migrated to other AZs".
- `node-high-availability`: implied by zone level (ADR 0011); three control plane replicas also survive
  the loss of one of them.
- Regions `MIL`, `PAR`: the Kubernetes regions page lists EU-WEST-PAR and EU-SOUTH-MIL as Standard plan, 3-AZ.
- **Status:** generally available. OVHcloud's roadmap closed "MKS Standard offer"
  (<https://github.com/ovh/public-cloud-roadmap/issues/693>) on 2025-09-17 with a collaborator's comment
  that the "Managed Kubernetes Standard plan is now available on General Availability", and closed
  "Multi-AZs clusters" (<https://github.com/ovh/public-cloud-roadmap/issues/22>) on 2025-10-02 after a
  comment that the Standard plan is in General Availability in EU-WEST-PAR with 3-AZ deployment. The
  regions page ticks both regions without the beta mark. Three lines still describe the plan as before
  GA, and are taken as stale: the product page FAQ says the 99.99% SLA "will be offered after the beta
  phase", the plans page gives a "99.99% SLA at General Availability", and the architecture page's
  comparison table gives "99.99% SLA (at GA)".

### Managed MySQL

Category `database.mysql`: Public Cloud Databases for MySQL ("Managed MySQL" on the product page) on the
Essential/Discovery plan, one node, in the 1-AZ regions. This is the base offering (tier-variant rule):
the multi-node plans add capabilities, and are *Managed MySQL Multi-node* and *Managed MySQL
Multi-node 3-AZ*. Pages: <https://www.ovhcloud.com/en/public-cloud/mysql/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/mysql-capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison>
(the deployment modes page),
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/backups>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/concepts-security-overview>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/faq>.

- `encryption-at-rest`: nodes use full volume encryption with LUKS and backups are encrypted; the security
  page answers "Yes, on every plan". **Needs:** nothing beyond the default Essential/Discovery plan.
- `point-in-time-restore`: PITR for MySQL to any point "within the retention period of the chosen customer
  plan" (backups page; feature table on the capabilities page). The MySQL capabilities page
  (`mysql-capabilities`) gives a backup retention for every plan, Discovery included. **Needs:**
  nothing beyond the default Essential/Discovery plan.
- No `node-high-availability`, no `zone-high-availability` and no `read-replicas`: the Discovery plan runs
  one node (MySQL capabilities page, *Plans*), and the "High availability" row of the capabilities page's
  plan overview is empty for Essential/Discovery. See *Left out on purpose*.
- Regions `GRA`, `LIM`, `RBX`, `SBG`, `WAW`: the regions page ticks MySQL in each, and the backups page lists
  GRA, DE, RBX, SBG and WAW as service locations. The MySQL capabilities page says the Public Cloud
  Databases offer is available "in all Public Cloud 1-AZ and 3-AZ regions". The 3-AZ regions are not
  listed for this plan; see *Left out on purpose*.

### Managed MySQL Multi-node 3-AZ

Category `database.mysql`: Public Cloud Databases for MySQL on a multi-node plan in the 3-AZ regions. A
tier variant of *Managed MySQL* (tier-variant rule). Pages: <https://www.ovhcloud.com/en/public-cloud/mysql/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/mysql-capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison>
(the deployment modes page),
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/migrate-from-1az-to-3az>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/backups>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/concepts-security-overview>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/faq>.

- `encryption-at-rest` and `point-in-time-restore`: as for *Managed MySQL*; the pages state them for every
  plan, and per engine, not per region.
- `zone-high-availability`. **Needs:** a multi-node plan, Business/Production or
  Enterprise/Advanced, Multi AZ (the capabilities page's plan overview ticks "High availability" for
  both). **Scope: zone-level.** The deployment modes page says a multi-node service "is deployed across
  three independent availability zones, ensuring service continuity even if one zone experiences an
  outage"; its data is replicated synchronously across the three zones (1-AZ to 3-AZ migration page).
  The capabilities page gives 99.95% and 99.99% SLAs for the Multi AZ plans. The reference architecture
  page
  (<https://docs.ovhcloud.com/en/guides/public-cloud/cross-functional/deployments-modes-reference-architecture>,
  row "DBaaS") corroborates it: in an AZ failure "databases and data will remain available".
- `node-high-availability`: implied by zone level (ADR 0011); failover after a node failure is automatic
  (FAQ).
- `read-replicas`. **Needs:** a multi-node plan, Business/Production or Enterprise/Advanced, Multi AZ.
  "Read-only nodes" for MySQL (capabilities page); a replica node "can accept READ operations only"
  (FAQ), and the product page lists "Read-only access to replication nodes" as a key feature. The
  product page names "Read replica included" on the Enterprise/Advanced plan only;
  Business/Production's second node is a replica in the FAQ's sense, so the capability is counted on
  every multi-node plan (PRD #14 decision). A refresh that reads the product page strictly would split
  this offering into a Business/Production and an Enterprise/Advanced variant, with `read-replicas` on
  Enterprise/Advanced only.
- Regions `MIL`, `PAR`: the regions page ticks MySQL in "3-AZ Paris" and "3-AZ Milan"; the backups page lists
  EU-WEST-PAR and EU-SOUTH-MIL as service locations.

### Managed MySQL Multi-node

Category `database.mysql`: Public Cloud Databases for MySQL on a multi-node plan in the 1-AZ regions. A
tier variant of *Managed MySQL* (tier-variant rule). Pages: <https://www.ovhcloud.com/en/public-cloud/mysql/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/mysql-capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison>
(the deployment modes page),
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/backups>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/concepts-security-overview>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/faq>.

- `encryption-at-rest` and `point-in-time-restore`: as for *Managed MySQL*; the pages state them for every
  plan.
- `node-high-availability`. **Needs:** a multi-node plan, Business/Production (2 nodes) or
  Enterprise/Advanced (3 nodes) (MySQL capabilities page, *Plans*); the capabilities page's plan overview
  ticks "High availability" for both in "Single AZ". **Scope: node-level only.** Data is replicated
  across nodes and "in case of a node failure, the failover mechanism is automatic" (FAQ). In a 1-AZ
  region the nodes "are deployed in different physical hosts", which "may be deployed within a single
  data center", and the service "offers resilience against node and disk failures" (deployment modes
  page). It survives the loss of a node, not of the zone, so `zone-high-availability` is not claimed.
- `read-replicas`. **Needs:** a multi-node plan, Business/Production or Enterprise/Advanced. As for
  *Managed MySQL Multi-node 3-AZ*: replica nodes accept reads only (FAQ), and the capability is counted on
  every multi-node plan although the product page names "Read replica included" on
  Enterprise/Advanced only.
- Regions `GRA`, `LIM`, `RBX`, `SBG`, `WAW`: as for *Managed MySQL*.

### Managed PostgreSQL

Category `database.postgresql`: Public Cloud Databases for PostgreSQL ("Managed PostgreSQL" on the product page) on the
Essential/Discovery plan, one node, in the 1-AZ regions. This is the base offering (tier-variant rule):
the multi-node plans add capabilities, and are *Managed PostgreSQL Multi-node* and *Managed
PostgreSQL Multi-node 3-AZ*. Pages: <https://www.ovhcloud.com/en/public-cloud/postgresql/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/postgresql-capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison>
(the deployment modes page),
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/backups>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/concepts-security-overview>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/faq>.

- `encryption-at-rest`: nodes use full volume encryption with LUKS and backups are encrypted; the security
  page answers "Yes, on every plan". **Needs:** nothing beyond the default Essential/Discovery plan.
- `point-in-time-restore`: PITR for PostgreSQL to any point "within the retention period of the chosen customer
  plan" (backups page; feature table on the capabilities page). The PostgreSQL capabilities page
  (`postgresql-capabilities`) gives a backup retention for every plan, Discovery included. **Needs:**
  nothing beyond the default Essential/Discovery plan.
- No `node-high-availability`, no `zone-high-availability` and no `read-replicas`: the Discovery plan runs
  one node (PostgreSQL capabilities page, *Plans*), and the "High availability" row of the capabilities page's
  plan overview is empty for Essential/Discovery. See *Left out on purpose*.
- Regions `GRA`, `LIM`, `RBX`, `SBG`, `WAW`: the regions page ticks PostgreSQL in each, and the backups page lists
  GRA, DE, RBX, SBG and WAW as service locations. The PostgreSQL capabilities page says the Public Cloud
  Databases offer is available "in all Public Cloud 1-AZ and 3-AZ regions". The 3-AZ regions are not
  listed for this plan; see *Left out on purpose*.

### Managed PostgreSQL Multi-node 3-AZ

Category `database.postgresql`: Public Cloud Databases for PostgreSQL on a multi-node plan in the 3-AZ regions. A
tier variant of *Managed PostgreSQL* (tier-variant rule). Pages: <https://www.ovhcloud.com/en/public-cloud/postgresql/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/postgresql-capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison>
(the deployment modes page),
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/migrate-from-1az-to-3az>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/backups>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/concepts-security-overview>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/faq>.

- `encryption-at-rest` and `point-in-time-restore`: as for *Managed PostgreSQL*; the pages state them for every
  plan, and per engine, not per region.
- `zone-high-availability`. **Needs:** a multi-node plan, Business/Production or
  Enterprise/Advanced, Multi AZ (the capabilities page's plan overview ticks "High availability" for
  both). **Scope: zone-level.** The deployment modes page says a multi-node service "is deployed across
  three independent availability zones, ensuring service continuity even if one zone experiences an
  outage"; its data is replicated synchronously across the three zones (1-AZ to 3-AZ migration page).
  The capabilities page gives 99.95% and 99.99% SLAs for the Multi AZ plans. The reference architecture
  page
  (<https://docs.ovhcloud.com/en/guides/public-cloud/cross-functional/deployments-modes-reference-architecture>,
  row "DBaaS") corroborates it: in an AZ failure "databases and data will remain available".
- `node-high-availability`: implied by zone level (ADR 0011); failover after a node failure is automatic
  (FAQ).
- `read-replicas`. **Needs:** a multi-node plan, Business/Production or Enterprise/Advanced, Multi AZ.
  "Read-only nodes" for PostgreSQL (capabilities page); a replica node "can accept READ operations only"
  (FAQ), and the product page lists "Read-only access to replication nodes" as a key feature. The
  product page names "Read replica included" on the Enterprise/Advanced plan only;
  Business/Production's second node is a replica in the FAQ's sense, so the capability is counted on
  every multi-node plan (PRD #14 decision). A refresh that reads the product page strictly would split
  this offering into a Business/Production and an Enterprise/Advanced variant, with `read-replicas` on
  Enterprise/Advanced only.
- Regions `MIL`, `PAR`: the regions page ticks PostgreSQL in "3-AZ Paris" and "3-AZ Milan"; the backups page lists
  EU-WEST-PAR and EU-SOUTH-MIL as service locations.

### Managed PostgreSQL Multi-node

Category `database.postgresql`: Public Cloud Databases for PostgreSQL on a multi-node plan in the 1-AZ regions. A
tier variant of *Managed PostgreSQL* (tier-variant rule). Pages: <https://www.ovhcloud.com/en/public-cloud/postgresql/>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/postgresql-capabilities>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison>
(the deployment modes page),
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/backups>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/concepts-security-overview>,
<https://docs.ovhcloud.com/en/guides/public-cloud/databases/faq>.

- `encryption-at-rest` and `point-in-time-restore`: as for *Managed PostgreSQL*; the pages state them for every
  plan.
- `node-high-availability`. **Needs:** a multi-node plan, Business/Production (2 nodes) or
  Enterprise/Advanced (3 nodes) (PostgreSQL capabilities page, *Plans*); the capabilities page's plan overview
  ticks "High availability" for both in "Single AZ". **Scope: node-level only.** Data is replicated
  across nodes and "in case of a node failure, the failover mechanism is automatic" (FAQ). In a 1-AZ
  region the nodes "are deployed in different physical hosts", which "may be deployed within a single
  data center", and the service "offers resilience against node and disk failures" (deployment modes
  page). It survives the loss of a node, not of the zone, so `zone-high-availability` is not claimed.
- `read-replicas`. **Needs:** a multi-node plan, Business/Production or Enterprise/Advanced. As for
  *Managed PostgreSQL Multi-node 3-AZ*: replica nodes accept reads only (FAQ), and the capability is counted on
  every multi-node plan although the product page names "Read replica included" on
  Enterprise/Advanced only.
- Regions `GRA`, `LIM`, `RBX`, `SBG`, `WAW`: as for *Managed PostgreSQL*.

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
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-regions-comparison>
(the deployment modes page),
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-versioning>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-encrypt-your-objects-with-sse-c>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-s3-compliancy>.

- `encryption-at-rest`: server-side encryption with OVHcloud-managed keys (SSE-OMK) or customer keys (SSE-C)
  (encryption page). The compliance page marks both as supported in every region (its "Regions" column, as
  opposed to Local Zones).
- `object-versioning`: bucket versioning, enabled per bucket and then only suspendable (versioning page);
  marked supported in every region on the compliance page.
- `node-high-availability`. **Needs:** nothing beyond the default class; the deployment modes page
  states it for the 1-AZ setup as a whole. **Scope: node-level only.** The 1-AZ setup "offers resilience
  against server rack and drive failures", erasure coding ensures "continuity in the event of hardware
  failures by splitting data across multiple disks and servers", and the page's comparison table gives
  *Data Availability* as "protected against server/disk failures". That is continued service through the
  loss of a server, with no customer action, not only a copy of data. "Continuity in the event of
  hardware failures" is read as continued service (availability), not as recovery of lost data; that is
  what separates it from the Scaleway snapshot's Object Storage, whose FAQ says only that Scaleway can
  rebuild the data after a data-centre failure, a recovery, and states no continued service. The same table gives "Data center
  outage risk", and data "may become unavailable" in a data-centre outage, so `zone-high-availability` is
  not claimed.
- Regions `GRA`, `LIM`, `RBX`, `SBG`, `WAW`: the endpoints page lists `gra`, `de`, `rbx`, `sbg` and `waw` as
  "1-AZ region" endpoints, and the regions page ticks Object Storage there.

### Object Storage 3-AZ

Category `storage.object`: S3-compatible Object Storage in the 3-AZ regions (Standard, Infrequent Access,
Active Archive, and Cold Archive in Paris only). Pages: <https://www.ovhcloud.com/en/public-cloud/object-storage/>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-location>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-regions-comparison>
(the deployment modes page),
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-choosing-the-right-storage-class-for-your-needs>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-versioning>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-encrypt-your-objects-with-sse-c>,
<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-s3-compliancy>.

- `encryption-at-rest` and `object-versioning`: as for *Object Storage*; the compliance page states them for
  every region.
- `zone-high-availability`. **Needs:** the default class, Standard (Infrequent Access and Active
  Archive also qualify, each with a 99.99% SLA in 3-AZ regions on the storage class page; Cold Archive
  does not). **Scope: zone-level.** In a 3-AZ region, data "remains available for both read and write
  operations, even in the event of a zone failure" (deployment modes page), and the reference
  architecture page's "Object Storage ( Standard class )" row says the same. Cold Archive objects must be
  restored before they can be read (storage class page), so that class lacks the capability, and as a
  tier that only lacks capabilities it gets no offering (tier-variant rule).
- `node-high-availability`: implied by zone level (ADR 0011).
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
- **Managed Kubernetes Service (1-AZ) `node-high-availability` and `zone-high-availability` (PRD #14,
  target rule):** two pages claim high availability in general terms. The product page FAQ
  (<https://www.ovhcloud.com/en/public-cloud/kubernetes/>) says that in a Free cluster the control plane
  components "also have high availability, but within a single zone", and the architecture page's table
  of what OVHcloud manages
  (<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/understanding-mks-architecture>)
  gives the API Server "Deployment, scaling, high availability, security patches", with no distinction
  between plans. Neither says how. The plans page
  (<https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/mks-plans>)
  calls the Free plan's control plane a "single zone managed control plane" with a 99.5% SLO, and the
  architecture page's diagram draws it as one shared control plane, against three control plane
  replicas, one per zone, for the Standard plan. The Standard plan in Roubaix (1-AZ) has a "99.9% SLA for
  1-AZ regions" (regions page,
  <https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/datacenters-nodes-storage-flavors>),
  but no page says whether its control plane is replicated there. No replica or failover mechanism is
  documented, so neither level is stated in full. This matches the Scaleway snapshot's Kapsule Mutualized
  tier.
- **Managed MySQL and Managed PostgreSQL (Essential/Discovery) `node-high-availability`,
  `zone-high-availability` and `read-replicas`:** the plan runs one node
  (<https://docs.ovhcloud.com/en/guides/public-cloud/databases/postgresql-capabilities>,
  <https://docs.ovhcloud.com/en/guides/public-cloud/databases/mysql-capabilities>, *Plans*), so it has no
  standby to fail over to and no replica to read from. The capabilities page's plan overview
  (<https://docs.ovhcloud.com/en/guides/public-cloud/databases/capabilities>) leaves "High availability"
  empty for it. These capabilities are on the Multi-node variants. Before PRD #14 the base offering listed
  them, a union of plans that no single plan has.
- **The Essential/Discovery plan of MySQL and PostgreSQL in Paris and Milan:** no page read states that
  these engines offer Discovery in the 3-AZ regions. The capabilities page's plan overview
  (<https://docs.ovhcloud.com/en/guides/public-cloud/databases/capabilities>) has "Multi AZ" columns for
  Business/Production and Enterprise/Advanced only, and the deployment modes page
  (<https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison>)
  describes multi-node services there. So *Managed MySQL* and *Managed PostgreSQL* list the 1-AZ regions
  only, and the former *Managed MySQL 3-AZ* and *Managed PostgreSQL 3-AZ* became *Multi-node 3-AZ*
  variants. A Discovery service there would have exactly the base offering's capabilities, so a later
  refresh adds `MIL` and `PAR` to the base offering once a page states it, rather than a new offering.
- **Managed MySQL and Managed PostgreSQL Multi-node (1-AZ) `zone-high-availability`:** the nodes "may be
  deployed within a single data center"
  (<https://docs.ovhcloud.com/en/guides/public-cloud/databases/public-cloud-databases-regions-comparison>),
  so surviving a zone outage is not stated.
- **Block Storage `encryption-at-rest` in Warsaw, and on *Block Storage 3-AZ Regional Classic*:** not in
  Warsaw (rollout open, see *Block Storage LUKS*). In Paris and Milan, encrypted volumes are zonal and are
  listed as *Block Storage LUKS Zonal*. *Block Storage 3-AZ Regional Classic* does not have the
  capability: its class cannot be encrypted, and
  listing both capabilities on it would describe a volume that has both, which does not exist.
- **Block Storage `snapshots` in Paris and Milan:** the regions page ticks neither "Volume Snapshot" nor
  "Volume Backup" there.
- **Block Storage (1-AZ), Block Storage LUKS and Block Storage LUKS Zonal `node-high-availability` and
  `zone-high-availability`:** the storage class page
  (<https://docs.ovhcloud.com/en/guides/storage-and-backup/block-storage/block-storage-the-right-storage-class>)
  gives these classes "Zonal" replication, a copy of data within one zone, and states no continuity of an
  attached volume through the loss of a storage server (see *Rules applied*).
- **Object Storage (1-AZ) `zone-high-availability`:** the deployment modes page
  (<https://docs.ovhcloud.com/en/guides/storage-and-backup/object-storage/s3-regions-comparison>) gives
  "Data center outage risk" for the 1-AZ setup; only node level is listed.
- **Instances and Metal Instances `node-high-availability`, `zone-high-availability`, `autoscaling` and
  `encryption-at-rest`:** the pages
  read describe no automatic restart of an instance on another host, no autoscaling group for instances
  (autoscaling is documented for Kubernetes node pools), and no encryption of an instance's own disks
  (encrypted volumes are *Block Storage LUKS* and *Block Storage LUKS Zonal*).
- **Managed MySQL and Managed PostgreSQL `snapshots`, `autoscaling` and `ipv6`:** backups are automated
  (with PITR) and manual backups use the engines' own dump tools (FAQ), with no snapshot of the service's
  volume. Storage and node size are increased by hand
  (<https://docs.ovhcloud.com/en/guides/public-cloud/databases/update-cluster-plan>). IPv6 is not mentioned.
- **Managed Kubernetes `encryption-at-rest` and `ipv6`:** the architecture page lists etcd encryption at rest
  among OVHcloud's tasks, and its security features include "Secrets encryption in transit and at rest".
  Kubernetes keeps Secrets in etcd, and OVHcloud's roadmap still has "etcd encrypted at rest" open
  (<https://github.com/ovh/public-cloud-roadmap/issues/1035>), so neither line is counted. IPv6 for
  clusters is not documented.
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

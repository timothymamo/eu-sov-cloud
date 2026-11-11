# UpCloud catalog snapshot: sources

`catalog.json` is a hand-compiled, dated snapshot of what UpCloud offers, in taxonomy terms. This file
records where each fact comes from, so the snapshot can be checked and refreshed.

## How it was compiled

- **Retrieved on:** 2026-11-11, for PRD #77. Every page cited below was fetched that day with `curl`
  (8.18.0), as an unauthenticated HTTP GET, and answered HTTP 200. No UpCloud API was called. Some
  web-fetch tools get HTTP 403 from `upcloud.com`; a plain `curl` does not.
- **Documentation:** every page lives under <https://upcloud.com/docs/>, the `sourceURL` of the snapshot.
  The text was read from each page's `<article>` element. Pages were found through
  <https://upcloud.com/docs/llms.txt> and <https://upcloud.com/docs/sitemap.xml>, which were used to
  discover pages only, not as sources of facts.
- **Availability pages:** each product has one, with a table of zones. Its cells carry an icon whose label
  is "Available"; no other mark appeared in any cell cited below.
- **`sourceURL`** is the documentation root, because the zones and the capabilities both come from the
  documentation. The pricing pages were not read.

## Licences of the sources

- **Documentation** (`upcloud.com/docs/`, every page cited below): under UpCloud's copyright, with no
  open licence stated. Every page's footer reads "UpCloud Ltd 2026. All rights reserved."
- **Rule for this file and `catalog.json`:** take facts only (which product exists, in which zones, with
  which feature) and brief quotations, each in quotation marks and attributed to the page it comes from.
  Paraphrase everything else. Never copy documentation text in bulk, and keep it out of `catalog.json`,
  which holds only names, taxonomy terms and zone codes. A refresh follows the same rule.

## Rules applied

- **Regions are zones.** An UpCloud zone (`de-fra1`, ...) is one data centre. The snapshot's regions are
  zones, and an offering lists a zone only when the product's availability page marks it "Available"
  there. Only zones in EU member states are declared (PRD #77).
- **Managed Object Storage** is placed by the zone that hosts each of its regions (`EUROPE-1`, ...): the
  availability page says a region "is hosted in one location" and "A service's data is stored in its
  region's location". The page's "Accessible via SDN from" column names where a region can be reached
  from, not where data is stored, so it adds no zone.
- **Capabilities:** only from the taxonomy vocabulary, and only when the documentation describes the
  feature as generally available. Features in beta are left out and listed under *Left out on purpose*.
- **Target rule** (ADR 0011): an offering lists a capability only where the documentation states it
  **in full**. An offering that meets only part of a capability's definition does not list it, and
  *Left out on purpose* records the omission with its reason.
- **Tier-variant rule** (ADR 0011): all of an offering's capabilities hold in **one** tier or option. The
  base offering is the product's default, or where none is stated, its lowest tier: the one that adds no
  capability. A tier or option that adds a capability is a variant named `<base offering> <tier>`, with
  its own section below. Here the base tiers are unencrypted storage (Block Storage, encryption is an
  option chosen at creation), the Development control plane (Managed Kubernetes) and a single node
  (Managed PostgreSQL and Managed MySQL). The variants are *Block Storage Encrypted*, *Managed Kubernetes
  Production*, *Managed PostgreSQL Multi-node* and *Managed MySQL Multi-node*. A **Needs:** line names
  the tier its offering is named after, or the base tier.
- **Tier-variant zones:** a tier variant lists every zone its product lists. Each of these products'
  availability pages says it is available in all UpCloud locations, with one column per product or per
  storage tier and no column per plan, and the pages that state the plans (the Managed Kubernetes control
  plane page, the PostgreSQL and MySQL configurations pages, the Block Storage encryption page) state them
  with no location caveat. Each variant's section cites those pages. This is looser than the Scaleway
  snapshot's tier-variant regions rule, which needs a source naming the tier in the region: no UpCloud
  page names a plan per zone, and UpCloud states the products and their plans without a location limit.
- **Scope:** high availability is listed at the level the documentation states in full:
  `node-high-availability` when the offering survives the loss of a node with no manual failover,
  `zone-high-availability` when it keeps serving through the loss of a whole zone. Each UpCloud zone is
  one data centre, and no product documents a service that spans zones, so no offering lists zone level.
  A **Scope:** line on each high-availability capability says why.

## Declared EU regions

`de-fra1` (Frankfurt, Germany), `dk-cph1` (Copenhagen, Denmark), `es-mad1` (Madrid, Spain), `fi-hel1`
(Helsinki, Finland), `fi-hel2` (Helsinki, Finland), `nl-ams1` (Amsterdam, the Netherlands), `pl-waw1`
(Warsaw, Poland), `se-sto1` (Stockholm, Sweden).

- Page: <https://upcloud.com/docs/getting-started/locations/>. Under "Europe" it lists these eight zones
  and also `uk-lon1` (London) and `no-svg1` (Stavanger), which are left out: the United Kingdom and Norway
  are not EU member states. The Americas (`us-chi1`, `us-nyc1`, `us-sjo1`) and Asia-Pacific (`sg-sin1`,
  `au-syd1`) zones are left out too.
- The availability pages give the country of each zone (for example "de-fra1 - Frankfurt, Germany"),
  which confirms the split.
- Recent zones, from the changelog: Stockholm opened on 2023-11-15 and Copenhagen on 2025-12-16.

## Offerings

### Block Storage

Category `storage.block`. Pages: <https://upcloud.com/docs/products/block-storage/availability/>,
<https://upcloud.com/docs/products/block-storage/backups/>,
<https://upcloud.com/docs/products/block-storage/tiers/>,
<https://upcloud.com/docs/products/block-storage/storage-system/>,
<https://upcloud.com/docs/products/cloud-servers/storage/>.

The base offering is a storage device created without encryption, the default: the Cloud Servers storage
page says "You can optionally create storage devices with encryption at rest enabled". The tiers MaxIOPS,
Standard and Archive add no capability ("All storage tiers share the same production model", storage system page),
so none gets an offering.

- `snapshots`: the backups page says "Backups on UpCloud are instant one-to-one snapshots of an entire
  storage device", and "On-demand backups offer instant snapshots of the selected storage devices and can
  be taken manually at any time". **Needs:** nothing beyond the base, unencrypted storage.
- Regions `de-fra1`, `dk-cph1`, `es-mad1`, `fi-hel1`, `fi-hel2`, `nl-ams1`, `pl-waw1`, `se-sto1`: the
  availability page says "All Block Storage tiers are available in all UpCloud locations" and marks every
  tier available in each of these zones.

### Block Storage Encrypted

Category `storage.block`. A variant of *Block Storage* (tier-variant rule): a storage device created with
encryption at rest, an option that "can be enabled whenever a storage is created" and "cannot be enabled
after creation". Pages: <https://upcloud.com/docs/products/block-storage/encryption-at-rest/>,
<https://upcloud.com/docs/products/block-storage/backups/>,
<https://upcloud.com/docs/products/block-storage/availability/>.

- `encryption-at-rest`: the encryption page says "All block storage devices can be optionally encrypted at
  rest", using AES "with 256-bit keys". **Needs:** the encryption option.
- `snapshots`: as for *Block Storage*; the encryption page adds that "Backups and custom images created
  from an encrypted storage device are encrypted automatically", so backups hold on this option too.
  **Needs:** the encryption option, which keeps them.
- Regions `de-fra1`, `dk-cph1`, `es-mad1`, `fi-hel1`, `fi-hel2`, `nl-ams1`, `pl-waw1`, `se-sto1`
  (tier-variant zones): the availability page says "All Block Storage tiers are available in all UpCloud
  locations", and the encryption page says "Encryption is available for all block storage tiers", with no
  location caveat.

### Cloud Servers

Category `compute.vm`. Pages: <https://upcloud.com/docs/products/cloud-servers/availability/>,
<https://upcloud.com/docs/products/cloud-servers/configurations/>,
<https://upcloud.com/docs/products/networking/public-network/>,
<https://upcloud.com/docs/products/block-storage/backups/>.

- `ipv6`: the public network page says "Every Cloud Server is configured with 1 IPv4 address and 1 IPv6
  address by default", on the public network, which "allows access to the open Internet", and that "Each
  Cloud Server can have 0-5 IPv6 addresses". A public address that reaches the Internet carries traffic
  both ways, as ADR 0011's definition requires. The configurations page lists "1 public IPv4 & IPv6
  address" in the Starter and Premium plans, and public IP addresses as an add-on of the Cloud Native
  plans. **Needs:** nothing beyond the base offering.
- `snapshots`: every plan type lists backup options as an add-on (configurations page), and the backups
  page describes backups as "instant one-to-one snapshots of an entire storage device", on demand or
  scheduled. **Needs:** nothing beyond the base offering.
- Regions `de-fra1`, `dk-cph1`, `es-mad1`, `fi-hel1`, `fi-hel2`, `nl-ams1`, `pl-waw1`, `se-sto1`: the
  availability page says "Cloud Servers are available in all UpCloud locations", and marks the Starter,
  Premium and Cloud Native plans available in each of these zones.

### Managed Kubernetes

Category `kubernetes.managed`. Pages: <https://upcloud.com/docs/products/managed-kubernetes/availability/>,
<https://upcloud.com/docs/products/managed-kubernetes/control-plane/>,
<https://upcloud.com/docs/products/managed-kubernetes/billing/>.

The base offering is the lowest control-plane plan, **Development** (`dev-md`), which the control plane
page says "runs a single control plane instance, with one API server and one etcd member, so it has no
redundancy". The billing page lists it with "No high availability". It lists no capability: autoscaling
and encryption are left out (*Left out on purpose*), and high availability comes with the Production
plan, the variant *Managed Kubernetes Production*. Worker nodes are Cloud Servers (billing page); no
capability follows from that.

- Regions `de-fra1`, `dk-cph1`, `es-mad1`, `fi-hel1`, `fi-hel2`, `nl-ams1`, `pl-waw1`, `se-sto1`: the
  availability page says "Managed Kubernetes is available in all UpCloud locations" and marks each of
  these zones.

### Managed Kubernetes Production

Category `kubernetes.managed`. A variant of *Managed Kubernetes* (tier-variant rule): a cluster on the
**Production** control-plane plan (`prod-md-ha`). Pages:
<https://upcloud.com/docs/products/managed-kubernetes/control-plane/>,
<https://upcloud.com/docs/products/managed-kubernetes/billing/>,
<https://upcloud.com/docs/products/managed-kubernetes/availability/>,
<https://upcloud.com/docs/products/managed-kubernetes/data-plane/>,
<https://upcloud.com/docs/products/managed-kubernetes/faq/>.

- `node-high-availability`: the control plane page says the Production plan "runs a highly available
  control plane with three API servers and a three-member etcd cluster. The etcd members run on separate
  hosts." The billing page lists it with "High Availability". Three API servers and a three-member etcd
  on separate hosts keep the control plane serving through the loss of a host with no manual action.
  **Needs:** the Production plan. **Scope:** node level only. The cluster lives in one zone (its nodes
  are connected through private networks "in the same zone as the Managed Kubernetes cluster", data plane page, and "Worker nodes
  are deployed to the same zone as the cluster is located in", Kubernetes FAQ), and a
  zone is one data centre, so zone level is not claimed.
- Regions `de-fra1`, `dk-cph1`, `es-mad1`, `fi-hel1`, `fi-hel2`, `nl-ams1`, `pl-waw1`, `se-sto1`
  (tier-variant zones): the availability page says "Managed Kubernetes is available in all UpCloud
  locations", with one column for the product and none per plan, and the control plane page states both
  plans with no location caveat.

### Managed MySQL

Category `database.mysql`. Pages: <https://upcloud.com/docs/products/managed-mysql/availability/>,
<https://upcloud.com/docs/products/managed-mysql/configurations/>,
<https://upcloud.com/docs/products/managed-mysql/backups/>,
<https://upcloud.com/docs/products/managed-mysql/encryption/>,
<https://upcloud.com/docs/getting-started/encryption/>.

The base offering is a single-node service, the lowest tier: the configurations page says "Developer
compute plans run on a single node", and Standard and High Memory plans "can run on 1, 2, or 3 nodes". A
cluster of two or three nodes is the variant *Managed MySQL Multi-node*.

- `point-in-time-restore`: the backups page says "Each Managed Database plan includes automated backups",
  which "allows users to restore to any given point in time (PITR) within the selected retention window".
  A restore creates a new service. **Needs:** nothing beyond the base, single-node tier.
- `encryption-at-rest`: the encryption page says "Data-at-rest encryption covers both active service
  instances and backups", with full-volume LUKS encryption, and the encryption overview says "Managed
  Databases generate a unique key for each service instance and volume". **Needs:** nothing beyond the
  base, single-node tier.
- Regions `de-fra1`, `dk-cph1`, `es-mad1`, `fi-hel1`, `fi-hel2`, `nl-ams1`, `pl-waw1`, `se-sto1`: the
  availability page marks each of these zones available.

### Managed MySQL Multi-node

Category `database.mysql`. A variant of *Managed MySQL* (tier-variant rule): a cluster of two or three
nodes, which the documentation calls a multi-node plan. Pages:
<https://upcloud.com/docs/products/managed-mysql/high-availability/>,
<https://upcloud.com/docs/products/managed-mysql/faq/>,
<https://upcloud.com/docs/products/managed-mysql/configurations/>,
<https://upcloud.com/docs/products/managed-mysql/availability/>.

- `point-in-time-restore`, `encryption-at-rest`: as for *Managed MySQL*; the backups and encryption pages
  state them for every plan. **Needs:** nothing beyond the base tier, which this tier keeps.
- `node-high-availability`: the configurations page says a cluster of 2 or 3 nodes "has a primary node and
  standby nodes that take over automatically if the primary fails". The high availability page says the
  nodes "are created on physically separated backend hosts to ensure redundancy in case of hardware
  failure", that a standby is "elevated to the role of primary" and the hostname "instantly pointed
  towards the new primary node". The FAQ says "the automated fail-over is only possible with clusters of
  2 or more nodes". **Needs:** two or three nodes. **Scope:** node level only. Every node is in the
  service's one zone, a single data centre; off-site backups protect the data, but recovering from the
  loss of the zone is a restore into a new service, not failover (*Left out on purpose*).
- `read-replicas`: the high availability page says standby nodes "can serve as additional nodes dedicated
  to handling read-only queries", reached through a separate DNS entry that balances queries across the
  standby nodes. **Needs:** two or three nodes; a single node has no standby to read from.
- Regions `de-fra1`, `dk-cph1`, `es-mad1`, `fi-hel1`, `fi-hel2`, `nl-ams1`, `pl-waw1`, `se-sto1`
  (tier-variant zones): the availability page marks each of these zones, with no column per plan, and the
  configurations page states the 1-, 2- and 3-node forms with no location caveat.

### Managed Object Storage

Category `storage.object`. Pages: <https://upcloud.com/docs/products/managed-object-storage/>,
<https://upcloud.com/docs/products/managed-object-storage/availability/>,
<https://upcloud.com/docs/products/managed-object-storage/full-encryption/>,
<https://upcloud.com/docs/products/managed-object-storage/s3-standard-compatibility/>,
<https://upcloud.com/docs/guides/enable-and-manage-s3-object-versioning/>.

No storage classes or tiers are documented, so there are no variants.

- `encryption-at-rest`: the full encryption page says the service "encrypts all the data it stores at rest,
  using server-side encryption with the AES-256 algorithm. Encryption is always on." **Needs:** nothing.
- `object-versioning`: the S3 compatibility page lists "Bucket versioning, Object versioning" among the
  supported S3 APIs, and the versioning guide says "Versioning keeps every version of the files in a
  bucket." **Needs:** nothing.
- Regions `de-fra1`, `dk-cph1`, `fi-hel2`, `se-sto1`: the availability page says the service is
  "available in six regions. Each region is hosted in one location", and gives the hosting location of
  each. The four in the EU are the zones listed:
  - `EUROPE-1` is hosted in `fi-hel2`;
  - `EUROPE-2` is hosted in `de-fra1`;
  - `EUROPE-3` is hosted in `se-sto1`;
  - `EUROPE-4` is hosted in `dk-cph1`.

  `APAC-1` (in `sg-sin1`) and `US-1` (in `us-chi1`) are outside the EU. The "Accessible via SDN from"
  column, which names every European zone for each `EUROPE-n` region, `uk-lon1` and `no-svg1` included,
  is reach, not where data is stored, so it adds no zone (*Rules applied*).

### Managed PostgreSQL

Category `database.postgresql`. Pages:
<https://upcloud.com/docs/products/managed-postgresql/availability/>,
<https://upcloud.com/docs/products/managed-postgresql/configurations/>,
<https://upcloud.com/docs/products/managed-postgresql/backups/>,
<https://upcloud.com/docs/products/managed-postgresql/encryption/>,
<https://upcloud.com/docs/getting-started/encryption/>.

The base offering is a single-node service, the lowest tier: the configurations page says "Developer
compute plans run on a single node", and Standard and High Memory plans "can run on 1, 2, or 3 nodes". A
cluster of two or three nodes is the variant *Managed PostgreSQL Multi-node*.

- `point-in-time-restore`: the backups page says "Each Managed Database plan includes automated backups",
  which "allows users to restore to any given point in time (PITR) within the selected retention window".
  A restore creates a new service. **Needs:** nothing beyond the base, single-node tier.
- `encryption-at-rest`: the encryption page says "Data-at-rest encryption covers both active service
  instances and backups", with full-volume LUKS encryption, and the encryption overview says "Managed
  Databases generate a unique key for each service instance and volume". **Needs:** nothing beyond the
  base, single-node tier.
- Regions `de-fra1`, `dk-cph1`, `es-mad1`, `fi-hel1`, `fi-hel2`, `nl-ams1`, `pl-waw1`, `se-sto1`: the
  availability page marks each of these zones available.

### Managed PostgreSQL Multi-node

Category `database.postgresql`. A variant of *Managed PostgreSQL* (tier-variant rule): a cluster of two
or three nodes, which the documentation calls a multi-node plan. Pages:
<https://upcloud.com/docs/products/managed-postgresql/high-availability/>,
<https://upcloud.com/docs/products/managed-postgresql/faq/>,
<https://upcloud.com/docs/products/managed-postgresql/connection-pooling/>,
<https://upcloud.com/docs/products/managed-postgresql/configurations/>,
<https://upcloud.com/docs/products/managed-postgresql/availability/>.

- `point-in-time-restore`, `encryption-at-rest`: as for *Managed PostgreSQL*; the backups and encryption
  pages state them for every plan. **Needs:** nothing beyond the base tier, which this tier keeps.
- `node-high-availability`: the configurations page says a cluster of 2 or 3 nodes "has a primary node and
  standby nodes that take over automatically if the primary fails". The high availability page says the
  nodes "are created on physically separated backend hosts to ensure redundancy in case of hardware
  failure", that a standby is "elevated to the role of primary" and the hostname "instantly pointed
  towards the new primary node". The FAQ says "the automated fail-over is only possible with clusters of
  2 or more nodes". **Needs:** two or three nodes. **Scope:** node level only. Every node is in the
  service's one zone, a single data centre. If all nodes fail, the high availability page says they are
  re-provisioned from backup, "which may result in some data loss" (*Left out on purpose*).
- `read-replicas`: the high availability page says standby nodes "can serve as additional nodes dedicated
  to handling read-only queries", reached through a separate DNS entry; the connection pooling page says
  "On plans with two or more nodes, connecting through the read-only replica hostname runs your queries on
  the replica." **Needs:** two or three nodes; a single node has no standby to read from.
- Regions `de-fra1`, `dk-cph1`, `es-mad1`, `fi-hel1`, `fi-hel2`, `nl-ams1`, `pl-waw1`, `se-sto1`
  (tier-variant zones): the availability page marks each of these zones, with no column per plan, and the
  configurations page states the 1-, 2- and 3-node forms with no location caveat.

## Left out on purpose

- **No `serverless.function` or `messaging.queue` offering.** The product sections of the documentation
  (from <https://upcloud.com/docs/llms.txt> and <https://upcloud.com/docs/sitemap.xml>) are block storage,
  CDN, cloud servers, container registry, file storage, GPU servers, managed Kubernetes, managed load
  balancer, managed MySQL, managed object storage, managed OpenSearch, managed PostgreSQL, managed Valkey,
  NAT gateway, networking, private cloud and VPN gateway. None is a function-as-a-service or a message
  queue product. So a resource of either category gets fit none on UpCloud.
- **Managed Valkey is not a queue.** `llms.txt` describes it as "commonly used as a database, cache,
  streaming engine, or message broker". It is an in-memory data store, a category the taxonomy does not
  have, not a queue product with acknowledgement or a dead-letter queue, so it does not serve
  `messaging.queue`.
- **Other products not in the snapshot:** CDN, Container Registry, File Storage, GPU Servers (Cloud
  Servers already serve `compute.vm`), Managed Load Balancer, Managed OpenSearch, NAT Gateway, Private
  Cloud and VPN Gateway: no matching taxonomy category in scope.
- **Managed Kubernetes `autoscaling` (beta, user-installed).** The autoscaling page
  (<https://upcloud.com/docs/products/managed-kubernetes/autoscaling/>) describes Cluster Autoscaler as
  "an additional component" that "has to be configured & installed"; the guide
  (<https://upcloud.com/docs/guides/cluster-autoscaler/>) has the user apply manifests from a branch of
  UpCloud's autoscaler fork and run it on worker nodes. The only status statement is the changelog entry
  "Cluster Autoscaler beta released" (2023-09-26,
  <https://upcloud.com/docs/changelog/2023-09-26-cluster-autoscaler-beta/>), with no later general
  availability entry. It is neither generally available nor managed, so no tier lists it, and a
  `kubernetes.managed` resource that needs autoscaling is partial on UpCloud. OVHcloud and Scaleway both
  list it.
- **Block Storage `node-high-availability` (target rule).** The storage system page
  (<https://upcloud.com/docs/products/block-storage/storage-system/>) says data "is stored in clusters of
  two separate storage backends to ensure high performance and uninterrupted availability", with
  "tolerance against failures through the use of storage clusters". That describes copies of the data,
  but no page names a failure that an attached device keeps serving through with no manual action, so the
  capability is not stated in full, as for OVHcloud Block Storage. Zone level is not claimed either: a
  device is "bound to the location" it is created in (Cloud Servers storage page), and backups are stored
  "in the same data centre as the origin Cloud Server" (backups page).
- **`encryption-at-rest` on Cloud Servers.** A server's storage devices, the operating system disk
  included, can be created encrypted ("Any block storage device can be encrypted, including the storage
  device holding the Cloud Server's operating system", Block Storage encryption page). That is a feature
  of the storage device, which *Block Storage Encrypted* carries, not of the server, as for the OVHcloud
  and Scaleway Instances offerings. Leaving it off only understates the server.
- **`encryption-at-rest` on Managed Kubernetes.** The encrypted clusters page
  (<https://upcloud.com/docs/products/managed-kubernetes/encrypted-clusters/>) describes encryption of
  node storage and persistent volumes, chosen per cluster or node group and "currently only available
  over Terraform and API". It is an option of the storage the cluster uses, not of the managed control
  plane; neither OVHcloud nor Scaleway lists it on Kubernetes, and leaving it off only understates.
- **Cloud Servers `node-high-availability` and `zone-high-availability`.** The system architecture page
  (<https://upcloud.com/docs/products/cloud-servers/system-architecture/>) describes "Automatic transfer of
  Cloud Servers to healthy hosts" after compute host failures: the server is moved and restarted, not
  kept serving through the loss, as for the other targets' virtual machines. Live migrations are "performed
  only within the same data centre" (<https://upcloud.com/docs/products/cloud-servers/live-migration/>). No Cloud Server autoscaling product is documented either.
- **`zone-high-availability` on every offering.** Each zone is one data centre, and no product documents a
  service that keeps serving through the loss of its zone. For Managed PostgreSQL and MySQL, the backups
  pages say backups are kept "in another location in the same region", available "even in the unlikely
  event that the database's whole location goes down"; that is backup durability, and recovery is a
  restore into a new service, not failover without manual action. Managed Kubernetes documents no
  cluster spanning zones, and Managed Object Storage stores each region's data in one location.
- **Managed Object Storage `node-high-availability`.** No page read describes replication, durability or
  failover for the service. Object Lock and Bucket Replication are listed as unsupported S3 APIs (S3
  compatibility page).
- **`snapshots` on Managed PostgreSQL and MySQL.** No user snapshot feature is documented: backups are
  automated and restore is point-in-time into a new service. OVHcloud's databases do not list it either.
- **Features the vocabulary has no capability for**, left out: customer-managed keys (Object Storage
  server-side encryption with customer keys; LUKS inside a server), off-site database backups in another
  location, Kubernetes private node groups, node anti-affinity (documented as "best effort",
  <https://upcloud.com/docs/products/managed-kubernetes/data-plane/>), S3
  lifecycle policies, and PostgreSQL connection pooling.

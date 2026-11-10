# Scaleway catalog snapshot: sources

`catalog.json` is a hand-compiled, dated snapshot of what Scaleway offers, in taxonomy terms. This file
records where each fact comes from, so the snapshot can be checked and refreshed.

## How it was compiled

- **Retrieved on:** 2026-11-10.
- **Compiled, then verified against the live pages.** The snapshot was first compiled from Scaleway's
  documentation source and Go SDK at the commits below, because `www.scaleway.com` then answered HTTP 403.
  On the same date every region and capability was checked against the live public pages cited below
  (issue #15), without credentials and without calling any Scaleway API. The check changed two facts:
  Managed Database for PostgreSQL and MySQL gained the region `it-mil`, and Serverless Functions gained
  the capability `ipv6`. Their sections say why.
- **Availability page:** <https://www.scaleway.com/en/product-availability-by-region/>, the source of
  every offering's regions. It has one column per region (Paris, Amsterdam, Warsaw, Milan). For a zonal
  product it gives one mark per zone, and for a regional product it says "Available across the region".
  The legend has three marks: "Available", "Available soon" and "Not available". The page warns that
  "upcoming availability is based on our current roadmap and is subject to change". A plain HTTP client
  gets a JavaScript challenge instead of this page, so no copy of it was saved. It was read twice through
  a web-fetch tool, and both reads gave the same marks for every row cited below. A third read the same
  day was refused with HTTP 403. A reviewer should check the cells in a browser.
- **Documentation:** each page cited below was fetched live from `https://www.scaleway.com/en/docs/`, in
  the Markdown form the site publishes beside each page (`<page>.md`, linked from
  <https://www.scaleway.com/en/docs/llms.txt>). The documentation is built from
  [`scaleway/docs-content`](https://github.com/scaleway/docs-content). On the retrieval date its branch
  `main` was at commit `e149770a6eacd436b33ef8c36ad40cdff90ebdca`, the commit the snapshot was first
  compiled from, so the live pages and that source agree. A file `pages/<product>/<page>.mdx` is
  published at `https://www.scaleway.com/en/docs/<product>/<page>/`.
- **Changelog:** <https://www.scaleway.com/en/docs/changelog/>, read through its RSS feed
  <https://www.scaleway.com/en/docs/changelog/rss.xml>. The page draws its entries in the browser. The
  changelog gives the dates of launches and betas.
- **API reference and SDK, for cross-checking regions:** the API reference pages under
  <https://www.scaleway.com/en/developers/api/> with the OpenAPI schemas they publish, and Scaleway's
  official Go SDK, [`scaleway/scaleway-sdk-go`](https://github.com/scaleway/scaleway-sdk-go), at commit
  `f408edc91a855d0b585a8fd0458f7f4761c4eefe`. That commit was still the head of `main` on the retrieval
  date. Each product API declares the regions or zones it serves (`Regions()` or `Zones()` in the SDK,
  the `region` or `zone` enum in the schema). They agree with the availability page except in one case,
  recorded under Managed Database for PostgreSQL and MySQL.
- **`sourceURL`** remains the documentation root, because the capabilities come from the documentation.
  The regions come from the availability page, which each offering's section cites. The pricing page was
  not read, and it does not list per-product regions or features anyway.

## Licences of the sources

- **Documentation and changelog** (`www.scaleway.com/en/docs/`): built from `scaleway/docs-content`, which
  is licensed CC BY-NC-SA 4.0 (Creative Commons Attribution-NonCommercial-ShareAlike 4.0 International),
  as its `LICENSE.md` states at the commit above. GitHub's licence detection reports it only as "Other".
  The site's footer reads "© 2023-2026 – Scaleway".
- **Availability page and API reference** (`www.scaleway.com/en/product-availability-by-region/`,
  `www.scaleway.com/en/developers/api/`): under Scaleway's copyright, with no open licence stated. The
  availability page's footer reads "© 1999-2026 - Scaleway SAS.", and the API reference reads "© 1999 -
  2026 Scaleway. All rights reserved." Only facts are taken from these pages: where each product is
  available, and which regions each API accepts.
- `scaleway/scaleway-sdk-go` is licensed Apache-2.0. Only facts are taken from it: the regions and zones
  each API declares.
- **Rule for this file and `catalog.json`:** take facts only (which product exists, in which regions, with
  which feature) and brief quotations, each in quotation marks and attributed to the page it comes from.
  Paraphrase everything else. Never copy documentation or web text in bulk, and keep it out of
  `catalog.json`, which holds only names, taxonomy terms and region codes. A refresh follows the same rule.

## Rules applied

- **Regions:** an offering lists a region when the availability page marks the product "Available" there,
  either across the region or in at least one of its zones. "Available soon" does not count, and zones are
  not modelled. Each region was also compared with the SDK, the API reference and the changelog. Where
  they disagree with the availability page, the offering's section says so. Regions are within the
  declared list below.
- **Capabilities:** only from the taxonomy vocabulary, and only when the documentation describes the
  feature as generally available. Features in public or private beta are left out and listed under
  *Left out on purpose*. A capability may need a particular tier or option of the product; that is
  noted where it applies, as **Needs:** on the capability's line, and **Scope:** says where an offering
  meets only part of the taxonomy's definition.

## Declared EU regions

`fr-par` (Paris, France), `it-mil` (Milan, Italy), `nl-ams` (Amsterdam, the Netherlands), `pl-waw` (Warsaw,
Poland).

- The availability page has exactly these four regions, and labels each of them "3 AZ". SDK
  `scw/locality.go` defines the same four regions, each with three zones.
- `it-mil` is recent. The changelog entry *New region opening - MIL-1 is now available!* (2026-03-10)
  opens Milan with one zone, for "Instances, Kapsule, Block Storage, Object Storage, VPC, Databases, KMS,
  Audit Trail, and more". Serverless Containers and Container Registry arrived there on 2026-08-31. The
  entry *A full 3-AZ Region in Milan!* (2026-10-12) adds zones MIL-2 and MIL-3, naming Instances, Block
  Storage and the Object Storage Standard Multi-AZ class. Some pages still name only Paris, Amsterdam and
  Warsaw: the Instances FAQ, the Block Storage concepts page, and the introductions of the Kubernetes and
  Managed Database API references.

## Offerings

### Block Storage

Category `storage.block`. Pages: <https://www.scaleway.com/en/docs/block-storage/concepts/>,
<https://www.scaleway.com/en/docs/block-storage/faq/>.

- `snapshots`: the concepts page describes volumes that "can be snapshotted", and the FAQ covers snapshot
  export and import.
- Regions `fr-par`, `it-mil`, `nl-ams`, `pl-waw`: the availability page marks Block Storage 5k and Block
  Storage 15k available in all three zones of each of the four regions. SDK `api/block/v1` serves all
  twelve zones.

### Elastic Metal

Category `compute.vm` (dedicated servers running a general purpose operating system). Pages:
<https://www.scaleway.com/en/docs/elastic-metal/faq/>,
<https://www.scaleway.com/en/docs/elastic-metal/reference-content/elastic-metal-networking/>,
<https://www.scaleway.com/en/docs/ipam/reference-content/ipv6/>.

- `ipv6`: the FAQ says servers are "delivered with an IPv6 address (/128) pre-configured", and flexible
  IPv6 /64 subnets can be ordered. The page *IPv6 and the Scaleway ecosystem* lists Elastic Metal among
  the products that support IPv6.
- Regions `fr-par`, `nl-ams`, `pl-waw`: the availability page marks the Aluminium, Beryllium, Iridium and
  Lithium ranges available in two zones each of Paris, Amsterdam and Warsaw. It marks no Elastic Metal
  range available in Milan. SDK `api/baremetal/v1` serves fr-par-1, fr-par-2, nl-ams-1, nl-ams-2,
  pl-waw-2 and pl-waw-3.

### Instances

Category `compute.vm`. Pages: <https://www.scaleway.com/en/docs/instances/concepts/>,
<https://www.scaleway.com/en/docs/instances/how-to/use-flexips/>,
<https://www.scaleway.com/en/docs/ipam/reference-content/ipv6/>.

- `ipv6`: flexible routed IPv6 gives each Instance a public /64 prefix (use-flexips page). The page *IPv6
  and the Scaleway ecosystem* lists Instances among the products that support IPv6.
- `snapshots`: the concepts page defines snapshots and images built from them; the Elastic Metal FAQ
  contrasts Elastic Metal with "Instances, which have the snapshot feature".
- Regions `fr-par`, `it-mil`, `nl-ams`, `pl-waw`: the availability page marks the PRO2 range, among
  others, available in all three zones of Paris, Amsterdam and Warsaw. It marks the BASIC3-X,
  STANDARD3-X, COMPUTE3-X and MEMORY3-X ranges available in all three Milan zones, as well as in Paris
  and Amsterdam. SDK
  `api/instance/v1` serves every Paris, Amsterdam and Warsaw zone but only it-mil-1; both Milan changelog
  entries name Instances.

### Kubernetes Kapsule

Category `kubernetes.managed`. Pages: <https://www.scaleway.com/en/docs/kubernetes/concepts/>,
<https://www.scaleway.com/en/docs/kubernetes/faq/>,
<https://www.scaleway.com/en/docs/kubernetes/reference-content/kubernetes-control-plane-offers/>.

- `autoscaling`: pools autoscale between a minimum and maximum node count (concepts *Autoscale*, FAQ).
- `high-availability`. **Needs:** a Dedicated control plane (Dedicated 4, 8 or 16). Those tiers run two
  API server replicas "for HA" (control-plane offers page); the free Mutualized tier runs one API server
  replica, so a cluster on it does not have this capability. On every tier etcd keeps three replicas
  across zones, and regional clusters spread the control plane across zones.
- Regions `fr-par`, `it-mil`, `nl-ams`, `pl-waw`: the availability page marks Kapsule "Available across
  the region" in all four regions, and the changelog entry opening MIL-1 (2026-03-10) names Kapsule. SDK
  `api/k8s/v1` and the schema of the Kubernetes API reference
  (<https://www.scaleway.com/en/developers/api/kubernetes/>) accept all four regions, although the API
  reference's introduction names only Paris, Amsterdam and Warsaw.

### Managed Database for PostgreSQL and MySQL

Categories `database.mysql` and `database.postgresql`: one product for both engines, with the same
features. Pages: <https://www.scaleway.com/en/docs/managed-databases-for-postgresql-and-mysql/concepts/>,
<https://www.scaleway.com/en/docs/managed-databases-for-postgresql-and-mysql/faq/>,
<https://www.scaleway.com/en/docs/managed-databases-for-postgresql-and-mysql/how-to/manage-read-replicas/>.

- `encryption-at-rest`: LUKS volume encryption, generally available since 2025-01-16 (changelog).
- `high-availability`. **Needs:** the High Availability option (a standalone node can be upgraded to it,
  not back). **Scope: node-level only.** A hot standby node, kept current by synchronous replication,
  takes over when the main node fails (concepts *High availability*). The FAQ says the two nodes are in
  the same data centre, in separate racks. So the pair survives the failure of a node, not the loss of
  the data centre or zone: it meets the "node" half of the taxonomy's definition (surviving the loss of
  a node or zone), never the "zone" half.
- `read-replicas`: Read Replicas for PostgreSQL and MySQL Database Instances.
- `snapshots`: Database snapshots of the instance's volume (concepts *Database snapshot*).
- Regions `fr-par`, `it-mil`, `nl-ams`, `pl-waw`: the availability page marks the product "Available
  across the region" in all four regions. **Changed on verification:** `it-mil` was added. The first
  compilation listed only `fr-par`, `nl-ams` and `pl-waw`, from the SDK.
- **Unresolved conflict over `it-mil`.** The Managed Database API reference
  (<https://www.scaleway.com/en/developers/api/managed-database-postgre-mysql/>) says the product "is
  available in the Paris, Amsterdam and Warsaw regions". Its OpenAPI schema accepts only `fr-par`,
  `nl-ams` and `pl-waw`, and SDK `api/rdb/v1` declares the same three. Unlike the Kubernetes API, whose
  schema accepts `it-mil`, nothing in this API does. The changelog entry opening MIL-1 names "Databases"
  without saying which product. The snapshot follows the availability page, as the region rule says, but
  Milan rests on that page alone. Check it before relying on this offering in Milan.

### Object Storage

Category `storage.object`. Pages: <https://www.scaleway.com/en/docs/object-storage/concepts/>,
<https://www.scaleway.com/en/docs/object-storage/faq/>.

- `encryption-at-rest`: server-side encryption with SSE-C, SSE-ONE and SSE-KMS (FAQ *How can I encrypt
  my objects?*).
- `high-availability`. **Needs:** the Standard Multi-AZ storage class (S3 class `STANDARD`). The FAQ
  says that class spreads data across three Availability Zones, and that after "a complete data center
  failure in an AZ" Scaleway can still rebuild the data. Standard One Zone keeps data in three racks of
  a single zone, and Glacier is archive storage: objects in either class do not have this capability.
  The availability page marks the Standard Multi-AZ class available in all four regions, so the
  capability is not limited to some of the offering's regions.
- `object-versioning`: bucket versioning (concepts *Versioning*; the 2026-05-20 changelog adds full
  versioning and Object Lock support to the console).
- Regions `fr-par`, `it-mil`, `nl-ams`, `pl-waw`: the availability page marks the Standard Multi-AZ and
  Standard One Zone classes "Available across the region" in all four regions. The concepts page lists
  an S3 endpoint in each of the four regions. Object Storage has no API in the SDK (it is S3-compatible).

### Queues

Category `messaging.queue`. Pages: <https://www.scaleway.com/en/docs/queues/concepts/>,
<https://www.scaleway.com/en/docs/queues/faq/>.

- `dead-letter-queue`: dead-letter (undelivered-message) queues with a maximum receive count, available
  since 2025-04-01 (changelog).
- Regions `fr-par`, `nl-ams`: the availability page marks Queues available in Paris and Amsterdam, and
  "Not available" in Warsaw and Milan. SDK `api/mnq/v1beta1` `SqsAPI` serves the same two regions.

### Serverless Functions

Category `serverless.function`. Pages: <https://www.scaleway.com/en/docs/serverless-functions/faq/>,
<https://www.scaleway.com/en/docs/serverless-functions/reference-content/functions-autoscaling/>,
<https://www.scaleway.com/en/docs/ipam/reference-content/ipv6/>.

- `autoscaling`: functions scale between a minimum (down to zero) and maximum number of instances with
  demand.
- `ipv6`. **Added on verification.** The page *IPv6 and the Scaleway ecosystem* lists Serverless
  Functions among the products that support IPv6: "A function or container endpoint resolves to both
  IPv4 and IPv6 addresses (dual stack)". The first compilation left this capability out for want of a
  clear statement; it had not read this page. **Scope:** the page states it for a function's public
  endpoint, so for incoming traffic. It says nothing about IPv6 for traffic a function sends.
- Regions `fr-par`, `nl-ams`, `pl-waw`: the availability page marks Serverless Functions available in
  Paris, Amsterdam and Warsaw, and "Available soon" in Milan, so `it-mil` is not listed. SDK
  `api/function/v1beta1` serves the same three regions.

### Serverless SQL Database

Category `database.postgresql`: a PostgreSQL-compatible database with documented differences from
default PostgreSQL. Generally available since 2024-06-28 (changelog). Pages:
<https://www.scaleway.com/en/docs/serverless-sql-databases/faq/>,
<https://www.scaleway.com/en/docs/serverless-sql-databases/reference-content/serverless-sql-databases-overview/>,
<https://www.scaleway.com/en/docs/serverless-sql-databases/reference-content/known-differences/>.

- **Compatibility caveat:** the known-differences page lists what does not behave as in standard
  PostgreSQL, because of connection pooling and scaling to zero. In summary: no temporary tables or
  views, no subscriptions (so no change-data-capture), no prepared transactions, and no concurrent
  materialized-view refresh; `LISTEN`/`NOTIFY` delivery, advisory locks (and so state locking of the
  Terraform/OpenTofu `pg` backend) and session settings are not guaranteed; databases and users are
  managed through the console, API or IAM, not DDL; and some clients need recent versions for SNI. The
  page points to Managed Database for PostgreSQL where strict compatibility is needed. The
  `database.postgresql` category does not capture these differences; a workload that relies on any of
  them needs Managed Database for PostgreSQL instead.

- `autoscaling`: compute scales up and down with vCPU use, down to zero.
- Region `fr-par`: the availability page marks Serverless SQL Database available in Paris, "Not
  available" in Amsterdam and Warsaw, and "Available soon" in Milan. SDK `api/serverless_sqldb/v1alpha1`
  serves Paris only.

## Left out on purpose

- **Instances `autoscaling`:** Autoscaling groups are in public beta. The changelog entry *Autoscaling
  Groups in Public Beta* (2026-08-25) opens them in fr-par, nl-ams and pl-waw, no later entry announces
  general availability, and the Autoscaling Groups API guide
  (<https://www.scaleway.com/en/docs/instances/api-cli/getting-started-autoscaling-groups/>) still
  requires "Beta access".
- **Block Storage `encryption-at-rest`:** the Key Manager encryption page
  (<https://www.scaleway.com/en/docs/block-storage/api-cli/encrypt-volumes-with-key-manager/>, validated
  2026-10-15) says volume encryption "is currently in Private Beta, and is only available via the Block
  Storage API".
- **Elastic Metal `snapshots`:** the FAQ says Elastic Metal servers must be backed up differently from
  Instances, which have the snapshot feature.
- **Managed Database `point-in-time-restore` and `ipv6`:** no point-in-time restore is documented (backups
  are restored as taken; snapshots and clones copy the current state), and the FAQ says IPv6 is not
  supported.
- **Serverless SQL Database `point-in-time-restore`:** seven days of daily automated backups, no restore to
  an arbitrary moment.
- **`ipv6` on Kubernetes Kapsule, Object Storage, Queues and Serverless SQL Database:** the page *IPv6 and
  the Scaleway ecosystem* (<https://www.scaleway.com/en/docs/ipam/reference-content/ipv6/>) lists the
  products that support IPv6, and none of these is among them. It says that "Products other than those
  listed here do not officially support IPv6", and names Managed Databases and Kubernetes Kapsule among
  them.
- **Products not in the snapshot:** Kubernetes Kosmos (multi-cloud Kubernetes whose nodes may run
  outside Scaleway; Kapsule already serves the category); Serverless Containers and Serverless Jobs
  (containers and batch jobs, not per-invocation functions); NATS and Topics and Events (streams and
  pub/sub, not queues); Managed Database for Redis and Document Database (no matching taxonomy category).

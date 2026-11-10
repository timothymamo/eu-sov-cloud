# Scaleway catalog snapshot: sources

`catalog.json` is a hand-compiled, dated snapshot of what Scaleway offers, in taxonomy terms. This file
records where each fact comes from, so the snapshot can be checked and refreshed.

## How it was compiled

- **Retrieved on:** 2026-11-10.
- **Live pages were not reachable.** From the environment that compiled the snapshot,
  `www.scaleway.com` answered HTTP 403 to every request, including the pricing page
  (<https://www.scaleway.com/en/pricing/>), the availability page
  (<https://www.scaleway.com/en/product-availability-by-region/>) and the documentation.
- **What was read instead:** two public sources published by Scaleway, at fixed commits.
  - The source of Scaleway's documentation, which is what <https://www.scaleway.com/en/docs/> publishes:
    [`scaleway/docs-content`](https://github.com/scaleway/docs-content) at commit
    `e149770a6eacd436b33ef8c36ad40cdff90ebdca`. A file `pages/<product>/<page>.mdx` is published at
    `https://www.scaleway.com/en/docs/<product>/<page>/`, and `changelog/` holds the product changelog.
  - Scaleway's official Go SDK, [`scaleway/scaleway-sdk-go`](https://github.com/scaleway/scaleway-sdk-go)
    at commit `f408edc91a855d0b585a8fd0458f7f4761c4eefe` (branch `main`). Each product API is generated
    from Scaleway's API definitions and declares the regions or zones it serves (`Regions()` or `Zones()`).
- **`sourceURL`** is the documentation root, because the documentation is where the data came from. The
  pricing page was not read, and it does not list per-product regions or features anyway.
- **Must be checked before release.** The snapshot was compiled without access to the live web pages. Check
  every region against the availability page and every capability against the product's documentation
  before relying on this data in a release.

## Licences of the sources

- **`scaleway/docs-content` is licensed CC BY-NC-SA 4.0** (Creative Commons
  Attribution-NonCommercial-ShareAlike 4.0 International), as its `LICENSE.md` states at the commit above.
  GitHub's licence detection reports it only as "Other".
- **Rule for this file and `catalog.json`:** take facts only (which product exists, in which regions, with
  which feature) and brief quotations, each in quotation marks and attributed to the page it comes from.
  Paraphrase everything else. Never copy documentation text in bulk, and keep it out of `catalog.json`,
  which holds only names, taxonomy terms and region codes. A refresh follows the same rule.
- `scaleway/scaleway-sdk-go` is licensed Apache-2.0. Only facts are taken from it: the regions and zones
  each API declares.

## Rules applied

- **Regions:** an offering lists a region when Scaleway's SDK says the product's API serves that region
  (or at least one zone in it), or the documentation names an endpoint there. Zones are not modelled.
  Regions are within the declared list below.
- **Capabilities:** only from the taxonomy vocabulary, and only when the documentation describes the
  feature as generally available. Features in public or private beta are left out and listed under
  *Left out on purpose*. A capability may need a particular tier or option of the product; that is
  noted where it applies, as **Needs:** on the capability's line, and **Scope:** says where an offering
  meets only part of the taxonomy's definition.

## Declared EU regions

`fr-par` (Paris, France), `it-mil` (Milan, Italy), `nl-ams` (Amsterdam, the Netherlands), `pl-waw` (Warsaw,
Poland).

- SDK `scw/locality.go` defines exactly these four regions, each with three zones.
- `it-mil` is recent: Serverless Containers and Container Registry arrived there on 2026-08-31, and the
  changelog entry *A full 3-AZ Region in Milan!* (2026-10-12) announces zones MIL-2 and MIL-3 with
  Instances, Block Storage and the Object Storage Standard Multi-AZ class. Some documentation pages still
  name only Paris, Amsterdam and Warsaw (for example the Instances FAQ).

## Offerings

### Block Storage

Category `storage.block`. Pages: <https://www.scaleway.com/en/docs/block-storage/concepts/>,
<https://www.scaleway.com/en/docs/block-storage/faq/>.

- `snapshots`: the concepts page describes volumes that "can be snapshotted", and the FAQ covers snapshot
  export and import.
- Regions `fr-par`, `it-mil`, `nl-ams`, `pl-waw`: SDK `api/block/v1` serves all twelve zones; the Milan
  changelog entry names Block Storage.

### Elastic Metal

Category `compute.vm` (dedicated servers running a general purpose operating system). Pages:
<https://www.scaleway.com/en/docs/elastic-metal/faq/>,
<https://www.scaleway.com/en/docs/elastic-metal/reference-content/elastic-metal-networking/>.

- `ipv6`: the FAQ says servers are "delivered with an IPv6 address (/128) pre-configured", and flexible
  IPv6 /64 subnets can be ordered.
- Regions `fr-par`, `nl-ams`, `pl-waw`: SDK `api/baremetal/v1` serves fr-par-1, fr-par-2, nl-ams-1,
  nl-ams-2, pl-waw-2 and pl-waw-3; no Milan zone.

### Instances

Category `compute.vm`. Pages: <https://www.scaleway.com/en/docs/instances/concepts/>,
<https://www.scaleway.com/en/docs/instances/how-to/use-flexips/>.

- `ipv6`: flexible routed IPv6 gives each Instance a public /64 prefix (use-flexips page).
- `snapshots`: the concepts page defines snapshots and images built from them; the Elastic Metal FAQ
  contrasts Elastic Metal with "Instances, which have the snapshot feature".
- Regions `fr-par`, `it-mil`, `nl-ams`, `pl-waw`: SDK `api/instance/v1` serves all Paris, Amsterdam and
  Warsaw zones and it-mil-1; the Milan changelog entry names Instances.

### Kubernetes Kapsule

Category `kubernetes.managed`. Pages: <https://www.scaleway.com/en/docs/kubernetes/concepts/>,
<https://www.scaleway.com/en/docs/kubernetes/faq/>,
<https://www.scaleway.com/en/docs/kubernetes/reference-content/kubernetes-control-plane-offers/>.

- `autoscaling`: pools autoscale between a minimum and maximum node count (concepts *Autoscale*, FAQ).
- `high-availability`. **Needs:** a Dedicated control plane (Dedicated 4, 8 or 16). Those tiers run two
  API server replicas "for HA" (control-plane offers page); the free Mutualized tier runs one API server
  replica, so a cluster on it does not have this capability. On every tier etcd keeps three replicas
  across zones, and regional clusters spread the control plane across zones.
- Regions `fr-par`, `it-mil`, `nl-ams`, `pl-waw`: SDK `api/k8s/v1` serves all four regions. Milan rests
  on the SDK alone; the Milan changelog entry does not name Kapsule.

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
- Regions `fr-par`, `nl-ams`, `pl-waw`: SDK `api/rdb/v1` serves these three regions.

### Object Storage

Category `storage.object`. Pages: <https://www.scaleway.com/en/docs/object-storage/concepts/>,
<https://www.scaleway.com/en/docs/object-storage/faq/>.

- `encryption-at-rest`: server-side encryption with SSE-C, SSE-ONE and SSE-KMS (FAQ *How can I encrypt
  my objects?*).
- `high-availability`. **Needs:** the Standard Multi-AZ storage class (S3 class `STANDARD`). The FAQ
  says that class spreads data across three Availability Zones, one data centre each, and can rebuild it
  after a whole data centre fails. Standard One Zone keeps data in three racks of a single zone, and
  Glacier is archive storage: objects in either class do not have this capability.
- `object-versioning`: bucket versioning (concepts *Versioning*; the 2026-05-20 changelog adds full
  versioning and Object Lock support to the console).
- Regions `fr-par`, `it-mil`, `nl-ams`, `pl-waw`: the concepts page lists an S3 endpoint in each. Object
  Storage has no API in the SDK (it is S3-compatible).

### Queues

Category `messaging.queue`. Pages: <https://www.scaleway.com/en/docs/queues/concepts/>,
<https://www.scaleway.com/en/docs/queues/faq/>.

- `dead-letter-queue`: dead-letter (undelivered-message) queues with a maximum receive count, available
  since 2025-04-01 (changelog).
- Regions `fr-par`, `nl-ams`: SDK `api/mnq/v1beta1` `SqsAPI` serves these two regions.

### Serverless Functions

Category `serverless.function`. Pages: <https://www.scaleway.com/en/docs/serverless-functions/faq/>,
<https://www.scaleway.com/en/docs/serverless-functions/reference-content/functions-autoscaling/>.

- `autoscaling`: functions scale between a minimum (down to zero) and maximum number of instances with
  demand.
- Regions `fr-par`, `nl-ams`, `pl-waw`: SDK `api/function/v1beta1` serves these three regions.

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
- Region `fr-par`: SDK `api/serverless_sqldb/v1alpha1` serves Paris only.

## Left out on purpose

- **Instances `autoscaling`:** Autoscaling groups are in public beta (since 2026-08-25, fr-par, nl-ams and
  pl-waw).
- **Block Storage `encryption-at-rest`:** volume encryption with Key Manager is in private beta and API-only.
- **Elastic Metal `snapshots`:** the FAQ says Elastic Metal servers must be backed up differently from
  Instances, which have the snapshot feature.
- **Managed Database `point-in-time-restore` and `ipv6`:** no point-in-time restore is documented (backups
  are restored as taken; snapshots and clones copy the current state), and the FAQ says IPv6 is not
  supported.
- **Serverless SQL Database `point-in-time-restore`:** seven days of daily automated backups, no restore to
  an arbitrary moment.
- **Kubernetes Kapsule `ipv6` and Serverless Functions `ipv6`:** no clear statement. The Kapsule pages
  read (concepts, FAQ, control-plane offers, multi-AZ clusters, Private Networks) state no IPv6 support.
  The Functions changelog (2024-11-21) says only "compatible with IPv6 environments".
- **Object Storage `ipv6`:** not stated in the concepts or FAQ pages.
- **Products not in the snapshot:** Kubernetes Kosmos (multi-cloud Kubernetes whose nodes may run
  outside Scaleway; Kapsule already serves the category); Serverless Containers and Serverless Jobs
  (containers and batch jobs, not per-invocation functions); NATS and Topics and Events (streams and
  pub/sub, not queues); Managed Database for Redis and Document Database (no matching taxonomy category).

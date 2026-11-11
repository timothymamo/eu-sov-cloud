# PRD #77: Assess an inventory file against the UpCloud catalog

**Status:** Complete
**Issue:** #77

## Problem

`eusov` answers "what can move?" for two of the three targets the project names: Scaleway (PRD #2) and
OVHcloud (PRD #11). A platform engineer comparing EU providers also needs the answer for UpCloud, and
today `--target upcloud` does not exist. `internal/adapter/target/upcloud` holds only a `doc.go`.

UpCloud is the third user of the shared snapshot format (ADR 0007) and the first target written after
PRD #14 changed the capability vocabulary (ADR 0011). It tests both. If UpCloud needs a change to the
snapshot format, the reader, `targettest`, the domain or the reporter, those were shaped around the first
two targets. If its snapshot cannot follow ADR 0011's target and tier-variant rules as they are written,
the rules need work before the live source adapters depend on them.

UpCloud is also built differently from the other two. Each of its locations is one data centre, called a
*zone* (`de-fra1`, `fi-hel2`). There are no multi-zone regions like OVHcloud's 3-AZ regions or
Scaleway's multi-AZ Paris. Object Storage is the exception: it has regions of its own (`EUROPE-1` …),
each hosted in one zone. A snapshot that copied the other targets' shape without thought could overstate
zone-level availability.

## Outcome

A user can run `eusov assess --inventory inventory.json --target upcloud` and get the same kind of report
as for Scaleway and OVHcloud. It comes from an embedded, dated snapshot of UpCloud's public cloud
products in its EU zones, and runs offline with no credentials. Resources in a category UpCloud has no
offering for (serverless functions and message queues, as of this PRD) are reported with fit `none` and
listed as blockers. UpCloud reads its snapshot through the shared reader and passes `targettest` without
changes. Adding UpCloud changes nothing in the domain, the use case, the reporter or the snapshot format.

## Scope

| | In scope | Out of scope |
|---|---|---|
| Source providers | None live. The versioned inventory file (ADR 0006). | Live discovery from AWS, Azure or GCP |
| Target providers | UpCloud: **Cloud Servers**, **Managed Kubernetes**, **Block Storage**, **Managed Object Storage** and **Managed Databases** for PostgreSQL and MySQL, from an embedded, dated catalog snapshot. | GPU Servers, Private Cloud and Private Cloud GPUs, File Storage, Managed Valkey and OpenSearch, Load Balancer, NAT and VPN Gateway, CDN, Container Registry, Simple Backup. Live catalog or pricing APIs. Assessing several targets in one run. |
| Regions | UpCloud zones in **EU member states**. On [UpCloud's locations page](https://upcloud.com/docs/getting-started/locations/) on 2026-11-11: Amsterdam `nl-ams1` (Netherlands), Copenhagen `dk-cph1` (Denmark), Frankfurt `de-fra1` (Germany), Helsinki `fi-hel1` and `fi-hel2` (Finland), Madrid `es-mad1` (Spain), Stockholm `se-sto1` (Sweden), Warsaw `pl-waw1` (Poland). | London `uk-lon1` (United Kingdom). Stavanger `no-svg1` (Norway, which is in the EEA but not in the EU). The Americas and Asia-Pacific zones. |
| Taxonomy categories | The current categories, unchanged. The snapshot must serve `compute.vm`, `kubernetes.managed`, `storage.object`, `storage.block`, `database.postgresql` and `database.mysql`. `serverless.function` and `messaging.queue` are expected to have no UpCloud offering. | New categories (for example caches, search, file storage or load balancers). New capabilities. |
| Capabilities | The current vocabulary, applied under ADR 0011: scoped high availability, the target rule, the tier-variant rule and strict definitions. | Changes to the vocabulary |
| Assessment and output | The current fit rules and Markdown report, unchanged | JSON report, readiness score, sizing, pricing, a comparison across targets |

## Acceptance criteria

All criteria are pinned by tests that run offline under `make test` or `make test-e2e`, unless marked
*review*.

**Target selection**

- [x] Given a valid inventory file, when I run `eusov assess --inventory <file> --target upcloud`, then
  the command exits 0 and prints a Markdown report naming the target `UpCloud`, the snapshot's source URL
  and its retrieval date.
- [x] `--target` accepts `ovhcloud`, `scaleway` and `upcloud`. Given an unknown target, the command exits
  2 and the usage error lists all three, sorted.
- [x] `eusov assess --target upcloud` needs no credentials or network. The e2e test runs it with an empty
  environment and gets the same report.

**Catalog**

- [x] The UpCloud catalog serves `compute.vm`, `kubernetes.managed`, `storage.object`, `storage.block`,
  `database.postgresql` and `database.mysql`, each with at least one offering (`targettest` *category*
  check).
- [x] Every offering is available only in the declared EU zones (`targettest` *regions* check). No
  offering lists `uk-lon1`, `no-svg1`, or a zone in the Americas or Asia-Pacific. A test pins the declared
  region list, so any change to it shows up in review.
- [x] Given a resource of category `serverless.function` or `messaging.queue`, its fit is `none` with
  the reason "no offering in the UpCloud catalog for `<category>`", and it is listed as a blocker.
- [x] No offering lists `zone-high-availability` unless its `SOURCES.md` section quotes documentation
  that states the service keeps serving through the loss of a whole zone or data centre without manual
  failover (ADR 0011's target rule). A test pins the list of offerings that claim it, which is empty if
  none qualifies.
- [x] Every capability that holds only on some plans is listed on a tier variant named after that plan,
  and the base offering does not list it (ADR 0011's tier-variant rule). For example, a database plan
  with more than one node is a variant if it is the only plan with high availability or read replicas.
  Tests pin the best offering for a resource that needs each such capability, and for one that needs
  none of them.
- [x] A golden file pins the parsed UpCloud catalog, as for the other targets.
- [x] Every offering in the snapshot has its own section in `internal/adapter/target/upcloud/SOURCES.md`.
  That section names the pages the offering comes from, the fact behind each capability and region, and
  the licence of each source. The existing test that every offering has a section runs for UpCloud.

**Report**

- [x] Given the same fixture inventory, the report for `--target upcloud` has the same sections, columns
  and resource order as the reports for the other two targets. Only the target name, the snapshot's
  source and date, the offerings, the fits and the reasons differ. Pinned by an e2e golden report of the
  same fixture against all three targets.
- [x] No report text or error message names another target when the target is UpCloud, or names UpCloud
  when the target is another one.
- [x] Given the same inventory twice with `--target upcloud`, the two reports are byte-identical.

**Architecture**

- [x] `make check` passes, including depguard and `test/architecture` for every release target.
- [x] The UpCloud adapter passes `targettest` as it stands. If this PRD changes the suite, the change
  only adds checks, and all three adapters pass them.
- [x] *Review:* adding UpCloud changes no file under `internal/domain`, `internal/app`,
  `internal/adapter/report` or the shared snapshot reader. Needing such a change is a finding against
  the earlier design and gets an ADR.
- [x] The `go list -deps` check covers the UpCloud adapter on the assess path: no `net/http`, no
  `os/exec`, no cloud SDK.

## Milestones

Each milestone ships as one PR. Both depend on this PRD being merged first.

1. [x] **UpCloud catalog snapshot and adapter.** `internal/adapter/target/upcloud` serves an embedded,
   dated snapshot of the products under *Scope*, in the EU zones under *Scope*, read through the shared
   reader. Its `SOURCES.md` is written to the rules under *Decisions*. A golden test pins the parsed
   catalog. The adapter runs `targettest` with the categories listed under *Scope*, a test pins the
   declared region list, and tests pin the tier-variant and zone-level criteria. The milestone confirms
   that no generally available UpCloud product serves `serverless.function` or `messaging.queue`. If one
   does, the same PR updates this PRD's scope and criteria. The reviewer checks every zone against
   UpCloud's locations page and each product's availability page, and every capability against the
   product documentation. The adapter's `doc.go` says how to refresh the snapshot.
2. [x] **CLI, e2e and docs.** `--target upcloud` is wired in `cmd/eusov`, and the usage error lists all
   three targets. The e2e test runs one fixture inventory against all three targets and compares the
   output with golden reports. For UpCloud the fixture exercises direct, partial, `none` (from a category
   UpCloud does not serve) and unclassified. The `go list -deps` check includes the UpCloud adapter, and
   the README and `CLAUDE.md` status lines list UpCloud. Moves this PRD to `prds/done/` and closes #77.
   Depends on 1.

## Decisions and open questions

- **Decided: the products under *Scope* only.** They are UpCloud's region-based, API-driven services that
  match the six categories the other targets serve. GPU Servers are Cloud Servers with dedicated GPUs, in
  fewer zones and with no GPU capability in the vocabulary. They are left out so that a `compute.vm`
  resource is never pointed at a GPU plan. Private Cloud is single-tenant and sold per host, like
  OVHcloud Hosted Private Cloud (out of scope in PRD #11). The other products have no taxonomy category.
  Each can follow in its own PRD.
- **Decided: EU member states only. The UK and Norway are out.** This is the rule from PRD #11, applied
  as it stands. Norway is in the EEA but not in the EU, so `no-svg1` is out for the same reason as the
  UK. Revisit both together, with PRD #2's open question on region semantics, if users ask for EEA or
  adequacy-based residency.
- **Decided: regions are UpCloud zone IDs.** Every product but Object Storage is placed by zone, and a
  zone is one data centre. The snapshot uses the zone IDs from the locations page (`de-fra1`, `fi-hel1`
  …), lowercase, as its region codes. Object Storage's own regions are each hosted in one zone (on
  2026-11-11 its availability page lists `EUROPE-1` in `FI-HEL2`, `EUROPE-2` in `DE-FRA1`, `EUROPE-3` in
  `SE-STO1` and `EUROPE-4` in `DK-CPH1`). So the Object Storage offering lists the zone that hosts each
  region, and `SOURCES.md` records which Object Storage region each zone stands for. The report says
  where an offering exists, not which endpoint to call. This is PRD #11's "one region name per location"
  decision, adapted to UpCloud.
- **Decided: ADR 0011 applies as written.** UpCloud is the first target written after PRD #14, so its
  snapshot follows the target rule, the tier-variant rule (variants named `<base offering> <tier>`) and
  strict definitions from the start. Every **Needs:** and **Scope:** line in `SOURCES.md` either becomes
  a tier variant or leads to a capability that is not listed. Because each zone is one data centre,
  `zone-high-availability` is expected nowhere: two nodes in one data centre survive the loss of a node,
  not of the zone. Milestone 1 checks this per product. Helsinki's two zones (`fi-hel1`, `fi-hel2`)
  count as zone level only if UpCloud documents a service that fails over between them without manual
  action.
- **Decided: the taxonomy and the snapshot format are unchanged.** A capability UpCloud offers that has
  no vocabulary entry is left out and noted in `SOURCES.md`. Extending the vocabulary, or needing a
  snapshot v2, is a change of its own with an ADR.
- **Decided: snapshot sources and licences.** On 2026-11-11, `upcloud.com/docs` (including each
  product's *Availability* page) and `developers.upcloud.com` answered `curl` from the development VM
  with HTTP 200. Some fetch tools got HTTP 403 from `upcloud.com`, so `SOURCES.md` says how each page was
  retrieved. The documentation pages say "All rights reserved", so `SOURCES.md` takes only facts and
  brief attributed quotes, and states that licence for every source. No authenticated UpCloud API is
  called (CLAUDE.md rule 3). The zone list in the API documentation's examples is older than the
  locations page and is not a source.
- **Accepted risk: the snapshot goes stale.** As for the other targets, the report shows the retrieval
  date, and the adapter's `doc.go` documents how to refresh the snapshot.
- **Open: several targets in one run** (`--target ovhcloud,scaleway,upcloud`, or a comparison report).
  PRD #11 left this until UpCloud existed, so that the report shape is designed for three targets. Once
  this PRD is done, it gets its own PRD.

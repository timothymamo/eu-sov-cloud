# PRD #11: Assess an inventory file against the OVHcloud catalog

**Status:** Complete
**Issue:** #11

## Problem

After PRD #2, `eusov` answers "what can move?" for one target only, Scaleway. A platform engineer choosing
an EU provider needs the same answer for every candidate. OVHcloud is the second of the three targets the
project names, and today `--target ovhcloud` does not exist.

A second target also tests PRD #2's design. The target port, the `targettest` contract suite, the
embedded snapshot format and the target-neutral reasons were all written with one adapter in view. If a
second adapter needs changes to the domain, the reporter or the suite, those were shaped around Scaleway.
If it copies Scaleway's snapshot parser, the two copies drift apart.

## Outcome

A user can run `eusov assess --inventory inventory.json --target ovhcloud` and get the same kind of report
PRD #2 gives for Scaleway. It comes from an embedded, dated snapshot of OVHcloud Public Cloud in its EU
regions, and runs offline with no credentials. Resources in a category OVHcloud has no offering for
(serverless functions and message queues, as of this PRD) are reported with fit `none` and listed as
blockers. Both targets read their snapshots through one shared format and reader. Both pass `targettest`
without changes. Adding OVHcloud changes nothing in the domain, the use case or the reporter.

## Scope

| | In scope | Out of scope |
|---|---|---|
| Source providers | None live. The versioned inventory file from PRD #2. | Live discovery from AWS, Azure or GCP |
| Target providers | OVHcloud **Public Cloud**, from an embedded, dated catalog snapshot. Scaleway only to move its snapshot onto the shared reader. | UpCloud (own PRD). OVHcloud Bare Metal dedicated-server ranges, Hosted Private Cloud, Web Cloud and Local Zones. Live catalog or pricing APIs. Assessing several targets in one run. |
| Regions | OVHcloud Public Cloud regions in **EU member states**. On [OVHcloud's regions page](https://www.ovhcloud.com/en/public-cloud/regions-availability/) on 2026-11-10: Gravelines, Roubaix, Strasbourg and Paris (France), Frankfurt/Limburg (Germany), Milan (Italy), Warsaw (Poland). | United Kingdom (London/Erith), Canada, United States and Asia-Pacific regions. Local Zones. |
| Taxonomy categories | PRD #2's categories, unchanged. The snapshot must serve `compute.vm`, `kubernetes.managed`, `storage.object`, `storage.block`, `database.postgresql` and `database.mysql`. `serverless.function` and `messaging.queue` are expected to have no OVHcloud offering. | New categories (for example a stream category for Managed Kafka). New capabilities. |
| Assessment and output | PRD #2's fit rules and Markdown report, unchanged | JSON report, readiness score, sizing, pricing |

## Acceptance criteria

All criteria are pinned by tests that run offline under `make test` or `make test-e2e`, unless marked
*review*.

**Target selection**

- [x] Given a valid inventory file, when I run `eusov assess --inventory <file> --target ovhcloud`, then
  the command exits 0 and prints a Markdown report naming the target `OVHcloud`, the snapshot's source URL
  and its retrieval date.
- [x] `--target` accepts `ovhcloud` and `scaleway`. Given an unknown target, the command exits 2 and the
  usage error lists both, sorted.
- [x] `eusov assess --target ovhcloud` needs no credentials or network. The e2e test runs it with an empty
  environment and gets the same report.

**Catalog**

- [x] The OVHcloud catalog serves `compute.vm`, `kubernetes.managed`, `storage.object`, `storage.block`,
  `database.postgresql` and `database.mysql`, each with at least one offering (`targettest` *category*
  check).
- [x] Every offering is available only in the declared EU regions (`targettest` *regions* check). No
  offering lists a UK, Canadian, US or Asia-Pacific region. A test pins the declared region list, so any
  change to it shows up in review.
- [x] Given a resource of category `serverless.function` or `messaging.queue`, its fit is `none` with
  the reason "no offering in the OVHcloud catalog for `<category>`", and it is listed as a blocker.
- [x] Given a resource that needs `high-availability` from a product OVHcloud makes highly available
  only in its 3-AZ regions, the best offering reported is the 3-AZ variant (see *Decisions*).
- [x] A golden file pins the parsed OVHcloud catalog, as for Scaleway.
- [x] Every offering in an embedded snapshot has its own section in the `SOURCES.md` beside it. That
  section names the pages the offering comes from, the fact behind each capability and region, and the
  licence of each source. A test fails when an offering has no section. The test runs for both targets.

**Shared snapshot format**

- [x] Both target adapters read their embedded snapshot through one reader, with the same schema
  version and the same strictness. An unknown field, an unsupported schema version, an offering in an
  undeclared region, or a value the taxonomy or `catalog.New` refuses is an error and is never dropped
  or defaulted. Errors are one line, worded the same for both targets. The reader's own tests pin this
  once. Each adapter's test pins that its embedded snapshot parses.
- [x] Moving Scaleway onto the shared reader changes nothing observable: in that milestone's diff, its
  catalog golden file and its `targettest` expectations are unchanged.

**Report**

- [x] Given one fixture inventory, the report for `--target ovhcloud` has the same sections, columns and
  resource order as the report for `--target scaleway`. Only the target name, the snapshot's source and
  date, the offerings, the fits and the reasons differ. Pinned by e2e golden reports of the same fixture
  against both targets.
- [x] No report text or error message says "Scaleway" when the target is OVHcloud, or "OVHcloud" when
  it is Scaleway.
- [x] Given the same inventory twice with `--target ovhcloud`, the two reports are byte-identical.

**Architecture**

- [x] `make check` passes, including depguard.
- [x] The OVHcloud adapter passes `targettest` as it stands. If this PRD changes the suite, the change
  only adds checks, and both adapters pass them.
- [x] *Review:* adding OVHcloud changes no file under `internal/domain`, `internal/app` or
  `internal/adapter/report`. The taxonomy, the capability vocabulary, the mapping rules and the reporter
  are reused as they are. Needing such a change is a finding against PRD #2's design and gets an ADR.
- [x] The `go list -deps` test from PRD #2 milestone 7 covers the OVHcloud adapter on the assess path:
  no `net/http`, no `os/exec`, no cloud SDK.

## Milestones

Each milestone ships as one PR. This PRD starts after PRD #2: milestone 1 needs PRD #2 milestone 5
(Scaleway catalog adapter) merged, and milestone 3 needs PRD #2 milestone 7 (CLI and wiring).

1. [x] **Shared target snapshot format.** Move the Scaleway adapter's snapshot schema and strict reader
   into one package that every target with an embedded snapshot uses. An ADR records it as the target
   snapshot format, the counterpart of ADR 0006 for inventories, and says why it is shared. Scaleway
   reads its snapshot through the shared reader, and its golden file and `targettest` run stay the same.
   Adds the test that every offering in a snapshot has a section in its `SOURCES.md`, run for Scaleway.
   The reader keeps the Scaleway adapter's size cap and one-line error rules.
2. [x] **OVHcloud catalog snapshot and adapter.** `internal/adapter/target/ovhcloud` serves an embedded,
   dated snapshot of OVHcloud Public Cloud in its EU regions, read through the shared reader. Its
   `SOURCES.md` is written to the rules under *Decisions*, and a golden test pins the parsed catalog.
   It runs `targettest` with the categories listed under *Scope*, and a test pins the declared region
   list. The milestone confirms that no generally available offering serves `serverless.function` or
   `messaging.queue`. If one does, it updates this PRD's scope and criteria in the same PR. The reviewer
   checks every region against OVHcloud's published regions page and every capability against the
   product documentation. Depends on 1.
3. [x] **CLI, e2e and docs.** `--target ovhcloud` is wired in `cmd/eusov`, and the usage error lists
   both targets. The e2e test runs one fixture inventory against both targets and compares the output
   with golden reports. For OVHcloud the fixture exercises direct, partial, `none` (from a category
   OVHcloud does not serve) and unclassified. The `go list -deps` check includes the OVHcloud adapter,
   and the README usage section lists OVHcloud. Moves this PRD to `prds/done/` and closes #11. Depends
   on 2 and on PRD #2 milestone 7.

## Decisions and open questions

- **Decided: Public Cloud only.** OVHcloud Public Cloud is the region-based, API-driven product line
  that corresponds to the hyperscaler services the taxonomy covers. Instances and Metal Instances serve
  `compute.vm`. Bare Metal dedicated-server ranges, Hosted Private Cloud (VMware, Nutanix) and Web Cloud
  are ordered per server or per pack and have their own availability. Each can follow in its own PRD if
  users need it.
- **Decided: EU member states only. The UK is out.** The project asks whether a workload can move to an
  EU provider's EU regions. The UK left the EU in 2020, and data stored there is under UK law, even though
  the EU recognises the UK's data protection as adequate. One rule ("in an EU member state") keeps
  "EU region" the same for every target. Revisit together with PRD #2's open question on region
  semantics if users ask for EEA or adequacy-based residency.
- **Decided: no Local Zones.** Local Zones offer a reduced product set. Capabilities in the catalog are
  per offering, not per region, so listing a Local Zone beside a full region would overstate what is
  available there.
- **Decided: one region name per location.** OVHcloud names regions differently per product: numbered
  per datacentre for compute (for example `GRA11`), per location for databases, and `EU-WEST-PAR` style
  for the 3-AZ regions. The snapshot uses one code per location, the codes on the regions page (`GRA`,
  `RBX`, `SBG`, `PAR`, `LIM`, `MIL`, `WAW` on 2026-11-10). `SOURCES.md` records which product-level names
  each code covers. Regions are compared as strings, and the report says where an offering exists, not
  which endpoint to call.
- **Decided: a capability limited to some regions makes a separate offering.** Paris and Milan are
  3-AZ regions, and some products (for example Object Storage and Managed Kubernetes) are highly available
  across zones only there. A catalog offering has one set of capabilities for all its regions. So the
  snapshot lists a 3-AZ variant, with `high-availability`, in the 3-AZ regions only, beside the
  single-zone offering in every region. This needs no domain change. Mapping already reports the
  best-fitting candidate, so a resource that needs high availability is pointed at the 3-AZ variant.
  Open: whether Scaleway's snapshot (Milan multi-AZ Object Storage) should follow the same rule.
- **Decided: Managed Kafka is not `messaging.queue`.** Kafka is a partitioned log. Consumers track
  offsets, and there is no per-message acknowledgement and no dead-letter queue. Mapping it to
  `messaging.queue` would report `direct` for every queue that uses no listed capability, which overstates
  portability. A stream category is a candidate for a later taxonomy PRD.
- **Decided: no `serverless.function` offering.** OVHcloud's AI Endpoints is serverless model inference,
  not a service that runs user code. Milestone 2 re-checks this against OVHcloud's product pages.
- **Decided: one shared snapshot format and reader.** Two copies of a strict parser drift apart, and its
  rules (unknown fields, undeclared regions, size cap, one-line errors) protect the correctness and safety
  of every target. UpCloud will be the third user. Recorded in
  [ADR 0007](../../docs/adr/0007-shared-target-catalog-snapshot-format-v1.md) (milestone 1).
- **Decided: the taxonomy is unchanged.** A capability OVHcloud offers that has no vocabulary entry is
  left out and noted in `SOURCES.md`. Extending the vocabulary is a change of its own, with tests across
  every target.
- **Decided: snapshot sources and licences.** Unlike `www.scaleway.com`, `www.ovhcloud.com` and
  `docs.ovhcloud.com` are reachable from the development VM. The snapshot cites those pages and may use
  their source repository at a pinned commit. `SOURCES.md` takes only facts and brief attributed quotes
  and states the licence of every source, which PRD #2's Scaleway audit asked for. No authenticated
  OVHcloud API is called (CLAUDE.md rule 3).
  *Corrected in milestone 2:* this decision first named [`ovh/docs`](https://github.com/ovh/docs)
  (CC BY-NC-SA 4.0) as the source repository. It is archived, and its README points to
  [`ovh/ovhcloud-docs`](https://github.com/ovh/ovhcloud-docs), which states no licence (all rights
  reserved). So the snapshot takes facts and brief attributed quotes only from `docs.ovhcloud.com` and
  `ovh/ovhcloud-docs` at the commit pinned in `SOURCES.md`, and nothing from `ovh/docs`.
- **Accepted risk: the snapshot goes stale.** As for Scaleway, the report shows the retrieval date, and
  the adapter's `doc.go` documents how to refresh the snapshot.
- **Open: several targets in one run** (`--target ovhcloud,scaleway`, or a comparison report). Left
  until UpCloud exists, so that the report shape is designed for three targets.

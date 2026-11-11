# PRD #14: Capability granularity: node versus zone high availability, tier-dependent capabilities

**Status:** Draft. On 2026-11-11 the user accepted the recommended options, A + E plus F (see
*Decisions and open questions*).
**Issue:** #14

## Problem

The capability vocabulary is too coarse, so some fits come out better than they really are. The project
treats overstating portability as the worst error. Both catalog snapshots already record the gaps, as
**Scope:** and **Needs:** notes in their `SOURCES.md`, but reports never show those notes. Today's
reports have four kinds of gap:

1. **Scope: node-level or zone-level high availability.** The taxonomy defines `high-availability` as
   "surviving the loss of a node or zone without manual failover". Scaleway Managed Database for
   PostgreSQL and MySQL survives the loss of a node only: the standby is in the same data centre. The
   same is true of OVHcloud Managed MySQL and Managed PostgreSQL in the 1-AZ regions. Today:
   - An RDS Multi-AZ PostgreSQL instance, which survives the loss of a zone, maps `direct` to Scaleway's
     Managed Database.
   - On OVHcloud, the 1-AZ and 3-AZ database variants both list `high-availability`. They tie, and the
     tie-break by name picks the node-level one: the e2e golden shows "`Managed PostgreSQL` (tie:
     `Managed PostgreSQL 3-AZ`)". PRD #11's rule was that a resource needing high availability is
     pointed at the 3-AZ variant. That rule holds for Object Storage, Block Storage and Managed
     Kubernetes, but not for the databases.
   - Two OVHcloud offerings wait for this issue: Object Storage and Managed Kubernetes Service in the
     1-AZ regions. Their sections say they are not counted "until issue #14 settles node versus zone
     granularity".
2. **Tier: a capability needs a tier, plan, class or option.** Scaleway Kapsule is highly available only
   with a Dedicated control plane, and its free Mutualized tier is not. Scaleway Managed Database needs
   the High Availability option, and Scaleway Object Storage needs the Standard Multi-AZ class. OVHcloud
   databases need a multi-node plan for high availability and read replicas. OVHcloud Block Storage 3-AZ
   needs the Regional Classic class, and OVHcloud Object Storage 3-AZ is not highly available in Cold
   Archive. The report says "`Kubernetes Kapsule` offers every capability the resource uses" and never
   names the tier to buy. This is the required change the `direct` verdict hides.
   There is a worse problem. An offering's capabilities are the union of all its tiers' capabilities, so
   a snapshot can list two capabilities that no single tier has together, and the mapping reports
   `direct`. OVHcloud Block Storage had to be split by hand for exactly this reason: the 3-AZ class
   cannot be encrypted. Nothing makes the next snapshot do the same.
3. **Direction: a capability is met in one direction only.** The taxonomy's `ipv6` ("IPv6 connectivity")
   has no direction. Scaleway Serverless Functions supports IPv6 for incoming traffic only, at its public
   endpoint. A Lambda function in a dual-stack VPC subnet that calls IPv6 destinations would still map
   `direct`.
4. **Compatibility: an offering serves a category only approximately.** Scaleway Serverless SQL Database
   is PostgreSQL-compatible, with documented differences. It has no temporary tables, no subscriptions
   (and so no change-data-capture), and no guaranteed `LISTEN`/`NOTIFY` or advisory locks. A
   `database.postgresql` resource whose only capability is `autoscaling`, such as an Aurora Serverless v2
   cluster, maps `direct` to it. The reason is that Managed Database lacks `autoscaling`.

**Why now.** The first live source adapter has to decide which capabilities to emit. Once
`eusov discover` writes inventory files, every change to the vocabulary breaks files that users already
have. Today the only inventories are hand-written, and only two snapshots exist. UpCloud would be a
third snapshot to migrate. PRD #2 left this as an open question ("capability granularity") until the
first live source adapter. That adapter is next.

## Outcome

A capability means one thing. A source can tell whether a resource uses it, and a reviewer can check a
target's claim to it against the documentation. A `direct` verdict never rests on a capability that the
offering meets only in part, or only in a tier the report does not name. Under the accepted options
(below), the report for an RDS Multi-AZ PostgreSQL instance looks like this:
- On Scaleway: `partial`, with `zone-high-availability` as a required change.
- On OVHcloud: `direct`, pointed at a 3-AZ variant.
- A Kubernetes cluster that needs high availability is pointed at an offering named after the tier
  that provides it.
- Every **Scope:** note in the snapshots has either become a capability or become a capability the
  offering does not list.

Reports keep their sections and columns. Neither file format changes version. On 2026-11-11 the user
accepted the recommended options, and the acceptance criteria and milestones below follow them.

## Scope

| | In scope | Out of scope |
|---|---|---|
| Source providers | None live. The versioned inventory file (ADR 0006). The rules that the AWS, Azure and GCP source adapters must follow when they emit the capabilities this PRD changes. | Live discovery (one PRD per source) |
| Target providers | Scaleway and OVHcloud: their snapshots, `SOURCES.md` files and golden files | UpCloud (its PRD follows the rules recorded here) |
| Taxonomy categories | Unchanged. The PRD changes how existing categories are served (Scaleway Serverless SQL Database and `database.postgresql`). | New categories |
| Capabilities | `high-availability` (scope), `ipv6` (direction), and capabilities that depend on a tier (`high-availability`, `read-replicas`) | Capability versions and parameters, for example "PostgreSQL 16" (PRD #2's open question stays open for versions). Availability across regions (multi-region failover). Customer-managed keys. |
| Assessment and output | Fits, reasons and required changes as they are, with more precise capability and offering names | New fits, new report sections, JSON report, readiness score, pricing |

## Options

The six options below are not exclusive, and the recommendation combines three of them. For each option,
the list gives what it changes in each part of the system and how it avoids overstating portability.
*Format* means the inventory file (ADR 0006) and the snapshot (ADR 0007). Both are strict and at
`schemaVersion` 1, and any new field in either is a new version with a new ADR.

| | Option | Gaps it closes | Format change | Production code it changes |
|---|---|---|---|---|
| A | Split `high-availability` into node level and zone level | Scope | None | `taxonomy`, `catalog.New` |
| B | Capability notes (needs, scope) carried into reasons | Tier, Scope, Direction, Compatibility (scope notes cap the fit at partial) | Snapshot v2 | `catalog`, `mapping`, `assessment`, `markdown`, snapshot reader |
| C | Give `ipv6` a direction | Direction | None | `taxonomy` |
| D | Ordered capability levels | Scope (and future levels) | Inventory v2, snapshot v2 | `taxonomy`, `inventory`, `catalog`, `mapping`, `markdown`, both readers |
| E | One offering per tier, as PRD #11 did for 3-AZ | Tier (with the union problem) | None | None |
| F | Strict definitions: claim a capability only when it is met in full | Scope, Direction, Compatibility (by leaving things out) | None | None |

### A. Split `high-availability` into node level and zone level

Replace `high-availability` with `node-high-availability` (the resource keeps serving through the loss
of one node, server or host, without manual failover) and `zone-high-availability` (it keeps serving
through the loss of a whole availability zone or data centre). Zone level implies node level.

- **Taxonomy and vocabulary:** remove one capability and add two. The taxonomy holds the implication
  as data. `ParseCapability` refuses `high-availability`, and its error names the two replacements.
  Inventory files that use the old value are refused loudly, and none of them is reread silently.
  `schemaVersion` stays 1, because no tool writes inventory files yet (see *Decisions*).
- **Catalogs:** every `high-availability` follows its **Scope:** line. Node level: Scaleway Managed
  Database, and OVHcloud Managed MySQL and PostgreSQL in the 1-AZ regions. Zone level: the OVHcloud 3-AZ
  variants, and Scaleway Object Storage (Standard Multi-AZ). The scope of Scaleway Kapsule is
  re-verified, because its notes speak of etcd and regional control planes spread across zones. The
  OVHcloud offerings that defer to #14 are decided.
- **Mapping:** no change if `catalog.New` adds implied capabilities to every offering. A resource that
  uses node level then matches a zone-level offering. Only offerings are expanded, never resources, so a
  partial fit names one missing capability, not two.
- **Report:** no structural change. Reasons and required changes name the exact capability ("does not
  offer `zone-high-availability`").
- **targettest:** unchanged. `catalog.New` guarantees the implication for every adapter.
- **Golden files:** both catalog goldens (zone-level offerings also show `node-high-availability`), the
  e2e fixtures and goldens, and the Markdown goldens and OVHcloud tests that name `high-availability`.
  OVHcloud's database tie disappears: the 3-AZ variant becomes the only direct fit for zone level.
- **Future source adapters:** they must tell node from zone, and the sources expose it. Examples are
  RDS `MultiAZ`, Cloud SQL `availabilityType` (`REGIONAL` or `ZONAL`), and the high-availability mode
  of Azure Database for PostgreSQL flexible server (`ZoneRedundant` or `SameZone`). When the data cannot
  tell, the adapter emits `zone-high-availability`.
- **Against overstating:** a node-level offering never satisfies a zone-level need. The implication runs
  only downwards. A source that is unsure claims the stronger level.
- **Cost:** each split is a breaking vocabulary change, so it is cheap only before `discover` exists.
  It does nothing for tiers, direction or compatibility.

### B. Capability notes carried into reasons

An offering's capability may carry a short note of one of two kinds. A `needs` note names a tier, plan,
class or option ("Dedicated control plane"). A `scope` note says which part of the definition the
offering meets ("node level only", "incoming traffic only"). An offering may also carry a
`compatibility` note for its category ("PostgreSQL-compatible, with documented differences").

- **Taxonomy and vocabulary:** unchanged. The note kinds belong to the catalog.
- **Catalogs:** snapshot format v2 (a new ADR, replacing ADR 0007's v1), and `catalog.Offering` gains
  notes, validated with `text.Validate` and capped in length. Both snapshots move their **Needs:** and
  **Scope:** lines into notes, in short form.
- **Mapping:** new reason codes for a capability offered with a scope limit and for a capability that
  needs a tier. A `scope` or `compatibility` note on a capability the resource uses caps the candidate at
  `partial`, whatever the resource needs, because the inventory cannot say which part it needs. A
  `needs` note keeps the candidate `direct`. The candidate order has to decide whether a direct
  candidate without notes ranks above one with notes.
- **Report:** reasons gain the note text, and Required changes gains a second kind of entry, "choose
  `<tier>`", for direct verdicts. `assessment.Verdict.RequiredChanges`, a list of capabilities today,
  has to change shape. Note text is catalog text, so it is a new untrusted value: it is rendered as a
  code span, and the hostile goldens must cover it. The planned JSON report has to carry notes too.
- **targettest:** a new check that every note is attached to a capability the offering lists, and is
  non-empty, valid text.
- **Golden files:** catalog goldens, all Markdown goldens including the hostile one, e2e goldens, and
  the snapshot reader's tests for v2.
- **Future source adapters:** nothing new. They keep emitting `high-availability`.
- **Against overstating:** scope and compatibility notes force `partial`, and needs notes appear on
  direct verdicts. Two weaknesses remain. The option understates every node-level need: an Azure
  same-zone high-availability database is `partial` on Scaleway, although the offering meets its need.
  And it still cannot see the union problem: two capabilities from different tiers still give `direct`,
  unless a snapshot splits the offering by hand.

### C. Give `ipv6` a direction

Replace `ipv6` with `ipv6-inbound` (reachable over IPv6) and `ipv6-outbound` (able to reach IPv6
destinations). Or keep `ipv6` with the meaning "both directions", and let it imply each of the two.

- **Taxonomy and vocabulary:** two capabilities, or one plus an implication, as in A.
- **Catalogs:** instances and Elastic Metal list both directions, and Scaleway Serverless Functions
  lists only `ipv6-inbound`. OVHcloud's instance caveats (no IPv6 on Floating IP or Gateway, and
  provisioning only through the API in the 3-AZ regions) stay notes in `SOURCES.md`.
- **Mapping, report, targettest:** no change (an implication works as in A).
- **Golden files:** catalog goldens, the e2e fixture (`i-0123456789abcdef0` uses `ipv6`) and goldens,
  and the Markdown and test fixtures that use `ipv6`.
- **Future source adapters:** they must tell the direction. An instance with a dual-stack interface
  uses both. A function in a dual-stack subnet that only calls out uses outbound. A public endpoint
  that is dual stack is inbound, but it is often a separate resource (an API gateway or a function
  URL), and whether a workload depends on it is hard to discover.
- **Against overstating:** an inbound-only offering never satisfies an outbound need.
- **Cost:** it adds vocabulary for a single offering today, and a direction that sources can rarely
  detect.

### D. Ordered capability levels

This is the general form of A. A capability may have ordered levels, for example `high-availability`
with `node` < `zone` (and a later `region`). It is written with its level, such as
`high-availability:zone`, and compared by order. This is PRD #2's other answer to "capability
parameters".

- **Taxonomy and vocabulary:** levels per capability, and parsing of the level.
- **Catalogs:** inventory format v2 and snapshot format v2 (two ADRs), because a value changes its
  grammar. Both snapshots are migrated as in A.
- **Mapping:** compares levels instead of testing equality. A reason carries the missing level.
- **Report:** renders a capability with its level, which is a new value shape for the code-span rules.
- **targettest:** unchanged, or a check that each level exists.
- **Golden files:** everything A changes, plus the tests and fixtures of both readers.
- **Future source adapters:** as A, written as levels.
- **Against overstating:** as A.
- **Cost:** two format versions and parser work. It pays off only when a second capability needs ordered
  levels. Engine versions are sets, not orders, so they would not use it.

### E. One offering per tier

This generalises PRD #11's decision that "a capability limited to some regions makes a separate
offering" to tiers. All of an offering's capabilities hold in one tier: the product's default tier for
the base offering, or the tier a variant is named after. A tier that adds a capability becomes a
variant named "<base offering> <tier>", listed beside the base offering, for example
`Kubernetes Kapsule Dedicated` beside `Kubernetes Kapsule`. A tier that only lacks capabilities gets
no offering, as OVHcloud's unencrypted zonal volumes get none today.

- **Taxonomy and vocabulary:** unchanged.
- **Catalogs:** new variants, each with its own `SOURCES.md` section. These include Kapsule with the
  Dedicated control plane, Managed Database with the High Availability option, and OVHcloud MySQL and
  PostgreSQL on a multi-node plan, in both region groups. Whether Scaleway Object Storage needs a
  variant depends on whether Standard Multi-AZ is the default class. The **Needs:** lines become the
  descriptions of these variants. Both `SOURCES.md` files state the rule.
- **Mapping, report, targettest:** no change. The tier appears in the best offering's name. A variant
  name starts with the base name, so for a resource that needs none of the tier's capabilities the base
  offering wins the tie by name and the variant is listed as tied. Ties become more common.
- **Golden files:** catalog goldens (new offerings) and e2e goldens (new names and ties).
- **Future source adapters:** nothing. Sources state what a resource needs, never which tier.
- **Against overstating:** a `direct` fit means one tier has every capability the resource uses, and
  the report names that tier. This also closes the union problem, which B leaves open. It prepares for
  plan matching and pricing, which need tiers as offerings anyway.
- **Cost:** more offerings, more ties, and offering names that the project invents (as "3-AZ" already
  is).

### F. Strict definitions

Each capability means its strongest reading, and an offering that meets only part of a definition does
not list the capability. Sources claim a capability on the weakest evidence: a resource that uses it in
any form claims it. Only the definitions and the snapshots change, and no code.

- **Taxonomy and vocabulary:** the doc comments change. `high-availability` means zone level, and
  `ipv6` means both directions.
- **Catalogs:** Scaleway Managed Database and OVHcloud's 1-AZ databases lose `high-availability`, and
  Scaleway Serverless Functions loses `ipv6`. Scaleway Serverless SQL Database leaves the snapshot,
  because it does not fully serve `database.postgresql`. Each of these moves to *Left out on purpose*.
- **Mapping, report, targettest:** no change.
- **Golden files:** catalog goldens and e2e goldens.
- **Future source adapters:** they emit the capability on any evidence. A same-zone high-availability
  database claims `high-availability`, and a function that only calls out over IPv6 claims `ipv6`.
- **Against overstating:** by construction. The cost is understating: every node-level need is `partial`
  on node-level offerings, and offerings that would fit leave the candidate list.

### Recommendation (accepted on 2026-11-11)

**A for scope, E for tiers, and F for direction and compatibility.** B, C and D are deferred until a
source adapter or a new target shows a gap that these three cannot express. The user accepted this
recommendation on 2026-11-11.

- **A** closes the gap with the largest effect: high availability of managed databases, Kubernetes and
  storage, the services a migration is planned around. It is precise on both sides, the sources already
  expose the distinction, and it makes PRD #11's 3-AZ database variants the answer they were meant to be.
  It changes only the taxonomy and `catalog.New`. Mapping, assessment, the reporters and both formats
  stay as they are.
- **E** follows a decision this project already made, needs no code, and is the only option that closes
  the union problem. Its cost, more offerings and ties, is visible and safe.
- **F for the two one-off cases.** Direction and compatibility each affect one offering today. A split of
  `ipv6` adds vocabulary that no source can yet detect. A compatibility note needs all of B's machinery.
  Leaving out `ipv6` on Serverless Functions and the whole Serverless SQL Database understates two cases,
  which the project prefers to overstating them.
- F's asymmetric rule becomes the general principle that the ADR records: sources claim on the weakest
  evidence, and targets claim only what the documentation states in full.
- **B is the fallback** if the user wants product names kept as they are (no variants), or wants
  Serverless SQL Database kept as a `partial` candidate. **D replaces A** if a second ordered capability
  appears before `discover` ships.

## Acceptance criteria

These criteria follow the accepted options (A + E, plus F). All are pinned by tests that run offline
under `make test` or `make test-e2e`, unless marked *review*.

**Vocabulary**

- [ ] Given a resource that uses `node-high-availability`, and an offering in its category that lists
  `zone-high-availability` and every other capability the resource uses, then the fit is `direct`.
- [ ] Given a resource that uses `zone-high-availability`, and only offerings with
  `node-high-availability`, then the fit is `partial`, and Required changes lists
  `zone-high-availability` (not offered by the best offering).
- [ ] The parsed catalog of every target lists `node-high-availability` on every offering that lists
  `zone-high-availability`, whether or not its snapshot does (catalog goldens).
- [ ] Given an inventory file that lists `high-availability`, `eusov assess` exits 1 with one error line.
  The line names the resource, the value, and both `node-high-availability` and
  `zone-high-availability`. The value is never read as either of them.
- [ ] *Review:* the doc comments of `node-high-availability`, `zone-high-availability` and `ipv6` state
  the definitions recorded in the ADR. `ipv6` means both directions.

**Catalogs**

- [ ] Given a `database.postgresql` resource that uses `zone-high-availability` (an RDS Multi-AZ instance):
  - with `--target scaleway`, the fit is `partial`, and Required changes names `zone-high-availability`;
  - with `--target ovhcloud`, the fit is `direct`, the best offering is a 3-AZ variant, and no 1-AZ
    offering ties with it.
- [ ] Given the same resource using `node-high-availability`, both targets report `direct`. The best
  Scaleway offering is the Managed Database variant named after the High Availability option.
- [ ] Given a resource that uses a capability only one tier of a product provides (for example a
  `kubernetes.managed` resource with high availability on Scaleway), then the best offering is the
  variant for that tier, and its name says the tier. Given a resource of the same category that uses
  none of the tier's capabilities, then the base offering is reported first.
- [ ] Given a `serverless.function` resource that uses `ipv6`, then with `--target scaleway` its fit is
  `partial`.
- [ ] Given a `database.postgresql` resource whose only capability is `autoscaling`, then with
  `--target scaleway` no candidate is named Serverless SQL Database. (Today it is the best offering,
  with fit `direct`.)
- [ ] *Review:*
  - every high-availability capability in both snapshots matches its section's **Scope:** line, checked
    against the provider's documentation;
  - every **Needs:** line names the tier its offering is named after, or the product's default tier;
  - no `SOURCES.md` still defers a decision to issue #14;
  - Serverless Functions' `ipv6` and Serverless SQL Database are listed under *Left out on purpose*,
    with the reason.
- [ ] Both adapters pass `targettest` as it stands. A test pins each adapter's declared regions, and
  those tests are unchanged.

**Report and architecture**

- [ ] The report's sections, columns and resource order are unchanged. In the goldens, only offering
  names, fits, ties and capability names differ.
- [ ] *Review:*
  - no production code changes under `internal/domain/mapping`, `internal/domain/assessment`,
    `internal/app`, `internal/adapter/report`, `internal/adapter/source/inventoryfile` or
    `internal/adapter/target/snapshot`;
  - their tests and goldens change only where they name a retired capability;
  - neither format's `schemaVersion` changes.
- [ ] `make check` passes, including depguard.

## Milestones

Each milestone ships as one PR. Milestones 2 and 3 depend only on 1, and can run in parallel.

1. [x] **ADR and the scoped high-availability vocabulary.** The ADR
   ([0011](../docs/adr/0011-scope-capabilities-and-one-offering-per-tier.md)) records:
   - the chosen options and the definitions of each capability they change;
   - the implication, zone level implies node level, which lives in the taxonomy and which `catalog.New`
     applies to offerings only;
   - the source rule: claim a capability on the weakest evidence, and claim the stronger level when
     unsure;
   - the target rule: list a capability only when the documentation states it in full;
   - the tier-variant rule;
   - the policy for retiring a vocabulary value (no format version change while no tool writes
     inventory files).

   The taxonomy adds `node-high-availability` and `zone-high-availability`. `high-availability` stays
   until milestone 4. Unit tests in `taxonomy`, `catalog` and `mapping`, against test catalogs, pin the
   first three *Vocabulary* criteria. No snapshot changes, so no golden changes.
2. [x] **Scaleway snapshot.** In this milestone:
   - re-verify against the live pages, as issue #15 did, the scope of every high-availability claim,
     Kapsule's included, and replace each claim with its scoped capability;
   - add tier variants: Kapsule with the Dedicated control plane, Managed Database with the High
     Availability option, and Object Storage if Standard Multi-AZ is not the default class;
   - remove `ipv6` from Serverless Functions, and Serverless SQL Database from the snapshot;
   - update *Rules applied* in `SOURCES.md`;
   - resolve PRD #11's open question on Milan multi-AZ Object Storage;
   - update the Scaleway catalog golden and its e2e golden.

   The reviewer checks each claim against Scaleway's documentation. Depends on 1.
3. [x] **OVHcloud snapshot.** In this milestone:
   - set the 1-AZ databases to node level, and the 3-AZ variants (databases, Managed Kubernetes, Object
     Storage, Block Storage) to zone level;
   - add multi-node plan variants for MySQL and PostgreSQL, carrying high availability and read replicas;
   - decide Object Storage and Managed Kubernetes Service in the 1-AZ regions under the new definitions,
     and remove the references to #14;
   - extend `TestThreeAZVariantsServeHighAvailability` to the databases at zone level;
   - rewrite `TestDatabaseVariantsBothServeHighAvailability` to the new rule;
   - update the catalog golden and the e2e golden.

   The reviewer checks each claim against OVHcloud's documentation. Depends on 1.
4. [ ] **Retire `high-availability`, e2e and docs.** In this milestone:
   - remove the value from the vocabulary;
   - make its parse error name the two replacements, through `text.Quote`;
   - give the e2e fixture a database with `zone-high-availability`, one with `node-high-availability`, a
     Kubernetes cluster that needs a tier, a function with `ipv6`, and a PostgreSQL resource with only
     `autoscaling`;
   - regenerate the goldens of both targets and the hostile golden;
   - move the Markdown reporter's test fixtures, `internal/testutil` and the remaining tests off the
     retired value;
   - update `docs/architecture.md` if it lists capabilities;
   - move this PRD to `prds/done/`, and close #14.

   Depends on 2 and 3.

## Decisions and open questions

- **Decided (2026-11-11): A for scope, E for tiers, and F for direction and compatibility.** The user
  accepted the recommendation (*Options*, *Recommendation*). B is the fallback if a later need cannot
  be expressed with these three. C and D are deferred until a source adapter shows a gap that needs
  them. Milestone 1's ADR,
  [0011](../docs/adr/0011-scope-capabilities-and-one-offering-per-tier.md), records the decision.
- **Decided with A: the names `node-high-availability` and `zone-high-availability`.** These are the
  names option A was accepted with. The alternatives were `high-availability-node` and
  `high-availability-zone`, which group together in sorted lists, and `multi-zone-high-availability`,
  the most explicit in a report. Names are hard to change once `discover` writes them, so a change has
  to come before milestone 1 merges.
- **Decided with A: `high-availability` is retired, not narrowed.** Inventory files that use it are
  refused, with an error that names the replacements. The rejected variant, A2, would have kept
  `high-availability` with the meaning "zone level" and added `node-high-availability`. Old files would
  have kept parsing, and their verdicts could only have become more conservative. But the bare name
  would keep inviting the wrong reading, and a report would say that Scaleway's Managed Database "does
  not offer `high-availability`", although the product calls the feature exactly that.
- **Decided with F: Scaleway Serverless SQL Database leaves the snapshot**, until a report can say
  "compatible, with differences". The rejected alternative was a compatibility note that caps its fit
  at `partial`. That is B's machinery, built for one offering.
- **Decided with A: where the implication lives.** It is data in the taxonomy, and `catalog.New`
  applies it, so `Offering.Has` answers correctly everywhere: in mapping, in tests and in a future
  comparison report. A snapshot may list only `zone-high-availability`. Resources are never expanded.
- **Decided with A: retiring a vocabulary value is not a format change, for now.**
  The format refers to the vocabulary, and does not fix it. A retired value is refused by name, and is
  never reread, so a bump to `schemaVersion` 2 would add no safety. It would also refuse every existing
  file, including files that do not use the value. Once `eusov discover` writes inventory files, a
  retirement needs a new format version. The ADR amends ADR 0006's version policy to say so. Adding a
  capability stays allowed at version 1.
- **Decided with F: the source rule.** A source adapter claims a capability whenever
  the resource may depend on it. Where a capability has levels, the adapter claims the strongest level
  the data does not rule out. A target claims a capability only where its documentation states it in
  full. Uncertainty on either side therefore makes a fit look worse, never better.
- **Decided (milestone 2): Scaleway Object Storage lists no high-availability level.** Its FAQ says
  that after a data centre failure Scaleway is "still able to rebuild your data and make it
  accessible". That is recovery, not continuity without manual failover, so the target rule allows
  neither level. As a consequence, under the source rule an S3 Standard bucket may claim
  `zone-high-availability`, and it is `partial` on Scaleway. OVHcloud Object Storage is held to the
  same bar in milestone 3: its 3-AZ documentation states read and write availability through a zone
  failure.
- **Note: what is not in this PRD.** Capability versions such as engine versions remain PRD #2's open
  question. Multi-region availability is not covered: an implication chain node < zone < region could
  later add `region-high-availability` without changing what this PRD decides. Two caveats stay as notes
  in `SOURCES.md`, because they do not break the capabilities' definitions: OVHcloud's cold snapshots on
  Metal Instances, and the limits on IPv6 for Floating IP and Gateway.
- **Accepted with F: understating two cases.** A resource that needs only inbound IPv6
  is `partial` on Scaleway Serverless Functions. A PostgreSQL workload that would run on Serverless SQL
  Database is pointed at Managed Database. C or B can recover each case when a source shows that it
  matters.
- **Accepted with E: more ties.** Resources that use none of a tier's capabilities tie
  between the base offering and its variants. An example is "`Kubernetes Kapsule` (tie:
  `Kubernetes Kapsule Dedicated`)". The base offering wins by name. Revisit this together with PRD #2's
  open question on a product-preference order. A second kind of tie: a resource that needs
  `zone-high-availability`, on a target whose tiers are all node level. Both tiers miss one
  capability and tie, and the tie-break by name makes the **lower** tier the best offering
  (Standalone on Managed Database, Mutualized on Kapsule). Required changes lists only
  `zone-high-availability`, so the report does not say that the named offering also lacks node level.
  Revisit this with the product-preference order too. A ranking rule could prefer a candidate that
  holds the implied lower level.
- **Decided (milestone 3): the tier comes before the region-group suffix.** A tier variant is named
  `<base offering> <tier>`, and a suffix such as `3-AZ` follows the tier: `Managed PostgreSQL Multi-node
  3-AZ`, not `Managed PostgreSQL 3-AZ Multi-node`, and `Managed PostgreSQL Multi-node Advanced 3-AZ` for
  the Advanced plan. The 1-AZ variant's name is then a prefix and wins ties by name, so a resource that
  needs only `node-high-availability` is pointed at `Managed PostgreSQL Multi-node`, and one that needs
  `read-replicas` at `Managed PostgreSQL Multi-node Advanced`, the variants offered in the 1-AZ regions,
  not at an offering in Milan and Paris only. A `zone-high-availability` need still has only 3-AZ
  variants as direct candidates. This departs from PRD #11's `<product> 3-AZ` pattern only where a tier
  variant also has a 3-AZ form. *Block Storage 3-AZ Regional Classic* keeps `<product> 3-AZ <tier>`,
  because that volume class exists only in the 3-AZ regions and has no 1-AZ form to win ties against.
- **Note: releases between milestones.** From milestone 1 to milestone 4, the vocabulary accepts
  `high-availability` beside its replacements. A release in that window would ship that mixed state, so
  tag none.

# PRD #190: Sizing, cost estimates and workloads

**Status:** Draft
**Issue:** #190 (takes in #186)

## Problem

`shift2eu assess` answers "does this target offer something for each resource?" It doesn't answer the
two questions that come right after it in every migration conversation:

1. **What will it cost?** A platform engineer or a finance owner who sees a `direct` fit still can't
   tell whether moving is cheaper or dearer. The inventory has no size (vCPU, memory, storage, node
   count), and the target catalogs have no plans or prices. So the tool can't pick a plan, let alone
   price one. Every PRD so far (#2, #11, #14, #89) has left sizing, plan matching and cost out of scope.
2. **What does this mean for my application?** An inventory is a flat list. It can't say that a
   database is a read replica of another, that a queue is another queue's dead-letter queue, or that a
   group of resources is one workload, for example "shop". The assessment therefore reports per
   resource only, never per workload. Yet "can the shop move, and what does it cost there?" is the
   question people bring (issue #186, raised while building the sample inventories in #179).

Both need new inventory fields, so both need a new inventory format version (ADR 0006: any new field is
a new version). Doing them together changes the format once instead of twice, before
`shift2eu discover` exists and starts writing files in the wild.

## Outcome

A user adds sizes, an optional usage profile, workloads and relations to an inventory (schema version
2), and `shift2eu assess` reports, per target:

- for each resource, the **plan** it would run on (the smallest plan that covers its size) and an
  **estimated monthly price** from the target's dated, embedded price list;
- per **workload** and in total, the fits, the blockers and the estimated monthly price. Where the
  inventory carries the source's own monthly cost, it is shown next to the estimate.

The comparison report (`--target all`) shows the estimates side by side. It still ranks nothing.

Every estimate is labelled as an estimate: list prices on the snapshot's retrieval date, excluding VAT,
with no discounts. A resource that can't be priced (no size, no matching plan, or a usage-priced
category) says so and why. It never counts as zero. A version 1 inventory still works and produces
today's report plus "size unknown" notes. The tool stays offline and needs no credentials.

## Scope

| | In scope | Out of scope |
|---|---|---|
| Source providers | None live. The inventory file, schema versions 1 and 2. Docs (#178's guide) on how to read sizes from AWS, GCP and Azure | Live discovery (`shift2eu discover`). Reading the source's billing data automatically |
| Target providers | OVHcloud, Scaleway and UpCloud: plans and list prices added to their embedded snapshots | Live pricing APIs. New targets. Committed-use, reserved or negotiated prices |
| Taxonomy categories | Priced: `compute.vm`, `compute.baremetal` (servers by monthly plan; ADR 0023), `kubernetes.managed` (control plane and nodes), `storage.block`, `storage.object` (stored GiB), `database.postgresql`, `database.mysql` | Priced by usage, so shown as "not estimated" with the reason: `serverless.function`, `messaging.queue`. `unclassified` is never priced. Network egress, IP addresses, load balancers, support plans |
| Inventory | Schema version 2: per resource `size`, `usage`, `workload`, `relations` and the source's `monthlyCost`. Version 1 still read | Account or tenant fields. Tags |
| Assessment | Plan matching, a monthly estimate per resource, workload grouping, relations in the report | A readiness score, ranking or recommended target. Changing a resource's fit because of its size (see *Decisions*) |
| Output | Markdown and JSON reports (JSON report schema version 2), single target and comparison. The HTTP API takes version 2 inventories and returns the new report | HTML. Currency conversion. Charts |

## Acceptance criteria

All criteria are pinned by tests that run offline under `make test` or `make test-e2e`, unless marked
*review*.

**Inventory format version 2**

- [ ] An inventory with `"schemaVersion": 2` may give each resource:
  - `size`: `vcpus`, `memoryGiB`, `storageGiB` and `count` (instances or nodes), each optional;
  - `usage`: `hoursPerMonth` (default 730);
  - `workload`: a free-text name;
  - `relations`: a list of `{"type", "target"}`, where `type` comes from a closed vocabulary: `replica-of`,
    `dead-letter-queue-of`, `attached-to` and `depends-on`, and `target` is another resource's `id`;
  - `monthlyCost`: `{"amount", "currency"}`.

  It's read with the same strict rules and caps as version 1.
- [ ] A version 1 inventory gives the same verdicts as today. Its resources get no plan and no estimate,
  and the report says that the sizes are unknown.
- [ ] Given a relation whose `target` names no resource, a resource that relates to itself, an unknown
  relation `type`, a negative or non-numeric size, or a `size` field that makes no sense for the
  category (for example `vcpus` on `storage.object`), the run exits 1 with one error line that names
  the resource and the field by path (`resources[3].relations[0].target`). Nothing is written.
- [ ] *Review:* an ADR records format version 2 and how it relates to ADR 0006 (supersedes or amends).

**Catalog plans and prices**

- [ ] Each target's snapshot lists, per offering, the plans that the priced categories need:
  - name, vCPU, memory and included storage;
  - a monthly list price in EUR excluding VAT, and the regions it applies to where prices differ;
  - per-GiB monthly prices for block storage, object storage and database storage.

  Each snapshot also carries the date the prices were retrieved and their source URL. Its `SOURCES.md`
  cites every price.
- [ ] *Review:* an ADR records the snapshot format version that adds plans and prices (amends ADR 0007),
  including how a price is stated (monthly, or hourly × 730) and the rounding.
- [ ] The snapshot reader refuses, with the existing strict errors, a plan without a price, a negative
  price, a currency other than EUR, or two plans of one offering with the same name.

**Plan matching and estimates**

- [ ] Given a sized resource with a fit other than `none`, the report names the plan on the best
  offering: the cheapest plan whose vCPU and memory are each at least the resource's. Ties are broken by
  fewer vCPU, then less memory, then name.
- [ ] Given a size that no plan covers, the resource shows "no plan large enough" with the largest plan's
  size, and gets no estimate. Its fit is unchanged.
- [ ] The monthly estimate of a resource is plan price × `count` × `hoursPerMonth` / 730, plus storage
  above what the plan includes × the per-GiB price. It's rounded to the cent only for display. A test
  table pins one example per priced category on each target.
- [ ] A resource with no size, no fit, a usage-priced category or no price for its region shows "not
  estimated" with that reason. Totals sum only the estimated resources and say how many weren't
  estimated ("€1,234.56 per month for 9 of 13 resources").
- [ ] The report states, next to every total, that it is an estimate: list prices on the snapshot's
  retrieval date, excluding VAT and discounts.

**Workloads and relations**

- [ ] Resources with the same `workload` are reported together. Each workload gets its fit counts, its
  blockers and its estimate. Resources without a workload are grouped under "No workload". A version 1
  inventory has only that group, and the per-workload section is left out.
- [ ] Each resource's row lists its relations. A blocker on a resource is also listed under every
  resource that `depends-on` it, or that is a `replica-of` it, as "affected by a blocker on `<id>`". No
  fit changes.
- [ ] Given a `monthlyCost` on resources, each workload and the total show the source's cost next to the
  estimate, per currency, with no conversion. Where some resources have no `monthlyCost`, the total says
  how many it covers.

**Reports, comparison and API**

- [ ] The Markdown report adds a plan and an estimate column to the resources table, a "Workloads"
  section, and a cost summary. Golden files pin a single-target and a comparison report for each sample
  inventory under `examples/` (updated to version 2 with sizes, workloads and relations).
- [ ] The comparison report adds one estimate column per target in its summary and resources tables,
  and the per-workload estimates per target. It contains no ranking, "cheapest" or recommendation: the
  "no ranking" test from PRD #89 gains "cheapest" and "best price".
- [ ] The JSON report moves to schema version 2 (ADR 0018) with the plan, the estimate, its basis
  (price date, currency, excluded items), the workloads and the relations. Its JSON Schemas are updated
  and the examples validate against them.
- [ ] `POST /v1/assessments` accepts version 1 and version 2 inventories and returns the version 2
  report. *Review:* whether that's still `/v1` or needs `/v2` is decided in the JSON report milestone,
  under ADR 0018 and ADR 0019's versioning rules.
- [ ] Given the same inventory twice, the reports are byte-identical. Nothing depends on the current
  date, so a snapshot's age is shown as its retrieval date, never "N days old".

**Architecture**

- [ ] `make check` passes, including depguard and `test/architecture`. Plan matching and estimating are
  domain logic (`internal/domain`). Prices enter only through the catalog port. No source-service to
  target-service price table exists anywhere.
- [ ] Default tests use invented plans and prices (`internal/testutil`). Real prices appear only in the
  embedded snapshots and the goldens built from them.

## Milestones

Each milestone ships as one PR, in dependency order. Milestones 3, 4 and 5 can run in parallel.

1. [ ] **Inventory format version 2.** Domain: size, usage, workload, relations (closed vocabulary) and
   source cost on `inventory.Resource`, validated by `inventory.New`. The inventory file reader accepts
   versions 1 and 2. ADR for the format. The sourcetest contract suite covers the new fields. No report
   change.
2. [ ] **Catalog plans and prices.** Domain: plans and prices on `catalog.Offering`, validated by
   `catalog.New`. The snapshot format version that adds them, with the shared reader, the targettest
   contract suite and an ADR. The existing snapshots move to the new version with no plans yet.
3. [ ] **OVHcloud plans and prices** in its snapshot and `SOURCES.md`. Depends on 2.
4. [ ] **Scaleway plans and prices** in its snapshot and `SOURCES.md`. Depends on 2.
5. [ ] **UpCloud plans and prices** in its snapshot and `SOURCES.md`. Depends on 2.
6. [ ] **Plan matching and estimates.** Domain: the plan per verdict, the monthly estimate, the
   "not estimated" reasons and the totals, per assessment and in the comparison. Depends on 1 and 2.
7. [ ] **Workloads and relations in the assessment.** Domain: per-workload summaries and estimates, and
   the "affected by a blocker" notes. Depends on 6.
8. [ ] **Markdown and JSON reports.** Both renderers, the comparison, JSON report schema version 2 and
   its Schemas, the HTTP API's request and response, and an ADR if the API path changes. Depends on 7
   and on 3, 4 and 5 for the real-price goldens.
9. [ ] **Examples, docs and e2e.** `examples/` move to version 2 with sizes, workloads, relations and
   some `monthlyCost`, and `docs/guide/try-it.md`, `assess.md`, `http-api.md` and the inventory guide
   (#178) cover the new fields and how to read sizes from each source's CLI. E2e goldens. README and
   `CLAUDE.md` status. Moves this PRD to `prds/done/`, closes #190 and #186. Depends on 8.

## Decisions and open questions

- **Proposed: one PRD and one inventory format version for sizing, cost, workloads and relations.** Each
  of them needs new resource fields, and each format version is a reader, an ADR and a migration of the
  samples. Doing them together costs one version instead of two. #186 is taken in here.
- **Proposed: sizing doesn't change the fit.** Fit stays "does the target offer this category with these
  capabilities". "No plan large enough" is reported next to the fit. Folding it into the fit would change
  verdicts for every existing user, and a too-large VM is often split or re-architected rather than
  blocked. *Open:* revisit once users have seen both.
- **Proposed: list prices only, EUR, excluding VAT, monthly.** All three targets publish EUR list
  prices. VAT depends on the buyer, and discounts on the contract. The report says all of this next to
  every total, so an estimate is never mistaken for a quote.
- **Proposed: never zero for unknown.** A resource that can't be priced is counted and named, never
  added as €0, so a total can't look cheaper than it is. That's the same principle as ADR 0006's "a
  silently ignored mistake is worse than a refused file".
- **Proposed: usage-priced categories are not estimated in this PRD.** Functions and queues are priced
  per request or per message. An honest estimate needs usage fields that sources expose unevenly.
  *Open:* a follow-up PRD with `usage.requestsPerMonth` and similar fields.
- **Proposed: the cheapest covering plan.** It's the usual right-sizing rule, and it's deterministic. It
  can pick a plan with more memory than a slightly dearer one with an exact fit. The tie-break order is
  fixed in the acceptance criteria. *Open:* whether to honour a CPU family (general, compute- or
  memory-optimised). This needs a vocabulary and stays out unless reviews ask for it.
- **Proposed: relations add notes, not verdicts.** "Affected by a blocker on `<id>`" makes a hidden
  dependency visible without inventing a rule for when a dependency blocks a move. *Open:* a workload
  readiness verdict, once there's agreement on what it means (PRD #2's readiness score).
- **Proposed: the source's own cost comes from the inventory, never from the tool.** The tool doesn't
  read billing APIs or know hyperscaler prices. A user who wants a comparison exports their bill and
  sets `monthlyCost`. That keeps the tool offline and never wrong about the source's discounts.
- **Accepted risk: prices go stale.** Prices are dated and cited, and the report shows the retrieval
  date. Updating them is a snapshot change with its `SOURCES.md`. *Open:* a scheduled check that flags
  snapshots older than a set age, which needs a workflow, not the binary (tests may not depend on time).
- **Open: per-region prices.** Where a target prices regions differently, the estimate uses the region
  matched for the resource. Whether a resource's target region is chosen (the nearest EU region, or one
  the user names) or every region's price is listed is decided in milestone 6.
- **Open: the HTTP API path.** A new report schema version may or may not need `/v2/assessments` under
  ADR 0019. Decided in milestone 8, with an ADR if it changes.

# 0011. Scope high availability by level, give each tier its own offering, and define capabilities strictly

- Status: Accepted (by the user on 2026-11-11, PRD #14). Amends the schema version policy of
  [0006](0006-json-inventory-file-format-v1.md) for retired vocabulary values
- Date: 2026-11-11

## Context

PRD #14 (issue #14) found that the capability vocabulary is too coarse, so some fits come out better
than they are. The project treats overstating portability as the worst error. Both catalog snapshots
record the gaps as **Scope:** and **Needs:** notes in their `SOURCES.md`, but reports never show them.
There are four kinds of gap:

- **Scope.** `high-availability` means "surviving the loss of a node or zone without manual failover".
  Scaleway Managed Database, and OVHcloud's databases in the 1-AZ regions, survive the loss of a node
  only. An RDS Multi-AZ instance, which survives the loss of a zone, maps `direct` to them. On OVHcloud
  the 1-AZ and 3-AZ database variants tie, and the tie-break by name picks the 1-AZ one, which breaks
  PRD #11's rule that a resource needing high availability is pointed at the 3-AZ variant.
- **Tier.** A capability often needs a tier, plan, class or option (Kapsule's Dedicated control plane,
  Managed Database's High Availability option, OVHcloud's multi-node database plans). A `direct` verdict
  never names it. Worse, an offering's capabilities are the union of all its tiers', so a snapshot can
  list two capabilities no single tier has together, and the mapping reports `direct`.
- **Direction.** `ipv6` has no direction. Scaleway Serverless Functions supports IPv6 inbound only.
- **Compatibility.** Scaleway Serverless SQL Database is PostgreSQL-compatible with documented
  differences, yet serves `database.postgresql` as if it were PostgreSQL.

Forces:

- The first live source adapter is next. Once `eusov discover` writes inventory files, every change to
  the vocabulary breaks files users already have. Today the only inventories are hand-written, and only
  two snapshots exist. This is the cheapest moment to change the vocabulary.
- Both file formats (ADR 0006, ADR 0007) are strict and at `schemaVersion` 1. A new field is a new
  version and a new ADR.
- The sources expose the node-versus-zone distinction (RDS `MultiAZ`, Cloud SQL `availabilityType`,
  the high-availability mode of Azure Database for PostgreSQL flexible server). They rarely expose
  traffic direction.
- PRD #11 already decided that "a capability limited to some regions makes a separate offering".

## Decision

We combine three of the PRD's options: **A** (split high availability by scope) for scope, **E** (one
offering per tier) for tiers, and **F** (strict definitions) for direction and compatibility.

**Definitions.** `high-availability` is replaced by two capabilities:

| Capability | Definition |
|---|---|
| `node-high-availability` | The resource keeps serving through the loss of one node, server or host, without manual failover. |
| `zone-high-availability` | The resource keeps serving through the loss of a whole availability zone or data centre, without manual failover. |
| `ipv6` | IPv6 connectivity in both directions: the resource is reachable over IPv6 (inbound) and can reach IPv6 destinations (outbound). |

The doc comments of these capabilities in `internal/domain/taxonomy` state these definitions.

**The implication: zone level implies node level.** The implication is data in the taxonomy, not code
in the mapping. `catalog.New` applies it to **offerings only**: an offering whose snapshot lists
`zone-high-availability` also has `node-high-availability`, so `Offering.Has` answers correctly
everywhere (mapping, tests, a future comparison report), and a snapshot may list only the stronger
level. Resources are **never** expanded: a resource that needs zone level and meets a node-level
offering gets a partial fit that names one missing capability, `zone-high-availability`, not two. The
implication runs downwards only, so a node-level offering never satisfies a zone-level need.

**The source rule.** A source adapter claims a capability whenever the resource **may** depend on it.
Where a capability has levels, the adapter claims the strongest level the data does not rule out. When
the data cannot tell node from zone, the adapter claims `zone-high-availability`. A resource that uses
IPv6 in either direction claims `ipv6`.

**The target rule.** A target snapshot lists a capability only where the provider's documentation
states it **in full**. An offering that meets part of a definition does not list the capability, and its
`SOURCES.md` lists the omission under *Left out on purpose*, with the reason. So Scaleway Serverless
Functions does not list `ipv6`, and Scaleway Serverless SQL Database leaves the snapshot because it does
not fully serve `database.postgresql`.

Together the two rules are asymmetric on purpose: uncertainty on either side makes a fit look worse,
never better.

**The tier-variant rule.** This generalises PRD #11's 3-AZ rule from regions to tiers. All of an
offering's capabilities hold in **one** tier: the product's default tier for the base offering, or the
tier a variant is named after.

- A tier that **adds** a capability becomes a variant named `<base offering> <tier>`, listed beside
  the base offering, with its own `SOURCES.md` section. Examples: `Kubernetes Kapsule Dedicated`,
  Managed Database with the High Availability option, OVHcloud MySQL and PostgreSQL on a multi-node plan.
- A tier that only **lacks** capabilities gets no offering, as OVHcloud's unencrypted zonal volumes get
  none today.
- A **Needs:** line in `SOURCES.md` names the tier its offering is named after, or the product's
  default tier. Both `SOURCES.md` files state the rule.

A `direct` fit therefore means one tier has every capability the resource uses, and the report names
that tier in the offering's name. This closes the union problem. Sources state what a resource needs,
never which tier.

**Retiring a vocabulary value.** `high-availability` is **retired, not narrowed**. `ParseCapability`
refuses it with an error that names both replacements, `node-high-availability` and
`zone-high-availability`, through `text.Quote`, and an inventory file or snapshot that uses it is
refused. The value is never reread as either replacement.

While no tool writes inventory files, retiring a value is **not** a format version change:
`schemaVersion` stays 1 for both formats. The formats refer to the vocabulary and do not fix it. A
retired value is refused by name, so a bump to version 2 would add no safety, and it would refuse every
existing file, including those that never used the value. **This amends ADR 0006's schema version
policy:** once `eusov discover` writes inventory files, retiring or narrowing a vocabulary value needs a
new format version and a new ADR. Adding a capability stays allowed at version 1, because a version 1
file that does not use it means the same as before. Snapshots (ADR 0007) are embedded and rewritten in
the same change as the vocabulary, so they follow the same rule without a version bump.

**Rollout.** PRD #14 milestone 1 adds the two capabilities and the implication beside
`high-availability`. Milestones 2 and 3 move the Scaleway and OVHcloud snapshots to the new
capabilities and tier variants. Milestone 4 retires `high-availability`.

## Alternatives considered

- **B. Capability notes carried into reasons** (`needs`, `scope` and `compatibility` notes in the
  snapshot, shown in reasons and Required changes). Deferred, and the fallback if a later need cannot
  be expressed with A, E and F. It needs snapshot format v2 and changes to `catalog`, `mapping`,
  `assessment`, the Markdown reporter and the snapshot reader, and a new untrusted text value in reports.
  It understates every node-level need (a scope note caps the fit at partial whatever the resource
  needs), and it still cannot see the union problem.
- **C. Give `ipv6` a direction** (`ipv6-inbound`, `ipv6-outbound`). Deferred until a source adapter shows
  the gap matters. It adds vocabulary for a single offering today, and a direction that sources can
  rarely detect (a dual-stack public endpoint is often a separate resource).
- **D. Ordered capability levels** (`high-availability:zone`, compared by order). Deferred; it replaces A
  if a second ordered capability appears before `discover` ships. It costs two format versions (the
  value grammar changes) and parser work, and pays off only with a second ordered capability. Engine
  versions are sets, not orders, so they would not use it.
- **A2. Keep `high-availability` with the meaning "zone level" and add `node-high-availability`.** Old
  files would keep parsing, and their verdicts could only become more conservative. Rejected: the bare
  name keeps inviting the wrong reading, and a report would say Scaleway Managed Database "does not offer
  `high-availability`", although the product calls the feature exactly that.
- **Expand resources too, or put the implication in the mapping.** Expanding resources would make a
  partial fit name two missing capabilities. Code in the mapping would leave `Offering.Has` wrong for
  every other caller.
- **Other names** (`high-availability-node` and `high-availability-zone`, which sort together;
  `multi-zone-high-availability`, the most explicit). The PRD accepted A with `node-high-availability`
  and `zone-high-availability`.
- **Bump `schemaVersion` to 2 for the retirement.** Adds no safety, since the retired value is refused
  by name, and refuses every existing file.

## Consequences

- Uncertainty makes fits look worse, never better. Overstating through scope, tier, direction or
  compatibility is closed for the current snapshots.
- Mapping, assessment, the reporters and both file readers keep their code. Only the taxonomy and
  `catalog.New` change, plus the snapshots and their goldens.
- More offerings and more ties. A resource that uses none of a tier's capabilities ties between the base
  offering and its variants, and the base wins by name, for example "`Kubernetes Kapsule` (tie:
  `Kubernetes Kapsule Dedicated`)". Offering names include tier names the project chooses, as "3-AZ"
  already is. Revisit together with PRD #2's open question on a product-preference order.
- Two accepted understatements: a resource needing only inbound IPv6 is `partial` on Scaleway
  Serverless Functions, and a PostgreSQL workload that would run on Scaleway Serverless SQL Database is
  pointed at Managed Database. C or B can recover each case when a source shows it matters.
- Source adapters (AWS, Azure, GCP) and the UpCloud target follow the source and target rules and the
  tier-variant rule recorded here.
- Once `discover` writes inventory files, every retirement or narrowing of a capability needs a new
  inventory format version. Names are hard to change after that point.
- No release is tagged between PRD #14 milestones 1 and 4: in that window the vocabulary accepts
  `high-availability` beside its replacements, a mixed state no release should ship.
- Multi-region availability is not covered. A later `region-high-availability` can extend the chain
  (node < zone < region) without changing this decision.

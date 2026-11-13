# 0023. Separate bare-metal servers from virtual machines in the taxonomy

- Status: Accepted
- Date: 2026-11-13

## Context

Issue #187. `compute.vm` meant "a virtual machine or a dedicated server running a general purpose
operating system". Scaleway's Elastic Metal and OVHcloud's Metal Instances, both dedicated physical
servers, served it beside each provider's virtual machines. For a virtual machine workload the two
offerings tied whenever they lacked as many of the resource's capabilities, and the mapping broke the
tie by offering name (`internal/domain/mapping`, `Map`). In the AWS sample (#179), the auto scaling
group `shop-web-asg` (`autoscaling`, `zone-high-availability`) was assessed against Scaleway as
"`Elastic Metal` (tie: `Instances`)", and its reasons and required changes named bare-metal servers for
a group of VMs, only because "Elastic Metal" sorts first. On OVHcloud every VM tied with Metal
Instances the same way, with Instances winning by name.

The Scaleway snapshot was checked first. Its facts are right: Instances autoscaling groups are still
in public beta, so `autoscaling` stays out under the snapshot's general-availability rule, and the
groups are zonal, so `zone-high-availability` is not claimed (ADR 0011). The tie is legitimate on the
capabilities. What is wrong is that the two offerings compete at all: a virtual machine and a
bare-metal server differ in what they are, not in a capability. Moving a VM to a bare-metal server
changes how it is provisioned, sized, snapshotted and billed.

PRD #2 left open whether a product-preference order per category should replace the name tie-break,
and ADR 0011 recorded more ties as a consequence and pointed back to that question.

## Decision

Split the category. `compute.vm` is a virtual machine running a general purpose operating system, on a
host the provider runs and may share. The new category `compute.baremetal` is a dedicated physical
server running a general purpose operating system, with no hypervisor between the operating system and
the hardware. The doc comments of `taxonomy.ComputeVM` and `taxonomy.ComputeBareMetal` state these
definitions.

- Scaleway's Elastic Metal and OVHcloud's Metal Instances serve `compute.baremetal` only. Scaleway's
  Instances and OVHcloud's Instances serve `compute.vm` only.
- UpCloud has no bare-metal offering, so a `compute.baremetal` resource gets fit `none` there.
- The name tie-break stays as the deterministic last resort. Ties left after the split are between
  tiers of one product (ADR 0011), where the base tier sorting first is the intended reading.

A new category needs no format version: ADR 0006 and ADR 0007 leave the valid category values to the
domain, and `taxonomy.ParseCategory` accepts the new value in an inventory file, a catalog snapshot and
an HTTP request alike.

## Alternatives considered

- **(a) Order tied candidates by how many capabilities they offer.** Provider-neutral, but it promotes
  tier variants over their base whenever the resource uses none of the variant's extra capabilities:
  18 golden files changed, each pointing a workload at a more expensive tier it does not need. It also
  leaves Elastic Metal competing with Instances, and would still pick it whenever it lists more.
- **(b) Report a tie without a single best offering.** Honest, but it changes the JSON report's
  `bestOffering` from always set to sometimes empty, a breaking change that needs a report format
  version bump under ADR 0018, and every reader of the Markdown report loses its one recommendation.
- **(c) An explicit preference per category in each catalog snapshot.** Needs catalog snapshot schema
  v2 under ADR 0007, and records a preference no provider documents: the snapshots hold only sourced
  facts.
- **Correct the Scaleway snapshot.** Checked first: the snapshot is right, so there was nothing to
  correct.

The split removes the cause, a category that mixed two kinds of product, without a format change and
without inventing a preference.

## Consequences

- A virtual machine is never pointed at a bare-metal server, nor the other way round, on any target.
  The VM ties with Elastic Metal and Metal Instances disappear from every report.
- Source adapters classify bare-metal resources as `compute.baremetal`: an AWS EC2 instance of a
  `*.metal` instance type, a GCP Bare Metal Solution instance, an Azure BareMetal Infrastructure
  instance. A plain instance type stays `compute.vm`. The classifying adapters do not exist yet, so
  this is a rule for them, not a change to them.
- The GCP sample in `examples/` gains a Bare Metal Solution instance, so the samples keep covering
  every category.
- An inventory file written before this change stays valid: its `compute.vm` resources keep their
  meaning for virtual machines. One that classified a dedicated server as `compute.vm` should be
  regenerated with `compute.baremetal`.
- PRD #2's open question on a product-preference order per category is closed: the name tie-break
  stays, and the only ties left are between tiers of one product. ADR 0011's consequence "More
  offerings and more ties" stands, and no longer needs a preference order to revisit.
- A future target with a bare-metal product classifies it as `compute.baremetal`, never as
  `compute.vm`.

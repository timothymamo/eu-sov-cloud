# 0028. Group workloads, note blocked relations and sum the source's cost in the assessment

- Status: Accepted
- Date: 2026-11-14

## Context

PRD #190 milestone 7 adds workloads and relations to the assessment: per-workload fit counts, blockers
and estimates, a note on every resource that depends on a blocked resource, and the source's own
`monthlyCost` per workload and in total. ADR 0025 fixes what the inventory holds (`workload`, the closed
relation vocabulary, `monthlyCost` with an amount and a currency); ADR 0027 fixes the estimate package,
its `Total` ("n of m") and exact money. Several choices are left open by both, and the milestone is test
first, written by another model family (ADR 0003), so every type, field, function and rule is fixed here
by name before any test is written:

- which relations carry a blocker to another resource, in which direction, and how far;
- how workload names compare, in which order the groups come, and how "No workload" is represented;
- how the per-workload section is "left out" for a version 1 inventory, when the domain inventory does
  not know its schema version;
- how the source's cost is summed when its amounts are `float64` and its currencies are not EUR, and
  whether it depends on the estimate switch;
- where the code lives, and what the application layer and the comparison need.

Milestone 7 is the domain only. No reporter, CLI or HTTP API changes (milestone 8), so no golden file
changes.

## Decision

The assessment groups its verdicts by workload into `Assessment.Workloads`, gives each verdict the IDs of
the directly related blocked resources in `Verdict.AffectedBy`, and sums the source's cost exactly per
currency in `Assessment.SourceCost` and per workload, with a new `estimate.SumSourceCost`. Everything
lives in the existing packages `assessment` and `estimate`; no package, depguard rule or architecture
pin is added.

### The API

In `internal/domain/assessment`:

```go
type Verdict struct {
	// ... the existing fields ...

	// AffectedBy holds the IDs of the resources this resource depends on or is
	// a replica of (inventory.DependsOn, inventory.ReplicaOf) that have fit
	// mapping.None in this assessment, each once, sorted in byte order, and is
	// nil when there is none.
	AffectedBy []string
}

type WorkloadSummary struct {
	// Name is the workload's name, its runs of white space collapsed to one
	// ASCII space, and "" for the "No workload" group.
	Name string
	// NoWorkload is true for the group of resources that name no workload.
	NoWorkload bool
	// ResourceIDs are the group's resources, in verdict order (ID order).
	ResourceIDs []string
	// Summary counts the group's verdicts per fit; Summary.Total is
	// len(ResourceIDs).
	Summary Summary
	// Blockers are the entries of Assessment.Blockers whose resource is in the
	// group, in their order there, and nil when there is none.
	Blockers []Blocker
	// Total is estimate.Sum over the group's verdicts' estimates, in verdict
	// order, and nil when estimates are off.
	Total *estimate.Total
	// SourceCost is estimate.SumSourceCost over the group's resources.
	SourceCost estimate.SourceCost
}

type Assessment struct {
	// ... the existing fields ...

	// Workloads holds one group per workload, by Name in byte order, then the
	// "No workload" group last when some resource names no workload. It is nil
	// when there is no resource.
	Workloads []WorkloadSummary
	// SourceCost is estimate.SumSourceCost over every resource, in verdict
	// order, whether estimates are on or off.
	SourceCost estimate.SourceCost
}

// ShowWorkloads reports whether a report shows a per-workload section: true
// when at least one resource names a workload, that is, when Workloads holds a
// group with NoWorkload false.
func (a Assessment) ShowWorkloads() bool
```

In `internal/domain/estimate`:

```go
// CurrencyCost is the source's monthly cost in one currency.
type CurrencyCost struct {
	// Currency is the ISO 4217 code, as the inventory gives it.
	Currency string
	// Monthly is the exact sum of the amounts in Currency.
	Monthly Money
	// Resources is how many resources have a cost in Currency.
	Resources int
}

// SourceCost sums what the source charges for some resources.
type SourceCost struct {
	// Currencies holds one sum per currency, by Currency in byte order, and
	// is nil when no resource has a cost.
	Currencies []CurrencyCost
	// Covered is how many resources have a cost (the sum of every
	// CurrencyCost.Resources).
	Covered int
	// Resources is how many resources there are, with a cost or not: a report
	// shows "for Covered of Resources resources".
	Resources int
}

// SumSourceCost sums the MonthlyCost of resources per currency.
func SumSourceCost(resources []inventory.Resource) SourceCost
```

`AssessWith` and `Assess` both fill `AffectedBy`, `Workloads` and `SourceCost`: `Assess` is `AssessWith`
with the zero `Options`, and only `WorkloadSummary.Total` depends on `Options.Estimate`. Fits, reasons,
`Assessment.Blockers`, `Assessment.Summary`, `Assessment.Total` and every estimate are exactly what they
are today.

### The "affected by a blocker" notes

A resource A gets B in `A.AffectedBy` exactly when:

1. A has a relation `{Type: inventory.DependsOn, Target: B}` or `{Type: inventory.ReplicaOf, Target: B}`,
   and
2. B's verdict in the same assessment has fit `mapping.None` (B appears in `Assessment.Blockers`).

- **Direction.** Only from the relating resource to its target: a blocker on the target is listed under
  the resource that relates to it. A blocker on A says nothing about B. `attached-to` and
  `dead-letter-queue-of` give no note: a volume attached to a blocked server or the dead-letter queue of a
  blocked queue do not need the other to work in the way a dependant does, and the PRD names only
  `depends-on` and `replica-of`.
- **Not transitive.** Only direct relations give a note. If A depends on B, B depends on C and C is
  blocked, B gets C and A gets nothing about C; B is not blocked by its note, so A gets nothing about B.
- **One entry per blocked resource.** B is listed once in A's notes however many reasons B's blocker has,
  and also when A both depends on and is a replica of B. Notes are sorted by the blocked resource's ID in
  byte order.
- **Blocked resources get notes too.** A resource with fit none that depends on another blocked one has
  both its own blocker and the note.
- **Every scope.** Notes are computed with and without a region (a region creates blockers: "not offered
  in `<region>`", ADR 0027) and with estimates on or off.
- **Matching.** `inventory.New` stores a relation's target as the target resource's ID as that resource
  writes it (ADR 0025), so B is matched to a verdict by exact string equality. A target that names no
  verdict, which only an inventory built without `inventory.New` can hold, gives no note.

The note is structured: `AffectedBy` holds IDs only. The sentence, "affected by a blocker on `<id>`", is
built by the renderers in milestone 8, which escape the ID as they escape every other ID. A
`mapping.Reason` with parts was considered and is not needed: the note has one fixed phrase and one ID.
The relations themselves need no new field: each verdict already carries them as
`Verdict.Resource.Relations`, sorted by type then target (ADR 0025), for the renderers to list.

### Workloads

- **Name comparison.** A workload is free text (`text.Validate`), stored as given, with no leading or
  trailing white space but any white space inside, a no-break space included. Two names that differ only
  in a run of white space look the same in a report (Markdown collapses runs of spaces), and IDs are
  already compared that way (ADR 0025, relations' targets). So the group key and `WorkloadSummary.Name`
  are the workload with every run of white space (`unicode.IsSpace`, as `strings.Fields` splits) replaced
  by one ASCII space: `"shop  web"` and `"shop web"` are the workload `"shop web"`. Nothing else is
  folded: `"Shop"` and `"shop"` are two workloads. `Verdict.Resource.Workload` keeps the value as given.
- **Order.** Groups by `Name` in byte order (`strings.Compare`), then the "No workload" group last.
  Resource IDs inside a group are in verdict order, which is ID order.
- **"No workload".** The group of the resources whose `Workload` is `""`: `Name` `""` and `NoWorkload`
  true; a workload of only white space counts as none (`inventory.New` refuses one). The label
  "No workload" is the renderer's (milestone 8). It is absent when every resource names a workload.
- **No empty group.** A group exists only for a name some resource carries, so every group has at least
  one resource. An inventory with no resources has `Workloads` nil.
- **Left out.** The domain inventory does not know its schema version, so the rule is about content:
  `Assessment.ShowWorkloads()` is false when no resource names a workload. That covers every version 1
  inventory (it can hold no `workload`), and a version 2 one without workloads, whose only group would
  repeat the totals. `Workloads` is filled either way (a version 1 inventory has its one "No workload"
  group); milestone 8's reports leave the section out when `ShowWorkloads()` is false.
- **Per group.** `Summary`, `Blockers`, `Total` and `SourceCost` are the assessment's own rules applied to
  the group's verdicts, so the groups' counts add up to `Assessment.Summary`, their blockers to
  `Assessment.Blockers`, their `Total.Resources` to `Assessment.Total.Resources`. Notes do not add
  blockers to a group.

### The source's cost

- **Exact.** `SumSourceCost` reads each `MonthlyCost.Amount` as `estimate` reads sizes (ADR 0027): the
  exact decimal of its shortest round-trip representation, rounded to **six fractional digits, half away
  from zero**, then summed exactly as `Money`. Six digits, not cents: rounding each amount to the cent
  before summing would change the user's figures (a thousand resources at 0.004 each are 4, not 0), and
  six digits bound the denominators for the same reason as sizes (an HTTP API client cannot make the sum
  expensive). The sum is rounded only for display, by `Money.String` (to the cent).
- **Per currency, no conversion.** One `CurrencyCost` per currency code, compared exactly (ADR 0025
  allows only three upper-case letters), sorted by code in byte order. A total in two currencies is two
  entries, never one number.
- **Coverage.** `Covered` counts resources with a cost; `Resources` counts every resource given. A report
  shows "for Covered of Resources resources" when they differ. A resource with no cost is never counted
  as 0. A cost that only a resource built without `inventory.New` can hold (a nil, NaN, infinite or
  negative amount, or an empty currency) is read as no cost: not covered.
- **`Money` is currency-neutral.** `estimate.Money` is an exact non-negative amount; its `String` already
  prints no currency sign. Estimates and totals are in euros (ADR 0024); a `CurrencyCost`'s `Monthly` is
  in its `Currency`. The implementation updates `Money`'s doc comment to say so. A second money type
  would repeat `Money`'s arithmetic and formatting for no gain.
- **Independent of the estimate switch.** `SourceCost` is a fact from the inventory, not an estimate, so
  the domain computes it always, with `Options.Estimate` on or off. The display rule, for milestone 8: the
  reports show the source's cost only when estimates are on, next to the estimate, because the PRD says a
  report with `--no-estimate` "says nothing about prices" and shows the source's cost "next to the
  estimate". JSON and Markdown follow the same rule.
- **Why in `estimate`.** It is exact money from inventory floats, which is what `estimate` already does
  (`Money`, the six-digit reading); placing it there reuses that code unexported, and `assessment`
  already imports `estimate`. A package of its own would need its own depguard entries and repeat the
  reading.

### The comparison

Each `TargetAssessment.Assessment` carries its own `Workloads`, `AffectedBy` notes and `SourceCost`.
Notes, blockers, counts and estimates differ per target; the groups' names and resource IDs and the
source's cost do not, since they come from the one inventory. `Comparison` gets no new field.
`Comparison.Validate` additionally checks that every target's `Workloads` holds the first target's
groups in the same order, with the same `Name`, `NoWorkload` and `ResourceIDs`, and returns an error
naming the offending target key and workload name (or the "No workload" group) otherwise. It does not
compare `SourceCost`: equal by construction, and a reporter takes it from the first target.

### The application layer

Nothing changes in `internal/app/analysis`: `Run` and `RunWith` already pass the assessment the domain
returns to the reporter, and `Comparison.RunWith` each target's. A test pins the pass-through: given an
inventory with workloads, relations and costs, `Analysis.RunWith` hands the reporter an assessment equal
(`cmp.Diff`) to `assessment.AssessWith` of the same inventory, catalog and options, with `Workloads`,
`SourceCost` and `AffectedBy` set; `Comparison.RunWith` likewise for each target. The existing
`TestRunWithPassesOptionsThrough` already pins the zero options equal to `Assess`.

### Determinism

No map is iterated to build any output: groups are sorted by name, currencies by code, notes by ID, and
resource IDs follow the verdicts. Nothing reads the clock.

## Alternatives considered

- **Transitive notes (the closure of `depends-on`).** Shows a deep dependency, but ADR 0025 allows cycles,
  so it needs a graph walk with cycle handling, and a chain of notes starts to act like a verdict. The
  PRD says relations add notes, not verdicts; a transitive rule belongs with a later workload readiness
  verdict.
- **Notes for every relation type.** A volume attached to a blocked server is moved with or replaced
  with the server, not broken by it, and a dead-letter queue does not need the queue it serves. The PRD
  names `depends-on` and `replica-of` only.
- **Notes in both directions.** A blocker on a dependant says nothing about whether its dependency can
  move.
- **Notes as `mapping.Reason` values or as blockers.** A blocker changes the per-workload and total
  blocker lists, which the PRD forbids ("no fit changes"); a reason with parts adds a part kind for one
  fixed sentence. An ID list is the smallest form, and the renderers own the sentence.
- **Workload names compared exactly.** Simplest, but `"shop  web"` and `"shop web"` would be two groups
  that render alike. Case folding was also refused: case is visible and may be meant.
- **Leaving the section out by schema version.** Needs the version in the domain inventory, which ADR
  0025 keeps out of it, and would still show a pointless section for a version 2 file with no workload.
- **"No workload" first, or sorted as `""`.** `""` sorts first; the named workloads are what a reader
  looks for, and "No workload" is the remainder, so it comes last.
- **Rounding the source's cost to the cent before summing.** Changes the figures the user gave (above).
- **A separate money type for other currencies.** Duplicates `Money`; `Money` carries no currency.
- **Computing the source's cost only with estimates on.** Couples a fact to the estimate switch in the
  domain; the display rule belongs to the reports.
- **A new domain package for workloads.** The grouping is over verdicts and reuses the assessment's own
  counting; a package of its own would need depguard and architecture entries for a few functions.

## Consequences

- `Verdict` and `Assessment` grow fields that every assessment fills, but no reporter reads them until
  milestone 8, so no golden changes. Domain tests that compare whole assessments with `cmp.Diff` against
  `Assess` keep passing, since both sides are built by the same code.
- Milestone 8 renders the relations (`Verdict.Resource.Relations`), the notes (`AffectedBy`, as
  "affected by a blocker on `<id>`"), the "Workloads" section when `ShowWorkloads()` is true (the "No
  workload" label for `NoWorkload`), and the source's cost per currency with its coverage, only when
  estimates are on.
- `estimate.Money` documents that it is currency-neutral; ADR 0027's rule that `Money` is the only type
  holding a `big.Rat` stays true.
- A later workload readiness verdict can build on `Workloads` and the relations without changing these
  notes.

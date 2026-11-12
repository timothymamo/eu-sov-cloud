# PRD #89: Compare several targets in one assessment

**Status:** Complete. All three milestones are done; the decisions are recorded in
[ADR 0016](../../docs/adr/0016-compare-targets-through-a-second-reporter-port-and-use-case.md).
**Issue:** #89

## Problem

`shift2eu` now has three targets: Scaleway (PRD #2), OVHcloud (PRD #11) and UpCloud (PRD #77). The user's
question is "where should this workload go?", but each run of `shift2eu assess` answers for one target only.
To compare, a platform engineer runs the command three times and puts three reports side by side by hand.
The reports are long, and the facts that matter are spread across them: which resources are `direct` on
one target and `partial` or `none` on another, and which blockers are on every target and so stay
wherever the workload moves.

PRD #11 and PRD #77 both left "several targets in one run" open until all three targets existed, so
that the report would be designed for three targets and not for two. They now exist.

## Outcome

A user can run `shift2eu assess --inventory inventory.json --target ovhcloud,scaleway,upcloud`, or
`--target all`, and get one Markdown report. It starts with a comparison: fits counted per target, then
one row per resource with its fit and best offering on each target, then the blockers that every target
shares. Each target's full report follows, with the same sections as today. A run with one target prints
exactly what it prints today, byte for byte. The comparison states facts per target. It does not rank the
targets or recommend one, because no score exists yet that would justify a ranking (PRD #2's open question
on a readiness score).

## Scope

| | In scope | Out of scope |
|---|---|---|
| Source providers | None live. The versioned inventory file (ADR 0006). | Live discovery |
| Target providers | OVHcloud, Scaleway and UpCloud, from their embedded snapshots as they are | Catalog changes. New targets. |
| Taxonomy categories | Unchanged | New categories or capabilities |
| Assessment | One assessment per target, by today's fit rules, run on the same inventory | New fits. A readiness score, ranking, a recommended target, cost, sizing |
| Output | A Markdown comparison report when more than one target is selected. The single-target report, unchanged. | JSON report (its own PRD). HTML. A diff between two runs. |
| CLI | `--target` taking a comma-separated list of targets, or `all` | A config file. Per-target options. |

## Acceptance criteria

All criteria are pinned by tests that run offline under `make test` or `make test-e2e`, unless marked
*review*.

**Target selection**

- [x] `--target` accepts one target key, a comma-separated list of keys, or `all`. `all` means every
  supported target. Given `--target scaleway,ovhcloud`, the report covers OVHcloud and Scaleway, in sorted
  key order, whatever order they were given in.
- [x] Given a list with an unknown key, a repeated key, an empty element (`ovhcloud,,scaleway`, a leading
  or trailing comma), or `all` together with any other key, the command exits 2. The usage error names
  the bad element and lists the supported targets, sorted. Nothing is read.
- [x] Given a single target, the output is byte-identical to today's. The existing single-target e2e
  golden files do not change in this PRD.
- [x] A multi-target run needs no credentials or network. The e2e test runs it with an empty environment
  and gets the same report.

**Comparison report**

- [x] The report's title names every target compared, in order. Its header lines give the inventory file
  name and source once, then one line per target with that target's catalog snapshot source and
  retrieval date.
- [x] A "Comparison" section comes first, with:
  - a summary table, one column per target and one row per fit (`direct`, `partial`, `none`) plus a
    total, holding the counts that target's own summary shows;
  - one table row per resource, in the inventory's order, with the ID, the category, and for each
    target the fit and the best offering (the tie note left out, which the target's own section keeps);
  - a "Blockers on every target" list: each resource that is a blocker on all the targets compared, with
    each target's reason. "None." when there is none.
- [x] Then one section per target, in the same order. Each holds that target's summary, resources,
  required changes and blockers, with the same content and the same row order as that target's
  single-target report, one heading level lower.
- [x] The comparison holds no ranking, score, "best target" or recommendation. A test pins that the words
  "best target", "recommend" and "winner" do not appear (the per-resource "best offering" is kept).
- [x] Given the same inventory and targets twice, the two reports are byte-identical. Given the targets
  in a different order on the command line, the report is the same.
- [x] Every value from the inventory or a catalog is escaped as the single-target report escapes it.
  The comparison table cannot be broken by a resource ID or an offering name, since it renders them
  the same way.

**Errors**

- [x] If any selected target's catalog fails to load, or the analysis fails for any target, the command
  exits 1 with one error line that names the target, and writes no report (not to stdout, and not to the
  `--output` file).
- [x] An inventory is read once per run, not once per target. Pinned with a fake inventory source that
  counts reads.

**Architecture**

- [x] `make check` passes, including depguard and `test/architecture`.
- [x] The comparison is built from the per-target assessments, and changes none of them. No fit,
  reason or required change in a target's section differs from that target's single-target report for
  the same inventory. Pinned by an e2e test that compares each section with the single-target golden.
- [x] *Review:* the domain's fit rules, the target adapters and their snapshots are unchanged. How the
  application layer runs several catalogs, and how the reporter port takes several assessments, are
  recorded in an ADR (see *Decisions*).

## Milestones

Each milestone ships as one PR, in this order.

1. [x] **Comparison model and use case.** The application layer runs one inventory against several
   catalogs and returns one assessment per target, in key order, reading the inventory once. A domain
   type, or one in the application layer, holds the comparison: the per-target assessments, and the
   blockers every target shares. Its tests pin the order, the shared blockers and the single read. An
   ADR records how the ports change: either a second reporter port for comparisons, or one port that
   takes one or more assessments. The single-target path still uses it, unchanged. No CLI change.
2. [x] **Markdown comparison renderer.** `internal/adapter/report/markdown` renders a comparison as
   described under *Comparison report*. Golden files pin a three-target and a two-target report from
   fixed assessments. The single-target report's goldens do not change. Depends on 1.
3. [x] **CLI, e2e and docs.** `--target` takes a list or `all`, with the usage errors above. The e2e
   test runs the fixture inventory with `--target all`, compares it with a golden comparison report, and
   checks each target's section against that target's single-target golden. README usage and the
   `CLAUDE.md` status line are updated. Moves this PRD to `prds/done/` and closes #89. Depends on 2.

## Decisions and open questions

- **Proposed: one flag, a comma-separated list, plus `all`.** `--target ovhcloud,scaleway,upcloud`
  keeps one flag for one concept and stays a single argument in shells and CI. A repeated flag
  (`--target a --target b`) was the alternative. It reads well, but Go's `flag` package does not support
  it without a custom value type, and it is easier to repeat a target by mistake. `all` saves typing the
  list, and picks up a new target when one is added.
- **Proposed: one report with a comparison first, then the per-target reports.** The alternatives were a
  comparison only (it loses the detail that explains each fit), or one file per target plus a
  comparison file (it needs an output directory, and is harder to read in a terminal or a pull request
  comment). The per-target sections use the existing renderer, one heading level lower, so the detail is
  never written twice.
- **Proposed: no ranking.** Counting `direct` fits per target would be easy to sort by, but it would say
  a target with many direct VMs and one blocked database is better than one with all databases served.
  Without a readiness score, which PRD #2 left open until sizing and workload grouping exist, a ranking
  would overstate what the tool knows. The comparison shows facts per target and leaves the choice to
  the reader.
- **Proposed: "blockers on every target" is the only cross-target finding.** It is a fact that follows
  directly from the per-target verdicts, and it is the most useful one: those resources need a change
  before any of these moves. Other cross-target views (for example "direct only on X") can follow if
  users ask for them.
- **Proposed: a single-target run is unchanged.** Existing users and the existing golden files see no
  difference. The comparison layout is used only when two or more targets are selected.
- **Decided: an ADR for the ports.** The use case and the reporter port change shape. The choice
  between a second port and a widened one is made in milestone 1 and recorded in an ADR (skill: `adr`).
- **Accepted risk: snapshots of different dates.** Each target's snapshot has its own retrieval date,
  and the header shows each one, so a reader can tell when one target's facts are older than another's.
- **Open: JSON output.** A JSON comparison report waits for the JSON report PRD (`jsonreport`), so that
  the single-target and comparison JSON are designed together.

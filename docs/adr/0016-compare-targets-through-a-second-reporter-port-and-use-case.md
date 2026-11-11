# 0016. Compare targets through a second reporter port and use case

- Status: Accepted
- Date: 2026-11-11

## Context

PRD #89 lets one run assess an inventory against several targets (OVHcloud, Scaleway, UpCloud) and
report them side by side, with the blockers every target shares. ADR 0005 gave the application layer
one use case, `analysis.Analysis`, and one `Reporter` port that renders one `assessment.Assessment`.
The PRD requires a single-target run to print exactly what it prints today, byte for byte, and leaves a
JSON comparison report to its own PRD.

## Decision

Add a second reporter port and a second use case beside the unchanged ones, instead of widening
`Reporter`:

```go
type ComparisonReporter interface {
	ReportComparison(ctx context.Context, w io.Writer, c assessment.Comparison, meta ReportMeta) error
}

type Target struct {
	Key     string
	Catalog CatalogSource
}

func NewComparison(inv InventorySource, targets []Target, rep ComparisonReporter) (*Comparison, error)
func (c *Comparison) Run(ctx context.Context, w io.Writer, meta ReportMeta) error
```

- The comparison model lives in the domain: `assessment.TargetAssessment`, `assessment.Comparison` and
  `assessment.Compare`. Shared blockers (resources with fit `none` on every target, with each target's
  reasons) are a pure fact derived from the per-target verdicts. `Compare` sorts a copy of the targets by
  key, changes no assessment, and lists shared blockers in resource ID order, nil when there is none.
- `NewComparison` rejects a nil port (`ErrNilPort`, naming the target for a nil catalog) and a target
  list it cannot compare (`ErrInvalidTargets`: no targets, or a key that is empty, repeated or not valid
  free text). It keeps its own copy of the targets, in key order.
- `Run` validates `ReportMeta` exactly as `Analysis.Run` does, checks the context before reading,
  reads the inventory once, then loads each target's catalog once, in key order, and assesses it. It
  checks the context before each target's catalog, so a cancelled run stops at the next target
  (`target "scaleway": cancelled before loading the catalog: ...`) without loading the rest. Any
  target's failure fails the whole run with one line naming the target
  (`target "scaleway": load catalog: ...`); nothing reaches the reporter or the writer. It checks the
  context again before rendering and wraps a reporter failure as `render report: ...`.

## Alternatives considered

- **Widen `Reporter` to take one or more assessments** (`Report(ctx, w, []Assessment, meta)`). Rejected:
  every reporter and its golden files would change at once, risking the byte-identical single-target
  output the PRD requires; `jsonreport` would have to handle comparisons before the JSON PRD designs
  them; and every reporter would be forced to support comparisons rather than opt in.
- **Compute shared blockers in the reporter.** Rejected: it is a domain fact, not formatting, and every
  reporter would have to repeat it.
- **Return per-target errors and report the targets that succeeded.** Rejected for now: a partial
  comparison would silently drop a target from "blockers on every target".

## Consequences

- The single-target path (`Analysis`, `Reporter`, both reporters and their goldens) is unchanged.
- Two use cases share the metadata validation (`ReportMeta.validate`) and the error texts of their
  stages.
- The CLI picks one use case or the other by the number of targets selected (PRD #89, milestone 3).
- The Markdown reporter implements `ReportComparison` in milestone 2; `jsonreport` does not, until the
  JSON report PRD designs a comparison.

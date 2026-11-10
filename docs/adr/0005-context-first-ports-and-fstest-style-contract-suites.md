# 0005. Declare three context-first ports in the use case and pin adapters with fstest-style contract suites

- Status: Accepted
- Date: 2026-11-10

## Context

ADR 0001 names three ports (inventory source, catalog source, reporter) without giving them a shape.
PRD 2 (issue #2) builds the first use case, `internal/app/analysis`. After it come one adapter per port
(an inventory file, a Scaleway catalog snapshot, a Markdown reporter), and later AWS, Azure, GCP,
OVHcloud, UpCloud and a JSON reporter. Each of those will be built by agents working in parallel from
nothing but the port and its tests, so the ports must be small and the shared behaviour must be
checkable by a test that every adapter runs unchanged.

Constraints that shape the ports:

- The domain is pure: no I/O, no `context`, no clock (ADR 0001). The application layer may use the
  standard library's `context` and `io`.
- Live sources will make slow network calls, so cancellation has to reach them.
- A report names the inventory file it came from (PRD 2, *Report*). That name is untrusted text, and
  every reporter would otherwise have to check it.
- The CLI decides where a report goes (standard output or `--output <path>`).
- go-cmp is a test-only dependency (depguard), and the contract suites are ordinary packages that
  adapter tests import, not `_test` files.

## Decision

`internal/app/analysis` declares three ports, each with a context, and a value for report metadata:

```go
type InventorySource interface {
	Inventory(ctx context.Context) (inventory.Inventory, error)
}

type CatalogSource interface {
	Catalog(ctx context.Context) (catalog.Catalog, error)
}

type ReportMeta struct {
	InventoryName string // base name of the inventory file; empty for a source that is not a file
}

type Reporter interface {
	Report(ctx context.Context, w io.Writer, a assessment.Assessment, meta ReportMeta) error
}
```

`analysis.New(inv, cat, rep)` rejects a nil port (`ErrNilPort`). `(*Analysis).Run(ctx, w, meta)`:

1. validates `meta.InventoryName` before any port is called (empty is allowed): it must be valid free
   text (`text.Validate`) and a base name, never a path. The base-name rule rejects a name that holds
   `/` or `\`, or is `.` or `..`, the same on every OS, so a report never shows the analyst's directory
   layout. The error wraps `ErrInvalidReportMeta` (and `text.ErrInvalid` when the name is not valid free
   text) and shows the name through `text.Quote`;
2. checks `ctx.Err()` itself, so a run cancelled before it starts calls no port;
3. loads the inventory, then the catalog, wrapping a failure as `load inventory: ...` or
   `load catalog: ...`;
4. assesses with `assessment.Assess`;
5. checks `ctx.Err()` itself again, so a cancelled run never starts a report even if an adapter ignored
   the context; the error says the run was cancelled before rendering and does not blame the reporter;
6. renders to `w`, wrapping a failure as `render report: ...`.

Blockers are a result, not an error. The driving adapter chooses and owns `w`; the reporter neither
opens nor closes it.

Adapters are pinned by contract suites in the style of `testing/fstest`:
`internal/adapter/source/sourcetest` and `internal/adapter/target/targettest`. Each exports
`Check(ctx, src, want) error`, which runs every check and returns all violations joined with
`errors.Join`, each prefixed with the name of the check it breaks, and `Run(t, src, want)`, which calls
`Check` with `t.Context()` and fails the test. Because `Check` returns an error, each suite is itself
tested against fakes that violate it on purpose.

- `sourcetest` takes the inventory the adapter must produce from its recorded input. Checks: **want**
  (the result equals it), **repeat** (a second call returns the same inventory), **cancel** (an
  already-cancelled context gives an error wrapping `context.Canceled`).
- `targettest` takes `Expectations{Target, Regions, Categories}` that the adapter declares for itself:
  its display name, its EU regions and the categories its catalog must serve. Checks: **expectations**
  (every field is set), **catalog** (no error), **target**, **snapshot** (source URL and retrieval date
  set), **regions** (every offering is only in the declared EU regions; skipped when Regions is empty),
  **category** (every declared category is served), **repeat** and **cancel**.

The suites compare through the domain accessors (`Source()`, `Resources()`, `Target()`, `Snapshot()`,
`Offerings()`) and `slices.Equal`, not go-cmp, and quote every value they print with `text.Quote`,
an adapter's own error included; that error stays in the violation's chain for `errors.Is`.

## Alternatives considered

- **No context on the ports.** The file and embedded-snapshot adapters do no slow I/O, but AWS, Azure
  and GCP discovery will. Adding a context later would change every port and adapter at once.
- **No context on the reporter only.** Rendering is local today, but a reporter writing to a slow or
  remote writer should stop when the run is cancelled, and one shape for all three ports is easier to
  learn.
- **Reporter returns `[]byte` or opens its own file.** Returning bytes holds the whole report in memory
  and leaves the writing to the use case. Opening a path would put file handling and the
  stdout-or-file choice into every reporter. Passing an `io.Writer` from the driving adapter keeps both
  in one place.
- **Put the inventory name in the domain `Assessment`.** A file name is not part of an assessment: a
  live source has none, and the domain would carry presentation data. A small `ReportMeta` in the
  application layer can grow (tool version, a generation time passed in) without touching the domain.
- **Validate the inventory name in each reporter, or only in the CLI.** Every reporter, and every future
  driving adapter, would have to repeat the check. The use case is the one place all of them pass
  through, and it can refuse before doing any work.
- **Streaming ports (`iter.Seq2[inventory.Resource, error]`).** The assessment needs the whole inventory
  sorted by ID, and the inventory-file adapter will cap the input size (PRD 2 milestone 4), so streaming
  would add complexity without saving memory.
- **Suites that only take a `*testing.T`.** A suite that reports through `t.Errorf` can only be tested
  with a fake `testing.T`. Returning an error from `Check`, as `fstest.TestFS` does, makes each check
  testable with an ordinary violating fake.
- **go-cmp in the suites.** The suites are non-test packages, where depguard forbids go-cmp. The
  domain types are small enough to compare field by field.
- **`targettest` compares the whole catalog to a recorded one.** A catalog snapshot is large and
  changes with every refresh. Each target adapter pins its parsed catalog with its own golden test; the
  suite pins only what every target shares: a name, a dated source, EU regions and the categories it
  claims to serve.

## Consequences

- Every source and target adapter, from the inventory file to OVHcloud, runs the same suite unchanged,
  and a check added to a suite applies to all of them at once.
- An adapter must honour cancellation even when it does no I/O (checking `ctx.Err()` before it starts),
  and must return the same result on every call. A source that can be read only once, such as standard
  input, must buffer its input.
- `targettest.Expectations` must be filled in completely: empty Expectations fields are reported as
  **expectations** violations, so a check with nothing to compare against never passes vacuously.
- Every reporter receives a valid `ReportMeta` and still escapes every value for its own format; the
  use case validates text, it does not escape it.
- Any field added to `ReportMeta` must be validated in `Run` the same way, before any port is called.
  It must never carry account IDs, profile names or anything derived from credentials unless a PRD
  decides the report shows it.
- The application layer's imports are a strict allow list (depguard rule `app-uses-ports-not-adapters` in
  `.golangci.yml`): the domain and `context`, `errors`, `fmt`, `io` and `strings`. Any other import
  fails lint, so I/O stays in the adapters behind the ports; a new package needs its own entry and a
  reason.
- A reporter may write part of its output before failing. The driving adapter decides what to do about a
  partial file (for example, write to a temporary file and rename it), in PRD 2 milestone 7.
- The use case handles one target per run. Comparing several targets in one run will need a new use
  case or a change to this one, recorded in a new ADR.

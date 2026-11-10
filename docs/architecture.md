# Architecture

`eusov` follows a hexagonal (ports and adapters) architecture with a small, explicit domain model.
The decision and its alternatives are in [ADR 0001](adr/0001-hexagonal-architecture-with-provider-neutral-taxonomy.md).

## Layers and the dependency rule

```
            ┌──────────────────────────── cmd/eusov (composition root) ───────────────────────────┐
            │                                                                                      │
            │   internal/cli ──────▶ internal/app/analysis ◀────── internal/adapter/...           │
            │   (driving adapter)      (use case + ports)          (driven adapters: implement     │
            │                                 │                     the ports)                     │
            │                                 ▼                           │                        │
            │                         internal/domain/... ◀───────────────┘                        │
            │                         (stdlib only, no I/O)                                         │
            └──────────────────────────────────────────────────────────────────────────────────────┘
```

Arrows are imports. They point inwards only:

- **Domain** (`internal/domain/...`) imports nothing outside the standard library and other domain
  packages. No I/O, no `context`, no clock, no logging. This is where the value of the project lives,
  so it is the most heavily unit-tested code.
- **Application** (`internal/app/analysis`) orchestrates a use case and declares the interfaces (ports)
  it needs, next to where they are consumed. It never imports an adapter.
- **Adapters** (`internal/adapter/...`) implement ports. Each cloud SDK is confined to one adapter
  package, and its types are translated to domain types at the boundary.
- **CLI** (`internal/cli`) turns user input into use-case calls and prints results. No business rules.
- **`cmd/eusov`** is the only place that constructs adapters and injects them.

The rule is enforced by `depguard` in `.golangci.yml`, so `make lint` fails on a violating import.

## Domain model

| Package | Holds |
|---|---|
| `taxonomy` | Provider-neutral categories (`compute.vm`, `kubernetes.managed`, `storage.object`, `database.postgresql`, `serverless.function`, `messaging.queue`, ...) and the capability vocabulary |
| `inventory` | Resources discovered in a source: category, region, size, capabilities used, dependencies, and the raw provider type for traceability |
| `catalog` | Offerings of a target: categories, capabilities, plans and sizes, EU regions |
| `mapping` | Rules that match a resource to candidate offerings and assign a fit (direct, partial, none) with reasons |
| `assessment` | Per-resource and per-workload verdicts, blockers, required changes, readiness score |
| `text` | Validation and bounded quoting of untrusted free text (IDs, raw types, regions, names); no output-format escaping |

Why a taxonomy: with S sources and T targets, direct service-to-service tables need S×T mappings that
drift independently. With a taxonomy, each provider maps once into a shared vocabulary (S+T), and the
matching rules live in one place.

## Ports (declared in `internal/app/analysis`)

The shapes and their alternatives are in [ADR 0005](adr/0005-context-first-ports-and-fstest-style-contract-suites.md).

| Port | Method | Implemented by | Pinned by |
|---|---|---|---|
| `InventorySource` | `Inventory(ctx) (inventory.Inventory, error)` | `adapter/source/inventoryfile` (the JSON inventory file, [ADR 0006](adr/0006-json-inventory-file-format-v1.md)), `adapter/source/{aws,azure,gcp}` | Contract suite `adapter/source/sourcetest` |
| `CatalogSource` | `Catalog(ctx) (catalog.Catalog, error)` | `adapter/target/{ovhcloud,scaleway,upcloud}` | Contract suite `adapter/target/targettest` |
| `Reporter` | `Report(ctx, w io.Writer, assessment.Assessment, ReportMeta) error` | `adapter/report/{markdown,jsonreport}` | Golden files |

The use case is `analysis.New(inventorySource, catalogSource, reporter)` followed by
`Run(ctx, w, meta)`. Run checks `ReportMeta` (the inventory file's base name must pass `text.Validate`)
before any port is called, loads the inventory and the catalog, assesses, checks the context itself and
renders to `w`. The driving adapter chooses `w` (standard output or a file) and owns it. Blockers are a
result, not an error.

A contract suite exports `Check(ctx, src, want) error`, which returns every violation joined and named
after its check, and `Run(t, src, want)`, which fails the test on a violation. A target adapter declares
its expectations itself: its display name, its EU regions and the categories its catalog must serve.

## Testing strategy

| Level | Scope | Location | Runs in CI |
|---|---|---|---|
| Unit | Domain rules, use cases with fakes | next to the code, `package foo_test` | yes |
| Contract | Behaviour every source or target adapter must share | `adapter/source/sourcetest`, `adapter/target/targettest` | yes |
| Golden | Report and CLI output | `testdata/*.golden` | yes |
| Integration | Real provider APIs, read-only | `//go:build integration` | no (manual, credentials) |
| E2E | The built binary against fixtures | `test/e2e`, `//go:build e2e` | yes |

Most tests sit in the domain, where they are fast and need no fakes. Adapters are tested against
recorded, sanitised API responses through the contract suites, so adding a provider means passing an
existing suite rather than inventing new expectations.

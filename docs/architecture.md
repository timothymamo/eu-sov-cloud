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

Why a taxonomy: with S sources and T targets, direct service-to-service tables need S×T mappings that
drift independently. With a taxonomy, each provider maps once into a shared vocabulary (S+T), and the
matching rules live in one place.

## Planned ports (declared in `internal/app/analysis`)

| Port | Implemented by | Purpose |
|---|---|---|
| Inventory source | `adapter/source/{aws,azure,gcp}` | Discover and classify source resources, read-only |
| Catalog source | `adapter/target/{ovhcloud,scaleway,upcloud}` | Describe target offerings |
| Reporter | `adapter/report/{markdown,jsonreport}` | Render an assessment |

These are intentions, not code yet. The first PRD that implements them will pin their exact shape.

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

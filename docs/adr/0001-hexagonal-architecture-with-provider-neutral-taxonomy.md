# 0001. Use a hexagonal architecture with a provider-neutral taxonomy

- Status: Accepted
- Date: 2026-11-09

## Context

The tool must read infrastructure from three hyperscalers and compare it with three EU providers, and
both lists will grow. Cloud SDKs are large and change often. Most of the value is in the rules that
decide whether a resource can move, and those rules must be testable without cloud access. Several AI
agents will work on the code in parallel, so boundaries must be obvious and checkable by tools.

## Decision

Use ports and adapters. A pure domain (`internal/domain`, standard library only) holds the taxonomy,
inventory, catalog, mapping and assessment models. The application layer (`internal/app`) declares the
ports it needs. Each source and target provider is an adapter in its own package, and cloud SDKs exist
only there. `cmd/eusov` wires everything. The dependency rule is enforced with `depguard`.

Sources and targets meet only through a provider-neutral taxonomy of service categories and
capabilities: each provider maps into it once, instead of maintaining a table for every
source-target pair.

## Alternatives considered

- **Flat packages per provider with direct service-to-service tables.** Quick to start, but needs S×T
  tables that drift independently, and mixes SDK code with business rules.
- **A shared `pkg/` public API.** Nothing outside this repository consumes the code yet; `internal/`
  keeps the freedom to change it.
- **A plugin system (separate binaries per provider).** Unneeded complexity until third parties write
  providers.

## Consequences

- Domain rules are unit-tested with no fakes and no network.
- Adding a provider means implementing one port and passing its contract suite.
- Designing the taxonomy is real up-front work and its quality bounds the quality of the mapping.
- `make lint` fails on a layering violation, which keeps parallel agents from eroding the design.

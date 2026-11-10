# CLAUDE.md

Guidance for every agent working in this repository. Claude Code and pi both load this file.

## What this project is

`eusov` is a Go CLI that analyses infrastructure running in a hyperscaler (**AWS, Azure, GCP**) and
reports what can be moved to an EU cloud provider (**OVHcloud, Scaleway, UpCloud**): which resources
have a direct equivalent, which a partial one, which have none, and what blocks the move.

**Status:** scaffolding only. The package layout, tooling and agent workflow exist; the analysis code does
not yet. Work is planned as PRDs in `prds/`.

## Ubiquitous language

Use these words, in code and prose, with exactly these meanings.

| Term | Meaning |
|---|---|
| **Source** | The hyperscaler being analysed (AWS, Azure, GCP). Accessed **read-only**. |
| **Target** | An EU cloud provider being compared (OVHcloud, Scaleway, UpCloud). |
| **Inventory** | Normalised, provider-neutral list of resources discovered in a source. |
| **Catalog** | What a target offers: services, plans, regions, capabilities. |
| **Taxonomy category** | Provider-neutral service type (`compute.vm`, `storage.object`, `database.postgresql`, ...). The only bridge between sources and targets. |
| **Capability** | A feature a resource depends on or an offering provides (e.g. point-in-time restore, IPv6). |
| **Mapping** | Matching an inventory resource to candidate catalog offerings, with a **fit**: direct, partial, none. |
| **Assessment** | The verdict per resource and workload: portability, blockers, required changes, readiness score. |

## Architecture

Hexagonal (ports and adapters). Dependencies point inwards only. See `docs/architecture.md` and
`docs/adr/`.

| Layer | Path | May import |
|---|---|---|
| Domain | `internal/domain/{taxonomy,inventory,catalog,mapping,assessment,text}` | Standard library and other domain packages only |
| Application | `internal/app/analysis` | Domain. Declares the ports (interfaces) it consumes |
| Adapters | `internal/adapter/source/{aws,azure,gcp}`, `internal/adapter/target/{ovhcloud,scaleway,upcloud}`, `internal/adapter/report/{markdown,jsonreport}` | Domain, the ports they implement, SDKs |
| Driving adapter | `internal/cli` | Application, domain |
| Composition root | `cmd/eusov` | Everything; the only place adapters are constructed |

- The dependency rule is **enforced by `golangci-lint` (depguard)**. Do not add `//nolint:depguard`; fix the design.
- Cloud SDKs live only in their provider's adapter package. SDK types never cross the adapter boundary.
- Providers meet only through the taxonomy. Never write a direct source-service-to-target-service table.
- Every package has a `doc.go` stating its responsibility. Read it before changing the package; update it when the responsibility changes.

## Commands

```bash
make worktree BRANCH=feat/12-short-name   # create or resume the worktree for a PR (prints its path)
make check             # lint + test + govulncheck: must pass before every commit
make test              # unit + contract tests, race detector, no network
make test-integration  # real cloud APIs, read-only credentials (never in default CI)
make test-e2e          # build the binary and run it against testdata fixtures
make lint              # golangci-lint, including architecture rules
make fmt               # gofumpt + goimports
make build             # ./bin/eusov
```

## Testing

- Test first. Behaviour changes start with a failing test (skill: `go-tdd`).
- Table-driven tests, black-box `package foo_test`, standard `testing` plus `go-cmp`. Fakes, not mocks.
- Shared adapter behaviour is pinned by contract suites: `internal/adapter/source/sourcetest`,
  `internal/adapter/target/targettest`. Every provider adapter runs them.
- Renderers and CLI output use golden files under `testdata/`, updated with `-update`.
- Integration tests use `//go:build integration`; e2e tests use `//go:build e2e` and live in `test/e2e`.
- Default tests never touch the network, never need credentials, never depend on time or map order.

## Rules

1. **Never write to a cloud.** Source adapters use read-only APIs only. No create, update, delete or tag calls.
2. **Never commit real data.** Inventories and reports from real accounts stay out of git. Fixtures are
   sanitised (`123456789012`, `192.0.2.0/24`, `example.com`).
3. **Never handle credentials yourself.** Use the SDK's default credential chain. Never read, print, log or
   persist credentials or the files that hold them. The agents' API keys and tokens are in the
   environment and in `~/.config/dot-agent-deck/daemon.env`: never print the environment (`env`,
   `printenv`, `set`), never read that file, and never copy either into files, output or tests.
4. **Never work on `main`.** Every PRD milestone or issue gets its own branch, `<type>/<issue>-<slug>`, in
   its own worktree under `.worktrees/` (`make worktree BRANCH=...`, skill: `start-work`). Work only
   inside that worktree; the repository root stays on `main` and is never edited. Never commit or push to
   `main`, and never pass `--no-verify` to get past the hooks that refuse it. Commit messages are
   Conventional Commits (`feat(mapping): ...`, `fix(aws): ...`, `test(...)`, `docs(...)`).
5. Architectural decisions get an ADR in `docs/adr/` (skill: `adr`).
6. New third-party dependencies need a reason in the commit message. Prefer the standard library.
7. Version bumps come from Renovate PRs (`.github/renovate.json5`), not by hand. Renovate finds `go.mod`
   and workflow actions by itself; any other pinned version (such as the `param:` versions in
   `dev/lima.yaml`) needs a `# renovate: datasource=... depName=...` comment on the line above. Keep those
   comments when editing, and add one to every new pin of that kind.

## Multi-agent workflow

The project is driven by [dot-agent-deck](https://github.com/vfarcic/dot-agent-deck) with six roles,
defined in `.dot-agent-deck.toml`: **orchestrator**, **coder**, **tester**, **reviewer**, **auditor**,
**releaser**. It runs inside an isolated Lima VM (`docs/dev-environment.md`).

| Role | Agent and model | Skills |
|---|---|---|
| orchestrator | Claude Code, Opus | `start-work`, `prd-create` |
| tester | pi, DeepSeek V4.1 Flash | `go-tdd` |
| coder | Claude Code, Opus | `go-implement`, `add-cloud-provider`, `adr` |
| reviewer | Claude Code, Opus | `review-change` |
| auditor | Claude Code, Opus | `security-audit` |
| releaser | Claude Code, Sonnet | `pr-create`, `release-tag` |

Tests and implementation come from different model families on purpose (`docs/adr/0003-...`).
Skills live only in `.claude/skills`: Claude Code finds them there, and pi loads them through
`.pi/settings.json` (which needs `pi --approve`). Keep skills agent-neutral: refer to files and shell
commands, never to one agent's built-in tools.

Workers start with no memory of earlier tasks: everything needed is in the task text, the PRD, this file
and the skills. The first line of every task, `Worktree: <path>`, names the worktree to work in. Workers report only to the orchestrator via `dot-agent-deck work-done`; only the
orchestrator talks to the user. The single human gate is merging a PR.

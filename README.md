# eu-sov-cloud

Analyse what you run on a hyperscaler today, and find out what can move to a European cloud provider.

> **Status: early scaffolding.** The repository layout, tooling, CI and agent workflow are in place.
> The analysis itself is not implemented yet.

## What it will do

`eusov` reads your current infrastructure **read-only** from a source hyperscaler, compares it with the
service catalogs of EU providers, and reports per resource and per workload whether it can move as is,
with changes, or not at all, and why.

| Source (analysed, read-only) | Target (compared) |
|---|---|
| Amazon Web Services | OVHcloud |
| Microsoft Azure | Scaleway |
| Google Cloud | UpCloud |

```
  AWS / Azure / GCP                                   OVHcloud / Scaleway / UpCloud
          │                                                       │
          ▼                                                       ▼
   ┌─────────────┐   classify into    ┌──────────┐   declare    ┌─────────┐
   │  Inventory  │ ─────────────────▶ │ Taxonomy │ ◀─────────── │ Catalog │
   └─────────────┘   categories       └──────────┘   categories └─────────┘
          │                                 │                         │
          └──────────────────────┐          │          ┌──────────────┘
                                 ▼          ▼          ▼
                              ┌──────────────────────────┐
                              │ Mapping (direct/partial/ │
                              │ none) → Assessment       │
                              └────────────┬─────────────┘
                                           ▼
                                Report (Markdown, JSON)
```

Sources and targets never reference each other directly. Both speak a provider-neutral **taxonomy**
(`compute.vm`, `kubernetes.managed`, `storage.object`, `database.postgresql`, ...), so adding a provider
on either side does not mean writing a mapping for every pair. See
[docs/architecture.md](docs/architecture.md).

## Repository layout

```
cmd/eusov/                    CLI entry point and composition root
internal/
  domain/                     Pure business rules, standard library only
    taxonomy/                   provider-neutral service categories
    inventory/                  what runs in the source today
    catalog/                    what a target offers
    mapping/                    matching resources to offerings
    assessment/                 verdicts, blockers, readiness
  app/analysis/               The analyse use case and the ports it needs
  adapter/
    source/{aws,azure,gcp}      read-only discovery   (+ sourcetest contract suite)
    target/{ovhcloud,scaleway,upcloud}  EU catalogs   (+ targettest contract suite)
    report/{markdown,jsonreport}        renderers
  cli/                        Command-line parsing (driving adapter)
  config/                     Configuration loading
  version/                    Build metadata
  testutil/                   Test-only helpers (golden files, fixtures)
test/e2e/                     Black-box tests of the built binary
testdata/                     Shared, sanitised fixtures
docs/                         Architecture, ADRs, dev environment
prds/                         Product requirements, one per feature
dev/lima.yaml                 Isolated VM for running the agents
dev/worktree.sh               Creates the worktree for a branch (make worktree)
.githooks/                    Refuse commits and pushes to main (make git-hooks)
.worktrees/                   One git worktree per pull request (git-ignored)
.dot-agent-deck.toml          Multi-agent orchestration
.claude/skills/               Skills used by the agents (Claude Code and pi)
.pi/settings.json             Points pi at .claude/skills
```

## Development

Requirements: Go (version in `go.mod`), [golangci-lint](https://golangci-lint.run) v2,
[govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck), GNU Make. All of them come
preinstalled in the Lima VM below.

```bash
make help     # list targets
make check    # lint (incl. architecture rules) + tests + vulnerability scan
make build    # ./bin/eusov
```

## Working with agents

The project is built by a team of AI agents orchestrated with
[dot-agent-deck](https://github.com/vfarcic/dot-agent-deck): an **orchestrator** that plans from PRDs
and delegates to a **tester** (writes failing tests first), a **coder**, a **reviewer**, a security
**auditor** and a **releaser** that opens and verifies pull requests. The only human gate is the merge.

The tester runs [pi](https://pi.dev) on DeepSeek V4.1 Flash; every other role runs Claude Code. Tests and
the code they check come from different model families, so one model's blind spots are less likely to
end up in both ([ADR 0003](docs/adr/0003-pi-with-deepseek-for-the-tester-role.md)).

The agents run inside a [Lima](https://lima-vm.io) VM that can see this repository and nothing else on
your Mac:

```bash
brew install lima
make vm-up        # first run provisions Go, Node.js, linters, gh, Claude Code, pi and dot-agent-deck
make vm-env       # copy your Claude, DeepSeek and GitHub credentials into the VM
make vm-shell     # once: `claude` first-run setup, git name and email, `dot-agent-deck daemon install`
make deck         # open the deck; detach and the agents keep running in the VM
```

The credentials come from your shell's environment, for example a direnv `.envrc` above this
repository. Claude Code runs on your Claude subscription, through a token from `claude setup-token`.

Details, including credentials and why Lima rather than devbox:
[docs/dev-environment.md](docs/dev-environment.md).

## Contributing

1. Start from a PRD (`prds/`) or an issue.
2. Work in its own worktree, never on `main`: `make worktree BRANCH=feat/<issue>-<slug>` prints the
   path. The hooks in `.githooks/` refuse commits and pushes to `main` (`make git-hooks` enables them;
   `make vm-up` does too).
3. Test first, then implement; keep `make check` green.
4. Conventional Commits, one PR per milestone.

## License

To be decided.

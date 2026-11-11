# eu-sov-cloud

Analyse what you run on a hyperscaler today, and find out what can move to a European cloud provider.

> **Status: walking skeleton.** `eusov assess` assesses an inventory file against an embedded snapshot
> of the OVHcloud or the Scaleway catalog, offline. Live discovery from AWS, Azure and GCP, and the
> UpCloud catalog, are not implemented yet.

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

## Usage

```text
eusov assess --inventory <file> --target <target> [--output <file>]
```

`<target>` is `ovhcloud` or `scaleway`; `eusov help` lists them. For example:

```bash
make build
./bin/eusov assess --inventory inventory.json --target ovhcloud
./bin/eusov assess --inventory inventory.json --target scaleway --output report.md
```

`assess` reads an inventory file, compares every resource with the target's catalog and writes a
Markdown report: per resource its taxonomy category, best offering, fit (direct, partial, none) and the
reasons, then the required changes and the blockers. Without `--output` the report goes to standard
output. With it, the report is written to the file with mode 0600, and an existing regular file is
replaced atomically, only once the whole report is rendered; a failed or cancelled run leaves it as it
was. A symbolic link to a regular file, or a dangling link, at that path is replaced, not followed.
`--output` naming anything else (a directory, a named pipe, a socket, a device, or a symbolic link to
one of them, such as `/dev/stdout`) or the inventory file itself (the same path, a hard link or a
symbolic link to it) is refused with exit code 1 before the inventory is read; leave out `--output` to
write to standard output.

The inventory file is versioned JSON, described in
[ADR 0006](docs/adr/0006-json-inventory-file-format-v1.md). `--inventory` takes a path; standard input
is not supported. The targets are `ovhcloud` (OVHcloud Public Cloud) and `scaleway`, each served from
a dated snapshot of the provider's EU regions, embedded in the binary, so `assess` runs offline and
needs no credentials.

| Exit code | Meaning |
|---|---|
| 0 | The assessment ran. Blockers are a result, not a failure. |
| 1 | The inventory, the catalog, the report or the output failed, or the run was interrupted (SIGINT, SIGTERM, SIGHUP): one `eusov: ...` line on standard error. |
| 2 | Usage error: missing or unknown flag, command or target. Usage text on standard error. |

SIGQUIT is not part of the exit-code contract: it is the Go runtime's debugging signal, which dumps
every goroutine to standard error and exits with code 2.

`eusov version` prints the version, `eusov help` the usage.

## Run with Docker

The `Dockerfile` builds a minimal image: the static `eusov` binary on
[distroless static](https://github.com/GoogleContainerTools/distroless) (CA certificates and a
non-root user, no shell or package manager), for `linux/amd64` and `linux/arm64`
([ADR 0012](docs/adr/0012-ship-eusov-as-a-distroless-container-image.md)). No image is published to a
registry yet, so build it yourself:

```bash
make image                       # eusov:<version>, eusov:dev and eusov:latest, stamped from git
docker build -t eusov .          # eusov:latest without the version stamp
docker buildx build --platform linux/amd64,linux/arm64 .
```

Both tag `eusov:latest` locally, which is what `eusov` in the commands below resolves to. The entrypoint
is `eusov` and the working directory `/work`. Mount only the inventory file there, read-only, and
since `assess` is offline, give the container no network:

```bash
docker run --rm --read-only --network none \
  -v "$PWD/inventory.json:/work/inventory.json:ro" \
  eusov assess --inventory inventory.json --target scaleway

umask 077   # the report describes your infrastructure: keep the file private
docker run --rm --read-only --network none \
  -v "$PWD/inventory.json:/work/inventory.json:ro" \
  eusov assess --inventory inventory.json --target ovhcloud > report.md
```

The image runs as the non-root user `nonroot` (uid 65532) and works with `--read-only`. Redirect the
report from standard output as above. To use `--output` instead, mount a directory you own and run the
container as your own user, so it can write there without the directory being opened up:

```bash
mkdir -p reports
docker run --rm --read-only --network none --user "$(id -u):$(id -g)" \
  -v "$PWD/inventory.json:/work/inventory.json:ro" -v "$PWD/reports:/out" \
  eusov assess --inventory inventory.json --target ovhcloud --output /out/report.md
```

Never make the report directory world-writable (`chmod 777`) to let the container's user in.

Cloud credentials, for the live sources to come, reach the container the way the providers' SDKs read
them, through their default credential chain, at `docker run` time:

- Environment variables passed **by name only**, so Docker copies the value from your shell:
  `-e AWS_PROFILE -e AWS_REGION -e AWS_ACCESS_KEY_ID ...`. Never write `-e NAME=value`: the value ends
  up in your shell history and in `docker inspect`.
- Or `--env-file` with a file of mode `0600` kept outside the repository.
- Or the provider's config directory mounted read-only into the user's home, for example
  `-v "$HOME/.aws:/home/nonroot/.aws:ro"`. SDK flows that write a token cache there (AWS SSO's
  `~/.aws/sso/cache`, for example) then need that path writable despite `--read-only`: add a
  `--tmpfs` or a volume for it.

**Never** bake credentials into an image (no `COPY` or `ENV` of them, no build args), and never print
or log them.

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
    internal/jsondoc            the strict JSON decoder adapters share
    source/inventoryfile        the JSON inventory file
    source/{aws,azure,gcp}      read-only discovery   (+ sourcetest contract suite)
    target/{ovhcloud,scaleway,upcloud}  EU catalogs   (+ targettest contract suite)
    report/{markdown,jsonreport}        renderers
  cli/                        Command-line parsing (driving adapter)
  config/                     Configuration loading
  version/                    Build metadata
  testutil/                   Test-only helpers (golden files, fixtures)
test/e2e/                     Black-box tests of the built binary
test/lint/                    Source rules golangci-lint cannot express
test/architecture/            Layer dependencies checked package by package, per release target
test/container/               The Dockerfile pinned to go.mod and .goreleaser.yaml
testdata/                     Shared, sanitised fixtures
docs/                         Architecture, ADRs, dev environment
prds/                         Product requirements, one per feature
Dockerfile, .dockerignore     The container image (make image)
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

The agents run inside a [Lima](https://lima-vm.io) VM that can see this repository's workspace (the
directory you clone it into, which holds only the repository and the worktrees dot-agent-deck's
dispatcher creates beside it) and nothing else on your Mac:

```bash
mkdir eu-sov-cloud-ws && cd eu-sov-cloud-ws   # the workspace the VM mounts
git clone https://github.com/timothymamo/eu-sov-cloud.git && cd eu-sov-cloud
brew install lima
make vm-up        # first run provisions Go, Node.js, linters, gh, Claude Code, pi and dot-agent-deck
make vm-env       # copy your Claude, DeepSeek and GitHub credentials into the VM
make vm-shell     # once: `claude` first-run setup, git name and email, `dot-agent-deck daemon install`
make deck         # open the deck; detach and the agents keep running in the VM
```

The credentials come from your shell's environment, for example a direnv `.envrc` above the
workspace. Claude Code runs on your Claude subscription, through a token from `claude setup-token`.

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

`eusov` is licensed under the [MIT License](LICENSE).

The sources cited in `internal/adapter/target/*/SOURCES.md` keep their own licences, which each
`SOURCES.md` states. The embedded catalog snapshots hold only facts taken from them (names, taxonomy
terms, region codes) and brief attributed quotations, so they are not relicensed by this project.

# shift2eu

Analyse what you run on a hyperscaler today, and find out what can move to a European cloud provider.

> **Status: walking skeleton.** `shift2eu assess` assesses an inventory file against an embedded snapshot
> of the OVHcloud, the Scaleway or the UpCloud catalog, offline, or compares several of them in one
> report. `shift2eu-server` serves the same assessments over an HTTP API
> ([Run the HTTP API](docs/guide/http-api.md)). Live discovery from AWS, Azure and GCP
> is not implemented yet.

## What it will do

`shift2eu` reads your current infrastructure **read-only** from a source hyperscaler, compares it with the
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

## Quick start

To see a report without an inventory of your own, follow [Try it](docs/guide/try-it.md): it uses the
dummy sample inventories of the repository's `examples/` directory.

Build from a clone with `make build`, or download a release for your platform
([Install](docs/guide/install.md)) and run its `shift2eu` in place of `./bin/shift2eu`. Then assess an
inventory file ([ADR 0006](docs/adr/0006-json-inventory-file-format-v1.md) describes its format)
against a target, or `all` of them. [Build an inventory from your cloud](docs/guide/inventory.md)
makes one from your AWS, GCP or Azure estate with the provider's read-only CLI and `jq`:

```bash
./bin/shift2eu assess --inventory inventory.json --target all --output comparison.md
```

Or run the image, with no network and the inventory mounted read-only
([Run with Docker](docs/guide/docker.md) explains each flag):

```bash
docker run --rm --read-only --network none --user "$(id -u):$(id -g)" \
  --mount type=bind,src="$PWD/inventory.json",dst=/work/inventory.json,readonly \
  ghcr.io/timothymamo/shift2eu:v0.2.0 assess --inventory inventory.json --target scaleway
```

## Documentation

The [documentation's index](docs/README.md) lists every page. Using `shift2eu`:

- [Install](docs/guide/install.md): the release archives, the images, and building from source.
- [Try it](docs/guide/try-it.md): a walkthrough on a sample inventory, from install to reading the report.
- [Build an inventory from your cloud](docs/guide/inventory.md): an inventory from AWS, GCP or Azure, with read-only CLIs and `jq`.
- [Assess an inventory](docs/guide/assess.md): the `assess` command, its reports and its exit codes.
- [Run with Docker](docs/guide/docker.md): the image, mounts, users, reports and credentials.
- [Verify a release](docs/guide/verify-releases.md): signatures, provenance and SBOMs, and the
  releases before the rename.
- [Run the HTTP API](docs/guide/http-api.md): `shift2eu-server`, its routes and example requests.
- [Behind a reverse proxy](docs/guide/reverse-proxy.md): deploying the server, with its flags and an
  nginx configuration.

Working on `shift2eu`:

- [Development](docs/contributing/development.md): requirements, make targets and how to contribute.
- [Repository layout](docs/contributing/repository-layout.md): where everything lives.
- [Working with agents](docs/contributing/agents.md): the agents' roles and their Lima VM.
- [Architecture](docs/architecture.md), the [development environment](docs/dev-environment.md) and the
  [architecture decision records](docs/README.md#architecture-decision-records).

## Contributing

Start from a PRD (`prds/`) or an issue, work in its own worktree, test first and keep `make check`
green: [Development](docs/contributing/development.md#contributing) has the steps.

## License

`shift2eu` is licensed under the [MIT License](LICENSE).

The sources cited in `internal/adapter/target/*/SOURCES.md` keep their own licences, which each
`SOURCES.md` states. The embedded catalog snapshots hold only facts taken from them (names, taxonomy
terms, region codes) and brief attributed quotations, so they are not relicensed by this project.

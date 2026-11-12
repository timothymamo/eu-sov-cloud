# shift2eu

Analyse what you run on a hyperscaler today, and find out what can move to a European cloud provider.

> **Status: walking skeleton.** `shift2eu assess` assesses an inventory file against an embedded snapshot
> of the OVHcloud, the Scaleway or the UpCloud catalog, offline, or compares several of them in one
> report. `shift2eu-server` serves the same assessments over an HTTP API
> ([Run the HTTP API](#run-the-http-api)). Live discovery from AWS, Azure and GCP
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

## Usage

```text
shift2eu assess --inventory <file> --target <targets> [--format <format>] [--output <file>]
```

`<targets>` is one of `ovhcloud`, `scaleway` or `upcloud`, a comma-separated list of them, or `all` for
every target; `shift2eu help` lists them. For example:

```bash
make build
./bin/shift2eu assess --inventory inventory.json --target ovhcloud
./bin/shift2eu assess --inventory inventory.json --target scaleway --output report.md
./bin/shift2eu assess --inventory inventory.json --target upcloud
./bin/shift2eu assess --inventory inventory.json --target ovhcloud,upcloud
./bin/shift2eu assess --inventory inventory.json --target all --output comparison.md
./bin/shift2eu assess --inventory inventory.json --target scaleway --format json --output report.json
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

`--format` chooses the report format: `markdown` (the default) or `json`. The JSON report holds the same
content as the Markdown one, as a versioned document (`"schemaVersion": 1`, `"kind": "assessment"`, or
`"comparison"` for several targets) for scripts and other tools. Its format, its JSON Schemas
(`internal/adapter/report/jsonreport/schema/`) and its versioning rules are in
[ADR 0018](docs/adr/0018-version-the-json-report-format-for-the-cli-and-the-api.md).

With two or more targets, `assess` reads the inventory once and writes one comparison report. The
targets are compared in sorted key order, whatever order they are given in. The report starts with a
comparison: the fits counted per target, one row per resource with its fit and best offering on each
target, and the resources that are blockers on every target. Each target's own report follows, one
heading level lower. The comparison states facts per target; it does not rank the targets or recommend
one. Its start looks like this:

```markdown
# Migration assessment: `OVHcloud`, `Scaleway`, `UpCloud`

- Inventory file: `inventory.json`
- Source: `aws`
- Catalog: `OVHcloud` snapshot from `https://www.ovhcloud.com/en/public-cloud/regions-availability/`, retrieved 2026-11-11
- Catalog: `Scaleway` snapshot from `https://www.scaleway.com/en/docs/`, retrieved 2026-11-11
- Catalog: `UpCloud` snapshot from `https://upcloud.com/docs/`, retrieved 2026-11-11

## Comparison

### Summary

| Fit | `OVHcloud` | `Scaleway` | `UpCloud` |
|---|---|---|---|
| direct | 6 | 5 | 5 |
| partial | 2 | 5 | 3 |
| none | 3 | 1 | 3 |
| total | 11 | 11 | 11 |

### Resources

| ID | Category | `OVHcloud` fit | `OVHcloud` best offering | `Scaleway` fit | ... |
|---|---|---|---|---|---|
| `fn-example` | `serverless.function` | none |  | direct | ... |

### Blockers on every target

- `misc-example`
  - `OVHcloud`: not classified
  ...
```

A target named twice, an unknown target, an empty element (`ovhcloud,,scaleway`) or `all` together
with another target is a usage error. If any selected target's catalog fails, the run fails with exit
code 1 and writes no report.

The inventory file is versioned JSON, described in
[ADR 0006](docs/adr/0006-json-inventory-file-format-v1.md). `--inventory` takes a path; standard input
is not supported. The targets are `ovhcloud` (OVHcloud Public Cloud), `scaleway` and `upcloud`, each
served from a dated snapshot of the provider's EU regions (for UpCloud, its zones in EU member states),
embedded in the binary, so `assess` runs offline and needs no credentials.

| Exit code | Meaning |
|---|---|
| 0 | The assessment ran. Blockers are a result, not a failure. |
| 1 | The inventory, the catalog, the report or the output failed, or the run was interrupted (SIGINT, SIGTERM, SIGHUP): one `shift2eu: ...` line on standard error. |
| 2 | Usage error: missing or unknown flag, command or target, or a malformed target list. Usage text on standard error. |

SIGQUIT is not part of the exit-code contract: it is the Go runtime's debugging signal, which dumps
every goroutine to standard error and exits with code 2.

`shift2eu version` prints the version, `shift2eu help` the usage.

## Run with Docker

`shift2eu` images and binaries start at `v0.2.0`. `v0.1.0` and `v0.1.0-rc.1` were published under the
project's old names: see [Releases before the rename](#releases-before-the-rename-v010-and-v010-rc1).

Each release publishes a minimal image to `ghcr.io/timothymamo/shift2eu`: the static `shift2eu` binary on
[distroless static](https://github.com/GoogleContainerTools/distroless) (CA certificates and a
non-root user, no shell or package manager), for `linux/amd64` and `linux/arm64`
([ADR 0013](docs/adr/0013-ship-shift2eu-as-a-distroless-container-image.md),
[ADR 0015](docs/adr/0015-publish-the-signed-image-to-ghcr-with-docker-buildx.md)). It is tagged with
the release tag (`vX.Y.Z`), `X.Y` and, for the highest release version that is not a pre-release,
`latest`: a patch to an older line does not move it. A pre-release tag (`v0.2.0-rc.1`) gets only its
own image tag and is a pre-release on GitHub too, never the latest release. Pin a release tag, or better its digest (see
below); `docker run ghcr.io/timothymamo/shift2eu:v0.2.0 version` reports that version without its `v`
(`0.2.0`). The commands below use `v0.2.0`; put the release you want in its place:

```bash
docker pull ghcr.io/timothymamo/shift2eu:v0.2.0
```

The entrypoint is `shift2eu` and the working directory `/work`, so relative paths such as
`--inventory inventory.json` resolve there. Mount only the inventory file, read-only, and since
`assess` is offline, give the container no network:

```bash
docker run --rm --read-only --network none --user "$(id -u):$(id -g)" \
  --mount type=bind,src="$PWD/inventory.json",dst=/work/inventory.json,readonly \
  ghcr.io/timothymamo/shift2eu:v0.2.0 assess --inventory inventory.json --target scaleway

# The report describes your infrastructure: keep the file private.
(umask 077 && docker run --rm --read-only --network none --user "$(id -u):$(id -g)" \
  --mount type=bind,src="$PWD/inventory.json",dst=/work/inventory.json,readonly \
  ghcr.io/timothymamo/shift2eu:v0.2.0 assess --inventory inventory.json --target ovhcloud > report.md)
```

To build the image yourself from a clone instead, `make image` tags `shift2eu:<version>`, `shift2eu:dev` and
`shift2eu:latest`, stamped from git (`docker build -t shift2eu .` builds it without the version stamp); use
`shift2eu` in place of `ghcr.io/timothymamo/shift2eu:v0.2.0` in the commands here.
`make image PLATFORMS=linux/amd64,linux/arm64` builds both release platforms (with Docker, that needs
the containerd image store). `make image` runs `docker`, or `nerdctl` where there is no `docker`, as in
the project's Lima VM; set `DOCKER=` to choose.

### Verify the image

Every published image is signed with [cosign](https://github.com/sigstore/cosign), keyless, by the
release workflow's GitHub identity for that tag, and carries build provenance and an SPDX SBOM as
GitHub attestations. You need cosign v3 or later, which reads the signature from the registry in the
bundle format the release writes, and the [GitHub CLI](https://cli.github.com/).

A tag can be moved; a digest cannot. Resolve the release tag to its digest once, then verify and run
that digest:

```bash
TAG=v0.2.0
DIGEST="$(docker buildx imagetools inspect "ghcr.io/timothymamo/shift2eu:$TAG" --format '{{.Manifest.Digest}}')"
IMAGE="ghcr.io/timothymamo/shift2eu@$DIGEST"

# The signature: made by release.yml, run on a push of exactly this tag.
cosign verify "$IMAGE" \
  --certificate-identity "https://github.com/timothymamo/shift2eu/.github/workflows/release.yml@refs/tags/$TAG" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-github-workflow-trigger push

# The build provenance (SLSA), then the SBOM: from release.yml, for this tag, on GitHub's runners.
gh attestation verify "oci://$IMAGE" --repo timothymamo/shift2eu \
  --signer-workflow timothymamo/shift2eu/.github/workflows/release.yml \
  --source-ref "refs/tags/$TAG" --deny-self-hosted-runners
gh attestation verify "oci://$IMAGE" --repo timothymamo/shift2eu \
  --signer-workflow timothymamo/shift2eu/.github/workflows/release.yml \
  --source-ref "refs/tags/$TAG" --deny-self-hosted-runners \
  --predicate-type https://spdx.dev/Document/v2.3

docker run --rm "$IMAGE" version
```

Inside the project's Lima VM, which has rootless nerdctl and no Docker
([ADR 0017](docs/adr/0017-run-a-rootless-container-runtime-in-the-lima-vm.md)), resolve the digest with
[crane](https://github.com/google/go-containerregistry/tree/main/cmd/crane), which reads only the
index and pulls nothing, and run with `nerdctl`; the `cosign` and `gh` commands are the same:

```bash
# Replaces the DIGEST= line and the final `docker run` line above; TAG, IMAGE, cosign and gh stay as they are.
DIGEST="$(crane digest "ghcr.io/timothymamo/shift2eu:$TAG")"
nerdctl run --rm --read-only --network none "$IMAGE" version
```

Each platform's image is signed too, so `cosign verify` also accepts the `linux/amd64` or
`linux/arm64` digest that `docker buildx imagetools inspect` lists. `gh attestation verify ...
--format json` prints the attested statements, the SBOM's packages among them. The SBOM is attested
on the index digest but generated from its `linux/amd64` image; the `linux/arm64` image is built from
the same source, Go modules and distroless base (`crane digest --platform linux/arm64 ...` prints one
platform's digest). The image's labels
name the source repository (`org.opencontainers.image.source`) and the commit
(`org.opencontainers.image.revision`).

#### Releases before the rename: v0.1.0 and v0.1.0-rc.1

The project was called `eu-sov-cloud` until v0.2.0
([ADR 0020](docs/adr/0020-rename-the-project-to-shift2eu.md)). `v0.1.0` and `v0.1.0-rc.1` keep the
names they were published with: binaries `eusov_*`, image `ghcr.io/timothymamo/eusov`, and the
signing identity of the repository's old name. Verify them with the commands above, with these
changes:

```bash
TAG=v0.1.0   # or v0.1.0-rc.1
DIGEST="$(docker buildx imagetools inspect "ghcr.io/timothymamo/eusov:$TAG" --format '{{.Manifest.Digest}}')"
IMAGE="ghcr.io/timothymamo/eusov@$DIGEST"

cosign verify "$IMAGE" \
  --certificate-identity "https://github.com/timothymamo/eu-sov-cloud/.github/workflows/release.yml@refs/tags/$TAG" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-github-workflow-trigger push

gh attestation verify "oci://$IMAGE" --repo timothymamo/eu-sov-cloud \
  --signer-workflow timothymamo/eu-sov-cloud/.github/workflows/release.yml \
  --source-ref "refs/tags/$TAG" --deny-self-hosted-runners
```

Add `--predicate-type https://spdx.dev/Document/v2.3` to the `gh attestation verify` line for the
SBOM, as above. GitHub redirects the old repository name to the new one.

### Mounts, users and reports

The image's own user is the non-root `nonroot` (uid 65532), which cannot read a file only you may read,
such as an inventory of mode `0600`. `--user "$(id -u):$(id -g)"` runs the container as you instead,
still not as root, so it reads your private files without their mode being loosened; every example here
passes it. `--mount type=bind` fails if the file is missing, where `-v` would quietly create a
root-owned directory in its place. The image works with `--read-only`, and the subshell keeps
`umask 077` from applying to the rest of your session.

With a rootless runtime (rootless Docker, or nerdctl in the project's Lima VM), container user 0 is you,
not root, and `--user "$(id -u):$(id -g)"` maps to a subordinate UID that cannot read your private files:
pass `--user 0:0` instead.

Redirect the report from standard output as above. To use `--output` instead, mount a directory you
own; running as your own user lets the container write there without the directory being opened up:

```bash
mkdir -p reports && chmod 700 reports
docker run --rm --read-only --network none --user "$(id -u):$(id -g)" \
  --mount type=bind,src="$PWD/inventory.json",dst=/work/inventory.json,readonly \
  --mount type=bind,src="$PWD/reports",dst=/out \
  ghcr.io/timothymamo/shift2eu:v0.2.0 assess --inventory inventory.json --target ovhcloud --output /out/report.md
```

Never make the report directory world-writable (`chmod 777`) to let the container's user in.

Live sources do not exist yet. When they do, cloud credentials reach the container the way the
providers' SDKs read them, through their default credential chain, at `docker run` time. Under
`--user`, the container has no home directory of its own (`HOME` is `/`), so set
`-e HOME=/home/nonroot` wherever the SDK looks for config under the home directory:

- Preferably, the provider's config directory mounted read-only, with short-lived credentials in it
  (AWS SSO, `aws sts`) rather than long-lived keys. Its files are usually mode `0600`, which `--user`
  lets the container read. The AWS SSO token cache must stay writable, and must still show the token
  you already have, so bind-mount just that directory writable on top of the read-only mount:

  ```bash
  docker run --rm --read-only --user "$(id -u):$(id -g)" -e HOME=/home/nonroot -e AWS_PROFILE \
    --mount type=bind,src="$HOME/.aws",dst=/home/nonroot/.aws,readonly \
    --mount type=bind,src="$HOME/.aws/sso/cache",dst=/home/nonroot/.aws/sso/cache \
    ghcr.io/timothymamo/shift2eu:v0.2.0 ...
  ```

  A `--tmpfs` there would hide the cached token and SSO would fail. Keep `--tmpfs` for cache paths
  the SDK creates from scratch.
- Or environment variables, passed by name (`-e AWS_PROFILE -e AWS_REGION`) or with `--env-file` (a
  file of mode `0600` kept outside the repository). Never write `-e NAME=value`: the value ends up in
  your shell history and the process list. Passing by name or by file avoids only that. Any variable
  the container gets, however it was passed, is readable with `docker inspect` by anyone with access
  to the Docker daemon for as long as the container exists; `--rm` shortens that window but does not
  close it. Prefer short-lived credentials, or the read-only config mount above, to secrets in
  variables.

**Never** bake credentials into an image (no `COPY` or `ENV` of them, no build args), and never print
or log them.

## Run the HTTP API

`shift2eu-server` serves the same assessments over a small, versioned HTTP API, for a website or any
other client that cannot run the CLI ([ADR 0019](docs/adr/0019-serve-assessments-over-a-versioned-http-api-from-a-separate-binary.md)).
It is a second binary, so `shift2eu` itself still opens no network connection. It takes an inventory
document and target keys, never cloud credentials, and keeps them only in memory while it answers: it
stores nothing and logs no request content. Its answers are byte for byte what `shift2eu assess` prints
for the same inventory and targets.

| Route | Answer |
|---|---|
| `POST /v1/assessments` | The assessment of one target, or the comparison of several (`["all"]` for every target), as the JSON report ([ADR 0018](docs/adr/0018-version-the-json-report-format-for-the-cli-and-the-api.md)) or, with `Accept: text/markdown`, the Markdown report |
| `GET /v1/targets` | Every target's key, name, snapshot source and retrieval date |
| `GET /healthz` | `200 {"status":"ok"}` while the process runs |
| `GET /readyz` | `200 {"status":"ready"}` once every catalog has loaded, `503` until then |

[`api/openapi.json`](api/openapi.json) (OpenAPI 3.1) describes every route, request, response, header and
error; it refers to the report's JSON Schemas in
[`internal/adapter/report/jsonreport/schema/`](internal/adapter/report/jsonreport/schema/) by relative
path. Every error is `{"error": {"code": "...", "message": "..."}}`, and no message echoes a value from
the request.

### Get it

Each release, from the first after `v0.2.0`, publishes `ghcr.io/timothymamo/shift2eu-server` for
`linux/amd64` and `linux/arm64`, with the same tags, signature, provenance and SBOM as the CLI's image:
verify it with the commands of [Verify the image](#verify-the-image), with `shift2eu-server` in place of
`shift2eu` in the image name. It is distroless and runs as `nonroot`, with the entrypoint
`/shift2eu-server`. The release archives hold the `shift2eu-server` binary beside `shift2eu`. From a
clone, `make server` builds `./bin/shift2eu-server` and `make server-image` the image
`shift2eu-server:<version>` (`docker build --target server .` without the version stamp).

### Behind a reverse proxy

The server speaks plain HTTP and knows no users. Run it behind a reverse proxy, or the website's own
backend, that terminates TLS, authenticates each user and limits the rate of requests per client, and
let only that proxy reach it. The server's own defences are a second line, not a replacement:

- **A bearer token.** Set one, of at least 32 characters (`openssl rand -hex 32`), in the environment
  variable `SHIFT2EU_SERVER_TOKEN` or in a file named by `--token-file`, never both; there is no token
  flag, as an argument shows in the process list. Every `/v1/` request then needs
  `Authorization: Bearer <token>`, or it is `401`. `/healthz` and `/readyz` stay open, for probes.
- **Loopback unless told otherwise.** It listens on `127.0.0.1:8080` by default. It refuses to start on
  any other address (`0.0.0.0` in a container) without a token, unless `--insecure-no-auth` says you
  mean it, and then warns at startup. On loopback without a token it answers only to the `Host`
  `localhost` or its own address, against DNS rebinding.
- **Bounded work.** At most `--concurrency` assessments run at once and a request over it is refused at
  once with `503 busy` and `Retry-After: 1`, never queued. A request body is capped at 10 MiB plus
  64 KiB, a slow client is cut off by the read and write timeouts, and at most `--max-connections`
  connections stay open. On `SIGTERM` it stops accepting connections and finishes the requests in
  flight, within 25 seconds.

| Flag or variable | Default | What it sets |
|---|---|---|
| `--listen <host:port>` | `127.0.0.1:8080` | The address to listen on. A non-loopback one needs a token or `--insecure-no-auth`. |
| `SHIFT2EU_SERVER_TOKEN` | unset | The bearer token. Set but empty, or shorter than 32 characters, is a startup error. |
| `--token-file <path>` | unset | Read the token from this file instead, white space trimmed. A file other users can read earns a warning: `chmod 600` it. |
| `--concurrency <n>` | `0` | Assessments served at once. `0` sizes it from the CPUs and the memory budget: `max(1, min(CPUs, budget / 1 GiB))`, the budget being `GOMEMLIMIT` or, without it, 2 GiB. The largest request takes about 1 GB. |
| `GOMEMLIMIT` | unset | The Go runtime's soft memory limit, read by the runtime and by the `--concurrency` default. Set it to the container's memory limit or a little under (`GOMEMLIMIT=1800MiB` for 2 GiB). |
| `--max-connections <n>` | `256` | Connections held open at once; one over the cap is closed at once. |
| `--cors-origin <origin>` | none | Allow CORS from this origin, such as `https://www.example.com` (repeatable). CORS is off without one; never `*`, never credentials. |
| `--allowed-host <host>` | none | Serve only this `Host` header value, with or without a port (repeatable); any other is `421`. Set it to the name the proxy forwards. |
| `--insecure-no-auth` | off | Listen on a non-loopback address without a token. Only behind a proxy that authenticates. |

The server writes its startup lines and an access log to standard error, one line per request with
the method, the path, the status, the duration and the size, and nothing else.

With Docker, publish the port on loopback only, so the proxy on the same host is the one way in, and
pass the token by name, never as `-e NAME=value`:

```bash
export SHIFT2EU_SERVER_TOKEN="$(openssl rand -hex 32)"   # or read it from your secret store
docker run -d --name shift2eu-server --restart unless-stopped --read-only \
  --memory 2g -e GOMEMLIMIT=1800MiB -e SHIFT2EU_SERVER_TOKEN \
  -p 127.0.0.1:8080:8080 \
  ghcr.io/timothymamo/shift2eu-server:<version> \
  --listen 0.0.0.0:8080 --allowed-host api.example.com
```

To keep the token out of the container's environment (`docker inspect` shows it), mount a token file
read-only instead and run as its owner, as [Mounts, users and reports](#mounts-users-and-reports)
explains: replace `-e SHIFT2EU_SERVER_TOKEN` with
`--user "$(id -u):$(id -g)" --mount type=bind,src="$PWD/shift2eu-token",dst=/run/secrets/shift2eu-token,readonly`
and add `--token-file /run/secrets/shift2eu-token`.

A minimal nginx in front of it: TLS, a password per user, a rate limit per client address, a body
cap just above the server's, and the token added on the way in, so users never see it. The health
endpoints are left off the public name; probe them on `127.0.0.1:8080`.

```nginx
limit_req_zone $binary_remote_addr zone=shift2eu:10m rate=2r/s;

server {
    listen 443 ssl;
    server_name api.example.com;
    ssl_certificate     /etc/ssl/api.example.com/fullchain.pem;
    ssl_certificate_key /etc/ssl/api.example.com/privkey.pem;

    location /v1/ {
        auth_basic           "shift2eu";
        auth_basic_user_file /etc/nginx/shift2eu.htpasswd;
        limit_req            zone=shift2eu burst=10 nodelay;
        client_max_body_size 11m;

        proxy_set_header Host $host;
        # One line, mode 0600: proxy_set_header Authorization "Bearer <the token>";
        include /etc/nginx/shift2eu-token.conf;
        proxy_pass http://127.0.0.1:8080;
    }

    location / {
        return 404;
    }
}
```

### Example requests

An assessment request is a JSON object with the inventory document ([ADR 0006](docs/adr/0006-json-inventory-file-format-v1.md))
and the target keys. Started from a clone with a token, the server listens on its default loopback
address; the requests read the token from the same variable, which keeps it out of your shell history.
With the repository's sanitised example inventory:

```bash
export SHIFT2EU_SERVER_TOKEN="$(openssl rand -hex 32)"
make server && ./bin/shift2eu-server &   # listening on 127.0.0.1:8080

jq '{inventory: ., targets: ["scaleway"]}' test/e2e/testdata/inventory.json > request.json

# The JSON report.
curl -sS -X POST http://127.0.0.1:8080/v1/assessments \
  -H "Authorization: Bearer $SHIFT2EU_SERVER_TOKEN" \
  -H 'Content-Type: application/json' \
  --data-binary @request.json

# The Markdown report, comparing every target.
jq '{inventory: ., targets: ["all"]}' test/e2e/testdata/inventory.json > request.json
curl -sS -X POST http://127.0.0.1:8080/v1/assessments \
  -H "Authorization: Bearer $SHIFT2EU_SERVER_TOKEN" \
  -H 'Content-Type: application/json' -H 'Accept: text/markdown' \
  --data-binary @request.json

# The targets, and the health checks.
curl -sS -H "Authorization: Bearer $SHIFT2EU_SERVER_TOKEN" http://127.0.0.1:8080/v1/targets
curl -sS http://127.0.0.1:8080/healthz
curl -sS http://127.0.0.1:8080/readyz
```

Through the proxy above, the same requests go to `https://api.example.com/v1/...` with the user's own
credentials (`curl -u <user>`) in place of the token.

## Repository layout

```
cmd/shift2eu/                 CLI entry point and composition root
cmd/shift2eu-server/          HTTP API entry point and composition root
api/openapi.json              The HTTP API's OpenAPI 3.1 document
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
    request/assessmentrequest   the HTTP API's request decoder
  cli/                        Command-line parsing (driving adapter)
  httpapi/                    The HTTP API (driving adapter)
  config/                     Configuration loading
  version/                    Build metadata
  testutil/                   Test-only helpers (golden files, fixtures)
test/e2e/                     Black-box tests of the built binary
test/lint/                    Source rules golangci-lint cannot express
test/architecture/            Layer dependencies checked package by package, per release target
test/container/               The Dockerfile's two images pinned to go.mod and .goreleaser.yaml
testdata/                     Shared, sanitised fixtures
docs/                         Architecture, ADRs, dev environment
prds/                         Product requirements, one per feature
Dockerfile, .dockerignore     The container images (make image, make server-image)
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
make build    # ./bin/shift2eu and ./bin/shift2eu-server
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
mkdir shift2eu-ws && cd shift2eu-ws   # the workspace the VM mounts
git clone https://github.com/timothymamo/shift2eu.git && cd shift2eu
brew install lima
make vm-create    # create and provision the VM (Go, Node.js, linters, gh, Claude Code, pi, dot-agent-deck),
                  # copy your Claude, DeepSeek and GitHub credentials in, and do the once-only setup
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

`shift2eu` is licensed under the [MIT License](LICENSE).

The sources cited in `internal/adapter/target/*/SOURCES.md` keep their own licences, which each
`SOURCES.md` states. The embedded catalog snapshots hold only facts taken from them (names, taxonomy
terms, region codes) and brief attributed quotations, so they are not relicensed by this project.

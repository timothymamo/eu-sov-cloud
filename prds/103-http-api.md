# PRD #103: Serve assessments over an HTTP API

**Status:** Draft. The decisions marked *Proposed* are for the user to accept or change when this PRD's
pull request is reviewed.
**Issue:** #103

## Problem

The user wants to run `eusov` as the backend of a website at a later stage: a visitor uploads an
inventory, the site calls `eusov`, and shows the assessment. Today that is not possible:

- `eusov` is a CLI. Its only commands are `assess` and `version`, and nothing in the module serves
  HTTP.
- The only report format is Markdown. `internal/adapter/report/jsonreport` is an empty package, and every
  PRD so far deferred JSON to "its own PRD". A website needs data it can lay out, not Markdown.
- The CLI binary is guaranteed offline. `TestAssessPathHasNoNetworkOrCloudSDK` (`test/e2e`) fails if
  `cmd/eusov` depends on `net` or `net/http` at all. That is a promise to CLI users, that the tool never
  opens a connection, and it should not be given up to add a server.

A web backend also brings risks the CLI does not have. An inventory describes someone's infrastructure,
so it is sensitive (CLAUDE.md rule 2). Untrusted clients can send hostile or huge bodies, and many
requests at once.

## Outcome

A second binary, `eusov-server`, serves the same assessments over a small, versioned JSON API:
`POST /v1/assessments` takes an inventory document and one or more targets, and returns the assessment
(or the comparison, PRD #89) as JSON. `GET /v1/targets` lists the targets and their snapshot dates.
Health endpoints let a platform run it. It is stateless: it stores nothing, and logs no request content.
It has the same strict input rules as the CLI, plus HTTP limits (body size, timeouts, concurrency). It is
meant to run behind the website's own backend or a reverse proxy, which handles TLS and users. The CLI
`eusov` stays offline, and its output does not change. Both share one JSON report format, which the CLI
also offers as `eusov assess --format json`.

## Scope

| | In scope | Out of scope |
|---|---|---|
| Source providers | None live. Inventories arrive as inventory documents (ADR 0006), in the request body. | Live discovery through the API. It would mean the server accepting cloud credentials, which needs its own PRD and threat model (see *Decisions*). |
| Target providers | OVHcloud, Scaleway and UpCloud, from their embedded snapshots as they are | Catalog changes |
| Output | A versioned JSON report (schema v1) for one assessment and for a comparison, shared by the CLI (`--format json`) and the API. The Markdown report, available from the API too. | HTML. PDF. Changes to the Markdown report. |
| API | `POST /v1/assessments`, `GET /v1/targets`, `GET /healthz`, `GET /readyz`, an OpenAPI 3.1 document in the repository | Users, accounts, sessions, storage of inventories or reports, async jobs, webhooks, a web UI |
| Deployment | A separate `eusov-server` binary and container image, built and signed by the release workflow like `eusov`. Docs for running it behind a reverse proxy. | A hosted service. TLS termination or user authentication inside the server (the proxy's job, see *Decisions*). |

## Acceptance criteria

All criteria are pinned by tests that run offline under `make test` or `make test-e2e` (the API's tests
use `net/http/httptest` and a server on `127.0.0.1`), unless marked *review*.

**JSON report**

- [x] `eusov assess --format json` prints the assessment as JSON. Its document has a `schemaVersion` of
  `1`, and holds the target, the snapshot source and date, the inventory name, the summary counts, and
  every verdict with its fit, best offering, ties, reasons and required changes, as well as blockers.
  This is the same content as the Markdown report, in the same resource order. `--format markdown` is
  the default, and its output is byte-identical to today's.
- [x] With several targets (PRD #89), `--format json` prints a comparison document. It holds the
  per-target assessments and the blockers every target shares.
- [x] Golden files pin both documents. The output is byte-identical across runs. A JSON Schema for each
  document is in the repository, and a test validates the goldens against it.
- [x] Every string comes from the inventory or a catalog, and is JSON-encoded, never concatenated.
  Hostile values (quotes, backslashes, control characters, invalid UTF-8) produce valid JSON. Pinned by
  a hostile golden.

**API**

- [ ] `POST /v1/assessments` with `Content-Type: application/json`, and a body holding `inventory` (an
  inventory document) and `targets` (one or more target keys, or `["all"]`), answers `200` with the JSON
  report (one target) or comparison (several). The body is the same document `eusov assess --format json`
  prints for the same inventory and targets. Pinned by comparing the two.
- [ ] With `Accept: text/markdown`, the same request answers with the Markdown report, byte-identical to
  the CLI's.
- [ ] `GET /v1/targets` lists every target with its key, display name, snapshot source URL and retrieval
  date, sorted by key.
- [ ] `GET /healthz` answers `200` while the process runs. `GET /readyz` answers `200` once every
  target's catalog has loaded, and `503` before that.
- [ ] Errors use one JSON shape (`{"error": {"code": "...", "message": "..."}}`) and the right status:
  - `400` for a malformed body, an inventory the reader refuses, or a bad target list (the same rules
    as the CLI's usage errors);
  - `404` or `405` for an unknown path or method;
  - `413` for a body over the limit;
  - `415` for the wrong content type;
  - `429` or `503` when the concurrency limit is reached;
  - `500` for an internal error, with no internal detail in the message.

  An error message never echoes request content beyond a quoted, escaped key or field name, as the
  CLI's one-line errors do.
- [ ] The OpenAPI 3.1 document describes every endpoint, request, response and error. A test checks that
  the handler's routes, status codes and JSON shapes match it.

**Limits and hardening**

- [ ] The request body is capped (*Proposed*: the inventory reader's existing size cap, plus a small
  margin for the envelope), and read through the same strict decoder (`jsondoc`, ADR 0009). An unknown
  field, a duplicate key or trailing data is a `400`.
- [ ] The server sets read-header, read, write and idle timeouts, and a maximum header size. A slow or
  oversized client gets a timeout or a `431`/`413`, and never blocks a worker for longer than the
  timeouts. Pinned with a slow-client test.
- [ ] Concurrent assessments are capped (*Proposed*: a flag, defaulting to the number of CPUs). A request
  over the cap is refused at once, not queued without bound.
- [ ] Nothing from a request (body, inventory, report, resource IDs) is written to disk or to the logs.
  The access log has the method, path, status, duration and size only. Pinned by a test that sends a
  marked inventory and checks that no log line holds the marker.
- [ ] Responses set `Cache-Control: no-store`, `X-Content-Type-Options: nosniff` and a JSON content type.
  CORS is off unless an allow-list of origins is configured, and then it applies only to those origins.
- [ ] *Proposed:* when a bearer token is configured (from an environment variable or a file, never a
  flag), every `/v1/` request without it gets a `401`. The health endpoints stay open. The server
  refuses to listen on a non-loopback address without a token, unless the operator passes an explicit
  `--insecure-no-auth`.
- [ ] On `SIGTERM` or `SIGINT`, the server stops accepting connections and finishes in-flight requests
  within a deadline, then exits `0`.

**Architecture**

- [ ] The HTTP API is a driving adapter, `internal/httpapi`, beside `internal/cli`. It may import the
  application and domain layers, and the report adapters through the composition root's wiring, but never
  `internal/cli`. `cmd/eusov-server` is its composition root. depguard rules and `test/architecture` hold
  this for every release target, and the depguard pin (#84) includes the new rules.
- [ ] `cmd/eusov` still depends on no `net` package: `TestAssessPathHasNoNetworkOrCloudSDK` is unchanged
  and passes. A matching test pins that `cmd/eusov-server` depends on no cloud SDK and no `os/exec`.
- [ ] *Review:* the domain, the use cases (`analysis.Analysis`, `analysis.Comparison`), the target
  adapters and the Markdown renderer are reused unchanged. An ADR records the separate binary, the API's
  versioning, and the security model.
- [ ] `make check` passes. The security-audit skill is run on milestones 2 to 4.

**Packaging**

- [ ] The release workflow builds `eusov-server` for the release targets, and publishes its container
  image `ghcr.io/timothymamo/eusov-server`. The image is non-root and distroless, signed and attested like
  `eusov`'s (ADR 0015), with `ENTRYPOINT ["/eusov-server"]`. `check.ReleaseWorkflow` and the CI dry run
  cover it.
- [ ] The README documents running the server behind a reverse proxy (TLS, authentication, rate limits),
  the environment it reads, and an example request.

## Milestones

Each milestone ships as one PR, in this order.

1. [x] **JSON report v1.** `internal/adapter/report/jsonreport` renders an assessment and a comparison as
   versioned JSON, with JSON Schemas, goldens and hostile goldens. `eusov assess --format json|markdown`.
   An ADR records the schema and its versioning, the counterpart of ADR 0006 for inventories. No HTTP yet.

   *Done:* `jsonreport` renders both documents (`schemaVersion` 1), with JSON Schemas under
   `internal/adapter/report/jsonreport/schema/`, goldens and hostile goldens validated against them,
   and `--format json` e2e goldens. `eusov assess --format json|markdown` selects the renderer;
   Markdown stays the default and unchanged. A comparison is validated once, in the domain, for every
   reporter. The format and its version policy are recorded in
   [ADR 0018](../docs/adr/0018-version-the-json-report-format-for-the-cli-and-the-api.md).

2. [ ] **HTTP API adapter.** `internal/httpapi`: the routes, request decoding and limits, error shapes,
   content negotiation, headers, and the OpenAPI document, tested with `httptest` and fakes for the use
   cases. An ADR records the API's design, versioning and security model. depguard and
   `test/architecture` rules for the new package. Depends on 1.
3. [ ] **`eusov-server` binary.** `cmd/eusov-server` wires the adapters. It adds flags and environment
   (listen address, token, concurrency, CORS origins), timeouts, graceful shutdown, the access log, the
   readiness check, and the loopback-without-token rule. e2e tests start the binary on `127.0.0.1` and call
   it. The offline test for `cmd/eusov` stays as it is, and a new one pins `cmd/eusov-server`'s
   dependencies. Depends on 2.
4. [ ] **Packaging and docs.** GoReleaser builds, the `eusov-server` image in the `Dockerfile`, the
   release workflow (publish, sign, attest) and its checks, and the README's deployment section. Moves this
   PRD to `prds/done/` and completes #103. Depends on 3. Security-sensitive: the release workflow changes.

## Decisions and open questions

- **Proposed: a separate binary, not `eusov serve`** ([ADR 0019](../docs/adr/0019-serve-assessments-over-a-versioned-http-api-from-a-separate-binary.md)
  records it with the API's design, versioning and security model). The CLI's guarantee, that nothing it reaches can
  open a connection, is pinned by a test, and is worth keeping for users who run it on sensitive
  inventories. A `serve` subcommand would put `net/http` in the CLI's dependencies and end that guarantee.
  A second binary in the same module reuses every package. It costs a second build target and image.
- **Proposed: the API takes inventory documents only, never cloud credentials.** Live discovery from the
  API would mean a web server holding customers' cloud credentials, the most sensitive thing this project
  could handle (CLAUDE.md rules 1 and 3). If the website needs it later, it gets its own PRD and threat
  model. Until then the website, or the user, runs discovery and uploads the inventory.
- **Proposed: TLS and users belong to the proxy or the website's backend.** The server is a stateless
  compute service: no accounts, no sessions, no storage. A shared bearer token protects it from anyone
  who reaches it directly. Users, rate limits per user, and TLS are better handled by the site's backend
  or a reverse proxy, which the docs describe. Built-in TLS can follow if a deployment needs it.
- **Proposed: synchronous requests.** An assessment over an embedded snapshot takes milliseconds, so a
  request answers directly. Async jobs, queues and webhooks are left out until something makes requests
  slow, such as live discovery.
- **Decided: one JSON report format for the CLI and the API** ([ADR 0018](../docs/adr/0018-version-the-json-report-format-for-the-cli-and-the-api.md)).
  Users can script the CLI and the website can call the API, and both read the same document. A test
  pins that both give the same bytes. The format carries a `schemaVersion`: adding an optional key or a
  new value of an open set (such as a reason code) keeps it, and readers ignore what they do not know;
  removing, renaming or retyping a key, or any other change that can break a reader, is a new
  `schemaVersion` with an ADR. Unlike the inventory format (ADR 0006), where `eusov` is the strict
  reader, here `eusov` is the writer.
- **Proposed: the API is versioned in the path (`/v1/`).** A breaking change is a `/v2/` route beside
  `/v1/` for a deprecation period. Additive fields are not breaking.
- **Decided: no request content is ever stored or logged.** Inventories and reports are customer data
  (CLAUDE.md rule 2). The server keeps them only in memory for the length of the request.
- **Open: a readiness score.** A website will want one number per target, but PRD #2's open question
  still applies: no score until sizing and workload grouping exist. The API returns the same facts as the
  CLI and nothing more.
- **Open: hosting.** Where the website and server run (and so which proxy, TLS and auth setup) is not
  decided. The PRD keeps the server deployable anywhere a container runs.

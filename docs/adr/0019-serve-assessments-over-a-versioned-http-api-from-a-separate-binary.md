# 0019. Serve assessments over a versioned HTTP API from a separate binary

- Status: Proposed
- Date: 2026-11-11

## Context

PRD #103 (issue #103) makes `eusov` usable as the backend of a website: a client sends an inventory and
gets the assessment back. Milestone 1 gave the CLI and the API one versioned JSON report format
([ADR 0018](0018-version-the-json-report-format-for-the-cli-and-the-api.md)). Milestone 2 adds the HTTP
driving adapter, and this ADR records its design, versioning and security model.

Forces:

- The CLI is guaranteed offline. `TestAssessPathHasNoNetworkOrCloudSDK` (`test/e2e`) fails if
  `cmd/eusov` depends on `net` or `net/http` at all. That promise to users who run it on sensitive
  inventories must survive.
- An inventory describes someone's infrastructure (CLAUDE.md rule 2). A server receives it from
  untrusted clients, who may send hostile or huge bodies, and many requests at once.
- Input rules must not drift between the CLI and the API: the strict decoder `jsondoc`
  ([ADR 0009](0009-decode-adapter-json-documents-through-one-strict-decoder.md)) and the inventory
  reader's rules ([ADR 0006](0006-json-inventory-file-format-v1.md)) are the only ones.
- `jsondoc` lives in `internal/adapter/internal/jsondoc`, so Go's internal rule lets only packages under
  `internal/adapter/` import it, and the inventory reader, `inventoryfile`, only reads a file path.
- The dependency rule (ADR 0001) is enforced by depguard and, package by package for every release
  target, by `test/architecture` ([ADR 0012](0012-check-layer-dependencies-by-package-for-every-release-target.md)).
  The driving adapter `internal/cli` already shows the shape: it gets every adapter from the
  composition root through `cli.Deps`.
- The use cases `analysis.Analysis` and `analysis.Comparison`
  ([ADR 0016](0016-compare-targets-through-a-second-reporter-port-and-use-case.md)), the target adapters
  and the reporters are reused unchanged.

## Decision

A second binary, `eusov-server`, serves a small JSON API under `/v1/`, through a new driving adapter
`internal/httpapi`. The API takes inventory documents only, never cloud credentials, stores and logs
nothing from a request, and leaves TLS and users to a reverse proxy or the website's backend.

**A separate binary.** `cmd/eusov-server` is the API's composition root (milestone 3). `cmd/eusov`
never imports `internal/httpapi` (depguard rule `httpapi-imported-only-by-the-server`), so the CLI's
offline test stays unchanged and passes. Both binaries are built from the same module and reuse every
package.

**Three pieces, so strict decoding stays in the adapters.**

1. `inventoryfile.FromBytes(data)` returns the same `*Source` as `New(path)`, over an in-memory
   inventory document: the same `MaxFileSize` cap, the same strict decoding and sentinels, and the
   `sourcetest` contract suite.
2. A new adapter, `internal/adapter/request/assessmentrequest`, decodes the body of
   `POST /v1/assessments` through `jsondoc`: one object with exactly the keys `inventory` (an object,
   returned as its raw bytes) and `targets` (a non-empty array of strings, in order). Anything else wraps
   `assessmentrequest.ErrDecode`, with jsondoc's wording, which never quotes a value from the body.
   `MaxBodySize` is the inventory cap plus 64 KiB for the envelope; an adapter imports no other adapter,
   so the constant is written out and a test pins it to `inventoryfile.MaxFileSize + 64<<10`. Its imports
   are a strict allow list (`request-decoders-parse-bytes`): `errors`, `fmt` and `jsondoc`, no `net/http`.
3. `internal/httpapi` is a driving adapter beside `internal/cli`. `httpapi.Deps` takes
   `DecodeRequest` (`assessmentrequest.Decode`), `NewInventorySource` (`inventoryfile.FromBytes`),
   `Targets` (target key to catalog source constructor, as `cli.Deps.Targets`) and the `JSON` and
   `Markdown` reporter pairs (as `cli.Format`). `New(deps, opts...)` returns an `http.Handler`, with
   options for the body cap, the concurrency cap, the bearer token, the CORS origins and the readiness
   check. Its imports are a strict allow list of named standard packages, the application, the domain
   and `internal/version` (`httpapi-gets-adapters-from-cmd`); it never imports an adapter,
   `internal/cli`, `cmd/` or `os`. Because each standard package is named rather than allowed through
   `$gostd`, `test/architecture` holds it package by package for every release target, as it holds the
   offline adapters.

**Routes.**

- `POST /v1/assessments`: `Content-Type: application/json` and `{"inventory": {...}, "targets": [...]}`.
  One target runs `analysis.Analysis`; several, or `["all"]`, run `analysis.Comparison` in sorted key
  order. The target list follows the CLI's rules (unknown, repeated or empty keys, and `all` with another
  key, are refused). The 200 body is what `eusov assess --format json` prints for the same inventory and
  targets, byte for byte, and a parity test in `test/apiparity` compares the two.
- `GET /v1/targets`: every target's key, display name and snapshot (`source`, `retrieved`), sorted by
  key, in the shape of the JSON report's `target` object.
- `GET /healthz` (always 200) and `GET /readyz` (200 when the readiness check passes, else 503).
- An unknown path is 404; a known path with another method is 405 with an `Allow` header.

**Report name.** The request carries no file name, so `ReportMeta.InventoryName` is the constant
`inventory.json`. The parity test names its CLI input `inventory.json` so the bytes can match.

**Content negotiation.** With no `Accept`, or `*/*`, `application/*` or `application/json`, the answer is
the JSON report as `application/json`. With `text/markdown` or `text/*` it is the Markdown report,
byte-identical to the CLI's, as `text/markdown; charset=utf-8`. A `q=0` range is excluded. When nothing
the client accepts is available the answer is **406 `not_acceptable`**, a status the PRD's list did not
name: answering JSON to a client that refused it would hide the mismatch.

**One error shape.** Every error, 404 and 405 included, is
`{"error": {"code": "<code>", "message": "<message>"}}` as `application/json`, with the codes
`invalid_request`, `invalid_inventory`, `invalid_targets` (400), `unauthorized` (401), `not_found` (404),
`method_not_allowed` (405), `not_acceptable` (406), `payload_too_large` (413),
`unsupported_media_type` (415), `internal` (500), `busy` and `not_ready` (503). A message never echoes
request content beyond a quoted, escaped key or field name: no target value, header value or path the
client sent. A 500 says `internal error` and nothing more; a panic is recovered into one.

**Versioning.** The API is versioned in the path. Within `/v1/`, adding a route, an optional request
key that changes nothing for a client that leaves it out, a response key, a response header or an
error code is not breaking: clients ignore what they do not know. Removing or renaming a route or a key,
changing a key's type or meaning, making an optional request key required, or changing a status code
for a request that used to succeed is breaking, and goes in a `/v2/` route beside `/v1/` for a
deprecation period, with an ADR. The report documents inside a 200 follow ADR 0018 on their own: a new
report `schemaVersion` is a breaking change of the report, and so of the route that serves it. The
OpenAPI 3.1 document `api/openapi.json` describes `/v1/`, and a test in `internal/httpapi` holds the
handler's routes, status codes and JSON shapes to it. It refers to the report's JSON Schemas in
`internal/adapter/report/jsonreport/schema/` by relative `$ref` rather than copying them, so they stay
the one source of truth for the report.

**Security model.**

- No credentials. The API reads inventory documents and embedded catalog snapshots only. Live discovery
  would mean a web server holding customers' cloud credentials (CLAUDE.md rules 1 and 3) and needs its
  own PRD and threat model.
- Stateless. A request's body, inventory and report exist only in memory while it is served; nothing
  is written to disk, and `internal/httpapi` takes no logger. The access log (milestone 3) holds the
  method, path, status, duration and size only.
- Limits. The body is read through `http.MaxBytesReader` up to `MaxBodySize` (413 over it) and decoded
  strictly (400 for an unknown key, a duplicate key or trailing data). At most N assessments run at once
  (default the number of CPUs, at least 1) through a non-blocking semaphore; one over the cap gets
  **503 `busy`** with `Retry-After: 1` at once, never queued. 503 rather than 429: the limit is the
  server's capacity, not the client's rate.
- Optional bearer token. When configured, every `/v1/` request without `Authorization: Bearer <token>`
  gets 401 with `WWW-Authenticate: Bearer`, compared in constant time and checked before the body is
  read. `/healthz` and `/readyz` stay open for the platform's probes.
- CORS is off unless an allow-list of origins is configured; then only an origin in the list gets
  `Access-Control-Allow-Origin` with that origin (never `*`, never credentials), `Vary: Origin`, and a
  204 preflight on `/v1/` routes.
- Every response sets `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`.
- TLS, users, accounts and per-user rate limits belong to the reverse proxy or the website's backend,
  which the README will describe (milestone 4).

**What milestone 3 adds, in `cmd/eusov-server`.** Flags and environment (listen address, token from an
environment variable or a file, never a flag; concurrency; CORS origins), the server's read-header,
read, write and idle timeouts and maximum header size, graceful shutdown on `SIGTERM` and `SIGINT`, the
access log, the readiness check over every target's catalog, and the refusal to listen on a non-loopback
address without a token unless `--insecure-no-auth` is given. `internal/httpapi` stays a handler with no
listener of its own.

## Alternatives considered

- **`eusov serve`, a subcommand of the CLI.** One binary and one image, but `net/http` in the CLI's
  dependencies ends the offline guarantee its test pins. A second binary from the same module costs
  only a build target and an image.
- **Decode the request in `internal/httpapi` with `encoding/json`.** No new adapter, but a second set of
  JSON rules beside `jsondoc`: case-insensitive keys, the last of a repeated key kept, trailing data
  accepted by a `Decoder`. ADR 0009 exists so there is one.
- **Move `jsondoc` out of `internal/adapter/internal/`** so `internal/httpapi` could import it. That
  would let a driving adapter, and anything else, decode adapter documents, and widen a helper that
  adapter-helpers-do-no-io and the strict rules hold to the adapters on purpose. Passing function values
  from the composition root is how `internal/cli` already gets its adapters.
- **A multipart upload, or the inventory as the whole body with targets in the query.** A file upload
  is what a browser form sends, but it needs a multipart parser and its limits; targets in the query
  string end up in access logs and proxy logs. One JSON object is simplest to decode strictly and to
  describe in OpenAPI.
- **Version by header or media type** (`Accept: application/vnd.eusov.v1+json`). Harder to route at a
  proxy, to see in a log and to try with `curl`; the PRD asks for path versioning.
- **429 when busy.** 429 says the client sent too many requests; the cap is shared by all clients and
  is about the server's capacity, so 503 with `Retry-After` is the honest status. The PRD allows either.
- **Fall back to JSON on an unacceptable `Accept`.** Lenient, but a client that asked only for, say,
  `text/html` would get a body it said it cannot read, and an error would hide nothing.
- **`api/openapi.yaml`.** The usual format, but a test would need a YAML parser in the module. JSON is
  read with `encoding/json` and is valid OpenAPI.
- **Copy the report schemas into the OpenAPI document.** Self-contained, but two copies of the report's
  contract that can drift; the relative `$ref` keeps one.

## Consequences

- The CLI stays offline, by test and by depguard; the API is a second composition root with its own
  binary and image (milestone 4).
- The API and the CLI share every rule that matters: the strict decoder, the inventory reader, the
  target-list rules, the use cases and the reporters, and a parity test pins their bytes.
- `inventoryfile` gains `FromBytes`, and a new adapter family, `internal/adapter/request`, holds request
  decoders; each new decoder falls under `request-decoders-parse-bytes`.
- Two depguard rules hold `internal/httpapi` (`httpapi-gets-adapters-from-cmd`,
  `httpapi-imported-only-by-the-server`), and one the request decoders; all three are in the depguard pin
  (`test/architecture/depguardpin_test.go`). A standard package `internal/httpapi` needs is added to its
  allow list and the pin on purpose. `cmd/eusov-server` will need `!**/cmd/eusov-server/**` in
  `adapters-are-built-in-cmd` to construct adapters (milestone 3).
- A change to a route, a status code or an error code updates `api/openapi.json` in the same change,
  and a breaking one is a `/v2/` route and an ADR.
- Operators must put the server behind a proxy for TLS and users; a token alone protects it from direct
  access, not from a client that holds the token.

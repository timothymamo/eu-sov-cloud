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
never imports `internal/httpapi` (depguard rule `httpapi-imported-only-by-the-server`, which exempts only
`cmd/eusov-server`, the package itself and the parity test in `test/apiparity`, test files elsewhere
included), so the CLI's offline test stays unchanged and passes. Both binaries are built from the same module and reuse every
package.

**Three pieces, so strict decoding stays in the adapters.**

1. `inventoryfile.FromBytes(data)` returns the same `*Source` as `New(path)`, over an in-memory
   inventory document: the same `MaxFileSize` cap, the same strict decoding and sentinels, and the
   `sourcetest` contract suite. It takes ownership of `data` (the caller must not change it afterwards)
   rather than copying it, so a request holds one copy of its inventory's bytes. Its errors are in an
   in-memory-only mode that never quotes a value from the document: they name what is wrong by its
   position and field (`resources[3].category`) and why (missing, the wrong JSON type, unknown,
   repeated, not in the taxonomy), never the value. `New(path)`, the CLI's reader, keeps its messages
   exactly as they are, values included: the CLI's user wrote the file and is the only one who reads
   them.
2. A new adapter, `internal/adapter/request/assessmentrequest`, decodes the body of
   `POST /v1/assessments` through `jsondoc`: one object with exactly the keys `inventory` (an object,
   returned as its raw bytes) and `targets` (a non-empty array of strings, in order). Anything else wraps
   `assessmentrequest.ErrDecode`. Its messages never quote a value from the body, by construction rather
   than because of `jsondoc`: `jsondoc`'s own errors place a problem by line and column and name a key,
   but its `Describe`, which a format may use for its own errors, does quote a scalar (`the string "1"`,
   `the number "1.0"`), so `assessmentrequest` never calls it (nor `Document.Version`, which does) and
   names a wrong value by its JSON type only (`an array`, `a string`).
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
byte-identical to the CLI's, as `text/markdown; charset=utf-8`. A `q=0` range is excluded, and an entry
whose `q` is not a number in [0, 1] (`NaN` included) is invalid and skipped, as if it were absent. When nothing
the client accepts is available the answer is **406 `not_acceptable`**, a status the PRD's list did not
name: answering JSON to a client that refused it would hide the mismatch.

**One error shape.** Every error, 404 and 405 included, is
`{"error": {"code": "<code>", "message": "<message>"}}` as `application/json`, with the codes
`invalid_request`, `invalid_inventory`, `invalid_targets` (400), `unauthorized` (401), `not_found` (404),
`method_not_allowed` (405), `not_acceptable` (406), `payload_too_large` (413),
`misdirected_request` (421), `unsupported_media_type` (415), `internal` (500), `busy` and `not_ready`
(503). **No value from a request is ever echoed**: a message names what is wrong by position and field
(`targets[2]`, `resources[3].category`) or by a quoted, escaped key name, never a target value, an
inventory value, a header value, the `Host` or the path the client sent. A website may show an error
message to a user, or log it, so a message must be safe to show to anyone and to store; the client
already holds the value it sent. A 500 says `internal error` and nothing more; a panic is recovered into
one.

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
- Media type. A POST body must be `application/json`; a `charset` parameter other than `utf-8`
  (compared ignoring case) is 415, as the body is decoded as UTF-8 only.
- Limits. At most N requests to `POST /v1/assessments` are served at once (default the number of
  CPUs, at least 1) through a non-blocking semaphore; one over the cap gets **503 `busy`** with
  `Retry-After: 1` at once, never queued. 503 rather than 429: the limit is the server's capacity, not
  the client's rate. **The slot is taken before the body is read**, so at most N bodies are in memory at
  once, whatever the clients send. The trade-off: a slow upload holds a slot while it trickles in, and N
  slow clients can keep everyone else at 503. `WithBodyReadTimeout` (default 30 s) bounds how long
  reading one body may hold its slot; past it the request is 400 `invalid_request` with
  `Connection: close`, the server closes the connection and the slot is freed. The handler sets the
  connection's read deadline for the body through `http.ResponseController` and clears it once the
  body is read, which replaces the server's `ReadTimeout`: for `POST /v1/assessments` the body's bound
  is `WithBodyReadTimeout`, measured from when the slot is taken, not `ReadTimeout`. Taking the slot
  after the body would free slots from slow clients but put no bound on memory, which the cap exists
  for. The body is read through `http.MaxBytesReader` up to `MaxBodySize` (413 over it) and decoded
  strictly (400 for an unknown key, a duplicate key or trailing data).
- Memory. One request holds its body (up to `MaxBodySize`), the decoded inventory and the rendered
  report, which is built in memory in full before it is written; a comparison of every target over a
  maximal inventory is the largest case. `FromBytes` takes the inventory's bytes without copying them.
  The peak is therefore about N times one request's, and N's default must be sized against it
  (milestone 3).
- Optional bearer token. When configured, every `/v1/` request without `Authorization: Bearer <token>`
  gets 401 with `WWW-Authenticate: Bearer`, checked before the body is read. The token sent and the
  token configured are compared as SHA-256 digests with `subtle.ConstantTimeCompare`, so neither the
  content nor the length of the token leaks through timing. An explicitly empty token, or one that is
  not a valid bearer token (RFC 6750 `token68`), is a configuration error from `New`, never a server
  that accepts an empty `Authorization`. `/healthz` and `/readyz` stay open for the platform's probes.
- Allowed hosts. `WithAllowedHosts` names the `Host` values the server answers to, compared ignoring case; any other `Host`
  gets **421 `misdirected_request`**, before anything else is checked. `/healthz` and `/readyz` are
  exempt, as probes address the server by its pod or container address. Without it every `Host` is
  served. It stops DNS rebinding: a page on an attacker's domain that resolves to `127.0.0.1` reaches a
  server on loopback as same-origin, past CORS, but sends the attacker's `Host`.
- CORS is off unless an allow-list of origins is configured. `WithAllowedOrigins` validates each entry
  and refuses, as a configuration error from `New`, `null`, `*`, an entry with a path, query, fragment
  or user info, and any scheme but `http` and `https`. An `Origin` is compared with the list by scheme
  and host ignoring case, and port exactly. Only an origin in the list gets
  `Access-Control-Allow-Origin` with that origin (never `*`, never credentials),
  `Access-Control-Expose-Headers: Retry-After, WWW-Authenticate`, so a browser client can read the
  back-off and the auth challenge, and a 204 preflight on `/v1/` routes. When CORS is configured, every
  response, to any origin or none, carries `Vary: Origin`, so a shared cache never serves one origin's
  answer to another.
- Every response sets `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`.
- `GET /v1/targets` loads every target's catalog once, on the first request, and keeps the result; a
  load that fails is not kept, and the next request tries again.
- Route enumeration. A request without the token learns whether a path exists (404 against 401 or
  405), because the route check runs before the token. This is accepted: the routes are public, in the
  OpenAPI document.
- The Markdown report contains text from the inventory (names, tags), escaped for Markdown but not for
  HTML. A website that renders it as HTML must use a sanitising renderer (or render the JSON report
  itself); the API does not make Markdown safe to inject as HTML.
- TLS, users, accounts and per-user rate limits belong to the reverse proxy or the website's backend,
  which the README will describe (milestone 4).

**What milestone 3 adds, in `cmd/eusov-server`.** Flags and environment (listen address, token from an
environment variable or a file, never a flag; concurrency; CORS origins; allowed hosts), the server's
read-header, read, write and idle timeouts and maximum header size, graceful shutdown on `SIGTERM` and
`SIGINT`, the access log, the readiness check over every target's catalog, and the refusal to listen on
a non-loopback address without a token unless `--insecure-no-auth` is given. `internal/httpapi` stays a
handler with no listener of its own. Milestone 3 **must** also:

- set `http.Server.ReadHeaderTimeout` and `ReadTimeout`: `WithBodyReadTimeout` bounds a body only once
  the handler runs, and a client that is slow to send its headers holds a connection, not a slot;
- pass a `WithBodyReadTimeout` no greater than its `ReadTimeout`: the body deadline replaces the
  server's for `POST /v1/assessments`, so a greater one would let an assessment body take longer than
  the server allows any other request;
- treat a token source that is configured but empty (an empty variable, an empty file) as a startup
  error, and trim the white space around a token read from a file, which ends with a line feed as
  editors write it;
- set `WithAllowedHosts` (to `localhost` and the loopback addresses with the port) whenever it listens
  on loopback without a token, the case DNS rebinding attacks, and treat an empty resolved host list in
  that case as a startup error, never as "any host";
- size the concurrency default against memory and `GOMEMLIMIT`, not only the number of CPUs, using the
  benchmark `BenchmarkMaximalAllComparison` in `test/apiparity` for one request's peak;
- check end to end that the built `eusov` and `eusov-server` binaries give the same bytes for the same
  inventory and targets, beside the in-process parity test.

**What milestone 4 adds (packaging).** The `eusov-server` image, the README's deployment notes, and a
published OpenAPI document that bundles the report's JSON Schemas: `api/openapi.json` refers to them by
repo-relative path, which a reader outside the repository cannot follow.

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
  contract that can drift; the relative `$ref` keeps one, and milestone 4 bundles them when it publishes
  the document.
- **Echo the offending value, escaped, in `invalid_inventory`** (the CLI's wording). Most helpful to a
  developer, but the message reaches a website, its users and its logs, and an inventory value can be a
  secret someone pasted into a tag. Naming the position and field is enough to find it in the document
  the client still holds.
- **Take the concurrency slot after the body is read.** A slow upload would hold no slot, but N slots
  would no longer bound memory: every connection could hold a body of up to `MaxBodySize` at once. A
  body read timeout bounds the slow-upload cost instead.
- **Compare the token bytes directly with `subtle.ConstantTimeCompare`.** It returns at once when the
  lengths differ, which leaks the token's length; comparing digests of fixed size does not.

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
  allow list and the pin on purpose. A new test that needs the handler goes in `internal/httpapi` or
  `test/apiparity`, or widens `httpapi-imported-only-by-the-server` on purpose. `cmd/eusov-server` will need `!**/cmd/eusov-server/**` in
  `adapters-are-built-in-cmd` to construct adapters (milestone 3).
- A change to a route, a status code or an error code updates `api/openapi.json` in the same change,
  and a breaking one is a `/v2/` route and an ADR.
- Operators must put the server behind a proxy for TLS and users; a token alone protects it from direct
  access, not from a client that holds the token.
- API errors are less precise than the CLI's: they say where in the inventory the problem is, not what
  value is there. A client that wants the value reads it from its own document.
- A slow client can hold a slot for up to the body read timeout; with N slots, N such clients make the
  API answer 503 `busy` for that long. Per-client limits belong to the proxy.

// Package httpapi is the HTTP driving adapter, beside internal/cli: it serves
// the analysis use cases over a small, versioned JSON API (PRD #103, ADR 0019).
// It contains no business rules and constructs no adapter: the composition
// root (cmd/eusov-server) passes everything in through Deps, as cmd/eusov does
// for the CLI through cli.Deps. New returns an http.Handler; listening, flags,
// the environment, timeouts, graceful shutdown and the access log belong to
// the composition root, not to this package.
//
// The package imports the application, the domain, internal/version and a
// named list of standard packages only, never an adapter, internal/cli or
// cmd/ (depguard rule httpapi-gets-adapters-from-cmd). Strict JSON decoding
// lives in the adapters, because jsondoc (ADR 0009) is internal to
// internal/adapter: Deps.DecodeRequest is assessmentrequest.Decode and
// Deps.NewInventorySource is inventoryfile.FromBytes. The OpenAPI 3.1 document
// api/openapi.json describes every route, request, response, header and error,
// and a test holds the handler to it.
//
// # Routes
//
//	POST /v1/assessments   assess one inventory against one target, or compare several
//	GET  /v1/targets       list the targets, their names and snapshot dates, sorted by key
//	GET  /healthz          200 {"status":"ok"} while the process runs
//	GET  /readyz           200 {"status":"ready"}, or 503 not_ready when the readiness check fails
//
// POST /v1/assessments takes Content-Type application/json (parameters such as
// charset=utf-8 allowed) and a body {"inventory": <inventory document>,
// "targets": [<key>, ...]}. One target runs analysis.Analysis; several, or
// ["all"], run analysis.Comparison in sorted key order. The target list
// follows the CLI's rules: an unknown, repeated or empty key, or all with
// another key, is refused. The report names the inventory inventory.json
// (analysis.ReportMeta), because the body carries no file name. The report is
// rendered into memory in full before anything is written, so a failed request
// never sends part of one.
//
// Content negotiation on Accept: none, */*, application/* or application/json
// gives the JSON report (ADR 0018), byte for byte what eusov assess --format
// json prints; text/markdown or text/* gives the Markdown report, byte for
// byte the CLI's, as text/markdown; charset=utf-8. A media range with q=0 is
// excluded. Nothing acceptable is 406 not_acceptable.
//
// An unknown path is 404 not_found; a known path with another method is 405
// method_not_allowed with an Allow header.
//
// # Limits and hardening
//
//   - The body is read through http.MaxBytesReader, capped at the limit
//     WithMaxBodyBytes sets (the composition root passes
//     assessmentrequest.MaxBodySize, the inventory reader's cap plus 64 KiB);
//     over it is 413 payload_too_large.
//   - At most WithMaxConcurrent assessments run at once (default
//     runtime.NumCPU(), at least 1). A request over the cap is refused at once
//     with 503 busy and Retry-After: 1, never queued.
//   - WithBearerToken, when set, requires Authorization: Bearer <token> on
//     every /v1/ request, compared in constant time and checked before the body
//     is read; without it the answer is 401 unauthorized with
//     WWW-Authenticate: Bearer. /healthz and /readyz stay open.
//   - CORS is off unless WithAllowedOrigins names origins. Then only an Origin
//     exactly in the list gets Access-Control-Allow-Origin with that origin and
//     Vary: Origin, and its preflight OPTIONS on a /v1/ route gets 204 with the
//     allowed methods, the headers Authorization, Content-Type and Accept, and
//     a max age of 600 seconds. Never *, never credentials.
//   - Every response, errors included, sets Cache-Control: no-store and
//     X-Content-Type-Options: nosniff; a JSON response sets Content-Type:
//     application/json.
//   - Nothing from a request is logged or written anywhere: the package takes no
//     logger and keeps a request only in memory while it is served.
//   - A panic in a handler is recovered and answered as 500 internal, with no
//     stack in the body.
//
// # Errors
//
// Every error has one shape, {"error":{"code":"<code>","message":"<message>"}}:
//
//	400 invalid_request         the body is not a valid request (assessmentrequest.Decode)
//	400 invalid_inventory       the inventory reader refuses the inventory
//	400 invalid_targets         the target list breaks the CLI's rules; the message lists the targets
//	401 unauthorized            a /v1/ request without the configured bearer token
//	404 not_found               an unknown path
//	405 method_not_allowed      a known path with another method
//	406 not_acceptable          no representation the Accept header allows
//	413 payload_too_large       a body over the cap
//	415 unsupported_media_type  a POST body that is not application/json
//	500 internal                anything else, with the fixed message "internal error"
//	503 busy                    the concurrency cap is reached
//	503 not_ready               the readiness check fails
//
// A message never echoes request content beyond a quoted, escaped key or field
// name, as the CLI's one-line errors do: never a target value, a header value
// or the path the client sent. A 500 carries nothing internal.
package httpapi

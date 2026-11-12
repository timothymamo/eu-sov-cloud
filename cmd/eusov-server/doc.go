// Command eusov-server serves the assessments of eusov over a versioned HTTP
// API (PRD #103, ADR 0019): POST /v1/assessments, GET /v1/targets, GET /healthz
// and GET /readyz.
//
// It is the HTTP API's composition root, the sibling of cmd/eusov: the only
// place that wires the concrete adapters (the request decoder, the in-memory
// inventory reader, the embedded OVHcloud, Scaleway and UpCloud catalogs and the
// JSON and Markdown reporters) into internal/httpapi. Around the handler it
// owns what a running server needs and the handler does not: the flags and the
// environment, the listener, its loopback rule and its connection cap, the
// http.Server timeouts, the bearer token, the Host allow-list, the concurrency
// default, the readiness check, the access log and the graceful shutdown.
//
// Its imports are a strict allow list (depguard rule server-composition-root):
// the module's own packages and the standard packages it needs, never a cloud
// SDK, a third-party package, internal/cli or os/exec. It stores nothing and
// logs no request content: the access log holds the method, the path, the
// status, the duration and the size of each request only.
package main

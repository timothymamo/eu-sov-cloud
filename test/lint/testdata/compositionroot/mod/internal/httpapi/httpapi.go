// Package httpapi stands for the HTTP API's production package, so its _test.go
// file has a package to attach to. Nothing here imports anything:
// httpapi-tests-no-env-or-process holds the tests, which run in the process the
// server's handler and the composition root that hands it the work run in, and
// not the handler.
package httpapi

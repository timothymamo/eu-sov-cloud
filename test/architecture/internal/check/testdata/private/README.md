# dependency fixture

`p/p.go` imports `example.invalid/dep`, which `go.mod` requires and no module
provides. `.invalid` is reserved, so the name never resolves.

`go.sum` holds a made-up checksum for that module, and that is what makes the
fixture prove anything. Its bytes are all it is: no module with that checksum
exists. Without the two lines, `go list -mod=readonly` refuses with "missing
go.sum entry" before it goes anywhere, whatever the environment says. With
them, go list has to reach the module — and then what it does depends on the
variables ListPackages passes it:

- `GOPROXY=off` and no `GOPRIVATE`/`GONOPROXY` for the name:
  `module lookup disabled by GOPROXY=off`, no network.
- `GOPRIVATE` or `GONOPROXY` naming `example.invalid`: the proxy is bypassed
  for that path, so go list dials the name instead, and the error names the
  `https` request and the failed DNS lookup.

The test `TestListPackagesFetchesNothing` fails on the second.

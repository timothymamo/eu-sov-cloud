// Package container holds plain tests that pin the repository's container build
// files, the Dockerfile and .dockerignore at the repository root, to go.mod and
// .goreleaser.yaml, so the image compiles the binary the way the release does:
// the Go version of go.mod, no cgo, -trimpath, the internal/version ldflags, a
// pinned base image for every stage, a minimal non-root final stage whose base
// is not the debug variant and that runs no RUN instruction, and a build context
// that holds no inventory, report or git data and keeps every embedded file.
//
// The tests read the files from the repository root. They need no Docker daemon,
// no network and no credentials, and they carry no build tag, so make test, make
// check and CI's make cover run them. The package has no code of its own.
package container

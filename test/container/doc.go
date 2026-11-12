// Package container holds plain tests that pin the repository's container build
// files, the Dockerfile and .dockerignore at the repository root, to go.mod and
// .goreleaser.yaml, so each image compiles its binary the way the release does:
// the Go version of go.mod, no cgo, -trimpath, the internal/version ldflags, a
// pinned base image for every stage, a minimal non-root stage per image whose
// base is not the debug variant and that runs no RUN instruction, and a build
// context that holds no inventory, report or git data and keeps every embedded
// file.
//
// There are two images. The last stage is the CLI, shift2eu, which docker build
// . and make image build. The stage named server is the HTTP API,
// shift2eu-server (PRD #103, ADR 0019), which --target server and make
// server-image build. .goreleaser.yaml has a builds entry for each, and the
// tests hold the server's entry to the CLI's, and each image's go build to its
// entry.
//
// The tests read the files from the repository root. They need no Docker daemon,
// no network and no credentials, and they carry no build tag, so make test, make
// check and CI's make cover run them. The package has no code of its own.
package container

# eusov as a minimal container image (docs/adr/0013-ship-eusov-as-a-distroless-container-image.md).
#
#   make image                                   # version, commit and date from git
#   docker buildx build --platform linux/amd64,linux/arm64 .
#
# Base images are pinned by the digest of their multi-arch index, with the tag kept in the reference so
# Renovate's dockerfile manager updates both. test/container pins the build settings to .goreleaser.yaml.

# The builder runs on the build host's platform and cross-compiles for the target one: the binary is
# pure Go, so no emulation is needed. Its tag is the Go version in go.mod.
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS build

WORKDIR /src

# Modules first, in their own step, so a source change does not download them again. The module and
# build caches are cache mounts: they persist between builds but are never part of a layer.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY cmd/ cmd/
COPY internal/ internal/

# Set by docker buildx from --platform.
ARG TARGETOS
ARG TARGETARCH
# The defaults match internal/version; make image passes the values make build uses.
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

# The same settings as the builds entry of .goreleaser.yaml.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w -X github.com/timothymamo/eu-sov-cloud/internal/version.version=${VERSION} -X github.com/timothymamo/eu-sov-cloud/internal/version.commit=${COMMIT} -X github.com/timothymamo/eu-sov-cloud/internal/version.date=${DATE}" \
    -o /out/eusov ./cmd/eusov

# distroless static: CA certificates (for the live sources' TLS calls), tzdata, /etc/passwd with the
# nonroot user (65532), and no shell, package manager or Go toolchain.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

ARG VERSION=dev
ARG COMMIT=none

LABEL org.opencontainers.image.title="eusov" \
      org.opencontainers.image.description="Assess what of a hyperscaler's infrastructure can move to an EU cloud provider" \
      org.opencontainers.image.source="https://github.com/timothymamo/eu-sov-cloud" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /out/eusov /eusov

# Mount the inventory here, as the README's "Run with Docker" section does (one command):
#   docker run --rm --read-only --network none --user "$(id -u):$(id -g)" --mount type=bind,src="$PWD/inventory.json",dst=/work/inventory.json,readonly eusov assess --inventory inventory.json --target scaleway
WORKDIR /work
USER nonroot:nonroot
ENTRYPOINT ["/eusov"]

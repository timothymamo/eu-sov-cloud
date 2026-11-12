BINARY  := shift2eu
# The HTTP API's binary (PRD #103, docs/adr/0019-serve-assessments-over-a-versioned-http-api-from-a-separate-binary.md).
SERVER  := shift2eu-server
PKG     := github.com/timothymamo/shift2eu
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(PKG)/internal/version.version=$(VERSION) \
	-X $(PKG)/internal/version.commit=$(COMMIT) \
	-X $(PKG)/internal/version.date=$(DATE)
# The container image make image builds (docs/adr/0013-ship-shift2eu-as-a-distroless-container-image.md).
IMAGE   ?= shift2eu
# The container CLI make image runs: docker where it is installed, otherwise nerdctl, the rootless runtime
# of the Lima VM (docs/adr/0017-run-a-rootless-container-runtime-in-the-lima-vm.md).
DOCKER  ?= $(if $(shell command -v docker 2>/dev/null),docker,nerdctl)
# The platforms make image builds, comma separated (linux/amd64,linux/arm64). Empty builds the host's own.
PLATFORMS ?=

# The GOOS/GOARCH pairs the release builds, all with CGO_ENABLED=0. Must match the goos and goarch lists in
# .goreleaser.yaml: test/architecture checks this by running make lint with a fake golangci-lint.
RELEASE_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

LIMA_INSTANCE ?= shift2eu
# The directory the VM mounts: this repository's parent, which must hold nothing but this repository and
# the worktrees dot-agent-deck's dispatcher creates beside it
# (docs/adr/0008-mount-a-workspace-directory-for-dispatcher-mode.md). Found from the main checkout, so it
# is the same when make runs in a worktree.
REPO_ROOT := $(patsubst %/.git,%,$(shell git rev-parse --path-format=absolute --git-common-dir 2>/dev/null || echo $(CURDIR)/.git))
WORKSPACE := $(abspath $(REPO_ROOT)/..)
REPO_NAME := $(notdir $(REPO_ROOT))
# Credentials vm-env copies from your shell (e.g. direnv) into the VM, by name. Never add
# ANTHROPIC_API_KEY: Claude Code prefers it over the subscription token and bills per call.
VM_ENV ?= CLAUDE_CODE_OAUTH_TOKEN,DEEPSEEK_API_KEY,GITHUB_TOKEN

.DEFAULT_GOAL := help

##@ Development

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^##@/ {printf "\n%s\n", substr($$0, 5)} /^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: server ## Build the shift2eu and shift2eu-server binaries into ./bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

.PHONY: server
server: ## Build the shift2eu-server binary (the HTTP API) into ./bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(SERVER) ./cmd/$(SERVER)

.PHONY: image
image: ## Build the local container image shift2eu:<version>, shift2eu:dev and shift2eu:latest from git, as make build (docker or nerdctl; PLATFORMS=linux/amd64,linux/arm64 for both; tags are local, nothing is pushed)
	$(DOCKER) build \
		$(if $(PLATFORMS),--platform "$(PLATFORMS)") \
		--build-arg VERSION="$(VERSION)" \
		--build-arg COMMIT="$(COMMIT)" \
		--build-arg DATE="$(DATE)" \
		--tag "$(IMAGE):$(VERSION)" \
		--tag "$(IMAGE):dev" \
		--tag "$(IMAGE):latest" \
		.

.PHONY: fmt
fmt: ## Format code (gofumpt + goimports via golangci-lint)
	golangci-lint fmt

# make lint runs golangci-lint natively first, for the host's GOOS/GOARCH with cgo at the host's default (the
# build make test's -race compiles; it warns when the host's cgo is off, and make test, so make check,
# refuses such a host), then once per release target with cgo off, and last runs the source rules in
# test/lint natively, with SHIFT2EU_LINT_PROBES=require so the forbidigo probes must run. It attempts every
# run and fails at the end if any of them failed. Each golangci-lint run waits for
# golangci-lint's lock (--allow-serial-runners) instead of failing while another worktree lints, and sees
# the same build settings: GOFLAGS=-mod=readonly (a non-empty GOFLAGS also overrides the go env file's, so
# a -tags of yours cannot take files out of lint), GOAMD64=v1 and GOARM64=v8.0 (the levels the release
# builds at), GOWORK=off (a go.work in a parent directory would switch the run into workspace mode), and
# no GOEXPERIMENT. A `go env -w GOEXPERIMENT` value still applies, as an empty variable
# cannot clear it; CI has none.
# Each run gets a cache directory of its own, as a cache entry of one target can hide another target's
# issues: native-<goos>-<goarch>-cgo<0|1> for the native run, named after what it lints so one cache root
# can serve two hosts (e.g. a macOS host and the Lima VM), and <goos>-<goarch> for a release target. The
# root is $XDG_CACHE_HOME/shift2eu/golangci-lint, else ~/.cache/shift2eu/golangci-lint, outside the repository
# so worktrees share it; GOLANGCI_LINT_CACHE overrides it. To clear the caches, rm -rf the root.
.PHONY: lint
lint: ## Lint natively and for every release target, incl. the architecture (depguard) rules and test/lint
	@cache="$${GOLANGCI_LINT_CACHE:-$${XDG_CACHE_HOME:-$${HOME:?set HOME or GOLANGCI_LINT_CACHE}/.cache}/shift2eu/golangci-lint}"; \
	export GOFLAGS=-mod=readonly GOAMD64=v1 GOARM64=v8.0 GOWORK=off; \
	unset GOEXPERIMENT; \
	failed=""; \
	( unset GOOS GOARCH CGO_ENABLED; \
		GOOS="$$(go env GOHOSTOS)" && GOARCH="$$(go env GOHOSTARCH)" && export GOOS GOARCH && \
		cgo="$$(go env CGO_ENABLED)" && \
		echo "lint: native GOOS=$$GOOS GOARCH=$$GOARCH CGO_ENABLED=$$cgo" && \
		{ [ "$$cgo" != 0 ] || echo "lint: warning: cgo is off on this host, so its cgo build is not linted" >&2; } && \
		GOLANGCI_LINT_CACHE="$$cache/native-$$GOOS-$$GOARCH-cgo$$cgo" golangci-lint run --allow-serial-runners \
	) || failed="$$failed native"; \
	for target in $(RELEASE_TARGETS); do \
		goos="$${target%/*}"; goarch="$${target#*/}"; \
		echo "lint: GOOS=$$goos GOARCH=$$goarch CGO_ENABLED=0"; \
		GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 GOLANGCI_LINT_CACHE="$$cache/$$goos-$$goarch" \
			golangci-lint run --allow-serial-runners || failed="$$failed $$target"; \
	done; \
	( unset GOOS GOARCH CGO_ENABLED; SHIFT2EU_LINT_PROBES=require go test -count=1 ./test/lint/ ) || failed="$$failed test/lint"; \
	if [ -n "$$failed" ]; then echo "lint: failed for:$$failed" >&2; exit 1; fi

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	go mod tidy

##@ Testing

.PHONY: test
test: ## Unit + contract tests with the race detector (no network, no credentials)
	go test -race -count=1 ./...

.PHONY: test-integration
test-integration: ## Integration tests against real cloud APIs (read-only credentials required)
	go test -race -count=1 -tags=integration ./...

.PHONY: test-e2e
test-e2e: ## Build and run the binary end to end against testdata fixtures
	go test -count=1 -tags=e2e ./test/e2e/...

.PHONY: cover
cover: ## Unit tests with a coverage report in coverage.out
	go test -race -count=1 -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -n 1

.PHONY: vuln
vuln: ## Scan code and dependencies for known vulnerabilities
	govulncheck ./...

.PHONY: check
check: lint test vuln ## Everything CI gates on; run before every commit

##@ Workflow

.PHONY: worktree
worktree: ## Create or resume the worktree for a PR: make worktree BRANCH=feat/12-short-name
	@./dev/worktree.sh "$(BRANCH)"

.PHONY: git-hooks
git-hooks: ## Enable .githooks, which refuse commits and pushes to main
	git config core.hooksPath .githooks

##@ Isolated environment (Lima)

.PHONY: vm-up
vm-up: git-hooks ## Create or start the Lima VM with only this repository's workspace directory mounted
	@stray="$$(ls -A "$(WORKSPACE)" | grep -vx -e '$(REPO_NAME)' -e '$(REPO_NAME)-dispatch-.*' -e '\.DS_Store' || true)"; \
	if [ -n "$$stray" ]; then \
		echo "vm-up: the VM mounts $(WORKSPACE), which must hold only $(REPO_NAME) and its dispatch worktrees; move these out first:" >&2; \
		echo "$$stray" | sed 's/^/  /' >&2; \
		exit 1; \
	fi
	@if limactl list --quiet | grep -qx "$(LIMA_INSTANCE)"; then \
		limactl start "$(LIMA_INSTANCE)"; \
	else \
		limactl start --name="$(LIMA_INSTANCE)" --mount-only "$(WORKSPACE):w" --tty=false dev/lima.yaml; \
	fi

.PHONY: vm-shell
vm-shell: ## Open a shell in the VM at the repository root
	limactl shell --workdir "$(CURDIR)" "$(LIMA_INSTANCE)"

.PHONY: vm-env
vm-env: ## Copy the VM_ENV credentials from your shell into the VM
	@env_file="$$(for name in $$(echo '$(VM_ENV)' | tr ',' ' '); do \
		value="$$(printenv "$$name")"; \
		[ -n "$$value" ] || { echo "vm-env: $$name is not set, skipped" >&2; continue; }; \
		case "$$value" in (*[!A-Za-z0-9._~+/=-]*) echo "vm-env: $$name has unsupported characters" >&2; exit 1;; esac; \
		printf '%s=%s\n' "$$name" "$$value"; \
	done)" && printf '%s\n' "$$env_file" | limactl shell --workdir / "$(LIMA_INSTANCE)" \
		sh -c 'umask 077 && mkdir -p ~/.config/dot-agent-deck && cat > ~/.config/dot-agent-deck/daemon.env && chmod 600 ~/.config/dot-agent-deck/daemon.env' >/dev/null
	@echo "vm-env: written to ~/.config/dot-agent-deck/daemon.env in the VM; 'make deck-restart' applies it to a running deck"

.PHONY: vm-claude-settings
vm-claude-settings: ## Merge dev/claude-user-settings.json into Claude Code's user settings in the VM
	limactl shell --workdir "$(CURDIR)" "$(LIMA_INSTANCE)" dev/claude-settings.sh "$(WORKSPACE)"

.PHONY: deck
deck: ## Open the dot-agent-deck TUI in the VM (attaches to the daemon service)
	limactl shell --workdir "$(CURDIR)" "$(LIMA_INSTANCE)" dot-agent-deck

.PHONY: deck-restart
deck-restart: ## Restart the deck's daemon service in the VM, e.g. after vm-env (stops running agents)
	limactl shell --workdir / "$(LIMA_INSTANCE)" systemctl --user restart dot-agent-deck.service

.PHONY: vm-stop
vm-stop: ## Stop the VM
	limactl stop "$(LIMA_INSTANCE)"

.PHONY: vm-delete
vm-delete: ## Delete the VM and everything inside it
	limactl delete --force "$(LIMA_INSTANCE)"

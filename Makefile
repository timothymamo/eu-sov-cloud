BINARY  := eusov
PKG     := github.com/timothymamo/eu-sov-cloud
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(PKG)/internal/version.version=$(VERSION) \
	-X $(PKG)/internal/version.commit=$(COMMIT) \
	-X $(PKG)/internal/version.date=$(DATE)

LIMA_INSTANCE ?= eu-sov-cloud
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
build: ## Build the eusov binary into ./bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

.PHONY: fmt
fmt: ## Format code (gofumpt + goimports via golangci-lint)
	golangci-lint fmt

.PHONY: lint
lint: ## Lint, including the architecture (depguard) rules
	golangci-lint run

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

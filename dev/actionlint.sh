#!/usr/bin/env bash
# Runs actionlint on .github/workflows, for make lint and CI (issue #164,
# docs/adr/0022-lint-shell-scripts-and-workflows-with-shellcheck-and-actionlint.md).
#
# actionlint is the version tools/go.mod pins, a module of its own so its requirements never join the
# shipped module's (ADR 0022), built by the go command for the host. It hands every run: script to the
# shellcheck binary, and runs it with --norc (rule_shellcheck.go in actionlint v1.7.12), so no .shellcheckrc
# changes what it reports. Given a shellcheck it cannot run, actionlint disables that check and still
# passes, so this script resolves shellcheck first, fails loudly without it, and hands actionlint the
# path it found: the pinned shellcheck make lint and CI put on the PATH (dev/lima.yaml, ci.yml).
#
# Usage: dev/actionlint.sh   (from the repository root, where make runs it)
set -euo pipefail

if ! shellcheck_path="$(command -v shellcheck)" || [ ! -x "${shellcheck_path}" ]; then
  echo >&2 "actionlint: shellcheck is not installed: re-provision the VM or see docs/contributing/development.md"
  exit 1
fi

exec go tool -modfile=tools/go.mod actionlint -shellcheck="${shellcheck_path}"

#!/usr/bin/env bash
# Runs actionlint on .github/workflows, for make lint and CI (issue #164,
# docs/adr/0022-lint-shell-scripts-and-workflows-with-shellcheck-and-actionlint.md).
#
# actionlint is the version tools/go.mod pins, a module of its own so its requirements never join the
# shipped module's (ADR 0022), built by the go command for the host. It hands every run: script to
# the shellcheck binary, and runs it with --norc (rule_shellcheck.go in actionlint v1.7.12), so no
# .shellcheckrc changes what it reports; the shellcheck it starts inherits this environment, and --norc
# does not stop SHELLCHECK_OPTS, whose words shellcheck puts before its own arguments, so the script
# unsets it. Given a shellcheck it cannot run, actionlint disables that check and still passes, so this
# script resolves the shellcheck first, fails loudly without it, and hands actionlint the path it
# found: the pinned one make lint and CI put on the PATH (dev/lima.yaml, ci.yml).
#
# actionlint reads .github/actionlint.yaml or .github/actionlint.yml whenever one exists, tracked or not,
# and such a file can ignore whole rules, so an untracked one would make a native run report less than
# CI's. test/lint refuses a tracked one; this script refuses one that merely exists. Passing actionlint a
# -config-file of our own instead would not do: v1.7.12 still parses the repository's file when it finds
# the project (project.go), and a refusal says why the run stopped where an override would hide the file.
#
# Usage: dev/actionlint.sh   (from the repository root, where make runs it)
set -euo pipefail

unset SHELLCHECK_OPTS

for config in .github/actionlint.yaml .github/actionlint.yml; do
  if [ -e "${config}" ] || [ -L "${config}" ]; then
    echo >&2 "actionlint: ${config} exists: it can silence actionlint's checks, so make lint does not run with it; remove it"
    exit 1
  fi
done

if ! shellcheck_path="$(command -v shellcheck)" || [ ! -x "${shellcheck_path}" ]; then
  echo >&2 "actionlint: shellcheck is not installed: re-provision the VM or see docs/contributing/development.md"
  exit 1
fi

exec go tool -modfile=tools/go.mod actionlint -shellcheck="${shellcheck_path}"

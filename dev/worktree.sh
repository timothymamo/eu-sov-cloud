#!/usr/bin/env bash
# Creates the git worktree for a branch, or finds it when it already exists, and prints its absolute
# path. Every pull request is built on its own branch in its own worktree under .worktrees/, never on
# main (docs/adr/0004-one-worktree-per-pull-request.md).
#
# The new worktree goes under the .worktrees/ of the checkout this runs in: the main checkout, or a
# dispatch worktree that dot-agent-deck created beside it, whose agents can then reach it without leaving
# their working directory (docs/adr/0009-create-pull-request-worktrees-inside-the-dispatch-worktree.md).
# Run from inside a pull request's worktree, it uses the checkout that holds that worktree, so pull
# request worktrees are never nested in each other.
#
# Usage: dev/worktree.sh <type>/<issue>-<slug>   (or: make worktree BRANCH=<type>/<issue>-<slug>)
set -euo pipefail

branch="${1:-}"
types='feat|fix|test|docs|refactor|perf|build|ci|chore|prd'
if [[ ! "${branch}" =~ ^(${types})/[0-9]+-[a-z0-9]+(-[a-z0-9]+)*$ ]]; then
  echo "usage: make worktree BRANCH=<type>/<issue>-<slug>, for example feat/12-aws-ec2-discovery" >&2
  echo "<type> is one of: ${types//|/, }" >&2
  exit 2
fi

# Resume: a branch that is already checked out keeps its worktree, wherever that is.
existing="$(git worktree list --porcelain | awk -v want="branch refs/heads/${branch}" '
  /^worktree / { path = substr($0, 10) }
  $0 == want   { print path; exit }
')"
if [[ -n "${existing}" ]]; then
  if [[ ! -d "${existing}" ]]; then
    echo "worktree: '${branch}' is registered at ${existing}, which no longer exists;" \
      "run 'git worktree prune' and try again" >&2
    exit 1
  fi
  echo "${existing}"
  exit 0
fi

# The checkout this runs in. A linked worktree directly under a .worktrees/ is a pull request's worktree,
# and stands for the checkout that holds it.
top="$(git rev-parse --show-toplevel)"
if [[ "$(git rev-parse --path-format=absolute --git-dir)" != "$(git rev-parse --path-format=absolute --git-common-dir)" &&
  "$(basename "$(dirname "${top}")")" == ".worktrees" ]]; then
  top="$(dirname "$(dirname "${top}")")"
fi
dir="${top}/.worktrees/${branch//\//-}"

if [[ -e "${dir}" ]]; then
  echo "worktree: ${dir} exists but is not the worktree of '${branch}'; move it out of the way first" >&2
  exit 1
fi

base=main
if git remote get-url origin >/dev/null 2>&1 && git fetch --quiet --prune origin; then
  base=origin/main
fi

if git show-ref --verify --quiet "refs/heads/${branch}"; then
  # Resume a local branch.
  git worktree add --quiet "${dir}" "${branch}"
elif git show-ref --verify --quiet "refs/remotes/origin/${branch}"; then
  # Resume a branch that was pushed from elsewhere.
  git worktree add --quiet --track -b "${branch}" "${dir}" "origin/${branch}"
else
  # New work, cut from the latest main. --no-track: the branch must never push to main.
  git worktree add --quiet --no-track -b "${branch}" "${dir}" "${base}"
fi
echo "${dir}"

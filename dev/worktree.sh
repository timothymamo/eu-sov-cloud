#!/usr/bin/env bash
# Creates the git worktree for a branch, or finds it when it already exists, and prints its absolute
# path. Every pull request is built on its own branch in its own worktree under .worktrees/, never on
# main (docs/adr/0004-one-worktree-per-pull-request.md).
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

# The main checkout, also when this runs inside another worktree.
root="$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")"
dir="${root}/.worktrees/${branch//\//-}"

if [[ -e "${dir}" ]]; then
  current="$(git -C "${dir}" branch --show-current)"
  if [[ "${current}" != "${branch}" ]]; then
    echo "worktree: ${dir} exists but is on '${current}', not '${branch}'" >&2
    exit 1
  fi
  echo "${dir}"
  exit 0
fi

base=main
if git -C "${root}" remote get-url origin >/dev/null 2>&1 && git -C "${root}" fetch --quiet --prune origin; then
  base=origin/main
fi

if git -C "${root}" show-ref --verify --quiet "refs/heads/${branch}"; then
  # Resume a local branch.
  git -C "${root}" worktree add --quiet "${dir}" "${branch}"
elif git -C "${root}" show-ref --verify --quiet "refs/remotes/origin/${branch}"; then
  # Resume a branch that was pushed from elsewhere.
  git -C "${root}" worktree add --quiet --track -b "${branch}" "${dir}" "origin/${branch}"
else
  # New work, cut from the latest main. --no-track: the branch must never push to main.
  git -C "${root}" worktree add --quiet --no-track -b "${branch}" "${dir}" "${base}"
fi
echo "${dir}"

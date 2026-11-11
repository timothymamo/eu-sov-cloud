#!/usr/bin/env bash
# Creates the git worktree for a branch, or finds it when it already exists, and prints its absolute
# path. Every pull request is built on its own branch in its own worktree under .worktrees/, never on
# main (docs/adr/0004-one-worktree-per-pull-request.md).
#
# The new worktree goes under the .worktrees/ of the checkout this runs in: the main checkout, or a
# dispatch worktree that dot-agent-deck created beside it, whose agents can then reach it without leaving
# their working directory (docs/adr/0010-create-pull-request-worktrees-inside-the-dispatch-worktree.md).
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

# Every worktree of the repository, read whole before it is searched: git gets SIGPIPE when a reader
# stops early, and with pipefail that would end this script without a message. One entry per worktree,
# the main worktree first: a "worktree <path>" line, then lines such as "HEAD <sha>", "branch <ref>" or
# "detached", and "locked [<reason>]", then a blank line.
worktrees="$(git worktree list --porcelain)"

# entry <line>: prints the entry that has exactly this line, or nothing. The main worktree's entry gets
# an extra first line, "main".
entry() {
  awk -v want="$1" '
    BEGIN { RS = ""; FS = "\n" }
    !found {
      for (i = 1; i <= NF; i++) if ($i == want) found = 1
      if (found) { if (NR == 1) print "main"; print }
    }
  ' <<<"${worktrees}"
}

# Resume: a branch that is already checked out keeps its worktree, wherever that is.
found="$(entry "branch refs/heads/${branch}")"
if [[ -n "${found}" ]]; then
  existing="$(sed -n 's/^worktree //p' <<<"${found}")"
  if [[ "${found%%$'\n'*}" == main ]]; then
    echo "worktree: '${branch}' is checked out in the main checkout, ${existing}, which stays on main" \
      "and is never a pull request's worktree (CLAUDE.md rule 4); commit any changes there, run" \
      "'git -C ${existing} switch main', and try again" >&2
    exit 1
  fi
  if [[ ! -d "${existing}" ]]; then
    if lock="$(grep -m 1 '^locked' <<<"${found}")"; then
      reason="${lock#locked}"
      echo "worktree: '${branch}' is registered at ${existing}, which no longer exists, and the entry is" \
        "locked${reason:+ (${reason# })}, so 'git worktree prune' keeps it; unless ${existing} is on a disk" \
        "that is not mounted, run 'git worktree unlock ${existing}', then 'git worktree prune', and try" \
        "again" >&2
    else
      echo "worktree: '${branch}' is registered at ${existing}, which no longer exists;" \
        "run 'git worktree prune' and try again" >&2
    fi
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
  held="$(entry "worktree ${dir}")"
  on="$(sed -n 's#^branch refs/heads/##p' <<<"${held}")"
  if [[ -z "${held}" ]]; then
    echo "worktree: ${dir} exists but is not a worktree; look at what it holds, move it out of the way," \
      "and try again" >&2
  elif [[ -n "${on}" ]]; then
    echo "worktree: ${dir} is the worktree of '${on}', not of '${branch}'; it may hold that branch's" \
      "work, so leave it where it is and report it" >&2
  else
    echo "worktree: ${dir} is a worktree on a detached HEAD, not on '${branch}': a rebase or bisect may be" \
      "in progress there. Leave it where it is; 'git -C ${dir} status' says what is in progress." \
      "Finish that, and try again" >&2
  fi
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

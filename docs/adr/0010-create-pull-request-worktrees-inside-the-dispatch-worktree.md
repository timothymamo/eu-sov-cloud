# 0010. Create a dispatched unit's pull request worktrees inside its dispatch worktree

- Status: Accepted
- Date: 2026-11-10

## Context

ADR 0004 builds every pull request in its own worktree under `.worktrees/`, and `make worktree`
(`dev/worktree.sh`) created it in the main checkout, also when run from another worktree. ADR 0008 runs
dot-agent-deck's dispatcher, which starts each unit's agents in a dispatch worktree beside the repository
(`../<repo>-dispatch-<name>`), and that worktree is their working directory.

A pull request's worktree in the main checkout's `.worktrees/` is outside that working directory, so
Claude Code asks "Allow this read outside the working directories?" before every read or edit there. In
an unattended unit nobody answers, and the worker stalls: the coder of PR #30 did, until the orchestrator
moved the worktree with `git worktree move` (#31).

## Decision

`make worktree` creates a pull request's worktree under the `.worktrees/` of the checkout it runs in: the
main checkout as before, or the dispatch worktree of a dispatched unit.

- Run from inside a pull request's worktree (a linked worktree directly under a `.worktrees/`), it uses
  the checkout that holds that worktree, so pull request worktrees are never nested in each other.
- A branch that already has a worktree gets that worktree's path, wherever it is (`git worktree list`),
  so resuming works from the main checkout and from any dispatch worktree. The one exception is the
  main checkout itself: a branch it is on is refused, never printed, because the main checkout stays on
  `main` and is never a pull request's worktree (CLAUDE.md rule 4).
- `.worktrees/` is git-ignored, so the dispatch worktree's `git status` stays clean, and the VM mounts
  the workspace (ADR 0008), which holds the dispatch worktree and everything in it.
- A pull request's worktree is removed before the unit whose dispatch worktree holds it is closed.

## Alternatives considered

- **Rename the dispatch branch to `<type>/<issue>-<slug>` and skip `make worktree` in a dispatched
  unit.** One orchestration can run several milestones that each need their own branch, and the deck
  tracks units by their `agent/dispatch-<name>` branch.
- **Allow the main checkout's `.worktrees/` in Claude Code's settings.** The settings would have to name
  the main checkout's path, which differs per machine, and only the checked-in settings reach a fresh
  dispatch worktree.
- **Move the worktree after creating it (`git worktree move`)**, as PR #30's orchestrator did. It works,
  but every orchestrator has to know to do it, and resuming the branch afterwards fails, because
  `make worktree` looked for it only in the main checkout.

## Consequences

- A dispatched unit's agents read and edit the pull request's worktree without leaving their working
  directory, so they do not stop at a permission prompt.
- Pull request worktrees live in more than one `.worktrees/`: the main checkout's and each dispatch
  worktree's. `git worktree list` shows them all.
- Closing a unit deletes the pull request worktrees inside it. `dot-agent-deck close` (0.49.7), and
  closing the unit's card or tab in the TUI or the desktop app, which run the same removal, keep a
  dispatch worktree only when `git status --porcelain` in it reports changes, which the git-ignored
  `.worktrees/` never does, and otherwise run `git worktree remove` on it. Git deletes the whole
  directory, nested worktrees and their uncommitted changes included, locked worktrees too, and leaves
  their entries behind, `prunable`, or `locked` for a locked one. Their branches and commits survive.
  So each pull request's worktree is removed first (`git -C <main checkout> worktree remove <path>`, the
  pr-create skill's merge step), and the unit is closed after. Only documentation guards that order.
- When a unit was closed first anyway, `git worktree remove <path>` still clears the missing worktree's
  entry, unless the entry is locked. Until it or `git worktree prune` has, `make worktree` refuses to
  resume the branch and says so. Neither clears a locked entry, such as one left locked `initializing` by
  a `git worktree add` that was killed, so for a locked entry `make worktree` says so and to run
  `git worktree unlock <path>` before `git worktree prune`.
- Follow-up: ask dot-agent-deck upstream to keep a dispatch worktree that contains other registered
  worktrees, as it keeps one with uncommitted changes. That would make closing a unit too early safe
  rather than only documented. The request has not been filed.

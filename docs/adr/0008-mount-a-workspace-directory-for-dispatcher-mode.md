# 0008. Mount a workspace directory so dot-agent-deck's dispatcher can run in the VM

- Status: Accepted; the pull request worktree a dispatched orchestration creates goes inside its
  dispatch worktree ([0010](0010-create-pull-request-worktrees-inside-the-dispatch-worktree.md))
- Date: 2026-11-10

## Context

ADR 0002 mounts only the repository into the Lima VM. ADR 0004 builds every pull request in a worktree
under `.worktrees/` and rejected dot-agent-deck's dispatcher mode, because the dispatcher creates its
worktrees beside the project (`../<repo>-dispatch-<name>`, a location that cannot be configured), outside
that mount.

Without the dispatcher, the deck runs one orchestration at a time: a second orchestrator, or a single
background agent, cannot be started (#25). Inside the VM the repository's parent is an empty mount-point
directory owned by root, so `dot-agent-deck dispatch` fails to create its worktree. Mounting the parent
as it is would expose every other repository and file next to this one.

## Decision

Keep the repository in a directory of its own, the workspace (for example
`shift2eu-ws/shift2eu`), and mount the workspace instead of the repository. The dispatcher's
worktrees are created in it, on the Mac, and nothing else on the Mac becomes visible to the VM.

- `make vm-up` mounts the repository's parent directory, found from the main checkout, and refuses to
  start while it holds anything other than the repository, its `<repo>-dispatch-*` worktrees and
  `.DS_Store`.
- ADR 0004 still holds: each pull request is built on its own `<type>/<issue>-<slug>` branch in
  `.worktrees/`. A dispatched orchestration starts its agents in its dispatch worktree and creates the
  pull request's worktree with `make worktree` from there. The `agent/dispatch-*` branch gets no commits.
- The permission deny rules move from the untracked `.claude/settings.local.json` to a checked-in
  `.claude/settings.json`, so agents started in a fresh dispatch worktree get them too.

## Alternatives considered

- **Mount the parent directory as it is.** No move, but the VM sees every sibling project.
- **Make the parent writable inside the VM** (`chown` the mount-point stub) and leave the dispatch
  worktrees on the VM's disk. No move on the Mac, but the worktrees exist only inside the VM: unpushed
  work is lost with `make vm-delete`, and `git worktree prune` on the Mac deletes their metadata.
- **Mount the repository at a different path inside the VM** with a writable parent of the VM's own.
  Same drawbacks as the previous option, and the absolute paths git records for worktrees would differ
  between the Mac and the VM.
- **Keep one orchestration at a time.** No change, but work that could run in parallel queues up.

## Consequences

- Several orchestrations, and single background agents, can run at once, each started from a dispatcher
  pane.
- The repository must be cloned into an otherwise empty directory; an existing checkout is moved once
  (`docs/dev-environment.md`). `make vm-up` enforces it.
- Dispatch worktrees are visible on the Mac and survive `make vm-delete`.
- Claude Code's folder trust is given to the workspace once, so it covers the repository and every
  dispatch worktree.
- Two worktree layouts exist side by side: the deck's dispatch worktrees beside the repository, and the
  per-PR worktrees inside it. The start-work skill says which is which.

# Development environment

The agents run inside a [Lima](https://lima-vm.io) virtual machine. The decision is recorded in
[ADR 0002](adr/0002-lima-vm-for-agent-isolation.md).

## Why Lima instead of devbox

dot-agent-deck's own repository uses devbox. Devbox gives reproducible tool versions, but it is a Nix
shell on the host: an agent started from it runs as you, with access to your whole home directory,
including `~/.ssh`, `~/.aws`, `~/.azure` and `~/.config/gcloud`. For a project whose job is to read
cloud estates, that is the wrong default.

Lima (Apache-2.0, a CNCF project, free, native on Apple Silicon through Apple's Virtualization.framework)
runs a real Linux VM. `make vm-up` starts it with `--mount-only <workspace>:w`, where the workspace is the
directory that holds this repository and nothing else, so the repository and the worktrees beside it are
the only host paths the VM can see ([ADR 0008](adr/0008-mount-a-workspace-directory-for-dispatcher-mode.md)).
dot-agent-deck publishes Linux arm64 and amd64 builds and is documented to run on Linux, so the whole deck
(daemon, TUI and agents) runs inside the VM.

## First-time setup

### On the Mac

1. Clone the repository into a directory of its own, the workspace. The VM mounts the workspace, and
   `make vm-up` refuses to start while it holds anything other than the repository and its dispatch
   worktrees ([Dispatcher mode](#dispatcher-mode)):

   ```bash
   mkdir eu-sov-cloud-ws && cd eu-sov-cloud-ws
   git clone https://github.com/timothymamo/eu-sov-cloud.git && cd eu-sov-cloud
   ```

2. Create the credentials ([Credentials](#credentials) explains each one). For Claude, generate a token
   for your subscription:

   ```bash
   claude setup-token        # opens the browser, then prints a token valid for a year
   ```

3. Export them from a direnv `.envrc` in a directory **above the workspace**, then `direnv allow`:

   ```bash
   export CLAUDE_CODE_OAUTH_TOKEN=...   # the token from step 1
   export DEEPSEEK_API_KEY=...          # the tester (pi)
   export GITHUB_TOKEN=...              # fine-grained, limited to this repository
   ```

4. Create the VM and copy the credentials into it:

   ```bash
   brew install lima          # Lima 2.0 or newer
   make vm-up                 # create and provision the VM (several minutes the first time)
   make vm-env                # copy the three variables into the VM
   make vm-shell              # shell inside the VM, at the repository path
   ```

### Inside the VM, once

```bash
(cd .. && claude)                           # first-run setup in the workspace, see below; then /exit
git config --global user.name "Your Name"   # the agents commit as you
git config --global user.email "you@example.com"
pi auth check --provider deepseek           # prints "ready"
gh auth status                              # logged in to github.com (GITHUB_TOKEN)
dot-agent-deck validate                     # checks .dot-agent-deck.toml
dot-agent-deck daemon install               # prints "the daemon is running as dot-agent-deck.service"
exit
```

- **`claude`** has to run once by hand, in the workspace directory, so the Claude Code roles the deck
  starts in the repository or in a dispatch worktree beside it do not stop at first-run screens (trusting
  a folder trusts everything under it). It does not ask you to log in, because the token is set. Pick a
  theme, answer **yes** to trusting this folder, and run `/status`: it must show the OAuth token (your
  subscription), not an API key.
- **`dot-agent-deck daemon install`** sets the deck's daemon up as a systemd user service. Run it before
  the first `make deck`: if the TUI has already started a daemon of its own, the service is installed but
  not started, and you have to run `dot-agent-deck daemon stop` and
  `systemctl --user start dot-agent-deck.service` first.

### Every day

```bash
make vm-up && make deck    # the TUI attaches to the daemon service
```

- **Detach, don't stop.** Leave the TUI with `Ctrl+c` and **Detach**: the agents keep running in the
  service, and the next `make deck` shows them again. **Stop** ends the agents and the service until the
  VM restarts or you run `systemctl --user start dot-agent-deck.service`.
- **The service starts with the VM.** Lima enables lingering for the VM user, so no login is needed.
  `make vm-stop` stops the VM and every agent in it; `make vm-delete` removes the VM and everything
  inside it.

Tool versions are pinned in `dev/lima.yaml` under `param:`, and Renovate opens a PR when a new version
is released. A VM keeps the template it was created with, so after merging a version bump recreate it
(`make vm-delete vm-up vm-env`) and repeat the once-only steps above.

## Dispatcher mode

A [dispatcher pane](https://agent-deck.devopstoolkit.ai/docs/dispatcher-mode.md) starts orchestrations
(or single agents) in the background, so several PRs can be worked on at once, each by its own
orchestrator. In the TUI press `Ctrl+n`, select the repository, set **Mode** to `dispatcher`, and ask it to
start work on an issue or PRD.

- **Where units run.** The deck creates each unit's worktree beside the repository, at
  `../eu-sov-cloud-dispatch-<name>` on branch `agent/dispatch-<name>`. That location is fixed, which is
  why the VM mounts the workspace rather than the repository
  ([ADR 0008](adr/0008-mount-a-workspace-directory-for-dispatcher-mode.md)).
- **PRs still get their own worktree.** A dispatched orchestrator follows the `start-work` skill like any
  other ([ADR 0004](adr/0004-one-worktree-per-pull-request.md)). Run from the dispatch worktree,
  `make worktree` creates the PR's branch and worktree under the dispatch worktree's own `.worktrees/`,
  for example `../eu-sov-cloud-dispatch-<name>/.worktrees/feat-12-short-name`, so the unit's agents
  never leave their working directory and never stop at a permission prompt
  ([ADR 0010](adr/0010-create-pull-request-worktrees-inside-the-dispatch-worktree.md)). `.worktrees/` is
  git-ignored, so the dispatch worktree stays clean. Its `agent/dispatch-*` branch never gets commits or
  a PR.
- **Cleaning up, in this order.** First remove each PR worktree inside the unit, which the `pr-create`
  merge step does: `git -C <main checkout> worktree remove <path>`. Then close the unit, with
  `dot-agent-deck close <unit>` or by closing its card or tab in the deck (in the TUI `Ctrl+W`, the
  card's `[Close]` or the tab's `[×]`; in the desktop app **Close agent** or **Close all N roles**). All
  of them run the same removal: they remove the unit's worktree, or keep it when `git status` there
  shows uncommitted changes. That check does not see the git-ignored `.worktrees/`, so a unit closed
  with a PR worktree still inside it loses that worktree, uncommitted changes included, even when the
  worktree is locked. The branch and its commits survive. `make worktree` then names the commands that
  clear that worktree's stale entry, `git worktree remove <path>`, after `git worktree unlock <path>`
  when the entry is locked, and brings the worktree back once they have run. Do not use
  `git worktree prune`: it clears every stale entry in the repository, including those of healthy
  worktrees on a disk that is not mounted.
  `dot-agent-deck worktree list` shows what is left. The `agent/dispatch-*` branches stay; delete them
  with `git branch -d`.

### Moving an existing checkout into a workspace

A checkout made before ADR 0008 sits directly in a shared directory, and its VM mounts only the
repository. Move it, then point the existing VM at the workspace; the VM keeps its tools, login and
configuration:

```bash
make vm-stop
mkdir ../eu-sov-cloud-ws && mv ../eu-sov-cloud ../eu-sov-cloud-ws/
cd ../eu-sov-cloud-ws/eu-sov-cloud
git worktree repair .worktrees/*                               # only if .worktrees/ holds any
limactl edit --tty=false --mount-only "$(dirname "$PWD"):w" eu-sov-cloud
make vm-up
direnv allow                                                   # direnv tracks .envrc files by path
```

Then run `claude` once in the VM from the workspace directory and trust it, as in the once-only steps:
the trust given to the old path does not carry over.

## Credentials

Nothing from the Mac's home directory is visible in the VM. `make vm-env` copies the variables named in
`VM_ENV` (Makefile) from the shell you run it in to `~/.config/dot-agent-deck/daemon.env` in the VM,
readable only by the VM user. The values travel over standard input, never on a command line; a variable
that is not set is skipped. Two things read the file:

- the deck's daemon service, through a systemd drop-in installed by `dev/lima.yaml`, so every agent it
  starts has the variables;
- every login shell in the VM (`/etc/profile.d/eu-sov-cloud.sh`), so `make vm-shell` has them too.

| Variable | Used by | Notes |
|---|---|---|
| `CLAUDE_CODE_OAUTH_TOKEN` | Claude Code roles | From `claude setup-token`. Billed to your Claude subscription (Pro, Max, Ultra, Team or Enterprise), valid for a year, and limited to model requests. |
| `DEEPSEEK_API_KEY` | pi (tester) | Create a key for this project, so it can be revoked on its own, and keep the balance low. |
| `GITHUB_TOKEN` | `gh`, and `git push` through `gh auth git-credential` | A [fine-grained token](https://github.com/settings/personal-access-tokens) limited to this repository: contents, pull requests and issues read/write; actions read. |

- **Subscription, not pay-as-you-go.** Whenever `ANTHROPIC_API_KEY` is set, Claude Code uses it instead
  of the token and bills each call. `VM_ENV` leaves it out on purpose; do not add it. The five Claude
  Code roles share your subscription's usage limits.
- **Rotating a credential:** change it on the Mac, then `make vm-env deck-restart`. Restarting the service
  stops its running agents, so do it between tasks. Run `claude setup-token` again before the year is up.
- **Keep the values outside the workspace.** The VM mounts it, so a `.envrc` or `.env` in the
  workspace or this repository would be readable by every agent and by the model providers they send
  files to. A direnv `.envrc` in a directory above the workspace works: direnv loads it in this
  repository, and the VM cannot see it.
- **Every agent gets every variable.** The tester holds the GitHub and Claude tokens, the Claude roles
  the DeepSeek key. Scope each one narrowly so it is cheap to revoke. The file stays on the VM's disk
  (under `~/.lima` on the Mac) until `make vm-delete`.
- **Cloud accounts (for integration tests and real analyses, later):** read-only identities only
  (`ReadOnlyAccess` or narrower on AWS, `Reader` on Azure, `roles/viewer` or narrower on GCP). Do not
  copy your personal admin credentials in.

## Agent permissions

Each role's command in `.dot-agent-deck.toml` sets its permission mode explicitly, so no role depends
on the machine's default:

| Role | Agent | Starts in |
|---|---|---|
| orchestrator, coder, reviewer, auditor | Claude Code (Opus) | `--permission-mode auto` |
| releaser | Claude Code (Sonnet) | `--permission-mode auto` |
| tester | pi | no permission modes (see below) |

In auto mode Claude Code checks every action before it runs. It approves routine actions, such as
reading files, editing in the worktree and running `make`, `go`, `git` and `gh`, without a prompt, so a
worker does not stop mid-task to wait for a human. It still refuses actions it judges risky, or asks
about them first. A worker reports a refused action rather than working around it. Permission rules
apply on top of the mode: an allow rule skips the check, and a deny rule refuses the action. The deny
rules (`--no-verify`, force pushes, `git branch -D`) are checked in, in `.claude/settings.json`, so every
checkout has them, including the dispatch worktrees the deck creates. The allow rules name absolute
worktree paths, so they stay in `.claude/settings.local.json`, which is not checked in: a fresh clone
has none of them, and moving the checkout means updating their paths. The coder used to start in
`acceptEdits`. That mode approves file edits only, so the coder stopped at almost every shell command that
was not on the allowlist (#16).

Because the VM confines the agents to this repository's workspace, running the Claude Code workers with
`--dangerously-skip-permissions` inside it is a reasonable trade-off. It is not the default here: auto
mode already lets routine work run and still checks risky actions. Never use it for agents running
directly on the Mac.

pi, which runs the tester, has no permission prompts at all: it reads, edits and runs commands without
asking. Its `--approve` flag only makes it load the project's `.pi/settings.json` (and with it the
skills) without a trust prompt. For the tester the VM is the only boundary, so never start the deck
outside it.

## What leaves the VM

Each agent sends what it reads (source, command output, files) to its model provider: Anthropic for the
Claude Code roles, DeepSeek for the tester. Everything under the workspace directory is visible to
every agent: this repository, its dispatch worktrees and the git-ignored `inventories/` and `reports/`. Keep inventories and reports from
real cloud accounts outside the repository, and use only sanitised fixtures inside it. The same goes for
the environment: an agent that prints it sends the API keys and tokens above to its provider, which is
why `CLAUDE.md` forbids printing it.

## Stronger isolation: sync instead of mount

With a writable mount, agents change files on the Mac live. Lima 2.1+ can instead copy the repository
into a VM that has no host mounts and sync changes back only when you accept them:

```bash
limactl start --name=eu-sov-cloud-sync --mount-none dev/lima.yaml
limactl shell --sync . --sync-exclude .git eu-sov-cloud-sync
```

This suits one-off agent runs. For a long-lived orchestration that commits and opens PRs, the writable
mount is the practical choice.

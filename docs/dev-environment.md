# Development environment

The agents run inside a [Lima](https://lima-vm.io) virtual machine. The decision is recorded in
[ADR 0002](adr/0002-lima-vm-for-agent-isolation.md).

## Why Lima instead of devbox

dot-agent-deck's own repository uses devbox. Devbox gives reproducible tool versions, but it is a Nix
shell on the host: an agent started from it runs as you, with access to your whole home directory,
including `~/.ssh`, `~/.aws`, `~/.azure` and `~/.config/gcloud`. For a project whose job is to read
cloud estates, that is the wrong default.

Lima (Apache-2.0, a CNCF project, free, native on Apple Silicon through Apple's Virtualization.framework)
runs a real Linux VM. `make vm-up` starts it with `--mount-only <this repo>:w`, so the repository is the
only host path the VM can see. dot-agent-deck publishes Linux arm64 and amd64 builds and is documented to
run on Linux, so the whole deck (daemon, TUI and agents) runs inside the VM.

## First-time setup

### On the Mac

1. Create the credentials ([Credentials](#credentials) explains each one). For Claude, generate a token
   for your subscription:

   ```bash
   claude setup-token        # opens the browser, then prints a token valid for a year
   ```

2. Export them from a direnv `.envrc` in a directory **above** this repository, then `direnv allow`:

   ```bash
   export CLAUDE_CODE_OAUTH_TOKEN=...   # the token from step 1
   export DEEPSEEK_API_KEY=...          # the tester (pi)
   export GITHUB_TOKEN=...              # fine-grained, limited to this repository
   ```

3. Create the VM and copy the credentials into it:

   ```bash
   brew install lima          # Lima 2.0 or newer
   make vm-up                 # create and provision the VM (several minutes the first time)
   make vm-env                # copy the three variables into the VM
   make vm-shell              # shell inside the VM, at the repository path
   ```

### Inside the VM, once

```bash
claude                                      # first-run setup, see below; then /exit
git config --global user.name "Your Name"   # the agents commit as you
git config --global user.email "you@example.com"
pi auth check --provider deepseek           # prints "ready"
gh auth status                              # logged in to github.com (GITHUB_TOKEN)
dot-agent-deck validate                     # checks .dot-agent-deck.toml
dot-agent-deck daemon install               # prints "the daemon is running as dot-agent-deck.service"
exit
```

- **`claude`** has to run once by hand, in the repository, so the Claude Code roles the deck starts here
  do not stop at first-run screens. It does not ask you to log in, because the token is set. Pick a theme,
  answer **yes** to trusting this folder, and run `/status`: it must show the OAuth token (your
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
- **Keep the values outside this repository.** The VM mounts it, so a `.envrc` or `.env` here would be
  readable by every agent and by the model providers they send files to. A direnv `.envrc` in a parent
  directory works: direnv loads it in this repository, and the VM cannot see it.
- **Every agent gets every variable.** The tester holds the GitHub and Claude tokens, the Claude roles
  the DeepSeek key. Scope each one narrowly so it is cheap to revoke. The file stays on the VM's disk
  (under `~/.lima` on the Mac) until `make vm-delete`.
- **Cloud accounts (for integration tests and real analyses, later):** read-only identities only
  (`ReadOnlyAccess` or narrower on AWS, `Reader` on Azure, `roles/viewer` or narrower on GCP). Do not
  copy your personal admin credentials in.

## Agent permissions

The Claude Code roles in `.dot-agent-deck.toml` start in Claude Code's normal permission mode
(`acceptEdits` for the coder), so a worker can stop at an approval prompt and wait. Either approve
prompts as they come, or add a project allowlist in `.claude/settings.json` for the routine commands
(`make`, `go`, `git`, `gh pr`, `dot-agent-deck work-done`).

Because the VM confines the agents to this repository, running those workers with
`--dangerously-skip-permissions` inside it is a reasonable trade-off. It is not the default here. Never
use it for agents running directly on the Mac.

pi, which runs the tester, has no permission prompts at all: it reads, edits and runs commands without
asking. Its `--approve` flag only makes it load the project's `.pi/settings.json` (and with it the
skills) without a trust prompt. For the tester the VM is the only boundary, so never start the deck
outside it.

## What leaves the VM

Each agent sends what it reads (source, command output, files) to its model provider: Anthropic for the
Claude Code roles, DeepSeek for the tester. Everything under the repository directory is visible to
every agent, including the git-ignored `inventories/` and `reports/`. Keep inventories and reports from
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

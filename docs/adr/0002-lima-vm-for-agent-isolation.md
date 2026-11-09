# 0002. Run the agent team in a Lima VM rather than devbox

- Status: Accepted
- Date: 2026-11-09

## Context

The project is built by several autonomous agents orchestrated with dot-agent-deck, whose own
repository uses devbox for its environment. Devbox gives reproducible tools but no isolation: agents run
as the user, on the host, with access to SSH keys and cloud credentials. This project will hold
read-only credentials for customer clouds, which raises the cost of an agent mistake or a prompt
injection. The environment must be open source, free, and run on macOS.

## Decision

Run dot-agent-deck and all agents inside a Lima VM defined in `dev/lima.yaml`, started with
`--mount-only <repository>:w` so the repository is the only host path visible to the agents. Tool
versions are pinned as template parameters. Make targets (`vm-up`, `vm-shell`, `deck`) wrap the
commands.

## Alternatives considered

- **Devbox / Flox / Nix flakes.** Reproducible toolchains, no isolation from the host.
- **mise.** Tool version management only, no isolation.
- **Dev Containers (with Podman or Colima).** Real isolation and a portable spec, but the container
  shares the VM with other containers, and Docker Desktop is not free for every organisation. A good
  fit later if a cloud dev environment (e.g. Codespaces) becomes the target.
- **Lima `--sync` mode.** Stronger (changes are reviewed before reaching the host) but a poor fit for a
  long-lived team that commits and opens PRs continuously. Documented as an option for one-off runs.

## Consequences

- An agent cannot read or modify anything on the Mac outside the repository.
- Credentials are narrowly scoped and copied in by name from the host shell (`make vm-env`) to one file
  only the VM user can read; the VM never sees the host's credential files.
- The first `make vm-up` takes several minutes and uses disk and memory on the Mac.
- Writable mount means agent changes reach the host's working tree immediately; review still happens
  through PRs.

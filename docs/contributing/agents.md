# Working with agents

The project is built by a team of AI agents, each in its own role, running in an isolated Lima VM.
This page says who does what and how to set up the VM and open the deck.

The project is built by a team of AI agents orchestrated with
[dot-agent-deck](https://github.com/vfarcic/dot-agent-deck): an **orchestrator** that plans from PRDs
and delegates to a **tester** (writes failing tests first), a **coder**, a **reviewer**, a security
**auditor** and a **releaser** that opens and verifies pull requests. The only human gate is the merge.

The tester runs [pi](https://pi.dev) on DeepSeek V4.1 Flash; every other role runs Claude Code. Tests and
the code they check come from different model families, so one model's blind spots are less likely to
end up in both ([ADR 0003](../adr/0003-pi-with-deepseek-for-the-tester-role.md)).

The agents run inside a [Lima](https://lima-vm.io) VM that can see this repository's workspace (the
directory you clone it into, which holds only the repository and the worktrees dot-agent-deck's
dispatcher creates beside it) and nothing else on your Mac:

```bash
mkdir shift2eu-ws && cd shift2eu-ws   # the workspace the VM mounts
git clone https://github.com/timothymamo/shift2eu.git && cd shift2eu
brew install lima
make vm-create    # create and provision the VM (Go, Node.js, linters, gh, Claude Code, pi, dot-agent-deck),
                  # copy your Claude, DeepSeek and GitHub credentials in, and do the once-only setup
make deck         # open the deck; detach and the agents keep running in the VM
```

The credentials come from your shell's environment, for example a direnv `.envrc` above the
workspace. Claude Code runs on your Claude subscription, through a token from `claude setup-token`.

Details, including credentials and why Lima rather than devbox:
[docs/dev-environment.md](../dev-environment.md).

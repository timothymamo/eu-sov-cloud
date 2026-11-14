# PRD #161: Agents open pull requests as a GitHub App, and main requires the maintainer's approval

**Status:** Draft
**Issue:** #161

## Problem

`Protect main` (ruleset 26865773) requires a pull request and the required status checks, but **0
approvals**, and has no bypass actor. Anything holding a token with Contents and Pull requests write can
open a pull request and merge it once the checks pass: the agents' fine-grained token, and since #142 the
release-please App's token, which the third-party `release-please-action` holds. #157 stopped that token
from creating release tags, not from merging.

A required approval would close this: GitHub never lets the author of a pull request approve it, so a
token could open a pull request but not land it. It cannot be turned on today, because the agents push
and open pull requests **as the maintainer's own account** (`GITHUB_TOKEN`, `make vm-env`), and the
maintainer cannot approve their own pull requests: every agent pull request would be stuck.

## Outcome

The agents push branches, open pull requests and file issues as a **GitHub App** of the maintainer's,
installed on this repository only. The App's private key never enters the Lima VM: a job on the Mac mints
one-hour installation tokens from it and hands only those to the VM, where `gh` and `git` read the current
one on every call. Agent commits are authored by the App's bot identity. `Protect main` then requires one
approval, dismisses stale approvals on push and requires the last push to be approved by someone other
than its pusher, with no bypass: no App, bot or token can land a change on `main` alone, and the
maintainer's approval and merge are the single human gate. Renovate's pull requests get no exception.
The maintainer's own `GITHUB_TOKEN` leaves the VM.

## Scope

| | In scope | Out of scope |
|---|---|---|
| Source providers | none (development environment only) | |
| Target providers | none | |
| Taxonomy categories | none | |
| Development environment | `Makefile` (`vm-env`, `vm-init`, a token target), `dev/lima.yaml`, `dev/vm-init.sh`, the host-side token job, the VM's `gh` and `git` credential path, the VM's git author | The deck's own code (dot-agent-deck upstream) |
| Repository settings | The documented `Protect main` change and the checks that read it | Changing the release Apps, their rulesets or environments (ADR 0021) |
| Skills and docs | `pr-create`, `release-tag`, `review-change` where they call `gh`; `docs/dev-environment.md`, `docs/contributing/agents.md`, CLAUDE.md, an ADR | An automated reviewer App that approves (dot-agent-deck's model, rejected below) |

## Acceptance criteria

### The App's credential (milestone 1)

- [ ] The maintainer's `.envrc` **above the workspace** names the App ID, the installation ID and the
  **path** of the App's private key file, itself outside the workspace and readable only by the
  maintainer. No file in the workspace holds any of them, and `make vm-env` never copies the key or its
  path into the VM.
- [ ] A make target on the Mac mints an installation token from the key (standard library or tools the
  Mac already has, no new third-party dependency without a reason in the commit message) and writes it
  into the VM over standard input, never on a command line, to a file readable only by the VM user.
- [ ] While the VM runs, the token is refreshed before it expires, without restarting the deck: a running
  agent's next `gh` or `git push` uses the new token. Given a stopped refresh, the next call after expiry
  fails with an error that names the refresh target, not a generic 401.
- [ ] No command prints a token, the key or its contents; nothing writes them to a log, the workspace or a
  test. Tests use invented keys generated in `t.TempDir()`.

### The agents' identity (milestone 1)

- [ ] In the VM, `gh` and the git credential helper (`gh auth git-credential`) use the App token when its
  file is present and fall back to `GITHUB_TOKEN` otherwise, so milestone 1 can merge and be switched on
  without a gap. `make vm-init`'s credential check passes with either.
- [ ] A pull request opened by an agent after the switch shows the App as its author
  (`gh pr view --json author` gives `is_bot` `true` and the App's login), and its commits' author is the
  App's bot identity (`<app id>+<slug>[bot]@users.noreply.github.com`), set by `make vm-init`.
- [ ] Every `gh` call the skills make works with the App token: push, `pr create`, `pr view`, `pr checks`,
  `run list/view/watch`, `issue create/view`, and the `release-tag` skill's reads of rulesets, environments
  and releases. Any call that needs a permission the App lacks is listed with the permission, and the
  maintainer's App permissions in `docs/dev-environment.md` cover them.
- [ ] Branch commits are not signed; `main`'s commits stay GitHub-signed squash merges (`verified`
  `true`). No ruleset requires signed commits.

### The gate (milestone 2)

- [ ] A check (in the `release-tag` skill's step 2 and as a make target the maintainer can run) reads
  `Protect main` and fails unless: `required_approving_review_count` is at least 1,
  `dismiss_stale_reviews_on_push` and `require_last_push_approval` are `true`, and `bypass_actors` is
  empty. Before the switch it reports the gaps without failing a release (a documented transition flag),
  after it, it fails closed.
- [ ] `pr-create` never approves a pull request and never merges one the maintainer has not approved;
  after the switch it reports "waiting for the maintainer's approval" instead of trying to merge.
- [ ] A Renovate pull request needs the maintainer's approval like any other; `.github/renovate.json5`
  keeps `automerge: false`.
- [ ] `docs/dev-environment.md` gives the maintainer's switch-over runbook (below) step by step, with a
  check after each step and how to roll back; CLAUDE.md and `docs/contributing/agents.md` say that the
  single human gate is the maintainer's approval and merge, and that nobody pushes to an agent's branch by
  hand after the switch (a push by the maintainer could not be approved).

### After the switch (milestone 3)

- [ ] `GITHUB_TOKEN` is no longer in `VM_ENV`, and the fallback to it is removed: without the App token
  file, `gh` and `git push` in the VM fail with an error naming the refresh target.
- [ ] The docs no longer tell the maintainer to create a fine-grained personal token for the agents.

## Milestones

1. [ ] **App token plumbing and bot identity.** The host-side mint and refresh, the VM token file, the `gh`
   and git credential path with the `GITHUB_TOKEN` fallback, the bot git author, `make vm-env`/`vm-init`
   changes, tests, an ADR (identity, token model, why not a machine user or a reviewer App), and
   `docs/dev-environment.md`. Merged under the old token; nothing is enforced yet.
2. [ ] **Gate checks, skills and docs.** The `Protect main` check with its transition flag, `pr-create`
   and `release-tag` changes, CLAUDE.md, `docs/contributing/agents.md`, the switch-over runbook. Depends
   on 1.
3. [ ] **Remove the maintainer's token.** After the maintainer's runbook step 6: drop `GITHUB_TOKEN` from
   `VM_ENV` and the fallback, turn the check's transition flag off, update the docs. Opened by the App and
   approved by the maintainer: the first change through the new gate. Depends on 2 and the runbook.

## Switch-over runbook (maintainer)

Agreed with the maintainer on 2026-11-14. Each step is safe to stop at.

1. Create the App: installed on this repository only; permissions Contents, Pull requests and Issues
   read and write, Workflows write (agents edit `.github/workflows/`), Actions, Checks and Commit statuses
   read, and whatever else milestone 1 lists. Generate its private key; store the key and the IDs as the
   acceptance criteria say. Nothing changes yet.
2. Merge milestone 1 (still opened under the maintainer's token).
3. Run the token target, reload the VM's credentials and restart the deck **between tasks**. A test pull
   request proves the App is the author. Merge milestone 2 (the first PR the App opens).
4. Merge or close every open pull request **authored by the maintainer**: with no bypass actor, one left
   open after step 5 can never be approved. Release-please's and Renovate's pull requests are not affected.
5. Change `Protect main`: 1 required approval, dismiss stale approvals on push, require approval of the
   most recent push. Run the gate check.
6. Remove `GITHUB_TOKEN` from the VM (milestone 3) and revoke that personal token.

Roll back at any step before 5 by deleting the token file in the VM (the fallback takes over); after 5 by
setting the required approvals back to 0.

## Decisions and open questions

**Decided (maintainer, 2026-11-14):**

- **A GitHub App, not a machine user.** No second account under GitHub's terms, no long-lived token, and
  permissions scoped to this repository.
- **The App's key stays on the Mac** (option B). The VM only ever holds a token that expires within the
  hour, so a compromised agent, or a model provider that sees a file, holds nothing that outlives it. Option
  A (the key in the VM, tokens minted there) was simpler but would give every agent a credential that
  never expires.
- **The credential lives in the maintainer's direnv `.envrc` above the workspace**, as `GITHUB_TOKEN`,
  the Claude token and the DeepSeek key do today (`docs/dev-environment.md`, Credentials).
- **Renovate pull requests are not exempt** from the approval.
- **No commit signing on branches.** An App has no account to register a signing key with; only commits
  made through GitHub's API would be signed for it, which would mean rewriting how every agent commits.
  `main` stays signed: every commit on it is a squash merge GitHub creates and signs.
- **The bot opens, the maintainer approves**, rather than dot-agent-deck's model, where the maintainer's
  account opens pull requests and an AI reviewer App (`dot-agent-deck-reviewer[bot]`) casts the required
  approval, with an admin bypass for the owner and Renovate automerging under a bypass. That model saves a
  click per pull request but lets a change reach `main` with no human reading it, which is the gap this PRD
  closes. An AI reviewer may comment or request changes later, but never counts as the approval.
- **The switch-over order** is the runbook above.

**Open (to settle in milestone 1, recorded in its ADR):**

- How the refresh runs on the Mac: a launchd agent, or a loop the `deck` target starts. It must survive
  the Mac sleeping and say clearly when it stopped.
- Whether `gh auth status` and `gh api user` (used by `make vm-init` and some skills) work with an
  installation token, and what replaces them where they do not.
- Whether the deck itself (dot-agent-deck upstream) reads `GITHUB_TOKEN` for anything, and what it needs.
- Which reads in the `release-tag` skill need permissions beyond those listed (rulesets with bypass
  actors, environments, deployment branch policies): if the App cannot be given them read-only, those
  checks stay maintainer checks and the skill says so, as it already does for the Actions secrets list.

Security-sensitive throughout: every milestone runs the `security-audit` skill. Refs #142, #157.

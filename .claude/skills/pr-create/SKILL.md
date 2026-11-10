---
name: pr-create
description: Take committed work on a branch to a verified GitHub pull request - push, open the PR from the template, wait for CI, report results, and stop before merging. Use when work is committed and ready for a pull request, or when explicitly told to merge an approved PR.
---

# Create and verify a pull request

You never modify source code in this flow.

## Open

1. Work in the worktree named in the task: run every non-git command from it (`cd <path> && ...`). Run
   git as `git -C <path> ...`, never `cd <path> && git ...`. Write the worktree path out in full; do
   not store it in a shell variable. Confirm the tree is clean (`git -C <path> status`) and the branch
   is not `main` (`git -C <path> branch --show-current`). If either check fails, or the task names no
   worktree, stop and report.
2. Run `make check`. If it fails, report the exact output and stop.
3. If this PR completes a file PRD: set its status to `Complete`,
   `git -C <path> mv prds/<n>-<slug>.md prds/done/`, fix links to the old path, and commit
   `docs(prd): complete PRD #<n>`.
4. `git -C <path> push -u origin HEAD`
5. `gh pr create --base main --title "<conventional commit style title>" --body-file <file>`, using
   `.github/pull_request_template.md` as the structure. Use `Closes #<n>` only when the PR completes the
   PRD or issue, otherwise `Refs #<n>`.

## Verify

6. `gh pr checks <n> --watch --fail-fast` until every check completes.
7. On failure: `gh run view <run-id> --log-failed`, report the failing job and the exact error, and stop.
   Do not diagnose or fix; the orchestrator re-delegates.

## Report and stop

8. Report through `dot-agent-deck work-done`: the PR URL, each check and its result, and anything notable.
   **Do not merge and do not enable auto-merge.**

## Merge (only when explicitly instructed)

9. From the repository root, remove the worktree: `git -C <repository root> worktree remove <path>`.
   It refuses when the worktree has uncommitted changes; then report and stop, and never force it.
10. Still from the root: `gh pr merge <n> --squash --delete-branch`, then confirm the linked issue's state
    matches the PR body (`gh issue view <n> --json state`) and report. If the merge fails, report the
    error; `make worktree BRANCH=<branch>` brings the worktree back for a fix.

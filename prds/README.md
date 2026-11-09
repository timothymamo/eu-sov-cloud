# PRDs

Product Requirements Documents: one per feature, named `<issue-number>-<slug>.md`. Each PRD has a GitHub
issue, and the orchestrator plans and delegates from it. Create one with the `prd-create` skill
(template: `.claude/skills/prd-create/template.md`).

A PRD is written in its own `prd/<n>-<slug>` worktree and merged before implementation starts; each
milestone is then built on its own branch and worktree (`start-work` skill). The PR that completes a PRD
moves it to `done/` and closes its issue.

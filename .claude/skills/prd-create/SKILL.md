---
name: prd-create
description: Write a Product Requirements Document (PRD) for a new eu-sov-cloud capability and open its GitHub issue. Use when the user wants to plan a feature, a new provider, a new service category, or any change too big for a single PR.
---

# Create a PRD

A PRD is the unit of work the orchestrator delegates against. It states *what* and *why*, and the
observable behaviour that proves it is done. It does not prescribe the implementation.

## Steps

1. Ask only for what you cannot infer: the problem, who it is for, and what is out of scope.
2. Open the GitHub issue first so the PRD has a number:
   `gh issue create --title "PRD: <title>" --label prd --body "PRD: prds/<n>-<slug>.md"`
3. Create the PRD's worktree with the `start-work` skill: `make worktree BRANCH=prd/<n>-<slug>`. Do the
   remaining steps there, never in the repository root.
4. Copy [template.md](template.md) to `prds/<n>-<slug>.md` (`<n>` = issue number, `<slug>` = kebab-case).
5. Fill every section. Each acceptance criterion must be observable from outside the code (CLI output,
   report content, exit code) so the tester can pin it with a test.
6. Break the work into milestones that can each ship as one PR, in dependency order.
7. Commit the PRD with message `docs(prd): add PRD #<n> <title>` and take it through `pr-create`. It is
   merged before implementation starts, so each milestone's worktree, cut from `main`, contains it.

## Rules

- Use the ubiquitous language from CLAUDE.md: *source* (hyperscaler), *target* (EU provider),
  *inventory*, *catalog*, *taxonomy category*, *mapping*, *assessment*.
- Name the source providers, target providers and taxonomy categories in scope explicitly.
- When a PRD completes, the PR that completes it moves it to `prds/done/` and says `Closes #<n>`.

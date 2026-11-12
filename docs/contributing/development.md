# Development

What you need to build and test `shift2eu`, the make targets you use every day, and how a change
reaches `main`.

Requirements: Go (version in `go.mod`), [golangci-lint](https://golangci-lint.run) v2,
[govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck),
[shellcheck](https://www.shellcheck.net) (the version `dev/lima.yaml` pins), GNU Make. All of them come
preinstalled in the project's Lima VM ([Working with agents](agents.md)). actionlint needs no install:
`go.mod` pins it as a tool, and `go tool actionlint` builds it.

```bash
make help     # list targets
make check    # lint (incl. architecture rules) + tests + vulnerability scan
make build    # ./bin/shift2eu and ./bin/shift2eu-server
```

## Contributing

1. Start from a PRD (`prds/`) or an issue.
2. Work in its own worktree, never on `main`: `make worktree BRANCH=feat/<issue>-<slug>` prints the
   path. The hooks in `.githooks/` refuse commits and pushes to `main` (`make git-hooks` enables them;
   `make vm-up` does too).
3. Test first, then implement; keep `make check` green.
4. Conventional Commits, one PR per milestone.

## Shell scripts and workflows

`make lint` also lints what is not Go
([ADR 0022](../adr/0022-lint-shell-scripts-and-workflows-with-shellcheck-and-actionlint.md)), and CI's
`Lint` check runs the same tools in its `lint-shell` job:

- **shellcheck** checks every shell script git tracks: every `*.sh` file and every file whose first line
  runs `sh` or `bash`, such as the hooks under `.githooks/`. `dev/shellcheck.sh` builds the list from
  `git ls-files`, so a new script is checked without any edit. It runs with `--severity=style`, so it
  reports every finding, including SC2086 (word splitting).
- **actionlint** checks `.github/workflows/*.yml`: syntax, `${{ }}` expressions and contexts. It passes
  each `run:` script to the shellcheck on the `PATH`.
- **Disable directives:** fix a finding rather than silence it. When a check cannot be satisfied,
  silence it on its own line with a code and a reason:
  `# shellcheck disable=SC2086 # FLAGS is a list and must split`. `test/lint` refuses a directive
  without a code of the form `SC` and four digits (`disable=all` included) or without the reason after
  a second `#`.

To install the pinned shellcheck outside the VM, take the version and the digest for your platform from
`dev/lima.yaml` (`shellcheckVersion`, `shellcheckSha256Amd64` or `shellcheckSha256Arm64`). The pins
cover Linux only. On a Mac, `brew install shellcheck` gives the current release, which can report
findings that the pinned version does not. On Linux:

```bash
v=v0.11.0; a=x86_64                    # shellcheckVersion; aarch64 on arm64
curl -fsSLO "https://github.com/koalaman/shellcheck/releases/download/${v}/shellcheck-${v}.linux.${a}.tar.gz"
sha256sum "shellcheck-${v}.linux.${a}.tar.gz"   # must equal shellcheckSha256Amd64 (Arm64 on aarch64)
tar -xzf "shellcheck-${v}.linux.${a}.tar.gz" && install -m 0755 "shellcheck-${v}/shellcheck" ~/.local/bin/
```

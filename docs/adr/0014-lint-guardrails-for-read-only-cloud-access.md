# 0014. Hold source adapters to read-only cloud access with probe-pinned lint guardrails

- Status: Accepted
- Date: 2026-11-11

## Context

CLAUDE.md rule 1 says a source adapter only reads from a cloud, and rule 3 that it never handles a
credential itself. Until now only code review and a read-only IAM role enforced that. Neither is
enough on its own. A cloud SDK puts reads and writes on the same client (`ec2.Client` has
`DescribeInstances` beside `TerminateInstances`), so depguard, which allows or refuses a whole
package, cannot keep a write out. And an operator's default credential chain often holds more rights
than the documented read-only role. One write call in an adapter, or in an integration test (which
runs with real credentials), would change a customer's cloud. That is the worst outcome this tool can
have (issue #38).

Review and the security audit of the first attempt showed how such a rule fails:

- A **denylist** of write verbs always misses one. `RequestSpotInstances`, `LeaveOrganization`,
  `Purchase*` and `Decrypt` got past a list of about 75 verbs.
- A **deny list of imports** misses every package that calls a method by name. `text/template` runs
  `{{.C.TerminateInstances .Ctx nil}}`, and `os.StartProcess` runs the cloud CLI.
- **Type inference** lets an SDK client satisfy a generic constraint without naming any operation type.
  forbidigo cannot see a call through a type parameter at all.
- A rule scoped to `source/{aws,azure,gcp}` misses a **sibling helper** (`source/awsutil`).
- forbidigo's type-aware patterns fail **silently**. golangci-lint merges issues per line, so a probe
  that compared lines only could not see a broken entry that shared a line with a working one.

## Decision

We hold source adapters to read-only cloud access with three lint guardrails. Each is an allowlist
wherever the code can tell reads from writes, and a probe pins every entry against the golangci-lint
version CI pins.

1. **forbidigo, group `cloud-read-only`.** On an AWS, Azure or GCP SDK client, only read verbs are
   allowed, and only a read operation's input, output, options, request or response type may be named.
   Packages no allowlist covers, and the hooks that rewrite a request or hand out a credential, are
   refused by name. The group applies to every file lint checks.
2. **depguard, a strict import allow list** for every package under `internal/adapter/source` except
   `inventoryfile` (which has its own strict rule) and the contract suite `sourcetest`. Tests are
   included. A second rule allows the cloud SDKs only in their provider's adapter package.
3. **`test/lint` on the AST.** An interface a cloud source adapter declares may declare only read
   verbs or unexported methods: named interfaces, interface literals and constraints alike. We chose
   this over refusing generic interfaces outright. The allowlist closes the inference route just as
   well, and still lets an adapter declare a small read interface (a paginator's client) to test
   against.

forbidigo's entry for an adapter's own types stays a denylist of write verbs. forbidigo cannot tell
a method from a field, so an allowlist there would refuse the adapter's own fields and the port's
`Inventory` method. The AST allowlist covers the declarations instead.

Every entry is pinned. `TestCloudReadOnlyProbe` runs lint with `--uniq-by-line=false` on probe adapters
over stub SDKs. Each want comment names the message it expects, and every entry of the group must
refuse a probe line of its own. `TestCloudReadOnlyPatterns` checks the patterns, read from
`.golangci.yml`, against lists of real write and read names, and requires a name for every
alternative and optional part (`?`, `*`) of each denylist entry. CI fails on a golangci-lint version
mismatch. Locally the probe runs anyway and warns.

The rules, how forbidigo renders a name, how to allow a new read verb, and the gaps that remain are
documented in `docs/architecture.md` (Cloud read-only rule), not in `.golangci.yml`.

## Alternatives considered

- **A write-verb denylist in forbidigo.** It is shorter and needs no regex trie, but a missing verb
  reaches a customer's cloud. With an allowlist, a missing read verb fails lint and is added on purpose.
- **Rely on the read-only IAM role.** It is the backstop, but adapters use the default credential chain,
  which may hold an admin profile.
- **A custom go/analysis linter** that resolves every call to its SDK method with full type information.
  It would see through type parameters, but it needs its own build, its own plugin wiring into
  golangci-lint and its own tests. forbidigo, depguard and AST tests cover the known routes with tools
  the project already runs.
- **Refusing generic interfaces with methods in source adapters.** It is simpler, but it is stricter
  than needed and still lets a non-generic interface with a write method through.
- **A version check that always fails on mismatch.** It kept developers off `make check` on any other
  golangci-lint, which invites skipping the probe. Only CI needs to prove the pinned version.

## Consequences

- A new read verb, a new SDK root, or a standard package an adapter needs fails lint until someone adds
  it. The change also needs a probe or pattern case, and review sees it.
- The probe needs a stub for every SDK package an entry names. Adding an entry means adding a probe
  line that only that entry refuses.
- Source adapters cannot import `os`, `reflect`, templates or `net`. A helper an adapter shares goes in
  the adapter's own package, or in a subpackage named in its allow list.
- Endpoint overrides (`aws.Config.BaseEndpoint`, `option.WithEndpoint`, an Azure `cloud.Configuration`)
  remain allowed, because a sovereign region needs them. Whether an adapter sets one safely is a
  reviewer check, and an accepted gap.
- A write sent by hand through an SDK's HTTP client, with no SDK type named, is still a gap. The
  read-only role is the backstop.

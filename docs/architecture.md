# Architecture

`eusov` follows a hexagonal (ports and adapters) architecture with a small, explicit domain model.
The decision and its alternatives are in [ADR 0001](adr/0001-hexagonal-architecture-with-provider-neutral-taxonomy.md).

## Layers and the dependency rule

```
            ┌──────────────────────────── cmd/eusov (composition root) ───────────────────────────┐
            │                                                                                      │
            │   internal/cli ──────▶ internal/app/analysis ◀────── internal/adapter/...           │
            │   (driving adapter)      (use case + ports)          (driven adapters: implement     │
            │                                 │                     the ports)                     │
            │                                 ▼                           │                        │
            │                         internal/domain/... ◀───────────────┘                        │
            │                         (stdlib only, no I/O)                                         │
            └──────────────────────────────────────────────────────────────────────────────────────┘
```

Arrows are imports. They point inwards only:

- **Domain** (`internal/domain/...`) imports only other domain packages and an explicit allow list of
  pure standard packages. No I/O, no `context`, no clock, no logging. This is where the value of the
  project lives, so it is the most heavily unit-tested code.
- **Application** (`internal/app/analysis`) orchestrates a use case and declares the interfaces (ports)
  it needs, next to where they are consumed. It never imports an adapter.
- **Adapters** (`internal/adapter/...`) implement ports. Each cloud SDK is confined to one adapter
  package, and its types are translated to domain types at the boundary. They never import the CLI
  or `cmd/`. Helpers that adapters share live in `internal/adapter/internal/`: every adapter that
  reads a JSON document (the inventory file, the target catalog snapshots) decodes it through the
  strict decoder `adapter/internal/jsondoc`
  ([ADR 0009](adr/0009-decode-adapter-json-documents-through-one-strict-decoder.md)). A helper does
  no I/O unless that is decided: lint holds every package there to an allow list of packages that
  do no I/O, and a helper that needs I/O needs a lint rule of its own. Lint names each helper an
  adapter may import (`adapters-are-built-in-cmd`), never by prefix, so lint flags any adapter that
  imports a helper it does not name, such as one under a `testdata` or `_` directory, which lint
  never visits. The adapters that must stay
  offline (the inventory-file reader, the snapshot reader, the embedded-snapshot targets and the
  reporters) each name what they import in a strict allow list. A reporter writes only to the
  `io.Writer` it is given: it reads no file, no environment and no clock, and makes no network
  call. The live sources share one strict allow list, `cloud-adapters-reach-the-cloud-through-the-sdk`
  (Cloud read-only rule below).
- **CLI** (`internal/cli`) turns user input into use-case calls and prints results. No business rules.
  Only `cmd/eusov` imports it.
- **`cmd/eusov`** is the only place that constructs adapters and injects them.

The rule is enforced by `depguard` in `.golangci.yml`, so `make lint`, which lints the host natively and
then every GOOS/GOARCH `.goreleaser.yaml` builds with cgo off, fails on a violating import. These are all
the ways code could be hidden from it, and what refuses each:

- A comment. Every file is linted, also one that looks generated, and `test/lint` fails on any nolint
  directive that would hide a depguard issue (one naming `depguard`, `all` or no linter) and on a build
  constraint that negates the `e2e` or `integration` tag (lint sets both, so it would never read that
  file).
- A line directive. depguard matches its file globs against the position a `//line` directive names,
  so `test/architecture` (`TestRepositoryHasNoLineDirectives`) fails on any line directive in a Go file
  git tracks or would add.
- A symbolic link. golangci-lint lints a linked Go file with the nolint directives of its target.
  `TestRepositoryHasNoLineDirectives` fails on any symlink git tracks or would add, and `test/lint`
  fails on any Go file the `./...` pattern reaches that is a symlink, git-ignored ones included, as
  deliberate defence in depth.
- A file name. `test/architecture` checks every directory a strict depguard rule names (the domain, the
  application layer, the snapshot reader, the adapter helpers, the inventory file reader and the
  targets that embed a snapshot) package by package, for every platform `.goreleaser.yaml` builds. It
  reads those directories from `.golangci.yml`, lists them with `go list` and holds each package to the
  allow list of every rule that covers it, and it holds every package of the module in the release
  binary to Go sources in their own directory, so assembly, cgo, line directives and symlinked
  directories cannot get past the file-name-based rules
  ([ADR 0012](adr/0012-check-layer-dependencies-by-package-for-every-release-target.md)).

A cloud SDK is imported only in its provider's adapter package (`source/aws`, `source/azure`,
`source/gcp`, `target/ovhcloud`, `target/scaleway`, `target/upcloud`): the depguard rule
`cloud-sdks-only-in-provider-adapters` refuses it everywhere else, a sibling helper package and
`cmd/eusov` included.

## Cloud read-only rule

A source adapter only reads from a cloud (CLAUDE.md rule 1) and never handles a credential itself
(rule 3). An import rule alone cannot hold that, because a cloud SDK puts reads and writes on the same
client, and the operator's credential chain may hold more rights than the read-only role. Three lint
guardrails hold it instead; [ADR 0014](adr/0014-lint-guardrails-for-read-only-cloud-access.md) records
why.

1. **forbidigo, group `cloud-read-only`, an allowlist of reads.** In every file lint checks, tests and
   integration tests included, an SDK client allows only read verbs: AWS `Describe*`, `Get*` and
   `List*`; Azure `Get*`, `List*`, `NewGet*` and `NewList*` (the pagers); GCP `Get*`, `List*`,
   `AggregatedList*` and `Close`. Only a read operation's input, output, options, request or response
   type may be named. Further entries refuse whole packages or single names: clients no allowlist
   covers (`google.golang.org/api`, the hand-written GCP clients, the AWS SDK v1, Azure track 1), the
   hooks through which a read is rewritten into a write or a credential is handed out (signers,
   credentials providers, middleware, `aws.Config.Credentials`, `config.With*`, Azure transports and
   policies, `option.With*` transports and credentials, gRPC connections), allowed verbs that return
   a secret, and, on a source adapter's own types, a method named for a write.
2. **depguard, rule `cloud-adapters-reach-the-cloud-through-the-sdk`, a strict import allow list.**
   Every package under `internal/adapter/source` but `inventoryfile` and `sourcetest`, tests
   included, imports only the standard packages an adapter needs, the cloud SDK roots and the project
   packages it uses. So nothing that calls a method by a name forbidigo cannot see (`reflect`,
   `text/template`, `html/template`), runs a program (`os`, `os/exec`, `syscall`, `golang.org/x/sys`,
   cgo) or opens a connection of its own (`net`, `database/sql`) gets in.
3. **`test/lint`, on the AST.** An interface that a cloud source adapter declares, named, literal or a
   type-parameter constraint, may declare only read verbs (`Describe`, `Get`, `List`, `NewGet`,
   `NewList`, `AggregatedList`, `Close`, `HasMorePages`, `NextPage`, `More`) or unexported methods:
   an SDK client would satisfy any other with its operation types left to inference, and a call
   through a type parameter has no type name forbidigo can match. Elsewhere in the module an interface
   may not declare a method named for a write (a denylist of verbs; a source adapter cannot import
   such a package anyway). No struct embeds an SDK type, no alias names one, no file dot-imports an
   SDK.

As with depguard, every file is linted, also one that looks generated, and `test/lint` fails on any
nolint directive that would hide a depguard or forbidigo issue.

### How the rule is pinned

forbidigo's type-aware patterns fail silently when they are wrong. `TestCloudReadOnlyProbe` runs the
golangci-lint version CI pins, forbidigo and depguard, on probe adapters over stub SDKs
(`test/lint/testdata/cloudreadonly`), offline, with lines left unmerged (`--uniq-by-line=false`). A
line marked `// want "<message prefix>"` must be refused with that message, no other line may be,
and every entry of the group must refuse a line of its own, so an entry that stops matching fails.
The stubs have the real SDKs' import paths, package names and shapes; azcore's `ClientOptions` is laid
out as in the real SDK. `TestCloudReadOnlyPatterns` checks the client and operation patterns against
long lists of write and read names, so a typo in a first-letter class fails without golangci-lint. It
covers the denylist entries too (the secret, credential, hook and gRPC entries and the refused
packages): `cloudDenied` gives names each entry must refuse, and the test drops each alternative and
optional part (`x?`, `x*`, `x{0,n}`) of an entry's pattern and pkg in turn and fails unless one of
those names then gets through. So narrowing an entry, say dropping `Secrets?` from the Azure secret
entry or the `\w*` in `List\w*AuthTokens`, fails at once. Where no real name needs a part, a stand-in
of its shape does, marked `// stand-in`. A character class is not mutated, and `regexp/syntax` turns
an alternation of single letters (`URI|URL`) into one (`UR[IL]`).
`cloudDenylistReads` holds reads the entries were narrowed to allow (`armkeyvault.KeysClient`).
`TestInterfaceFindingsProbe` runs the AST rule on the same probe. `make lint` and every entry of CI's
lint-target matrix, which reports as the Lint check, run them; CI fails on a golangci-lint version
that differs from the pinned one, and elsewhere the probe runs anyway and logs a warning that points
at the dev VM ([dev-environment.md](dev-environment.md)).

### How forbidigo renders a name

Probed on golangci-lint v2.15.0 (forbidigo v2.3.1). Probe again when either moves.

- A method is `<package name>.<type>.<method>`, with a pointer receiver dropped:
  `ec2.Client.RunInstances` for `(*ec2.Client).RunInstances`. A package-level name is
  `<package name>.<name>`: `ec2.RunInstancesInput`. It is the package name, not its path or import
  alias: `armcompute` for `.../armcompute/v5`, and `compute` for both GCP compute SDKs.
- A field read or set through a selector is `<package>.<type>.<field>`, as a method is:
  `policy.ClientOptions.Transport`. A key in a composite literal is `<package>.<field>`, without the
  type: `policy.Transport` in `policy.ClientOptions{Transport: t}`.
- The type is the one the selector is used on, not the one that declares the method or field: a
  method promoted from an embedded client is matched against the embedding type, and a field of an
  embedded struct against the outer one. A value of an alias type is matched against the type the
  alias names (a `*policy.Request` is an `exported.Request`), a generic type by its name alone.
- `pkg` is matched against the import path of the package that declares that type, or of the package a
  qualified name names, which is what tells the SDKs apart.
- A call through an interface is matched against the interface, not the type behind it. A selector on
  a type parameter, an interface literal or an unnamed struct has no type name, so no entry can match
  it; `test/lint` refuses the declarations that lead there.
- `pattern` and `pkg` are unanchored, so every entry spells out `^` and `$`.
- golangci-lint turns forbidigo's `//permit:` comments off. `exclude-godoc-examples` is false, so a
  godoc example is checked like other code.
- depguard sees the imports of the code cgo generates, not `C`: cgo is refused through `unsafe`.

### Allowing a new read verb

Go regexps have no lookahead, so "a name that does not start with an allowed verb" is written as a
prefix trie, one branch for each way a name can leave the verbs. For AWS:

```
[A-CEFH-KM-Z]      a first letter no verb starts with (not D, G, L)
D[^e]|De[^s]|...   the first letters of a verb, then another letter
Describe[^A-Z0-9]  the whole verb run on in lower case, so Describes, Getaway and Listen are refused
```

Every other name is allowed: `Get`, `GetObject`, `DescribeInstances`. A name that stops part-way
through a verb (`Ge`, `Lis`, `Aggregate`) is not refused; no SDK client has one. Azure's `New` branch
repeats the `Get` and `List` branches after `New` (`NewDeletePager` is refused); GCP's `Close` is a
whole name, so `Close\w` refuses anything longer.

To allow a read verb, say `Search` on AWS, drop `S` from the first-letter class and add
`S[^e]|Se[^a]|Sea[^r]|Sear[^c]|Searc[^h]|Search[^A-Z0-9]`. Where two verbs share a prefix, the class
at the fork leaves out both next letters (`L[^io]` for `List` and `Lookup`). Change the SDK's method
and operation entries alike, add only a verb that cannot write, add the verb to the reads in
`TestCloudReadOnlyPatterns` and to `readName` in `test/lint/sourcewrites_test.go`, add a read call of
it to the probe, and run `make lint`. Before using an SDK type no entry covers, such as a new
hand-written GCP package, add an entry for it, a probe line only it refuses and names for it in
`cloudDenied`.

### Gaps that remain

- **No package check of the source adapters' imports.** `test/architecture` (ADR 0012) skips
  `cloud-adapters-reach-the-cloud-through-the-sdk` (`skippedStrictRules`): its files cover every
  directory under `internal/adapter/source` but two, and its allow list names SDK roots as prefixes,
  neither of which a package-by-package list can say. depguard holds it, in `make lint` and CI, for
  the host and every release target.
- **Files lint never loads**: a file behind a build tag other than `integration` and `e2e`, a
  `//go:build ignore` helper, and the `testdata`, `_` and `.` directories `./...` skips. `test/lint`
  fails on such a file, and on an imported package in such a directory. A main package there, run
  with `go run`, is not checked. `make lint` and CI lint the host and every release target, but
  `test/lint` lists only linux/amd64, so it also fails on a file only for another release platform
  until it lists that platform too.
- **A write or a credential that uses no SDK type**: a request signed, or given a bearer token, by hand
  with only the standard library. The strict import list leaves no `net` package to send it with,
  but an SDK's own HTTP client could carry it. Lint is a guardrail; the read-only role is the
  backstop.
- **Endpoint overrides are a reviewer check, an accepted gap.** `aws.Config.BaseEndpoint`,
  `option.WithEndpoint` and an Azure `cloud.Configuration` send a signed request or a bearer token to
  another host. A sovereign cloud region needs one, so lint does not refuse them. A reviewer checks
  that every endpoint an adapter sets comes from configuration naming a known cloud endpoint, never
  from a constant or input that could point elsewhere.
- **An interface from another module** that declares a write and that an adapter could name: the SDK
  interfaces with a write method are refused by name (`manager.UploadAPIClient`), so check a new SDK,
  a new package of one, or a new dependency for such interfaces.
- **Allowed verbs that do more than read.** The secret entries refuse the known ones that mint
  credentials, register an identity, return a secret or a presigned URL; others remain, such as AWS
  Cost Explorer `Get*` calls, billed per request, and the secrets in the fields of an ordinary read
  (EC2 user data, Lambda and ECS environment variables), which the adapter drops. A secret read
  through a generic interface with its types inferred (a `Get*` method) is not refused by name. The
  secret entries also refuse a few reads that return none, such as Azure
  `GetAuthSettingsV2WithoutSecrets` and the key-vault `SecretsClient` listings.
- **Write verbs outside the source adapters** are a denylist (`writeVerbs` in `test/lint` and the
  last forbidigo entry), so an interface elsewhere named with a verb not on it gets through. A source
  adapter can import only the project packages its depguard rule lists, which keeps that interface
  out of its reach unless one of those packages declares it.

## Domain-is-pure rule

The domain does no I/O and reads no clock. The depguard rule `domain-is-pure` holds it to an allow list
of pure standard packages, but it allows `fmt` for `fmt.Errorf` and `time` for `time.Time`, and both
hold functions that reach standard I/O, the clock, zoneinfo files or the environment. An import rule
cannot see a call, so the forbidigo group `domain-is-pure` refuses those functions. It holds for
production files under `internal/domain/` only: two exclusion rules at the end of
`linters.exclusions.rules` drop its issues in every other path and in `_test.go` files, godoc examples
included, so the CLI and tests may print and read the clock.

### Why each entry is listed

Each claim was checked against the Go 1.27 source.

| Entry | Refuses | Why |
|---|---|---|
| `fmt.Print*` | `fmt.Print`, `Printf`, `Println` | They write to `os.Stdout`. |
| `fmt.Scan*` | `fmt.Scan`, `Scanf`, `Scanln` | They read `os.Stdin`. |
| `print` and `println` | the builtins | They write to standard error. `pkg: '^$'` matches a name with no package, which is the builtin, so a call of a domain function named `println` is not refused. A `print :=` that shadows the builtin is refused at the `:=`, because forbidigo finds no package for a name being declared. |
| the clock | `time.Now`, `Since`, `Until`, `Sleep`, `After`, `Tick`, `NewTimer`, `NewTicker`, `AfterFunc` | They read or wait on the clock. |
| `time.Timer` and `time.Ticker` | the types, their methods (`Stop`, `Reset`) and their field `C` | A timer or ticker handed in still waits on the clock. forbidigo renders `(*time.Timer).Stop` as `time.Timer.Stop`, so one pattern, `^time\.(?:Timer\|Ticker)(?:\.\w+)?$`, covers the type, each method and `t.C`. |
| `time.LoadLocation` | `time.LoadLocation` | It reads the `ZONEINFO` variable and zoneinfo files. |
| `time.Local` | the variable `time.Local` and the method `Time.Local` | A time in `Local` reads the `TZ` variable and `/etc/localtime` when its fields, zone or text are first read (`initLocal`). |
| `time.Unix*` | `time.Unix`, `UnixMilli`, `UnixMicro` | They return a time in `Local`, as `Time.Local` does. |
| a zone offset matched against `Local` | `time.Parse` and the `Time` methods `UnmarshalJSON`, `UnmarshalText`, `UnmarshalBinary`, `GobDecode` | They match a zone offset or abbreviation in the value against `Local`. |

What stays allowed takes or returns the data, or names its location: `fmt.Errorf`, `Sprint*`,
`Sscan*`, and `Fprint*` and `Fscan*` on a writer or reader passed in (the domain cannot import `os` to
reach a standard stream); `time.ParseInLocation` with `time.UTC` or a `*time.Location` passed in,
`time.FixedZone`, `time.Date`, and the `Time` methods that read no zone, such as `Unix` (an `int64`),
`UTC`, `In` and `Format`. Each entry's msg says what to use instead.

### How the domain rule is pinned

`TestDomainIsPureProbe` runs forbidigo, with the same harness as `TestCloudReadOnlyProbe`, on a probe
domain package (`test/lint/testdata/domainispure`). It has a line for every name an entry's pattern
lists, each marked `// want "<msg prefix>"` with the start of its own entry's msg, so narrowing a
pattern fails the probe, and lines for a method value (`_ = t.Local`), a func value
(`f := time.Now`), a method expression (`time.Time.Local`) and an aliased import. The pure calls, a
domain function named `println`, and the refused calls in a domain test and a CLI file must pass.
`TestDomainIsPurePatterns` checks the patterns, offline, against lists of refused and allowed names.
`TestForbidigoExclusionsDropOneGroup` pins the exclusion rules of every group: every rule that drops
forbidigo issues names forbidigo alone with a text of exactly `because "<tag>: [^"]*"$`, the
cloud-read-only group has none, and the domain-is-pure rules are exactly `path-except: ^internal/domain/`
and `path: _test\.go$`. A new group adds its tag and states its rules, even none. A line in the
cloud-read-only probe passes the string `"domain-is-pure: x"` to a refused call, so a text of just the
tag fails the probe.
For a new entry, add its names to both, each on a line of its own in the probe.

### Gaps in the domain rule

forbidigo matches a selector against its static type, so these get through:

- **A call through an interface or a type parameter.** `Local` or an `Unmarshal*` method called on a
  domain interface that a `time.Time` satisfies is matched against the interface, and one called
  through a type parameter has no type name.
- **A method promoted from an embedded `time.Time`**, which is matched against the embedding type.
- **A `time.Time` handed to the domain already in `Local`, or a `*time.Location` handed in that is
  `time.Local`.** Formatting such a time reads `TZ`, and so does building one with `time.Date` or
  `time.ParseInLocation` in that location, or in `t.Location()` of a `Local` time. No call rule can see
  where the value came from. The adapter converts a time with `.UTC()` before handing it in, and passes
  `time.UTC` or a location it loaded, never `time.Local`.
- **A dot import of `time`.** After `import . "time"`, `Local.String()` is not reported, yet it reads
  `TZ` and `/etc/localtime` through `initLocal`. staticcheck's ST1001 refuses dot imports, so this gets
  through only behind a nolint directive that silences it.
- **A receive from a channel handed in.** `<-t.C` on a `time.Timer` or `time.Ticker` is refused through
  the field `C`, but a `<-chan time.Time` passed as a parameter is a plain channel.

## Domain model

| Package | Holds |
|---|---|
| `taxonomy` | Provider-neutral categories (`compute.vm`, `kubernetes.managed`, `storage.object`, `database.postgresql`, `serverless.function`, `messaging.queue`, ...) and the capability vocabulary |
| `inventory` | Resources discovered in a source: category, region, size, capabilities used, dependencies, and the raw provider type for traceability |
| `catalog` | Offerings of a target: categories, capabilities, plans and sizes, EU regions |
| `mapping` | Rules that match a resource to candidate offerings and assign a fit (direct, partial, none) with reasons |
| `assessment` | Per-resource and per-workload verdicts, blockers, required changes, readiness score |
| `text` | Validation and bounded quoting of untrusted free text (IDs, raw types, regions, names); no output-format escaping |

Why a taxonomy: with S sources and T targets, direct service-to-service tables need S×T mappings that
drift independently. With a taxonomy, each provider maps once into a shared vocabulary (S+T), and the
matching rules live in one place.

## Ports (declared in `internal/app/analysis`)

The shapes and their alternatives are in [ADR 0005](adr/0005-context-first-ports-and-fstest-style-contract-suites.md).

| Port | Method | Implemented by | Pinned by |
|---|---|---|---|
| `InventorySource` | `Inventory(ctx) (inventory.Inventory, error)` | `adapter/source/inventoryfile` (the JSON inventory file, [ADR 0006](adr/0006-json-inventory-file-format-v1.md)), `adapter/source/{aws,azure,gcp}` | Contract suite `adapter/source/sourcetest` |
| `CatalogSource` | `Catalog(ctx) (catalog.Catalog, error)` | `adapter/target/{ovhcloud,scaleway,upcloud}`. Scaleway and OVHcloud each serve an embedded, dated snapshot read through the shared reader `adapter/target/snapshot` ([ADR 0007](adr/0007-shared-target-catalog-snapshot-format-v1.md)) | Contract suite `adapter/target/targettest`; the snapshot format once, in `adapter/target/snapshot` |
| `Reporter` | `Report(ctx, w io.Writer, assessment.Assessment, ReportMeta) error` | `adapter/report/{markdown,jsonreport}` | Golden files |

The use case is `analysis.New(inventorySource, catalogSource, reporter)` followed by
`Run(ctx, w, meta)`. Run checks `ReportMeta` (the inventory file's base name must pass `text.Validate`)
before any port is called, loads the inventory and the catalog, assesses, checks the context itself and
renders to `w`. The driving adapter chooses `w` (standard output or a file) and owns it. Blockers are a
result, not an error.

A contract suite exports `Check(ctx, src, want) error`, which returns every violation joined and named
after its check, and `Run(t, src, want)`, which fails the test on a violation. A target adapter declares
its expectations itself: its display name, its EU regions and the categories its catalog must serve.

## Testing strategy

| Level | Scope | Location | Runs in CI |
|---|---|---|---|
| Unit | Domain rules, use cases with fakes | next to the code, `package foo_test` | yes |
| Contract | Behaviour every source or target adapter must share | `adapter/source/sourcetest`, `adapter/target/targettest` | yes |
| Golden | Report and CLI output | `testdata/*.golden` | yes |
| Integration | Real provider APIs, read-only | `//go:build integration` | no (manual, credentials) |
| E2E | The built binary against fixtures | `test/e2e`, `//go:build e2e` | yes |
| Source rules | Rules on the Go source and its packages that golangci-lint cannot express, and the probes that prove the cloud-read-only and domain-is-pure lint rules fire | `test/lint`, `test/architecture` | yes |

Most tests sit in the domain, where they are fast and need no fakes. Adapters are tested against
recorded, sanitised API responses through the contract suites, so adding a provider means passing an
existing suite rather than inventing new expectations.

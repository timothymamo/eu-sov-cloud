# PRD #2: Walking skeleton: assess an inventory file against the Scaleway catalog

**Status:** Complete
**Issue:** #2

## Problem

`shift2eu` has a package layout and nothing else. Every later feature (each source adapter, each target,
sizing, pricing, workload grouping) depends on the domain model and the three ports (inventory source,
catalog source, reporter) that the architecture names but does not define. If the first real feature is
"discover AWS", the ports get shaped around one SDK, the first tests need credentials, and the mapping
rules (the actual value of the project) arrive last.

A platform engineer evaluating a move off a hyperscaler cannot get any answer from the tool today.

## Outcome

A user can run `shift2eu assess --inventory inventory.json --target scaleway` on a hand-written or exported
inventory file and get a deterministic Markdown report that lists, for every resource, its taxonomy
category, the candidate Scaleway offerings, the fit (direct, partial, none), the reasons for that fit, and
a summary of blockers. The whole path runs offline, with no credentials, and every layer exists and is
tested: taxonomy, inventory, catalog, mapping and assessment in the domain; the analysis use case and its
ports; one adapter for each port; the CLI; an e2e test of the built binary.

## Scope

| | In scope | Out of scope |
|---|---|---|
| Source providers | None live. Inventory is read from a versioned JSON inventory file (an inventory-source adapter). Resources in the file may carry an AWS, Azure or GCP raw type for traceability. | Live discovery from AWS, Azure or GCP (one PRD each, next) |
| Target providers | Scaleway, from an embedded, dated catalog snapshot | OVHcloud, UpCloud (one PRD each); live catalog or pricing APIs |
| Taxonomy categories | `compute.vm`, `kubernetes.managed`, `storage.object`, `storage.block`, `database.postgresql`, `database.mysql`, `serverless.function`, `messaging.queue`, plus `unclassified` | Further categories; sub-categories or a category hierarchy |
| Capabilities | An initial vocabulary large enough for the in-scope categories (for example `ipv6`, `point-in-time-restore`, `object-versioning`, `high-availability`, `read-replicas`) | Capability versions or parameters (for example "PostgreSQL 16") |
| Assessment | Fit per resource with reasons; blockers; required changes; summary counts per fit | Readiness score, sizing and plan matching, cost, workload grouping, dependency analysis |
| Output | Markdown report to stdout or `--output <path>` | JSON report (`jsonreport`, next PRD), HTML, interactive output |

## Acceptance criteria

All criteria are pinned by tests that run offline under `make test` or `make test-e2e`.

**Inventory input**

- [x] Given a valid inventory file, when I run `shift2eu assess --inventory <file> --target scaleway`, then the
  command exits 0 and prints a Markdown report to stdout.
- [x] Given `--output report.md`, then the report is written to that file and nothing but diagnostics goes
  to stdout or stderr.
- [x] Given a file that does not exist, is not valid JSON, or declares an unsupported `schemaVersion`, then
  the command exits 1 and prints one error line to stderr naming the file and the problem. It never panics
  or prints a stack trace.
- [x] Given a resource whose category is not in the taxonomy (for example a typo such as `compute.mv`),
  then the command exits 1 and the error names the resource ID and the unknown category. Unknown categories
  are never silently treated as `unclassified`.
- [x] Given a resource explicitly marked `unclassified`, then it appears in the report with fit `none` and
  a blocker "not classified", and is never dropped.

**Mapping**

- [x] Given a resource whose category the catalog serves and whose capabilities are all offered by a
  candidate offering, then its fit is `direct` and the offering is named.
- [x] Given a resource whose category is served but which uses a capability no candidate offers, then its
  fit is `partial` and the report lists each missing capability as a required change.
- [x] Given a resource whose category no offering in the catalog serves, then its fit is `none` with the
  reason "no offering in the Scaleway catalog for `<category>`", and it is listed as a blocker.
- [x] Given several candidate offerings, then the best fit is reported first: direct before partial,
  partial candidates missing fewer capabilities first, remaining ties broken by offering name.

**Report**

- [x] The report states the target, the catalog snapshot's source URL and retrieval date, and the
  inventory file's name (not its full path).
- [x] The report contains a summary with the number of resources per fit, a per-resource table (ID,
  raw type, category, fit, best offering, reasons) and a blockers section.
- [x] Given the same inventory twice, the two reports are byte-identical. Resources are ordered by ID,
  never by map or file order.
- [x] Every value taken from the inventory or catalog (IDs, raw types, regions, offering names), the
  target name, the snapshot source URL and the inventory file name is rendered as an inline code span
  (fence longer than any backtick run, `|` escaped in table cells), so `|`, newlines, backticks, HTML,
  link syntax and GitHub's filters (math, mentions, references, emoji, task lists, bare links) cannot
  forge rows, headings, links or images or rewrite it; the source URL is the stored string, never
  decoded. Pinned by a golden test with a hostile fixture that includes a hostile file name and source
  URL; the hostile fixture's source URL includes `%0A` and `%E2%80%AE`. The inventory file name must
  pass `text.Validate`; otherwise the command exits 1 with an error that shows the name through
  `text.Quote`.

**CLI**

- [x] `shift2eu assess` with a missing `--inventory`, a missing or unknown `--target`, or an unknown flag exits
  2 and prints usage to stderr. Unknown targets list the supported ones.
- [x] `shift2eu version` keeps working unchanged.
- [x] `shift2eu assess` needs no credentials or network: the e2e test runs the binary with an empty
  environment and gets the same report. No package on the `assess` path imports a cloud SDK or `net/http`.

**Architecture**

- [x] `make check` passes, including depguard: the domain imports only the standard library, the
  application imports only the domain, and adapters are constructed only in `cmd/shift2eu`.
- [x] The inventory-file adapter passes the contract suite in `internal/adapter/source/sourcetest`, and
  the Scaleway adapter passes the suite in `internal/adapter/target/targettest`. Both suites exist and are
  written so the future AWS, Azure, GCP, OVHcloud and UpCloud adapters can run them unchanged.

## Milestones

Each milestone ships as one PR. Milestones 4, 5 and 6 depend only on 3 and can run in parallel.

1. [x] **Domain models.** `taxonomy` (in-scope categories, `unclassified`, capability vocabulary, parsing
   that rejects unknown values), `inventory` (resource: ID, raw provider type, category, region,
   capabilities used), `catalog` (target, offerings with categories, capabilities and EU regions, snapshot
   source and date). Unit tests only.
2. [x] **Mapping and assessment.** Fit rules (direct, partial, none) with reasons, candidate ordering,
   per-resource verdicts, blockers, required changes and summary counts. Table-driven unit tests covering
   every acceptance criterion under *Mapping*.
3. [x] **Use case and ports.** `internal/app/analysis` declares the inventory-source, catalog-source and
   reporter ports and runs load, map, assess, report. Tested with fakes. Empty contract suites
   `sourcetest` and `targettest` with their first checks. ADR 0005 records the port shapes. The reporter
   port receives report metadata alongside the assessment, including the inventory file name (checked
   with `text.Validate`).
4. [x] **Inventory-file source adapter.** Reads the versioned JSON inventory format, validates it, passes
   `sourcetest`. Sanitised fixtures under `testdata/`. ADR 0006 records the file format, because it becomes
   the interchange format between future `discover` and `assess` commands. Caps the inventory file size
   before decoding. Caps the number of resources as well as the file size (each resource is mapped
   against every candidate offering). The adapter's errors name the file through `text.Quote(path)` and
   never pass `*os.PathError` or decoder text through unquoted.
5. [x] **Scaleway catalog adapter.** Embedded, dated catalog snapshot (`//go:embed`) covering at least
   `compute.vm`, `kubernetes.managed`, `storage.object` and `database.postgresql`, with its source URL and
   retrieval date in the file header. Passes `targettest`, plus a golden test of the parsed catalog.
   The reviewer checks the adapter's declared EU region list against Scaleway's published regions.
6. [x] **Markdown reporter.** Renders an assessment as described under *Report*. Golden-file tests.
   Includes a golden test with a hostile inventory fixture (Markdown metacharacters in IDs and raw types).
   Renders reasons from `mapping.Reason.Parts()`, escaping every part and building code spans for
   category and capability parts safely (a fence longer than any backtick run in the value), never from
   `String()` or by passing domain values to fmt/templates as Stringers. Escapes the inventory source too.
   The hostile fixture includes a catalog whose target and offering names contain `|`, a backtick, `<`,
   `[`, `](` and `*`. Regions are not a table column; offering regions appear only if a later milestone
   adds them, escaped like every other value. Defines how an empty inventory name renders (the line is
   omitted).
7. [x] **CLI and wiring.** `shift2eu assess` in `internal/cli`, adapters wired in `cmd/shift2eu`, exit codes,
   e2e test in `test/e2e` against a fixture inventory that exercises direct, partial, none and
   unclassified. README gains a usage section. Moves this PRD to `prds/done/` and closes #2. The CLI
   prints every error as `shift2eu: <err>` through one sanitiser (control, format, Zl/Zp characters
   escaped, one line, capped at 1 KiB). `--output` is written atomically (temp file in the target
   directory, mode 0600, rename on success, removed on failure; the destination is never truncated
   before success). A test runs `go list -deps` on the assess path and asserts no net/http, os/exec or
   cloud SDK.

## Inventory file format (illustrative)

The exact shape is decided in milestone 4 and recorded in ADR 0006. Illustrative only, so tests and
fixtures in earlier milestones share a picture of the data:

```json
{
  "schemaVersion": 1,
  "source": "aws",
  "resources": [
    {
      "id": "i-0123456789abcdef0",
      "rawType": "AWS::EC2::Instance",
      "category": "compute.vm",
      "region": "eu-west-1",
      "capabilities": ["ipv6"]
    },
    {
      "id": "db-example",
      "rawType": "AWS::RDS::DBInstance",
      "category": "database.postgresql",
      "region": "eu-west-1",
      "capabilities": ["point-in-time-restore", "read-replicas"]
    }
  ]
}
```

## Decisions and open questions

- **Decided: start offline, from a file, not from AWS.** It lets the ports be designed without an SDK
  pulling on their shape, keeps every test free of credentials and network, and puts the mapping rules
  first. The file format also becomes the hand-off between a future `shift2eu discover` and `shift2eu assess`,
  so users can review, sanitise or edit an inventory before assessing it.
- **Decided: one target.** One adapter is enough to pin the catalog port and its contract suite.
  Scaleway is first; OVHcloud and UpCloud follow in their own PRDs and must pass the same suite.
- **Decided: the CLI uses the standard library `flag` package** (CLAUDE.md rule 6). Revisit with an ADR
  if subcommands outgrow it.
- **Decided: exit codes.** 0 when the assessment ran (blockers are a result, not a failure), 1 for input
  or runtime errors, 2 for usage errors, matching the existing `cmd/shift2eu` behaviour.
- **Accepted risk: look-alike text.** The domain rejects control, invisible and format characters, edge
  white space and non-ASCII spaces in IDs, and compares IDs and offering names ignoring runs of white
  space. Homoglyphs (e.g. Cyrillic а for Latin a) and NFC/NFD differences are not detected: the standard
  library has no normaliser, and real cloud IDs are ASCII. Revisit if hand-edited inventories become
  common.
- **Open: readiness score.** Deferred until sizing and workload grouping exist, because a score from fit
  counts alone would mislead. Next candidate PRD.
- **Open: region semantics.** This PRD only reports the EU regions where an offering exists. Whether a
  resource can require a specific country (data residency) is a later decision.
- **Open: capability granularity.** Whether capabilities need versions or parameters (engine version,
  storage class) is left until the first live source adapter shows what data is available.
- **Decided: `--inventory` is a path; standard input is not supported.** ADR 0006 left `-` for standard
  input to milestone 7. The walking skeleton does without it: every inventory is a file the
  inventory-file adapter reads once per run. Supporting it later means buffering the input once in the
  CLI (ADR 0005).
- **Open: tie-break between equally direct offerings.** When several offerings are direct with nothing
  missing (e.g. Instances and Elastic Metal for a plain VM), the best offering is the first by name, and
  the report names the tied offerings next to it ("`Elastic Metal` (tie: `Instances`)"). A
  product-preference order per category may read better in reports; revisit with the readiness score.
  **Closed by [ADR 0023](../../docs/adr/0023-separate-bare-metal-servers-from-virtual-machines-in-the-taxonomy.md)
  (issue #187):** bare-metal servers moved to `compute.baremetal`, so Elastic Metal no longer ties with
  Instances; the name tie-break stays, and only ties between tiers of one product remain.
- **Accepted: Required changes for a partial tie.** When several offerings are partial with the same
  number of missing capabilities but different ones, Required changes lists only the best offering's
  gaps, not those of the tied offerings. Accepted for now: the case is rare, and it understates the
  options rather than overstating portability.
- **Note: fit `none` by "no offering" is pinned at the mapping and report levels, not end to end.** The
  current Scaleway snapshot serves every in-scope category, so no inventory can reach that verdict end
  to end. The mapping and Markdown report tests pin it, and the e2e fixture covers `none` through
  `unclassified` instead. A target without an offering for some category (OVHcloud for
  `serverless.function`, PRD #11) will make it reachable.
- **Accepted: long inventory paths in errors.** `text.Quote` caps the inventory path at 128 runes in
  adapter errors, so a very long path can lose its base name; the report always shows the base name, and
  the path is still recognisable from its start. Revisit with the shared decoder (#13).
  Resolved in issue #13: the adapter quotes the path with `text.QuotePath`, which keeps the first 32
  and the last 96 runes of a longer path, so the base name survives.
- **Decided: what `--output` may name.** The report is rendered into memory in full; only then is a
  temporary file created next to the destination, written, synced and renamed onto it, so a failed or
  cancelled run leaves no partial report and no temporary file. A path that does not exist is created,
  an existing regular file is replaced atomically, and a symbolic link to a regular file, or a dangling
  link, is replaced, not followed. Anything else (a directory, a named pipe, a socket, a device, or a
  symbolic link to one of them) and the inventory file itself (the same path, a hard link or a symbolic
  link to it) are refused with exit code 1 before the inventory is read, and left as they were, so that,
  run as root, `--output /dev/stdout` cannot replace the system's `/dev/stdout` link with a regular file. SIGINT, SIGTERM and SIGHUP cancel a run, which then writes nothing.

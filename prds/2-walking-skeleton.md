# PRD #2: Walking skeleton: assess an inventory file against the Scaleway catalog

**Status:** In progress
**Issue:** #2

## Problem

`eusov` has a package layout and nothing else. Every later feature (each source adapter, each target,
sizing, pricing, workload grouping) depends on the domain model and the three ports (inventory source,
catalog source, reporter) that the architecture names but does not define. If the first real feature is
"discover AWS", the ports get shaped around one SDK, the first tests need credentials, and the mapping
rules (the actual value of the project) arrive last.

A platform engineer evaluating a move off a hyperscaler cannot get any answer from the tool today.

## Outcome

A user can run `eusov assess --inventory inventory.json --target scaleway` on a hand-written or exported
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

- [ ] Given a valid inventory file, when I run `eusov assess --inventory <file> --target scaleway`, then the
  command exits 0 and prints a Markdown report to stdout.
- [ ] Given `--output report.md`, then the report is written to that file and nothing but diagnostics goes
  to stdout or stderr.
- [ ] Given a file that does not exist, is not valid JSON, or declares an unsupported `schemaVersion`, then
  the command exits 1 and prints one error line to stderr naming the file and the problem. It never panics
  or prints a stack trace.
- [ ] Given a resource whose category is not in the taxonomy (for example a typo such as `compute.mv`),
  then the command exits 1 and the error names the resource ID and the unknown category. Unknown categories
  are never silently treated as `unclassified`.
- [ ] Given a resource explicitly marked `unclassified`, then it appears in the report with fit `none` and
  a blocker "not classified", and is never dropped.

**Mapping**

- [ ] Given a resource whose category the catalog serves and whose capabilities are all offered by a
  candidate offering, then its fit is `direct` and the offering is named.
- [ ] Given a resource whose category is served but which uses a capability no candidate offers, then its
  fit is `partial` and the report lists each missing capability as a required change.
- [ ] Given a resource whose category no offering in the catalog serves, then its fit is `none` with the
  reason "no offering in the Scaleway catalog for `<category>`", and it is listed as a blocker.
- [ ] Given several candidate offerings, then the best fit is reported first: direct before partial,
  partial candidates missing fewer capabilities first, remaining ties broken by offering name.

**Report**

- [ ] The report states the target, the catalog snapshot's source URL and retrieval date, and the
  inventory file's name (not its full path).
- [ ] The report contains a summary with the number of resources per fit, a per-resource table (ID,
  raw type, category, fit, best offering, reasons) and a blockers section.
- [ ] Given the same inventory twice, the two reports are byte-identical. Resources are ordered by ID,
  never by map or file order.
- [ ] Every value taken from the inventory or catalog (IDs, raw types, regions, offering names, the
  target name and the snapshot source URL), and the inventory file name, is escaped for its Markdown
  context, so `|`, newlines, backticks, HTML and link syntax in an inventory cannot forge table rows,
  headings, links or images. The source URL is emitted percent-encoding `( ) [ ] < > |`, space and
  backtick (or as an autolink after encoding `<` `>`). Pinned by a golden test with a hostile fixture
  that includes a hostile file name and source URL. The inventory file name must pass `text.Validate`;
  otherwise the command exits 1 with an error that shows the name through `text.Quote`. The reporter
  emits the stored source URL string, never a decoded form; the hostile fixture's source URL includes
  `%0A` and `%E2%80%AE`.

**CLI**

- [ ] `eusov assess` with a missing `--inventory`, a missing or unknown `--target`, or an unknown flag exits
  2 and prints usage to stderr. Unknown targets list the supported ones.
- [ ] `eusov version` keeps working unchanged.
- [ ] `eusov assess` needs no credentials or network: the e2e test runs the binary with an empty
  environment and gets the same report. No package on the `assess` path imports a cloud SDK or `net/http`.

**Architecture**

- [ ] `make check` passes, including depguard: the domain imports only the standard library, the
  application imports only the domain, and adapters are constructed only in `cmd/eusov`.
- [ ] The inventory-file adapter passes the contract suite in `internal/adapter/source/sourcetest`, and
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
3. [ ] **Use case and ports.** `internal/app/analysis` declares the inventory-source, catalog-source and
   reporter ports and runs load, map, assess, report. Tested with fakes. Empty contract suites
   `sourcetest` and `targettest` with their first checks. ADR 0005 records the port shapes. The reporter
   port receives report metadata alongside the assessment, including the inventory file name (checked
   with `text.Validate`).
4. [ ] **Inventory-file source adapter.** Reads the versioned JSON inventory format, validates it, passes
   `sourcetest`. Sanitised fixtures under `testdata/`. ADR 0006 records the file format, because it becomes
   the interchange format between future `discover` and `assess` commands. Caps the inventory file size
   before decoding. Caps the number of resources as well as the file size (each resource is mapped
   against every candidate offering).
5. [ ] **Scaleway catalog adapter.** Embedded, dated catalog snapshot (`//go:embed`) covering at least
   `compute.vm`, `kubernetes.managed`, `storage.object` and `database.postgresql`, with its source URL and
   retrieval date in the file header. Passes `targettest`, plus a golden test of the parsed catalog.
6. [ ] **Markdown reporter.** Renders an assessment as described under *Report*. Golden-file tests.
   Includes a golden test with a hostile inventory fixture (Markdown metacharacters in IDs and raw types).
   Renders reasons from `mapping.Reason.Parts()`, escaping every part and building code spans for
   category and capability parts safely (a fence longer than any backtick run in the value), never from
   `String()` or by passing domain values to fmt/templates as Stringers. Escapes the inventory source too.
   The hostile fixture includes a catalog whose target and offering names contain `|`, a backtick, `<`,
   `[`, `](` and `*`. Regions are not a table column; offering regions appear only if a later milestone
   adds them, escaped like every other value.
7. [ ] **CLI and wiring.** `eusov assess` in `internal/cli`, adapters wired in `cmd/eusov`, exit codes,
   e2e test in `test/e2e` against a fixture inventory that exercises direct, partial, none and
   unclassified. README gains a usage section. Moves this PRD to `prds/done/` and closes #2.

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
  first. The file format also becomes the hand-off between a future `eusov discover` and `eusov assess`,
  so users can review, sanitise or edit an inventory before assessing it.
- **Decided: one target.** One adapter is enough to pin the catalog port and its contract suite.
  Scaleway is first; OVHcloud and UpCloud follow in their own PRDs and must pass the same suite.
- **Decided: the CLI uses the standard library `flag` package** (CLAUDE.md rule 6). Revisit with an ADR
  if subcommands outgrow it.
- **Decided: exit codes.** 0 when the assessment ran (blockers are a result, not a failure), 1 for input
  or runtime errors, 2 for usage errors, matching the existing `cmd/eusov` behaviour.
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

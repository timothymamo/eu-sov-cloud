---
name: add-cloud-provider
description: Add or extend a cloud provider adapter in eu-sov-cloud - a source hyperscaler (AWS, Azure, GCP, others) that is discovered, or a target EU provider (OVHcloud, Scaleway, UpCloud, others) whose catalog is compared. Use when adding a provider, adding a service or resource type to an existing provider, or changing how a provider's services are classified.
---

# Add or extend a cloud provider

Providers meet only through the **taxonomy** (`internal/domain/taxonomy`). A source adapter classifies
what it discovers into taxonomy categories; a target adapter declares which categories its offerings
serve. Never write a direct "AWS service X maps to OVH service Y" table.

## Source provider (hyperscaler, read-only discovery)

1. Package: `internal/adapter/source/<provider>`, implementing the inventory-source port declared in
   `internal/app/analysis`.
2. Use only read APIs (`Describe*`, `List*`, `Get*`). Document the minimal read-only role or policy in
   the package's `doc.go`.
3. Translate SDK types into `internal/domain/inventory` types inside the adapter. SDK types never escape.
4. Classify each resource into a taxonomy category. Unknown resource types are kept and marked
   unclassified, never dropped silently, so the assessment can report them.
5. Record provider-specific features a resource depends on (for example DynamoDB streams or Lambda@Edge)
   as capabilities, because they decide whether a target is a direct or a partial fit.
6. Tests: run the shared contract suite from `internal/adapter/source/sourcetest` against recorded,
   sanitised API responses in the package's `testdata/`. Live calls only under `//go:build integration`.

## Target provider (EU cloud catalog)

1. Package: `internal/adapter/target/<provider>`, implementing the catalog-source port.
2. Catalog data comes from the provider's public catalog or pricing API, or from an embedded, dated data
   file (`//go:embed`) with its source URL and retrieval date in the file header.
3. Every offering declares its taxonomy categories, capabilities, sizes and the EU regions it is offered in.
4. Tests: the shared suite in `internal/adapter/target/targettest`, plus a golden test of the parsed catalog.

## New taxonomy category

If nothing fits, add a category in `internal/domain/taxonomy` first (separate commit, with a unit test),
then classify into it from both sides. Write an ADR (adr skill) if it changes how categories are structured.

## Import rules (source and target)

A new source or target adapter gets its own strict depguard rule in `.golangci.yml`, modelled on
`embedded-snapshot-targets-stay-offline` and `reporters-stay-offline`: `list-mode: strict`, `files`
set to the adapter's directory (`"**/internal/adapter/<kind>/<provider>/**"`) and `"!$test"`, and an
allow list naming exactly what the adapter imports, each entry ending in `$` so it covers that package
only (for a live source, that includes its SDK packages). Add deny entries only to say why a package is
refused. Any new helper under `internal/adapter/internal/` must be named, ending in `$`, in the allow
list of `adapters-are-built-in-cmd` before an adapter can import it, and `adapter-helpers-do-no-io`
holds it to no I/O. Run `make lint` to confirm the rules hold.

## Finish

Wire the adapter in `cmd/eusov`, update the provider tables in README.md, and run `make check`.

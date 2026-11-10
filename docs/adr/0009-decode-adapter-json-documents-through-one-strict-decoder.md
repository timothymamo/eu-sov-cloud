# 0009. Decode adapter JSON documents through one strict decoder

- Status: Accepted
- Date: 2026-11-10

## Context

Two adapters read a JSON document that the project defines: the inventory-file reader (ADR 0006) and
the target catalog snapshot reader (ADR 0007). Both read strictly, with the same rules: no byte order
mark, valid UTF-8, one JSON object, exact keys, no repeated key, the right JSON type, no escaped lone
surrogate, errors placed by line and column. ADR 0007 kept them as two token walkers ("one reader, not
one decoder yet") and recorded the cost: a fix to one walker has to be checked against the other.
They had drifted already: the snapshot reader placed a malformed multi-byte character at its first
byte, the inventory-file reader at its last.

Issue #13 asks for one decoder, and gathers hardening from the PRD #2 audits and reviews that the same
change should settle:

- a declared region of a snapshot is free text, so a Cyrillic homoglyph (`fr-раr`), a case variant
  (`FR-PAR`) or a no-break space can pose as a real region;
- a catalog source URL can hold an HTML character reference (`&#38;`) or a percent escape that decodes
  to a control or format character (`%0A`, `%E2%80%AE`), which a renderer would show as something
  other than the URL the catalog holds;
- whether a source URL with a query string is refused or dropped once catalogs can be fetched;
- whether the declared EU regions should move from each snapshot into `catalog.Catalog`;
- `text.Quote` keeps the first 128 runes of a value, so an error about a deeply nested inventory file
  loses the file's base name.

Forces:

- **Shared rules in one place.** A rule written twice drifts; a rule written once is reviewed and
  tested once, and a third JSON format gets it for free.
- **Formats still differ.** The inventory format reads `null` as an absent value for its optional
  fields; the snapshot format refuses `null` everywhere. The inventory format caps resources and
  capabilities; the snapshot format caps its size. Each has its own keys, its own sentinels and its own
  checks after the walk. A shared decoder must not take those decisions away.
- **Errors.** ADR 0005, 0006 and 0007 want one-line errors that wrap the format's sentinels, show
  document values only through `text.Quote` and never pass on the text of `encoding/json`.
- **Layering and dependencies.** The decoder is adapter code (ADR 0001): the domain holds no encoding.
  CLAUDE.md rule 6 prefers the standard library.

## Decision

Every adapter that reads a JSON document decodes it through one package,
`internal/adapter/internal/jsondoc`. Go's internal rule keeps it to packages under
`internal/adapter`, and `adapters-are-built-in-cmd` already allows `internal/adapter/internal/`.

**What jsondoc enforces.** The API is small and drives no reflection:

- `Open(data, maxBytes, invalid) (Document, error)` checks the document as a whole, in this order: at
  most `maxBytes` bytes (the cap is the format's); no byte order mark, UTF-8 or UTF-16, refused by
  name; valid UTF-8; one JSON value with nothing after it but white space ("unexpected data after the
  first JSON value"); and that value an object.
- `Document.Lookup(key)` returns the JSON text of the first top-level key spelt exactly `key`, and the
  first key before it that equals `key` ignoring case. A format reads its schema version with it,
  before any other key or value is checked.
- `Document.Walker()` reads the document token by token. `Walker.Object(keys, value)` reads an object
  whose keys the format lists as `Key{Name, Required}` and calls `value` for each key, with the walker
  at the key's value; `Walker.Array(want, element)` calls `element` for each element;
  `Walker.String`, `Walker.Null` and `Walker.Raw` read one value. The walker refuses a key that is not
  character for character one the format lists (with a hint when it equals one ignoring case), a
  repeated key, a value of the wrong JSON type (`null` included, unless the format calls `Null`
  first), an object without a `Required` key, and a string, key or value, that escapes a lone
  surrogate. It holds one token at a time and keeps the path of the value it reads next, so a format
  builds its values as it reads and refuses a list at its first element past a cap before reading it.
- `Describe(raw)` names a raw JSON value for an error: an integer as written, any other number and a
  string through `text.Quote`, and anything else by its type.

**Errors.** Every error about the document wraps the sentinel the format passes to `Open`
(`inventoryfile.ErrDecode`, `snapshot.ErrDecode`), and an error a callback returns is passed on as it
is, so a format still reports `ErrSchemaVersion` or `ErrTooManyResources` through the walk. A
document error is one line and worded once for every format: it speaks of "the document", never names
the input (the format adds that), places the problem by line and column (both from 1, the column in
characters, a malformed multi-byte character at its first byte) unless it speaks of the whole
document, names a value by its path (`resources[1].capabilities[0]`), shows document values only
through `text.Quote`, and never passes on the text of `encoding/json`. A mistake in the format's use
of the package (a callback that returns without reading its value or reads it a second time, a list
of keys longer than `MaxKeys` or holding a name twice, a nil sentinel, the zero `Document`) is an error
that starts with `jsondoc:` and wraps `jsondoc.ErrMisuse`, never the format's sentinel, so it is never
blamed on the document. The package returns it; it does not panic.

**What each format decides.**

| Decision | Inventory file (ADR 0006) | Catalog snapshot (ADR 0007) |
|---|---|---|
| Keys | its own, at the top level and in a resource | its own, at the top level and in an offering |
| Required keys | `resources`; the version is checked first, and an absent `source`, `id` or `category` reaches the domain as empty | every key |
| `null` | an absent value for every field but `resources`; never an element of `resources` or `capabilities` | never a value |
| Caps and counts | `MaxFileSize` (also enforced while reading the file, `ErrTooLarge`), `MaxResources` (`ErrTooManyResources`), `MaxCapabilities` | `MaxDocumentBytes` |
| Read first | `schemaVersion`, exactly the integer 1 (`ErrSchemaVersion`) | the same |
| After the walk | `inventory.New` | the date, the declared regions, undeclared offering regions, `catalog.New` |
| Sentinels and wording | its own, for its own checks | its own, for its own checks |

**Lint rules.** `.golangci.yml` holds the decoder and the readers to this decision:

- `jsondoc-parses-bytes` (new) limits jsondoc's non-test files to the standard parsing packages it uses
  and `internal/domain/text`, so it opens no file and makes no network call.
- `inventory-file-reader-decodes-through-jsondoc` (new) denies `encoding/json`, and with it
  `encoding/json/v2` and `encoding/json/jsontext`, to the inventory-file reader's non-test files.
- `target-snapshot-reader-parses-bytes` lets the snapshot reader import jsondoc, and drops the parsing
  packages it no longer imports, `encoding/json` among them.
- `target-adapters-read-snapshots-through-the-reader` loses its exception for the reader: with the
  reader decoding through jsondoc, no non-test file under `internal/adapter/target` imports
  `encoding/json`.

**Declared region codes.** The snapshot format adds a rule for the values of its top-level `regions`
list: each is a region code, ASCII letters and digits in groups joined by single hyphens
(`^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$`; `fr-par`, `GRA`, `eu-central-1`), and no two are equal ignoring ASCII
case. Upper case is allowed, because OVHcloud names its regions `GRA` and `RBX`. The checks ADR 0007
lists keep their order and their messages: a region that is blank or not valid text is refused for
that, before its shape is checked, and a region spelt exactly like an earlier one is still "declared
more than once"; one that differs from an earlier one only in ASCII case is refused as the same
region declared again. The declared regions are held in a map keyed by the code in ASCII
lower case, so the check stays linear for the 60,000 regions `BenchmarkParseRepeatedRegionMention`
declares. An offering region still has to be a declared region spelt as declared. The format stays
**schema version 1**: both embedded snapshots already comply, and every version 1 snapshot is embedded
in this repository and reviewed there; none exists outside it, so tightening a value rule breaks no
document. ADR 0007's version policy is about documents in the wild, of which there are none yet.

**Source URLs.** As defence in depth for a reader who sees a source URL rendered, `catalog.New` also
refuses, after the checks it already makes (user info first, so nothing that could be a password is
described):

- an ampersand that starts the shape of an HTML character reference: `&`, an ASCII letter, ASCII
  letters or digits and `;` (`&amp;`, `&lt1;`); `&#`, decimal digits and `;`; `&#x` or `&#X`,
  hexadecimal digits and `;`. A bare `&` between query parameters stays allowed;
- percent escapes that, all decoded together as bytes (so a character spread over several escapes
  decodes as one), fail `text.Validate`: a byte that is not UTF-8 (`%FF`), a control or format character
  (`%0A`, `%00`, `%E2%80%AE`), or white space at the end of the URL.

Both errors say what was found and at which byte of the URL, and never show the URL or a part of it.

**Query strings in fetched catalogs.** When catalogs can be fetched, a source URL with a query string
is **refused**, never dropped. Dropping it would silently change the evidence the report cites, which
ADR 0006 and ADR 0007 refuse on principle, and a query can carry tracking or session tokens that do not
belong in a report. The fetching adapter enforces this when it is built. Embedded snapshots, which are
reviewed, keep today's rule: a query and a fragment are allowed. Nothing changes in code now.

**Declared regions stay out of `catalog.Catalog`.** Issue #13 asked to consider moving the declared EU
region list into the domain. It is not done:

- nothing in the domain or the application reads the declared list (only target adapter tests and
  `targettest` do), so moving it would add a required field to `catalog.New` and every call site
  without changing any verdict;
- the declared list is a statement about a snapshot's evidence ("checked for these regions and no
  others"), which the snapshot format owns;
- every declared region already enters through the shared reader, which now enforces the ASCII code
  shape, so the homoglyph, case-variant and no-break-space concern of issue #13 is met without a domain
  change.

Revisit when a domain or application consumer needs the declared regions, for example a report section
listing a target's regions, or when a target arrives that is not served from a snapshot.

**Long paths in errors.** `text.QuotePath` quotes a path of at most 128 runes exactly as `text.Quote`
does, and a longer one as its first 32 and its last 96 runes, each quoted, with the original byte
length. The inventory-file reader names its file with it, so the base name of a deep path survives.

## Alternatives considered

- **Keep two walkers** (ADR 0007's interim). They had drifted once already, and every new rule, such
  as the multi-byte column, would be written, tested and reviewed twice.
- **A generic decoder driven by struct tags and reflection.** `encoding/json` with
  `DisallowUnknownFields` still matches keys ignoring case and keeps the last of a repeated key, and a
  reflection decoder of our own would be larger than the walk, hide each format's null policy in tags
  and decode before it can count. A walker that the format drives keeps every decision in plain code
  next to the format.
- **`encoding/json/v2`.** It is case-sensitive and refuses repeated keys, but needs `GOEXPERIMENT` in
  Go 1.27, as ADR 0006 records. Revisit when it is stable.
- **jsondoc returns typed errors and each format words them.** This would keep each reader's words for
  the shared rules ("the file is empty" against "the document is empty") at the cost of a second
  layer of wording to keep in step. One wording per rule is the point of one decoder, and "the
  document" reads correctly for both formats, which name their input themselves.
- **jsondoc wraps a sentinel of its own, and each reader re-wraps it.** A reader could not tell the
  decoder's errors from its callbacks' own, such as `ErrTooManyResources`, without inspecting types;
  passing the format's sentinel to `Open` keeps both as they are.
- **Lower-case region codes only, as the issue's first comment suggested.** OVHcloud's codes are upper
  case. Comparing codes ignoring ASCII case gives the same protection against case variants.
- **A regular expression for the region code.** A hand-written loop over the bytes is as short, needs
  no `regexp` in the reader and costs nothing per region.
- **Bump the snapshot format to version 2 for the region rule.** No version 1 document exists outside
  this repository, so a new version would add a transition without protecting any reader.
- **Drop a query string from a fetched source URL, or allow it.** Dropping changes the cited evidence
  without a word; allowing it can put tokens in a report.
- **Move the declared regions into `catalog.Catalog` now.** See the decision: no consumer, more call
  sites, and the shape rule already meets the concern.
- **Have the reporter neutralise character references and escapes instead.** The Markdown reporter
  already shows the URL verbatim in a code span; refusing such a URL where the catalog is built covers
  every reporter, including ones not yet written.

## Consequences

- The strict-decoding rules live in jsondoc and are tested there; a third JSON format (a fetched
  catalog, a future `discover` round trip) decodes through it and decides only its own keys, nulls,
  caps and checks.
- This replaces ADR 0007's "one reader, not one decoder yet" section and its consequence that two
  token walkers implement the same rules. ADR 0007's lint rules change as listed above; its other
  decisions stand.
- ADR 0006's format is unchanged. Its reader now builds the inventory as it reads instead of decoding
  the document a second time, so it passes over the file three times (syntax check, version scan,
  walk), not four. Memory still follows the file: a resources list is refused at its first element
  past `MaxResources`, after at most that many resources are built.
- Some inventory-file error wording changed, none of it pinned by a test: the encoding and syntax
  errors now speak of "the document"; a missing `resources` list is a missing required key and a
  `null` one a value of the wrong type, placed by line and column; and a schema version that is not an
  integer is described as the snapshot reader describes it (`the string "1"`, `the number "-0"`, which
  was shown as `0`). A malformed multi-byte character is placed at its first byte.
- A new target whose provider names regions outside the code shape (with spaces, dots or non-ASCII
  letters) declares an ASCII code for each, as ADR 0007 already asks for one code per location.
- A provider page whose URL needs an escape for a control or format character, or holds an entity
  shape, cannot be cited as a source URL; such a snapshot cites another page.
- The fetching adapter, when it comes, must refuse a source URL with a query string, and must read at
  most the format's cap plus one byte before calling `Open` (ADR 0007).

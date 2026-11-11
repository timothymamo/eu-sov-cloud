// Package jsonreport renders an assessment, or a comparison of several targets'
// assessments, as a stable, versioned JSON document for other tools to consume:
// the JSON report format v1 (docs/adr/0018). It implements the
// analysis.Reporter and analysis.ComparisonReporter ports. The CLI writes it
// with --format json, and the HTTP API (PRD #103) serves the same documents.
//
// # Documents
//
// Every document is one JSON object with "schemaVersion": 1 and a "kind":
//
//   - "assessment", from Report: the inventory name, the source, the target and
//     its catalog snapshot, the summary, one resource per verdict and the
//     blockers. Schema: schema/assessment-v1.schema.json.
//   - "comparison", from ReportComparison: the inventory name, the source, one
//     entry per target in the comparison's order (its key and the single
//     document's target, summary, resources and blockers), and the blockers
//     every target shares. Schema: schema/comparison-v1.schema.json.
//
// A resource carries its ID, raw type, category, region, fit, best offering,
// the tied offerings and the required changes, the same ones the Markdown
// report shows (assessment.Verdict.Ties and ChangesToList), and its reasons. A
// reason carries its code, its plain sentence (mapping.Reason.String, no
// Markdown) and the target, category, offering and capability it mentions,
// each only when set.
//
// # Encoding
//
// Documents are built from private structs and encoded with encoding/json,
// never by concatenating strings: two-space indent, one trailing newline. Every
// value is a JSON string or number, so a value holding quotes, backslashes,
// control characters or an HTML end tag stays inside its string. encoding/json
// escapes <, >, &, U+2028 and U+2029 and writes every invalid UTF-8 byte as
// U+FFFD, so a document is always valid JSON and valid UTF-8.
//
// Keys are camelCase and every key the schemas require is always present: a
// string that holds nothing is "", an array that holds nothing is [], never
// null. Order is the domain's: resources and blockers in the assessment's
// order, targets in the comparison's, never map order. A document's bytes
// depend only on the assessment, or the comparison, and the report metadata.
//
// The whole document is encoded before it is written to the io.Writer in one
// call; a write error is returned, wrapped. A comparison of no target is an
// error, and nothing is written.
package jsonreport

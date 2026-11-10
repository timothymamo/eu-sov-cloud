// Package markdown renders an assessment as a human-readable Markdown report. It
// implements the analysis.Reporter port.
//
// # Layout
//
// A report is, in this order:
//
//   - the title, "# Migration assessment: " and the target;
//   - the header lines: the inventory file name (left out when the name is
//     empty), the source, and the catalog snapshot's source URL and retrieval
//     date, as a UTC calendar date;
//   - "## Summary": a table of the number of resources per fit, and in total;
//   - "## Resources": one table row per verdict, in the assessment's order, with
//     the ID, raw type, category, fit, best offering and reasons;
//   - "## Blockers": one list item per blocker, "ID: reason", or "None." when
//     there is none.
//
// The report ends with a single newline. Its bytes depend only on the
// assessment and the report metadata: never on the clock, the environment or
// map iteration.
//
// # Escaping
//
// Every value taken from the assessment or the metadata is untrusted.
//
//   - Text (target, source, IDs, raw types, fits, offering names, the inventory
//     file name and every reason part other than a category or capability) gets
//     a backslash before each of \ ` * _ [ ] ( ) < > | ! ~ # &. Dots are left as
//     they are. A line ending, which no valid value holds, becomes a character
//     reference.
//   - Categories and capabilities are code spans: the fence is one backtick
//     longer than the longest backtick run in the value, the value is padded with
//     a space when it starts or ends with a backtick or a space, and inside a
//     table cell a pipe is written as \|.
//   - The source URL is an autolink of the stored string, never decoded, with
//     ( ) [ ] < > | ` the space and ASCII control characters percent-encoded.
//
// Reasons are rendered from mapping.Reason.Parts, never from String, and joined
// with "; ". Domain values never reach fmt.
package markdown

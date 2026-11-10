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
//     the ID, raw type, category, fit, best offering and reasons. The best
//     offering is followed by "(tie: ...)" naming, in candidate order, the other
//     candidates with the best one's fit and as many missing capabilities;
//   - "## Required changes": one list item per partial verdict with required
//     changes, in the assessment's order, "ID: capabilities (not offered by
//     offering)", or "None." when there is none;
//   - "## Blockers": one list item per blocker, "ID: reason", or "None." when
//     there is none.
//
// The report ends with a single newline. Its bytes depend only on the
// assessment and the report metadata: never on the clock, the environment or
// map iteration.
//
// # Values are code spans
//
// Every value taken from the assessment or the metadata is untrusted: the
// target, the source, the source URL, the inventory file name, IDs, raw types,
// categories, offering names and capabilities, in reasons too. Each is an inline
// code span, the one form every renderer shows literally:
//
//   - Backslash escapes are not enough. GitHub resolves them first and then runs
//     its math, mention, reference, emoji, task-list and bare-link filters over
//     the text; those filters skip code.
//   - An autolink is not enough either. Renderers decode the visible text of
//     <...>, so a stored %E2%80%AE or &#8238; would show as a right-to-left
//     override and %0A as a line break. The source URL is the stored string,
//     never decoded and never percent-encoded.
//
// The fence is one backtick longer than the longest backtick run in the value,
// the value is padded with a space when it starts or ends with a backtick or a
// space, a line ending becomes a space as CommonMark would make it, and inside a
// table cell a pipe is written as \|. An empty value renders as nothing, never
// as an empty code span.
//
// The report's own words are plain text: fits, counts, the date and the literal
// text of reasons. Literal text and fits still get a backslash before each of
// \ ` * _ [ ] ( ) < > | ! ~ # & and have line endings turned into character
// references, so a reason code the domain does not know cannot become syntax.
//
// Reasons are rendered from mapping.Reason.Parts, never from String, and joined
// with "; ". Domain values never reach fmt.
package markdown

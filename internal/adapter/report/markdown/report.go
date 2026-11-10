package markdown

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping"
)

// dateLayout is how a report shows the catalog's retrieval date: a calendar date.
const dateLayout = "2006-01-02"

// reasonSeparator joins the reasons of one resource in its table cell.
const reasonSeparator = "; "

// Reporter renders an assessment as a Markdown report. It holds no state, so
// one Reporter may render any number of reports, concurrently too.
type Reporter struct{}

// New returns a Markdown reporter.
func New() *Reporter {
	return &Reporter{}
}

// Report writes the Markdown report of a and meta to w, laid out as the package
// documentation describes.
//
// The output depends on a and meta only. Report builds the whole report before
// writing it to w in one call, so w never receives a half-rendered report from a
// failure while rendering; a write error is returned, wrapped.
// When ctx is already done Report writes nothing and returns ctx.Err().
func (*Reporter) Report(ctx context.Context, w io.Writer, a assessment.Assessment, meta analysis.ReportMeta) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var b strings.Builder

	writeHeader(&b, a, meta)
	writeSummary(&b, a.Summary)
	writeResources(&b, a.Verdicts)
	writeBlockers(&b, a.Blockers)

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write markdown report: %w", err)
	}

	return nil
}

// writeHeader writes the title and the lines that say what was assessed against
// what. The inventory file line is left out when the inventory has no file name.
func writeHeader(b *strings.Builder, a assessment.Assessment, meta analysis.ReportMeta) {
	target := escapeText(a.Target)

	b.WriteString("# Migration assessment: " + target + "\n\n")

	if meta.InventoryName != "" {
		b.WriteString("- Inventory file: " + escapeText(meta.InventoryName) + "\n")
	}

	b.WriteString("- Source: " + escapeText(a.Source) + "\n")
	b.WriteString("- Catalog: " + target + " snapshot from " + autolink(a.Snapshot.SourceURL) +
		", retrieved " + a.Snapshot.RetrievedOn.UTC().Format(dateLayout) + "\n\n")
}

// writeSummary writes the number of resources per fit, and in total.
func writeSummary(b *strings.Builder, s assessment.Summary) {
	b.WriteString("## Summary\n\n")
	writeRow(b, "Fit", "Resources")
	writeSeparator(b, 2)
	writeRow(b, string(mapping.Direct), strconv.Itoa(s.Direct))
	writeRow(b, string(mapping.Partial), strconv.Itoa(s.Partial))
	writeRow(b, string(mapping.None), strconv.Itoa(s.None))
	writeRow(b, "total", strconv.Itoa(s.Total))
	b.WriteString("\n")
}

// writeResources writes one table row per verdict, in the order given.
func writeResources(b *strings.Builder, verdicts []assessment.Verdict) {
	b.WriteString("## Resources\n\n")
	writeRow(b, "ID", "Raw type", "Category", "Fit", "Best offering", "Reasons")
	writeSeparator(b, 6)

	for _, v := range verdicts {
		writeRow(b,
			escapeText(v.Resource.ID),
			escapeText(v.Resource.RawType),
			codeSpan(string(v.Resource.Category), true),
			escapeText(string(v.Fit)),
			escapeText(v.BestOffering),
			renderReasons(v.Reasons, true),
		)
	}

	b.WriteString("\n")
}

// writeBlockers writes one list item per blocker, in the order given, or "None."
// when there is none. It ends the report, so it ends with a single newline.
func writeBlockers(b *strings.Builder, blockers []assessment.Blocker) {
	b.WriteString("## Blockers\n\n")

	if len(blockers) == 0 {
		b.WriteString("None.\n")

		return
	}

	for _, blocker := range blockers {
		b.WriteString("- " + escapeText(blocker.ResourceID) + ": " + renderReason(blocker.Reason, false) + "\n")
	}
}

// writeRow writes one table row of cells that are already escaped for a table.
func writeRow(b *strings.Builder, cells ...string) {
	b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
}

// writeSeparator writes the row that separates a table's header from its body.
func writeSeparator(b *strings.Builder, columns int) {
	b.WriteString("|" + strings.Repeat("---|", columns) + "\n")
}

// renderReasons renders reasons one after the other, separated by "; ".
func renderReasons(reasons []mapping.Reason, inTable bool) string {
	rendered := make([]string, 0, len(reasons))
	for _, r := range reasons {
		rendered = append(rendered, renderReason(r, inTable))
	}

	return strings.Join(rendered, reasonSeparator)
}

// renderReason renders a reason from its parts: categories and capabilities as
// code spans, every other part, literal sentence text included, as escaped text.
// It never uses the reason's String method, which is not safe for Markdown.
func renderReason(r mapping.Reason, inTable bool) string {
	var b strings.Builder

	for _, p := range r.Parts() {
		switch p.Kind {
		case mapping.PartCategory, mapping.PartCapability:
			b.WriteString(codeSpan(p.Text, inTable))
		default:
			// PartLiteral, PartTarget, PartOffering, and any kind added later: text
			// is escaped whatever it is.
			b.WriteString(escapeText(p.Text))
		}
	}

	return b.String()
}

// textEscaper puts a backslash before every character that could start Markdown
// syntax in running text or a table cell: emphasis, code, links, images, HTML,
// headings, strikethrough, entity references and cell boundaries. Dots are left
// alone: they start no syntax in the middle of a line. Line endings, which a
// valid value never holds, become character references, so no value can end a
// line, a table row or a heading.
var textEscaper = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	"*", `\*`,
	"_", `\_`,
	"[", `\[`,
	"]", `\]`,
	"(", `\(`,
	")", `\)`,
	"<", `\<`,
	">", `\>`,
	"|", `\|`,
	"!", `\!`,
	"~", `\~`,
	"#", `\#`,
	"&", `\&`,
	"\n", "&#10;",
	"\r", "&#13;",
)

// escapeText returns s escaped for Markdown running text and table cells.
func escapeText(s string) string {
	return textEscaper.Replace(s)
}

// codeSpanLineEndings turns line endings into spaces, as CommonMark does inside a
// code span, so a value cannot end the line its code span is on.
var codeSpanLineEndings = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

// codeSpan returns s as a Markdown code span that shows s as given. The fence is
// one backtick longer than the longest run of backticks in s, and s is padded
// with a space on each side when it starts or ends with a backtick or a space,
// which CommonMark would otherwise merge into the fence or strip. In a table
// cell a pipe is written as "\|", the only escape a code span in a cell needs.
// An empty s renders as nothing.
func codeSpan(s string, inTable bool) string {
	if s == "" {
		return ""
	}

	s = codeSpanLineEndings.Replace(s)
	fence := strings.Repeat("`", longestRun(s, '`')+1)

	pad := ""
	if needsPadding(s) {
		pad = " "
	}

	if inTable {
		s = strings.ReplaceAll(s, "|", `\|`)
	}

	return fence + pad + s + pad + fence
}

// needsPadding reports whether code span content s must be padded with a space
// on each side: it starts or ends with a backtick, which would join the fence,
// or with a space, which CommonMark strips from content that starts and ends
// with one. Content of spaces only is never stripped, so it is never padded.
func needsPadding(s string) bool {
	if strings.Trim(s, " ") == "" {
		return false
	}

	first, last := s[0], s[len(s)-1]

	return first == '`' || last == '`' || first == ' ' || last == ' '
}

// longestRun returns the length of the longest run of byte c in s.
func longestRun(s string, c byte) int {
	longest, run := 0, 0

	for i := range len(s) {
		if s[i] != c {
			run = 0

			continue
		}

		run++
		longest = max(longest, run)
	}

	return longest
}

// upperHex are the digits of a percent escape.
const upperHex = "0123456789ABCDEF"

// urlUnsafe lists the characters, besides ASCII control characters and the
// space, that a source URL may not show as given in a Markdown autolink: the
// parentheses and square brackets of link syntax, the angle brackets that
// delimit the autolink, the pipe that ends a table cell and the backtick that
// starts a code span.
const urlUnsafe = "()[]<>|`"

// autolink returns the stored source URL u as a Markdown autolink. Every
// character of urlUnsafe, every ASCII control character and the space is
// percent-encoded; nothing else changes, and u is never decoded, so a stored
// escape such as "%0A" is shown as stored.
func autolink(u string) string {
	var b strings.Builder

	b.WriteByte('<')

	for i := range len(u) {
		c := u[i]
		if c > ' ' && c != 0x7f && strings.IndexByte(urlUnsafe, c) < 0 {
			b.WriteByte(c)

			continue
		}

		b.WriteByte('%')
		b.WriteByte(upperHex[c>>4])
		b.WriteByte(upperHex[c&0x0f])
	}

	b.WriteByte('>')

	return b.String()
}

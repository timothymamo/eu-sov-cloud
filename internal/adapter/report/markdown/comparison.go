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

// ReportComparison writes the Markdown comparison report of c and meta to w,
// laid out as the package documentation describes: the comparison of the
// targets first, then each target's single-target report body, one heading
// level lower.
//
// The output depends on c and meta only. ReportComparison builds the whole
// report before writing it to w in one call, and returns a write error wrapped.
// When ctx is already done it writes nothing and returns ctx.Err().
//
// The comparison must hold at least one target, every target must assess the
// same resources in the same order (the one inventory's order), and every
// shared blocker must name only compared targets. Otherwise ReportComparison
// writes nothing and returns an error naming the offending target key or
// resource ID.
func (*Reporter) ReportComparison(ctx context.Context, w io.Writer, c assessment.Comparison, meta analysis.ReportMeta) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(c.Targets) == 0 {
		// The package may not import errors (depguard reporters-stay-offline).
		return fmt.Errorf("render markdown comparison: the comparison holds no target")
	}

	if err := validateComparison(c); err != nil {
		return fmt.Errorf("render markdown comparison: %w", err)
	}

	var b strings.Builder

	writeComparisonHeader(&b, c.Targets, meta)
	b.WriteString(sectionHeading + " Comparison\n\n")
	writeComparisonSummary(&b, c.Targets)
	writeComparisonResources(&b, c.Targets)
	writeSharedBlockers(&b, c)

	for _, t := range c.Targets {
		b.WriteString("\n" + sectionHeading + " " + codeSpan(t.Assessment.Target, false) + "\n\n")
		writeBody(&b, t.Assessment, subsectionHeading)
	}

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write markdown comparison report: %w", err)
	}

	return nil
}

// validateComparison checks the preconditions ReportComparison documents for a
// comparison of at least one target: every target's verdicts hold the first
// target's resource IDs in the same order, and every shared blocker names only
// targets the comparison holds.
func validateComparison(c assessment.Comparison) error {
	first := c.Targets[0]

	for _, t := range c.Targets[1:] {
		if len(t.Assessment.Verdicts) != len(first.Assessment.Verdicts) {
			return fmt.Errorf("target %q has %d verdicts, target %q has %d: the targets must assess the same resources",
				t.Key, len(t.Assessment.Verdicts), first.Key, len(first.Assessment.Verdicts))
		}

		for i, v := range t.Assessment.Verdicts {
			if want := first.Assessment.Verdicts[i].Resource.ID; v.Resource.ID != want {
				return fmt.Errorf("target %q has resource %q at position %d where target %q has %q: "+
					"the targets must assess the same resources in the same order",
					t.Key, v.Resource.ID, i, first.Key, want)
			}
		}
	}

	// Target keys are unique by contract: analysis.NewComparison refuses a
	// repeated key and assessment.Compare documents it.
	keys := make(map[string]bool, len(c.Targets))
	for _, t := range c.Targets {
		keys[t.Key] = true
	}

	for _, blocker := range c.SharedBlockers {
		for _, t := range blocker.Targets {
			if !keys[t.Key] {
				return fmt.Errorf("the shared blocker of resource %q names target %q, which the comparison does not hold",
					blocker.ResourceID, t.Key)
			}
		}
	}

	return nil
}

// writeComparisonHeader writes the title naming every target, the inventory
// file line (left out when the inventory has no file name), the source, taken
// from the first target, and one catalog line per target.
func writeComparisonHeader(b *strings.Builder, targets []assessment.TargetAssessment, meta analysis.ReportMeta) {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, codeSpan(t.Assessment.Target, false))
	}

	b.WriteString("# Migration assessment: " + strings.Join(names, nameSeparator) + "\n\n")

	if meta.InventoryName != "" {
		b.WriteString("- Inventory file: " + codeSpan(meta.InventoryName, false) + "\n")
	}

	b.WriteString("- Source: " + codeSpan(targets[0].Assessment.Source, false) + "\n")

	for i, t := range targets {
		a := t.Assessment
		b.WriteString("- Catalog: " + names[i] + " snapshot from " + codeSpan(a.Snapshot.SourceURL, false) +
			", retrieved " + a.Snapshot.RetrievedOn.UTC().Format(dateLayout) + "\n")
	}

	b.WriteString("\n")
}

// writeComparisonSummary writes one row per fit, and a total, with one column
// per target holding that target's count.
func writeComparisonSummary(b *strings.Builder, targets []assessment.TargetAssessment) {
	b.WriteString(subsectionHeading + " Summary\n\n")

	header := []string{"Fit"}
	direct := []string{string(mapping.Direct)}
	partial := []string{string(mapping.Partial)}
	none := []string{string(mapping.None)}
	total := []string{"total"}

	for _, t := range targets {
		s := t.Assessment.Summary
		header = append(header, codeSpan(t.Assessment.Target, true))
		direct = append(direct, strconv.Itoa(s.Direct))
		partial = append(partial, strconv.Itoa(s.Partial))
		none = append(none, strconv.Itoa(s.None))
		total = append(total, strconv.Itoa(s.Total))
	}

	writeRow(b, header...)
	writeSeparator(b, len(header))
	writeRow(b, direct...)
	writeRow(b, partial...)
	writeRow(b, none...)
	writeRow(b, total...)
	b.WriteString("\n")
}

// writeComparisonResources writes one row per resource, in the first target's
// verdict order, which is the inventory's and, as validateComparison checks,
// every target's: the ID, the category, and each target's fit and best
// offering, without the tie note.
func writeComparisonResources(b *strings.Builder, targets []assessment.TargetAssessment) {
	b.WriteString(subsectionHeading + " Resources\n\n")

	header := []string{"ID", "Category"}

	for _, t := range targets {
		name := codeSpan(t.Assessment.Target, true)
		header = append(header, name+" fit", name+" best offering")
	}

	writeRow(b, header...)
	writeSeparator(b, len(header))

	for i, first := range targets[0].Assessment.Verdicts {
		row := []string{
			codeSpan(first.Resource.ID, true),
			codeSpan(string(first.Resource.Category), true),
		}

		for _, t := range targets {
			v := t.Assessment.Verdicts[i]
			row = append(row, escapeText(string(v.Fit)), codeSpan(v.BestOffering, true))
		}

		writeRow(b, row...)
	}

	b.WriteString("\n")
}

// writeSharedBlockers writes one list item per resource blocked on every
// target, with a nested item per target giving that target's reasons, or "None."
// when there is none. A target is named by its display name, never its key.
// It ends with a single newline.
func writeSharedBlockers(b *strings.Builder, c assessment.Comparison) {
	b.WriteString(subsectionHeading + " Blockers on every target\n\n")

	if len(c.SharedBlockers) == 0 {
		b.WriteString("None.\n")

		return
	}

	// Target keys are unique by contract (analysis.NewComparison refuses a
	// repeated key; assessment.Compare documents it), and validateComparison has
	// checked that every key a shared blocker names is here.
	names := make(map[string]string, len(c.Targets))
	for _, t := range c.Targets {
		names[t.Key] = t.Assessment.Target
	}

	for _, blocker := range c.SharedBlockers {
		b.WriteString("- " + codeSpan(blocker.ResourceID, false) + "\n")

		for _, t := range blocker.Targets {
			b.WriteString("  - " + codeSpan(names[t.Key], false) + ": " + renderReasons(t.Reasons, false) + "\n")
		}
	}
}

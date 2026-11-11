package assessment_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
)

// candidate is a candidate offering named name with fit and missing
// capabilities.
func candidate(name string, fit mapping.Fit, missing ...taxonomy.Capability) mapping.Candidate {
	return mapping.Candidate{Offering: catalog.Offering{Name: name}, Fit: fit, Missing: missing}
}

// TestVerdictTies pins which candidates every report shows as ties of the best
// one: the same fit and as many missing capabilities, in candidate order.
func TestVerdictTies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		candidates []mapping.Candidate
		want       []string
	}{
		{name: "no candidate"},
		{name: "one candidate", candidates: []mapping.Candidate{candidate("A", mapping.Direct)}},
		{
			name: "direct ties in candidate order",
			candidates: []mapping.Candidate{
				candidate("A", mapping.Direct),
				candidate("C", mapping.Direct),
				candidate("B", mapping.Direct),
			},
			want: []string{"C", "B"},
		},
		{
			name: "a partial candidate does not tie a direct one",
			candidates: []mapping.Candidate{
				candidate("A", mapping.Direct),
				candidate("B", mapping.Partial, taxonomy.IPv6),
			},
		},
		{
			name: "partial candidates tie only with as many missing capabilities",
			candidates: []mapping.Candidate{
				candidate("A", mapping.Partial, taxonomy.IPv6),
				candidate("B", mapping.Partial, taxonomy.Snapshots),
				candidate("C", mapping.Partial, taxonomy.IPv6, taxonomy.Snapshots),
			},
			want: []string{"B"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := assessment.Verdict{Candidates: tc.candidates}.Ties()
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Ties() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestVerdictChangesToList pins that a report lists required changes for a
// partial fit only.
func TestVerdictChangesToList(t *testing.T) {
	t.Parallel()

	changes := []taxonomy.Capability{taxonomy.IPv6}

	tests := []struct {
		fit  mapping.Fit
		want []taxonomy.Capability
	}{
		{fit: mapping.Direct},
		{fit: mapping.Partial, want: changes},
		{fit: mapping.None},
	}

	for _, tc := range tests {
		t.Run(string(tc.fit), func(t *testing.T) {
			t.Parallel()

			got := assessment.Verdict{Fit: tc.fit, RequiredChanges: changes}.ChangesToList()
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ChangesToList() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

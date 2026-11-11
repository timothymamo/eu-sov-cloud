package assessment

import (
	"fmt"
	"slices"
	"strings"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/mapping"
)

// TargetAssessment is the assessment of an inventory against one target, named
// by the key the target was selected with (such as "scaleway"), not by the
// catalog's display name.
type TargetAssessment struct {
	// Key names the target the assessment is for.
	Key string
	// Assessment is that target's assessment, as Assess returned it.
	Assessment Assessment
}

// TargetReasons is why a resource cannot move to one target.
type TargetReasons struct {
	// Key names the target.
	Key string
	// Reasons are the reasons of the resource's verdict on that target.
	Reasons []mapping.Reason
}

// SharedBlocker is a resource that is blocked on every target compared: its fit
// is mapping.None on each of them, so it stays a blocker wherever the workload
// moves.
type SharedBlocker struct {
	// ResourceID is the blocked resource's ID.
	ResourceID string
	// Targets holds each target's reasons for the block, in target key order.
	Targets []TargetReasons
}

// Comparison is the assessments of one inventory against several targets, side
// by side. It states facts per target; it does not rank the targets.
type Comparison struct {
	// Targets holds one assessment per target, in key order, and is nil when no
	// target was compared.
	Targets []TargetAssessment
	// SharedBlockers holds one entry per resource blocked on every target, in
	// resource ID order, and is nil when there is none.
	SharedBlockers []SharedBlocker
}

// Compare puts the given target assessments side by side. It sorts a copy of
// targets by key, so the caller's slice keeps its order, changes no assessment,
// and finds the blockers every target shares.
//
// Keys are expected to be unique; analysis.NewComparison guarantees it. With
// unique keys the result does not depend on the order the targets were given
// in; with a repeated key the stable sort keeps those targets in input order.
// Compare does not validate keys: they are untrusted values that reporters must
// escape like any other free text.
func Compare(targets []TargetAssessment) Comparison {
	if len(targets) == 0 {
		return Comparison{}
	}

	sorted := slices.Clone(targets)
	slices.SortStableFunc(sorted, func(a, b TargetAssessment) int {
		return strings.Compare(a.Key, b.Key)
	})

	return Comparison{Targets: sorted, SharedBlockers: sharedBlockers(sorted)}
}

// sharedBlockers returns the resources whose fit is mapping.None on every one of
// targets, in resource ID order, with each target's reasons in the order of
// targets. It is nil when there is none.
func sharedBlockers(targets []TargetAssessment) []SharedBlocker {
	// blocked holds, per target, the reasons of each resource with fit none.
	blocked := make([]map[string][]mapping.Reason, len(targets))

	for i, target := range targets {
		blocked[i] = make(map[string][]mapping.Reason)

		for _, v := range target.Assessment.Verdicts {
			if v.Fit == mapping.None {
				blocked[i][v.Resource.ID] = v.Reasons
			}
		}
	}

	// The candidates are the first target's blocked resources; walking its
	// verdicts, not a map, keeps the output independent of map order.
	var shared []SharedBlocker

	for _, v := range targets[0].Assessment.Verdicts {
		if v.Fit != mapping.None {
			continue
		}

		if blocker, ok := sharedBlocker(v.Resource.ID, targets, blocked); ok {
			shared = append(shared, blocker)
		}
	}

	slices.SortStableFunc(shared, func(a, b SharedBlocker) int {
		return strings.Compare(a.ResourceID, b.ResourceID)
	})

	return shared
}

// sharedBlocker returns the shared blocker for id when id is blocked on every
// target, and false otherwise.
func sharedBlocker(id string, targets []TargetAssessment, blocked []map[string][]mapping.Reason) (SharedBlocker, bool) {
	blocker := SharedBlocker{ResourceID: id, Targets: make([]TargetReasons, 0, len(targets))}

	for i, target := range targets {
		reasons, ok := blocked[i][id]
		if !ok {
			return SharedBlocker{}, false
		}

		blocker.Targets = append(blocker.Targets, TargetReasons{Key: target.Key, Reasons: slices.Clone(reasons)})
	}

	return blocker, true
}

// Validate checks what every report of a comparison relies on: every target's
// verdicts hold the first target's resource IDs in the same order (the one
// inventory's order), and every shared blocker names only targets the
// comparison holds. It returns an error naming the offending target key or
// resource ID. A comparison of no target is valid here; a reporter decides
// whether it has anything to lay out.
func (c Comparison) Validate() error {
	if len(c.Targets) == 0 {
		return nil
	}

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
	// repeated key and Compare documents it.
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

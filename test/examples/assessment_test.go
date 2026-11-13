package examples_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/internal/adapter/report/markdown"
	"github.com/timothymamo/shift2eu/internal/adapter/source/inventoryfile"
	"github.com/timothymamo/shift2eu/internal/app/analysis"
	"github.com/timothymamo/shift2eu/internal/domain/assessment"
	"github.com/timothymamo/shift2eu/internal/domain/mapping"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
)

// TestSamplesAssessAgainstEveryTarget pins that every sample runs through the
// real assess path against every target cmd/shift2eu wires: the inventory-file
// adapter, the embedded catalog snapshot and the Markdown reporter, through
// the analysis use case. A sample the walkthrough shows must not fail there.
func TestSamplesAssessAgainstEveryTarget(t *testing.T) {
	t.Parallel()

	for _, file := range sampleFiles(t) {
		for _, target := range targets {
			t.Run(file.name+"/"+target.key, func(t *testing.T) {
				t.Parallel()

				useCase, err := analysis.New(inventoryfile.New(file.path), target.catalog(), markdown.New())
				if err != nil {
					t.Fatalf("analysis.New: %v", err)
				}

				var report bytes.Buffer

				meta := analysis.ReportMeta{InventoryName: filepath.Base(file.path)}
				if err := useCase.Run(t.Context(), &report, meta); err != nil {
					t.Fatalf("assess %s against %s: %v", file.name, target.key, err)
				}

				if report.Len() == 0 {
					t.Errorf("assess %s against %s rendered no report", file.name, target.key)
				}
			})
		}
	}
}

// TestSamplesCoverTheTaxonomy pins issue #179's coverage promise: the samples
// together use every taxonomy category and every capability, so a demo shows
// the whole vocabulary, and they produce every fit on some target, so a report
// shows a resource that moves unchanged, one that needs changes and one that
// cannot move. The categories and capabilities are read from the taxonomy
// package, not copied here, so a value added to the vocabulary fails until a
// sample uses it.
func TestSamplesCoverTheTaxonomy(t *testing.T) {
	t.Parallel()

	usedCategories := make(map[taxonomy.Category]bool)
	usedCapabilities := make(map[taxonomy.Capability]bool)
	producedFits := make(map[mapping.Fit]bool)

	for _, file := range sampleFiles(t) {
		inv := loadSample(t, file.path)

		for _, resource := range inv.Resources() {
			usedCategories[resource.Category] = true

			for _, capability := range resource.Capabilities {
				usedCapabilities[capability] = true
			}
		}

		for _, target := range targets {
			cat, err := target.catalog().Catalog(t.Context())
			if err != nil {
				t.Fatalf("load the %s catalog: %v", target.key, err)
			}

			for _, verdict := range assessment.Assess(inv, cat).Verdicts {
				producedFits[verdict.Fit] = true
			}
		}
	}

	// Compare each whole declared vocabulary with the part in use, in the order
	// the vocabulary declares it, so the diff names exactly what is missing.
	var categoriesInUse []taxonomy.Category

	for _, category := range taxonomy.Categories() {
		if usedCategories[category] {
			categoriesInUse = append(categoriesInUse, category)
		}
	}

	if diff := cmp.Diff(taxonomy.Categories(), categoriesInUse); diff != "" {
		t.Errorf("the sample inventories together do not use every taxonomy category; add a resource of each missing category (declared minus used):\n%s", diff)
	}

	var capabilitiesInUse []taxonomy.Capability

	for _, capability := range taxonomy.Capabilities() {
		if usedCapabilities[capability] {
			capabilitiesInUse = append(capabilitiesInUse, capability)
		}
	}

	if diff := cmp.Diff(taxonomy.Capabilities(), capabilitiesInUse); diff != "" {
		t.Errorf("the sample inventories together do not use every taxonomy capability; add a resource that depends on each missing capability (declared minus used):\n%s", diff)
	}

	declaredFits := []mapping.Fit{mapping.Direct, mapping.Partial, mapping.None}

	var fitsInUse []mapping.Fit

	for _, fit := range declaredFits {
		if producedFits[fit] {
			fitsInUse = append(fitsInUse, fit)
		}
	}

	if diff := cmp.Diff(declaredFits, fitsInUse); diff != "" {
		t.Errorf("the sample inventories together do not produce every fit on every target; add a resource for each missing fit (declared minus produced):\n%s", diff)
	}
}

package main

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
)

// readiness is the check GET /readyz runs: ready once every target's catalog
// has loaded. A load that fails leaves it not ready, and the next check tries
// again; once every catalog has loaded, the check passes without loading them
// again, as the embedded snapshots cannot change while the process runs.
type readiness struct {
	targets map[string]func() analysis.CatalogSource

	mu    sync.Mutex
	ready bool
}

// check loads every target's catalog, in key order, and returns the first
// error, naming the target by its key.
func (r *readiness) check(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ready {
		return nil
	}

	for _, key := range slices.Sorted(mapKeys(r.targets)) {
		if _, err := r.targets[key]().Catalog(ctx); err != nil {
			return fmt.Errorf("load the %s catalog: %w", key, err)
		}
	}

	r.ready = true

	return nil
}

// mapKeys yields the keys of m.
func mapKeys(m map[string]func() analysis.CatalogSource) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for key := range m {
			if !yield(key) {
				return
			}
		}
	}
}

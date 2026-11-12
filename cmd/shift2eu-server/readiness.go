package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/timothymamo/shift2eu/internal/app/analysis"
)

// readiness is the check GET /readyz runs: ready once every target's catalog
// has loaded. A load that fails leaves it not ready, and the next check tries
// again; once every catalog has loaded, the check passes without loading them
// again, as the embedded snapshots cannot change while the process runs. A
// check that comes while another is loading answers not ready at once rather
// than waiting for that load: a probe's timeout is short, and 503 is the answer
// it understands (ADR 0019).
type readiness struct {
	targets map[string]func() analysis.CatalogSource

	// loading is held by the check that loads; mu guards ready, and is held
	// only to read or set it, so a check never waits on a load.
	loading sync.Mutex
	mu      sync.Mutex
	ready   bool
}

// isReady reports whether every catalog has loaded.
func (r *readiness) isReady() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.ready
}

// errLoading is the answer of a check that comes while another loads.
var errLoading = errors.New("the catalogs are loading")

// check loads every target's catalog, in key order, and returns the first
// error, naming the target by its key.
func (r *readiness) check(ctx context.Context) error {
	if r.isReady() {
		return nil
	}

	if !r.loading.TryLock() {
		return errLoading
	}
	defer r.loading.Unlock()

	if r.isReady() {
		return nil
	}

	for _, key := range slices.Sorted(mapKeys(r.targets)) {
		if _, err := r.targets[key]().Catalog(ctx); err != nil {
			return fmt.Errorf("load the %s catalog: %w", key, err)
		}
	}

	r.mu.Lock()
	r.ready = true
	r.mu.Unlock()

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

// White-box tests of the readiness check the composition root hands
// internal/httpapi: GET /readyz answers 200 once every target's embedded catalog
// has loaded, and 503 until then. cmd/shift2eu-server is a main package, so its
// tests are package main, the only way a test file reaches the package at all
// (Go refuses an import of a main package). The check is driven directly here,
// over a fake catalog source, so a test counts the loads a readiness check makes
// and sees whether it waits for one already running; the endpoint itself is
// pinned through run in run_test.go.
package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/timothymamo/shift2eu/internal/app/analysis"
	"github.com/timothymamo/shift2eu/internal/domain/catalog"
)

// readinessBound bounds how long a check that must not wait may take to answer:
// the check takes a lock, so a second check answers at once or not at all.
const readinessBound = 5 * time.Second

// fakeCatalogSource is an in-memory analysis.CatalogSource that counts its loads
// and can fail or block one, so a test sees how often the readiness check loads a
// catalog and whether a check waits for a load already running.
type fakeCatalogSource struct {
	mu      sync.Mutex
	loads   int
	err     error
	block   chan struct{}
	started chan struct{}
}

// Catalog counts the load, reports that it started when started is set, waits on
// block while block is set, and returns the configured error.
func (f *fakeCatalogSource) Catalog(ctx context.Context) (catalog.Catalog, error) {
	f.mu.Lock()
	f.loads++
	err, block, started := f.err, f.block, f.started
	f.mu.Unlock()

	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}

	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return catalog.Catalog{}, ctx.Err()
		}
	}

	if err != nil {
		return catalog.Catalog{}, err
	}

	return catalog.Catalog{}, nil
}

// setErr makes every following load fail with err, or succeed when err is nil.
func (f *fakeCatalogSource) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.err = err
}

// loadCount returns how often Catalog ran.
func (f *fakeCatalogSource) loadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.loads
}

// targetsOf returns the target map readiness takes, its key the target's name as
// /v1/targets carries it and its value the constructor of that target's catalog
// source.
func targetsOf(sources map[string]*fakeCatalogSource) map[string]func() analysis.CatalogSource {
	targets := make(map[string]func() analysis.CatalogSource, len(sources))

	for key, source := range sources {
		targets[key] = func() analysis.CatalogSource { return source }
	}

	return targets
}

// TestReadiness pins the check GET /readyz runs (audit L5, review S3): a failed
// load leaves the server not ready and the error names the target, the next check
// tries again, and once every catalog has loaded the check passes without loading
// anything again. A check that runs while a load is in flight answers not ready at
// once instead of waiting for that load: the probe's timeout is short, and 503 is
// the answer it understands.
func TestReadiness(t *testing.T) {
	t.Parallel()

	t.Run("a failed load is retried and a successful one is cached", func(t *testing.T) {
		t.Parallel()

		source := &fakeCatalogSource{err: errors.New("the embedded snapshot is missing")}
		r := &readiness{targets: targetsOf(map[string]*fakeCatalogSource{"ovhcloud": source})}

		err := r.check(t.Context())
		if err == nil {
			t.Fatalf("check with a failing load = nil, want an error, so /readyz answers 503")
		}

		if !strings.Contains(err.Error(), "ovhcloud") {
			t.Errorf("the error %q does not name the target ovhcloud, so the operator cannot tell which catalog failed", err)
		}

		if got := source.loadCount(); got != 1 {
			t.Errorf("the catalog was loaded %d times for one check, want 1", got)
		}

		source.setErr(nil)

		if err := r.check(t.Context()); err != nil {
			t.Fatalf("check after the load recovered = %v, want nil: a failed load is retried on the next check", err)
		}

		if got := source.loadCount(); got != 2 {
			t.Errorf("the catalog was loaded %d times for two checks, want 2: a check that is not ready yet loads again", got)
		}

		if err := r.check(t.Context()); err != nil {
			t.Fatalf("check once ready = %v, want nil", err)
		}

		if got := source.loadCount(); got != 2 {
			t.Errorf("the catalog was loaded %d times after three checks, want 2: a check that is ready loads nothing again", got)
		}
	})

	t.Run("a load already running is not waited for", func(t *testing.T) {
		t.Parallel()

		block := make(chan struct{})
		release := sync.OnceFunc(func() { close(block) })

		t.Cleanup(release)

		source := &fakeCatalogSource{block: block, started: make(chan struct{}, 1)}
		r := &readiness{targets: targetsOf(map[string]*fakeCatalogSource{"ovhcloud": source})}

		loading := make(chan error, 1)

		go func() { loading <- r.check(context.Background()) }()

		select {
		case <-source.started:
		case <-time.After(readinessBound):
			t.Fatalf("the first check did not start loading the catalog within %s", readinessBound)
		}

		answered := make(chan error, 1)

		go func() { answered <- r.check(context.Background()) }()

		select {
		case err := <-answered:
			if err == nil {
				t.Errorf("a check while a load was in flight = nil, want not ready: /readyz answers 503 rather than waiting")
			}
		case <-time.After(readinessBound):
			t.Fatalf("a check waited %s for a load already in flight, want it to answer not ready at once", readinessBound)
		}

		release()

		if err := <-loading; err != nil {
			t.Errorf("the check that loaded the catalog = %v, want nil", err)
		}

		if got := source.loadCount(); got != 1 {
			t.Errorf("the catalog was loaded %d times for two overlapping checks, want 1: the second check must not load it too", got)
		}
	})
}

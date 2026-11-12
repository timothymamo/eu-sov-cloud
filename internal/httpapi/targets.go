package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
)

// dateLayout is how a snapshot's retrieval date is written: its UTC date, as
// the JSON report writes it.
const dateLayout = "2006-01-02"

// targetsDocument is the body of GET /v1/targets.
type targetsDocument struct {
	Targets []targetEntry `json:"targets"`
}

// targetEntry is one target: its key and, as the JSON report's target object
// has them, its name and snapshot.
type targetEntry struct {
	Key      string        `json:"key"`
	Name     string        `json:"name"`
	Snapshot snapshotEntry `json:"snapshot"`
}

// snapshotEntry is where a catalog was taken from and the UTC date it was
// retrieved.
type snapshotEntry struct {
	Source    string `json:"source"`
	Retrieved string `json:"retrieved"`
}

// statusDocument is the body of GET /healthz and of a ready GET /readyz.
type statusDocument struct {
	Status string `json:"status"`
}

// targets serves GET /v1/targets: every supported target, sorted by key. The
// catalogs are loaded on the first request and the body kept; a catalog that
// cannot be loaded is 500 internal, and nothing is kept, so the next request
// loads them again.
func (a *api) targets(w http.ResponseWriter, r *http.Request) {
	body, err := a.cachedTargets(r.Context())
	if err != nil {
		writeInternal(w)

		return
	}

	writeBody(w, http.StatusOK, body)
}

// cachedTargets returns the body of GET /v1/targets, loading and keeping it
// on the first call. The lock is held while the catalogs load, so concurrent
// first requests load each catalog once between them, but not while the body
// is written: a client that reads its answer slowly never holds up another.
// The kept body is never changed, so it is shared without a copy.
func (a *api) cachedTargets(ctx context.Context) ([]byte, error) {
	a.targetsMu.Lock()
	defer a.targetsMu.Unlock()

	if a.targetsBody == nil {
		body, err := a.loadTargets(ctx)
		if err != nil {
			return nil, err
		}

		a.targetsBody = body
	}

	return a.targetsBody, nil
}

// loadTargets loads every target's catalog and returns the body of GET
// /v1/targets.
func (a *api) loadTargets(ctx context.Context) ([]byte, error) {
	doc := targetsDocument{Targets: make([]targetEntry, 0, len(a.keys))}

	for _, key := range a.keys {
		cat, err := a.deps.Targets[key]().Catalog(ctx)
		if err != nil {
			return nil, err
		}

		snapshot := cat.Snapshot()
		doc.Targets = append(doc.Targets, targetEntry{
			Key:  key,
			Name: cat.Target(),
			Snapshot: snapshotEntry{
				Source:    snapshot.SourceURL,
				Retrieved: snapshot.RetrievedOn.UTC().Format(dateLayout),
			},
		})
	}

	return json.Marshal(doc)
}

// healthz serves GET /healthz: the process runs.
func (a *api) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, statusDocument{Status: "ok"})
}

// readyz serves GET /readyz: ready unless the configured readiness check
// fails.
func (a *api) readyz(w http.ResponseWriter, r *http.Request) {
	if a.cfg.readiness != nil {
		if err := a.cfg.readiness(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, "not_ready", "the service is not ready")

			return
		}
	}

	writeJSON(w, http.StatusOK, statusDocument{Status: "ready"})
}

// writeJSON writes v as the JSON body of a response with status. It encodes
// into memory first, so a value that cannot be encoded is 500 internal, never
// half a body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		writeInternal(w)

		return
	}

	writeBody(w, status, body)
}

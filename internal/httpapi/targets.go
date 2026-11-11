package httpapi

import (
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

// targets serves GET /v1/targets: every supported target, sorted by key. A
// catalog that cannot be loaded is 500 internal.
func (a *api) targets(w http.ResponseWriter, r *http.Request) {
	doc := targetsDocument{Targets: make([]targetEntry, 0, len(a.keys))}

	for _, key := range a.keys {
		cat, err := a.deps.Targets[key]().Catalog(r.Context())
		if err != nil {
			writeInternal(w)

			return
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

	writeJSON(w, http.StatusOK, doc)
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

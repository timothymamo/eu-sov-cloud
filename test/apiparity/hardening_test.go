package apiparity_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// This file pins the API's answer to an inventory it refuses, over the real
// adapters: POST /v1/assessments names the problem without ever echoing a value
// from the inventory document, which an untrusted client sent and a reader
// error must not repeat (PRD #103, "an error message never echoes request
// content beyond a quoted, escaped field name").

// inventoryMarker is the value the hostile inventories carry where the reader
// would quote it, if it echoed one.
const inventoryMarker = "ACCT-MARKER-4242"

// hostileInventory is one inventory document the reader refuses, and the value
// in it that no response may repeat.
type hostileInventory struct {
	name     string
	document []byte
	// value is the document value the error must not echo.
	value string
}

// hostileInventories returns documents that fail at each layer of the reader:
// the document decoder, the taxonomy and the domain inventory rules.
func hostileInventories() []hostileInventory {
	return []hostileInventory{
		{
			// inventory.New names a duplicate ID by the position of the second
			// resource, not by repeating the ID both share.
			name: "a duplicate resource ID",
			document: []byte(`{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"` + inventoryMarker + `","category":"compute.vm"},` +
				`{"id":"` + inventoryMarker + `","category":"compute.vm"}]}`),
			value: inventoryMarker,
		},
		{
			// The taxonomy refuses an unknown category by quoting its value; the
			// API must name the field and the resource instead.
			name: "an unknown category",
			document: []byte(`{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"instance-one","category":"` + inventoryMarker + `"}]}`),
			value: inventoryMarker,
		},
		{
			name: "an unknown capability",
			document: []byte(`{"schemaVersion":1,"source":"aws","resources":[` +
				`{"id":"instance-one","category":"compute.vm","capabilities":["` + inventoryMarker + `"]}]}`),
			value: inventoryMarker,
		},
		{
			// A schemaVersion that is the wrong number is quoted by the shared
			// decoder: the API must name the key and the version it reads.
			name:     "a schema version number",
			document: []byte(`{"schemaVersion":4242,"source":"aws","resources":[]}`),
			value:    "4242",
		},
	}
}

// TestTheAPINeverEchoesAnInventoryValue pins that an inventory the reader
// refuses is 400 invalid_inventory and that the raw response body repeats no
// value from the document, at every layer that can refuse one.
func TestTheAPINeverEchoesAnInventoryValue(t *testing.T) {
	t.Parallel()

	for _, tc := range hostileInventories() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := postInventory(t, tc.document)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("POST %s = %d, want %d; body: %q",
					assessmentsPath, rec.Code, http.StatusBadRequest, rec.Body.String())
			}

			var doc struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}

			if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
				t.Fatalf("the error body is not the documented JSON shape: %v; body: %q", err, rec.Body.String())
			}

			if doc.Error.Code != "invalid_inventory" {
				t.Errorf("error code = %q, want %q; message: %q",
					doc.Error.Code, "invalid_inventory", doc.Error.Message)
			}

			if strings.TrimSpace(doc.Error.Message) == "" {
				t.Errorf("the %d message is empty", rec.Code)
			}

			if body := rec.Body.String(); strings.Contains(body, tc.value) {
				t.Errorf("the response body repeats the inventory value %q: %q", tc.value, body)
			}
		})
	}
}

// postInventory posts document to the real API handler with one target and
// returns the recorded response, whatever its status.
func postInventory(t *testing.T, document []byte) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(struct {
		Inventory json.RawMessage `json:"inventory"`
		Targets   []string        `json:"targets"`
	}{Inventory: document, Targets: []string{"scaleway"}})
	if err != nil {
		t.Fatalf("encoding the request body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, assessmentsPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	apiHandler(t).ServeHTTP(rec, req)

	return rec
}

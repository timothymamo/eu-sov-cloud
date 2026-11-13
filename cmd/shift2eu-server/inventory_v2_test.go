// White-box tests of the composition root for inventory format version 2 (PRD
// #190, milestone 1): cmd/shift2eu-server wires the real
// internal/adapter/source/inventoryfile.FromBytes, so the running server is the
// only place the API's acceptance of a version 2 inventory is pinned end to end.
// A version 2 inventory gives the report its version 1 twin gives, byte for
// byte; an invalid version 2 inventory is 400 invalid_inventory, and the message
// names the field by path, never the value (ADR 0019).

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// twinInventory returns one of the version 1 and version 2 twins the e2e suite
// holds: two files of the same base name with the same version 1 fields, the
// version 2 one carrying every version 2 field too. Both live in test/e2e's
// testdata, beside the inventory the whole suite shares.
func twinInventory(t *testing.T, version string) []byte {
	t.Helper()

	path := filepath.Join("..", "..", "test", "e2e", "testdata", "twin-"+version, "inventory.json")

	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the shared inventory fixture %s: %v", path, err)
	}

	return document
}

// postAccepting posts body to /v1/assessments and asks for the representation
// accept names: the JSON report or the Markdown one.
func postAccepting(t *testing.T, s *server, document []byte, accept string) answer {
	t.Helper()

	req := newRequest(t, http.MethodPost, "/v1/assessments", assessRequest(t, document, "ovhcloud"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", accept)

	return s.do(t, req)
}

// errorMessage is the code and message of an error body.
type errorMessage struct {
	code    string
	message string
}

// decodeErrorMessage decodes the API's one error shape, failing the test on
// anything else.
func decodeErrorMessage(t *testing.T, body []byte) errorMessage {
	t.Helper()

	var doc struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decoding the error body %q: %v", body, err)
	}

	return errorMessage{code: doc.Error.Code, message: doc.Error.Message}
}

// TestServerAcceptsInventoryVersion2 pins that POST /v1/assessments takes a
// version 2 inventory and answers with the report its version 1 twin gets, byte
// for byte, as JSON and as Markdown: milestone 1 changes no report.
func TestServerAcceptsInventoryVersion2(t *testing.T) {
	t.Parallel()

	version1 := twinInventory(t, "v1")
	version2 := twinInventory(t, "v2")

	// The twins must differ as documents, or the comparison would pass without a
	// version 2 field anywhere.
	if bytes.Equal(version1, version2) {
		t.Fatalf("the version 1 and version 2 twins are the same document")
	}

	s := startServer(t, loopbackListen, nil)

	for _, accept := range []string{"application/json", "text/markdown"} {
		t.Run(accept, func(t *testing.T) {
			t.Parallel()

			fromOne := postAccepting(t, s, version1, accept)
			fromTwo := postAccepting(t, s, version2, accept)

			for name, got := range map[string]answer{"version 1": fromOne, "version 2": fromTwo} {
				if got.status != http.StatusOK {
					t.Fatalf("POST /v1/assessments with the %s inventory = %d, want %d; body: %q",
						name, got.status, http.StatusOK, got.body)
				}

				if len(got.body) == 0 {
					t.Fatalf("POST /v1/assessments with the %s inventory returned an empty body", name)
				}
			}

			if !bytes.Equal(fromOne.body, fromTwo.body) {
				t.Errorf("the version 2 %s report differs from its version 1 twin:\nversion 1:\n%s\nversion 2:\n%s",
					accept, fromOne.body, fromTwo.body)
			}
		})
	}
}

// TestServerRefusesInvalidInventoryVersion2 pins the error side: a version 2
// inventory the reader refuses is 400 invalid_inventory, one line that names the
// relation by path, and no value from the request in the message.
func TestServerRefusesInvalidInventoryVersion2(t *testing.T) {
	t.Parallel()

	const marker = "ACCT-MARKER-4242"

	document := []byte(`{"schemaVersion":2,"source":"aws","resources":[` +
		`{"id":"db-primary","category":"database.postgresql"},` +
		`{"id":"db-replica","category":"database.postgresql",` +
		`"relations":[{"type":"replica-of","target":"` + marker + `"}]}]}`)

	s := startServer(t, loopbackListen, nil)

	got := postAccepting(t, s, document, "application/json")

	if got.status != http.StatusBadRequest {
		t.Fatalf("POST /v1/assessments with an invalid version 2 inventory = %d, want %d; body: %q",
			got.status, http.StatusBadRequest, got.body)
	}

	apiErr := decodeErrorMessage(t, got.body)
	if apiErr.code != "invalid_inventory" {
		t.Errorf("error code = %q, want %q; body: %q", apiErr.code, "invalid_inventory", got.body)
	}

	if !strings.Contains(apiErr.message, "resources[1].relations[0].target") {
		t.Errorf("error message = %q, want it to name the relation by path", apiErr.message)
	}

	if strings.Contains(apiErr.message, marker) {
		t.Errorf("error message = %q, want it not to echo the value %q", apiErr.message, marker)
	}

	if strings.ContainsAny(apiErr.message, "\r\n") {
		t.Errorf("error message = %q, want a single line", apiErr.message)
	}
}

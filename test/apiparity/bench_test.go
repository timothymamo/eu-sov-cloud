package apiparity_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/timothymamo/shift2eu/internal/adapter/source/inventoryfile"
)

// This file measures the memory a maximal request costs, over the real
// adapters: POST /v1/assessments with an inventory near inventoryfile.MaxResources
// and ["all"], which builds every target's assessment in one report (PRD #103,
// audit M2). It is a benchmark, not a pass/fail test, so a regression shows as a
// number in the report rather than as a red build.

// maximalInventory returns a sanitised inventory document of n resources, each
// as small as an inventory resource can be, so the document fits under the
// inventory reader's cap and the API's body cap.
func maximalInventory(tb testing.TB, n int) []byte {
	tb.Helper()

	var doc strings.Builder

	doc.WriteString(`{"schemaVersion":1,"source":"aws","resources":[`)

	for i := range n {
		if i > 0 {
			doc.WriteByte(',')
		}

		fmt.Fprintf(&doc, `{"id":"i-%08d","rawType":"AWS::EC2::Instance",`+
			`"category":"compute.vm","region":"eu-west-1"}`, i)
	}

	doc.WriteString("]}")

	if size := doc.Len(); size > inventoryfile.MaxFileSize {
		tb.Fatalf("the generated inventory is %d bytes, more than inventoryfile.MaxFileSize (%d)",
			size, inventoryfile.MaxFileSize)
	}

	return []byte(doc.String())
}

// BenchmarkMaximalAllComparison posts the largest inventory the reader accepts
// and every target to the real API handler, once per iteration, and reports the
// bytes allocated per request: the whole request path runs, from the strict
// request decoder through the inventory reader, the comparison of all targets
// and the JSON report rendered into memory.
func BenchmarkMaximalAllComparison(b *testing.B) {
	document := maximalInventory(b, inventoryfile.MaxResources)

	body, err := json.Marshal(struct {
		Inventory json.RawMessage `json:"inventory"`
		Targets   []string        `json:"targets"`
	}{Inventory: document, Targets: []string{"all"}})
	if err != nil {
		b.Fatalf("encoding the request body: %v", err)
	}

	handler := apiHandler(b)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		req := httptest.NewRequest(http.MethodPost, assessmentsPath, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			b.Fatalf("POST %s = %d, want %d; body: %q",
				assessmentsPath, rec.Code, http.StatusOK, rec.Body.String())
		}
	}
}

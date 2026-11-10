package analysis_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/assessment"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/catalog"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
	"github.com/timothymamo/eu-sov-cloud/internal/testutil"
)

const (
	// inventoryName is an ordinary inventory file name: valid free text that the
	// reporter may put in a report.
	inventoryName = "inventory.json"
	// reportBody is what the fake reporter writes, so a test can see that the
	// writer the use case passes on is the caller's.
	reportBody = "# eusov report\n"
	// sourceName names the source the fixture inventory was taken from.
	sourceName = "aws"
)

// awsInventory is a small inventory with a direct resource, a partial one and an
// unclassified one, so an assessment built from it has verdicts, required
// changes and blockers.
func awsInventory(t *testing.T) inventory.Inventory {
	t.Helper()

	inv, err := inventory.New(sourceName, []inventory.Resource{
		{
			ID:           "bucket-example",
			RawType:      "AWS::S3::Bucket",
			Category:     taxonomy.StorageObject,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.ObjectVersioning},
		},
		{
			ID:           "partial-example",
			RawType:      "AWS::EC2::Instance",
			Category:     taxonomy.ComputeVM,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.Snapshots},
		},
		{
			ID:       "misc-example",
			RawType:  "AWS::CloudFormation::CustomResource",
			Category: taxonomy.Unclassified,
			Region:   "eu-west-1",
		},
	})
	if err != nil {
		t.Fatalf("inventory.New() returned unexpected error: %v", err)
	}

	return inv
}

// scalewayCatalog is the shared Scaleway fixture the mapping rules are written
// against.
func scalewayCatalog(t *testing.T) catalog.Catalog {
	t.Helper()

	return testutil.ScalewayCatalog(t)
}

// recorder records the order in which the use case calls its ports.
type recorder struct {
	names []string
}

func (r *recorder) record(name string) {
	r.names = append(r.names, name)
}

func (r *recorder) calls() []string {
	return slices.Clone(r.names)
}

// fakeInventory is an in-memory InventorySource. It records the call, runs
// onCall (which a test uses to cancel the context mid-run) and returns err.
// Unless it ignores the context, it behaves like a well-behaved adapter and
// returns ctx.Err() once the context is done.
type fakeInventory struct {
	inv           inventory.Inventory
	err           error
	ignoreContext bool
	onCall        func()
	rec           *recorder
	calls         int
}

func (f *fakeInventory) Inventory(ctx context.Context) (inventory.Inventory, error) {
	f.calls++

	if f.rec != nil {
		f.rec.record("inventory")
	}

	if f.onCall != nil {
		f.onCall()
	}

	if !f.ignoreContext {
		if err := ctx.Err(); err != nil {
			return inventory.Inventory{}, err
		}
	}

	if f.err != nil {
		return inventory.Inventory{}, f.err
	}

	return f.inv, nil
}

// fakeCatalog is an in-memory CatalogSource, the catalog counterpart of
// fakeInventory.
type fakeCatalog struct {
	cat           catalog.Catalog
	err           error
	ignoreContext bool
	onCall        func()
	rec           *recorder
	calls         int
}

func (f *fakeCatalog) Catalog(ctx context.Context) (catalog.Catalog, error) {
	f.calls++

	if f.rec != nil {
		f.rec.record("catalog")
	}

	if f.onCall != nil {
		f.onCall()
	}

	if !f.ignoreContext {
		if err := ctx.Err(); err != nil {
			return catalog.Catalog{}, err
		}
	}

	if f.err != nil {
		return catalog.Catalog{}, f.err
	}

	return f.cat, nil
}

// fakeReporter is an in-memory Reporter. It keeps what the use case handed it and
// writes body to the writer, so a test can observe both sides.
type fakeReporter struct {
	body  string
	err   error
	rec   *recorder
	calls int

	gotAssessment assessment.Assessment
	gotMeta       analysis.ReportMeta
	gotWriter     io.Writer
}

func (f *fakeReporter) Report(_ context.Context, w io.Writer, a assessment.Assessment, meta analysis.ReportMeta) error {
	f.calls++
	f.gotAssessment = a
	f.gotMeta = meta
	f.gotWriter = w

	if f.rec != nil {
		f.rec.record("report")
	}

	// The body is written before an error is returned: a renderer that fails may
	// have written part of its output.
	_, writeErr := io.WriteString(w, f.body)

	if f.err != nil {
		return f.err
	}

	return writeErr
}

// TestNew pins the only construction rule: every port is required.
func TestNew(t *testing.T) {
	t.Parallel()

	inv := awsInventory(t)
	cat := scalewayCatalog(t)

	tests := []struct {
		name string
		inv  analysis.InventorySource
		cat  analysis.CatalogSource
		rep  analysis.Reporter
	}{
		{name: "nil inventory source", cat: &fakeCatalog{cat: cat}, rep: &fakeReporter{body: reportBody}},
		{name: "nil catalog source", inv: &fakeInventory{inv: inv}, rep: &fakeReporter{body: reportBody}},
		{name: "nil reporter", inv: &fakeInventory{inv: inv}, cat: &fakeCatalog{cat: cat}},
		{name: "every port set", inv: &fakeInventory{inv: inv}, cat: &fakeCatalog{cat: cat}, rep: &fakeReporter{body: reportBody}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := analysis.New(tt.inv, tt.cat, tt.rep)

			if tt.inv == nil || tt.cat == nil || tt.rep == nil {
				if !errors.Is(err, analysis.ErrNilPort) {
					t.Fatalf("New() error = %v, want an error wrapping analysis.ErrNilPort", err)
				}

				if got != nil {
					t.Errorf("New() = %v, want nil Analysis when a port is nil", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("New() returned unexpected error: %v", err)
			}

			if got == nil {
				t.Fatal("New() = nil, want a usable Analysis")
			}
		})
	}
}

// TestRun pins the happy path: load the inventory, load the catalog, assess, hand
// the assessment and the metadata to the reporter, and let its output through.
func TestRun(t *testing.T) {
	t.Parallel()

	inv := awsInventory(t)
	cat := scalewayCatalog(t)
	meta := analysis.ReportMeta{InventoryName: inventoryName}

	rec := &recorder{}
	invFake := &fakeInventory{inv: inv, rec: rec}
	catFake := &fakeCatalog{cat: cat, rec: rec}
	repFake := &fakeReporter{body: reportBody, rec: rec}

	useCase, err := analysis.New(invFake, catFake, repFake)
	if err != nil {
		t.Fatalf("New() returned unexpected error: %v", err)
	}

	var buf bytes.Buffer

	if err := useCase.Run(context.Background(), &buf, meta); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}

	if diff := cmp.Diff([]string{"inventory", "catalog", "report"}, rec.calls()); diff != "" {
		t.Errorf("port call order mismatch (-want +got):\n%s", diff)
	}

	if invFake.calls != 1 {
		t.Errorf("Inventory() called %d times, want 1", invFake.calls)
	}

	if catFake.calls != 1 {
		t.Errorf("Catalog() called %d times, want 1", catFake.calls)
	}

	if repFake.calls != 1 {
		t.Errorf("Report() called %d times, want 1", repFake.calls)
	}

	if diff := cmp.Diff(meta, repFake.gotMeta); diff != "" {
		t.Errorf("Report() metadata mismatch (-want +got):\n%s", diff)
	}

	if repFake.gotWriter != io.Writer(&buf) {
		t.Errorf("Report() got writer %T, want the io.Writer Run was given", repFake.gotWriter)
	}

	want := assessment.Assess(inv, cat)
	if diff := cmp.Diff(want, repFake.gotAssessment); diff != "" {
		t.Errorf("Report() assessment mismatch (-want +got):\n%s", diff)
	}

	if got := buf.String(); got != reportBody {
		t.Errorf("Run() wrote %q, want the reporter's output %q", got, reportBody)
	}
}

// TestRunBlockersAreNotAnError pins the exit-code rule: an assessment full of
// blockers is a result, not a failure.
func TestRunBlockersAreNotAnError(t *testing.T) {
	t.Parallel()

	inv := awsInventory(t)
	cat := scalewayCatalog(t)

	if blockers := assessment.Assess(inv, cat).Blockers; len(blockers) == 0 {
		t.Fatal("precondition failed: the fixture assessment has no blockers")
	}

	rep := &fakeReporter{body: reportBody}
	useCase, err := analysis.New(&fakeInventory{inv: inv}, &fakeCatalog{cat: cat}, rep)
	if err != nil {
		t.Fatalf("New() returned unexpected error: %v", err)
	}

	var buf bytes.Buffer

	if err := useCase.Run(context.Background(), &buf, analysis.ReportMeta{InventoryName: inventoryName}); err != nil {
		t.Fatalf("Run() returned unexpected error: %v, want nil for an assessment with blockers", err)
	}

	if rep.calls != 1 {
		t.Errorf("Report() called %d times, want 1", rep.calls)
	}

	if got := buf.String(); got != reportBody {
		t.Errorf("Run() wrote %q, want the reporter's output %q", got, reportBody)
	}
}

// TestRunPortErrors pins that a failing port stops the run and that the error
// says which stage failed.
func TestRunPortErrors(t *testing.T) {
	t.Parallel()

	errInventory := errors.New("inventory exploded")
	errCatalog := errors.New("catalog exploded")
	errReport := errors.New("report exploded")

	tests := []struct {
		name        string
		invErr      error
		catErr      error
		repErr      error
		wantErr     error
		wantPrefix  string
		wantCalled  []string
		wantWritten bool
	}{
		{
			name:       "inventory error",
			invErr:     errInventory,
			wantErr:    errInventory,
			wantPrefix: "load inventory: ",
			wantCalled: []string{"inventory"},
		},
		{
			name:       "catalog error",
			catErr:     errCatalog,
			wantErr:    errCatalog,
			wantPrefix: "load catalog: ",
			wantCalled: []string{"inventory", "catalog"},
		},
		{
			name:        "report error",
			repErr:      errReport,
			wantErr:     errReport,
			wantPrefix:  "render report: ",
			wantCalled:  []string{"inventory", "catalog", "report"},
			wantWritten: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			invFake := &fakeInventory{inv: awsInventory(t), err: tt.invErr, rec: rec}
			catFake := &fakeCatalog{cat: scalewayCatalog(t), err: tt.catErr, rec: rec}
			repFake := &fakeReporter{body: reportBody, err: tt.repErr, rec: rec}

			useCase, err := analysis.New(invFake, catFake, repFake)
			if err != nil {
				t.Fatalf("New() returned unexpected error: %v", err)
			}

			var buf bytes.Buffer

			err = useCase.Run(context.Background(), &buf, analysis.ReportMeta{InventoryName: inventoryName})
			if err == nil {
				t.Fatal("Run() returned nil, want an error")
			}

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Run() error = %v, want it to wrap %v", err, tt.wantErr)
			}

			if !strings.HasPrefix(err.Error(), tt.wantPrefix) {
				t.Errorf("Run() error = %q, want it to start with %q", err, tt.wantPrefix)
			}

			if diff := cmp.Diff(tt.wantCalled, rec.calls()); diff != "" {
				t.Errorf("port call order mismatch (-want +got):\n%s", diff)
			}

			if got := buf.Len() > 0; got != tt.wantWritten {
				t.Errorf("Run() wrote to the writer = %t, want %t", got, tt.wantWritten)
			}
		})
	}
}

// TestRunReportMeta pins the inventory-name rule: the reporter is never called
// with metadata the report cannot carry safely. The name must be a base name, so
// a path to the inventory file is rejected too, and only names rejected as free
// text also wrap text.ErrInvalid.
func TestRunReportMeta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		meta analysis.ReportMeta
		// wantErr is true when the metadata must be rejected.
		wantErr bool
		// wantTextInvalid is true when the rejection is a free-text one, so the
		// error wraps text.ErrInvalid. A name that is valid free text but not a base
		// name is rejected without it.
		wantTextInvalid bool
	}{
		{name: "ordinary name", meta: analysis.ReportMeta{InventoryName: inventoryName}},
		{name: "no name for a non-file source", meta: analysis.ReportMeta{InventoryName: ""}},
		{
			name:            "right-to-left override",
			meta:            analysis.ReportMeta{InventoryName: "inv\u202enosj.json"},
			wantErr:         true,
			wantTextInvalid: true,
		},
		{
			name:            "newline",
			meta:            analysis.ReportMeta{InventoryName: "a\nb.json"},
			wantErr:         true,
			wantTextInvalid: true,
		},
		{
			name:            "leading space",
			meta:            analysis.ReportMeta{InventoryName: " x.json"},
			wantErr:         true,
			wantTextInvalid: true,
		},
		{
			name:            "trailing space",
			meta:            analysis.ReportMeta{InventoryName: "x.json "},
			wantErr:         true,
			wantTextInvalid: true,
		},
		// A path is the file the inventory was read from, not its name: a report
		// that showed it would leak the analyst's directory layout. Each is valid
		// free text, so the rejection must not depend on text.Validate.
		{name: "path with a directory", meta: analysis.ReportMeta{InventoryName: "dir/inventory.json"}, wantErr: true},
		{name: "windows path", meta: analysis.ReportMeta{InventoryName: `C:\data\inventory.json`}, wantErr: true},
		{name: "current directory", meta: analysis.ReportMeta{InventoryName: "."}, wantErr: true},
		{name: "parent directory", meta: analysis.ReportMeta{InventoryName: ".."}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			invFake := &fakeInventory{inv: awsInventory(t), rec: rec}
			catFake := &fakeCatalog{cat: scalewayCatalog(t), rec: rec}
			repFake := &fakeReporter{body: reportBody, rec: rec}

			useCase, err := analysis.New(invFake, catFake, repFake)
			if err != nil {
				t.Fatalf("New() returned unexpected error: %v", err)
			}

			var buf bytes.Buffer

			err = useCase.Run(context.Background(), &buf, tt.meta)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Run() returned unexpected error: %v", err)
				}

				if repFake.calls != 1 {
					t.Errorf("Report() called %d times, want 1", repFake.calls)
				}

				return
			}

			if err == nil {
				t.Fatal("Run() returned nil, want an invalid-report-metadata error")
			}

			if !errors.Is(err, analysis.ErrInvalidReportMeta) {
				t.Errorf("Run() error = %v, want it to wrap analysis.ErrInvalidReportMeta", err)
			}

			if got := errors.Is(err, text.ErrInvalid); got != tt.wantTextInvalid {
				t.Errorf("Run() error wraps text.ErrInvalid = %t, want %t (error: %v)", got, tt.wantTextInvalid, err)
			}

			if quoted := text.Quote(tt.meta.InventoryName); !strings.Contains(err.Error(), quoted) {
				t.Errorf("Run() error = %q, want it to contain the quoted name %s", err, quoted)
			}

			if calls := rec.calls(); len(calls) != 0 {
				t.Errorf("ports called %v, want none for invalid report metadata", calls)
			}

			if buf.Len() != 0 {
				t.Errorf("Run() wrote %q, want nothing for invalid report metadata", buf.String())
			}
		})
	}
}

// TestRunContext pins that a cancelled run never renders a report, whether the
// adapters honour the context or the use case has to check it itself.
func TestRunContext(t *testing.T) {
	t.Parallel()

	t.Run("cancelled before the run", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// The fakes ignore the context, so only Run can notice the cancellation.
		// It must notice it before the first port is called, not after loading an
		// inventory it is not going to report.
		rec := &recorder{}
		invFake := &fakeInventory{inv: awsInventory(t), ignoreContext: true, rec: rec}
		catFake := &fakeCatalog{cat: scalewayCatalog(t), ignoreContext: true, rec: rec}
		repFake := &fakeReporter{body: reportBody, rec: rec}

		useCase, err := analysis.New(invFake, catFake, repFake)
		if err != nil {
			t.Fatalf("New() returned unexpected error: %v", err)
		}

		var buf bytes.Buffer

		err = useCase.Run(ctx, &buf, analysis.ReportMeta{InventoryName: inventoryName})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want it to wrap context.Canceled", err)
		}

		if calls := rec.calls(); len(calls) != 0 {
			t.Errorf("ports called %v, want none for a run cancelled before it started", calls)
		}

		if invFake.calls != 0 {
			t.Errorf("Inventory() called %d times, want 0 for a run cancelled before it started", invFake.calls)
		}

		if repFake.calls != 0 {
			t.Errorf("Report() called %d times, want 0 for a cancelled run", repFake.calls)
		}

		if buf.Len() != 0 {
			t.Errorf("Run() wrote %q, want nothing for a cancelled run", buf.String())
		}
	})

	t.Run("cancelled while loading the catalog", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// The fakes ignore the context, like an adapter that only discovers the
		// cancellation through a slow call: Run must check the context itself
		// before rendering.
		invFake := &fakeInventory{inv: awsInventory(t), ignoreContext: true}
		catFake := &fakeCatalog{cat: scalewayCatalog(t), ignoreContext: true, onCall: cancel}
		repFake := &fakeReporter{body: reportBody}

		useCase, err := analysis.New(invFake, catFake, repFake)
		if err != nil {
			t.Fatalf("New() returned unexpected error: %v", err)
		}

		var buf bytes.Buffer

		err = useCase.Run(ctx, &buf, analysis.ReportMeta{InventoryName: inventoryName})
		if err == nil {
			t.Fatal("Run() returned nil, want an error for a context cancelled mid-run")
		}

		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want it to wrap context.Canceled", err)
		}

		// Nothing was rendered, so the run did not fail at the reporter: the error
		// must not blame rendering.
		if prefix := "render report: "; strings.HasPrefix(err.Error(), prefix) {
			t.Errorf("Run() error = %q, want it not to start with %q for a cancellation before the report", err, prefix)
		}

		if repFake.calls != 0 {
			t.Errorf("Report() called %d times, want 0 for a cancelled run", repFake.calls)
		}

		if buf.Len() != 0 {
			t.Errorf("Run() wrote %q, want nothing for a cancelled run", buf.String())
		}
	})
}

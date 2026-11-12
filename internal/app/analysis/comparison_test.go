package analysis_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/internal/app/analysis"
	"github.com/timothymamo/shift2eu/internal/domain/assessment"
	"github.com/timothymamo/shift2eu/internal/domain/catalog"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	"github.com/timothymamo/shift2eu/internal/domain/text"
	"github.com/timothymamo/shift2eu/internal/testutil"
)

// The keys the comparison tests compare their targets by: the CLI's keys, lower
// case and one word, not the catalogs' display names.
const (
	ovhcloudKey = "ovhcloud"
	scalewayKey = "scaleway"
	upcloudKey  = "upcloud"
)

// comparisonBody is what the fake comparison reporter writes, so a test can see
// that the writer the use case passes on is the caller's.
const comparisonBody = "# shift2eu comparison report\n"

// fakeComparisonReporter is an in-memory ComparisonReporter. It keeps what the
// use case handed it and writes body to the writer, so a test can observe both
// sides.
type fakeComparisonReporter struct {
	body  string
	err   error
	calls int

	gotComparison assessment.Comparison
	gotMeta       analysis.ReportMeta
	gotWriter     io.Writer
}

// The use case must accept a fake as its reporter: this pins the port's shape.
var _ analysis.ComparisonReporter = (*fakeComparisonReporter)(nil)

func (f *fakeComparisonReporter) ReportComparison(
	_ context.Context, w io.Writer, c assessment.Comparison, meta analysis.ReportMeta,
) error {
	f.calls++
	f.gotComparison = c
	f.gotMeta = meta
	f.gotWriter = w

	// The body is written before an error is returned: a renderer that fails may
	// have written part of its output.
	_, writeErr := io.WriteString(w, f.body)

	if f.err != nil {
		return f.err
	}

	return writeErr
}

// comparisonInventory is the inventory the comparison tests assess: it holds a
// resource every target blocks (the serverless function and the unclassified
// one), a database only UpCloud blocks and resources every target maps, so a
// shared blocker is told apart from a merely blocked resource.
func comparisonInventory(t *testing.T) inventory.Inventory {
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
			ID:           "db-example",
			RawType:      "AWS::RDS::DBInstance",
			Category:     taxonomy.DatabasePostgreSQL,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.PointInTimeRestore, taxonomy.ReadReplicas},
		},
		{
			ID:       "fn-example",
			RawType:  "AWS::Lambda::Function",
			Category: taxonomy.ServerlessFunction,
			Region:   "eu-west-1",
		},
		{
			ID:       "misc-example",
			RawType:  "AWS::CloudFormation::CustomResource",
			Category: taxonomy.Unclassified,
			Region:   "eu-west-1",
		},
		{
			ID:           "partial-example",
			RawType:      "AWS::EC2::Instance",
			Category:     taxonomy.ComputeVM,
			Region:       "eu-west-1",
			Capabilities: []taxonomy.Capability{taxonomy.Snapshots},
		},
	})
	if err != nil {
		t.Fatalf("inventory.New() returned unexpected error: %v", err)
	}

	return inv
}

// comparisonCatalog returns the fixture catalog of one target key.
func comparisonCatalog(t *testing.T, key string) catalog.Catalog {
	t.Helper()

	switch key {
	case ovhcloudKey:
		return testutil.OVHcloudCatalog(t)
	case scalewayKey:
		return testutil.ScalewayCatalog(t)
	case upcloudKey:
		return testutil.UpCloudCatalog(t)
	default:
		t.Fatalf("no fixture catalog for target key %q", key)

		return catalog.Catalog{}
	}
}

// newTarget is one target to compare, with a fake catalog source over the
// fixture catalog for its key.
func newTarget(t *testing.T, key string) analysis.Target {
	t.Helper()

	return analysis.Target{Key: key, Catalog: &fakeCatalog{cat: comparisonCatalog(t, key)}}
}

// callLog is a shared log of the target keys whose catalogs the use case loaded,
// in the order it loaded them. The catalog sources of one comparison append to
// the same log, so a test pins which catalogs were loaded, how often and in what
// order, not merely that some load happened.
//
// It is mutex-guarded because the sources it serves are separate objects the use
// case calls in turn: nothing stops that from happening on more than one
// goroutine later, and a race in the test itself would hide a race in the code.
type callLog struct {
	mu   sync.Mutex
	keys []string
}

// add appends key to the log.
func (l *callLog) add(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.keys = append(l.keys, key)
}

// all returns a copy of the logged keys, in the order they were logged.
func (l *callLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return slices.Clone(l.keys)
}

// loggingCatalog returns a fake catalog source over the fixture catalog of key
// that logs key on every load and then runs onCall, which a test uses to cancel
// the context or to fail the load. onCall may be nil.
func loggingCatalog(t *testing.T, key string, log *callLog, onCall func()) *fakeCatalog {
	t.Helper()

	return &fakeCatalog{
		cat: comparisonCatalog(t, key),
		onCall: func() {
			log.add(key)

			if onCall != nil {
				onCall()
			}
		},
	}
}

// comparisonFakes returns the targets to compare in the given order, each with
// its own fake catalog source, those fakes by key so a test can count their
// loads and make one of them fail, and the log they share, so a test can pin the
// order the catalogs were loaded in.
func comparisonFakes(t *testing.T, keys ...string) ([]analysis.Target, map[string]*fakeCatalog, *callLog) {
	t.Helper()

	targets := make([]analysis.Target, 0, len(keys))
	catalogs := make(map[string]*fakeCatalog, len(keys))
	log := &callLog{}

	for _, key := range keys {
		fake := loggingCatalog(t, key, log, nil)
		targets = append(targets, analysis.Target{Key: key, Catalog: fake})
		catalogs[key] = fake
	}

	return targets, catalogs, log
}

// comparisonKeys returns the keys of a comparison's targets, in the order they
// hold.
func comparisonKeys(c assessment.Comparison) []string {
	if len(c.Targets) == 0 {
		return nil
	}

	keys := make([]string, 0, len(c.Targets))
	for _, target := range c.Targets {
		keys = append(keys, target.Key)
	}

	return keys
}

// sharedBlockerIDs returns the resource IDs of a comparison's shared blockers, in
// the order they hold.
func sharedBlockerIDs(c assessment.Comparison) []string {
	if len(c.SharedBlockers) == 0 {
		return nil
	}

	ids := make([]string, 0, len(c.SharedBlockers))
	for _, blocker := range c.SharedBlockers {
		ids = append(ids, blocker.ResourceID)
	}

	return ids
}

// catalogCalls is the number of times the fake catalog sources were called.
func catalogCalls(catalogs map[string]*fakeCatalog) int {
	calls := 0

	for _, fake := range catalogs {
		calls += fake.calls
	}

	return calls
}

// TestNewComparison pins the construction rules: every port is required, and the
// targets must be a non-empty list of distinct keys that are valid free text.
func TestNewComparison(t *testing.T) {
	t.Parallel()

	inv := comparisonInventory(t)

	tests := []struct {
		name    string
		inv     analysis.InventorySource
		targets func(t *testing.T) []analysis.Target
		rep     analysis.ComparisonReporter
		// wantNilPort is true when the error must wrap analysis.ErrNilPort.
		wantNilPort bool
		// wantInvalidTargets is true when the error must wrap analysis.ErrInvalidTargets.
		wantInvalidTargets bool
		// wantKey, when set, must appear in the error message.
		wantKey string
	}{
		{
			name: "nil inventory source",
			targets: func(t *testing.T) []analysis.Target {
				t.Helper()

				return []analysis.Target{newTarget(t, scalewayKey)}
			},
			rep:         &fakeComparisonReporter{body: comparisonBody},
			wantNilPort: true,
		},
		{
			name: "nil reporter",
			inv:  &fakeInventory{inv: inv},
			targets: func(t *testing.T) []analysis.Target {
				t.Helper()

				return []analysis.Target{newTarget(t, scalewayKey)}
			},
			wantNilPort: true,
		},
		{
			name:        "target without a catalog",
			inv:         &fakeInventory{inv: inv},
			targets:     func(_ *testing.T) []analysis.Target { return []analysis.Target{{Key: upcloudKey}} },
			rep:         &fakeComparisonReporter{body: comparisonBody},
			wantNilPort: true,
			wantKey:     upcloudKey,
		},
		{
			name:               "no targets",
			inv:                &fakeInventory{inv: inv},
			targets:            func(_ *testing.T) []analysis.Target { return nil },
			rep:                &fakeComparisonReporter{body: comparisonBody},
			wantInvalidTargets: true,
		},
		{
			name: "empty key",
			inv:  &fakeInventory{inv: inv},
			targets: func(t *testing.T) []analysis.Target {
				t.Helper()

				return []analysis.Target{{Key: "", Catalog: newTarget(t, scalewayKey).Catalog}}
			},
			rep:                &fakeComparisonReporter{body: comparisonBody},
			wantInvalidTargets: true,
		},
		{
			name: "duplicate key",
			inv:  &fakeInventory{inv: inv},
			targets: func(t *testing.T) []analysis.Target {
				t.Helper()

				return []analysis.Target{newTarget(t, scalewayKey), newTarget(t, scalewayKey)}
			},
			rep:                &fakeComparisonReporter{body: comparisonBody},
			wantInvalidTargets: true,
			wantKey:            scalewayKey,
		},
		{
			// A key is free text a report may carry: leading white space is not
			// text.Validate's text.
			name: "key that is not valid free text",
			inv:  &fakeInventory{inv: inv},
			// The key is rejected before its catalog is ever looked up, so the
			// catalog source is the fixture one of a valid key.
			targets: func(t *testing.T) []analysis.Target {
				t.Helper()

				return []analysis.Target{{Key: " ovhcloud", Catalog: newTarget(t, ovhcloudKey).Catalog}}
			},
			rep:                &fakeComparisonReporter{body: comparisonBody},
			wantInvalidTargets: true,
			wantKey:            " ovhcloud",
		},
		{
			name: "three distinct targets",
			inv:  &fakeInventory{inv: inv},
			targets: func(t *testing.T) []analysis.Target {
				t.Helper()

				return []analysis.Target{newTarget(t, upcloudKey), newTarget(t, ovhcloudKey), newTarget(t, scalewayKey)}
			},
			rep: &fakeComparisonReporter{body: comparisonBody},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := analysis.NewComparison(tt.inv, tt.targets(t), tt.rep)

			if !tt.wantNilPort && !tt.wantInvalidTargets {
				if err != nil {
					t.Fatalf("NewComparison() returned unexpected error: %v", err)
				}

				if got == nil {
					t.Fatal("NewComparison() = nil, want a usable Comparison")
				}

				return
			}

			if err == nil {
				t.Fatal("NewComparison() returned nil, want an error")
			}

			if got != nil {
				t.Errorf("NewComparison() = %v, want nil Comparison for a rejected configuration", got)
			}

			if want := tt.wantNilPort; errors.Is(err, analysis.ErrNilPort) != want {
				t.Errorf("NewComparison() error wraps analysis.ErrNilPort = %t, want %t (error: %v)",
					!want, want, err)
			}

			if want := tt.wantInvalidTargets; errors.Is(err, analysis.ErrInvalidTargets) != want {
				t.Errorf("NewComparison() error wraps analysis.ErrInvalidTargets = %t, want %t (error: %v)",
					!want, want, err)
			}

			if tt.wantKey != "" && !strings.Contains(err.Error(), tt.wantKey) {
				t.Errorf("NewComparison() error = %q, want it to name the target key %q", err, tt.wantKey)
			}
		})
	}
}

// TestComparisonRun pins the one comparison run, whatever order the targets were
// given in: the inventory is read once and each catalog once, every target gets
// its own assessment, and the reporter receives one comparison in key order with
// the caller's writer and metadata.
func TestComparisonRun(t *testing.T) {
	t.Parallel()

	inv := comparisonInventory(t)
	meta := analysis.ReportMeta{InventoryName: inventoryName}

	// The comparison's target order, whatever order the caller used.
	sortedKeys := []string{ovhcloudKey, scalewayKey, upcloudKey}

	tests := []struct {
		name string
		gave []string
	}{
		{name: "already in key order", gave: []string{ovhcloudKey, scalewayKey, upcloudKey}},
		{name: "in another order", gave: []string{upcloudKey, ovhcloudKey, scalewayKey}},
		{name: "reversed", gave: []string{upcloudKey, scalewayKey, ovhcloudKey}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			invFake := &fakeInventory{inv: inv}
			targets, _, log := comparisonFakes(t, tt.gave...)
			repFake := &fakeComparisonReporter{body: comparisonBody}

			useCase, err := analysis.NewComparison(invFake, targets, repFake)
			if err != nil {
				t.Fatalf("NewComparison() returned unexpected error: %v", err)
			}

			var buf bytes.Buffer

			if err := useCase.Run(context.Background(), &buf, meta); err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}

			// One read of the inventory serves every target.
			if invFake.calls != 1 {
				t.Errorf("Inventory() called %d times, want 1 for a run of %d targets", invFake.calls, len(targets))
			}

			// Each target's catalog is loaded exactly once, and the catalogs are
			// loaded in key order whatever order the targets were given in: two
			// failures in a row report as "target <key>: load catalog", so the
			// order decides which target the run blames.
			if diff := cmp.Diff(sortedKeys, log.all()); diff != "" {
				t.Errorf("catalog load order mismatch (-want +got):\n%s", diff)
			}

			if repFake.calls != 1 {
				t.Fatalf("ReportComparison() called %d times, want 1", repFake.calls)
			}

			if diff := cmp.Diff(sortedKeys, comparisonKeys(repFake.gotComparison)); diff != "" {
				t.Errorf("comparison target keys mismatch (-want +got):\n%s", diff)
			}

			// Each assessment is the one that target's own single-target run would
			// give for the same inventory and catalog.
			wantTargets := make([]assessment.TargetAssessment, 0, len(sortedKeys))
			for _, key := range sortedKeys {
				wantTargets = append(wantTargets, assessment.TargetAssessment{
					Key:        key,
					Assessment: assessment.Assess(inv, comparisonCatalog(t, key)),
				})
			}

			want := assessment.Compare(wantTargets)

			if diff := cmp.Diff(want.Targets, repFake.gotComparison.Targets); diff != "" {
				t.Errorf("comparison targets mismatch (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(want.SharedBlockers, repFake.gotComparison.SharedBlockers); diff != "" {
				t.Errorf("shared blockers mismatch (-want +got):\n%s", diff)
			}

			// The fixture shares the serverless function and the unclassified
			// resource; the PostgreSQL database is blocked on UpCloud only.
			if diff := cmp.Diff([]string{"fn-example", "misc-example"}, sharedBlockerIDs(repFake.gotComparison)); diff != "" {
				t.Errorf("shared blocker IDs mismatch (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(meta, repFake.gotMeta); diff != "" {
				t.Errorf("ReportComparison() metadata mismatch (-want +got):\n%s", diff)
			}

			if repFake.gotWriter != io.Writer(&buf) {
				t.Errorf("ReportComparison() got writer %T, want the io.Writer Run was given", repFake.gotWriter)
			}

			if got := buf.String(); got != comparisonBody {
				t.Errorf("Run() wrote %q, want the reporter's output %q", got, comparisonBody)
			}
		})
	}
}

// TestComparisonRunPortErrors pins that a failing port stops the run and that the
// error says which stage failed, naming the target whose catalog failed.
func TestComparisonRunPortErrors(t *testing.T) {
	t.Parallel()

	errInventory := errors.New("inventory exploded")
	errCatalog := errors.New("catalog exploded")
	errReport := errors.New("report exploded")

	tests := []struct {
		name string
		// invErr makes the inventory source fail.
		invErr error
		// failingCatalog, when set, is the key of the target whose catalog fails.
		failingCatalog string
		// repErr makes the reporter fail.
		repErr error
		// wantErr is the error the returned error must wrap.
		wantErr error
		// wantStage must appear in the returned error's message.
		wantStage string
		// wantKey, when set, must appear in the returned error's message.
		wantKey string
		// wantOneLine is true when the message must be a single line: a catalog
		// error carries a provider's own detail, so it must not become several
		// lines the CLI would print at once. A stage without such detail leaves
		// it false and the line count unchecked.
		wantOneLine bool
		// wantCatalogCalls is the exact number of catalog loads. The catalogs are
		// loaded in key order and a failing one stops the run, so the count is
		// fixed: a failure on the first target leaves the later ones unloaded.
		wantCatalogCalls int
		// wantCatalogLog is the exact list of loaded catalogs, in load order: it
		// names which targets were loaded, not merely how many.
		wantCatalogLog  []string
		wantReportCalls int
		wantWritten     bool
	}{
		{
			name:             "inventory error",
			invErr:           errInventory,
			wantErr:          errInventory,
			wantStage:        "load inventory",
			wantCatalogCalls: 0,
			wantReportCalls:  0,
		},
		{
			name:             "catalog error of the first target",
			failingCatalog:   ovhcloudKey,
			wantErr:          errCatalog,
			wantStage:        "load catalog",
			wantKey:          ovhcloudKey,
			wantOneLine:      true,
			wantCatalogCalls: 1,
			wantCatalogLog:   []string{ovhcloudKey},
			wantReportCalls:  0,
		},
		{
			name:             "catalog error of a later target",
			failingCatalog:   scalewayKey,
			wantErr:          errCatalog,
			wantStage:        "load catalog",
			wantKey:          scalewayKey,
			wantOneLine:      true,
			wantCatalogCalls: 2,
			wantCatalogLog:   []string{ovhcloudKey, scalewayKey},
			wantReportCalls:  0,
		},
		{
			name:             "report error",
			repErr:           errReport,
			wantErr:          errReport,
			wantStage:        "render report",
			wantCatalogCalls: 3,
			wantCatalogLog:   []string{ovhcloudKey, scalewayKey, upcloudKey},
			wantReportCalls:  1,
			wantWritten:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			invFake := &fakeInventory{inv: comparisonInventory(t), err: tt.invErr}
			targets, catalogs, log := comparisonFakes(t, upcloudKey, ovhcloudKey, scalewayKey)

			if tt.failingCatalog != "" {
				catalogs[tt.failingCatalog].err = errCatalog
			}

			repFake := &fakeComparisonReporter{body: comparisonBody, err: tt.repErr}

			useCase, err := analysis.NewComparison(invFake, targets, repFake)
			if err != nil {
				t.Fatalf("NewComparison() returned unexpected error: %v", err)
			}

			var buf bytes.Buffer

			err = useCase.Run(context.Background(), &buf, analysis.ReportMeta{InventoryName: inventoryName})
			if err == nil {
				t.Fatal("Run() returned nil, want an error")
			}

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Run() error = %v, want it to wrap %v", err, tt.wantErr)
			}

			if !strings.Contains(err.Error(), tt.wantStage) {
				t.Errorf("Run() error = %q, want it to contain %q", err, tt.wantStage)
			}

			if tt.wantKey != "" && !strings.Contains(err.Error(), tt.wantKey) {
				t.Errorf("Run() error = %q, want it to name the target key %q", err, tt.wantKey)
			}

			if tt.wantOneLine {
				if got := !strings.Contains(err.Error(), "\n"); !got {
					t.Errorf("Run() error is one line = %t, want true (error: %q)", got, err)
				}
			}

			if gotLog := log.all(); len(gotLog) != tt.wantCatalogCalls {
				t.Errorf("Catalog() called %d times, want %d (loaded: %v)", len(gotLog), tt.wantCatalogCalls, gotLog)
			}

			if diff := cmp.Diff(tt.wantCatalogLog, log.all()); diff != "" {
				t.Errorf("catalog loads mismatch (-want +got):\n%s", diff)
			}

			if repFake.calls != tt.wantReportCalls {
				t.Errorf("ReportComparison() called %d times, want %d", repFake.calls, tt.wantReportCalls)
			}

			if got := buf.Len() > 0; got != tt.wantWritten {
				t.Errorf("Run() wrote to the writer = %t, want %t", got, tt.wantWritten)
			}
		})
	}
}

// TestComparisonRunReportMeta pins that the comparison run rejects the same
// metadata Analysis.Run rejects, before any port is called.
func TestComparisonRunReportMeta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		meta analysis.ReportMeta
		// wantErr is true when the metadata must be rejected.
		wantErr bool
	}{
		{name: "ordinary name", meta: analysis.ReportMeta{InventoryName: inventoryName}},
		{name: "no name for a non-file source", meta: analysis.ReportMeta{InventoryName: ""}},
		{name: "path with a directory", meta: analysis.ReportMeta{InventoryName: "dir/inventory.json"}, wantErr: true},
		{
			name:    "right-to-left override",
			meta:    analysis.ReportMeta{InventoryName: "inv\u202enosj.json"},
			wantErr: true,
		},
		{name: "newline", meta: analysis.ReportMeta{InventoryName: "a\nb.json"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			invFake := &fakeInventory{inv: comparisonInventory(t)}
			targets, catalogs, _ := comparisonFakes(t, upcloudKey, ovhcloudKey, scalewayKey)
			repFake := &fakeComparisonReporter{body: comparisonBody}

			useCase, err := analysis.NewComparison(invFake, targets, repFake)
			if err != nil {
				t.Fatalf("NewComparison() returned unexpected error: %v", err)
			}

			var buf bytes.Buffer

			err = useCase.Run(context.Background(), &buf, tt.meta)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Run() returned unexpected error: %v", err)
				}

				if repFake.calls != 1 {
					t.Errorf("ReportComparison() called %d times, want 1", repFake.calls)
				}

				return
			}

			if err == nil {
				t.Fatal("Run() returned nil, want an invalid-report-metadata error")
			}

			if !errors.Is(err, analysis.ErrInvalidReportMeta) {
				t.Errorf("Run() error = %v, want it to wrap analysis.ErrInvalidReportMeta", err)
			}

			if quoted := text.Quote(tt.meta.InventoryName); !strings.Contains(err.Error(), quoted) {
				t.Errorf("Run() error = %q, want it to contain the quoted name %s", err, quoted)
			}

			if invFake.calls != 0 {
				t.Errorf("Inventory() called %d times, want 0 for invalid report metadata", invFake.calls)
			}

			if calls := catalogCalls(catalogs); calls != 0 {
				t.Errorf("Catalog() called %d times, want 0 for invalid report metadata", calls)
			}

			if repFake.calls != 0 {
				t.Errorf("ReportComparison() called %d times, want 0 for invalid report metadata", repFake.calls)
			}

			if buf.Len() != 0 {
				t.Errorf("Run() wrote %q, want nothing for invalid report metadata", buf.String())
			}
		})
	}
}

// TestComparisonRunContext pins when a cancelled run notices the cancellation:
// before the inventory it would not report, and between two targets, where a
// catalog that loaded while the context was cancelled must not let the run read
// the next one.
func TestComparisonRunContext(t *testing.T) {
	t.Parallel()

	t.Run("cancelled before the run", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// The fakes ignore the context, so only Run can notice the cancellation, and
		// it must notice it before the first port is called.
		invFake := &fakeInventory{inv: comparisonInventory(t), ignoreContext: true}
		targets, _, log := comparisonFakes(t, upcloudKey, ovhcloudKey, scalewayKey)
		repFake := &fakeComparisonReporter{body: comparisonBody}

		useCase, err := analysis.NewComparison(invFake, targets, repFake)
		if err != nil {
			t.Fatalf("NewComparison() returned unexpected error: %v", err)
		}

		var buf bytes.Buffer

		err = useCase.Run(ctx, &buf, analysis.ReportMeta{InventoryName: inventoryName})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want it to wrap context.Canceled", err)
		}

		if invFake.calls != 0 {
			t.Errorf("Inventory() called %d times, want 0 for a run cancelled before it started", invFake.calls)
		}

		if diff := cmp.Diff([]string(nil), log.all()); diff != "" {
			t.Errorf("catalog loads mismatch (-want +got):\n%s", diff)
		}

		if repFake.calls != 0 {
			t.Errorf("ReportComparison() called %d times, want 0 for a cancelled run", repFake.calls)
		}

		if buf.Len() != 0 {
			t.Errorf("Run() wrote %q, want nothing for a cancelled run", buf.String())
		}
	})

	t.Run("cancelled while loading the first target's catalog", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// The first catalog in key order, ovhcloud's, returns its catalog normally
		// and cancels the context on the way out. The fakes ignore the context, like
		// an adapter that only discovers the cancellation through a slow call, so
		// only Run itself can refuse to load the catalogs that follow.
		log := &callLog{}

		ovhFake := loggingCatalog(t, ovhcloudKey, log, cancel)
		scalewayFake := loggingCatalog(t, scalewayKey, log, nil)
		upcloudFake := loggingCatalog(t, upcloudKey, log, nil)

		invFake := &fakeInventory{inv: comparisonInventory(t), ignoreContext: true}

		fakes := []*fakeCatalog{ovhFake, scalewayFake, upcloudFake}
		for _, fake := range fakes {
			fake.ignoreContext = true
		}

		targets := []analysis.Target{
			{Key: ovhcloudKey, Catalog: ovhFake},
			{Key: scalewayKey, Catalog: scalewayFake},
			{Key: upcloudKey, Catalog: upcloudFake},
		}

		repFake := &fakeComparisonReporter{body: comparisonBody}

		useCase, err := analysis.NewComparison(invFake, targets, repFake)
		if err != nil {
			t.Fatalf("NewComparison() returned unexpected error: %v", err)
		}

		var buf bytes.Buffer

		err = useCase.Run(ctx, &buf, analysis.ReportMeta{InventoryName: inventoryName})
		if err == nil {
			t.Fatal("Run() returned nil, want an error for a context cancelled between two targets")
		}

		// The cancellation must be found before the next catalog is loaded, as a
		// wrapped context.Canceled the caller can test for.
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() error = %v, want it to wrap context.Canceled", err)
		}

		if strings.Contains(err.Error(), "\n") {
			t.Errorf("Run() error is one line = false, want true (error: %q)", err)
		}

		// The error names the target the run stopped before, through text.Quote as
		// every target key a message carries is, and says what it did not do. That
		// tells this cancellation apart from the one before rendering, which blames
		// no target.
		for _, want := range []string{
			"cancelled before loading the catalog",
			text.Quote(scalewayKey),
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Run() error = %q, want it to contain %q", err, want)
			}
		}

		// Only the first catalog was loaded: the run stopped before the others, so
		// neither their load nor their assessment could run.
		if diff := cmp.Diff([]string{ovhcloudKey}, log.all()); diff != "" {
			t.Errorf("catalog loads mismatch (-want +got):\n%s", diff)
		}

		for _, tt := range []struct {
			key  string
			fake *fakeCatalog
		}{
			{key: scalewayKey, fake: scalewayFake},
			{key: upcloudKey, fake: upcloudFake},
		} {
			if tt.fake.calls != 0 {
				t.Errorf("Catalog() for %s called %d times, want 0 for a run cancelled after the first target",
					tt.key, tt.fake.calls)
			}
		}

		if repFake.calls != 0 {
			t.Errorf("ReportComparison() called %d times, want 0 for a cancelled run", repFake.calls)
		}

		if buf.Len() != 0 {
			t.Errorf("Run() wrote %q, want nothing for a cancelled run", buf.String())
		}
	})
}

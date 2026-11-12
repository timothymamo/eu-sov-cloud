// Package analysis_test stands for a test of the application layer. This file
// holds every import app-tests-use-fakes-not-adapters allows (issue #144), and
// no line carries a want comment: the rule is a strict allow list, so an import
// it names is allowed and one it does not name is refused, and any issue here
// fails the probe.
//
// The allow list is minimal (issue #144's review, finding B1): the standard
// packages are the ones the application layer's tests import today, bytes,
// context, errors, io, slices, strings, sync and testing, and every one of them
// is pure. The domain's list names them all as well, and the five the domain's
// list does not name are the extras
// TestAppTestsRuleAllowsOnlyPurePackagesAndItsExtras holds, each with the reason
// a test gives: bytes and slices for what a test builds and compares, context
// for the context the use case takes, io for the writer a fake writes through,
// sync for a fake's mutex, and testing for the test itself. fmt and time are on
// neither list: a test needs neither, and both can do I/O (fmt.Println,
// fmt.Scanln, time.LoadLocation) or read the clock (time.Now), which no
// forbidigo group refuses in a test.
//
// The module's packages are the application package under test, a domain
// package and the test helpers. go-cmp is the one third-party package a test may
// import (CLAUDE.md's testing section), and the probe imports it through the
// stub module the harness writes.
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
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	"github.com/timothymamo/shift2eu/internal/testutil"
)

// TestProbeUsesTheAllowedImports uses every package the rule allows, so an
// allow list that lost an entry, or a deny entry that shadows one, fails this
// probe. The test itself touches no network, no file and no clock, as the rule
// requires of the application layer's tests.
func TestProbeUsesTheAllowedImports(t *testing.T) {
	t.Parallel()

	analysis.Assess()
	testutil.Fake()

	var (
		mu   sync.Mutex
		buf  bytes.Buffer
		seen []string
	)

	// A fake behind the port the use case declares records what it is handed.
	record := func(category taxonomy.Category) {
		mu.Lock()
		defer mu.Unlock()

		seen = append(seen, string(category))
	}

	record(taxonomy.StorageObject)

	if _, err := io.WriteString(&buf, strings.ToUpper(seen[0])); err != nil {
		t.Fatalf("io.WriteString: %v", err)
	}

	got := []string{
		buf.String(),
		strings.ToLower(strings.Join(seen, ",")),
	}

	want := []string{"STORAGE.OBJECT", "storage.object"}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the imports the rule allows build %v, want %v (-want +got):\n%s", got, want, diff)
	}

	if !slices.Contains(seen, string(taxonomy.StorageObject)) {
		t.Errorf("the fake recorded %v, want it to hold %q", seen, taxonomy.StorageObject)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := ctx.Err(); !errors.Is(err, context.Canceled) {
		t.Errorf("ctx.Err() = %v, want context.Canceled", err)
	}
}

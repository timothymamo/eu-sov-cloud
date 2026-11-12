// Package analysis_test stands for a test of the application layer. This file
// holds every import app-tests-use-fakes-not-adapters allows (issue #144), and
// no line carries a want comment: the rule is a strict allow list, so an import
// it names is allowed and one it does not name is refused, and any issue here
// fails the probe.
//
// The standard packages are the domain's pure ones, shared so app tests and the
// domain agree on what pure means, and the four the application layer's tests
// need beyond them: bytes and slices for what a test builds and compares, sync
// for a fake's mutex, and testing. The module's packages are the application
// package under test, a domain package and the test helpers. go-cmp is the one
// third-party package a test may import (CLAUDE.md's testing section), and the
// probe imports it through the stub module the harness writes.
package analysis_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/internal/app/analysis"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	"github.com/timothymamo/shift2eu/internal/testutil"
)

// TestProbeUsesTheAllowedImports uses every package the rule allows, so an
// allow list that lost an entry, or a deny entry that shadows one (net$ beside
// net/url and net/netip), fails this probe. The test itself touches no network,
// no file and no clock, as the rule requires of the application layer's tests.
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

	if got, want := buf.String(), "STORAGE.OBJECT"; got != want {
		t.Fatalf("io.WriteString(buf, strings.ToUpper(%q)) = %q, want %q", seen[0], got, want)
	}

	u, err := url.Parse("https://example.com/" + strconv.Itoa(utf8.RuneCountInString(buf.String())))
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	got := []string{
		u.Host,
		netip.MustParseAddr("192.0.2.1").String(),
		strings.ToLower(string(unicode.ToUpper('A'))),
		fmt.Sprintf("%s", slices.Max([]string{"a", "b"})),
		time.Duration(0).String(),
	}

	want := []string{"example.com", "192.0.2.1", "a", "b", "0s"}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the imports the rule allows build %v, want %v (-want +got):\n%s", got, want, diff)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := ctx.Err(); !errors.Is(err, context.Canceled) {
		t.Errorf("ctx.Err() = %v, want context.Canceled", err)
	}
}

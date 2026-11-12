// White-box tests of the concurrency the composition root serves assessments
// with: the --concurrency flag, zero meaning the computed default, and the
// default itself. defaultConcurrency is pure, so a test reads it directly; the
// flag is driven through run, the only seam the binary has.
package main

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

const (
	// maximalRequestPeak is the memory the test budgets for one assessment of
	// every target over a maximal inventory: the peak
	// BenchmarkMaximalAllComparison in test/apiparity measures (about
	// 1,053,934,556 bytes allocated per request on linux/arm64) rounded down to
	// a round number, so an implementation that budgets the benchmark's peak
	// keeps more headroom than this test demands and one that budgets far less
	// is caught.
	maximalRequestPeak = 1000 << 20

	// mib and gib are the units the cases below read their limits in.
	mib = 1 << 20
	gib = 1 << 30
)

// TestDefaultConcurrency pins the four properties defaultConcurrency must hold
// (m3-context.md): its result is at least 1, it never exceeds the CPU count, it
// grows or stays level with the memory limit, and a set GOMEMLIMIT never lets
// more maximal requests run at once than fit in it.
func TestDefaultConcurrency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cpus int
		// memLimit is what debug.SetMemoryLimit(-1) reports: math.MaxInt64 when
		// GOMEMLIMIT is unset.
		memLimit int64
		// want is the exact result, or 0 when only the bounds below apply.
		want int
		// wantMax is an upper bound beside want, for a case whose exact value
		// depends on the peak the implementation budgets.
		wantMax int
	}{
		{
			// Without GOMEMLIMIT the server picks a conservative budget of its
			// own (ADR 0019), so only the CPU bound is asserted here.
			name:     "no GOMEMLIMIT, many CPUs",
			cpus:     64,
			memLimit: math.MaxInt64,
		},
		{
			name:     "no GOMEMLIMIT, one CPU",
			cpus:     1,
			memLimit: math.MaxInt64,
			want:     1,
		},
		{
			// Less than one maximal request's peak, so one slot, not zero: a
			// server that refuses every assessment is useless.
			name:     "half a gigabyte, many CPUs",
			cpus:     64,
			memLimit: 512 * mib,
			want:     1,
		},
		{
			// Room for more than one maximal request, fewer than the CPUs.
			name:     "four gigabytes, many CPUs",
			cpus:     64,
			memLimit: 4 * gib,
			wantMax:  4,
		},
		{
			name:     "sixty-four gigabytes, many CPUs",
			cpus:     64,
			memLimit: 64 * gib,
		},
		{
			name:     "sixty-four gigabytes, one CPU",
			cpus:     1,
			memLimit: 64 * gib,
			want:     1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := defaultConcurrency(tc.cpus, tc.memLimit)

			if got < 1 {
				t.Errorf("defaultConcurrency(%d, %d) = %d, want at least 1", tc.cpus, tc.memLimit, got)
			}

			if got > tc.cpus {
				t.Errorf("defaultConcurrency(%d, %d) = %d, want at most the CPU count %d",
					tc.cpus, tc.memLimit, got, tc.cpus)
			}

			if tc.want != 0 && got != tc.want {
				t.Errorf("defaultConcurrency(%d, %d) = %d, want %d", tc.cpus, tc.memLimit, got, tc.want)
			}

			if tc.wantMax != 0 && got > tc.wantMax {
				t.Errorf("defaultConcurrency(%d, %d) = %d, want at most %d: one maximal request takes %d bytes",
					tc.cpus, tc.memLimit, got, tc.wantMax, maximalRequestPeak)
			}

			// More than one maximal request at once must fit in the limit the
			// operator set; one always runs, whatever the limit.
			if tc.memLimit != math.MaxInt64 && got > 1 && int64(got) > tc.memLimit/maximalRequestPeak {
				t.Errorf("defaultConcurrency(%d, %d) = %d allows %d maximal requests of %d bytes each, more than the limit %d holds",
					tc.cpus, tc.memLimit, got, got, maximalRequestPeak, tc.memLimit)
			}
		})
	}
}

// TestDefaultConcurrencyGrowsWithTheLimit pins that the result is non-decreasing
// in the memory limit: a larger GOMEMLIMIT never lowers the number of
// assessments the server allows at once, for any CPU count.
func TestDefaultConcurrencyGrowsWithTheLimit(t *testing.T) {
	t.Parallel()

	limits := []int64{
		64 * mib,
		256 * mib,
		512 * mib,
		1 * gib,
		2 * gib,
		4 * gib,
		8 * gib,
		16 * gib,
		32 * gib,
		64 * gib,
		1 << 40,
		math.MaxInt64,
	}

	for _, cpus := range []int{1, 2, 8, 64} {
		t.Run(strconv.Itoa(cpus)+" CPUs", func(t *testing.T) {
			t.Parallel()

			previous := 0
			previousLimit := limits[0]

			for _, limit := range limits {
				got := defaultConcurrency(cpus, limit)

				if got < previous {
					t.Fatalf("defaultConcurrency(%d, %d) = %d, less than defaultConcurrency(%d, %d) = %d: a larger memory limit must not lower the concurrency",
						cpus, limit, got, cpus, previousLimit, previous)
				}

				previous = got
				previousLimit = limit
			}
		})
	}
}

// TestServerConcurrencyFlag pins --concurrency: zero is the computed default
// (ADR 0019), so the server still serves an assessment with it, a positive value
// is accepted, and a negative or non-numeric one is a usage error rather than a
// server that takes no request at all.
func TestServerConcurrencyFlag(t *testing.T) {
	t.Parallel()

	t.Run("zero means the default", func(t *testing.T) {
		t.Parallel()

		s := startServer(t, loopbackListen, nil, "--concurrency", "0")

		status, body := s.postAssessment(t, assessRequest(t, fixtureInventory(t), "ovhcloud"))

		if status != http.StatusOK {
			t.Errorf("POST /v1/assessments with --concurrency 0 = %d, want %d; body: %q",
				status, http.StatusOK, body)
		}

		if code := s.stop(t); code != 0 {
			t.Errorf("run returned %d after a cancelled context, want 0; stderr = %q", code, s.stderr.String())
		}
	})

	t.Run("one is accepted", func(t *testing.T) {
		t.Parallel()

		s := startServer(t, loopbackListen, nil, "--concurrency", "1")

		status, body := s.postAssessment(t, assessRequest(t, fixtureInventory(t), "ovhcloud"))

		if status != http.StatusOK {
			t.Errorf("POST /v1/assessments with --concurrency 1 = %d, want %d; body: %q",
				status, http.StatusOK, body)
		}
	})

	t.Run("a negative value is a configuration error", func(t *testing.T) {
		t.Parallel()

		got := runOnce(t, nil, "--listen", loopbackListen, "--concurrency", "-1")

		if got.listened {
			t.Fatalf("run listened on %v, want a startup error for a negative --concurrency", got.addr)
		}

		requireStartupError(t, got)

		if !strings.Contains(got.stderr, "concurrency") {
			t.Errorf("stderr = %q, want it to name --concurrency", got.stderr)
		}
	})

	tests := []struct {
		name  string
		value string
	}{
		{name: "a value that is not a number", value: "many"},
		{name: "an empty value", value: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := runOnce(t, nil, "--listen", loopbackListen, "--concurrency", tc.value)

			if got.listened {
				t.Fatalf("run listened on %v, want a usage error for --concurrency %q", got.addr, tc.value)
			}

			requireFlagError(t, got, "concurrency")
		})
	}
}

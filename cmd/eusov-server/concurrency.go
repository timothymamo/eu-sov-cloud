package main

import "math"

const (
	// requestPeakBytes is the memory budgeted for one assessment: a
	// comparison of every target over the largest inventory the reader
	// accepts, the worst case one request can be. BenchmarkMaximalAllComparison
	// (test/apiparity) measures 1,053,936,082 bytes allocated per such request
	// on linux/arm64, which bounds the request's live memory from above; 1 GiB
	// is that, rounded up (ADR 0019).
	requestPeakBytes int64 = 1 << 30

	// fallbackMemoryBudget is the memory the concurrency default is sized
	// against when GOMEMLIMIT is not set: 2 GiB, room for two maximal requests,
	// the memory of a small container. A server that has more sets GOMEMLIMIT
	// or --concurrency (ADR 0019).
	fallbackMemoryBudget int64 = 2 << 30
)

// defaultConcurrency returns how many assessments the server runs at once when
// --concurrency is 0: as many as there are CPUs, and no more maximal requests
// than fit in memLimit, but always at least one. memLimit is the memory budget
// in bytes; math.MaxInt64 is no bound at all.
func defaultConcurrency(cpus int, memLimit int64) int {
	n := max(cpus, 1)

	if fit := memLimit / requestPeakBytes; fit < int64(n) {
		n = int(fit)
	}

	return max(n, 1)
}

// memoryBudget returns the budget defaultConcurrency sizes against, from
// limit, the soft memory limit debug.SetMemoryLimit(-1) reports: limit when
// GOMEMLIMIT is set, which the runtime reports as anything below
// math.MaxInt64, and fallbackMemoryBudget when it is not, since a server that
// knows nothing of its memory must not assume it has plenty.
func memoryBudget(limit int64) int64 {
	if limit == math.MaxInt64 {
		return fallbackMemoryBudget
	}

	return limit
}

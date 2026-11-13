// The probe of the probe (issue #194): the same assembly file under a testdata
// directory, which the go command leaves out of ./... and is no source of a held
// package, so the check of test/lint must let it through while it refuses the
// one beside it at test/e2e/probe_amd64.s; only an import of its path would
// build it, which the check does not cover (a follow-up to #194).
TEXT probe(SB), NOSPLIT, $0-0
	RET

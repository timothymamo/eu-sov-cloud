// The probe of the probe (issue #194): the same assembly file under a testdata
// directory, which is no package source, so the check of test/lint must let it
// through while it refuses the one beside it at test/e2e/probe_amd64.s.
TEXT probe(SB), NOSPLIT, $0-0
	RET

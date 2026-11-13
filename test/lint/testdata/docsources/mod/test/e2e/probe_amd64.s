// A probe for test/lint's refusal of non-Go sources in the doc-only test
// packages (issue #194): an assembly file beside a bodyless Go func needs no
// import, so depguard's test-docs-are-pure allow list of nothing has nothing to
// refuse, and forbidigo sees no call in it. In the layout this probe models, the
// go command would build it as a source of the package beside it; here it sits
// under test/lint/testdata and is never built, so the check refuses it for the
// shape it models: it sits in a held tree and is no Go source.
TEXT probe(SB), NOSPLIT, $0-0
	RET

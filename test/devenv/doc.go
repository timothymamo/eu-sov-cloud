// Package devenv holds the checks of the development environment's files under
// dev/ that need no tool beyond Go: today, the broken Lima templates the
// validate-template job of .github/workflows/vm-tools.yml must see Lima refuse
// (issue #137).
//
// Each template in dev/testdata/lima-invalid is dev/lima.yaml with one known
// breakage. TestInvalidLimaTemplatesDifferOnlyByTheirBreakage pins that each
// one is exactly dev/lima.yaml with its one-line edit applied, and that the
// directory holds a template for every case and nothing else. A template that
// differed in more than its breakage could be refused for another reason, and
// one left behind when dev/lima.yaml changes would no longer test the real
// template. Since dev/lima.yaml itself must pass the same job, a refusal of a
// template can then only come from its breakage. With -update the test writes
// the templates from dev/lima.yaml and the cases instead of comparing.
//
// The package has no production code. The test runs in make test, make check
// and CI's make cover, without the network or credentials; whether Lima
// refuses each template is checked by the job, with the pinned Lima.
package devenv

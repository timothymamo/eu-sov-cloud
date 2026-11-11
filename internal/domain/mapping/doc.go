// Package mapping holds the rules that match one inventory resource against a
// target's catalog through the taxonomy. Every offering that serves the
// resource's category is a candidate, with a fit: direct when it offers every
// capability the resource uses, partial when it lacks some. A resource that is
// unclassified, or whose category no offering serves, has fit none.
//
// Candidates are ordered best first (direct before partial, partial candidates
// missing fewer capabilities first, remaining ties by offering name), and every
// fit comes with the reasons behind it: the capabilities the best candidate
// lacks, the missing offering, or the missing classification. Regions are not
// compared.
//
// A reason is plain text split into parts at the names it mentions. The domain
// never escapes them: renderers build their output from Reason.Parts and escape
// every part for their own format; Reason.String is for logs, errors and JSON.
//
// Domain layer: standard library and other domain packages only, no I/O, no
// clock.
package mapping

// Package mapping holds the rules that match one inventory resource against a
// target's catalog through the taxonomy. Every offering that serves the
// resource's category is a candidate, with a fit: direct when it offers every
// capability the resource uses, partial when it lacks some. A resource that is
// unclassified, or whose category no offering serves, has fit none.
//
// Candidates are ordered best first (direct before partial, then by offering
// name), and every fit comes with the reasons behind it: the capabilities the
// best candidate lacks, the missing offering, or the missing classification.
// Regions are not compared.
//
// Domain layer: standard library and other domain packages only, no I/O.
package mapping

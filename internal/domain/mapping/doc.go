// Package mapping holds the rules that correlate an inventory resource with
// candidate catalog offerings: same taxonomy category, compatible capabilities,
// EU region, and a size that fits. It produces candidates with a fit level
// (direct, partial, none) and the reasons behind it.
//
// Domain layer: standard library only, no I/O.
package mapping

// Package probe stands in for a live source adapter under a rule that holds
// every directory under a/source: its own file imports only what the rule
// allows, and its test files are what reaches for the network.
package probe

import "errors"

// ErrProbe is what the package's own file imports errors for.
var ErrProbe = errors.New("probe")

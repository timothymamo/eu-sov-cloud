// Package skip is the excluded directory: neither it nor its subpackage is held
// by the rule whose negated glob names it.
package skip

import "errors"

// ErrSkip is what the package imports errors for.
var ErrSkip = errors.New("skip")

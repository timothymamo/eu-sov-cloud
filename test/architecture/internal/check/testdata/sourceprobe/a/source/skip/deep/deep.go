// Package deep is the subpackage of the excluded directory: the negated glob
// that takes skip out of the rule takes its subpackages out too.
package deep

import "errors"

// ErrDeep is what the package imports errors for.
var ErrDeep = errors.New("deep")

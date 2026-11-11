// Package integration stands in for an adapter whose network test is behind the
// integration build tag: listed without the tag, its imports are the empty set
// of test files.
package integration

import "errors"

// ErrIntegration is what the package's own file imports errors for.
var ErrIntegration = errors.New("integration")

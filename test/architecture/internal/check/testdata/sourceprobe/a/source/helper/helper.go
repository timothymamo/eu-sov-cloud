// Package helper is the sibling helper directory the rule holds too: an adapter
// may share code through it, so the rule's files: glob must reach it.
package helper

import "errors"

// ErrHelper is what the package imports errors for.
var ErrHelper = errors.New("helper")

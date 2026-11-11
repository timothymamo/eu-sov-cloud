// Package xprobe is probe's twin for an external test package: its own file
// imports only what the rule allows, and its _test.go file is in package
// xprobe_test.
package xprobe

import "errors"

// ErrXProbe is what the package's own file imports errors for.
var ErrXProbe = errors.New("xprobe")

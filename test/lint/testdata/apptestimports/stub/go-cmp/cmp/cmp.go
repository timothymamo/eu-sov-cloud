// Package cmp stands in for github.com/google/go-cmp/cmp, the one third-party
// package an application-layer test may import (CLAUDE.md's testing section),
// so the probe needs it offline: the harness writes this stub module and
// replaces the real path with it (forbidigoProbe.stubs).
package cmp

// Diff stands for the comparison report a test puts in its failure message.
func Diff(_, _ any, _ ...Option) string { return "" }

// Option stands for an option a caller passes to Diff.
type Option interface{}

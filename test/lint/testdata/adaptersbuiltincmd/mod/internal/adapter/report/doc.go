// Package report stands for the package at the root of the report subtree. The
// deny entries of adapters-are-built-in-cmd for the subtrees end in "/", which
// matches a package under the subtree but not the package at its root, so this
// package is what the rule must also refuse (issue #119).
package report

// Name names the subtree root package.
const Name = "report"

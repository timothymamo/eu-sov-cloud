// Package source stands for the package at the root of the source subtree. The
// deny entries of adapters-are-built-in-cmd for the subtrees end in "/", which
// matches a package under the subtree but not the package at its root, so this
// package is what the rule must also refuse (issue #119).
package source

// Name names the subtree root package.
const Name = "source"

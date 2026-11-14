package check

import (
	"slices"
	"strings"
)

// HiddenPathPackages returns the import paths among importPaths that are
// under modulePath and hold, below it, an element the go command skips when it
// walks ./...: one that is exactly testdata or starts with _ or . (issue
// #206). The go command still builds such a package when another package
// imports it by explicit path, so no rule that walks the tree sees it.
//
// Only the elements below the module path count, and a path that only shares
// the module path's prefix as a string, or the module root itself, is not
// under it. The result is sorted, each path once.
func HiddenPathPackages(modulePath string, importPaths []string) []string {
	var hidden []string

	prefix := modulePath + "/"

	for _, importPath := range importPaths {
		rest, ok := strings.CutPrefix(importPath, prefix)
		if !ok {
			continue
		}

		if slices.ContainsFunc(strings.Split(rest, "/"), isHiddenElement) {
			hidden = append(hidden, importPath)
		}
	}

	slices.Sort(hidden)

	return slices.Compact(hidden)
}

// HiddenPathMessages returns one finding per package HiddenPathPackages
// reports among importPaths, sorted, naming rootDir, the composition root's
// directory relative to the module root, and the package it reaches: the
// listing is go list -deps, so a package imported only transitively counts.
func HiddenPathMessages(modulePath, rootDir string, importPaths []string) []string {
	hidden := HiddenPathPackages(modulePath, importPaths)
	messages := make([]string, 0, len(hidden))

	for _, pkg := range hidden {
		messages = append(messages, rootDir+" reaches "+pkg+
			", which ./... never walks but go builds when a package imports it by explicit path: no lint or architecture rule sees it")
	}

	return messages
}

// isHiddenElement reports whether the go command skips a directory of this
// name when it walks a pattern ending in /...: testdata, or a name starting
// with _ or a dot.
func isHiddenElement(element string) bool {
	return element == "testdata" || strings.HasPrefix(element, "_") || strings.HasPrefix(element, ".")
}

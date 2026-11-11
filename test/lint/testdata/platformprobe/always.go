// Package platformprobe is the probe module of TestUnloadedGoFilesProbe: five
// files, each built for a different set of platforms or build settings, so the
// test can pin which of them unloadedGoFiles reports for a list of release
// targets. Its own go.mod keeps it out of the main module's ./... pattern
// (issue #82).
package platformprobe

// Built on every platform no other file here is built for.
const Always = "always"

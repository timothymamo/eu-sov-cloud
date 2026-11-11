//go:build amd64.v3

package platformprobe

// Built for amd64 only at GOAMD64=v3 and above: the release builds every target
// at the default GOAMD64 level, v1, so unloadedGoFiles reports it whatever the
// target list holds.
const AMD64V3 = "amd64.v3"

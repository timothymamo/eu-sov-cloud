//go:build cgo

package platformprobe

// Built only with cgo: the release builds every target with CGO_ENABLED=0, so
// unloadedGoFiles reports it whatever the target list holds.
const CgoOnly = "cgo"

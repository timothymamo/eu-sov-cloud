//go:build plan9

package platformprobe

// Built for plan9 alone: no release target of .goreleaser.yaml builds it, so
// unloadedGoFiles reports it whatever the target list holds.
const Plan9 = "plan9"

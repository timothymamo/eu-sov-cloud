//go:build windows && arm64

package platformprobe

// Built for windows on arm64 alone: the file a target list that leaves
// windows/arm64 out must still report, and one that holds it must not, since
// lint lints it in the windows/arm64 run (issue #82).
const WindowsARM = "windows/arm64"

// Package logging stands in for a third-party library the composition root must
// not import: no rule for cmd/shift2eu allows a package outside the module, so the
// probe's import of it is refused. Test/architecture lists the probe module for
// every platform the release builds.
package logging

// Printf stands in for a library's logging entry point. It writes nothing.
func Printf(format string, args ...any) {
	_, _ = format, args
}

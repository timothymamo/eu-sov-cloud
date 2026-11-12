// Package markdown stands in for a report adapter. It builds, but an
// application-layer test must not import it: the tests use fakes behind the
// application's ports, never an adapter, and the rule refuses every adapter
// package whether or not its deny list names the adapter.
package markdown

// Render stands for the adapter's rendering of a report.
func Render() string { return "" }

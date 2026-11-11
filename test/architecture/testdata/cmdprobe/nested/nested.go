// Package nested is a module of its own under the probe's module path,
// example.com/cmdprobe/nested: the probe's cmd/eusov imports it, so the listing
// holds a package under that path that is not the main module's, as a module
// added under the real module path would be. Test/architecture requires the
// composition root's listing to report one (issue #101, security audit LOW-1).
package nested

// Name is what the probe's cmd/eusov prints, so the package is reached.
func Name() string { return "nested" }

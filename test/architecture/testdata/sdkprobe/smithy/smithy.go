// Package smithy stands in for a package the cloud SDK depends on: no
// .golangci.yml rule names it, and depguard lints no third-party file. It is
// the transitive dependency the package check must not report once a held
// adapter imports the SDK through an allowed prefix entry.
package smithy

// Client is the SDK's client base, which the ec2 stand-in builds on.
type Client struct{}

// Method is what the stand-in client offers.
func (Client) Method() string { return "GET" }

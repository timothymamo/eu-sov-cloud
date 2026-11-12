// Package ec2 stands in for a cloud SDK package the composition root must not
// import: a cloud SDK lives only in its provider's adapter (CLAUDE.md), and the
// composition-root rule denies it, as cloud-sdks-only-in-provider-adapters does
// everywhere but the adapters.
package ec2

// Client stands in for an SDK client.
type Client struct{}

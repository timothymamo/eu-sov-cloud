// Package probe is outside the cloud source adapters, where test/lint refuses
// an interface method named for a write and allows any other.
package probe

import "context"

// Port is an interface of the module outside the source adapters.
type Port interface {
	Inventory(ctx context.Context) error
	Settings() string
	RequestSpotInstances(ctx context.Context) error // want "test/lint: an interface method is named for a write"
}

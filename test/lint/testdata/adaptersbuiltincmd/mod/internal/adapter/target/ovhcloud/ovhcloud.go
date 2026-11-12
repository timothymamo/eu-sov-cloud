// Package ovhcloud stands for a target adapter. It imports the shared snapshot
// reader, which adapters-are-built-in-cmd allows, so importing it is not
// reported here.
package ovhcloud

import "github.com/timothymamo/shift2eu/internal/adapter/target/snapshot"

// Name names the adapter.
const Name = snapshot.Name + " ovhcloud"

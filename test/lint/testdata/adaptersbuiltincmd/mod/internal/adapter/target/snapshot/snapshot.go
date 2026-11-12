// Package snapshot stands for the shared target snapshot reader (ADR 0007).
// It is under the target subtree but no target adapter, so it is the second
// helper adapters-are-built-in-cmd names with an exact allow entry.
package snapshot

import "github.com/timothymamo/shift2eu/internal/adapter/internal/jsondoc"

// Name names the reader.
const Name = jsondoc.Name + " snapshot"

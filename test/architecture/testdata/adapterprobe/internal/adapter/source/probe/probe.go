// Package probe is a live source adapter that opens a connection of its own:
// it imports net/http directly instead of reaching its cloud through the SDK,
// which the SDK-only rule refuses. Test/architecture lists it for every
// platform the release builds.
package probe

import "net/http"

// Methods is the package's direct use of the network package.
var Methods = http.MethodGet

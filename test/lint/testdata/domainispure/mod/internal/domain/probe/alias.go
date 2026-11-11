package probe

import clock "time"

// Aliased reads the clock through an aliased import: forbidigo matches the
// package name, not the alias.
func Aliased() clock.Time {
	return clock.Now() // want "domain-is-pure: the domain reads no clock"
}

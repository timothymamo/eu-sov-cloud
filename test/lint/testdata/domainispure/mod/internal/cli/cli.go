// Package cli is outside the domain, so the domain-is-pure group does not
// apply to it: none of these calls is reported.
package cli

import (
	"fmt"
	"time"
)

// Print prints the local time.
func Print() {
	fmt.Println(time.Now().Local())
	_, _ = time.LoadLocation("Europe/Paris")
	_ = time.Local
}

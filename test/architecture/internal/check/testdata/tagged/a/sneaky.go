//go:build sneaky

package a

import "os"

var _ = os.Getenv

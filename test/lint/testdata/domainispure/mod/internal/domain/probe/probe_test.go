package probe_test

import (
	"fmt"
	"testing"
	"time"
)

// A domain test may print and read the clock: none of these is reported.
func TestProbe(t *testing.T) {
	fmt.Println("probe")
	var s string
	_, _ = fmt.Scanln(&s)
	_, _ = time.LoadLocation("Europe/Paris")
	now := time.Now()
	_ = now.In(time.Local)
	_ = now.Local()
}

// A godoc example is a test file too.
func Example() {
	fmt.Println(time.Now().Local())
}

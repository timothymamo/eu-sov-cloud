// Command eusov stands in for the composition root: it imports the standard
// packages cmd/eusov imports today, a third-party library and a cloud SDK it
// must not, and a module of its own that sits under the probe's module path, so
// the listing holds a package the composition-root rule's prefix entry would
// allow and the main module is not (issue #101, security audit LOW-1). Test/
// architecture holds it to the composition-root rule for every platform the
// release builds.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"example.com/cmdprobe/nested"
	"example.com/logging"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

func main() {
	_, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	logging.Printf("%v %v %s", os.Args, ec2.Client{}, nested.Name())

	stop()
}

// Command eusov stands in for the composition root: it imports the standard
// packages cmd/eusov imports today, and a third-party library and a cloud SDK it
// must not. Test/architecture holds it to the composition-root rule for every
// platform the release builds.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"example.com/logging"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

func main() {
	_, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	logging.Printf("%v %v", os.Args, ec2.Client{})

	stop()
}

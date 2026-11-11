// Package awsutil is a sibling of the source adapters, a natural name for a
// shared helper: it is held to the same import rule, and may not import a
// cloud SDK at all.
package awsutil

import (
	"reflect" // want "import 'reflect' is not allowed from list 'cloud-adapters-reach-the-cloud-through-the-sdk'"

	"github.com/aws/aws-sdk-go-v2/service/ec2" // want "import 'github.com/aws/aws-sdk-go-v2/service/ec2' is not allowed from list 'cloud-sdks-only-in-provider-adapters'"
)

// Call calls the client's method by name, which forbidigo cannot see.
func Call(client *ec2.Client, name string) {
	reflect.ValueOf(client).MethodByName(name).Call(nil)
}

// Fleet is a helper type of the sibling package; the adapter-types entry
// covers it as it covers the adapters.
type Fleet struct{}

// Terminate is named for a write.
func (Fleet) Terminate() {}

// Drain calls the helper's write-named method.
func Drain(f Fleet) {
	f.Terminate() // want "cloud-read-only: a source adapter calls no method"
}

// Package ec2 stands in for a cloud SDK package an adapter reaches its cloud
// through: the SDK-only rule allows it with a prefix entry for its root, and it
// imports a second stand-in package, the way the real ec2 package imports the
// SDK's own dependencies.
package ec2

import "github.com/aws/smithy-go"

// Client is the SDK client the probe adapter holds.
type Client = smithy.Client

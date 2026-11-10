package taxonomy

import (
	"errors"
	"fmt"
	"slices"
)

// Capability is a feature a source resource depends on or a target offering
// provides, such as ipv6. Its string value is shared by sources, targets and the
// inventory file format.
type Capability string

// The capability vocabulary.
const (
	// IPv6 is IPv6 connectivity.
	IPv6 Capability = "ipv6"
	// PointInTimeRestore is restoring data to any moment within a retention window.
	PointInTimeRestore Capability = "point-in-time-restore"
	// ObjectVersioning is keeping earlier versions of overwritten or deleted objects.
	ObjectVersioning Capability = "object-versioning"
	// HighAvailability is surviving the loss of a node or zone without manual failover.
	HighAvailability Capability = "high-availability"
	// ReadReplicas is serving reads from replicas of a primary database.
	ReadReplicas Capability = "read-replicas"
	// Autoscaling is adding and removing capacity automatically with load.
	Autoscaling Capability = "autoscaling"
	// EncryptionAtRest is encrypting stored data.
	EncryptionAtRest Capability = "encryption-at-rest"
	// Snapshots is taking point-in-time copies of a volume or instance.
	Snapshots Capability = "snapshots"
	// DeadLetterQueue is setting aside messages that repeatedly fail processing.
	DeadLetterQueue Capability = "dead-letter-queue"
)

// ErrUnknownCapability is returned for a value that is not in the capability
// vocabulary.
var ErrUnknownCapability = errors.New("unknown capability")

// capabilities lists every capability, sorted by value. It is read-only.
var capabilities = []Capability{
	Autoscaling,
	DeadLetterQueue,
	EncryptionAtRest,
	HighAvailability,
	IPv6,
	ObjectVersioning,
	PointInTimeRestore,
	ReadReplicas,
	Snapshots,
}

// ParseCapability returns the capability whose value is exactly s. It does not
// trim or fold case: anything else is an error wrapping ErrUnknownCapability.
func ParseCapability(s string) (Capability, error) {
	c := Capability(s)
	if !slices.Contains(capabilities, c) {
		return "", fmt.Errorf("%w %q", ErrUnknownCapability, s)
	}

	return c, nil
}

// Capabilities returns the whole capability vocabulary, sorted by value. The
// slice is a fresh copy on each call.
func Capabilities() []Capability {
	return slices.Clone(capabilities)
}

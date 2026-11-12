package taxonomy

import (
	"errors"
	"fmt"
	"slices"

	"github.com/timothymamo/shift2eu/internal/domain/text"
)

// Category is a provider-neutral service type, such as compute.vm. Its string
// value is the name sources, targets and the inventory file format share.
type Category string

// The taxonomy categories. Sources classify resources into one of these, and
// targets declare which of them their offerings serve.
const (
	// ComputeVM is a virtual machine or a dedicated server running a general
	// purpose operating system.
	ComputeVM Category = "compute.vm"
	// KubernetesManaged is a Kubernetes cluster whose control plane the provider runs.
	KubernetesManaged Category = "kubernetes.managed"
	// StorageObject is object storage: buckets of objects addressed by key.
	StorageObject Category = "storage.object"
	// StorageBlock is a block volume attached to a machine.
	StorageBlock Category = "storage.block"
	// DatabasePostgreSQL is a PostgreSQL database run by the provider.
	DatabasePostgreSQL Category = "database.postgresql"
	// DatabaseMySQL is a MySQL database run by the provider.
	DatabaseMySQL Category = "database.mysql"
	// ServerlessFunction is code the provider runs on demand, per invocation.
	ServerlessFunction Category = "serverless.function"
	// MessagingQueue is a message queue the provider runs.
	MessagingQueue Category = "messaging.queue"
	// Unclassified marks a source resource that fits no other category. It is a
	// fact about the resource, not a fallback for unknown values, and no target
	// offering ever serves it.
	Unclassified Category = "unclassified"
)

// ErrUnknownCategory is returned for a value that is not a taxonomy category.
var ErrUnknownCategory = errors.New("unknown taxonomy category")

// categories lists every category, sorted by value. It is read-only.
var categories = []Category{
	ComputeVM,
	DatabaseMySQL,
	DatabasePostgreSQL,
	KubernetesManaged,
	MessagingQueue,
	ServerlessFunction,
	StorageBlock,
	StorageObject,
	Unclassified,
}

// ParseCategory returns the category whose value is exactly s. It does not trim
// or fold case, and it never maps an unknown value to Unclassified: anything
// else is an error wrapping ErrUnknownCategory. The error quotes s with
// text.Quote, so it stays short however long s is.
func ParseCategory(s string) (Category, error) {
	c := Category(s)
	if !slices.Contains(categories, c) {
		return "", fmt.Errorf("%w %s", ErrUnknownCategory, text.Quote(s))
	}

	return c, nil
}

// Categories returns every category, sorted by value. The slice is a fresh copy
// on each call.
func Categories() []Category {
	return slices.Clone(categories)
}

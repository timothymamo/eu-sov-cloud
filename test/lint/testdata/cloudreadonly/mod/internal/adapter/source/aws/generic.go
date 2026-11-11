package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
)

// spot is a constraint whose operation types are left to inference from
// *ec2.Client, so no Input or Output type is named, and the call in run is a
// selector on a type parameter, which forbidigo cannot see. test/lint refuses
// the declaration: an interface of a cloud source adapter declares read verbs
// only.
type spot[I, O any] interface {
	RequestSpotInstances(context.Context, I, ...func(*ec2.Options)) (O, error) // want "test/lint: an interface of a cloud source adapter declares only read verbs"
}

func run[I, O any, A spot[I, O]](ctx context.Context, a A) (O, error) {
	var in I
	return a.RequestSpotInstances(ctx, in)
}

// leaver is a generic interface: the call through it is matched against the
// interface, whose verb Leave the adapter-types entry refuses.
type leaver[I, O any] interface {
	LeaveOrganization(context.Context, I, ...func(*organizations.Options)) (O, error) // want "test/lint: an interface of a cloud source adapter declares only read verbs"
}

func leave[I, O any](ctx context.Context, l leaver[I, O]) error {
	var in I
	_, err := l.LeaveOrganization(ctx, in) // want "cloud-read-only: a source adapter calls no method"
	return err
}

// Generic runs both writes with every type argument inferred.
func Generic(ctx context.Context, client *ec2.Client, org *organizations.Client) error {
	if _, err := run(ctx, client); err != nil {
		return err
	}
	var lit interface {
		TerminateInstances(context.Context, any) error // want "test/lint: an interface of a cloud source adapter declares only read verbs"
	}
	_ = lit
	return leave(ctx, org)
}

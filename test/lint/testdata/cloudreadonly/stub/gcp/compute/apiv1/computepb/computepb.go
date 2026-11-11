// Package computepb stands in for cloud.google.com/go/compute/apiv1/computepb
// in the cloud-read-only probe.
package computepb

type Instance struct{}

type GetInstanceRequest struct{}

type DeleteInstanceRequest struct{}

// DeleteInstanceResponse has no counterpart in compute; it stands for the
// <Op>Response of another GCP API.
type DeleteInstanceResponse struct{}

type ListInstancesResponse struct{}

// Package policy stands in for github.com/Azure/azure-sdk-for-go/sdk/azcore/policy in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package policy

import "github.com/Azure/azure-sdk-for-go/sdk/azcore/internal/exported"

type (
	Request             = exported.Request
	Policy              = exported.Policy
	Transporter         = exported.Transporter
	TokenRequestOptions = exported.TokenRequestOptions
)

type ClientOptions struct {
	Transport        Transporter
	PerCallPolicies  []Policy
	PerRetryPolicies []Policy
	Telemetry        struct{ ApplicationID string }
}

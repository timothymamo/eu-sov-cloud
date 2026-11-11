// Package policy stands in for github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package policy

import "github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"

// ClientOptions embeds the azcore options, as the real SDK does.
type ClientOptions struct {
	policy.ClientOptions
	DisableRPRegistration bool
}

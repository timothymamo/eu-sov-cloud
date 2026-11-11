// Package armkeyvault stands in for github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package armkeyvault

import "context"

type SecretsClient struct{}

type Secret struct{}

func (*SecretsClient) Get(context.Context, string, string, string, any) (Secret, error) {
	return Secret{}, nil
}

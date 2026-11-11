// Package armstorage stands in for github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package armstorage

import "context"

type AccountsClient struct{}

type AccountsClientListKeysOptions struct{}

type AccountListKeysResult struct{}

func (*AccountsClient) ListKeys(context.Context, string, string, *AccountsClientListKeysOptions) (AccountListKeysResult, error) {
	return AccountListKeysResult{}, nil
}

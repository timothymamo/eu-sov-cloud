// Package azcore stands in for github.com/Azure/azure-sdk-for-go/sdk/azcore in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package azcore

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/internal/exported"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type TokenCredential = exported.TokenCredential

type ClientOptions = policy.ClientOptions

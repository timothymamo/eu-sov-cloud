// Package arm stands in for github.com/Azure/azure-sdk-for-go/sdk/azcore/arm in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package arm

import armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"

type ClientOptions = armpolicy.ClientOptions

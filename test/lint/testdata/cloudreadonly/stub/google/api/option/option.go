// Package option stands in for google.golang.org/api/option in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package option

type ClientOption interface{}

func WithQuotaProject(string) ClientOption { return nil }

func WithEndpoint(string) ClientOption { return nil }

func WithHTTPClient(any) ClientOption { return nil }

// Package oauth2 stands in for golang.org/x/oauth2 in the
// cloud-read-only probe: the same import path, package name and shapes, with
// no code behind them.
package oauth2

type TokenSource interface{}

func StaticTokenSource(any) TokenSource { return nil }

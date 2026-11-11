// Package exported stands in for the azcore package that declares the types
// azcore and its policy package alias, laid out like the real SDK.
package exported

import "context"

type TokenRequestOptions struct{ Scopes []string }

type AccessToken struct{ Token string }

type TokenCredential interface {
	GetToken(context.Context, TokenRequestOptions) (AccessToken, error)
}

type Request struct{}

type Policy interface {
	Do(*Request) (any, error)
}

type Transporter interface {
	Do(any) (any, error)
}

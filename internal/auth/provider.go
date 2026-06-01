package auth

import "context"

// Identity is the normalised result from any OAuth2/OIDC provider.
type Identity struct {
	Provider   string // "github" | "oidc"
	ProviderID string // stable subject ID
	Email      string
	Name       string
}

// Provider abstracts a single OAuth2/OIDC login flow.
type AuthRequestOptions struct {
	CodeVerifier string
	Nonce        string
}

type ExchangeOptions struct {
	CodeVerifier string
	Nonce        string
}

type Provider interface {
	// Name returns the slug used in callback URLs (e.g. "github").
	Name() string
	// AuthCodeURL returns the redirect URL the browser should visit.
	AuthCodeURL(state string, opts AuthRequestOptions) string
	// Exchange completes the flow: exchanges the code and returns an Identity.
	Exchange(ctx context.Context, code string, opts ExchangeOptions) (Identity, error)
}

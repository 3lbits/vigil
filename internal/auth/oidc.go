package auth

import (
	"context"
	"errors"
	"fmt"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var errNoIDToken = errors.New("oidc: no id_token in response")
var errMissingVerifier = errors.New("oidc: missing code verifier")
var errNonceMismatch = errors.New("oidc: nonce mismatch")

type OIDCProvider struct {
	name     string
	cfg      *oauth2.Config
	verifier *gooidc.IDTokenVerifier
}

// NewOIDCProvider creates a generic OIDC provider. name is the slug used in
// callback URLs and stored as the provider discriminator (e.g. "oidc", "entra").
func NewOIDCProvider(ctx context.Context, name, issuerURL, clientID, clientSecret, callbackURL string) (*OIDCProvider, error) {
	provider, err := gooidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery %s: %w", issuerURL, err)
	}
	return &OIDCProvider{
		name: name,
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  callbackURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{gooidc.ScopeOpenID, "profile", "email"},
		},
		verifier: provider.Verifier(&gooidc.Config{ClientID: clientID}),
	}, nil
}

// NewEntraIDProvider constructs an OIDCProvider pointed at the Microsoft
// Entra ID v2.0 endpoint for the given tenant.
func NewEntraIDProvider(ctx context.Context, tenantID, clientID, clientSecret, callbackURL string) (*OIDCProvider, error) {
	issuer := "https://login.microsoftonline.com/" + tenantID + "/v2.0"
	return NewOIDCProvider(ctx, "entra", issuer, clientID, clientSecret, callbackURL)
}

func (p *OIDCProvider) Name() string { return p.name }

func (p *OIDCProvider) AuthCodeURL(state string, opts AuthRequestOptions) string {
	authOpts := []oauth2.AuthCodeOption{oauth2.AccessTypeOnline}
	if opts.CodeVerifier != "" {
		authOpts = append(authOpts, oauth2.S256ChallengeOption(opts.CodeVerifier))
	}
	if opts.Nonce != "" {
		authOpts = append(authOpts, gooidc.Nonce(opts.Nonce))
	}
	return p.cfg.AuthCodeURL(state, authOpts...)
}

func (p *OIDCProvider) Exchange(ctx context.Context, code string, opts ExchangeOptions) (Identity, error) {
	if opts.CodeVerifier == "" {
		return Identity{}, errMissingVerifier
	}
	token, err := p.cfg.Exchange(ctx, code, oauth2.VerifierOption(opts.CodeVerifier))
	if err != nil {
		return Identity{}, fmt.Errorf("oidc exchange: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return Identity{}, errNoIDToken
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return Identity{}, fmt.Errorf("oidc verify: %w", err)
	}
	if opts.Nonce != "" && idToken.Nonce != opts.Nonce {
		return Identity{}, errNonceMismatch
	}

	var claims struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("oidc claims: %w", err)
	}

	return Identity{
		Provider:   p.name,
		ProviderID: claims.Sub,
		Email:      claims.Email,
		Name:       claims.Name,
	}, nil
}

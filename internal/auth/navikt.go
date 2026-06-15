package auth

import (
	"context"
	"fmt"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
)

// NaviktClaims holds the token claims extracted from a NAIS/Entra ID bearer token.
type NaviktClaims struct {
	NAVident          string   `json:"NAVident"`
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
	Groups            []string `json:"groups"`
}

// NaviktVerifier verifies Entra ID bearer tokens injected by the Wonderwall proxy.
// It validates the signature against the JWKS endpoint and checks iss, aud, exp.
type NaviktVerifier struct {
	verifier *gooidc.IDTokenVerifier
}

// NewNaviktVerifier constructs a NaviktVerifier that fetches signing keys from
// the given JWKS URI and validates against the provided issuer and client ID.
func NewNaviktVerifier(ctx context.Context, issuer, clientID, jwksURI string) *NaviktVerifier {
	keySet := gooidc.NewRemoteKeySet(ctx, jwksURI)
	return newNaviktVerifierWithKeySet(issuer, clientID, keySet)
}

// newNaviktVerifierWithKeySet creates a NaviktVerifier with an explicit key set.
// Used in tests to inject a mock key set.
func newNaviktVerifierWithKeySet(issuer, clientID string, keySet gooidc.KeySet) *NaviktVerifier {
	v := gooidc.NewVerifier(issuer, keySet, &gooidc.Config{ClientID: clientID})
	return &NaviktVerifier{verifier: v}
}

// Verify validates the raw bearer token and extracts NAIS-specific claims.
// It checks signature, issuer, audience, and expiry.
func (v *NaviktVerifier) Verify(ctx context.Context, rawToken string) (NaviktClaims, error) {
	idToken, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return NaviktClaims{}, fmt.Errorf("navikt: verify token: %w", err)
	}
	var claims NaviktClaims
	if err := idToken.Claims(&claims); err != nil {
		return NaviktClaims{}, fmt.Errorf("navikt: extract claims: %w", err)
	}
	return claims, nil
}

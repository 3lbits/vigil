package auth

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestOIDCProviderAuthCodeURLIncludesPKCEAndNonce(t *testing.T) {
	p := &OIDCProvider{
		name: "oidc",
		cfg: &oauth2.Config{
			ClientID: "client",
			Endpoint: oauth2.Endpoint{AuthURL: "https://issuer.example/authorize"},
		},
	}
	url := p.AuthCodeURL("state-123", AuthRequestOptions{
		CodeVerifier: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
		Nonce:        "nonce-123",
	})
	if !strings.Contains(url, "code_challenge=") {
		t.Fatalf("expected PKCE challenge in auth URL, got %s", url)
	}
	if !strings.Contains(url, "code_challenge_method=S256") {
		t.Fatalf("expected PKCE S256 in auth URL, got %s", url)
	}
	if !strings.Contains(url, "nonce=nonce-123") {
		t.Fatalf("expected nonce in auth URL, got %s", url)
	}
}

func TestOIDCProviderExchangeRequiresCodeVerifier(t *testing.T) {
	p := &OIDCProvider{name: "oidc"}
	_, err := p.Exchange(t.Context(), "code-123", ExchangeOptions{})
	if !errors.Is(err, errMissingVerifier) {
		t.Fatalf("expected errMissingVerifier, got %v", err)
	}
}

func TestOIDCProviderName(t *testing.T) {
	p := &OIDCProvider{name: "entra"}
	if got := p.Name(); got != "entra" {
		t.Fatalf("name = %q, want %q", got, "entra")
	}
}

func TestOIDCProviderAuthCodeURLWithoutOptionalParams(t *testing.T) {
	p := &OIDCProvider{
		name: "oidc",
		cfg: &oauth2.Config{
			ClientID: "client",
			Endpoint: oauth2.Endpoint{AuthURL: "https://issuer.example/authorize"},
		},
	}
	url := p.AuthCodeURL("state-123", AuthRequestOptions{})
	if strings.Contains(url, "code_challenge=") {
		t.Fatalf("did not expect PKCE challenge in auth URL, got %s", url)
	}
	if strings.Contains(url, "nonce=") {
		t.Fatalf("did not expect nonce in auth URL, got %s", url)
	}
}

func TestNewOIDCProviderDiscoveryError(t *testing.T) {
	_, err := NewOIDCProvider(t.Context(), "oidc", "http://127.0.0.1:1", "client", "secret", "http://localhost/callback")
	if err == nil || !strings.Contains(err.Error(), "oidc discovery") {
		t.Fatalf("expected discovery error, got %v", err)
	}
}

func TestNewEntraIDProviderDiscoveryError(t *testing.T) {
	_, err := NewEntraIDProvider(t.Context(), "invalid-tenant", "client", "secret", "http://localhost/callback")
	if err == nil || !strings.Contains(err.Error(), "oidc discovery") {
		t.Fatalf("expected discovery error, got %v", err)
	}
}

func TestOIDCProviderExchange_WrapsExchangeError(t *testing.T) {
	p := &OIDCProvider{
		name: "oidc",
		cfg: &oauth2.Config{
			ClientID:     "client",
			ClientSecret: "secret",
		},
	}
	_, err := p.Exchange(t.Context(), "code-123", ExchangeOptions{CodeVerifier: "verifier"})
	if err == nil || !strings.Contains(err.Error(), "oidc exchange") {
		t.Fatalf("expected wrapped exchange error, got %v", err)
	}
}

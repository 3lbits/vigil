package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

var errUnexpectedURL = errors.New("unexpected URL")

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func githubHTTPResponse(contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestGitHubProviderExchange_UsesVerifiedPrimaryEmail(t *testing.T) {
	p := NewGitHubProvider("id", "secret", "http://localhost/cb")
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.String() {
			case "https://github.com/login/oauth/access_token":
				return githubHTTPResponse("application/x-www-form-urlencoded", "access_token=t&token_type=bearer"), nil
			case "https://api.github.com/user":
				return githubHTTPResponse("application/json", `{"id":123,"login":"attacker","name":"Attacker","email":"admin@example.com"}`), nil
			case "https://api.github.com/user/emails":
				return githubHTTPResponse("application/json", `[{"email":"owner@example.com","primary":true,"verified":true}]`), nil
			default:
				t.Fatalf("unexpected URL: %s", r.URL.String())
				return nil, errUnexpectedURL
			}
		}),
	}
	ctx := context.WithValue(t.Context(), oauth2.HTTPClient, client)

	id, err := p.Exchange(ctx, "code-1", ExchangeOptions{})
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if id.Email != "owner@example.com" {
		t.Fatalf("email: got %q, want %q", id.Email, "owner@example.com")
	}
	if id.ProviderID != "123" {
		t.Fatalf("provider id: got %q, want %q", id.ProviderID, "123")
	}
}

func TestGitHubProviderExchange_RejectsWhenNoVerifiedPrimaryEmail(t *testing.T) {
	p := NewGitHubProvider("id", "secret", "http://localhost/cb")
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.String() {
			case "https://github.com/login/oauth/access_token":
				return githubHTTPResponse("application/x-www-form-urlencoded", "access_token=t&token_type=bearer"), nil
			case "https://api.github.com/user":
				return githubHTTPResponse("application/json", `{"id":123,"login":"attacker","name":"Attacker","email":"admin@example.com"}`), nil
			case "https://api.github.com/user/emails":
				return githubHTTPResponse("application/json", `[{"email":"admin@example.com","primary":true,"verified":false}]`), nil
			default:
				t.Fatalf("unexpected URL: %s", r.URL.String())
				return nil, errUnexpectedURL
			}
		}),
	}
	ctx := context.WithValue(t.Context(), oauth2.HTTPClient, client)

	_, err := p.Exchange(ctx, "code-1", ExchangeOptions{})
	if !errors.Is(err, ErrVerifiedEmailRequired) {
		t.Fatalf("expected ErrVerifiedEmailRequired, got %v", err)
	}
}

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

var ErrVerifiedEmailRequired = errors.New("verified email required")

type GitHubProvider struct {
	cfg *oauth2.Config
}

func NewGitHubProvider(clientID, clientSecret, callbackURL string) *GitHubProvider {
	return &GitHubProvider{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  callbackURL,
			Scopes:       []string{"read:user", "user:email"},
			Endpoint:     github.Endpoint,
		},
	}
}

func (p *GitHubProvider) Name() string { return "github" }

func (p *GitHubProvider) AuthCodeURL(state string, _ AuthRequestOptions) string {
	return p.cfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (p *GitHubProvider) Exchange(ctx context.Context, code string, _ ExchangeOptions) (Identity, error) {
	token, err := p.cfg.Exchange(ctx, code)
	if err != nil {
		return Identity{}, fmt.Errorf("github exchange: %w", err)
	}

	client := p.cfg.Client(ctx, token)

	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if fetchErr := ghGet(client, "https://api.github.com/user", &user); fetchErr != nil {
		return Identity{}, fetchErr
	}

	name := user.Name
	if name == "" {
		name = user.Login
	}

	email, err := ghPrimaryEmail(client)
	if err != nil {
		return Identity{}, err
	}
	if email == "" {
		return Identity{}, ErrVerifiedEmailRequired
	}

	return Identity{
		Provider:   "github",
		ProviderID: fmt.Sprintf("%d", user.ID),
		Email:      email,
		Name:       name,
	}, nil
}

func ghGet(client *http.Client, url string, dest any) error {
	resp, err := client.Get(url) // #nosec G107 — URL is a compile-time constant
	if err != nil {
		return fmt.Errorf("github api get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("github api read: %w", err)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("github api json: %w", err)
	}
	return nil
}

func ghPrimaryEmail(client *http.Client) (string, error) {
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := ghGet(client, "https://api.github.com/user/emails", &emails); err != nil {
		return "", fmt.Errorf("get primary email: %w", err)
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	return "", nil
}

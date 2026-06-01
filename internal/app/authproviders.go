package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/3lbits/vigil/internal/auth"
	"github.com/3lbits/vigil/internal/config"
)

var (
	errUnknownAuthProvider = errors.New("unknown auth provider")
	errNoAuthProviders     = errors.New("no auth providers configured")
)

func buildProviders(ctx context.Context, cfg *config.Config) ([]auth.Provider, error) {
	providers := make([]auth.Provider, 0, len(cfg.AuthProviders))
	for _, name := range cfg.AuthProviders {
		switch name {
		case "github":
			providers = append(providers, auth.NewGitHubProvider(
				cfg.GitHubClientID,
				cfg.GitHubClientSecret,
				cfg.AppBaseURL+"/auth/github/callback",
			))
		case "entra":
			p, err := auth.NewEntraIDProvider(ctx,
				cfg.EntraTenantID,
				cfg.EntraClientID,
				cfg.EntraClientSecret,
				cfg.AppBaseURL+"/auth/entra/callback",
			)
			if err != nil {
				return nil, fmt.Errorf("entra provider init: %w", err)
			}
			providers = append(providers, p)
		case "oidc":
			p, err := auth.NewOIDCProvider(ctx, "oidc",
				cfg.OIDCIssuerURL,
				cfg.OIDCClientID,
				cfg.OIDCClientSecret,
				cfg.AppBaseURL+"/auth/oidc/callback",
			)
			if err != nil {
				return nil, fmt.Errorf("oidc provider init: %w", err)
			}
			providers = append(providers, p)
		default:
			return nil, fmt.Errorf("%w: %s", errUnknownAuthProvider, name)
		}
	}
	if len(providers) == 0 {
		return nil, errNoAuthProviders
	}
	return providers, nil
}

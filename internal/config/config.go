// Package config loads and validates application configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Port         string
	AppEnv       string
	DatabaseURL  string
	DevStubAuth  bool
	DevSeed      bool
	AvvikEnabled bool

	// NaisAuth enables the NAIS/Wonderwall bearer-token auth mode.
	// When true, the app verifies the Authorization: Bearer header injected by
	// the Wonderwall login-proxy sidecar instead of running its own OAuth flow.
	// Set via NAIS_AUTH=true.
	NaisAuth bool
	// NaisIssuer is the Entra ID token issuer. Set via AZURE_OPENID_CONFIG_ISSUER.
	NaisIssuer string
	// NaisClientID is the Entra ID application client ID. Set via AZURE_APP_CLIENT_ID.
	NaisClientID string
	// NaisJWKSURI is the JWKS endpoint for token signature verification.
	// Set via AZURE_OPENID_CONFIG_JWKS_URI.
	NaisJWKSURI string
	// AdminGroups is the list of Entra ID group object IDs whose members are
	// granted the admin role at request time. Set via ADMIN_GROUPS (comma-separated).
	AdminGroups []string

	// AuthProviders is the list of enabled auth providers (e.g. ["github", "entra"]).
	// Set via AUTH_PROVIDERS (comma-separated) or the legacy AUTH_PROVIDER variable.
	AuthProviders []string
	// AllowedEmailDomains is the optional OAuth self-registration allowlist.
	// Set via AUTH_ALLOWED_EMAIL_DOMAINS (comma-separated domains).
	AllowedEmailDomains []string

	GitHubClientID     string
	GitHubClientSecret string

	OIDCIssuerURL    string
	OIDCClientID     string
	OIDCClientSecret string

	EntraTenantID     string
	EntraClientID     string
	EntraClientSecret string

	SessionCookieName   string
	SessionCookieSecure bool
	SessionIdleTimeout  time.Duration
	AppBaseURL          string

	// MetricsPath is the HTTP path for the Prometheus metrics endpoint.
	// Set to "" to disable.
	MetricsPath string

	// SessionHMACKey is the secret used to HMAC-SHA256 session tokens before
	// including them in security event logs. Set via SESSION_HMAC_KEY.
	// Leave empty to omit session token hashes from log lines (not recommended
	// in production).
	SessionHMACKey string

	// TrustedProxyCIDRs is a comma-separated list of CIDR ranges whose
	// X-Forwarded-For header is trusted for source IP extraction.
	// Example: "10.0.0.0/8,172.16.0.0/12". Empty = trust no proxy (use RemoteAddr).
	TrustedProxyCIDRs string

	// GlobalRateLimitPerWindow is the global in-memory per-key request limit for
	// non-exempt routes during GlobalRateLimitWindow. Set to 0 to disable.
	GlobalRateLimitPerWindow int
	GlobalRateLimitWindow    time.Duration
}

// Load reads config from environment variables and validates required fields.
func Load() *Config {
	// NAIS injects the Cloud SQL URL under a dynamically-named variable
	// (NAIS_DATABASE_<APP>_<DB>_URL). Fall back to it when DATABASE_URL is empty.
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("NAIS_DATABASE_VIGIL_VIGIL_URL")
	}

	cfg := &Config{
		Port:         getEnvWithDefault("PORT", "8080"),
		AppEnv:       getEnvWithDefault("APP_ENV", "development"),
		DatabaseURL:  databaseURL,
		DevStubAuth:  os.Getenv("DEV_STUB_AUTH") == "true",
		DevSeed:      os.Getenv("DEV_SEED") == "true",
		AvvikEnabled: os.Getenv("AVVIK_ENABLED") == "true",

		NaisAuth:     os.Getenv("NAIS_AUTH") == "true",
		NaisIssuer:   os.Getenv("AZURE_OPENID_CONFIG_ISSUER"),
		NaisClientID: os.Getenv("AZURE_APP_CLIENT_ID"),
		NaisJWKSURI:  os.Getenv("AZURE_OPENID_CONFIG_JWKS_URI"),
		AdminGroups:  parseCSV(os.Getenv("ADMIN_GROUPS"), false),

		AuthProviders: parseProviders(
			os.Getenv("AUTH_PROVIDERS"),
			getEnvWithDefault("AUTH_PROVIDER", "github"),
		),
		AllowedEmailDomains: parseEmailDomains(os.Getenv("AUTH_ALLOWED_EMAIL_DOMAINS")),

		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),

		OIDCIssuerURL:    os.Getenv("OIDC_ISSUER_URL"),
		OIDCClientID:     os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),

		EntraTenantID:     os.Getenv("ENTRA_TENANT_ID"),
		EntraClientID:     os.Getenv("ENTRA_CLIENT_ID"),
		EntraClientSecret: os.Getenv("ENTRA_CLIENT_SECRET"),

		SessionCookieName:   getEnvWithDefault("SESSION_COOKIE_NAME", "vigil_session"),
		SessionCookieSecure: os.Getenv("SESSION_COOKIE_SECURE") != "false",
		SessionIdleTimeout:  mustDurationEnv("SESSION_IDLE_TIMEOUT", 45*time.Minute),
		AppBaseURL:          getEnvWithDefault("APP_BASE_URL", "http://localhost:8080"),

		MetricsPath:              getEnvWithDefault("METRICS_PATH", "/metrics"),
		SessionHMACKey:           os.Getenv("SESSION_HMAC_KEY"),
		TrustedProxyCIDRs:        os.Getenv("TRUSTED_PROXY_CIDRS"),
		GlobalRateLimitPerWindow: intEnv("GLOBAL_RATE_LIMIT_PER_WINDOW", 240),
		GlobalRateLimitWindow:    mustDurationEnv("GLOBAL_RATE_LIMIT_WINDOW", time.Minute),
	}

	validateConfig(cfg)
	return cfg
}

func validateConfig(cfg *Config) {
	validateDevOnlyToggles(cfg)
	validateAppEnvironment(cfg)
	validateProductionSecurity(cfg)
	validateRequiredValues(cfg)
	validateRateLimitConfig(cfg)
	validateNaisAuth(cfg)
	// Cloud SQL on NAIS connects via a local proxy; the connection string does
	// not include sslmode=require. Skip the SSL check in NaisAuth mode.
	if !cfg.NaisAuth {
		validateDBSSLMode(cfg)
	}
}

func validateDevOnlyToggles(cfg *Config) {
	if cfg.DevStubAuth && cfg.AppEnv != "development" {
		panic("DEV_STUB_AUTH must not be true outside APP_ENV=development")
	}
	if cfg.DevSeed && cfg.AppEnv != "development" {
		panic("DEV_SEED must not be true outside APP_ENV=development")
	}
}

func validateAppEnvironment(cfg *Config) {
	if !isValidAppEnv(cfg.AppEnv) {
		panic("APP_ENV must be one of development, staging, or production")
	}
}

func validateProductionSecurity(cfg *Config) {
	if cfg.AppEnv != "production" {
		return
	}
	if !cfg.SessionCookieSecure {
		panic("SESSION_COOKIE_SECURE must not be false in production")
	}
	if cfg.SessionHMACKey == "" {
		panic("SESSION_HMAC_KEY must be set in production")
	}
}

func validateRequiredValues(cfg *Config) {
	if cfg.DatabaseURL == "" {
		panic("required environment variable DATABASE_URL is not set")
	}
	if hasProvider(cfg.AuthProviders, "github") && len(cfg.AllowedEmailDomains) == 0 {
		panic("AUTH_ALLOWED_EMAIL_DOMAINS must be set when AUTH_PROVIDERS includes github")
	}
}

func validateRateLimitConfig(cfg *Config) {
	if cfg.GlobalRateLimitPerWindow < 0 {
		panic("GLOBAL_RATE_LIMIT_PER_WINDOW must be >= 0")
	}
	if cfg.GlobalRateLimitWindow <= 0 {
		panic("GLOBAL_RATE_LIMIT_WINDOW must be > 0")
	}
}

func validateNaisAuth(cfg *Config) {
	if !cfg.NaisAuth {
		return
	}
	if cfg.NaisIssuer == "" {
		panic("AZURE_OPENID_CONFIG_ISSUER must be set when NAIS_AUTH=true")
	}
	if cfg.NaisClientID == "" {
		panic("AZURE_APP_CLIENT_ID must be set when NAIS_AUTH=true")
	}
	if cfg.NaisJWKSURI == "" {
		panic("AZURE_OPENID_CONFIG_JWKS_URI must be set when NAIS_AUTH=true")
	}
}

func validateDBSSLMode(cfg *Config) {
	allowInsecureDBSSL := os.Getenv("ALLOW_INSECURE_DB_SSL") == "true"
	if allowInsecureDBSSL && cfg.AppEnv != "staging" {
		panic("ALLOW_INSECURE_DB_SSL may only be true when APP_ENV=staging")
	}
	if !hasSecureDBSSLMode(cfg.DatabaseURL) {
		switch cfg.AppEnv {
		case "production":
			panic("DATABASE_URL must explicitly set sslmode=require, verify-ca, or verify-full in production")
		case "staging":
			if !allowInsecureDBSSL {
				panic("DATABASE_URL must explicitly set sslmode=require, verify-ca, or verify-full in staging unless ALLOW_INSECURE_DB_SSL=true")
			}
		}
	}
}

func hasSecureDBSSLMode(databaseURL string) bool {
	dsn := strings.ToLower(databaseURL)
	switch {
	case strings.Contains(dsn, "sslmode=require"):
		return true
	case strings.Contains(dsn, "sslmode=verify-ca"):
		return true
	case strings.Contains(dsn, "sslmode=verify-full"):
		return true
	default:
		return false
	}
}

func isValidAppEnv(appEnv string) bool {
	switch appEnv {
	case "development", "staging", "production":
		return true
	default:
		return false
	}
}

// parseProviders returns a deduplicated, trimmed list of provider slugs.
// multi takes precedence over the legacy single value.
func parseProviders(multi, single string) []string {
	raw := multi
	if raw == "" {
		raw = single
	}
	return parseCSV(raw, false)
}

func parseEmailDomains(raw string) []string {
	return parseCSV(raw, true)
}

func parseCSV(raw string, lower bool) []string {
	parts := strings.Split(raw, ",")
	seen := make(map[string]bool, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if lower {
			p = strings.ToLower(strings.TrimPrefix(p, "@"))
		}
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func hasProvider(providers []string, want string) bool {
	for _, p := range providers {
		if p == want {
			return true
		}
	}
	return false
}

func getEnvWithDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func mustDurationEnv(key string, defaultVal time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		panic("invalid " + key + ": " + err.Error())
	}
	return d
}

func intEnv(key string, defaultVal int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		panic("invalid " + key + ": " + err.Error())
	}
	return v
}

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
	Port        string
	AppEnv      string
	DatabaseURL string
	DevStubAuth bool
	DevSeed     bool
	AvvikEnabled bool

	// AuthProviders is the list of enabled auth providers (e.g. ["github", "entra"]).
	// Set via AUTH_PROVIDERS (comma-separated) or the legacy AUTH_PROVIDER variable.
	AuthProviders []string

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
	cfg := &Config{
		Port:        getEnvWithDefault("PORT", "8080"),
		AppEnv:      getEnvWithDefault("APP_ENV", "development"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		DevStubAuth: os.Getenv("DEV_STUB_AUTH") == "true",
		DevSeed:     os.Getenv("DEV_SEED") == "true",
		AvvikEnabled: os.Getenv("AVVIK_ENABLED") == "true",

		AuthProviders: parseProviders(
			os.Getenv("AUTH_PROVIDERS"),
			getEnvWithDefault("AUTH_PROVIDER", "github"),
		),

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
	if cfg.DevStubAuth && cfg.AppEnv != "development" {
		panic("DEV_STUB_AUTH must not be true outside APP_ENV=development")
	}
	if cfg.DevSeed && cfg.AppEnv != "development" {
		panic("DEV_SEED must not be true outside APP_ENV=development")
	}

	if !isValidAppEnv(cfg.AppEnv) {
		panic("APP_ENV must be one of development, staging, or production")
	}

	if cfg.AppEnv == "production" && !cfg.SessionCookieSecure {
		panic("SESSION_COOKIE_SECURE must not be false in production")
	}

	if cfg.AppEnv == "production" && cfg.SessionHMACKey == "" {
		panic("SESSION_HMAC_KEY must be set in production")
	}

	if cfg.DatabaseURL == "" {
		panic("required environment variable DATABASE_URL is not set")
	}
	if cfg.GlobalRateLimitPerWindow < 0 {
		panic("GLOBAL_RATE_LIMIT_PER_WINDOW must be >= 0")
	}
	if cfg.GlobalRateLimitWindow <= 0 {
		panic("GLOBAL_RATE_LIMIT_WINDOW must be > 0")
	}
	validateDBSSLMode(cfg)
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
	parts := strings.Split(raw, ",")
	seen := make(map[string]bool, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
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

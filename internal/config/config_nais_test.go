package config

import "testing"

func setBaseNaisEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://cloudsql-proxy/vigil")
	t.Setenv("SESSION_HMAC_KEY", "hmac-key")
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("DEV_STUB_AUTH", "false")
	t.Setenv("DEV_SEED", "false")
	t.Setenv("AVVIK_ENABLED", "false")
	t.Setenv("ALLOW_INSECURE_DB_SSL", "false")
	t.Setenv("AUTH_ALLOWED_EMAIL_DOMAINS", "nav.no")
	t.Setenv("NAIS_AUTH", "true")
	t.Setenv("AZURE_OPENID_CONFIG_ISSUER", "https://login.microsoftonline.com/test/v2.0")
	t.Setenv("AZURE_APP_CLIENT_ID", "test-client-id")
	t.Setenv("AZURE_OPENID_CONFIG_JWKS_URI", "https://login.microsoftonline.com/test/discovery/keys")
}

func TestLoad_NaisAuth_ValidConfig(t *testing.T) {
	setBaseNaisEnv(t)
	cfg := Load()
	if !cfg.NaisAuth {
		t.Fatal("NaisAuth = false, want true")
	}
	if cfg.NaisIssuer != "https://login.microsoftonline.com/test/v2.0" {
		t.Errorf("NaisIssuer = %q", cfg.NaisIssuer)
	}
	if cfg.NaisClientID != "test-client-id" {
		t.Errorf("NaisClientID = %q", cfg.NaisClientID)
	}
	if cfg.NaisJWKSURI != "https://login.microsoftonline.com/test/discovery/keys" {
		t.Errorf("NaisJWKSURI = %q", cfg.NaisJWKSURI)
	}
}

func TestLoad_NaisAuth_SkipsDBSSLCheck(t *testing.T) {
	setBaseNaisEnv(t)
	// Cloud SQL proxy URL without sslmode — would panic without the NaisAuth gate.
	t.Setenv("DATABASE_URL", "postgres://localhost/vigil")
	cfg := Load()
	if !cfg.NaisAuth {
		t.Fatal("NaisAuth = false, want true")
	}
}

func TestLoad_NaisAuth_MissingIssuerPanics(t *testing.T) {
	setBaseNaisEnv(t)
	t.Setenv("AZURE_OPENID_CONFIG_ISSUER", "")
	expectPanicContains(t, "AZURE_OPENID_CONFIG_ISSUER must be set when NAIS_AUTH=true", func() { Load() })
}

func TestLoad_NaisAuth_MissingClientIDPanics(t *testing.T) {
	setBaseNaisEnv(t)
	t.Setenv("AZURE_APP_CLIENT_ID", "")
	expectPanicContains(t, "AZURE_APP_CLIENT_ID must be set when NAIS_AUTH=true", func() { Load() })
}

func TestLoad_NaisAuth_MissingJWKSURIPanics(t *testing.T) {
	setBaseNaisEnv(t)
	t.Setenv("AZURE_OPENID_CONFIG_JWKS_URI", "")
	expectPanicContains(t, "AZURE_OPENID_CONFIG_JWKS_URI must be set when NAIS_AUTH=true", func() { Load() })
}

func TestLoad_NaisAuth_AdminGroupsParsed(t *testing.T) {
	setBaseNaisEnv(t)
	t.Setenv("ADMIN_GROUPS", "aaa-bbb-111, ccc-ddd-222")
	cfg := Load()
	want := []string{"aaa-bbb-111", "ccc-ddd-222"}
	if len(cfg.AdminGroups) != len(want) {
		t.Fatalf("AdminGroups = %v, want %v", cfg.AdminGroups, want)
	}
	for i, g := range want {
		if cfg.AdminGroups[i] != g {
			t.Errorf("AdminGroups[%d] = %q, want %q", i, cfg.AdminGroups[i], g)
		}
	}
}

func TestLoad_NaisAuth_DatabaseURLFallback(t *testing.T) {
	setBaseNaisEnv(t)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("NAIS_DATABASE_VIGIL_VIGIL_URL", "postgres://nais-injected/vigil")
	cfg := Load()
	if cfg.DatabaseURL != "postgres://nais-injected/vigil" {
		t.Errorf("DatabaseURL = %q, want postgres://nais-injected/vigil", cfg.DatabaseURL)
	}
}

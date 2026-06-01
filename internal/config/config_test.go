package config

import (
	"strings"
	"testing"
	"time"
)

func TestHasSecureDBSSLMode(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want bool
	}{
		{
			name: "uri require",
			dsn:  "postgres://localhost:5432/db?sslmode=require",
			want: true,
		},
		{
			name: "uri verify-ca",
			dsn:  "postgres://localhost:5432/db?sslmode=verify-ca",
			want: true,
		},
		{
			name: "uri verify-full",
			dsn:  "postgres://localhost:5432/db?sslmode=verify-full",
			want: true,
		},
		{
			name: "uri disable",
			dsn:  "postgres://localhost:5432/db?sslmode=disable",
			want: false,
		},
		{
			name: "uri missing",
			dsn:  "postgres://localhost:5432/db",
			want: false,
		},
		{
			name: "kv require",
			dsn:  "host=localhost port=5432 user=vigil dbname=vigil sslmode=require",
			want: true,
		},
		{
			name: "kv prefer",
			dsn:  "host=localhost port=5432 user=vigil dbname=vigil sslmode=prefer",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hasSecureDBSSLMode(tc.dsn)
			if got != tc.want {
				t.Fatalf("hasSecureDBSSLMode(%q) = %v, want %v", tc.dsn, got, tc.want)
			}
		})
	}
}

func TestLoad_ProductionGuardrailsPanic(t *testing.T) {
	t.Run("missing session hmac key", func(t *testing.T) {
		setBaseProdEnv(t)
		t.Setenv("SESSION_HMAC_KEY", "")
		expectPanicContains(t, "SESSION_HMAC_KEY must be set in production", func() { Load() })
	})

	t.Run("session cookie secure false", func(t *testing.T) {
		setBaseProdEnv(t)
		t.Setenv("SESSION_COOKIE_SECURE", "false")
		expectPanicContains(t, "SESSION_COOKIE_SECURE must not be false in production", func() { Load() })
	})

	t.Run("database url insecure sslmode", func(t *testing.T) {
		setBaseProdEnv(t)
		t.Setenv("DATABASE_URL", "postgres://localhost:5432/db?sslmode=disable")
		expectPanicContains(t, "DATABASE_URL must explicitly set sslmode=require, verify-ca, or verify-full in production", func() { Load() })
	})

	t.Run("dev stub auth enabled in production", func(t *testing.T) {
		setBaseProdEnv(t)
		t.Setenv("DEV_STUB_AUTH", "true")
		expectPanicContains(t, "DEV_STUB_AUTH must not be true outside APP_ENV=development", func() { Load() })
	})

	t.Run("insecure db ssl override true in production", func(t *testing.T) {
		setBaseProdEnv(t)
		t.Setenv("ALLOW_INSECURE_DB_SSL", "true")
		expectPanicContains(t, "ALLOW_INSECURE_DB_SSL may only be true when APP_ENV=staging", func() { Load() })
	})

	t.Run("dev seed enabled in production", func(t *testing.T) {
		setBaseProdEnv(t)
		t.Setenv("DEV_SEED", "true")
		expectPanicContains(t, "DEV_SEED must not be true outside APP_ENV=development", func() { Load() })
	})
}

func TestLoad_DBSSLPoliciesByEnvironment(t *testing.T) {
	t.Run("development allows insecure db ssl", func(t *testing.T) {
		setBaseDevEnv(t)
		t.Setenv("DATABASE_URL", "postgres://localhost:5432/db?sslmode=disable")
		_ = Load()
	})

	t.Run("staging insecure db ssl without override panics", func(t *testing.T) {
		setBaseStagingEnv(t)
		t.Setenv("DATABASE_URL", "postgres://localhost:5432/db?sslmode=disable")
		expectPanicContains(t, "DATABASE_URL must explicitly set sslmode=require, verify-ca, or verify-full in staging unless ALLOW_INSECURE_DB_SSL=true", func() { Load() })
	})

	t.Run("staging insecure db ssl with override is allowed", func(t *testing.T) {
		setBaseStagingEnv(t)
		t.Setenv("DATABASE_URL", "postgres://localhost:5432/db?sslmode=disable")
		t.Setenv("ALLOW_INSECURE_DB_SSL", "true")
		_ = Load()
	})
}

func TestLoad_AppEnvValidation(t *testing.T) {
	t.Run("invalid app env panics", func(t *testing.T) {
		setBaseDevEnv(t)
		t.Setenv("APP_ENV", "qa")
		expectPanicContains(t, "APP_ENV must be one of development, staging, or production", func() { Load() })
	})
}

func TestLoad_AvvikEnabled(t *testing.T) {
	t.Run("default false", func(t *testing.T) {
		setBaseDevEnv(t)
		cfg := Load()
		if cfg.AvvikEnabled {
			t.Fatal("AvvikEnabled = true, want false by default")
		}
	})

	t.Run("true when env enabled", func(t *testing.T) {
		setBaseDevEnv(t)
		t.Setenv("AVVIK_ENABLED", "true")
		cfg := Load()
		if !cfg.AvvikEnabled {
			t.Fatal("AvvikEnabled = false, want true when AVVIK_ENABLED=true")
		}
	})
}

func TestLoad_SessionIdleTimeout(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		setBaseDevEnv(t)
		cfg := Load()
		if cfg.SessionIdleTimeout != 45*time.Minute {
			t.Fatalf("SessionIdleTimeout = %v, want %v", cfg.SessionIdleTimeout, 45*time.Minute)
		}
	})

	t.Run("custom duration", func(t *testing.T) {
		setBaseDevEnv(t)
		t.Setenv("SESSION_IDLE_TIMEOUT", "30m")
		cfg := Load()
		if cfg.SessionIdleTimeout != 30*time.Minute {
			t.Fatalf("SessionIdleTimeout = %v, want %v", cfg.SessionIdleTimeout, 30*time.Minute)
		}
	})

	t.Run("invalid duration panics", func(t *testing.T) {
		setBaseDevEnv(t)
		t.Setenv("SESSION_IDLE_TIMEOUT", "not-a-duration")
		expectPanicContains(t, "invalid SESSION_IDLE_TIMEOUT", func() { Load() })
	})
}

func TestLoad_GlobalRateLimitConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		setBaseDevEnv(t)
		cfg := Load()
		if cfg.GlobalRateLimitPerWindow != 240 {
			t.Fatalf("GlobalRateLimitPerWindow = %d, want 240", cfg.GlobalRateLimitPerWindow)
		}
		if cfg.GlobalRateLimitWindow != time.Minute {
			t.Fatalf("GlobalRateLimitWindow = %v, want %v", cfg.GlobalRateLimitWindow, time.Minute)
		}
	})

	t.Run("custom values", func(t *testing.T) {
		setBaseDevEnv(t)
		t.Setenv("GLOBAL_RATE_LIMIT_PER_WINDOW", "120")
		t.Setenv("GLOBAL_RATE_LIMIT_WINDOW", "30s")
		cfg := Load()
		if cfg.GlobalRateLimitPerWindow != 120 {
			t.Fatalf("GlobalRateLimitPerWindow = %d, want 120", cfg.GlobalRateLimitPerWindow)
		}
		if cfg.GlobalRateLimitWindow != 30*time.Second {
			t.Fatalf("GlobalRateLimitWindow = %v, want %v", cfg.GlobalRateLimitWindow, 30*time.Second)
		}
	})

	t.Run("invalid per window panics", func(t *testing.T) {
		setBaseDevEnv(t)
		t.Setenv("GLOBAL_RATE_LIMIT_PER_WINDOW", "nope")
		expectPanicContains(t, "invalid GLOBAL_RATE_LIMIT_PER_WINDOW", func() { Load() })
	})

	t.Run("negative per window panics", func(t *testing.T) {
		setBaseDevEnv(t)
		t.Setenv("GLOBAL_RATE_LIMIT_PER_WINDOW", "-1")
		expectPanicContains(t, "GLOBAL_RATE_LIMIT_PER_WINDOW must be >= 0", func() { Load() })
	})

	t.Run("invalid window panics", func(t *testing.T) {
		setBaseDevEnv(t)
		t.Setenv("GLOBAL_RATE_LIMIT_WINDOW", "bad")
		expectPanicContains(t, "invalid GLOBAL_RATE_LIMIT_WINDOW", func() { Load() })
	})

	t.Run("zero window panics", func(t *testing.T) {
		setBaseDevEnv(t)
		t.Setenv("GLOBAL_RATE_LIMIT_WINDOW", "0s")
		expectPanicContains(t, "GLOBAL_RATE_LIMIT_WINDOW must be > 0", func() { Load() })
	})
}

func setBaseProdEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/db?sslmode=require")
	t.Setenv("SESSION_HMAC_KEY", "hmac-key")
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("DEV_STUB_AUTH", "false")
	t.Setenv("DEV_SEED", "false")
	t.Setenv("AVVIK_ENABLED", "false")
	t.Setenv("ALLOW_INSECURE_DB_SSL", "false")
}

func setBaseStagingEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "staging")
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/db?sslmode=require")
	t.Setenv("SESSION_HMAC_KEY", "")
	t.Setenv("SESSION_COOKIE_SECURE", "false")
	t.Setenv("DEV_STUB_AUTH", "false")
	t.Setenv("DEV_SEED", "false")
	t.Setenv("AVVIK_ENABLED", "false")
	t.Setenv("ALLOW_INSECURE_DB_SSL", "false")
}

func setBaseDevEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/db")
	t.Setenv("SESSION_HMAC_KEY", "")
	t.Setenv("SESSION_COOKIE_SECURE", "false")
	t.Setenv("DEV_STUB_AUTH", "false")
	t.Setenv("DEV_SEED", "false")
	t.Setenv("AVVIK_ENABLED", "false")
	t.Setenv("ALLOW_INSECURE_DB_SSL", "false")
}

func expectPanicContains(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		got, ok := recovered.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T", recovered)
		}
		if !strings.Contains(got, want) {
			t.Fatalf("panic %q does not contain %q", got, want)
		}
	}()
	fn()
}

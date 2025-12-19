package config

import (
	"net/netip"
	"testing"
)

func TestLoadFromEnv_MissingJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != "JWT_SECRET is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFromEnv_DefaultsAndTrustedProxies(t *testing.T) {
	t.Setenv("APP_PORT", "")
	t.Setenv("DB_DSN", "")
	t.Setenv("JWT_ISSUER", "")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("TRUSTED_PROXIES", "127.0.0.1/32, 10.0.0.0/8")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Fatalf("expected default port 8080, got %q", cfg.Port)
	}
	if cfg.JWTIssuer != "polling-system" {
		t.Fatalf("expected default issuer polling-system, got %q", cfg.JWTIssuer)
	}
	if len(cfg.TrustedProxies) != 2 {
		t.Fatalf("expected 2 trusted proxies, got %d", len(cfg.TrustedProxies))
	}
	if cfg.TrustedProxies[0] != netip.MustParsePrefix("127.0.0.1/32") {
		t.Fatalf("unexpected first proxy: %v", cfg.TrustedProxies[0])
	}
	if cfg.TrustedProxies[1] != netip.MustParsePrefix("10.0.0.0/8") {
		t.Fatalf("unexpected second proxy: %v", cfg.TrustedProxies[1])
	}
}

func TestLoadFromEnv_InvalidTrustedProxies(t *testing.T) {
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("TRUSTED_PROXIES", "not-a-cidr")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatalf("expected error")
	}
}


package config

import (
	"log"
	"net/netip"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	DB_DSN         string
	JWTSecret      string
	JWTIssuer      string
	TrustedProxies []netip.Prefix
}

func Load() Config {
	_ = godotenv.Load()

	cfg := Config{
		Port:           getEnv("APP_PORT", "8080"),
		DB_DSN:         getEnv("DB_DSN", "postgres://polling_user:polling_pass@localhost:5432/polling_db?sslmode=disable"),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		JWTIssuer:      getEnv("JWT_ISSUER", "polling-system"),
		TrustedProxies: parseCIDRList(getEnv("TRUSTED_PROXIES", "")),
	}

	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}

	return cfg
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseCIDRList(s string) []netip.Prefix {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]netip.Prefix, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(p)
		if err != nil {
			log.Fatalf("invalid TRUSTED_PROXIES entry %q: %v", p, err)
		}
		out = append(out, prefix)
	}
	return out
}

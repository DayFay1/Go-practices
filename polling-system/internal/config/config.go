package config

import (
	"fmt"
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

	cfg, err := LoadFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	return cfg
}

func LoadFromEnv() (Config, error) {
	cfg := Config{
		Port:      getEnv("APP_PORT", "8080"),
		DB_DSN:    getEnv("DB_DSN", "postgres://polling_user:polling_pass@localhost:5432/polling_db?sslmode=disable"),
		JWTSecret: getEnv("JWT_SECRET", ""),
		JWTIssuer: getEnv("JWT_ISSUER", "polling-system"),
	}

	trusted, err := parseCIDRList(getEnv("TRUSTED_PROXIES", ""))
	if err != nil {
		return Config{}, err
	}
	cfg.TrustedProxies = trusted

	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseCIDRList(s string) ([]netip.Prefix, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
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
			return nil, fmt.Errorf("invalid TRUSTED_PROXIES entry %q: %v", p, err)
		}
		out = append(out, prefix)
	}
	return out, nil
}

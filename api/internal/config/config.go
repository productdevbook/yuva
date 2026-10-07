package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	Version     string
	ListenAddr  string
	MetricsAddr string
	DatabaseURL string
}

func Load(version string) (Config, error) {
	return Config{
		Version:     version,
		ListenAddr:  env("YUVA_LISTEN_ADDR", ":8080"),
		MetricsAddr: env("YUVA_METRICS_ADDR", ":9090"),
		DatabaseURL: env("YUVA_DATABASE_URL", ""),
	}, nil
}

func (c Config) RequireDatabase() error {
	if c.DatabaseURL == "" {
		return errors.New("YUVA_DATABASE_URL is required")
	}
	return nil
}

func env(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

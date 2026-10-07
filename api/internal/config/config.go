package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Version     string
	ListenAddr  string
	MetricsAddr string
	DatabaseURL string

	PublicURL      string
	CookieSecure   bool
	ClientIPHeader string

	WebAuthnRPID    string
	WebAuthnRPName  string
	WebAuthnOrigins []string

	SMTP SMTP
}

type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLS      string
}

func (s SMTP) Enabled() bool { return s.Host != "" }

func Load(version string) (Config, error) {
	c := Config{
		Version:        version,
		ListenAddr:     env("YUVA_LISTEN_ADDR", ":8080"),
		MetricsAddr:    env("YUVA_METRICS_ADDR", ":9090"),
		DatabaseURL:    env("YUVA_DATABASE_URL", ""),
		PublicURL:      strings.TrimRight(env("YUVA_PUBLIC_URL", "http://localhost:8080"), "/"),
		ClientIPHeader: env("YUVA_CLIENT_IP_HEADER", ""),
		WebAuthnRPName: env("YUVA_WEBAUTHN_RP_NAME", "Yuva"),
		SMTP: SMTP{
			Host:     env("YUVA_SMTP_HOST", ""),
			Username: env("YUVA_SMTP_USERNAME", ""),
			Password: os.Getenv("YUVA_SMTP_PASSWORD"),
			From:     env("YUVA_SMTP_FROM", ""),
			TLS:      env("YUVA_SMTP_TLS", "starttls"),
		},
	}
	public, err := url.Parse(c.PublicURL)
	if err != nil || (public.Scheme != "http" && public.Scheme != "https") || public.Host == "" {
		return c, fmt.Errorf("YUVA_PUBLIC_URL must be an absolute http(s) URL, got %q", c.PublicURL)
	}
	c.WebAuthnRPID = env("YUVA_WEBAUTHN_RP_ID", public.Hostname())
	c.WebAuthnOrigins = list(env("YUVA_WEBAUTHN_ORIGINS", public.Scheme+"://"+public.Host))
	if c.CookieSecure, err = boolEnv("YUVA_COOKIE_SECURE", public.Scheme == "https"); err != nil {
		return c, err
	}
	switch c.SMTP.TLS {
	case "starttls", "tls", "none":
	default:
		return c, fmt.Errorf("YUVA_SMTP_TLS must be starttls, tls or none, got %q", c.SMTP.TLS)
	}
	defaultPort := "587"
	if c.SMTP.TLS == "tls" {
		defaultPort = "465"
	}
	if c.SMTP.Port, err = strconv.Atoi(env("YUVA_SMTP_PORT", defaultPort)); err != nil {
		return c, fmt.Errorf("YUVA_SMTP_PORT: %w", err)
	}
	if c.SMTP.Enabled() && c.SMTP.From == "" {
		return c, errors.New("YUVA_SMTP_FROM is required when YUVA_SMTP_HOST is set")
	}
	return c, nil
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

func boolEnv(name string, fallback bool) (bool, error) {
	v := env(name, "")
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", name, err)
	}
	return b, nil
}

func list(v string) []string {
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, strings.TrimRight(item, "/"))
		}
	}
	return out
}

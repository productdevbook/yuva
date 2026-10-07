package config

import (
	"errors"
	"fmt"
	"net/mail"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Version     string
	ListenAddr  string
	MetricsAddr string
	DatabaseURL string

	PublicURL      string
	CookieSecure   bool
	ClientIPHeader string
	TrustedProxies []netip.Prefix

	WebAuthnRPID    string
	WebAuthnRPName  string
	WebAuthnOrigins []string

	SMTP SMTP

	MasterKey string

	Storage     Storage
	Attachments Attachments

	IngressSecret        string
	IngressAcceptV1      bool
	IngressAuthservID    string
	IngressMaxConcurrent int
	SESTopicARNs         []string
	EmailSenderHourlyCap int

	ChatEmailDelay           time.Duration
	AnonymousContactsPerHour int

	WebhookAllowPrivate bool
	SMTPAllowPrivate    bool

	VAPID VAPID
}

type VAPID struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

func (v VAPID) Enabled() bool { return v.PublicKey != "" }

type Storage struct {
	Driver            string
	Dir               string
	S3Endpoint        string
	S3Region          string
	S3Bucket          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3PathStyle       bool
}

type Attachments struct {
	MaxBytes int64
	Types    []string
}

var defaultAttachmentTypes = "image/png,image/jpeg,image/gif,image/webp,image/heic,application/pdf,text/plain,text/csv," +
	"application/zip,application/json,video/mp4,video/quicktime,audio/mpeg,audio/mp4"

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
		MetricsAddr:    metricsAddr(),
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
		MasterKey: os.Getenv("YUVA_MASTER_KEY"),
		Storage: Storage{
			Driver:            env("YUVA_STORAGE", "local"),
			Dir:               env("YUVA_STORAGE_DIR", "data/attachments"),
			S3Endpoint:        env("YUVA_S3_ENDPOINT", ""),
			S3Region:          env("YUVA_S3_REGION", "auto"),
			S3Bucket:          env("YUVA_S3_BUCKET", ""),
			S3AccessKeyID:     env("YUVA_S3_ACCESS_KEY_ID", ""),
			S3SecretAccessKey: os.Getenv("YUVA_S3_SECRET_ACCESS_KEY"),
		},
		Attachments:       Attachments{Types: lowerList(env("YUVA_ATTACHMENT_TYPES", defaultAttachmentTypes))},
		IngressSecret:     strings.TrimSpace(os.Getenv("YUVA_INGRESS_SECRET")),
		IngressAuthservID: env("YUVA_INGRESS_AUTHSERV_ID", ""),
		SESTopicARNs:      list(env("YUVA_SES_TOPIC_ARNS", "")),
		VAPID: VAPID{
			PublicKey:  env("YUVA_VAPID_PUBLIC_KEY", ""),
			PrivateKey: strings.TrimSpace(os.Getenv("YUVA_VAPID_PRIVATE_KEY")),
			Subject:    env("YUVA_VAPID_SUBJECT", ""),
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
	switch c.Storage.Driver {
	case "local", "s3":
	default:
		return c, fmt.Errorf("YUVA_STORAGE must be local or s3, got %q", c.Storage.Driver)
	}
	if c.Storage.S3PathStyle, err = boolEnv("YUVA_S3_PATH_STYLE", false); err != nil {
		return c, err
	}
	for _, v := range list(env("YUVA_TRUSTED_PROXIES", "")) {
		p, err := netip.ParsePrefix(v)
		if err != nil {
			a, aerr := netip.ParseAddr(v)
			if aerr != nil {
				return c, fmt.Errorf("YUVA_TRUSTED_PROXIES: %q is not an IP address or CIDR range", v)
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		c.TrustedProxies = append(c.TrustedProxies, p.Masked())
	}
	if c.WebhookAllowPrivate, err = boolEnv("YUVA_WEBHOOK_ALLOW_PRIVATE", false); err != nil {
		return c, err
	}
	if c.SMTPAllowPrivate, err = boolEnv("YUVA_SMTP_ALLOW_PRIVATE", false); err != nil {
		return c, err
	}
	if c.Attachments.MaxBytes, err = strconv.ParseInt(env("YUVA_ATTACHMENT_MAX_BYTES", "26214400"), 10, 64); err != nil || c.Attachments.MaxBytes < 1 {
		return c, errors.New("YUVA_ATTACHMENT_MAX_BYTES must be a positive number of bytes")
	}
	if c.EmailSenderHourlyCap, err = strconv.Atoi(env("YUVA_EMAIL_SENDER_HOURLY_CAP", "500")); err != nil || c.EmailSenderHourlyCap < 1 {
		return c, errors.New("YUVA_EMAIL_SENDER_HOURLY_CAP must be a positive number of messages")
	}
	if c.ChatEmailDelay, err = time.ParseDuration(env("YUVA_CHAT_EMAIL_DELAY", "5m")); err != nil || c.ChatEmailDelay < time.Second {
		return c, errors.New("YUVA_CHAT_EMAIL_DELAY must be a duration of at least 1s, such as 5m")
	}
	if c.IngressAcceptV1, err = boolEnv("YUVA_INGRESS_ACCEPT_V1", false); err != nil {
		return c, err
	}
	if c.IngressMaxConcurrent, err = strconv.Atoi(env("YUVA_INGRESS_MAX_CONCURRENT", "8")); err != nil || c.IngressMaxConcurrent < 1 {
		return c, errors.New("YUVA_INGRESS_MAX_CONCURRENT must be a positive number of messages")
	}
	if c.AnonymousContactsPerHour, err = strconv.Atoi(env("YUVA_ANONYMOUS_CONTACTS_PER_HOUR", "20")); err != nil || c.AnonymousContactsPerHour < 1 {
		return c, errors.New("YUVA_ANONYMOUS_CONTACTS_PER_HOUR must be a positive number of visitors")
	}
	if err := c.loadVAPID(); err != nil {
		return c, err
	}
	if c.SMTP.Enabled() && c.SMTP.From == "" {
		return c, errors.New("YUVA_SMTP_FROM is required when YUVA_SMTP_HOST is set")
	}
	return c, nil
}

func (c *Config) loadVAPID() error {
	v := &c.VAPID
	if (v.PublicKey == "") != (v.PrivateKey == "") {
		return errors.New("set both YUVA_VAPID_PUBLIC_KEY and YUVA_VAPID_PRIVATE_KEY, or neither (yuva vapid-keys makes a pair)")
	}
	if !v.Enabled() {
		return nil
	}
	if v.Subject == "" {
		switch {
		case c.SMTP.From != "":
			addr, err := mail.ParseAddress(c.SMTP.From)
			if err != nil {
				return fmt.Errorf("YUVA_SMTP_FROM: %w", err)
			}
			v.Subject = "mailto:" + addr.Address
		case strings.HasPrefix(c.PublicURL, "https://"):
			v.Subject = c.PublicURL
		default:
			return errors.New("YUVA_VAPID_SUBJECT is required (mailto:you@example.com or an https URL)")
		}
	}
	if !strings.HasPrefix(v.Subject, "mailto:") && !strings.HasPrefix(v.Subject, "https://") {
		return fmt.Errorf("YUVA_VAPID_SUBJECT must start with mailto: or https://, got %q", v.Subject)
	}
	return nil
}

func (c Config) RequireDatabase() error {
	if c.DatabaseURL == "" {
		return errors.New("YUVA_DATABASE_URL is required")
	}
	return nil
}

func (c Config) RequireMasterKey() error {
	if strings.TrimSpace(c.MasterKey) == "" {
		return errors.New("YUVA_MASTER_KEY is required: 32 random bytes, base64-encoded (openssl rand -base64 32)")
	}
	return nil
}

func lowerList(v string) []string {
	var out []string
	for _, item := range list(v) {
		out = append(out, strings.ToLower(item))
	}
	return out
}

func metricsAddr() string {
	v, ok := os.LookupEnv("YUVA_METRICS_ADDR")
	if !ok {
		return ":9090"
	}
	v = strings.TrimSpace(v)
	if strings.EqualFold(v, "off") {
		return ""
	}
	return v
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

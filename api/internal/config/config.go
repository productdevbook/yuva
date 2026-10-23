package config

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
)

type Config struct {
	Version     string
	ListenAddr  string
	MetricsAddr string
	DatabaseURL string

	PublicURL string
	// LocaleCookieDomain is lowercase without a leading dot; empty when unset.
	LocaleCookieDomain string
	CookieSecure       bool
	ClientIPHeader     string
	TrustedProxies     []netip.Prefix

	WebAuthnRPID    string
	WebAuthnRPName  string
	WebAuthnOrigins []string

	SignupOpen                bool
	SignupWorkspacesPerPerson int
	SignupWorkspacesPerIP     int

	OperatorEmails []string

	SMTP SMTP

	MasterKey string

	Storage     Storage
	Attachments Attachments
	// EmailImagesMaxBytes caps each workspace's e-mail images; 0 is no limit.
	EmailImagesMaxBytes int64
	// ImportMaxRows caps the data rows of a subscriber CSV import, at most 100,000.
	ImportMaxRows int

	IngressSecret        string
	IngressAcceptV1      bool
	IngressAcceptV2      bool
	IngressAuthservID    string
	IngressMaxConcurrent int
	SESTopicARNs         []string
	EmailSenderHourlyCap int

	SES SES

	ChatEmailDelay           time.Duration
	DiscussionEmailDelay     time.Duration
	AnonymousContactsPerHour int

	WebhookAllowPrivate bool
	SMTPAllowPrivate    bool

	VAPID VAPID

	MCP    bool
	Panel  bool
	Widget bool

	PanelAnalyticsKey string

	// Broadcasts is on, approval or off (YUVA_BROADCASTS).
	Broadcasts               string
	BroadcastWorkers         int
	BroadcastDailyRecipients int64
	// MailLinkURL is the base of /m/, /s/, /c/ and /i/ links in mail; PublicURL when unset.
	MailLinkURL string
	// StatusURL is the origin public status pages are named from (YUVA_STATUS_URL); empty means
	// <PublicURL>/status.
	StatusURL string
	// StatusMonitors caps a workspace's status monitors (YUVA_STATUS_MONITORS, default 20); 0 is no limit.
	StatusMonitors int
	// StatusNotices is YUVA_STATUS_NOTICES: off hides the subscribe form and sends no notices.
	StatusNotices bool
	// StatusHome is YUVA_STATUS_HOME: off makes the root of the status host answer a plain 404.
	// StatusHomePage is YUVA_STATUS_HOME_PAGE, the slug of the published page the home page shows as
	// its live card; empty leaves the card out.
	StatusHome     bool
	StatusHomePage string
	// StatusNoticesDaily caps notice mails per workspace and UTC day (YUVA_STATUS_NOTICES_DAILY); 0 is
	// no limit. StatusMaxSubscribers caps subscribed addresses per page (YUVA_STATUS_MAX_SUBSCRIBERS).
	StatusNoticesDaily   int64
	StatusMaxSubscribers int
	// StatusMinInterval is the shortest interval of http and tcp monitors in seconds
	// (YUVA_STATUS_MIN_INTERVAL; 60, or 300 with open signup). StatusCanaryURLs tell the server's own
	// trouble from its customers' (YUVA_STATUS_CANARY_URLS; nil for the built-in two, off sets
	// StatusCanariesOff).
	StatusMinInterval int
	StatusCanaryURLs  []string
	StatusCanariesOff bool

	// HelpURL is the origin public help centers are named from (YUVA_HELP_URL); empty means
	// <PublicURL>/help. Help is YUVA_HELP: off removes help centers. HelpMaxArticles caps a
	// workspace's help articles (YUVA_HELP_MAX_ARTICLES; 500 with open signup); 0 is no limit.
	HelpURL         string
	Help            bool
	HelpMaxArticles int

	// Roadmap is YUVA_ROADMAP: off removes roadmap boards. RoadmapMaxIdeas caps a workspace's ideas
	// (YUVA_ROADMAP_MAX_IDEAS; 2000 with open signup); 0 is no limit.
	Roadmap         bool
	RoadmapMaxIdeas int

	// WhatsNew is YUVA_WHATS_NEW: off removes What's new changelogs. ChangelogMaxEntries caps a
	// workspace's entries (YUVA_CHANGELOG_MAX_ENTRIES; 1000 with open signup); 0 is no limit.
	WhatsNew            bool
	ChangelogMaxEntries int

	StatusDomains      string
	StatusCNAME        string
	StatusDomainLimit  int
	CloudflareZoneID   string
	CloudflareAPIToken string

	// MigrationSources are the sources teams may move from (YUVA_MIGRATION_SOURCES); nil for every
	// source the server has.
	MigrationSources      []string
	MigrationAllowPrivate bool
	// MigrationWorkers is the migrations queue's workers in this process; 0 runs none here.
	MigrationWorkers int
	// MigrationMaxActive caps a workspace's scanning, copying and syncing migrations; 0 is no limit.
	MigrationMaxActive int
	// MigrationMaxBytes caps the bytes a workspace imports a month; 0 is no limit.
	MigrationMaxBytes int64
	// MigrationMaxMessageBytes is the largest message imported; larger ones are skipped.
	MigrationMaxMessageBytes int64
	// MigrationUploadMaxBytes is the largest export file; 0 turns file imports off.
	MigrationUploadMaxBytes int64
	// MigrationSyncMaxDays ends a migration's catch-up sync, and deletes its credentials, after so many days.
	MigrationSyncMaxDays int
	// MigrationAuditDays keeps a migration's audit entries so many days after it ended.
	MigrationAuditDays int
	// AuditDays keeps a workspace's audit log entries so many days.
	AuditDays int
	// ForwardingDomain gives every e-mail channel the address <token>@ForwardingDomain; empty for none.
	ForwardingDomain string
	// MicrosoftClientID and MicrosoftClientSecret are the server's own Microsoft Entra app
	// registration for Sign in with Microsoft (YUVA_MICROSOFT_CLIENT_ID, _CLIENT_SECRET); without
	// them a team signs in with an app registration of its own. MicrosoftTenant is its authority
	// (YUVA_MICROSOFT_TENANT): common for a multi-tenant app, else its tenant.
	MicrosoftClientID     string
	MicrosoftClientSecret string
	MicrosoftTenant       string

	Analytics Analytics
}

type Analytics struct {
	// On is YUVA_ANALYTICS; off removes analytics from the server.
	On     bool
	Buffer int
	// VisitCache is how many visitors' current visits are kept in memory.
	VisitCache int
	SaltGrace  time.Duration
	// MonthlyEvents caps each workspace's events a month; 0 is no limit. The default is 1,000,000.
	MonthlyEvents           int64
	PropertyEventsPerMinute int
	PropertiesPerWorkspace  int
	// RawDays is how long raw visits and events are kept; Workers how many analytics jobs run at
	// once in this process; RollupTop how many values a dimension keeps per day.
	RawDays   int
	Workers   int
	RollupTop int
	// GeoIP is dbip (DB-IP Lite downloaded monthly into GeoIPDir), file (GeoIPFile) or off.
	GeoIP     string
	GeoIPDir  string
	GeoIPFile string
}

type VAPID struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

func (v VAPID) Enabled() bool { return v.PublicKey != "" }

// SES is the server's own Amazon SES account, whose tenants Yuva manages for the inboxes that send
// through it (YUVA_SES_*); all of region, key, secret and configuration set, or none.
type SES struct {
	Region           string
	AccessKeyID      string
	SecretAccessKey  string
	ConfigurationSet string
	TenantPrefix     string
}

func (s SES) Enabled() bool { return s.AccessKeyID != "" }

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
	if c.StatusURL = strings.TrimRight(env("YUVA_STATUS_URL", ""), "/"); c.StatusURL != "" {
		u, err := url.Parse(c.StatusURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" {
			return c, fmt.Errorf("YUVA_STATUS_URL must be an origin such as https://status.example.com, got %q", c.StatusURL)
		}
		if strings.EqualFold(u.Host, public.Host) {
			return c, errors.New("YUVA_STATUS_URL must name another host than YUVA_PUBLIC_URL; leave it unset for <YUVA_PUBLIC_URL>/status")
		}
	}
	switch strings.ToLower(env("YUVA_STATUS_HOME", "on")) {
	case "on":
		c.StatusHome = true
	case "off":
	default:
		return c, errors.New("YUVA_STATUS_HOME must be on or off")
	}
	c.StatusHomePage = strings.ToLower(strings.TrimSpace(env("YUVA_STATUS_HOME_PAGE", "")))
	if c.StatusMonitors, err = strconv.Atoi(env("YUVA_STATUS_MONITORS", "20")); err != nil || c.StatusMonitors < 0 {
		return c, errors.New("YUVA_STATUS_MONITORS must be a number of monitors, 0 for no limit")
	}
	if c.LocaleCookieDomain, err = localeCookieDomain(env("YUVA_LOCALE_COOKIE_DOMAIN", ""), public.Hostname()); err != nil {
		return c, err
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
	if c.EmailImagesMaxBytes, err = strconv.ParseInt(env("YUVA_EMAIL_IMAGES_MAX_BYTES", "524288000"), 10, 64); err != nil || c.EmailImagesMaxBytes < 0 {
		return c, errors.New("YUVA_EMAIL_IMAGES_MAX_BYTES must be a number of bytes, 0 for no limit")
	}
	if c.ImportMaxRows, err = strconv.Atoi(env("YUVA_IMPORT_MAX_ROWS", "100000")); err != nil || c.ImportMaxRows < 1 || c.ImportMaxRows > 100000 {
		return c, errors.New("YUVA_IMPORT_MAX_ROWS must be a number of rows from 1 to 100000")
	}
	if c.EmailSenderHourlyCap, err = strconv.Atoi(env("YUVA_EMAIL_SENDER_HOURLY_CAP", "500")); err != nil || c.EmailSenderHourlyCap < 1 {
		return c, errors.New("YUVA_EMAIL_SENDER_HOURLY_CAP must be a positive number of messages")
	}
	if c.ChatEmailDelay, err = time.ParseDuration(env("YUVA_CHAT_EMAIL_DELAY", "5m")); err != nil || c.ChatEmailDelay < time.Second {
		return c, errors.New("YUVA_CHAT_EMAIL_DELAY must be a duration of at least 1s, such as 5m")
	}
	if c.DiscussionEmailDelay, err = time.ParseDuration(env("YUVA_DISCUSSION_EMAIL_DELAY", "10m")); err != nil || c.DiscussionEmailDelay < time.Second {
		return c, errors.New("YUVA_DISCUSSION_EMAIL_DELAY must be a duration of at least 1s, such as 10m")
	}
	if c.MCP, err = switchEnv("YUVA_MCP", true); err != nil {
		return c, err
	}
	if c.Panel, err = switchEnv("YUVA_PANEL", true); err != nil {
		return c, err
	}
	if c.PanelAnalyticsKey = env("YUVA_PANEL_ANALYTICS_KEY", ""); c.PanelAnalyticsKey != "" && !analyticsKeyRe.MatchString(c.PanelAnalyticsKey) {
		return c, errors.New("YUVA_PANEL_ANALYTICS_KEY must be the public key of an analytics property (yuva_ak_...)")
	}
	if c.Widget, err = switchEnv("YUVA_WIDGET", true); err != nil {
		return c, err
	}
	if c.IngressAcceptV1, err = boolEnv("YUVA_INGRESS_ACCEPT_V1", false); err != nil {
		return c, err
	}
	if c.IngressAcceptV2, err = boolEnv("YUVA_INGRESS_ACCEPT_V2", true); err != nil {
		return c, err
	}
	if c.IngressMaxConcurrent, err = strconv.Atoi(env("YUVA_INGRESS_MAX_CONCURRENT", "8")); err != nil || c.IngressMaxConcurrent < 1 {
		return c, errors.New("YUVA_INGRESS_MAX_CONCURRENT must be a positive number of messages")
	}
	switch strings.ToLower(env("YUVA_SIGNUP", "invite")) {
	case "invite":
	case "open":
		c.SignupOpen = true
	default:
		return c, errors.New("YUVA_SIGNUP must be invite or open")
	}
	if c.SignupWorkspacesPerPerson, err = strconv.Atoi(env("YUVA_SIGNUP_WORKSPACES_PER_PERSON", "3")); err != nil || c.SignupWorkspacesPerPerson < 1 {
		return c, errors.New("YUVA_SIGNUP_WORKSPACES_PER_PERSON must be a positive number of workspaces")
	}
	if c.SignupWorkspacesPerIP, err = strconv.Atoi(env("YUVA_SIGNUP_WORKSPACES_PER_IP", "5")); err != nil || c.SignupWorkspacesPerIP < 1 {
		return c, errors.New("YUVA_SIGNUP_WORKSPACES_PER_IP must be a positive number of workspaces")
	}
	for _, v := range list(env("YUVA_OPERATOR_EMAIL", "")) {
		a, err := mail.ParseAddress(v)
		if err != nil {
			return c, fmt.Errorf("YUVA_OPERATOR_EMAIL: %q is not an e-mail address", v)
		}
		c.OperatorEmails = append(c.OperatorEmails, a.Address)
	}
	if c.AnonymousContactsPerHour, err = strconv.Atoi(env("YUVA_ANONYMOUS_CONTACTS_PER_HOUR", "20")); err != nil || c.AnonymousContactsPerHour < 1 {
		return c, errors.New("YUVA_ANONYMOUS_CONTACTS_PER_HOUR must be a positive number of visitors")
	}
	if err := c.loadVAPID(); err != nil {
		return c, err
	}
	if err := c.rejectDevSecrets(public.Hostname()); err != nil {
		return c, err
	}
	if err := c.loadMigrations(); err != nil {
		return c, err
	}
	if err := c.loadBroadcasts(); err != nil {
		return c, err
	}
	if err := c.loadStatusNotices(); err != nil {
		return c, err
	}
	if err := c.loadStatusChecks(); err != nil {
		return c, err
	}
	if err := c.loadStatusDomains(public.Hostname()); err != nil {
		return c, err
	}
	if err := c.loadHelp(public); err != nil {
		return c, err
	}
	if err := c.loadRoadmap(); err != nil {
		return c, err
	}
	if err := c.loadWhatsNew(); err != nil {
		return c, err
	}
	if err := c.loadAnalytics(); err != nil {
		return c, err
	}
	if err := c.loadSES(); err != nil {
		return c, err
	}
	if c.SMTP.Enabled() && c.SMTP.From == "" {
		return c, errors.New("YUVA_SMTP_FROM is required when YUVA_SMTP_HOST is set")
	}
	return c, nil
}

func (c *Config) loadStatusChecks() error {
	def := "60"
	if c.SignupOpen {
		def = "300"
	}
	var err error
	if c.StatusMinInterval, err = strconv.Atoi(env("YUVA_STATUS_MIN_INTERVAL", def)); err != nil || c.StatusMinInterval < 60 || c.StatusMinInterval > 86400 {
		return errors.New("YUVA_STATUS_MIN_INTERVAL must be 60 to 86400 seconds")
	}
	canaries := env("YUVA_STATUS_CANARY_URLS", "")
	if strings.EqualFold(strings.TrimSpace(canaries), "off") {
		c.StatusCanariesOff = true
		return nil
	}
	for _, item := range strings.Split(canaries, ",") {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		if u, err := url.Parse(item); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("YUVA_STATUS_CANARY_URLS must be comma-separated http or https URLs, got %q", item)
		}
		c.StatusCanaryURLs = append(c.StatusCanaryURLs, item)
	}
	return nil
}

func (c *Config) loadStatusDomains(publicHost string) error {
	c.CloudflareZoneID = env("YUVA_CLOUDFLARE_ZONE_ID", "")
	c.CloudflareAPIToken = strings.TrimSpace(os.Getenv("YUVA_CLOUDFLARE_API_TOKEN"))
	switch c.StatusDomains = strings.ToLower(env("YUVA_STATUS_DOMAINS", "ask")); c.StatusDomains {
	case "ask", "off":
	case "cloudflare":
		if c.CloudflareZoneID == "" || c.CloudflareAPIToken == "" {
			return errors.New("YUVA_STATUS_DOMAINS=cloudflare needs YUVA_CLOUDFLARE_ZONE_ID and YUVA_CLOUDFLARE_API_TOKEN")
		}
	default:
		return errors.New("YUVA_STATUS_DOMAINS must be ask, cloudflare or off")
	}
	c.StatusCNAME = strings.TrimSuffix(strings.ToLower(env("YUVA_STATUS_CNAME", publicHost)), ".")
	if strings.ContainsAny(c.StatusCNAME, "/:@ ") || !strings.Contains(c.StatusCNAME, ".") && c.StatusCNAME != "localhost" {
		return fmt.Errorf("YUVA_STATUS_CNAME must be a hostname such as status-proxy.example.com, got %q", c.StatusCNAME)
	}
	def := "0"
	if c.SignupOpen {
		def = "1"
	}
	var err error
	if c.StatusDomainLimit, err = strconv.Atoi(env("YUVA_STATUS_DOMAIN_LIMIT", def)); err != nil || c.StatusDomainLimit < 0 {
		return errors.New("YUVA_STATUS_DOMAIN_LIMIT must be a number of domains, 0 for no limit")
	}
	return nil
}

func (c *Config) loadHelp(public *url.URL) error {
	switch strings.ToLower(env("YUVA_HELP", "on")) {
	case "on":
		c.Help = true
	case "off":
	default:
		return errors.New("YUVA_HELP must be on or off")
	}
	if c.HelpURL = strings.TrimRight(env("YUVA_HELP_URL", ""), "/"); c.HelpURL != "" {
		u, err := url.Parse(c.HelpURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" {
			return fmt.Errorf("YUVA_HELP_URL must be an origin such as https://help.example.com, got %q", c.HelpURL)
		}
		if strings.EqualFold(u.Host, public.Host) {
			return errors.New("YUVA_HELP_URL must name another host than YUVA_PUBLIC_URL; leave it unset for <YUVA_PUBLIC_URL>/help")
		}
	}
	def := "0"
	if c.SignupOpen {
		def = "500"
	}
	var err error
	if c.HelpMaxArticles, err = strconv.Atoi(env("YUVA_HELP_MAX_ARTICLES", def)); err != nil || c.HelpMaxArticles < 0 {
		return errors.New("YUVA_HELP_MAX_ARTICLES must be a number of articles, 0 for no limit")
	}
	return nil
}

func (c *Config) loadRoadmap() error {
	switch strings.ToLower(env("YUVA_ROADMAP", "on")) {
	case "on":
		c.Roadmap = true
	case "off":
	default:
		return errors.New("YUVA_ROADMAP must be on or off")
	}
	def := "0"
	if c.SignupOpen {
		def = "2000"
	}
	var err error
	if c.RoadmapMaxIdeas, err = strconv.Atoi(env("YUVA_ROADMAP_MAX_IDEAS", def)); err != nil || c.RoadmapMaxIdeas < 0 {
		return errors.New("YUVA_ROADMAP_MAX_IDEAS must be a number of ideas, 0 for no limit")
	}
	return nil
}

func (c *Config) loadWhatsNew() error {
	switch strings.ToLower(env("YUVA_WHATS_NEW", "on")) {
	case "on":
		c.WhatsNew = true
	case "off":
	default:
		return errors.New("YUVA_WHATS_NEW must be on or off")
	}
	def := "0"
	if c.SignupOpen {
		def = "1000"
	}
	var err error
	if c.ChangelogMaxEntries, err = strconv.Atoi(env("YUVA_CHANGELOG_MAX_ENTRIES", def)); err != nil || c.ChangelogMaxEntries < 0 {
		return errors.New("YUVA_CHANGELOG_MAX_ENTRIES must be a number of entries, 0 for no limit")
	}
	return nil
}

func (c *Config) loadStatusNotices() error {
	switch strings.ToLower(env("YUVA_STATUS_NOTICES", "on")) {
	case "on":
		c.StatusNotices = true
	case "off":
	default:
		return errors.New("YUVA_STATUS_NOTICES must be on or off")
	}
	daily, max := "0", "0"
	if c.SignupOpen {
		daily, max = "5000", "10000"
	}
	var err error
	if c.StatusNoticesDaily, err = strconv.ParseInt(env("YUVA_STATUS_NOTICES_DAILY", daily), 10, 64); err != nil || c.StatusNoticesDaily < 0 {
		return errors.New("YUVA_STATUS_NOTICES_DAILY must be a number of mails, 0 for no limit")
	}
	if c.StatusMaxSubscribers, err = strconv.Atoi(env("YUVA_STATUS_MAX_SUBSCRIBERS", max)); err != nil || c.StatusMaxSubscribers < 0 {
		return errors.New("YUVA_STATUS_MAX_SUBSCRIBERS must be a number of subscribers, 0 for no limit")
	}
	return nil
}

func (c *Config) loadBroadcasts() error {
	def := "on"
	if c.SignupOpen {
		def = "approval"
	}
	c.Broadcasts = strings.ToLower(env("YUVA_BROADCASTS", def))
	switch c.Broadcasts {
	case "on", "approval", "off":
	default:
		return errors.New("YUVA_BROADCASTS must be on, approval or off")
	}
	var err error
	if c.BroadcastWorkers, err = strconv.Atoi(env("YUVA_BROADCAST_WORKERS", "2")); err != nil || c.BroadcastWorkers < 1 {
		return errors.New("YUVA_BROADCAST_WORKERS must be a positive number of workers")
	}
	if c.BroadcastDailyRecipients, err = strconv.ParseInt(env("YUVA_BROADCAST_DAILY_RECIPIENTS", "0"), 10, 64); err != nil || c.BroadcastDailyRecipients < 0 {
		return errors.New("YUVA_BROADCAST_DAILY_RECIPIENTS must be a number of recipients, 0 for no limit")
	}
	c.MailLinkURL = strings.TrimRight(env("YUVA_MAIL_LINK_URL", c.PublicURL), "/")
	u, err := url.Parse(c.MailLinkURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("YUVA_MAIL_LINK_URL must be an absolute http(s) URL without a path, got %q", c.MailLinkURL)
	}
	return nil
}

func (c *Config) loadAnalytics() error {
	a := &c.Analytics
	var err error
	if a.On, err = switchEnv("YUVA_ANALYTICS", true); err != nil {
		return err
	}
	if a.Buffer, err = strconv.Atoi(env("YUVA_ANALYTICS_BUFFER", "50000")); err != nil || a.Buffer < 100 {
		return errors.New("YUVA_ANALYTICS_BUFFER must be a number of events, at least 100")
	}
	if a.VisitCache, err = strconv.Atoi(env("YUVA_ANALYTICS_VISIT_CACHE", "200000")); err != nil || a.VisitCache < 1000 {
		return errors.New("YUVA_ANALYTICS_VISIT_CACHE must be a number of visits, at least 1000")
	}
	if a.SaltGrace, err = time.ParseDuration(env("YUVA_ANALYTICS_SALT_GRACE", "2h")); err != nil || a.SaltGrace <= 0 || a.SaltGrace > 6*time.Hour {
		return errors.New("YUVA_ANALYTICS_SALT_GRACE must be a positive duration of at most 6h, such as 2h")
	}
	if a.MonthlyEvents, err = strconv.ParseInt(env("YUVA_ANALYTICS_MONTHLY_EVENTS", "1000000"), 10, 64); err != nil || a.MonthlyEvents < 0 {
		return errors.New("YUVA_ANALYTICS_MONTHLY_EVENTS must be a number of events, 0 for no limit")
	}
	if a.PropertyEventsPerMinute, err = strconv.Atoi(env("YUVA_ANALYTICS_PROPERTY_EVENTS_PER_MINUTE", "60000")); err != nil || a.PropertyEventsPerMinute < 1 {
		return errors.New("YUVA_ANALYTICS_PROPERTY_EVENTS_PER_MINUTE must be a positive number of events")
	}
	if a.PropertiesPerWorkspace, err = strconv.Atoi(env("YUVA_ANALYTICS_PROPERTIES_PER_WORKSPACE", "50")); err != nil || a.PropertiesPerWorkspace < 1 {
		return errors.New("YUVA_ANALYTICS_PROPERTIES_PER_WORKSPACE must be a positive number of properties")
	}
	if a.RawDays, err = strconv.Atoi(env("YUVA_ANALYTICS_RAW_DAYS", "90")); err != nil || a.RawDays < 7 || a.RawDays > 400 {
		return errors.New("YUVA_ANALYTICS_RAW_DAYS must be 7 to 400 days")
	}
	if a.Workers, err = strconv.Atoi(env("YUVA_ANALYTICS_WORKERS", "2")); err != nil || a.Workers < 0 {
		return errors.New("YUVA_ANALYTICS_WORKERS must be a number of workers, 0 for none in this process")
	}
	if a.RollupTop, err = strconv.Atoi(env("YUVA_ANALYTICS_ROLLUP_TOP", "1000")); err != nil || a.RollupTop < 10 || a.RollupTop > 100000 {
		return errors.New("YUVA_ANALYTICS_ROLLUP_TOP must be 10 to 100000 values")
	}
	a.GeoIP = strings.ToLower(env("YUVA_GEOIP", "dbip"))
	a.GeoIPDir = env("YUVA_GEOIP_DIR", "data/geoip")
	a.GeoIPFile = env("YUVA_GEOIP_FILE", "")
	switch a.GeoIP {
	case "dbip", "off":
	case "file":
		if a.GeoIPFile == "" {
			return errors.New("YUVA_GEOIP=file needs YUVA_GEOIP_FILE, the path of an MMDB city database")
		}
	default:
		return errors.New("YUVA_GEOIP must be dbip, file or off")
	}
	return nil
}

var (
	sesRegionRe = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-\d$`)
	sesNameRe   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	sesPrefixRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
)

func (c *Config) loadSES() error {
	s := SES{
		Region:           env("YUVA_SES_REGION", ""),
		AccessKeyID:      env("YUVA_SES_ACCESS_KEY_ID", ""),
		SecretAccessKey:  strings.TrimSpace(os.Getenv("YUVA_SES_SECRET_ACCESS_KEY")),
		ConfigurationSet: env("YUVA_SES_CONFIGURATION_SET", ""),
		TenantPrefix:     env("YUVA_SES_TENANT_PREFIX", "yuva-"),
	}
	set := map[string]bool{
		"YUVA_SES_REGION": s.Region != "", "YUVA_SES_ACCESS_KEY_ID": s.AccessKeyID != "",
		"YUVA_SES_SECRET_ACCESS_KEY": s.SecretAccessKey != "", "YUVA_SES_CONFIGURATION_SET": s.ConfigurationSet != "",
	}
	n := 0
	for _, v := range set {
		if v {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	if n != len(set) {
		var missing []string
		for _, name := range []string{"YUVA_SES_REGION", "YUVA_SES_ACCESS_KEY_ID", "YUVA_SES_SECRET_ACCESS_KEY", "YUVA_SES_CONFIGURATION_SET"} {
			if !set[name] {
				missing = append(missing, name)
			}
		}
		return fmt.Errorf("set YUVA_SES_REGION, YUVA_SES_ACCESS_KEY_ID, YUVA_SES_SECRET_ACCESS_KEY and YUVA_SES_CONFIGURATION_SET together, or none; missing %s", strings.Join(missing, ", "))
	}
	if !sesRegionRe.MatchString(s.Region) {
		return fmt.Errorf("YUVA_SES_REGION must be an AWS region such as eu-west-1, got %q", s.Region)
	}
	if !sesNameRe.MatchString(s.ConfigurationSet) {
		return fmt.Errorf("YUVA_SES_CONFIGURATION_SET must be 1 to 64 letters, digits, - or _, got %q", s.ConfigurationSet)
	}
	if !sesPrefixRe.MatchString(s.TenantPrefix) {
		return fmt.Errorf("YUVA_SES_TENANT_PREFIX must be 1 to 32 letters, digits, - or _, got %q", s.TenantPrefix)
	}
	c.SES = s
	return nil
}

var migrationSources = func() []string {
	out := []string{"imap", "file"}
	for _, d := range helpdesk.Desks() {
		out = append(out, d.Source)
	}
	return out
}()

var forwardingDomainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
var microsoftTenantRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)
var analyticsKeyRe = regexp.MustCompile(`^yuva_ak_[A-Za-z0-9_-]{1,64}$`)

func (c *Config) loadMigrations() error {
	var err error
	if c.MigrationAllowPrivate, err = boolEnv("YUVA_MIGRATION_ALLOW_PRIVATE", false); err != nil {
		return err
	}
	if c.MigrationWorkers, err = strconv.Atoi(env("YUVA_MIGRATION_WORKERS", "2")); err != nil || c.MigrationWorkers < 0 {
		return errors.New("YUVA_MIGRATION_WORKERS must be a number of workers, 0 for none in this process")
	}
	if c.MigrationMaxActive, err = strconv.Atoi(env("YUVA_MIGRATION_MAX_ACTIVE", "0")); err != nil || c.MigrationMaxActive < 0 {
		return errors.New("YUVA_MIGRATION_MAX_ACTIVE must be a number of migrations, 0 for no limit")
	}
	if c.MigrationMaxBytes, err = strconv.ParseInt(env("YUVA_MIGRATION_MAX_BYTES", "0"), 10, 64); err != nil || c.MigrationMaxBytes < 0 {
		return errors.New("YUVA_MIGRATION_MAX_BYTES must be a number of bytes, 0 for no limit")
	}
	if c.MigrationMaxMessageBytes, err = strconv.ParseInt(env("YUVA_MIGRATION_MAX_MESSAGE_BYTES", "52428800"), 10, 64); err != nil || c.MigrationMaxMessageBytes < 1 {
		return errors.New("YUVA_MIGRATION_MAX_MESSAGE_BYTES must be a positive number of bytes")
	}
	if c.MigrationUploadMaxBytes, err = strconv.ParseInt(env("YUVA_MIGRATION_UPLOAD_MAX_BYTES", "21474836480"), 10, 64); err != nil || c.MigrationUploadMaxBytes < 0 {
		return errors.New("YUVA_MIGRATION_UPLOAD_MAX_BYTES must be a number of bytes, 0 for no file imports")
	}
	if c.MigrationSyncMaxDays, err = strconv.Atoi(env("YUVA_MIGRATION_SYNC_MAX_DAYS", "30")); err != nil || c.MigrationSyncMaxDays < 1 || c.MigrationSyncMaxDays > 365 {
		return errors.New("YUVA_MIGRATION_SYNC_MAX_DAYS must be 1 to 365 days")
	}
	if c.MigrationAuditDays, err = strconv.Atoi(env("YUVA_MIGRATION_AUDIT_DAYS", "365")); err != nil || c.MigrationAuditDays < 1 || c.MigrationAuditDays > 36500 {
		return errors.New("YUVA_MIGRATION_AUDIT_DAYS must be 1 to 36500 days")
	}
	if c.AuditDays, err = strconv.Atoi(env("YUVA_AUDIT_DAYS", "365")); err != nil || c.AuditDays < 1 || c.AuditDays > 36500 {
		return errors.New("YUVA_AUDIT_DAYS must be 1 to 36500 days")
	}
	c.ForwardingDomain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(os.Getenv("YUVA_FORWARDING_DOMAIN"))), ".")
	if c.ForwardingDomain != "" && !forwardingDomainRe.MatchString(c.ForwardingDomain) {
		return fmt.Errorf("YUVA_FORWARDING_DOMAIN must be a domain name such as forward.example.com, got %q", c.ForwardingDomain)
	}
	c.MicrosoftClientID = strings.TrimSpace(os.Getenv("YUVA_MICROSOFT_CLIENT_ID"))
	c.MicrosoftClientSecret = os.Getenv("YUVA_MICROSOFT_CLIENT_SECRET")
	if (c.MicrosoftClientID == "") != (c.MicrosoftClientSecret == "") {
		return errors.New("set both YUVA_MICROSOFT_CLIENT_ID and YUVA_MICROSOFT_CLIENT_SECRET, or neither")
	}
	c.MicrosoftTenant = strings.TrimSpace(env("YUVA_MICROSOFT_TENANT", "common"))
	if !microsoftTenantRe.MatchString(c.MicrosoftTenant) {
		return fmt.Errorf("YUVA_MICROSOFT_TENANT must be common, organizations, consumers, a tenant id or a verified domain, got %q", c.MicrosoftTenant)
	}
	if _, set := os.LookupEnv("YUVA_MIGRATION_SOURCES"); set {
		c.MigrationSources = []string{}
		for _, s := range lowerList(os.Getenv("YUVA_MIGRATION_SOURCES")) {
			if !slices.Contains(migrationSources, s) {
				return fmt.Errorf("YUVA_MIGRATION_SOURCES: %q is not one of %s", s, strings.Join(migrationSources, ", "))
			}
			c.MigrationSources = append(c.MigrationSources, s)
		}
	}
	if c.MigrationUploadMaxBytes == 0 {
		if c.MigrationSources == nil {
			c.MigrationSources = slices.Clone(migrationSources)
		}
		c.MigrationSources = slices.DeleteFunc(c.MigrationSources, func(s string) bool { return s == "file" })
	}
	return nil
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

// The values deploy/compose.yaml falls back to; they are public, so only a loopback instance may use them.
var devSecrets = map[string]string{
	"YUVA_MASTER_KEY":        "eXV2YS1kZXYtb25seS1tYXN0ZXIta2V5LTMyYnl0ZXM=",
	"YUVA_INGRESS_SECRET":    "yuva-dev-only-ingress-secret",
	"YUVA_VAPID_PRIVATE_KEY": "151tODJ-wlzzEOSc3wpPlKPbCSxI8fEO8P6IKs-EPeg",
}

func (c Config) rejectDevSecrets(host string) error {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil
	}
	if ip, err := netip.ParseAddr(host); err == nil && ip.IsLoopback() {
		return nil
	}
	set := map[string]string{
		"YUVA_MASTER_KEY":        c.MasterKey,
		"YUVA_INGRESS_SECRET":    c.IngressSecret,
		"YUVA_VAPID_PRIVATE_KEY": c.VAPID.PrivateKey,
	}
	for name, known := range devSecrets {
		if strings.TrimSpace(set[name]) == known {
			return fmt.Errorf("%s is the public development value from deploy/compose.yaml; generate your own (see docs/install.md) or serve from localhost", name)
		}
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

func switchEnv(name string, fallback bool) (bool, error) {
	switch strings.ToLower(env(name, "")) {
	case "":
		return fallback, nil
	case "on":
		return true, nil
	case "off":
		return false, nil
	}
	return false, fmt.Errorf("%s must be on or off", name)
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

func localeCookieDomain(v, host string) (string, error) {
	d := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(v), "."))
	if d == "" {
		return "", nil
	}
	host = strings.ToLower(host)
	if !strings.Contains(d, ".") || net.ParseIP(d) != nil || (host != d && !strings.HasSuffix(host, "."+d)) {
		return "", fmt.Errorf("YUVA_LOCALE_COOKIE_DOMAIN must be a domain with a dot that the host of YUVA_PUBLIC_URL (%s) is inside of, e.g. example.com, got %q", host, v)
	}
	return d, nil
}

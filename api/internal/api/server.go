package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/microcosm-cc/bluemonday"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/productdevbook/yuva/api/internal/apppush"
	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/metrics"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/push"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/secret"
	"github.com/productdevbook/yuva/api/internal/slack"
	"github.com/productdevbook/yuva/api/internal/storage"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/telemetry"
	"github.com/productdevbook/yuva/api/internal/ui"
	"github.com/productdevbook/yuva/api/internal/webhook"
	"github.com/productdevbook/yuva/api/internal/widget"
)

const maxBodyBytes = 8 << 20

var apiPrefixes = []string{"/v1/", "/client/v1/", "/ingress/", "/.well-known/", emailImagePathPrefix, emailTemplatesPathPrefix, clickPathPrefix, statusPathPrefix, samlPathPrefix}

var apiPaths = []string{"/v1", "/client/v1", "/healthz", "/readyz", "/oauth/register", "/oauth/authorize", "/oauth/token", "/oauth/revoke", "/mcp", signInCallbackPath, slackCallbackPath, ssoCallbackPath}

type Server struct {
	log      *slog.Logger
	st       *store.Store
	version  string
	mailer   mail.Mailer
	webauthn *webauthn.WebAuthn
	auth     AuthSettings
	now      func() time.Time
	sleep    func(ctx context.Context, d time.Duration) error
	secrets  *secret.Key
	optKeys  sync.Map
	objects  storage.Storage
	attach   AttachmentSettings
	sanitize *bluemonday.Policy
	original *bluemonday.Policy
	hub      *realtime.Hub
	sockets  *clientSockets
	jobs     *river.Client[pgx.Tx]
	ingress  IngressSettings
	ingestQ  chan struct{}
	sender   email.Sender
	smtpPriv bool
	snsCerts *certCache
	fetch    *http.Client
	chat     ChatSettings
	limits   *rateLimiter
	webhooks WebhookSettings
	hooks    *webhook.Client
	sso      *ssoState
	push     *push.Sender
	appPush  *apppush.Client
	slack    SlackSettings
	slackAPI *slack.Client

	imagesMax         int64
	issueTrackers     IssueTrackerSettings
	analytics         *analyticsState
	broadcasts        BroadcastSettings
	imports           ImportSettings
	migrations        MigrationSettings
	auditDays         int
	ses               SESSettings
	status            StatusSettings
	statusCache       *statusCache
	statusDomainCache *statusDomainCache
	help              HelpSettings
	roadmapCache      *publicCache
	roadmap           RoadmapSettings
	changelog         ChangelogSettings
	changelogCache    *publicCache
	helpCounts        *helpCounters
	helpCache         *publicCache
	canaries          canaryGuard
	started           time.Time
	sesCaller         atomic.Pointer[string]

	mcp        http.Handler
	mcpSchemas *mcp.SchemaCache
	noPanel    bool
	panelKey   string
	noWidget   bool
	operator   []string
}

type Deps struct {
	Log      *slog.Logger
	Store    *store.Store
	Version  string
	Mailer   mail.Mailer
	WebAuthn *webauthn.WebAuthn
	Auth     AuthSettings
	Now      func() time.Time
	// Sleep replaces the timer that spaces out the mails of a send, for tests; nil waits for real.
	Sleep func(ctx context.Context, d time.Duration) error
	// RateLimitClock replaces the database's clock that rate limit windows, query slot leases and
	// the trash's 30 days are measured with, for tests; nil uses the database's.
	RateLimitClock func() time.Time

	Secrets     *secret.Key
	Storage     storage.Storage
	Attachments AttachmentSettings
	Hub         *realtime.Hub

	Ingress     IngressSettings
	EmailSender email.Sender
	// SMTPAllowPrivate lets e-mail channels use SMTP servers on private addresses (Mailpit in
	// development): when a channel is saved, and at send time when EmailSender is nil.
	SMTPAllowPrivate bool
	HTTPClient       *http.Client

	Chat ChatSettings

	Webhooks WebhookSettings

	Push PushSettings

	AppPush AppPushSettings
	// IssueTrackers points the GitHub and Linear clients at fakes, for tests.
	IssueTrackers IssueTrackerSettings

	Slack SlackSettings

	// DisableMCP turns the /mcp endpoint off (YUVA_MCP=off).
	DisableMCP bool
	// DisablePanel stops serving the panel (YUVA_PANEL=off); /oauth/authorize then has no consent page.
	DisablePanel bool
	// PanelAnalyticsKey is the analytics property the panel counts its own screens in
	// (YUVA_PANEL_ANALYTICS_KEY); empty is off.
	PanelAnalyticsKey string
	// DisableWidget stops serving the widget scripts (YUVA_WIDGET=off).
	DisableWidget bool
	// OperatorEmails get a digest of new sign-ups and workspaces (YUVA_OPERATOR_EMAIL); empty is off.
	OperatorEmails []string
	// EmailImagesMaxBytes caps a workspace's e-mail images (YUVA_EMAIL_IMAGES_MAX_BYTES); 0 is no limit.
	EmailImagesMaxBytes int64
	Broadcasts          BroadcastSettings
	Imports             ImportSettings
	Migrations          MigrationSettings
	AuditDays           int
	Analytics           AnalyticsSettings
	SES                 SESSettings
	Status              StatusSettings
	Help                HelpSettings
	Roadmap             RoadmapSettings
	Changelog           ChangelogSettings
}

// PushSettings holds the VAPID keys; push is off without them.
type PushSettings struct {
	Keys push.Keys
	// HTTPClient replaces the client that refuses private addresses; for tests.
	HTTPClient webpush.HTTPClient
}

type ChatSettings struct {
	// EmailDelay is how long a chat contact must be gone, and a reply unread, before it is e-mailed.
	EmailDelay time.Duration
	// AnonymousContactsPerHour caps new anonymous visitors per channel, IP address and hour; 0 means 20.
	AnonymousContactsPerHour int
	// DiscussionEmailDelay is the least time between two e-mails to a follower about one thread; 0 means 10 minutes.
	DiscussionEmailDelay time.Duration
}

type IngressSettings struct {
	Secret       string
	SESTopicARNs []string
	// SenderHourlyCap refuses mail from a sender beyond this many inbound mails per hour; 0 means 500.
	SenderHourlyCap int
	// OwnAddresses are addresses the server sends from besides its channels, such as YUVA_SMTP_FROM.
	OwnAddresses []string
	// AcceptV1 accepts the deprecated v1 signature, which leaves the envelope sender unsigned.
	AcceptV1 bool
	// AcceptV2 accepts the deprecated v2 signature, whose body is read before the request is
	// known to be genuine.
	AcceptV2 bool
	// AuthservID is the authserv-id of the receiving server whose Authentication-Results are
	// trusted; without it DMARC verdicts are stored but not used.
	AuthservID string
	// MaxConcurrent caps messages processed at once; more are answered 503. 0 means 8.
	MaxConcurrent int
	// ReadTimeout bounds the time a request may take to send its body; 0 means 30 seconds.
	ReadTimeout time.Duration
	// ForwardingDomain gives every e-mail channel the address <forward token>@ForwardingDomain
	// (YUVA_FORWARDING_DOMAIN); empty for none.
	ForwardingDomain string
}

type AttachmentSettings struct {
	MaxBytes int64
	Types    []string
}

type AuthSettings struct {
	PublicURL          string
	CookieSecure       bool
	LocaleCookieDomain string
	ClientIPHeader     string
	// TrustedProxies are the proxies whose entries in ClientIPHeader are skipped from the right.
	TrustedProxies []netip.Prefix
	// CodeReplyDelay is how long every sign-in code request takes; 0 means 800 ms.
	CodeReplyDelay time.Duration
	// FirstPartyClients reports the member apps: OAuth clients that may ask for the whole-API
	// resource and deliver as the member whatever bots_may_send says. Nil means none.
	FirstPartyClients func(clientID string) bool
	Signup            SignupSettings
}

// ClientList is a FirstPartyClients that allows exactly ids.
func ClientList(ids []string) func(string) bool {
	return func(id string) bool { return slices.Contains(ids, id) }
}

// SignupSettings: Open lets anyone sign in with a code and create workspaces, within the limits;
// zero limits mean the defaults.
type SignupSettings struct {
	Open                bool
	WorkspacesPerPerson int
	WorkspacesPerIP     int
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func New(d Deps) *Server {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	sleep := d.Sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	jobs, err := river.NewClient(riverpgxv5.New(d.Store.Pool), &river.Config{Logger: d.Log})
	if err != nil {
		panic(err)
	}
	sender := d.EmailSender
	if sender == nil {
		sender = email.SMTPSender{AllowPrivate: d.SMTPAllowPrivate, Resolver: d.Webhooks.Resolver}
	}
	fetch := d.HTTPClient
	if fetch == nil {
		fetch = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	ingestSlots := d.Ingress.MaxConcurrent
	if ingestSlots <= 0 {
		ingestSlots = defaultIngestConcurrency
	}
	chat := d.Chat
	if chat.EmailDelay <= 0 {
		chat.EmailDelay = defaultChatEmailDelay
	}
	if chat.DiscussionEmailDelay <= 0 {
		chat.DiscussionEmailDelay = defaultDiscussionEmailDelay
	}
	if chat.AnonymousContactsPerHour <= 0 {
		chat.AnonymousContactsPerHour = defaultAnonymousContactsPerHour
	}
	hooks := webhook.NewClient(d.Webhooks.AllowPrivate, d.Webhooks.Resolver)
	var pusher *push.Sender
	if d.Push.Keys.PublicKey != "" {
		pusher = &push.Sender{Keys: d.Push.Keys, Client: hooks}
		if d.Push.HTTPClient != nil {
			pusher.Client = d.Push.HTTPClient
		}
	}
	srv := &Server{
		log: d.Log, st: d.Store, version: d.Version, mailer: d.Mailer, webauthn: d.WebAuthn, auth: d.Auth, now: now, sleep: sleep,
		secrets: d.Secrets, objects: d.Storage, attach: d.Attachments, sanitize: htmlPolicy(), original: originalHTMLPolicy(),
		hub: d.Hub, sockets: newClientSockets(), jobs: jobs, ingress: d.Ingress, ingestQ: make(chan struct{}, ingestSlots), sender: sender, smtpPriv: d.SMTPAllowPrivate, snsCerts: newCertCache(fetch), fetch: fetch,
		chat: chat, limits: newRateLimiter(d.Store, d.Secrets, d.Log, d.RateLimitClock, sleep), webhooks: d.Webhooks, hooks: hooks, sso: newSSOState(d.Webhooks), push: pusher, appPush: newAppPushClient(d.AppPush), issueTrackers: d.IssueTrackers,
		slack: d.Slack, slackAPI: newSlackClient(d.Slack, sleep),
		noPanel: d.DisablePanel, panelKey: d.PanelAnalyticsKey, noWidget: d.DisableWidget, operator: d.OperatorEmails, imagesMax: d.EmailImagesMaxBytes,
		broadcasts: d.Broadcasts, imports: importSettings(d.Imports), migrations: migrationSettings(d.Migrations), ses: d.SES,
		status: d.Status, statusCache: newStatusCache(), statusDomainCache: newStatusDomainCache(), started: now(),
		roadmap: d.Roadmap, changelog: d.Changelog, changelogCache: newPublicCache(changelogCacheTTL, changelogCacheMax), roadmapCache: newPublicCache(roadmapCacheTTL, roadmapCacheMax), help: d.Help, helpCounts: newHelpCounters(), helpCache: newPublicCache(helpPageCacheTTL, helpPageCacheMax),
	}
	if srv.auditDays = d.AuditDays; srv.auditDays <= 0 {
		srv.auditDays = defaultWorkspaceAuditDays
	}
	if !d.DisableMCP {
		srv.mcp, srv.mcpSchemas = newMCPHandler(), mcp.NewSchemaCache()
	}
	if !d.Analytics.Off {
		srv.analytics = srv.newAnalytics(d)
	}
	return srv
}

var _ oas.StrictServerInterface = (*Server)(nil)

// htmlPolicy keeps `cid:` sources so the panel can show inline parts of an e-mail from their attachments.
func htmlPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowURLSchemeWithCustomPolicy("cid", func(u *url.URL) bool { return u.Opaque != "" })
	return p
}

var originalStyles = []string{
	"color", "background-color", "background",
	"font", "font-family", "font-size", "font-style", "font-variant", "font-weight", "font-stretch",
	"text-align", "text-decoration", "text-decoration-color", "text-decoration-line", "text-decoration-style",
	"text-indent", "text-overflow", "text-shadow", "text-transform", "text-size-adjust",
	"line-height", "letter-spacing",
	"margin", "margin-top", "margin-right", "margin-bottom", "margin-left",
	"padding", "padding-top", "padding-right", "padding-bottom", "padding-left",
	"border", "border-color", "border-style", "border-width", "border-radius",
	"border-top", "border-right", "border-bottom", "border-left",
	"border-top-color", "border-right-color", "border-bottom-color", "border-left-color",
	"border-top-style", "border-right-style", "border-bottom-style", "border-left-style",
	"border-top-width", "border-right-width", "border-bottom-width", "border-left-width",
	"border-top-left-radius", "border-top-right-radius", "border-bottom-right-radius", "border-bottom-left-radius",
	"border-collapse", "border-spacing",
	"width", "max-width", "min-width", "height", "max-height",
	"display", "vertical-align", "table-layout", "caption-side", "empty-cells",
	"list-style", "list-style-type", "list-style-position",
	"white-space", "word-break", "overflow-wrap", "word-wrap",
}

var refusedStyleValues = []string{"url(", "image(", "image-set(", "cross-fade(", "element(", "src(", "expression", "@import", "javascript:", "\\"}

func safeStyleValue(v string) bool {
	v = strings.ToLower(v)
	for _, r := range refusedStyleValues {
		if strings.Contains(v, r) {
			return false
		}
	}
	return true
}

var (
	htmlColour     = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|[a-zA-Z]{1,30}|rgba?\([0-9., %]{1,40}\))$`)
	htmlLength     = regexp.MustCompile(`^[0-9]{1,5}(\.[0-9]{1,3})?(%|px)?$`)
	htmlFontFace   = regexp.MustCompile(`^[\p{L}\p{N} ,'"_-]{1,200}$`)
	htmlFontSize   = regexp.MustCompile(`^[+-]?[1-7]$`)
	htmlTableParts = []string{"table", "thead", "tbody", "tfoot", "tr", "td", "th", "col", "colgroup", "caption"}
)

// originalHTMLPolicy draws an e-mail as its sender designed it: htmlPolicy plus inline styles from an
// allowlist and the presentational attributes of tables and font.
func originalHTMLPolicy() *bluemonday.Policy {
	p := htmlPolicy()
	p.AllowStyles(originalStyles...).MatchingHandler(safeStyleValue).Globally()
	presentational := append(append([]string{}, htmlTableParts...), "font", "center")
	p.AllowAttrs("bgcolor").Matching(htmlColour).OnElements(htmlTableParts...)
	p.AllowAttrs("color").Matching(htmlColour).OnElements("font")
	p.AllowAttrs("face").Matching(htmlFontFace).OnElements("font")
	p.AllowAttrs("size").Matching(htmlFontSize).OnElements("font")
	p.AllowAttrs("align").Matching(bluemonday.CellAlign).OnElements(presentational...)
	p.AllowAttrs("valign").Matching(bluemonday.CellVerticalAlign).OnElements(htmlTableParts...)
	p.AllowAttrs("width", "height").Matching(htmlLength).OnElements(htmlTableParts...)
	p.AllowAttrs("border", "cellpadding", "cellspacing").Matching(bluemonday.Integer).OnElements("table")
	p.AllowNoAttrs().OnElements("font", "center")
	return p
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	strict := oas.NewStrictHandlerWithOptions(s, []oas.StrictMiddlewareFunc{s.authenticate}, oas.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  s.writeRequestError,
		ResponseErrorHandlerFunc: s.writeError,
	})
	oas.HandlerWithOptions(strict, oas.StdHTTPServerOptions{
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, errValidation(err.Error()))
		},
	})
	mux.HandleFunc("GET /v1/realtime", s.serveRealtime)
	mux.HandleFunc("GET "+signInCallbackPath, s.serveMigrationSignIn)
	mux.HandleFunc("GET "+ssoCallbackPath, s.serveSSOCallback)
	mux.HandleFunc("POST "+samlACSPath, s.serveSAMLACS)
	mux.HandleFunc("GET "+samlMetadataPath+"{workspace}", s.serveSAMLMetadata)
	mux.HandleFunc("GET /client/v1/realtime", s.serveClientRealtime)
	if s.analytics != nil {
		mux.HandleFunc("POST "+analyticsPrefix+"{key}", s.serveAnalyticsCollect)
	}
	s.routeStatus(mux)
	if !s.help.Off {
		s.routeHelp(mux)
	}
	s.routeRoadmap(mux)
	s.routeChangelog(mux)
	mux.HandleFunc("POST /ingress/email", s.serveIngressEmail)
	mux.HandleFunc("POST /ingress/ses", s.serveIngressSES)
	mux.HandleFunc("POST "+slackEventsPath, s.serveSlackEvents)
	mux.HandleFunc("GET "+slackCallbackPath, s.serveSlackCallback)
	mux.HandleFunc("GET "+heartbeatPathPrefix+"{token}", s.serveHeartbeat(false))
	mux.HandleFunc("POST "+heartbeatPathPrefix+"{token}", s.serveHeartbeat(false))
	mux.HandleFunc("GET "+heartbeatPathPrefix+"{token}/fail", s.serveHeartbeat(true))
	mux.HandleFunc("POST "+heartbeatPathPrefix+"{token}/fail", s.serveHeartbeat(true))
	mux.HandleFunc("POST "+statusPushPathPrefix+"{token}", s.serveStatusPush)
	mux.HandleFunc("POST "+issueIngressPathPrefix+"{integrationId}", s.serveIssueWebhook)
	mux.HandleFunc("GET "+emailConfirmPath, s.serveEmailConfirm)
	mux.HandleFunc("POST "+emailConfirmPath, s.serveEmailConfirmPost)
	mux.HandleFunc("GET "+ratingPathPrefix+"{token}", s.serveRatingPage)
	mux.HandleFunc("POST "+ratingPathPrefix+"{token}", s.serveRatingPost)
	mux.HandleFunc("GET "+unsubscribePathPrefix+"{token}", s.serveUnsubscribePage)
	mux.HandleFunc("POST "+unsubscribePathPrefix+"{token}", s.serveUnsubscribePost)
	mux.HandleFunc("GET "+confirmPathPrefix+"{token}", s.serveConfirmPage)
	mux.HandleFunc("POST "+confirmPathPrefix+"{token}", s.serveConfirmPost)
	mux.HandleFunc("GET "+managePathPrefix+"{token}", s.serveManagePage)
	mux.HandleFunc("POST "+managePathPrefix+"{token}", s.serveManagePost)
	mux.HandleFunc("GET "+clickPathPrefix+"{token}", s.serveClick)
	mux.HandleFunc("GET "+emailImagePathPrefix+"s/{file}", s.serveSocialIcon)
	mux.HandleFunc("GET "+emailImagePathPrefix+"{token}", s.serveEmailImage)
	mux.HandleFunc("GET "+emailTemplatesPathPrefix+"{slug}/{file}", s.serveTemplatePreview)
	mux.HandleFunc("GET "+emailTemplatesPathPrefix+"{slug}/assets/{file}", s.serveTemplateAsset)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.serveOAuthServerMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.serveResourceMetadata(s.apiResource(), "Yuva"))
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", s.serveResourceMetadata(s.mcpResource(), "Yuva MCP"))
	mux.HandleFunc("POST /oauth/register", s.serveOAuthRegister)
	mux.HandleFunc("GET /oauth/authorize", s.serveOAuthAuthorize)
	mux.HandleFunc("POST /oauth/token", s.serveOAuthToken)
	mux.HandleFunc("POST /oauth/revoke", s.serveOAuthRevoke)
	if s.mcp != nil {
		mux.HandleFunc("/mcp", s.serveMCP)
	}
	if !s.noWidget {
		for _, name := range widget.Files {
			if (name == widget.AnalyticsScript || name == widget.AnalyticsConsentScript) && s.analytics == nil {
				continue
			}
			mux.Handle("GET /"+name, widget.Handler(name))
		}
		mux.Handle("GET /widget/{file}", widget.Chunks())
	}
	var panel http.Handler
	if !s.noPanel {
		panel = ui.Handler(s.panelKey)
		mux.HandleFunc("GET "+sharePagePrefix+"{token}", s.serveSharePage)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if panel == nil || isAPIPath(r.URL.Path) {
			writeProblem(w, errNotFound)
			return
		}
		panel.ServeHTTP(w, r)
	})
	return telemetry.Middleware(s.recoverer(s.logRequests(noSniff(s.helpHost(s.statusHost(s.linkHost(oauthCORS(s.clientCORS(s.guardCookieWrites(s.limitBody(s.idempotency(telemetry.Route(mux)))))))))))))
}

var mailLinkPrefixes = []string{managePathPrefix, confirmPathPrefix, clickPathPrefix, emailImagePathPrefix}

// linkHost answers only the mail link routes on YUVA_MAIL_LINK_URL's host when it differs from the
// server's own, so the link domain never serves the panel or the API.
func (s *Server) linkHost(next http.Handler) http.Handler {
	link, err := url.Parse(s.linkBase())
	own, err2 := url.Parse(s.auth.PublicURL)
	if err != nil || err2 != nil || strings.EqualFold(link.Host, own.Host) {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Host, link.Host) && !slices.ContainsFunc(mailLinkPrefixes, func(p string) bool { return strings.HasPrefix(r.URL.Path, p) }) {
			writeProblem(w, errNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func noSniff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func isAPIPath(p string) bool {
	for _, prefix := range apiPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	for _, exact := range apiPaths {
		if p == exact {
			return true
		}
	}
	return false
}

func (s *Server) writeRequestError(w http.ResponseWriter, r *http.Request, err error) {
	writeProblem(w, errValidation(err.Error()))
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var e *apiError
	if errors.As(err, &e) {
		writeProblem(w, e)
		return
	}
	s.log.ErrorContext(r.Context(), "handler failed", slog.String("path", loggedPath(r.URL.Path)), slog.Any("error", err))
	writeProblem(w, errInternal)
}

func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(maxBodyBytes)
		if r.URL.Path == "/ingress/email" {
			limit = MaxIngressBytes + 1
		}
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
			limit += max(s.attach.MaxBytes*maxAttachmentsPerMessage, maxImportBytes)
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		took := time.Since(start)
		metrics.Requests.WithLabelValues(r.Method, strconv.Itoa(rec.status)).Inc()
		metrics.RequestSeconds.WithLabelValues(r.Method).Observe(took.Seconds())
		if strings.HasPrefix(r.URL.Path, emailImagePathPrefix) {
			return
		}
		s.log.InfoContext(r.Context(), "request", slog.String("method", r.Method), slog.String("path", loggedPath(r.URL.Path)),
			slog.Int("status", rec.status), slog.Duration("duration", took))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.ErrorContext(r.Context(), "panic", slog.Any("value", v), slog.String("path", loggedPath(r.URL.Path)))
				writeProblem(w, errInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

var tokenPathPrefixes = []string{
	sharePagePrefix, shareClientPrefix, confirmPathPrefix, managePathPrefix,
	clickPathPrefix, ratingPathPrefix, unsubscribePathPrefix, emailImagePathPrefix,
	heartbeatPathPrefix, statusPushPathPrefix,
}

// loggedPath leaves bearer tokens out of logs: anyone holding one can use the link.
func loggedPath(p string) string {
	for _, prefix := range tokenPathPrefixes {
		if rest, ok := strings.CutPrefix(p, prefix); ok {
			_, tail, _ := strings.Cut(rest, "/")
			if tail != "" {
				tail = "/" + tail
			}
			return prefix + "…" + tail
		}
	}
	return p
}

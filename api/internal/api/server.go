package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/microcosm-cc/bluemonday"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/metrics"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/push"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/secret"
	"github.com/productdevbook/yuva/api/internal/storage"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/ui"
	"github.com/productdevbook/yuva/api/internal/webhook"
	"github.com/productdevbook/yuva/api/internal/widget"
)

const maxBodyBytes = 8 << 20

var apiPrefixes = []string{"/v1/", "/client/v1/", "/ingress/", "/.well-known/"}

var apiPaths = []string{"/v1", "/client/v1", "/healthz", "/readyz", "/oauth/register", "/oauth/authorize", "/oauth/token", "/oauth/revoke", "/mcp"}

type Server struct {
	log      *slog.Logger
	st       *store.Store
	version  string
	mailer   mail.Mailer
	webauthn *webauthn.WebAuthn
	auth     AuthSettings
	now      func() time.Time
	secrets  *secret.Key
	objects  storage.Storage
	attach   AttachmentSettings
	sanitize *bluemonday.Policy
	original *bluemonday.Policy
	hub      *realtime.Hub
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
	push     *push.Sender

	mcp        http.Handler
	mcpSchemas *mcp.SchemaCache
	noPanel    bool
	noWidget   bool
}

type Deps struct {
	Log      *slog.Logger
	Store    *store.Store
	Version  string
	Mailer   mail.Mailer
	WebAuthn *webauthn.WebAuthn
	Auth     AuthSettings
	Now      func() time.Time

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

	// DisableMCP turns the /mcp endpoint off (YUVA_MCP=off).
	DisableMCP bool
	// DisablePanel stops serving the panel (YUVA_PANEL=off); /oauth/authorize then has no consent page.
	DisablePanel bool
	// DisableWidget stops serving the widget scripts (YUVA_WIDGET=off).
	DisableWidget bool
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
	// AuthservID is the authserv-id of the receiving server whose Authentication-Results are
	// trusted; without it DMARC verdicts are stored but not used.
	AuthservID string
	// MaxConcurrent caps messages processed at once; more are answered 503. 0 means 8.
	MaxConcurrent int
}

type AttachmentSettings struct {
	MaxBytes int64
	Types    []string
}

type AuthSettings struct {
	PublicURL      string
	CookieSecure   bool
	ClientIPHeader string
	// TrustedProxies are the proxies whose entries in ClientIPHeader are skipped from the right.
	TrustedProxies []netip.Prefix
	// CodeReplyDelay is how long every sign-in code request takes; 0 means 800 ms.
	CodeReplyDelay time.Duration
}

func New(d Deps) *Server {
	now := d.Now
	if now == nil {
		now = time.Now
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
		log: d.Log, st: d.Store, version: d.Version, mailer: d.Mailer, webauthn: d.WebAuthn, auth: d.Auth, now: now,
		secrets: d.Secrets, objects: d.Storage, attach: d.Attachments, sanitize: htmlPolicy(), original: originalHTMLPolicy(),
		hub: d.Hub, jobs: jobs, ingress: d.Ingress, ingestQ: make(chan struct{}, ingestSlots), sender: sender, smtpPriv: d.SMTPAllowPrivate, snsCerts: newCertCache(fetch), fetch: fetch,
		chat: chat, limits: newRateLimiter(), webhooks: d.Webhooks, hooks: hooks, push: pusher,
		noPanel: d.DisablePanel, noWidget: d.DisableWidget,
	}
	if !d.DisableMCP {
		srv.mcp, srv.mcpSchemas = newMCPHandler(), mcp.NewSchemaCache()
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
	mux.HandleFunc("GET /client/v1/realtime", s.serveClientRealtime)
	mux.HandleFunc("POST /ingress/email", s.serveIngressEmail)
	mux.HandleFunc("POST /ingress/ses", s.serveIngressSES)
	mux.HandleFunc("GET "+emailConfirmPath, s.serveEmailConfirm)
	mux.HandleFunc("POST "+emailConfirmPath, s.serveEmailConfirmPost)
	mux.HandleFunc("GET "+ratingPathPrefix+"{token}", s.serveRatingPage)
	mux.HandleFunc("POST "+ratingPathPrefix+"{token}", s.serveRatingPost)
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
			mux.Handle("GET /"+name, widget.Handler(name))
		}
	}
	var panel http.Handler
	if !s.noPanel {
		panel = ui.Handler()
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if panel == nil || isAPIPath(r.URL.Path) {
			writeProblem(w, errNotFound)
			return
		}
		panel.ServeHTTP(w, r)
	})
	return s.recoverer(s.logRequests(noSniff(oauthCORS(s.clientCORS(s.guardCookieWrites(s.limitBody(s.idempotency(mux))))))))
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
	s.log.ErrorContext(r.Context(), "handler failed", slog.String("path", r.URL.Path), slog.Any("error", err))
	writeProblem(w, errInternal)
}

func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(maxBodyBytes)
		if r.URL.Path == "/ingress/email" {
			limit = MaxIngressBytes + 1
		}
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
			limit += s.attach.MaxBytes * maxAttachmentsPerMessage
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
		s.log.Info("request", slog.String("method", r.Method), slog.String("path", r.URL.Path),
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
				s.log.ErrorContext(r.Context(), "panic", slog.Any("value", v), slog.String("path", r.URL.Path))
				writeProblem(w, errInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

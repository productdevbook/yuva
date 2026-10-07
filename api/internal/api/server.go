package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/microcosm-cc/bluemonday"
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

var apiPrefixes = []string{"/v1/", "/client/v1/", "/ingress/"}

var apiPaths = []string{"/v1", "/client/v1", "/healthz", "/readyz"}

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
	hub      *realtime.Hub
	jobs     *river.Client[pgx.Tx]
	ingress  IngressSettings
	sender   email.Sender
	snsCerts *certCache
	fetch    *http.Client
	chat     ChatSettings
	limits   *rateLimiter
	webhooks WebhookSettings
	hooks    *webhook.Client
	push     *push.Sender
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
	HTTPClient  *http.Client

	Chat ChatSettings

	Webhooks WebhookSettings

	Push PushSettings
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
}

type IngressSettings struct {
	Secret       string
	SESTopicARNs []string
}

type AttachmentSettings struct {
	MaxBytes int64
	Types    []string
}

type AuthSettings struct {
	PublicURL      string
	CookieSecure   bool
	ClientIPHeader string
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
		sender = email.SMTPSender{}
	}
	fetch := d.HTTPClient
	if fetch == nil {
		fetch = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	chat := d.Chat
	if chat.EmailDelay <= 0 {
		chat.EmailDelay = defaultChatEmailDelay
	}
	hooks := webhook.NewClient(d.Webhooks.AllowPrivate, d.Webhooks.Resolver)
	var pusher *push.Sender
	if d.Push.Keys.PublicKey != "" {
		pusher = &push.Sender{Keys: d.Push.Keys, Client: hooks}
		if d.Push.HTTPClient != nil {
			pusher.Client = d.Push.HTTPClient
		}
	}
	return &Server{
		log: d.Log, st: d.Store, version: d.Version, mailer: d.Mailer, webauthn: d.WebAuthn, auth: d.Auth, now: now,
		secrets: d.Secrets, objects: d.Storage, attach: d.Attachments, sanitize: htmlPolicy(),
		hub: d.Hub, jobs: jobs, ingress: d.Ingress, sender: sender, snsCerts: newCertCache(fetch), fetch: fetch,
		chat: chat, limits: newRateLimiter(), webhooks: d.Webhooks, hooks: hooks, push: pusher,
	}
}

var _ oas.StrictServerInterface = (*Server)(nil)

// htmlPolicy keeps `cid:` sources so the panel can show inline parts of an e-mail from their attachments.
func htmlPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowURLSchemeWithCustomPolicy("cid", func(u *url.URL) bool { return u.Opaque != "" })
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
	for _, name := range widget.Files {
		mux.Handle("GET /"+name, widget.Handler(name))
	}
	panel := ui.Handler()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			writeProblem(w, errNotFound)
			return
		}
		panel.ServeHTTP(w, r)
	})
	return s.recoverer(s.logRequests(s.clientCORS(s.limitBody(mux))))
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

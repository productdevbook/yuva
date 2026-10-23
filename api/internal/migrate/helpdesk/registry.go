package helpdesk

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// How a help desk connects.
const (
	AuthOAuth = "oauth"
	AuthToken = "token"
)

// What names the account a team moves: nothing (one API host for every account), its subdomain,
// or the base URL of a self-hosted install.
const (
	AccountNone      = ""
	AccountSubdomain = "subdomain"
	AccountBaseURL   = "base_url"
)

// The fields of a token desk's connect form, as the API names them.
const (
	FieldToken     = "token"
	FieldSecret    = "secret"
	FieldSubdomain = "subdomain"
	FieldBaseURL   = "base_url"
	FieldAccount   = "account"
)

// How the token goes on a request.
const (
	SendBearer = "bearer"
	SendBasic  = "basic"
	SendHeader = "header"
)

// SyncCursor is how a help desk's changes are read once the scan read every ticket.
type SyncCursor int

const (
	// SyncIncremental goes on from the scan's own cursor (Zendesk's incremental export).
	SyncIncremental SyncCursor = iota
	// SyncModifiedSince lists the tickets modified since the listing started, a little early.
	SyncModifiedSince
)

// Send is how the token is put on a request to the help desk's API hosts: as a Bearer token, as
// Basic auth (the token as user name, the second secret or Password as password), or in a header
// of its own.
type Send struct {
	Scheme   string
	Header   string
	Password string
}

// Limit is one of a help desk's rate limits: its window (a minute, ten seconds or a Day), the
// requests a window Yuva makes until the help desk reports the limit, and where it reports it.
type Limit struct {
	Window  time.Duration
	Default int
	Headers []string
}

// Account is what names the team's account at the help desk.
type Account struct {
	Subdomain string `json:"subdomain,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	ID        string `json:"id,omitempty"`
}

// Credentials are what the reader sends: the access or API token, and a token desk's second
// secret.
type Credentials struct {
	Token  func() string
	Secret string
}

// OAuth is a help desk's sign-in with the team's own OAuth client.
type OAuth struct {
	PKCE         bool
	AuthorizeURL func(a Account, clientID, redirect, state, challenge string) string
	Exchange     func(ctx context.Context, cfg Config, a Account, app App, code, redirect, verifier string) (Tokens, error)
	Refresh      func(ctx context.Context, cfg Config, a Account, app App, refresh string) (Tokens, error)
}

// Desk is one help desk Yuva reads: everything that differs between help desks, so the rest of
// the migration reads this entry instead of asking which help desk it is.
type Desk struct {
	Source string
	Name   string
	Guide  string
	Auth   string
	// Account is what names the account; Secret, that a token desk needs a second secret.
	Account string
	Secret  bool
	// AccountID: the connect form names which of the token's accounts to read (Chatwoot's).
	AccountID bool
	// Scopes are what an OAuth sign-in asks for and accepts nothing beyond; empty when the help
	// desk has no read-only permission.
	Scopes []string
	// Admin: only an admin's token may read the account.
	Admin bool
	// APIHosts are where every API request goes and the token with it.
	APIHosts func(Account) Hosts
	// FileHosts are other hosts an OAuth desk keeps attachments on that take its token; a token
	// desk's token goes only to its API hosts.
	FileHosts func(Account) Hosts
	// FilesAnywhere lets attachments be fetched from any host the dialer allows, without the token.
	FilesAnywhere bool
	// FilesNoToken: attachments are fetched by signed URLs and never carry the token.
	FilesNoToken bool
	Send         Send
	Limits       []Limit
	// IncrementalMax caps ClassIncremental requests a minute.
	IncrementalMax int
	// IncrementalShare: the cap is this share of the reported limit instead (Front's search guard).
	IncrementalShare float64
	RetryHeaders     []string
	// AttributePrefix names a contact's attributes from this help desk (zendesk_id).
	AttributePrefix string
	Sync            SyncCursor
	OAuth           *OAuth

	read func(c *client, a Account) Source
}

var (
	registryMu sync.RWMutex
	registry   = []Desk{zendeskDesk, helpScoutDesk, chatwootDesk, frontDesk, freshdeskDesk}
)

// Desks are the help desks in the order the panel shows them.
func Desks() []Desk {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return slices.Clone(registry)
}

func Lookup(source string) (Desk, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, d := range registry {
		if d.Source == source {
			return d, true
		}
	}
	return Desk{}, false
}

// Replace swaps the entry of d.Source for d until restore is called; for tests only.
func Replace(d Desk) (restore func()) {
	registryMu.Lock()
	defer registryMu.Unlock()
	i := slices.IndexFunc(registry, func(o Desk) bool { return o.Source == d.Source })
	if i < 0 {
		panic("helpdesk.Replace: no desk " + d.Source)
	}
	old := registry[i]
	if d.read == nil {
		d.read = old.read
	}
	registry[i] = d
	return func() {
		registryMu.Lock()
		defer registryMu.Unlock()
		registry[i] = old
	}
}

// Fields are what a token desk's connect form asks for, the token first.
func (d Desk) Fields() []string {
	if d.Auth != AuthToken {
		return nil
	}
	out := []string{FieldToken}
	if d.Secret {
		out = append(out, FieldSecret)
	}
	switch d.Account {
	case AccountSubdomain:
		out = append(out, FieldSubdomain)
	case AccountBaseURL:
		out = append(out, FieldBaseURL)
	}
	if d.AccountID {
		out = append(out, FieldAccount)
	}
	return out
}

// Host is the account's API host, which the stored credentials name.
func (d Desk) Host(a Account) string {
	h := d.APIHosts(a)
	if len(h.Exact) == 0 {
		return ""
	}
	return h.Exact[0]
}

// Budget sets b's limits from the desk's; a nil b is a new budget.
func (d Desk) Budget(b *Budget) *Budget {
	if b == nil {
		b = &Budget{}
	}
	b.IncrementalMax, b.IncrementalShare = d.IncrementalMax, d.IncrementalShare
	for _, l := range d.Limits {
		if l.Window >= Day {
			b.DayDefault, b.DayHeaders = l.Default, l.Headers
			continue
		}
		b.Default, b.Window, b.LimitHeaders = l.Default, l.Window, l.Headers
	}
	return b
}

// PerMinute is the requests a minute Yuva makes until the help desk reports its limit.
func (d Desk) PerMinute() int {
	for _, l := range d.Limits {
		if l.Window > 0 && l.Window < Day {
			return int(int64(l.Default) * int64(time.Minute) / int64(l.Window))
		}
	}
	return 0
}

// PerDay is the requests a day Yuva makes until the help desk reports its daily limit.
func (d Desk) PerDay() int {
	for _, l := range d.Limits {
		if l.Window >= Day {
			return l.Default
		}
	}
	return 0
}

// Open reads the account a with creds within budget.
func (d Desk) Open(cfg Config, a Account, creds Credentials, budget *Budget) Source {
	return d.read(d.client(cfg, a, creds, budget), a)
}

func (d Desk) client(cfg Config, a Account, creds Credentials, budget *Budget) *client {
	api := d.APIHosts(a)
	tokenHosts := api
	if d.Auth == AuthOAuth && d.FileHosts != nil {
		f := d.FileHosts(a)
		tokenHosts = Hosts{Exact: append(slices.Clone(api.Exact), f.Exact...), Suffixes: append(slices.Clone(api.Suffixes), f.Suffixes...)}
	}
	files := api
	if d.Auth == AuthToken && d.FileHosts != nil {
		f := d.FileHosts(a)
		files = Hosts{Exact: append(slices.Clone(api.Exact), f.Exact...), Suffixes: append(slices.Clone(api.Suffixes), f.Suffixes...), Pattern: f.Pattern}
	}
	if d.FilesAnywhere {
		files = Hosts{Any: true}
	}
	b := bearer{hosts: tokenHosts, token: creds.Token, send: d.Send, secret: creds.Secret}
	fb := b
	if d.FilesNoToken {
		fb.hosts = Hosts{}
	}
	return &client{
		cfg: cfg, budget: d.Budget(budget), retryHeaders: d.RetryHeaders,
		api: cfg.dataClient(api, b), files: cfg.dataClient(files, fb),
	}
}

var errBaseURL = errors.New("the base URL must be an HTTPS address such as https://help.example.com, without a user name, query or fragment")

// BaseURL checks a team-entered base URL and gives it without a trailing slash.
func BaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", errBaseURL
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u.String(), nil
}

// BaseURLHosts are a base URL account's API hosts: its own.
func BaseURLHosts(a Account) Hosts {
	u, err := url.Parse(a.BaseURL)
	if err != nil || u.Hostname() == "" {
		return Hosts{}
	}
	return Hosts{Exact: []string{strings.ToLower(u.Hostname())}, Port: u.Port()}
}

var zendeskDesk = Desk{
	Source: "zendesk", Name: "Zendesk", Guide: "migrate-zendesk", Auth: AuthOAuth, Account: AccountSubdomain,
	Scopes: []string{ZendeskScope}, Admin: true,
	APIHosts:       func(a Account) Hosts { return Hosts{Exact: []string{ZendeskHost(a.Subdomain)}} },
	FileHosts:      func(Account) Hosts { return Hosts{Suffixes: []string{"zdusercontent.com"}} },
	FilesAnywhere:  true,
	Send:           Send{Scheme: SendBearer},
	Limits:         []Limit{{Window: time.Minute, Default: 100, Headers: []string{"X-Rate-Limit", "X-Ratelimit-Limit"}}},
	IncrementalMax: 5, RetryHeaders: []string{"Retry-After"}, AttributePrefix: "zendesk", Sync: SyncIncremental,
	OAuth: &OAuth{
		PKCE: true,
		AuthorizeURL: func(a Account, clientID, redirect, state, challenge string) string {
			return ZendeskAuthorizeURL(a.Subdomain, clientID, redirect, state, challenge)
		},
		Exchange: func(ctx context.Context, cfg Config, a Account, app App, code, redirect, verifier string) (Tokens, error) {
			return ZendeskExchange(ctx, cfg, a.Subdomain, app, code, redirect, verifier)
		},
		Refresh: func(ctx context.Context, cfg Config, a Account, app App, refresh string) (Tokens, error) {
			return ZendeskRefresh(ctx, cfg, a.Subdomain, app, refresh)
		},
	},
	read: func(c *client, a Account) Source { return &Zendesk{c: c, host: ZendeskHost(a.Subdomain)} },
}

var helpScoutDesk = Desk{
	Source: "helpscout", Name: "Help Scout", Guide: "migrate-helpscout", Auth: AuthOAuth, Account: AccountNone,
	Scopes:       []string{},
	APIHosts:     func(Account) Hosts { return Hosts{Exact: []string{HelpScoutAPIHost}} },
	Send:         Send{Scheme: SendBearer},
	Limits:       []Limit{{Window: time.Minute, Default: 100, Headers: []string{"X-RateLimit-Limit-Minute"}}},
	RetryHeaders: []string{"X-RateLimit-Retry-After", "Retry-After"}, AttributePrefix: "helpscout", Sync: SyncModifiedSince,
	OAuth: &OAuth{
		AuthorizeURL: func(_ Account, clientID, _, state, _ string) string { return HelpScoutAuthorizeURL(clientID, state) },
		Exchange: func(ctx context.Context, cfg Config, _ Account, app App, code, _, _ string) (Tokens, error) {
			return HelpScoutExchange(ctx, cfg, app, code)
		},
		Refresh: func(ctx context.Context, cfg Config, _ Account, app App, refresh string) (Tokens, error) {
			return HelpScoutRefresh(ctx, cfg, app, refresh)
		},
	},
	read: func(c *client, _ Account) Source { return &HelpScout{c: c} },
}

var chatwootDesk = Desk{
	Source: "chatwoot", Name: "Chatwoot", Guide: "migrate-chatwoot", Auth: AuthToken, Account: AccountBaseURL, AccountID: true, Admin: true,
	APIHosts:      BaseURLHosts,
	FilesAnywhere: true, FilesNoToken: true,
	Send:         Send{Scheme: SendHeader, Header: "api_access_token"},
	Limits:       []Limit{{Window: time.Minute, Default: 60}},
	RetryHeaders: []string{"Retry-After"}, AttributePrefix: "chatwoot", Sync: SyncModifiedSince,
	read: func(c *client, a Account) Source { return &Chatwoot{c: c, base: a.BaseURL, account: a.ID} },
}

var frontDesk = Desk{
	Source: "front", Name: "Front", Guide: "migrate-front", Auth: AuthToken, Account: AccountNone,
	APIHosts: func(Account) Hosts { return Hosts{Exact: []string{FrontAPIHost}} },
	Send:     Send{Scheme: SendBearer},
	Limits:   []Limit{{Window: time.Minute, Default: 25, Headers: []string{"X-Ratelimit-Limit"}}},
	// The search guard is 40% of the company's limit; 20 is that of the smallest plan.
	IncrementalMax: 20, IncrementalShare: 0.4, RetryHeaders: []string{"Retry-After"}, AttributePrefix: "front", Sync: SyncModifiedSince,
	read: func(c *client, _ Account) Source { return &Front{c: c} },
}

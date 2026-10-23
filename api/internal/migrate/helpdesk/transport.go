// Package helpdesk reads the help desks of its registry (Desks) over their APIs, only ever with GET:
// every data request goes through a transport that refuses any other method, and the one POST
// Yuva sends, the OAuth token exchange, goes through a client that reaches the token endpoint
// and nothing else.
package helpdesk

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// RefusedError is a request the transport kept from leaving the process.
type RefusedError struct {
	Method string
	Host   string
}

func (e *RefusedError) Error() string {
	return "refused to send " + e.Method + " to " + e.Host
}

// Hosts is where a client may send requests: exact names and every subdomain of the suffixes;
// Any lets an attachment client reach any host the dialer allows.
type Hosts struct {
	Exact    []string
	Suffixes []string
	Any      bool
	// Port is the port a token host is reached on; empty means 443.
	Port string
	// Pattern matches host names no list can name, such as an object store's regional hosts.
	Pattern *regexp.Regexp
}

// AllowsURL is Allows for the host and the port of u, which a token goes to.
func (h Hosts) AllowsURL(u *url.URL) bool {
	port := u.Port()
	if port == "" {
		port = "443"
	}
	want := h.Port
	if want == "" {
		want = "443"
	}
	return h.Allows(u.Hostname()) && port == want
}

func (h Hosts) Allows(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return false
	}
	if h.Any {
		return true
	}
	for _, e := range h.Exact {
		if host == e {
			return true
		}
	}
	for _, s := range h.Suffixes {
		if strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return h.Pattern != nil && h.Pattern.MatchString(host)
}

// getOnly is the transport under every data client: a request that is not a GET over HTTPS to an
// allowed host is refused before anything is sent.
type getOnly struct {
	hosts Hosts
	next  http.RoundTripper
}

func (g getOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet || r.URL.Scheme != "https" || !g.hosts.Allows(r.URL.Hostname()) {
		closeBody(r)
		return nil, &RefusedError{Method: r.Method, Host: r.URL.Host}
	}
	return g.next.RoundTrip(r)
}

// tokenOnly is the transport of the token client: one POST to one URL.
type tokenOnly struct {
	url  string
	next http.RoundTripper
}

func (t tokenOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	u := *r.URL
	u.RawQuery, u.Fragment = "", ""
	if r.Method != http.MethodPost || u.String() != t.url {
		closeBody(r)
		return nil, &RefusedError{Method: r.Method, Host: r.URL.Host}
	}
	return t.next.RoundTrip(r)
}

func closeBody(r *http.Request) {
	if r.Body != nil {
		_ = r.Body.Close()
	}
}

// bearer adds the token, as send says (a Bearer token when it is empty), to requests for the
// token's own hosts, and only those, so a redirect or an attachment elsewhere never carries it.
type bearer struct {
	hosts  Hosts
	token  func() string
	send   Send
	secret string
	next   http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Del("Authorization")
	if b.send.Header != "" {
		r.Header.Del(b.send.Header)
	}
	if b.hosts.AllowsURL(r.URL) {
		for k, v := range b.send.Headers {
			r.Header.Set(k, v)
		}
		switch b.send.Scheme {
		case SendBasic:
			password := b.secret
			if password == "" {
				password = b.send.Password
			}
			r.SetBasicAuth(b.token(), password)
		case SendHeader:
			r.Header.Set(b.send.Header, b.token())
		default:
			r.Header.Set("Authorization", "Bearer "+b.token())
		}
	}
	return b.next.RoundTrip(r)
}

// Config is how a reader reaches the help desk. Dial connects to a host after the caller's
// address checks; RootCAs replaces the system's, for tests; Now is the clock budgets use.
type Config struct {
	Dial    func(ctx context.Context, network, addr string) (net.Conn, error)
	RootCAs *x509.CertPool
	Now     func() time.Time
	Timeout time.Duration
	// MaxBytes is the largest response body read, an attachment included.
	MaxBytes int64
	// Sleep waits for the budget; tests move their clock instead.
	Sleep func(ctx context.Context, d time.Duration) error
}

func (c Config) now() time.Time {
	if c.Now == nil {
		return time.Now()
	}
	return c.Now()
}

func (c Config) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c Config) base() *http.Transport {
	t := &http.Transport{
		TLSClientConfig:       &tls.Config{RootCAs: c.RootCAs, MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConnsPerHost:   2,
		Proxy:                 nil,
	}
	if c.Dial != nil {
		t.DialContext = c.Dial
	}
	return t
}

func (c Config) timeout() time.Duration {
	if c.Timeout <= 0 {
		return 40 * time.Second
	}
	return c.Timeout
}

// dataClient sends GETs to hosts, adding the token as auth says for its hosts.
func (c Config) dataClient(hosts Hosts, auth bearer) *http.Client {
	auth.next = getOnly{hosts: hosts, next: c.base()}
	return &http.Client{
		Transport: auth,
		Timeout:   c.timeout(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

func (c Config) tokenClient(tokenURL string) *http.Client {
	return &http.Client{
		Transport:     tokenOnly{url: tokenURL, next: c.base()},
		Timeout:       c.timeout(),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// StatusError is an answer the help desk gave that is not a success.
type StatusError struct {
	Status int
	Path   string
	Body   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s answered %d %s", e.Path, e.Status, e.Body)
}

// AuthError is a refused token: the sign-in no longer works.
type AuthError struct{ StatusError }

// LimitError is the help desk asking Yuva to wait.
type LimitError struct{ Until time.Time }

func (e *LimitError) Error() string {
	return "the help desk asked to wait until " + e.Until.Format(time.RFC3339)
}

// WaitError is Yuva's own budget asking to wait; Incremental when for the incremental exports,
// Daily when for the day's budget.
type WaitError struct {
	Until       time.Time
	Incremental bool
	Daily       bool
}

func (e *WaitError) Error() string {
	return "waiting for the request budget until " + e.Until.Format(time.RFC3339)
}

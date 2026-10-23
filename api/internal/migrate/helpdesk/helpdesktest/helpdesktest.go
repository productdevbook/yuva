// Package helpdesktest serves recorded Zendesk, Help Scout, Chatwoot and Front answers from a local TLS server for
// tests: every request is recorded, the API refuses a missing or stale token, and the rate-limit
// headers are the ones the providers document.
package helpdesktest

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

//go:embed testdata
var fixtures embed.FS

const (
	ZendeskSubdomain = "acme"
	ZendeskCode      = "zd-code"
	HelpScoutCode    = "hs-code"
	ZendeskLimit     = 20
	HelpScoutLimit   = 40
	FrontToken       = "front-api-token-8f3a21"
	FrontLimit       = 30
	FrontRetryAfter  = 7
	OutsideHost      = "files.example.org"
	ChatwootHost     = "chatwoot.acme.example"
	ChatwootToken    = "cw-admin-token-1"
)

// Request is one request the server saw, at the test's clock.
type Request struct {
	Method string
	Host   string
	Path   string
	Query  string
	Auth   string
	Token  string
	Body   string
	At     time.Time
}

type Server struct {
	Pool *x509.CertPool
	// ZendeskScope is the scope the Zendesk token endpoint grants.
	ZendeskScope string
	// ZendeskExpires is the access tokens' lifetime in seconds; 0 sends null.
	ZendeskExpires int
	// FrontThrottle is how many of the next Front data requests are answered 429 with Retry-After.
	FrontThrottle int

	t        testing.TB
	srv      *httptest.Server
	now      func() time.Time
	mu       sync.Mutex
	requests []Request
	files    map[string][]byte
	access   map[string]bool
	refresh  map[string]bool
	issued   int

	throttle      int
	throttleRetry string
}

// New starts the server; now is the clock requests are recorded at.
func New(t testing.TB, now func() time.Time) *Server {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "helpdesk fixtures"},
		DNSNames:  []string{"*.zendesk.com", "*.zdusercontent.com", "api.helpscout.net", "secure.helpscout.net", "api2.frontapp.com", OutsideHost, ChatwootHost},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	s := &Server{Pool: pool, ZendeskScope: "read", ZendeskExpires: 1800, t: t, now: now, files: map[string][]byte{},
		access: map[string]bool{}, refresh: map[string]bool{}}
	s.srv = httptest.NewUnstartedServer(http.HandlerFunc(s.serve))
	s.srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	s.srv.StartTLS()
	t.Cleanup(s.srv.Close)
	return s
}

// Dial connects every host to the fixture server.
func (s *Server) Dial(ctx context.Context, _, _ string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", s.srv.Listener.Addr().String())
}

// Override answers a fixture file with body from now on.
func (s *Server) Override(name string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[name] = body
}

func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

func (s *Server) file(name string) ([]byte, bool) {
	s.mu.Lock()
	b, ok := s.files[name]
	s.mu.Unlock()
	if ok {
		return b, true
	}
	b, err := fixtures.ReadFile("testdata/" + name)
	return b, err == nil
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	host := strings.ToLower(r.Host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	s.mu.Lock()
	s.requests = append(s.requests, Request{Method: r.Method, Host: host, Path: r.URL.Path, Query: r.URL.RawQuery, Auth: r.Header.Get("Authorization"),
		Token: r.Header.Get("api_access_token"), Body: string(body), At: s.now()})
	s.mu.Unlock()
	switch {
	case host == ZendeskSubdomain+".zendesk.com":
		w.Header().Set("X-Rate-Limit", fmt.Sprint(ZendeskLimit))
		s.zendesk(w, r, body)
	case strings.HasSuffix(host, ".zdusercontent.com"):
		s.attachment(w, r.URL.Path)
	case host == OutsideHost:
		s.attachment(w, r.URL.Path)
	case host == ChatwootHost:
		s.chatwoot(w, r)
	case host == "api.helpscout.net":
		w.Header().Set("X-RateLimit-Limit-Minute", fmt.Sprint(HelpScoutLimit))
		s.helpscout(w, r, body)
	case host == "api2.frontapp.com":
		w.Header().Set("X-Ratelimit-Limit", fmt.Sprint(FrontLimit))
		s.front(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) json(w http.ResponseWriter, r *http.Request, name string) {
	b, ok := s.file(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

func (s *Server) authorized(w http.ResponseWriter, r *http.Request) bool {
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	s.mu.Lock()
	valid := ok && s.access[tok]
	s.mu.Unlock()
	if !valid {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
	}
	return valid
}

func (s *Server) issue(w http.ResponseWriter, prefix, scope string, expires int) {
	s.mu.Lock()
	s.issued++
	access, refresh := fmt.Sprintf("%s-access-%d", prefix, s.issued), fmt.Sprintf("%s-refresh-%d", prefix, s.issued)
	s.access[access], s.refresh[refresh] = true, true
	s.mu.Unlock()
	out := map[string]any{"access_token": access, "token_type": "bearer", "refresh_token": refresh}
	if scope != "" {
		out["scope"] = scope
	}
	if expires > 0 {
		out["expires_in"] = expires
	} else {
		out["expires_in"] = nil
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) takeRefresh(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.refresh[token] {
		return false
	}
	delete(s.refresh, token)
	return true
}

func tokenRefused(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"The provided authorization grant is invalid"}`))
}

func (s *Server) zendesk(w http.ResponseWriter, r *http.Request, body []byte) {
	if r.URL.Path == "/oauth/tokens" && r.Method == http.MethodPost {
		var in map[string]string
		_ = json.Unmarshal(body, &in)
		switch {
		case in["grant_type"] == "authorization_code" && in["code"] == ZendeskCode && in["code_verifier"] != "" && in["client_secret"] == "zd-secret":
			s.issue(w, "zd", s.ZendeskScope, s.ZendeskExpires)
		case in["grant_type"] == "refresh_token" && s.takeRefresh(in["refresh_token"]):
			s.issue(w, "zd", s.ZendeskScope, s.ZendeskExpires)
		default:
			tokenRefused(w)
		}
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/attachments/token/") {
		if s.authorized(w, r) {
			s.attachment(w, r.URL.Path)
		}
		return
	}
	if !s.authorized(w, r) {
		return
	}
	q := r.URL.Query()
	p := strings.TrimSuffix(r.URL.Path, ".json")
	switch {
	case p == "/api/v2/users/me":
		s.json(w, r, "zendesk/users_me.json")
	case p == "/api/v2/brands":
		s.json(w, r, "zendesk/brands.json")
	case p == "/api/v2/recipient_addresses":
		s.json(w, r, "zendesk/recipient_addresses.json")
	case p == "/api/v2/users" && len(q["role[]"]) > 0:
		s.json(w, r, "zendesk/agents.json")
	case p == "/api/v2/users/show_many":
		s.showMany(w, r, strings.Split(q.Get("ids"), ","))
	case p == "/api/v2/incremental/tickets/cursor":
		cursor := q.Get("cursor")
		switch {
		case cursor == "":
			s.json(w, r, "zendesk/tickets_1.json")
		case cursor == "zd-c1":
			s.json(w, r, "zendesk/tickets_2.json")
		default:
			if _, ok := s.file("zendesk/tickets_" + cursor + ".json"); ok {
				s.json(w, r, "zendesk/tickets_"+cursor+".json")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"tickets": []any{}, "after_cursor": cursor, "before_cursor": nil, "end_of_stream": true})
		}
	case strings.HasPrefix(p, "/api/v2/tickets/") && strings.HasSuffix(p, "/comments"):
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v2/tickets/"), "/comments")
		name := "zendesk/comments_" + id
		if after := q.Get("page[after]"); after != "" {
			name += "_" + after
		}
		s.json(w, r, name+".json")
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) showMany(w http.ResponseWriter, r *http.Request, ids []string) {
	b, _ := s.file("zendesk/users.json")
	var all struct {
		Users []map[string]any `json:"users"`
	}
	_ = json.Unmarshal(b, &all)
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := struct {
		Users []map[string]any `json:"users"`
	}{Users: []map[string]any{}}
	for _, u := range all.Users {
		if want[fmt.Sprint(u["id"])] {
			out.Users = append(out.Users, u)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// modifiedSince answers the conversations of conversations_since.json changed after since, as
// Help Scout filters them.
func (s *Server) modifiedSince(w http.ResponseWriter, since string) {
	at, _ := time.Parse(time.RFC3339, since)
	b, _ := s.file("helpscout/conversations_since.json")
	var page map[string]any
	_ = json.Unmarshal(b, &page)
	embedded, _ := page["_embedded"].(map[string]any)
	var kept []any
	if embedded != nil {
		list, _ := embedded["conversations"].([]any)
		for _, c := range list {
			updated, _ := time.Parse(time.RFC3339, fmt.Sprint(c.(map[string]any)["userUpdatedAt"]))
			if updated.After(at) {
				kept = append(kept, c)
			}
		}
	}
	if kept == nil {
		kept = []any{}
	}
	out := map[string]any{"_embedded": map[string]any{"conversations": kept}, "page": map[string]any{"size": 25, "totalElements": len(kept), "totalPages": 1, "number": 1}}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// Attachment is the content the server answers for a file path.
func Attachment(path string) []byte {
	if strings.HasSuffix(strings.TrimSuffix(path, "/"), "png") || strings.Contains(path, "tok-") {
		var buf bytes.Buffer
		_ = png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1)))
		return buf.Bytes()
	}
	sum := sha256.Sum256([]byte(path))
	return []byte(fmt.Sprintf("file %s %x\n", path, sum[:4]))
}

func (s *Server) attachment(w http.ResponseWriter, path string) {
	_, _ = w.Write(Attachment(path))
}

func (s *Server) helpscout(w http.ResponseWriter, r *http.Request, body []byte) {
	if r.URL.Path == "/v2/oauth2/token" && r.Method == http.MethodPost {
		form, _ := url.ParseQuery(string(body))
		switch {
		case form.Get("grant_type") == "authorization_code" && form.Get("code") == HelpScoutCode && form.Get("client_secret") == "hs-secret":
			s.issue(w, "hs", "", 172800)
		case form.Get("grant_type") == "refresh_token" && s.takeRefresh(form.Get("refresh_token")):
			s.issue(w, "hs", "", 172800)
		default:
			tokenRefused(w)
		}
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(w, r) {
		return
	}
	q := r.URL.Query()
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.URL.Path == "/v2/users/me":
		s.json(w, r, "helpscout/users_me.json")
	case r.URL.Path == "/v2/mailboxes":
		s.json(w, r, "helpscout/mailboxes.json")
	case r.URL.Path == "/v2/users":
		s.json(w, r, "helpscout/users.json")
	case r.URL.Path == "/v2/conversations":
		switch {
		case q.Get("modifiedSince") != "":
			s.modifiedSince(w, q.Get("modifiedSince"))
		case q.Get("page") == "2":
			s.json(w, r, "helpscout/conversations_2.json")
		default:
			s.json(w, r, "helpscout/conversations_1.json")
		}
	case len(parts) == 4 && parts[1] == "conversations" && parts[3] == "threads":
		s.json(w, r, "helpscout/threads_"+parts[2]+".json")
	case len(parts) == 6 && parts[3] == "threads" && parts[5] == "original-source":
		b, ok := s.file("helpscout/customer_" + parts[2] + "_" + parts[4] + ".eml")
		if !ok || r.Header.Get("Accept") != "message/rfc822" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "message/rfc822")
		_, _ = w.Write(b)
	case len(parts) == 6 && parts[3] == "attachments" && parts[5] == "data":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"data": base64.StdEncoding.EncodeToString(Attachment(r.URL.Path))})
	case len(parts) == 3 && parts[1] == "customers":
		s.json(w, r, "helpscout/customer_"+parts[2]+".json")
	default:
		http.NotFound(w, r)
	}
}

// ThrottleChatwoot answers the next n Chatwoot conversation listings with 429, with a Retry-After
// of retryAfter seconds when it is not empty.
func (s *Server) ThrottleChatwoot(n int, retryAfter string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.throttle, s.throttleRetry = n, retryAfter
}

func (s *Server) chatwoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/rails/") {
		s.attachment(w, r.URL.Path)
		return
	}
	if r.Header.Get("api_access_token") != ChatwootToken || r.Header.Get("Authorization") != "" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"You need to sign in or sign up before continuing."}`))
		return
	}
	q := r.URL.Query()
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.URL.Path == "/api/v1/profile":
		s.json(w, r, "chatwoot/profile.json")
	case len(parts) >= 5 && parts[0] == "api" && parts[2] == "accounts":
		if parts[3] != "1" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"You are not authorized to do this action"}`))
			return
		}
		s.chatwootAccount(w, r, parts[4:], q)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) chatwootAccount(w http.ResponseWriter, r *http.Request, parts []string, q url.Values) {
	switch {
	case len(parts) == 1 && parts[0] == "inboxes":
		s.json(w, r, "chatwoot/inboxes.json")
	case len(parts) == 1 && parts[0] == "agents":
		s.json(w, r, "chatwoot/agents.json")
	case len(parts) == 1 && parts[0] == "conversations":
		s.mu.Lock()
		throttled, retry := s.throttle > 0, s.throttleRetry
		if throttled {
			s.throttle--
		}
		s.mu.Unlock()
		if throttled {
			if retry != "" {
				w.Header().Set("Retry-After", retry)
			}
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("Retry later\n"))
			return
		}
		switch q.Get("page") {
		case "2":
			s.json(w, r, "chatwoot/conversations_2.json")
		case "", "1":
			s.json(w, r, "chatwoot/conversations_1.json")
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"meta":{},"payload":[]}}`))
		}
	case len(parts) == 3 && parts[0] == "conversations" && parts[2] == "messages":
		name := "chatwoot/messages_" + parts[1]
		if before := q.Get("before"); before != "" {
			name += "_before_" + before
		}
		if _, ok := s.file(name + ".json"); !ok && q.Get("before") == "" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"payload":[{"id":9000,"content":"Hello","message_type":0,"private":false,"created_at":1760000000,"sender_type":"Contact","sender_id":12,"sender":{"id":12,"name":"Carol Example","email":"carol@example.net","type":"contact"},"attachments":[]}]}`))
			return
		}
		if _, ok := s.file(name + ".json"); !ok {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"payload":[]}`))
			return
		}
		s.json(w, r, name+".json")
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) front(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+FrontToken {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"_error":{"status":401,"title":"Unauthorized","message":"Invalid token"}}`))
		return
	}
	s.mu.Lock()
	throttled := s.FrontThrottle > 0
	if throttled {
		s.FrontThrottle--
	}
	s.mu.Unlock()
	if throttled {
		w.Header().Set("Retry-After", fmt.Sprint(FrontRetryAfter))
		w.Header().Set("X-Front-Tier", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	q := r.URL.Query()
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.URL.Path == "/me":
		s.json(w, r, "front/me.json")
	case r.URL.Path == "/inboxes":
		s.json(w, r, "front/inboxes.json")
	case r.URL.Path == "/teammates":
		s.json(w, r, "front/teammates.json")
	case len(parts) == 3 && parts[0] == "inboxes" && parts[2] == "channels":
		s.json(w, r, "front/channels_"+parts[1]+".json")
	case len(parts) == 3 && parts[0] == "inboxes" && parts[2] == "conversations":
		name := "front/conversations_" + parts[1]
		if t := q.Get("page_token"); t != "" {
			name += "_" + t
		}
		s.json(w, r, name+".json")
	case len(parts) == 3 && parts[0] == "conversations" && parts[1] == "search":
		s.frontSearch(w, parts[2])
	case len(parts) == 3 && parts[0] == "conversations" && parts[2] == "messages":
		name := "front/messages_" + parts[1]
		if t := q.Get("page_token"); t != "" {
			name += "_" + t
		}
		s.json(w, r, name+".json")
	case len(parts) == 3 && parts[0] == "conversations" && parts[2] == "comments":
		s.json(w, r, "front/comments_"+parts[1]+".json")
	case len(parts) == 2 && parts[0] == "contacts" && strings.HasPrefix(parts[1], "alt:email:"):
		s.json(w, r, "front/contact_"+strings.NewReplacer("@", "_", ".", "_").Replace(strings.TrimPrefix(parts[1], "alt:email:"))+".json")
	case len(parts) == 2 && parts[0] == "download":
		s.attachment(w, r.URL.Path)
	default:
		http.NotFound(w, r)
	}
}

// frontSearch answers the conversations of front/search.json in the inbox and with activity after
// the time the query names, as Front's search filters them.
func (s *Server) frontSearch(w http.ResponseWriter, query string) {
	var inbox string
	var after float64
	for _, f := range strings.Fields(query) {
		switch k, v, _ := strings.Cut(f, ":"); k {
		case "inbox":
			inbox = v
		case "after":
			_, _ = fmt.Sscan(v, &after)
		}
	}
	b, _ := s.file("front/search.json")
	var all struct {
		Results []map[string]any `json:"_results"`
	}
	_ = json.Unmarshal(b, &all)
	byInbox := map[string]string{"cnv_1a": "inb_a1", "cnv_1b": "inb_a1", "cnv_1c": "inb_a1", "cnv_2a": "inb_b2", "cnv_2b": "inb_b2"}
	kept := []map[string]any{}
	for _, c := range all.Results {
		updated, _ := c["updated_at"].(float64)
		if byInbox[fmt.Sprint(c["id"])] == inbox && updated > after && c["status"] != "deleted" {
			kept = append(kept, c)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"_total": len(kept), "_pagination": map[string]any{"next": nil}, "_results": kept})
}

// Fixture is a fixture file as recorded, for a test that answers a changed copy.
func Fixture(name string) ([]byte, error) { return fixtures.ReadFile("testdata/" + name) }

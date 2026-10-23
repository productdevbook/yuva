package helpdesk

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type seenRequest struct {
	method, host, path, auth, header string
}

// tokenServer answers every host the test names over TLS and records what each request carried.
type tokenServer struct {
	srv  *httptest.Server
	pool *x509.CertPool
	mu   sync.Mutex
	seen []seenRequest
}

func newTokenServer(t *testing.T, header string) *tokenServer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "token desk"},
		DNSNames:  []string{"api.desk.example", "files.desk.example", "other.example", "help.acme.example"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
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
	s := &tokenServer{pool: x509.NewCertPool()}
	s.pool.AddCert(cert)
	s.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		s.mu.Lock()
		s.seen = append(s.seen, seenRequest{method: r.Method, host: host, path: r.URL.Path, auth: r.Header.Get("Authorization"), header: r.Header.Get(header)})
		s.mu.Unlock()
		switch path.Base(r.URL.Path) {
		case "redirect-out":
			http.Redirect(w, r, "https://files.desk.example/moved", http.StatusFound)
		case "redirect-other":
			http.Redirect(w, r, "https://other.example/moved", http.StatusFound)
		case "redirect-http":
			http.Redirect(w, r, "http://"+r.Host+"/plain", http.StatusFound)
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	s.srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	s.srv.StartTLS()
	t.Cleanup(s.srv.Close)
	return s
}

func (s *tokenServer) requests() []seenRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]seenRequest(nil), s.seen...)
}

func send(c *http.Client, method, url string) error {
	req, err := http.NewRequest(method, url, strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer leaked-by-caller")
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	return res.Body.Close()
}

// A token desk's token goes only in its own auth header and only on GETs over HTTPS to its API
// hosts: a non-GET, a non-HTTPS URL, another host, a redirect and an attachment host never
// receive it, and refused requests never open a connection.
func TestTokenOnlyToAPIHosts(t *testing.T) {
	const token, secret = "desk-api-token-1234", "desk-second-secret-5678"
	for _, tc := range []struct {
		name    string
		send    Send
		secret  string
		account Account
		hosts   func(Account) Hosts
		host    string
		auth    string
		header  string
	}{
		{name: "bearer", send: Send{Scheme: SendBearer}, host: "api.desk.example", auth: "Bearer " + token},
		{name: "basic with a second secret", send: Send{Scheme: SendBasic}, secret: secret, host: "api.desk.example", auth: basicAuth(token, secret)},
		{name: "basic with a fixed password", send: Send{Scheme: SendBasic, Password: "X"}, host: "api.desk.example", auth: basicAuth(token, "X")},
		{name: "named header", send: Send{Scheme: SendHeader, Header: "X-Api-Token"}, host: "api.desk.example", header: token},
		{name: "base URL", send: Send{Scheme: SendBearer}, account: Account{BaseURL: "https://help.acme.example:8443/support"}, hosts: BaseURLHosts,
			host: "help.acme.example", auth: "Bearer " + token},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTokenServer(t, "X-Api-Token")
			var dials atomic.Int32
			cfg := Config{RootCAs: srv.pool, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				dials.Add(1)
				var d net.Dialer
				return d.DialContext(ctx, "tcp", srv.srv.Listener.Addr().String())
			}}
			hosts := tc.hosts
			if hosts == nil {
				hosts = func(Account) Hosts { return Hosts{Exact: []string{"api.desk.example"}} }
			}
			d := Desk{Source: "tokendesk", Name: "Token desk", Auth: AuthToken, APIHosts: hosts, FilesAnywhere: true, Send: tc.send,
				FileHosts: func(Account) Hosts { return Hosts{Exact: []string{"files.desk.example"}} },
				Limits:    []Limit{{Window: time.Minute, Default: 1000}}}
			c := d.client(cfg, tc.account, Credentials{Token: func() string { return token }, Secret: tc.secret}, nil)
			api := "https://" + tc.host
			if tc.account.BaseURL != "" {
				api = tc.account.BaseURL
			}

			if err := send(c.api, http.MethodGet, api+"/tickets"); err != nil {
				t.Fatalf("GET to the API host: %v", err)
			}
			for _, cl := range []*http.Client{c.api, c.files} {
				for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead} {
					mustRefuse(t, send(cl, method, api+"/tickets"), method+" to the API host")
				}
				mustRefuse(t, send(cl, http.MethodGet, strings.Replace(api, "https://", "http://", 1)+"/tickets"), "plain HTTP to the API host")
			}
			before := dials.Load()
			mustRefuse(t, send(c.api, http.MethodGet, "https://other.example/steal"), "another host")
			if n := dials.Load(); n != before {
				t.Errorf("%d connections were opened for refused requests", n-before)
			}
			mustRefuse(t, send(c.api, http.MethodGet, api+"/redirect-other"), "a redirect to another host")
			mustRefuse(t, send(c.api, http.MethodGet, api+"/redirect-http"), "a redirect to plain HTTP")
			if err := send(c.files, http.MethodGet, api+"/redirect-out"); err != nil {
				t.Fatalf("attachment redirected to its file host: %v", err)
			}
			if err := send(c.files, http.MethodGet, "https://files.desk.example/attachment"); err != nil {
				t.Fatalf("attachment on its file host: %v", err)
			}

			reqs := srv.requests()
			var toAPI, elsewhere int
			for _, r := range reqs {
				if r.method != http.MethodGet {
					t.Errorf("%s %s%s was sent", r.method, r.host, r.path)
				}
				if r.host == tc.host {
					toAPI++
					if r.auth != tc.auth || r.header != tc.header {
						t.Errorf("%s%s carried Authorization %q and X-Api-Token %q, want %q and %q", r.host, r.path, r.auth, r.header, tc.auth, tc.header)
					}
					continue
				}
				elsewhere++
				if r.auth != "" || r.header != "" {
					t.Errorf("%s%s received the token: Authorization %q, X-Api-Token %q", r.host, r.path, r.auth, r.header)
				}
				if r.host != "files.desk.example" {
					t.Errorf("a request reached %s", r.host)
				}
			}
			if toAPI == 0 || elsewhere != 2 {
				t.Errorf("%d requests to the API host and %d elsewhere, want some and 2: %+v", toAPI, elsewhere, reqs)
			}
		})
	}
}

func mustRefuse(t *testing.T, err error, what string) {
	t.Helper()
	var r *RefusedError
	if !errors.As(err, &r) {
		t.Errorf("%s: %v, want a refusal", what, err)
	}
}

func basicAuth(user, password string) string {
	r, _ := http.NewRequest(http.MethodGet, "https://x.example", nil)
	r.SetBasicAuth(user, password)
	return r.Header.Get("Authorization")
}

// Package webhook signs and sends webhooks per the Standard Webhooks specification
// (https://www.standardwebhooks.com) and refuses to send them into private networks.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	SecretPrefix  = "whsec_"
	secretBytes   = 32
	Timeout       = 10 * time.Second
	MaxBodyBytes  = 64 << 10
	KeptBodyBytes = 1 << 10
)

// NewSecret returns a signing secret in its `whsec_` form and the raw key it encodes.
func NewSecret() (string, []byte) {
	key := make([]byte, secretBytes)
	_, _ = rand.Read(key)
	return SecretPrefix + base64.StdEncoding.EncodeToString(key), key
}

// Sign returns the `v1,<base64>` signature of one attempt.
func Sign(key []byte, msgID string, ts time.Time, body []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msgID + "." + strconv.FormatInt(ts.Unix(), 10) + "."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// ErrRefusedAddress is returned when the target resolves to an address webhooks may not reach.
var ErrRefusedAddress = errors.New("refused address")

var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
		"192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24",
		"203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		"::/128", "::1/128", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/32", "2001:db8::/32", "2002::/16",
		"fc00::/7", "fe80::/10", "fec0::/10", "ff00::/8",
	} {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}()

var neverAllowedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fd00:ec2::254/128"),
}

// NeverAllowed reports whether ip is link-local or a cloud metadata address, which webhooks may
// not reach even when private targets are allowed.
func NeverAllowed(ip netip.Addr) bool {
	ip = ip.WithZone("").Unmap()
	for _, p := range neverAllowedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// Blocked reports whether ip is private, loopback, link-local, multicast, reserved or otherwise
// not a public unicast address.
func Blocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// CheckURL validates an endpoint URL when it is saved: http(s), a host, no credentials, and no
// literal address or `localhost` that Blocked refuses, and never a literal address NeverAllowed
// refuses. Names are resolved on every attempt.
func CheckURL(raw string, allowPrivate bool) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return nil, errors.New("url must be an http or https URL with a host and no credentials")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	ip, ipErr := netip.ParseAddr(host)
	if ipErr == nil && NeverAllowed(ip) {
		return nil, ErrRefusedAddress
	}
	if allowPrivate {
		return u, nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, ErrRefusedAddress
	}
	if ipErr == nil && Blocked(ip) {
		return nil, ErrRefusedAddress
	}
	return u, nil
}

type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Client sends webhook requests. Every request resolves the host once, refuses it when any
// address is blocked (unless AllowPrivate) or never allowed, connects to the resolved address only, follows no
// redirects and reads at most MaxBodyBytes of the answer.
type Client struct {
	AllowPrivate bool
	Resolver     Resolver
	http         *http.Client
}

func NewClient(allowPrivate bool, r Resolver) *Client {
	if r == nil {
		r = net.DefaultResolver
	}
	c := &Client{AllowPrivate: allowPrivate, Resolver: r}
	c.http = &http.Client{
		Timeout: Timeout,
		Transport: &http.Transport{
			Proxy:                  nil,
			DialContext:            c.dial,
			DisableKeepAlives:      true,
			TLSHandshakeTimeout:    Timeout,
			ResponseHeaderTimeout:  Timeout,
			MaxResponseHeaderBytes: 64 << 10,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return c
}

func (c *Client) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, err
	}
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{ip}
	} else if ips, err = c.Resolver.LookupNetIP(ctx, "ip", host); err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("%s has no address", host)
	}
	for _, ip := range ips {
		if NeverAllowed(ip) || (!c.AllowPrivate && Blocked(ip)) {
			return nil, fmt.Errorf("%w %s for %s", ErrRefusedAddress, ip.Unmap(), host)
		}
	}
	d := net.Dialer{Timeout: Timeout}
	var last error
	for _, ip := range ips {
		conn, err := d.DialContext(ctx, network, netip.AddrPortFrom(ip.Unmap(), uint16(p)).String())
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

type Request struct {
	URL       string
	MessageID string
	Timestamp time.Time
	Body      []byte
	// Keys sign the request in order; more than one during a secret rotation.
	Keys      [][]byte
	UserAgent string
}

type Result struct {
	StatusCode int
	Latency    time.Duration
	Body       string
	Err        error
}

func (r Result) OK() bool { return r.Err == nil && r.StatusCode >= 200 && r.StatusCode < 300 }

func (c *Client) Send(ctx context.Context, req Request) Result {
	u, err := CheckURL(req.URL, c.AllowPrivate)
	if err != nil {
		return Result{Err: err}
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(req.Body))
	if err != nil {
		return Result{Err: err}
	}
	sigs := make([]string, len(req.Keys))
	for i, k := range req.Keys {
		sigs[i] = Sign(k, req.MessageID, req.Timestamp, req.Body)
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("User-Agent", req.UserAgent)
	hr.Header.Set("webhook-id", req.MessageID)
	hr.Header.Set("webhook-timestamp", strconv.FormatInt(req.Timestamp.Unix(), 10))
	hr.Header.Set("webhook-signature", strings.Join(sigs, " "))
	start := time.Now()
	res, err := c.http.Do(hr)
	if err != nil {
		return Result{Latency: time.Since(start), Err: err}
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, MaxBodyBytes))
	out := Result{StatusCode: res.StatusCode, Latency: time.Since(start), Body: truncate(b, KeptBodyBytes)}
	if err != nil {
		out.Err = err
	} else if !out.OK() {
		out.Err = fmt.Errorf("answered %d", res.StatusCode)
	}
	return out
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		b = b[:n]
	}
	return strings.ToValidUTF8(string(b), "")
}

// RetryDelays spaces the automatic attempts after the first: about 24 hours in all.
var RetryDelays = []time.Duration{
	5 * time.Second, time.Minute, 5 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour, 8 * time.Hour, 9 * time.Hour,
}

// NextDelay is the wait before automatic attempt n+1 after attempt n (1-based) failed, with up to
// 10% jitter derived from seed so the same attempt always gets the same delay.
func NextDelay(attempt int, seed []byte) (time.Duration, bool) {
	if attempt < 1 || attempt > len(RetryDelays) {
		return 0, false
	}
	d := RetryDelays[attempt-1]
	sum := sha256.Sum256(append(append([]byte{}, seed...), byte(attempt)))
	jitter := time.Duration(uint64(d/10) * uint64(sum[0]) / 255)
	return d + jitter, true
}

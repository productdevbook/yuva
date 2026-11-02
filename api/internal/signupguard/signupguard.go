// Package signupguard decides which addresses may start an open sign-up: not a throwaway domain,
// not an operator-blocked one, and a domain that can receive mail.
package signupguard

import (
	"context"
	_ "embed"
	"errors"
	"net"
	"strings"
	"time"
)

//go:embed disposable_email_blocklist.conf
var disposableList string

const mxTimeout = 3 * time.Second

// MXResolver is the part of net.Resolver the guard needs.
type MXResolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
}

type Guard struct {
	blocked  map[string]struct{}
	resolver MXResolver
}

// New blocks the embedded disposable list plus extra. A nil resolver turns the MX rule off.
func New(extra []string, resolver MXResolver) *Guard {
	g := &Guard{blocked: map[string]struct{}{}, resolver: resolver}
	for _, line := range strings.Split(disposableList, "\n") {
		g.add(line)
	}
	for _, d := range extra {
		g.add(d)
	}
	return g
}

func (g *Guard) add(d string) {
	d = strings.ToLower(strings.TrimSpace(d))
	if d == "" || strings.HasPrefix(d, "#") {
		return
	}
	g.blocked[strings.Trim(d, ".")] = struct{}{}
}

// Blocked reports whether the domain or one of its parent domains is on a list.
func (g *Guard) Blocked(domain string) bool {
	d := strings.Trim(strings.ToLower(domain), ".")
	for d != "" {
		if _, ok := g.blocked[d]; ok {
			return true
		}
		i := strings.IndexByte(d, '.')
		if i < 0 {
			return false
		}
		d = d[i+1:]
	}
	return false
}

// Refuses reports whether a new person at email must not be sent a code. A lookup that fails
// for any reason other than the domain not existing lets the address through.
func (g *Guard) Refuses(ctx context.Context, email string) bool {
	domain := Domain(email)
	if domain == "" || g.Blocked(domain) {
		return true
	}
	if g.resolver == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, mxTimeout)
	defer cancel()
	records, err := g.resolver.LookupMX(ctx, domain)
	for _, r := range records {
		if strings.Trim(r.Host, ".") != "" {
			return false
		}
	}
	if err == nil {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

// Domain is the lowercase part of email after the last @.
func Domain(email string) string {
	i := strings.LastIndexByte(email, '@')
	if i < 0 {
		return ""
	}
	return strings.Trim(strings.ToLower(email[i+1:]), ".")
}

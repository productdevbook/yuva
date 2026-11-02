package signupguard

import (
	"context"
	"errors"
	"net"
	"testing"
)

type fakeResolver struct {
	mx  []*net.MX
	err error
}

func (f fakeResolver) LookupMX(context.Context, string) ([]*net.MX, error) { return f.mx, f.err }

func TestBlocked(t *testing.T) {
	g := New([]string{" Spam.Example ", "", "bad.test."}, nil)
	for domain, want := range map[string]bool{
		"mailinator.com":         true,
		"MAILINATOR.com":         true,
		"sub.mailinator.com":     true,
		"a.b.mailinator.com":     true,
		"mailinator.com.example": false,
		"gmail.com":              false,
		"outlook.com":            false,
		"spam.example":           true,
		"x.spam.example":         true,
		"bad.test":               true,
		"example":                false,
		"":                       false,
	} {
		if got := g.Blocked(domain); got != want {
			t.Errorf("Blocked(%q) = %v, want %v", domain, got, want)
		}
	}
}

func TestRefusesMX(t *testing.T) {
	notFound := &net.DNSError{Err: "no such host", Name: "x", IsNotFound: true}
	for name, tc := range map[string]struct {
		r    fakeResolver
		want bool
	}{
		"has mx":        {fakeResolver{mx: []*net.MX{{Host: "mx.example.org."}}}, false},
		"null mx":       {fakeResolver{mx: []*net.MX{{Host: "."}}}, true},
		"no records":    {fakeResolver{}, true},
		"nxdomain":      {fakeResolver{err: notFound}, true},
		"timeout":       {fakeResolver{err: &net.DNSError{Err: "i/o timeout", IsTimeout: true, IsTemporary: true}}, false},
		"servfail":      {fakeResolver{err: &net.DNSError{Err: "server misbehaving", IsTemporary: true}}, false},
		"other error":   {fakeResolver{err: errors.New("boom")}, false},
		"mx with error": {fakeResolver{mx: []*net.MX{{Host: "mx.example.org."}}, err: errors.New("bad record")}, false},
	} {
		if got := New(nil, tc.r).Refuses(context.Background(), "a@real.example.org"); got != tc.want {
			t.Errorf("%s: Refuses = %v, want %v", name, got, tc.want)
		}
	}
}

func TestRefusesListBeforeMX(t *testing.T) {
	g := New(nil, fakeResolver{mx: []*net.MX{{Host: "mx."}}})
	if !g.Refuses(context.Background(), "a@sub.mailinator.com") {
		t.Fatal("a listed domain with MX must be refused")
	}
	if New(nil, nil).Refuses(context.Background(), "a@gmail.com") {
		t.Fatal("a nil resolver must skip the MX rule")
	}
}

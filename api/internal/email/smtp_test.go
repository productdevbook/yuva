package email_test

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/webhook"
)

type fixedResolver []netip.Addr

func (r fixedResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r, nil
}

func TestSMTPSenderRefusesPrivateAddresses(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan struct{}, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted <- struct{}{}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	send := func(s email.SMTPSender, host string) error {
		return s.Send(context.Background(), email.SMTPConfig{Host: host, Port: port, TLS: "none"}, "a@example.com", []string{"b@example.com"}, []byte("x"))
	}
	for name, tc := range map[string]struct {
		sender email.SMTPSender
		host   string
	}{
		"loopback literal":       {email.SMTPSender{}, "127.0.0.1"},
		"name resolving private": {email.SMTPSender{Resolver: fixedResolver{netip.MustParseAddr("10.1.2.3")}}, "smtp.example.com"},
		"metadata even allowed":  {email.SMTPSender{AllowPrivate: true}, "169.254.169.254"},
	} {
		err := send(tc.sender, tc.host)
		if !errors.Is(err, webhook.ErrRefusedAddress) || !email.IsPermanent(err) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(accepted) != 0 {
		t.Fatal("a refused address was dialled")
	}
	err = send(email.SMTPSender{AllowPrivate: true, Resolver: fixedResolver{netip.MustParseAddr("127.0.0.1")}}, "mailpit")
	if errors.Is(err, webhook.ErrRefusedAddress) {
		t.Fatalf("allowed private address refused: %v", err)
	}
	if len(accepted) != 1 {
		t.Fatalf("allowed private address not dialled: %v", err)
	}
}

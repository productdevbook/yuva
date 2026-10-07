package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/smtp"
	"net/textproto"
	"strings"

	"github.com/productdevbook/yuva/api/internal/webhook"
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	TLS      string
}

type Sender interface {
	Send(ctx context.Context, cfg SMTPConfig, from string, to []string, msg []byte) error
}

// PermanentError is a refusal that retrying will not change (an SMTP 5xx reply).
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

func IsPermanent(err error) bool {
	var p *PermanentError
	return errors.As(err, &p)
}

func classify(err error) error {
	var tp *textproto.Error
	if errors.As(err, &tp) && tp.Code >= 500 {
		return &PermanentError{Err: err}
	}
	return err
}

// SMTPSender sends through a channel's SMTP server. It resolves the host once and refuses
// private, loopback and other non-public addresses unless AllowPrivate, and link-local and cloud
// metadata addresses always, so a channel cannot be used to probe the server's own network.
type SMTPSender struct {
	AllowPrivate bool
	Resolver     webhook.Resolver
}

// CheckSMTPHost refuses, when a channel is saved, a host that is a literal address or `localhost`
// the sender would refuse; names are resolved and checked again on every send.
func CheckSMTPHost(host string, allowPrivate bool) error {
	host = strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	ip, ipErr := netip.ParseAddr(host)
	if ipErr == nil && webhook.NeverAllowed(ip) {
		return webhook.ErrRefusedAddress
	}
	if allowPrivate {
		return nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || (ipErr == nil && webhook.Blocked(ip)) {
		return webhook.ErrRefusedAddress
	}
	return nil
}

func (s SMTPSender) Send(ctx context.Context, cfg SMTPConfig, from string, to []string, msg []byte) error {
	conn, err := s.dial(ctx, cfg)
	if err != nil {
		return err
	}
	return classify(send(ctx, conn, cfg, from, to, msg))
}

func (s SMTPSender) dial(ctx context.Context, cfg SMTPConfig) (net.Conn, error) {
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(cfg.Host); err == nil {
		ips = []netip.Addr{ip}
	} else {
		r := s.Resolver
		if r == nil {
			r = net.DefaultResolver
		}
		if ips, err = r.LookupNetIP(ctx, "ip", cfg.Host); err != nil {
			return nil, err
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("%s has no address", cfg.Host)
	}
	for _, ip := range ips {
		if webhook.NeverAllowed(ip) || (!s.AllowPrivate && webhook.Blocked(ip)) {
			return nil, &PermanentError{Err: fmt.Errorf("%w %s for %s", webhook.ErrRefusedAddress, ip.Unmap(), cfg.Host)}
		}
	}
	var (
		d    net.Dialer
		last error
	)
	for _, ip := range ips {
		conn, err := d.DialContext(ctx, "tcp", netip.AddrPortFrom(ip.Unmap(), uint16(cfg.Port)).String())
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

func send(ctx context.Context, conn net.Conn, cfg SMTPConfig, from string, to []string, msg []byte) error {
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	tlsConfig := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	if cfg.TLS == "tls" {
		conn = tls.Client(conn, tlsConfig)
	}
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if cfg.TLS == "starttls" {
		if err := c.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

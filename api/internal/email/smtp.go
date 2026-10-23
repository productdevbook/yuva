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
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/productdevbook/yuva/api/internal/telemetry"
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

// RelaySender is a Sender that also reports the Message-ID the relay gives the message in place
// of ours, or "" when it keeps ours or does not say.
type RelaySender interface {
	SendRelayed(ctx context.Context, cfg SMTPConfig, from string, to []string, msg []byte) (string, error)
}

// SendRelayed sends msg and returns the Message-ID the relay put on it in place of ours, or "".
func SendRelayed(ctx context.Context, s Sender, cfg SMTPConfig, from string, to []string, msg []byte) (string, error) {
	ctx, span := telemetry.Start(ctx, "smtp.send", attribute.Int("yuva.recipients", len(to)), attribute.Int("yuva.message_bytes", len(msg)))
	var id string
	var err error
	if r, ok := s.(RelaySender); ok {
		id, err = r.SendRelayed(ctx, cfg, from, to, msg)
	} else {
		err = s.Send(ctx, cfg, from, to, msg)
	}
	telemetry.End(span, err)
	return id, err
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
	_, err := s.SendRelayed(ctx, cfg, from, to, msg)
	return err
}

func (s SMTPSender) SendRelayed(ctx context.Context, cfg SMTPConfig, from string, to []string, msg []byte) (string, error) {
	conn, err := s.dial(ctx, cfg)
	if err != nil {
		return "", err
	}
	reply, err := send(ctx, conn, cfg, from, to, msg)
	if err != nil {
		return "", classify(err)
	}
	return RelayMessageID(cfg.Host, DataReplyID(reply)), nil
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

func send(ctx context.Context, conn net.Conn, cfg SMTPConfig, from string, to []string, msg []byte) (string, error) {
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
		return "", err
	}
	defer c.Close()
	if cfg.TLS == "starttls" {
		if err := c.StartTLS(tlsConfig); err != nil {
			return "", err
		}
	}
	if cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return "", err
		}
	}
	reply, err := transaction(c, from, to, msg)
	if err != nil {
		return "", err
	}
	return reply, c.Quit()
}

func transaction(c *smtp.Client, from string, to []string, msg []byte) (string, error) {
	if err := c.Mail(from); err != nil {
		return "", err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return "", err
		}
	}
	return data(c, msg)
}

func Data(c *smtp.Client, msg []byte) (string, error) { return data(c, msg) }

// data sends DATA through the client's text connection, because net/smtp drops the final reply's
// text, which is where a relay names the id it gave the message.
func data(c *smtp.Client, msg []byte) (string, error) {
	id, err := c.Text.Cmd("DATA")
	if err != nil {
		return "", err
	}
	c.Text.StartResponse(id)
	_, _, err = c.Text.ReadResponse(354)
	c.Text.EndResponse(id)
	if err != nil {
		return "", err
	}
	w := c.Text.DotWriter()
	if _, err := w.Write(msg); err != nil {
		w.Close()
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	_, reply, err := c.Text.ReadResponse(250)
	return reply, err
}

var (
	queuedAs = regexp.MustCompile(`(?i)\bqueued as ([A-Za-z0-9._-]+)`)
	okID     = regexp.MustCompile(`(?i)^ok ([A-Za-z0-9._-]+)`)
	sesHost  = regexp.MustCompile(`^email-smtp(?:-fips)?\.([a-z]{2}(?:-[a-z]+)+-\d)\.amazonaws\.com$`)
	sesID    = regexp.MustCompile(`^[0-9a-f]{8,32}(?:-[0-9a-f]{4,32}){3,8}$`)
)

// DataReplyID is the id a relay gives the message in its reply to DATA ("Ok <id>" from Amazon SES,
// "queued as <id>" from Postfix and others), or "".
func DataReplyID(reply string) string {
	reply = strings.TrimSpace(reply)
	if m := enhancedStatus.FindStringIndex(reply); m != nil {
		reply = strings.TrimSpace(reply[m[1]:])
	}
	if m := okID.FindStringSubmatch(reply); m != nil {
		return m[1]
	}
	if m := queuedAs.FindStringSubmatch(reply); m != nil {
		return m[1]
	}
	return ""
}

// IsSESHost reports whether host is an Amazon SES SMTP endpoint.
func IsSESHost(host string) bool {
	return sesHost.MatchString(strings.ToLower(strings.TrimSuffix(host, ".")))
}

// SESRegion is the AWS region of an Amazon SES SMTP endpoint, or "".
func SESRegion(host string) string {
	m := sesHost.FindStringSubmatch(strings.ToLower(strings.TrimSuffix(host, ".")))
	if m == nil {
		return ""
	}
	return m[1]
}

// RelayMessageID is the Message-ID a relay puts on a message in place of ours, given the id it
// named in its reply to DATA: Amazon SES writes <id>@<region>.amazonses.com (email.amazonses.com
// in us-east-1). Other relays keep our Message-ID, and "" is returned.
func RelayMessageID(host, providerID string) string {
	m := sesHost.FindStringSubmatch(strings.ToLower(strings.TrimSuffix(host, ".")))
	if m == nil || !sesID.MatchString(providerID) {
		return ""
	}
	if m[1] == "us-east-1" {
		return providerID + "@email.amazonses.com"
	}
	return providerID + "@" + m[1] + ".amazonses.com"
}

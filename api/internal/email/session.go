package email

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/smtp"
	"net/textproto"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/productdevbook/yuva/api/internal/telemetry"
)

// Session sends several messages over one SMTP connection. Send returns the Message-ID the relay
// put on the message in place of ours, or "" (see RelayMessageID).
type Session interface {
	Send(ctx context.Context, from string, to []string, msg []byte) (string, error)
	Close() error
}

// SessionOpener connects and authenticates once for a Session.
type SessionOpener interface {
	Open(ctx context.Context, cfg SMTPConfig) (Session, error)
}

// OpenSession opens a reused connection when the sender supports it, else sends each message
// with its own connection.
func OpenSession(ctx context.Context, s Sender, cfg SMTPConfig) (Session, error) {
	if o, ok := s.(SessionOpener); ok {
		return o.Open(ctx, cfg)
	}
	return &oneShot{s: s, cfg: cfg}, nil
}

type oneShot struct {
	s   Sender
	cfg SMTPConfig
}

func (o *oneShot) Send(ctx context.Context, from string, to []string, msg []byte) (id string, err error) {
	ctx, span := telemetry.Start(ctx, "smtp.send", attribute.Int("yuva.recipients", len(to)), attribute.Int("yuva.message_bytes", len(msg)))
	defer func() { telemetry.End(span, err) }()
	return SendRelayed(ctx, o.s, o.cfg, from, to, msg)
}

func (o *oneShot) Close() error { return nil }

// SetupError is a failure to connect, secure or authenticate a session: the account or server is
// wrong, not one message.
type SetupError struct {
	Err error
	// Permanent is a refusal (bad credentials, a refused address, a TLS failure) rather than a
	// network problem that may pass.
	Permanent bool
}

func (e *SetupError) Error() string { return e.Err.Error() }
func (e *SetupError) Unwrap() error { return e.Err }

func (s SMTPSender) Open(ctx context.Context, cfg SMTPConfig) (Session, error) {
	conn, err := s.dial(ctx, cfg)
	if err != nil {
		return nil, &SetupError{Err: err, Permanent: IsPermanent(err)}
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(time.Minute))
	}
	tlsConfig := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	if cfg.TLS == "tls" {
		conn = tls.Client(conn, tlsConfig)
	}
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return nil, &SetupError{Err: err, Permanent: isTLSError(err)}
	}
	if cfg.TLS == "starttls" {
		if err := c.StartTLS(tlsConfig); err != nil {
			c.Close()
			return nil, &SetupError{Err: err, Permanent: true}
		}
	}
	if cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			c.Close()
			code, _, _ := Reply(err)
			return nil, &SetupError{Err: err, Permanent: code >= 500 || code == 0}
		}
	}
	return &smtpSession{c: c, conn: conn, host: cfg.Host}, nil
}

func isTLSError(err error) bool {
	var rec tls.RecordHeaderError
	var cert *tls.CertificateVerificationError
	return errors.As(err, &rec) || errors.As(err, &cert)
}

type smtpSession struct {
	c    *smtp.Client
	conn net.Conn
	host string
}

func (s *smtpSession) Send(ctx context.Context, from string, to []string, msg []byte) (id string, err error) {
	_, span := telemetry.Start(ctx, "smtp.send", attribute.Int("yuva.recipients", len(to)), attribute.Int("yuva.message_bytes", len(msg)))
	defer func() { telemetry.End(span, err) }()
	deadline := time.Now().Add(time.Minute)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = s.conn.SetDeadline(deadline)
	reply, err := transaction(s.c, from, to, msg)
	if err == nil {
		return RelayMessageID(s.host, DataReplyID(reply)), nil
	}
	var tp *textproto.Error
	if errors.As(err, &tp) {
		if rerr := s.c.Reset(); rerr != nil {
			return "", &BrokenError{Err: err}
		}
		return "", classify(err)
	}
	return "", &BrokenError{Err: err}
}

func (s *smtpSession) Close() error {
	if err := s.c.Quit(); err != nil {
		return s.c.Close()
	}
	return nil
}

// BrokenError is a message that failed because the connection broke; the session is unusable and
// the message may be tried again on a new one.
type BrokenError struct{ Err error }

func (e *BrokenError) Error() string { return e.Err.Error() }
func (e *BrokenError) Unwrap() error { return e.Err }

var enhancedStatus = regexp.MustCompile(`^([245]\.\d{1,3}\.\d{1,3})\b`)

// Reply is the SMTP reply code and enhanced status code (RFC 3463) of a refusal; ok is false for
// errors that are not SMTP replies.
func Reply(err error) (code int, enhanced string, ok bool) {
	var tp *textproto.Error
	if !errors.As(err, &tp) {
		return 0, "", false
	}
	if m := enhancedStatus.FindStringSubmatch(strings.TrimSpace(tp.Msg)); m != nil {
		enhanced = m[1]
	}
	return tp.Code, enhanced, true
}

// BadMailbox reports a refusal whose enhanced status names a bad mailbox or domain (5.1.x) or a
// disabled mailbox (5.2.1): a hard bounce. Other refusals (policy, full mailbox) say nothing
// about the address.
func BadMailbox(err error) bool {
	code, enhanced, ok := Reply(err)
	if !ok || code < 500 {
		return false
	}
	return strings.HasPrefix(enhanced, "5.1.") || enhanced == "5.2.1"
}

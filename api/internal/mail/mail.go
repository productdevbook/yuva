package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"embed"
	"encoding/hex"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/productdevbook/yuva/api/internal/config"
)

type Message struct {
	To      string
	Subject string
	Text    string
}

type Mailer interface {
	Send(ctx context.Context, m Message) error
}

//go:embed templates
var templatesFS embed.FS

var templates = map[string]*template.Template{
	"en": template.Must(template.ParseFS(templatesFS, "templates/en/*.txt")),
	"tr": template.Must(template.ParseFS(templatesFS, "templates/tr/*.txt")),
}

func Render(name, locale string, data any) (Message, error) {
	t, ok := templates[locale]
	if !ok {
		t = templates["en"]
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name+".txt", data); err != nil {
		return Message{}, err
	}
	subject, body, _ := strings.Cut(buf.String(), "\n\n")
	return Message{Subject: strings.TrimSpace(subject), Text: strings.TrimLeft(body, "\n")}, nil
}

type Log struct{ Log *slog.Logger }

func (l Log) Send(ctx context.Context, m Message) error {
	l.Log.InfoContext(ctx, "mail (log mailer, not sent)",
		slog.String("to", m.To), slog.String("subject", m.Subject), slog.String("text", m.Text))
	return nil
}

type Async struct {
	Mailer  Mailer
	Log     *slog.Logger
	Timeout time.Duration
}

func (a Async) Send(ctx context.Context, m Message) error {
	ctx = context.WithoutCancel(ctx)
	go func() {
		ctx, cancel := context.WithTimeout(ctx, a.Timeout)
		defer cancel()
		if err := a.Mailer.Send(ctx, m); err != nil {
			a.Log.ErrorContext(ctx, "mail not sent", slog.String("to", m.To), slog.Any("error", err))
		}
	}()
	return nil
}

type SMTP struct{ Config config.SMTP }

func (s SMTP) Send(ctx context.Context, m Message) error {
	from, err := mail.ParseAddress(s.Config.From)
	if err != nil {
		return fmt.Errorf("smtp from: %w", err)
	}
	addr := net.JoinHostPort(s.Config.Host, strconv.Itoa(s.Config.Port))
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	tlsConfig := &tls.Config{ServerName: s.Config.Host, MinVersion: tls.VersionTLS12}
	if s.Config.TLS == "tls" {
		conn = tls.Client(conn, tlsConfig)
	}
	c, err := smtp.NewClient(conn, s.Config.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if s.Config.TLS == "starttls" {
		if err := c.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if s.Config.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Config.Username, s.Config.Password, s.Config.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(m.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(compose(from, m)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func compose(from *mail.Address, m Message) []byte {
	var b bytes.Buffer
	domain := "yuva.invalid"
	if _, d, ok := strings.Cut(from.Address, "@"); ok {
		domain = d
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	header := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	header("From", from.String())
	header("To", m.To)
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", time.Now().UTC().Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	header("Auto-Submitted", "auto-generated")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	_, _ = qp.Write([]byte(strings.ReplaceAll(m.Text, "\n", "\r\n")))
	_ = qp.Close()
	return b.Bytes()
}

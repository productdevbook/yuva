package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"embed"
	"encoding/hex"
	"fmt"
	htmltemplate "html/template"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/productdevbook/yuva/api/internal/config"
	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/mail/style"
	"github.com/productdevbook/yuva/api/internal/telemetry"
)

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
	// Unsubscribe is a one-click unsubscribe URL (RFC 8058), sent as List-Unsubscribe.
	Unsubscribe string
	// MessageID is without angle brackets; empty makes a random one.
	MessageID string
	// OnRelayed gets the Message-ID a relay such as Amazon SES put on the mail in place of ours.
	OnRelayed func(ctx context.Context, relayID string)
}

type Mailer interface {
	Send(ctx context.Context, m Message) error
}

//go:embed templates
var templatesFS embed.FS

var locales = []string{"en", "tr", "de"}

var templates = map[string]*template.Template{}

var htmlTemplates = map[string]*htmltemplate.Template{}

func init() {
	for _, l := range locales {
		templates[l] = template.Must(template.ParseFS(templatesFS, "templates/"+l+"/*.txt"))
		htmlTemplates[l] = htmltemplate.Must(htmltemplate.New("layout.html").Funcs(layoutFuncs(resolveBrand(Brand{}))).
			ParseFS(templatesFS, "templates/layout.html", "templates/"+l+"/*.html"))
	}
}

// Brand is who a mail speaks for: the zero value is Yuva itself, for mails to members and people
// signing in; a contact-facing mail sent on behalf of an inbox names the inbox and its colour.
type Brand struct {
	Name  string
	Color string
}

const (
	yuvaColor      = "#c2410c"
	yuvaDarkColor  = "#ff8a5c"
	inboxColor     = "#2563eb"
	pageColor      = "#f4f4f5"
	darkCardColor  = "#131316"
	maxBrandLength = 80
	maxLinkLabel   = 40
	layoutWidth    = 560
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type brand struct {
	Name, Color, Ink, DarkColor, OnColor string
	Yuva                                 bool
}

func resolveBrand(b Brand) brand {
	if strings.TrimSpace(b.Name) == "" {
		return brand{Name: "Yuva", Color: yuvaColor, Ink: yuvaColor, DarkColor: yuvaDarkColor, OnColor: "#ffffff", Yuva: true}
	}
	name := strings.TrimSpace(b.Name)
	if r := []rune(name); len(r) > maxBrandLength {
		name = string(r[:maxBrandLength-1]) + "…"
	}
	c := strings.ToLower(b.Color)
	if !hexColor.MatchString(c) {
		c = inboxColor
	}
	return brand{Name: name, Color: c, Ink: style.ReadableOn(c, pageColor, 0), DarkColor: style.ReadableOn(c, darkCardColor, 255), OnColor: style.TextOn(c)}
}

func layoutFuncs(b brand) htmltemplate.FuncMap {
	return htmltemplate.FuncMap{
		"brand": func() brand { return b },
		"button": func(url, label string) map[string]string {
			return map[string]string{"URL": url, "Label": label}
		},
		"rating": func(goodURL, good, badURL, bad string) map[string]string {
			return map[string]string{"GoodURL": goodURL, "Good": good, "BadURL": badURL, "Bad": bad}
		},
		"shortURL": shortURL,
		"lines": func(s string) htmltemplate.HTML {
			parts := strings.Split(strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n"), "\n")
			for i, p := range parts {
				parts[i] = htmltemplate.HTMLEscapeString(p)
			}
			return htmltemplate.HTML(strings.Join(parts, "<br>"))
		},
		"buttonHTML": func(url, label string) htmltemplate.HTML {
			return htmltemplate.HTML(style.LayoutButton(url, label, b.Color, b.OnColor).HTML())
		},
		"msoOpen":  func() htmltemplate.HTML { return htmltemplate.HTML(style.MSOOpen(layoutWidth)) },
		"msoClose": func() htmltemplate.HTML { return htmltemplate.HTML(style.MSOClose()) },
	}
}

// shortURL is the link text for a URL whose query holds a long token: the host and path, shortened
// so it never has to break mid-token on a phone; the href keeps the full URL.
func shortURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	label := u.Host + strings.TrimSuffix(u.Path, "/")
	cut := u.RawQuery != "" || u.Fragment != ""
	if r := []rune(label); len(r) > maxLinkLabel {
		label, cut = string(r[:maxLinkLabel]), true
	}
	if cut {
		label += "…"
	}
	return label
}

type layoutData struct {
	Lang, Subject   string
	Brand           brand
	Content, Footer htmltemplate.HTML
}

func Render(name, locale string, data any) (Message, error) {
	return RenderAs(Brand{}, name, locale, data)
}

// RenderAs renders a mail's subject and text, and its HTML part when the template has one, in the
// locale's language (else English) under brand b.
func RenderAs(b Brand, name, locale string, data any) (Message, error) {
	lang := locale
	t, ok := templates[lang]
	if !ok {
		base, _, _ := strings.Cut(strings.ReplaceAll(locale, "_", "-"), "-")
		lang = strings.ToLower(base)
		if t, ok = templates[lang]; !ok {
			lang, t = "en", templates["en"]
		}
	}
	if t.Lookup(name+".txt") == nil {
		lang, t = "en", templates["en"]
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name+".txt", data); err != nil {
		return Message{}, err
	}
	subject, body, _ := strings.Cut(buf.String(), "\n\n")
	m := Message{Subject: strings.TrimSpace(subject), Text: strings.TrimLeft(body, "\n")}
	h := htmlTemplates[lang]
	if h.Lookup(name+".html") == nil {
		return m, nil
	}
	h, err := h.Clone()
	if err != nil {
		return Message{}, err
	}
	br := resolveBrand(b)
	h.Funcs(layoutFuncs(br))
	ld := layoutData{Lang: lang, Subject: m.Subject, Brand: br}
	buf.Reset()
	if err := h.ExecuteTemplate(&buf, name+".html", data); err != nil {
		return Message{}, err
	}
	ld.Content = htmltemplate.HTML(buf.String())
	if h.Lookup(name+".footer") != nil {
		buf.Reset()
		if err := h.ExecuteTemplate(&buf, name+".footer", data); err != nil {
			return Message{}, err
		}
		ld.Footer = htmltemplate.HTML(strings.TrimSpace(buf.String()))
	}
	buf.Reset()
	if err := h.ExecuteTemplate(&buf, "layout", ld); err != nil {
		return Message{}, err
	}
	m.HTML = buf.String()
	return m, nil
}

type Log struct{ Log *slog.Logger }

func (l Log) Send(ctx context.Context, m Message) error {
	l.Log.InfoContext(ctx, "mail (log mailer, not sent)",
		slog.String("to", m.To), slog.String("subject", m.Subject), slog.String("text", m.Text), slog.Int("html_bytes", len(m.HTML)))
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

func (s SMTP) Send(ctx context.Context, m Message) (err error) {
	ctx, span := telemetry.Start(ctx, "smtp.send", attribute.Int("yuva.recipients", 1), attribute.Int("yuva.message_bytes", len(m.Text)+len(m.HTML)))
	defer func() { telemetry.End(span, err) }()
	if err := checkHeaders(m); err != nil {
		return err
	}
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
	reply, err := email.Data(c, compose(from, m))
	if err != nil {
		return err
	}
	if relayID := email.RelayMessageID(s.Config.Host, email.DataReplyID(reply)); relayID != "" && m.OnRelayed != nil {
		m.OnRelayed(ctx, relayID)
	}
	// The relay accepted the mail with its reply to DATA; a failing QUIT does not unsend it.
	_ = c.Quit()
	return nil
}

func checkHeaders(m Message) error {
	for name, v := range map[string]string{"To": m.To, "List-Unsubscribe": m.Unsubscribe, "Message-ID": m.MessageID} {
		if strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			return fmt.Errorf("mail: %s has a control character", name)
		}
	}
	return nil
}

func compose(from *mail.Address, m Message) []byte {
	var b bytes.Buffer
	domain := "yuva.invalid"
	if _, d, ok := strings.Cut(from.Address, "@"); ok {
		domain = d
	}
	msgID := m.MessageID
	if msgID == "" {
		id := make([]byte, 16)
		_, _ = rand.Read(id)
		msgID = hex.EncodeToString(id) + "@" + domain
	}
	header := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	header("From", from.String())
	header("To", m.To)
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", time.Now().UTC().Format(time.RFC1123Z))
	header("Message-ID", "<"+msgID+">")
	header("Auto-Submitted", "auto-generated")
	if m.Unsubscribe != "" {
		header("List-Unsubscribe", "<"+m.Unsubscribe+">")
		header("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}
	header("MIME-Version", "1.0")
	if m.HTML == "" {
		header("Content-Type", "text/plain; charset=utf-8")
		header("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		writeQP(&b, m.Text)
		return b.Bytes()
	}
	mw := multipart.NewWriter(&b)
	header("Content-Type", `multipart/alternative; boundary="`+mw.Boundary()+`"`)
	b.WriteString("\r\n")
	for _, part := range []struct{ kind, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		w, _ := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.kind + "; charset=utf-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		var pb bytes.Buffer
		writeQP(&pb, part.body)
		_, _ = w.Write(pb.Bytes())
	}
	_ = mw.Close()
	return b.Bytes()
}

func writeQP(b *bytes.Buffer, s string) {
	qp := quotedprintable.NewWriter(b)
	_, _ = qp.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")))
	_ = qp.Close()
}

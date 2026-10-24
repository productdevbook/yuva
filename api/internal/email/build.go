package email

import (
	"bytes"
	"html"
	"net/mail"
	"strings"
	"time"

	"github.com/jhillyerd/enmime/v2"
)

type Outgoing struct {
	From          Address
	Sender        Address
	ReplyTo       Address
	To            Address
	AlsoTo        []Address
	Cc            []Address
	Subject       string
	MessageID     string
	InReplyTo     string
	References    []string
	Text          string
	HTML          string
	AutoSubmitted bool
	Attachments   []Attachment
	Date          time.Time
	Headers       []Header
}

type Header struct{ Name, Value string }

const maxReferences = 20

// TrimReferences keeps the thread root and the most recent ids, as RFC 5322 suggests when the
// list grows long.
func TrimReferences(refs []string) []string {
	if len(refs) <= maxReferences {
		return refs
	}
	out := append([]string{refs[0]}, refs[len(refs)-(maxReferences-1):]...)
	return out
}

func TextToHTML(text string) string {
	var b strings.Builder
	b.WriteString(`<div dir="auto">`)
	for i, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if i > 0 {
			b.WriteString("<br>\n")
		}
		b.WriteString(html.EscapeString(line))
	}
	b.WriteString("</div>")
	return b.String()
}

// FormatAddress writes an address the way normalizing relays such as Amazon SES rewrite it, so a
// DKIM signature over it survives them: a bare address without angle brackets, and a display name
// quoted only when it is not a plain phrase. Non-ASCII names are encoded as net/mail does.
func FormatAddress(a Address) string {
	full := (&mail.Address{Name: a.Name, Address: a.Email}).String()
	if a.Name == "" {
		return strings.TrimSuffix(strings.TrimPrefix(full, "<"), ">")
	}
	if !plainPhrase(a.Name) {
		return full
	}
	return a.Name + " " + full[strings.LastIndex(full, " <")+1:]
}

func plainPhrase(s string) bool {
	if s == "" || strings.HasPrefix(s, "=?") {
		return false
	}
	for _, word := range strings.Split(s, " ") {
		if word == "" {
			return false
		}
		for _, r := range word {
			if !atext(r) {
				return false
			}
		}
	}
	return true
}

func atext(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("!#$%&'*+-/=?^_`{|}~", r)
}

func Build(o Outgoing) ([]byte, error) {
	date := o.Date
	if date.IsZero() {
		date = time.Now()
	}
	text := o.Text
	htmlBody := o.HTML
	if htmlBody == "" {
		htmlBody = TextToHTML(text)
	}
	b := enmime.Builder().
		From(o.From.Name, o.From.Email).
		To(o.To.Name, o.To.Email).
		Subject(o.Subject).
		Date(date).
		Header("Message-ID", FormatID(o.MessageID)).
		Text([]byte(text)).
		HTML([]byte(htmlBody))
	if o.ReplyTo.Email != "" {
		b = b.ReplyTo(o.ReplyTo.Name, o.ReplyTo.Email)
	}
	if o.InReplyTo != "" {
		b = b.Header("In-Reply-To", FormatID(o.InReplyTo))
	}
	if refs := TrimReferences(o.References); len(refs) > 0 {
		formatted := make([]string, len(refs))
		for i, r := range refs {
			formatted[i] = FormatID(r)
		}
		b = b.Header("References", strings.Join(formatted, " "))
	}
	if o.AutoSubmitted {
		b = b.Header("Auto-Submitted", "auto-replied").Header("X-Auto-Response-Suppress", "All")
	}
	for _, h := range o.Headers {
		b = b.Header(h.Name, h.Value)
	}
	for _, a := range o.Attachments {
		b = b.AddAttachment(a.Data, a.ContentType, a.Filename)
	}
	root, err := b.Build()
	if err != nil {
		return nil, err
	}
	root.Header.Set("From", FormatAddress(o.From))
	to := []string{FormatAddress(o.To)}
	for _, a := range o.AlsoTo {
		to = append(to, FormatAddress(a))
	}
	root.Header.Set("To", strings.Join(to, ", "))
	if len(o.Cc) > 0 {
		cc := make([]string, len(o.Cc))
		for i, c := range o.Cc {
			cc[i] = FormatAddress(c)
		}
		root.Header.Set("Cc", strings.Join(cc, ", "))
	}
	if o.Sender.Email != "" {
		root.Header.Set("Sender", FormatAddress(o.Sender))
	}
	if o.ReplyTo.Email != "" {
		root.Header.Set("Reply-To", FormatAddress(o.ReplyTo))
	}
	var buf bytes.Buffer
	if err := root.Encode(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

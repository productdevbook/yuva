package email

import (
	"bytes"
	"html"
	"strings"
	"time"

	"github.com/jhillyerd/enmime/v2"
)

type Outgoing struct {
	From          Address
	ReplyTo       Address
	To            Address
	Subject       string
	MessageID     string
	InReplyTo     string
	References    []string
	Text          string
	HTML          string
	AutoSubmitted bool
	Attachments   []Attachment
	Date          time.Time
}

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
	for _, a := range o.Attachments {
		b = b.AddAttachment(a.Data, a.ContentType, a.Filename)
	}
	root, err := b.Build()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := root.Encode(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

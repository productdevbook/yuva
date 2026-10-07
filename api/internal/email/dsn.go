package email

import (
	"bufio"
	"bytes"
	"mime"
	"net/mail"
	"net/textproto"
	"strings"

	"github.com/jhillyerd/enmime/v2"
)

type BounceRecipient struct {
	Email      string
	Action     string
	Status     string
	Diagnostic string
}

func (r BounceRecipient) Failed() bool { return strings.EqualFold(r.Action, "failed") }

func (r BounceRecipient) Permanent() bool { return r.Failed() && strings.HasPrefix(r.Status, "5") }

type Bounce struct {
	Recipients        []BounceRecipient
	OriginalMessageID string
}

func (b *Bounce) Failed() []BounceRecipient {
	var out []BounceRecipient
	for _, r := range b.Recipients {
		if r.Failed() {
			out = append(out, r)
		}
	}
	return out
}

func parseBounce(root *enmime.Part) *Bounce {
	report := root.DepthMatchFirst(func(p *enmime.Part) bool {
		if p.ContentType != "multipart/report" {
			return false
		}
		_, params, err := mime.ParseMediaType(p.Header.Get("Content-Type"))
		return err == nil && strings.EqualFold(params["report-type"], "delivery-status")
	})
	if report == nil {
		return nil
	}
	b := &Bounce{}
	for p := report.FirstChild; p != nil; p = p.NextSibling {
		switch p.ContentType {
		case "message/delivery-status", "message/global-delivery-status":
			b.Recipients = deliveryStatus(p.Content)
		case "message/rfc822", "text/rfc822-headers", "message/rfc822-headers", "message/global", "message/global-headers":
			b.OriginalMessageID = originalMessageID(p)
		}
	}
	return b
}

func deliveryStatus(data []byte) []BounceRecipient {
	var out []BounceRecipient
	r := textproto.NewReader(bufio.NewReader(bytes.NewReader(append(bytes.TrimLeft(data, "\r\n"), "\r\n\r\n"...))))
	for {
		h, err := r.ReadMIMEHeader()
		if rcpt := recipientAddress(h); rcpt != "" {
			out = append(out, BounceRecipient{
				Email:      rcpt,
				Action:     strings.ToLower(strings.TrimSpace(h.Get("Action"))),
				Status:     strings.TrimSpace(h.Get("Status")),
				Diagnostic: diagnostic(h.Get("Diagnostic-Code")),
			})
		}
		if err != nil {
			return out
		}
	}
}

func recipientAddress(h textproto.MIMEHeader) string {
	v := h.Get("Final-Recipient")
	if v == "" {
		v = h.Get("Original-Recipient")
	}
	if _, addr, ok := strings.Cut(v, ";"); ok {
		v = addr
	}
	return strings.ToLower(strings.Trim(strings.TrimSpace(v), "<>"))
}

func diagnostic(v string) string {
	if _, rest, ok := strings.Cut(v, ";"); ok {
		v = rest
	}
	return strings.Join(strings.Fields(v), " ")
}

func originalMessageID(p *enmime.Part) string {
	data := p.Content
	if p.FirstChild != nil && len(data) == 0 {
		for k, v := range p.FirstChild.Header {
			if strings.EqualFold(k, "Message-Id") && len(v) > 0 {
				return NormalizeID(v[0])
			}
		}
	}
	if !bytes.Contains(data, []byte("\n\n")) && !bytes.Contains(data, []byte("\r\n\r\n")) {
		data = append(append([]byte{}, data...), "\r\n\r\n"...)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	return NormalizeID(msg.Header.Get("Message-Id"))
}

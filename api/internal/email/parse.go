package email

import (
	"bytes"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/jhillyerd/enmime/v2"
)

type Address struct {
	Name  string
	Email string
}

type Attachment struct {
	Filename    string
	ContentType string
	ContentID   string
	Inline      bool
	Data        []byte
}

type Message struct {
	MessageID   string
	InReplyTo   []string
	References  []string
	From        Address
	To          []Address
	Cc          []Address
	Subject     string
	Date        time.Time
	Text        string
	HTML        string
	Attachments []Attachment
	Auto        bool
	AuthResults string
	Headers     map[string]string
	Bounce      *Bounce
}

const (
	DMARCPass    = "pass"
	DMARCFail    = "fail"
	DMARCNone    = "none"
	DMARCUnknown = "unknown"
)

var keptHeaders = []string{
	"Date", "Reply-To", "Return-Path", "Auto-Submitted", "Precedence", "X-Autoreply", "X-Autorespond",
	"X-Auto-Response-Suppress", "List-Id", "X-Mailer", "User-Agent",
}

var parser = enmime.NewParser(enmime.MaxMIMEParts(1000), enmime.SkipMalformedParts(true))

func Parse(raw []byte) (*Message, error) {
	env, err := parser.ReadEnvelope(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	m := &Message{
		MessageID:  NormalizeID(env.GetHeader("Message-ID")),
		InReplyTo:  boundedIDs(env.GetHeader("In-Reply-To"), maxInReplyTo, 0),
		References: boundedIDs(env.GetHeader("References"), 1, maxInboundRefs-1),
		Subject:    strings.TrimSpace(env.GetHeader("Subject")),
		Text:       env.Text,
		HTML:       env.HTML,
		Headers:    map[string]string{},
	}
	if from, err := env.AddressList("From"); err == nil && len(from) > 0 {
		m.From = Address{Name: strings.TrimSpace(from[0].Name), Email: strings.ToLower(strings.TrimSpace(from[0].Address))}
	}
	m.To = addresses(env, "To")
	m.Cc = addresses(env, "Cc")
	if d, err := env.Date(); err == nil {
		m.Date = d
	}
	for _, h := range keptHeaders {
		if v := strings.TrimSpace(env.GetHeader(h)); v != "" {
			m.Headers[h] = v
		}
	}
	if ar := env.GetHeaderValues("Authentication-Results"); len(ar) > 0 {
		m.AuthResults = strings.TrimSpace(ar[0])
	}
	m.Auto = Automatic(m.Headers, m.From.Email)
	m.Attachments = attachments(env)
	if env.Root != nil {
		m.Bounce = parseBounce(env.Root)
	}
	if m.Bounce != nil {
		m.Auto = true
	}
	return m, nil
}

func addresses(env *enmime.Envelope, key string) []Address {
	list, err := env.AddressList(key)
	if err != nil {
		return nil
	}
	out := make([]Address, 0, len(list))
	for _, a := range list {
		out = append(out, Address{Name: strings.TrimSpace(a.Name), Email: strings.ToLower(strings.TrimSpace(a.Address))})
	}
	return out
}

func attachments(env *enmime.Envelope) []Attachment {
	var out []Attachment
	seen := map[*enmime.Part]bool{}
	add := func(p *enmime.Part, inline bool) {
		if seen[p] || len(p.Content) == 0 {
			return
		}
		seen[p] = true
		ct := strings.ToLower(p.ContentType)
		if p.FileName == "" && (ct == "text/plain" || ct == "text/html" || strings.HasPrefix(ct, "message/") || strings.HasPrefix(ct, "text/rfc822")) {
			return
		}
		out = append(out, Attachment{
			Filename: p.FileName, ContentType: ct, ContentID: strings.Trim(p.ContentID, "<> "), Inline: inline, Data: p.Content,
		})
	}
	for _, p := range env.Attachments {
		add(p, false)
	}
	for _, p := range env.Inlines {
		add(p, true)
	}
	for _, p := range env.OtherParts {
		add(p, p.ContentID != "")
	}
	return out
}

// TrustedDMARC is the DMARC verdict of the topmost Authentication-Results only when its authserv-id
// is authservID, the receiving server Yuva trusts: a header with any other id may have come with the
// message itself.
func TrustedDMARC(authResults, authservID string) string {
	if authservID == "" || !strings.EqualFold(AuthservID(authResults), authservID) {
		return DMARCUnknown
	}
	return DMARCResult(authResults)
}

// AuthservID is the authserv-id that starts an Authentication-Results value (RFC 8601).
func AuthservID(authResults string) string {
	head, _, _ := strings.Cut(authResults, ";")
	if f := strings.Fields(head); len(f) > 0 {
		return f[0]
	}
	return ""
}

var dmarcResult = regexp.MustCompile(`(?i)\bdmarc\s*=\s*([a-z]+)`)

func DMARCResult(authResults string) string {
	m := dmarcResult.FindStringSubmatch(authResults)
	if m == nil {
		return DMARCUnknown
	}
	switch strings.ToLower(m[1]) {
	case "pass":
		return DMARCPass
	case "fail":
		return DMARCFail
	case "none":
		return DMARCNone
	}
	return DMARCUnknown
}

func headerToken(v string) string {
	v, _, _ = strings.Cut(v, ";")
	return strings.ToLower(strings.TrimSpace(v))
}

// Automatic reports mail that must never trigger an automatic message: auto-replies, bulk and
// list mail, and reports from mail systems.
func Automatic(h map[string]string, from string) bool {
	if v, ok := h["Auto-Submitted"]; ok && headerToken(v) != "no" {
		return true
	}
	switch headerToken(h["Precedence"]) {
	case "bulk", "junk", "list", "auto_reply":
		return true
	}
	if h["X-Autoreply"] != "" || h["X-Autorespond"] != "" || h["List-Id"] != "" {
		return true
	}
	if v, ok := h["X-Auto-Response-Suppress"]; ok && headerToken(v) != "none" {
		return true
	}
	if rp, ok := h["Return-Path"]; ok && strings.TrimSpace(rp) == "<>" {
		return true
	}
	local, _, _ := strings.Cut(from, "@")
	switch local {
	case "mailer-daemon", "postmaster":
		return true
	}
	return false
}

func NormalizeID(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, '<'); i >= 0 {
		if j := strings.IndexByte(v[i:], '>'); j > 0 {
			v = v[i+1 : i+j]
		}
	}
	return strings.TrimSpace(v)
}

var bracketed = regexp.MustCompile(`<([^<>\s]+)>`)

func ParseIDList(v string) []string {
	var out []string
	matches := bracketed.FindAllStringSubmatch(v, -1)
	if len(matches) == 0 {
		for _, f := range strings.Fields(v) {
			if strings.Contains(f, "@") {
				out = append(out, strings.Trim(f, "<>,"))
			}
		}
		return out
	}
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

const (
	maxInboundRefs = 50
	maxInReplyTo   = 5
	maxIDBytes     = 998
	idHeaderScan   = 64 << 10
)

// boundedIDs reads at most the first and last idHeaderScan bytes of an id list header and keeps
// its first `first` and last `last` ids of at most 998 bytes: a thread needs its root and its
// recent parents, and a header of millions of ids would cost seconds and gigabytes.
func boundedIDs(v string, first, last int) []string {
	if len(v) > 2*idHeaderScan {
		v = v[:idHeaderScan] + " " + v[len(v)-idHeaderScan:]
	}
	var ids []string
	for _, id := range ParseIDList(v) {
		if len(id) <= maxIDBytes {
			ids = append(ids, id)
		}
	}
	if len(ids) <= first+last {
		return ids
	}
	return append(ids[:first:first], ids[len(ids)-last:]...)
}

func FormatID(id string) string { return "<" + id + ">" }

func ParseAddress(s string) (Address, bool) {
	a, err := mail.ParseAddress(strings.TrimSpace(s))
	if err != nil {
		return Address{}, false
	}
	return Address{Name: a.Name, Email: strings.ToLower(a.Address)}, true
}

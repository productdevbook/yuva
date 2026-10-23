package email

import (
	"bytes"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/inbucket/html2text"
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
	Feedback    *Feedback

	AllAuthResults []string
	DKIMSignatures []string
	Probe          string
	MigrationProbe string
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

var parser = enmime.NewParser(enmime.MaxMIMEParts(1000), enmime.SkipMalformedParts(true), enmime.DisableTextConversion(true))

// maxTextFromHTMLBytes bounds the HTML turned into text for a message without a text part:
// converting costs about a quarter of a second per megabyte.
const maxTextFromHTMLBytes = 1 << 20

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
	if m.Text == "" && m.HTML != "" {
		h := m.HTML
		if len(h) > maxTextFromHTMLBytes {
			h = h[:maxTextFromHTMLBytes]
		}
		m.Text, _ = html2text.FromString(h)
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
	for _, ar := range env.GetHeaderValues("Authentication-Results") {
		m.AllAuthResults = append(m.AllAuthResults, strings.TrimSpace(ar))
	}
	if len(m.AllAuthResults) > 0 {
		m.AuthResults = m.AllAuthResults[0]
	}
	m.DKIMSignatures = env.GetHeaderValues("DKIM-Signature")
	m.Probe = strings.TrimSpace(env.GetHeader(ProbeHeader))
	m.MigrationProbe = strings.TrimSpace(env.GetHeader(MigrationProbeHeader))
	m.Auto = Automatic(m.Headers, m.From.Email)
	m.Attachments = attachments(env)
	if env.Root != nil {
		m.Bounce = parseBounce(env.Root)
		m.Feedback = parseFeedback(env.Root)
	}
	if m.Bounce != nil || m.Feedback != nil {
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

// HeaderSender reads only the header of raw: the From address and the Authentication-Results
// values in order, so that limits apply before the body is parsed.
func HeaderSender(raw []byte) (from string, authResults []string) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return "", nil
	}
	if list, err := msg.Header.AddressList("From"); err == nil && len(list) > 0 {
		from = strings.ToLower(strings.TrimSpace(list[0].Address))
	}
	for _, v := range msg.Header["Authentication-Results"] {
		authResults = append(authResults, strings.TrimSpace(v))
	}
	return from, authResults
}

// TrustedDMARC is the DMARC verdict of the topmost Authentication-Results only when its authserv-id
// is authservID; when several carry that id a sender added one, so only a fail among them counts.
func TrustedDMARC(authResults []string, authservID string) string {
	trusted, ok := trustedAuthResults(authResults, authservID)
	if !ok {
		return DMARCUnknown
	}
	if len(trusted) == 1 {
		return DMARCResult(trusted[0])
	}
	for _, ar := range trusted {
		if DMARCResult(ar) == DMARCFail {
			return DMARCFail
		}
	}
	return DMARCUnknown
}

func trustedAuthResults(authResults []string, authservID string) ([]string, bool) {
	if authservID == "" || len(authResults) == 0 || !strings.EqualFold(AuthservID(authResults[0]), authservID) {
		return nil, false
	}
	var out []string
	for _, ar := range authResults {
		if strings.EqualFold(AuthservID(ar), authservID) {
			out = append(out, ar)
		}
	}
	return out, true
}

// AuthservID is the authserv-id that starts an Authentication-Results value (RFC 8601).
func AuthservID(authResults string) string {
	parts, _ := authResInfo(authResults)
	if f := strings.Fields(parts[0]); len(f) > 0 {
		return f[0]
	}
	return ""
}

// authResInfo splits an Authentication-Results value at the semicolons between its resinfo, drops
// comments and turns each quoted string into a lone '"', so nothing written inside one can read as a
// method or a result. ok is false when a comment or a quoted string is left open.
func authResInfo(v string) (parts []string, ok bool) {
	var b strings.Builder
	depth, quoted := 0, false
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case quoted:
			if c == '\\' {
				i++
			} else if c == '"' {
				quoted = false
			}
		case depth > 0:
			switch c {
			case '\\':
				i++
			case '(':
				depth++
			case ')':
				depth--
			}
		case c == '"':
			quoted = true
			b.WriteString(` " `)
		case c == '(':
			depth = 1
			b.WriteByte(' ')
		case c == ';':
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}
	return append(parts, b.String()), !quoted && depth == 0
}

type authResult struct{ method, result string }

var methodSpec = regexp.MustCompile(`^\s*([A-Za-z0-9][A-Za-z0-9-]*)\s*(?:/\s*[0-9]+\s*)?=\s*([A-Za-z0-9][A-Za-z0-9-]*)(?:\s|$)`)

// parseAuthResults reads the method=result that starts each resinfo (RFC 8601 §2.2). Reasons and
// properties such as smtp.mailfrom or header.i carry values the sender chose and are never read.
func parseAuthResults(v string) ([]authResult, bool) {
	parts, ok := authResInfo(v)
	if !ok || len(strings.Fields(parts[0])) == 0 {
		return nil, false
	}
	var out []authResult
	for _, p := range parts[1:] {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "", "none":
			continue
		}
		m := methodSpec.FindStringSubmatch(p)
		if m == nil {
			return nil, false
		}
		out = append(out, authResult{strings.ToLower(m[1]), strings.ToLower(m[2])})
	}
	return out, true
}

// DMARCResult is the dmarc result of one Authentication-Results value. A value that does not parse
// is unknown; when it carries several dmarc results, a fail among them wins and any disagreement
// otherwise is unknown.
func DMARCResult(authResults string) string {
	results, ok := parseAuthResults(authResults)
	if !ok {
		return DMARCUnknown
	}
	verdict := ""
	for _, r := range results {
		if r.method != "dmarc" {
			continue
		}
		switch {
		case r.result == "fail":
			return DMARCFail
		case verdict == "":
			verdict = r.result
		case verdict != r.result:
			verdict = "mixed"
		}
	}
	switch verdict {
	case "pass":
		return DMARCPass
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

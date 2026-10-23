package email

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"regexp"
	"strings"
)

var lowerBase32 = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

func randomToken(bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return lowerBase32.EncodeToString(b)
}

// NewConversationToken is the opaque per-conversation part of our outbound Message-IDs.
func NewConversationToken() string { return randomToken(15) }

func NewMessageID(token, domain string) string {
	return token + "." + randomToken(10) + "@" + strings.ToLower(domain)
}

var ourID = regexp.MustCompile(`^([a-z2-7]{24})\.([a-z2-7]{16})@(.+)$`)

// TokenFromID returns the conversation token of a Message-ID we generated. Case is folded because
// some relays upper-case or otherwise rewrite the header.
func TokenFromID(id string) (token, domain string, ok bool) {
	m := ourID.FindStringSubmatch(strings.ToLower(NormalizeID(id)))
	if m == nil {
		return "", "", false
	}
	return m[1], m[3], true
}

func Domain(addr string) string {
	_, d, _ := strings.Cut(addr, "@")
	return strings.ToLower(d)
}

var replyPrefix = regexp.MustCompile(`(?i)^\s*((re|aw|sv|ynt|cvp|antw|rv|res|odp)\s*(\[\d+\])?\s*:\s*)+`)

func ReplySubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
	}
	return "Re: " + strings.TrimSpace(replyPrefix.ReplaceAllString(subject, ""))
}

var forwardPrefix = regexp.MustCompile(`(?i)^\s*((fwd?|fw|wg|tr|ilt)\s*(\[\d+\])?\s*:\s*)+`)

func ForwardSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	subject = strings.TrimSpace(forwardPrefix.ReplaceAllString(replyPrefix.ReplaceAllString(subject, ""), ""))
	if subject == "" {
		return ""
	}
	return "Fwd: " + subject
}

// SynthesizedID is the Message-ID stored for a message that has none: a hash of the message.
func SynthesizedID(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256-" + hex.EncodeToString(sum[:20]) + "@yuva.invalid"
}

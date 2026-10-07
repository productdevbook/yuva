// Package identity signs the identity tokens a host backend gives its signed-in users, so the Yuva
// widget and mobile SDKs can open a contact session as that user.
//
// A token is a JWT signed with HS256 under the inbox's identity secret (shown once when the inbox
// is created or its secret rotated). Yuva accepts it only until `exp`, which may be at most 10
// minutes after it is signed, so sign a fresh token for each page or app start.
package identity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	// DefaultTTL is how long a token is valid when Claims.TTL is zero.
	DefaultTTL = 5 * time.Minute
	// MaxTTL is the longest validity Yuva accepts.
	MaxTTL = 10 * time.Minute
)

// Claims describe the signed-in user.
type Claims struct {
	// Subject is the host app's user id. Required; the same id always finds the same contact in
	// the inbox.
	Subject string
	// Email is stored on the contact. Yuva links it to an existing contact with this address
	// only when EmailVerified is set.
	Email string
	// EmailVerified says the host app has verified that the user owns Email. Leave it false
	// unless the address was confirmed; an unverified address is kept apart and never used to
	// find a contact.
	EmailVerified bool
	Name          string
	// Locale is a language tag such as "en" or "tr".
	Locale string
	// Attrs are shown to members next to the conversation (plan, app version, …); at most 16 KiB
	// as JSON.
	Attrs map[string]any
	// IssuedAt defaults to the current time.
	IssuedAt time.Time
	// TTL defaults to DefaultTTL and may be at most MaxTTL.
	TTL time.Duration
}

type payload struct {
	Sub           string         `json:"sub"`
	Email         string         `json:"email,omitempty"`
	EmailVerified bool           `json:"email_verified,omitempty"`
	Name          string         `json:"name,omitempty"`
	Locale        string         `json:"locale,omitempty"`
	Attrs         map[string]any `json:"attrs,omitempty"`
	Iat           int64          `json:"iat"`
	Exp           int64          `json:"exp"`
}

var header = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// Sign returns an identity token for c, signed with the inbox identity secret.
func Sign(secret string, c Claims) (string, error) {
	if secret == "" {
		return "", errors.New("identity: the inbox identity secret is empty")
	}
	sub := strings.TrimSpace(c.Subject)
	if sub == "" || len([]rune(sub)) > 200 {
		return "", errors.New("identity: Subject must be 1 to 200 characters")
	}
	ttl := c.TTL
	if ttl == 0 {
		ttl = DefaultTTL
	}
	if ttl < 0 || ttl > MaxTTL {
		return "", errors.New("identity: TTL must be at most 10 minutes")
	}
	iat := c.IssuedAt
	if iat.IsZero() {
		iat = time.Now()
	}
	body, err := json.Marshal(payload{
		Sub: sub, Email: c.Email, EmailVerified: c.EmailVerified, Name: c.Name, Locale: c.Locale, Attrs: c.Attrs,
		Iat: iat.Unix(), Exp: iat.Add(ttl).Unix(),
	})
	if err != nil {
		return "", err
	}
	signing := header + "." + base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signing))
	return signing + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

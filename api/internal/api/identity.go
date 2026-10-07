package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/productdevbook/yuva/api/internal/oas"
)

const (
	identityMaxAhead = 10 * time.Minute
	identityLeeway   = 30 * time.Second
	maxIdentityToken = 8192
)

func errIdentity(detail string) *apiError {
	return problem(http.StatusUnauthorized, "invalid_identity_token", detail)
}

type identityClaims struct {
	sub           string
	email         string
	emailVerified bool
	name          *string
	locale        *string
	attrs         []byte
	jti           string
	exp           time.Time
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

func decodeSegment(s string) ([]byte, bool) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return b, err == nil
}

func numericDate(v any) (time.Time, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return time.Time{}, false
	}
	f, err := n.Float64()
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > 1e11 {
		return time.Time{}, false
	}
	sec, frac := math.Modf(f)
	return time.Unix(int64(sec), int64(frac*1e9)), true
}

// verifyIdentityToken checks a host backend's identity token: a compact JWS signed with HS256
// under the inbox identity secret, with `sub` and an `exp` at most 10 minutes ahead.
func verifyIdentityToken(token string, secret []byte, now time.Time) (identityClaims, error) {
	var c identityClaims
	if len(token) > maxIdentityToken {
		return c, errIdentity("the token is too long")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return c, errIdentity("the token is not a signed JWT")
	}
	rawHeader, ok := decodeSegment(parts[0])
	if !ok {
		return c, errIdentity("the token header is not base64url")
	}
	var h jwtHeader
	if err := json.Unmarshal(rawHeader, &h); err != nil {
		return c, errIdentity("the token header is not JSON")
	}
	if h.Alg != "HS256" {
		return c, errIdentity("the token must be signed with HS256")
	}
	if h.Typ != "" && !strings.EqualFold(h.Typ, "JWT") {
		return c, errIdentity("the token type must be JWT")
	}
	sig, ok := decodeSegment(parts[2])
	if !ok {
		return c, errIdentity("the token signature is not base64url")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return c, errIdentity("the token signature does not match the inbox identity secret")
	}
	rawClaims, ok := decodeSegment(parts[1])
	if !ok {
		return c, errIdentity("the token claims are not base64url")
	}
	dec := json.NewDecoder(bytes.NewReader(rawClaims))
	dec.UseNumber()
	var claims map[string]any
	if err := dec.Decode(&claims); err != nil || claims == nil {
		return c, errIdentity("the token claims are not a JSON object")
	}
	rawExp, present := claims["exp"]
	if !present {
		return c, errIdentity("the token has no exp")
	}
	exp, ok := numericDate(rawExp)
	if !ok {
		return c, errIdentity("exp must be a NumericDate")
	}
	if !exp.After(now.Add(-identityLeeway)) {
		return c, errIdentity("the token has expired")
	}
	if exp.After(now.Add(identityMaxAhead + identityLeeway)) {
		return c, errIdentity("exp must be at most 10 minutes ahead")
	}
	for _, k := range []string{"iat", "nbf"} {
		v, present := claims[k]
		if !present {
			continue
		}
		t, ok := numericDate(v)
		if !ok {
			return c, errIdentity(k + " must be a NumericDate")
		}
		if t.After(now.Add(identityLeeway)) {
			return c, errIdentity(k + " is in the future")
		}
	}
	c.exp = exp
	if v, present := claims["jti"]; present && v != nil {
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" || len(s) > 200 {
			return c, errIdentity("jti must be a string of 1 to 200 characters")
		}
		c.jti = s
	}
	sub, _ := claims["sub"].(string)
	if c.sub = strings.TrimSpace(sub); c.sub == "" || len([]rune(c.sub)) > 200 {
		return c, errIdentity("sub must be the host's user id, 1 to 200 characters")
	}
	if v, present := claims["email"]; present && v != nil {
		s, ok := v.(string)
		if !ok {
			return c, errIdentity("email must be a string")
		}
		if strings.TrimSpace(s) != "" {
			e, err := normalizeEmail(oas.Email(s))
			if err != nil {
				return c, errIdentity("email is not a valid address")
			}
			c.email = e
		}
	}
	if v, present := claims["email_verified"]; present && v != nil {
		b, ok := v.(bool)
		if !ok {
			return c, errIdentity("email_verified must be a boolean")
		}
		c.emailVerified = b && c.email != ""
	}
	if v, present := claims["name"]; present && v != nil {
		s, ok := v.(string)
		if !ok || len([]rune(strings.TrimSpace(s))) > 200 {
			return c, errIdentity("name must be a string of at most 200 characters")
		}
		s = strings.TrimSpace(s)
		c.name = &s
	}
	if v, present := claims["locale"]; present && v != nil {
		s, ok := v.(string)
		if !ok || len(s) > 35 || !languagePattern.MatchString(s) {
			return c, errIdentity("locale must be a language tag such as en or tr")
		}
		c.locale = &s
	}
	if v, present := claims["attrs"]; present && v != nil {
		obj, ok := v.(map[string]any)
		if !ok {
			return c, errIdentity("attrs must be an object")
		}
		c.attrs = mustJSON(obj)
		if len(c.attrs) > maxAttributesBytes {
			return c, errIdentity("attrs must be at most 16 KiB")
		}
	}
	return c, nil
}

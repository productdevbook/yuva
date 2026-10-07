package identity_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/sdk/go/identity"
)

func decode(t *testing.T, part string, v any) {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func TestSign(t *testing.T) {
	const secret = "yuva_is_test-secret"
	at := time.Unix(1_800_000_000, 0)
	token, err := identity.Sign(secret, identity.Claims{
		Subject: "user-42", Email: "success@simulator.amazonses.com", EmailVerified: true, Name: "Ada", ID: "t-1", Locale: "tr",
		Attrs: map[string]any{"plan": "pro"}, IssuedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q", token)
	}
	var h map[string]string
	decode(t, parts[0], &h)
	if h["alg"] != "HS256" || h["typ"] != "JWT" {
		t.Fatalf("header %v", h)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil)); parts[2] != want {
		t.Fatal("signature does not verify with the secret")
	}
	var c map[string]any
	decode(t, parts[1], &c)
	if c["sub"] != "user-42" || c["email"] != "success@simulator.amazonses.com" || c["name"] != "Ada" || c["locale"] != "tr" || c["email_verified"] != true || c["jti"] != "t-1" {
		t.Fatalf("claims %v", c)
	}
	if c["attrs"].(map[string]any)["plan"] != "pro" {
		t.Fatalf("attrs %v", c["attrs"])
	}
	if c["iat"].(float64) != float64(at.Unix()) || c["exp"].(float64) != float64(at.Add(identity.DefaultTTL).Unix()) {
		t.Fatalf("iat/exp %v %v", c["iat"], c["exp"])
	}
}

func TestSignDefaultsAndLimits(t *testing.T) {
	before := time.Now().Unix()
	token, err := identity.Sign("s", identity.Claims{Subject: "u"})
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	decode(t, strings.Split(token, ".")[1], &c)
	if iat := int64(c["iat"].(float64)); iat < before || int64(c["exp"].(float64)) != iat+300 {
		t.Fatalf("claims %v", c)
	}
	for _, k := range []string{"email", "email_verified", "name", "locale", "attrs", "jti"} {
		if _, ok := c[k]; ok {
			t.Fatalf("empty %s is sent: %v", k, c)
		}
	}
	for name, tc := range map[string]struct {
		secret string
		c      identity.Claims
	}{
		"no secret":    {"", identity.Claims{Subject: "u"}},
		"no subject":   {"s", identity.Claims{Subject: " "}},
		"long subject": {"s", identity.Claims{Subject: strings.Repeat("x", 201)}},
		"ttl too long": {"s", identity.Claims{Subject: "u", TTL: 11 * time.Minute}},
		"negative ttl": {"s", identity.Claims{Subject: "u", TTL: -time.Minute}},
	} {
		if _, err := identity.Sign(tc.secret, tc.c); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

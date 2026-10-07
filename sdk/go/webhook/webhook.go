// Package webhook verifies the webhooks Yuva sends to a host backend.
//
// Yuva signs every request per the Standard Webhooks specification
// (https://www.standardwebhooks.com): the `webhook-id`, `webhook-timestamp` and
// `webhook-signature` headers carry an HMAC-SHA256 over `<id>.<timestamp>.<body>` keyed with the
// endpoint's secret, which Yuva shows once as `whsec_<base64>`. Verify the raw body before parsing
// it, and use `webhook-id` to ignore a delivery you have already handled.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultTolerance is how far the timestamp may be from the current time when Verify is given a
// tolerance of zero.
const DefaultTolerance = 5 * time.Minute

const secretPrefix = "whsec_"

var (
	ErrInvalidSecret    = errors.New("webhook: the secret is not a whsec_ base64 secret")
	ErrMissingHeaders   = errors.New("webhook: webhook-id, webhook-timestamp or webhook-signature is missing")
	ErrInvalidTimestamp = errors.New("webhook: webhook-timestamp is not a unix time")
	ErrTimestampTooOld  = errors.New("webhook: webhook-timestamp is too old")
	ErrTimestampTooNew  = errors.New("webhook: webhook-timestamp is too far in the future")
	ErrInvalidSignature = errors.New("webhook: no signature matches")
)

func key(secret string) ([]byte, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(strings.TrimSpace(secret), secretPrefix))
	if err != nil || len(k) == 0 {
		return nil, ErrInvalidSecret
	}
	return k, nil
}

func sign(k []byte, id, ts string, body []byte) []byte {
	mac := hmac.New(sha256.New, k)
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(body)
	return mac.Sum(nil)
}

// Sign returns the `webhook-signature` value for a request, as Yuva computes it. It helps test a
// webhook handler.
func Sign(secret, id string, timestamp time.Time, body []byte) (string, error) {
	k, err := key(secret)
	if err != nil {
		return "", err
	}
	return "v1," + base64.StdEncoding.EncodeToString(sign(k, id, strconv.FormatInt(timestamp.Unix(), 10), body)), nil
}

// Verify checks that body and headers come from Yuva: one of the `v1` signatures in
// `webhook-signature` matches under secret, and `webhook-timestamp` is within tolerance of now
// (DefaultTolerance when tolerance is zero or less).
func Verify(secret string, headers http.Header, body []byte, tolerance time.Duration) error {
	return VerifyAt(secret, headers, body, tolerance, time.Now())
}

// VerifyAt is Verify with the current time given.
func VerifyAt(secret string, headers http.Header, body []byte, tolerance time.Duration, now time.Time) error {
	k, err := key(secret)
	if err != nil {
		return err
	}
	id, ts, sigs := headers.Get("webhook-id"), headers.Get("webhook-timestamp"), headers.Get("webhook-signature")
	if id == "" || ts == "" || sigs == "" {
		return ErrMissingHeaders
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ErrInvalidTimestamp
	}
	if tolerance <= 0 {
		tolerance = DefaultTolerance
	}
	at := time.Unix(unix, 0)
	if now.Sub(at) > tolerance {
		return ErrTimestampTooOld
	}
	if at.Sub(now) > tolerance {
		return ErrTimestampTooNew
	}
	want := sign(k, id, ts, body)
	for _, s := range strings.Fields(sigs) {
		version, value, ok := strings.Cut(s, ",")
		if !ok || version != "v1" {
			continue
		}
		got, err := base64.StdEncoding.DecodeString(value)
		if err == nil && hmac.Equal(got, want) {
			return nil
		}
	}
	return ErrInvalidSignature
}

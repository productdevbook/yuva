package email

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

const ReplayWindow = 5 * time.Minute

var (
	ErrBadSignature   = errors.New("signature does not match")
	ErrStaleTimestamp = errors.New("timestamp is outside the replay window")
)

func Sign(secret, timestamp, envelopeTo string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + envelopeTo + "."))
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifyIngress checks an /ingress/email request signed per edge/README.md.
func VerifyIngress(secret, timestamp, envelopeTo, signature string, body []byte, now time.Time) error {
	hexSig, ok := strings.CutPrefix(strings.TrimSpace(signature), "v1=")
	if !ok {
		return ErrBadSignature
	}
	got, err := hex.DecodeString(hexSig)
	if err != nil || len(got) != sha256.Size {
		return ErrBadSignature
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return ErrBadSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + envelopeTo + "."))
	mac.Write(body)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return ErrBadSignature
	}
	if d := now.Sub(time.Unix(ts, 0)); d > ReplayWindow || d < -ReplayWindow {
		return ErrStaleTimestamp
	}
	return nil
}

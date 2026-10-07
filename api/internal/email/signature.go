package email

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"strconv"
	"strings"
	"time"
)

const ReplayWindow = 5 * time.Minute

var (
	ErrBadSignature   = errors.New("signature does not match")
	ErrStaleTimestamp = errors.New("timestamp is outside the replay window")
)

// Sign signs an /ingress/email request with the current version, v2, which covers the envelope
// sender too.
func Sign(secret, timestamp, envelopeTo, envelopeFrom string, body []byte) string {
	mac := ingressMAC(secret, timestamp+"."+envelopeTo+"."+envelopeFrom+".")
	mac.Write(body)
	return "v2=" + hex.EncodeToString(mac.Sum(nil))
}

// SignV1 is the deprecated version that leaves the envelope sender unsigned.
func SignV1(secret, timestamp, envelopeTo string, body []byte) string {
	mac := ingressMAC(secret, timestamp+"."+envelopeTo+".")
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func ingressMAC(secret, prefix string) hash.Hash {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(prefix))
	return mac
}

// IngressCheck verifies an /ingress/email request (edge/README.md) while its body is read: the
// headers and the timestamp are checked before the first byte, the MAC once the body is complete.
type IngressCheck struct {
	mac  hash.Hash
	want []byte
	v1   bool
}

// StartIngress checks the signature headers. A v1 signature is refused unless acceptV1.
func StartIngress(secret, timestamp, envelopeTo, envelopeFrom, signature string, now time.Time, acceptV1 bool) (*IngressCheck, error) {
	version, hexSig, ok := strings.Cut(strings.TrimSpace(signature), "=")
	if !ok || (version != "v2" && (version != "v1" || !acceptV1)) {
		return nil, ErrBadSignature
	}
	want, err := hex.DecodeString(hexSig)
	if err != nil || len(want) != sha256.Size {
		return nil, ErrBadSignature
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || ts < 0 {
		return nil, ErrBadSignature
	}
	if d := now.Sub(time.Unix(ts, 0)); d > ReplayWindow || d < -ReplayWindow {
		return nil, ErrStaleTimestamp
	}
	c := &IngressCheck{want: want, v1: version == "v1"}
	if c.v1 {
		c.mac = ingressMAC(secret, timestamp+"."+envelopeTo+".")
	} else {
		c.mac = ingressMAC(secret, timestamp+"."+envelopeTo+"."+envelopeFrom+".")
	}
	return c, nil
}

func (c *IngressCheck) Write(p []byte) (int, error) { return c.mac.Write(p) }

// Verify compares the MAC over everything written with the signature, in constant time.
func (c *IngressCheck) Verify() error {
	if !hmac.Equal(c.want, c.mac.Sum(nil)) {
		return ErrBadSignature
	}
	return nil
}

// V1 reports a deprecated v1 signature, under which the envelope sender is not authenticated.
func (c *IngressCheck) V1() bool { return c.v1 }

// VerifyIngress checks a request whose body is already in memory.
func VerifyIngress(secret, timestamp, envelopeTo, envelopeFrom, signature string, body []byte, now time.Time, acceptV1 bool) (*IngressCheck, error) {
	c, err := StartIngress(secret, timestamp, envelopeTo, envelopeFrom, signature, now, acceptV1)
	if err != nil {
		return nil, err
	}
	c.mac.Write(body)
	return c, c.Verify()
}

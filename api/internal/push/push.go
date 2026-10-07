// Package push sends Web Push messages (RFC 8030) encrypted per RFC 8291 and signed with VAPID
// (RFC 8292).
package push

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"
)

type Keys struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

// GenerateKeys returns a new VAPID key pair, base64url without padding.
func GenerateKeys() (public, private string, err error) {
	private, public, err = webpush.GenerateVAPIDKeys()
	return public, private, err
}

func decode(v string) ([]byte, error) {
	v = strings.TrimRight(strings.TrimSpace(v), "=")
	return base64.RawURLEncoding.DecodeString(v)
}

// Check reports whether the private key is a P-256 key whose public half is the public key.
func (k Keys) Check() error {
	priv, err := decode(k.PrivateKey)
	if err != nil {
		return fmt.Errorf("private key is not base64url: %w", err)
	}
	pk, err := ecdh.P256().NewPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("private key: %w", err)
	}
	pub, err := decode(k.PublicKey)
	if err != nil {
		return fmt.Errorf("public key is not base64url: %w", err)
	}
	if !bytes.Equal(pk.PublicKey().Bytes(), pub) {
		return errors.New("the public key does not belong to the private key")
	}
	return nil
}

// CheckSubscriptionKeys validates a browser's p256dh key and auth secret.
func CheckSubscriptionKeys(p256dh, auth string) error {
	pub, err := decode(p256dh)
	if err != nil {
		return errors.New("keys.p256dh must be base64url")
	}
	if _, err := ecdh.P256().NewPublicKey(pub); err != nil {
		return errors.New("keys.p256dh must be an uncompressed P-256 public key")
	}
	secret, err := decode(auth)
	if err != nil || len(secret) != 16 {
		return errors.New("keys.auth must be 16 bytes, base64url")
	}
	return nil
}

type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

type Message struct {
	Payload []byte
	TTL     time.Duration
	Urgency string
	Topic   string
}

type Result struct {
	StatusCode int
	Err        error
	// Gone means the subscription will never work again (404 or 410) and should be deleted.
	Gone bool
	// Retry means a later attempt may succeed (429, 5xx or a network error).
	Retry bool
}

func (r Result) OK() bool { return r.Err == nil }

type Sender struct {
	Keys   Keys
	Client webpush.HTTPClient
}

// Send encrypts and posts one message. The Client decides which addresses may be reached.
func (s *Sender) Send(ctx context.Context, sub Subscription, m Message) Result {
	subscriber := strings.TrimPrefix(s.Keys.Subject, "mailto:")
	res, err := webpush.SendNotificationWithContext(ctx, m.Payload, &webpush.Subscription{
		Endpoint: sub.Endpoint, Keys: webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		HTTPClient:      s.Client,
		Subscriber:      subscriber,
		TTL:             int(m.TTL / time.Second),
		Urgency:         webpush.Urgency(m.Urgency),
		Topic:           m.Topic,
		VAPIDPublicKey:  s.Keys.PublicKey,
		VAPIDPrivateKey: s.Keys.PrivateKey,
	})
	if err != nil {
		return Result{Err: err, Retry: ctx.Err() == nil}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	out := Result{StatusCode: res.StatusCode}
	switch {
	case res.StatusCode >= 200 && res.StatusCode < 300:
		return out
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		out.Gone = true
	case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
		out.Retry = true
	}
	out.Err = fmt.Errorf("push service answered %d %s", res.StatusCode, strings.ToValidUTF8(strings.TrimSpace(string(body)), ""))
	return out
}

// Package apppush sends notifications to iOS apps through APNs (token-based authentication) and
// to Android apps through Firebase Cloud Messaging (HTTP v1 with a service account).
package apppush

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	APNsProduction = "https://api.push.apple.com"
	APNsSandbox    = "https://api.sandbox.push.apple.com"
	FCMBase        = "https://fcm.googleapis.com"
	GoogleTokenURL = "https://oauth2.googleapis.com/token"

	fcmScope       = "https://www.googleapis.com/auth/firebase.messaging"
	apnsTokenAge   = 40 * time.Minute
	tokenEarly     = 5 * time.Minute
	maxAnswerBytes = 64 << 10
)

// Notification is what both services show: the title, the body, the badge and the conversation
// the app opens, sent as yuva_conversation_id.
type Notification struct {
	Title          string
	Body           string
	Badge          int
	ConversationID string
}

// Result of one attempt. Gone: the device token is no longer valid and should be removed.
// BadToken: APNs refused the token, which is also its answer for a token of the other
// environment, so it means Gone only once the key has worked in this environment. Retry: a
// transient failure.
type Result struct {
	Gone     bool
	BadToken bool
	Retry    bool
	Err      error
}

func (r Result) OK() bool { return r.Err == nil }

type APNs struct {
	KeyID      string
	TeamID     string
	BundleID   string
	Production bool
	Key        *ecdsa.PrivateKey
}

type ServiceAccount struct {
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	Type         string `json:"type"`
	key          *rsa.PrivateKey
}

// ParseAPNsKey reads the PEM text of an APNs .p8 key: a PKCS #8 P-256 private key.
func ParseAPNsKey(text string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(text)))
	if block == nil {
		return nil, errors.New("not a PEM private key")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("not a PKCS #8 private key")
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok || ec.Curve != elliptic.P256() {
		return nil, errors.New("not a P-256 key")
	}
	return ec, nil
}

// ParseServiceAccount reads a Google service account's JSON key file.
func ParseServiceAccount(raw []byte) (*ServiceAccount, error) {
	var sa ServiceAccount
	if err := json.Unmarshal(raw, &sa); err != nil {
		return nil, errors.New("not a JSON key file")
	}
	if sa.Type != "service_account" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("not a service account key file")
	}
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return nil, errors.New("private_key is not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		if k, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
			return nil, errors.New("private_key is not an RSA key")
		}
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private_key is not an RSA key")
	}
	sa.key = rk
	return &sa, nil
}

// Client sends through APNs and FCM, keeping the APNs provider tokens and FCM access tokens it
// made for reuse. The URLs default to the real services.
type Client struct {
	HTTP           *http.Client
	APNsProduction string
	APNsSandbox    string
	FCM            string
	TokenURL       string
	Now            func() time.Time

	mu     sync.Mutex
	tokens map[string]cachedToken
}

type cachedToken struct {
	value   string
	expires time.Time
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func or(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

func (c *Client) cached(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.tokens[key]
	if !ok || !c.now().Before(t.expires) {
		return "", false
	}
	return t.value, true
}

func (c *Client) store(key, value string, expires time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokens == nil {
		c.tokens = map[string]cachedToken{}
	}
	for k, t := range c.tokens {
		if !c.now().Before(t.expires) {
			delete(c.tokens, k)
		}
	}
	c.tokens[key] = cachedToken{value: value, expires: expires}
}

func (c *Client) forget(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tokens, key)
}

func b64(v []byte) string { return base64.RawURLEncoding.EncodeToString(v) }

func signedJWT(header, claims any, sign func(digest []byte) ([]byte, error)) (string, error) {
	h, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cl, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	input := b64(h) + "." + b64(cl)
	digest := sha256.Sum256([]byte(input))
	sig, err := sign(digest[:])
	if err != nil {
		return "", err
	}
	return input + "." + b64(sig), nil
}

func keyFingerprint(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *Client) apnsToken(a APNs) (string, string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(a.Key)
	if err != nil {
		return "", "", err
	}
	key := "apns:" + keyFingerprint(a.KeyID, a.TeamID, string(der))
	if t, ok := c.cached(key); ok {
		return t, key, nil
	}
	now := c.now()
	t, err := signedJWT(map[string]string{"alg": "ES256", "kid": a.KeyID},
		map[string]any{"iss": a.TeamID, "iat": now.Unix()},
		func(digest []byte) ([]byte, error) {
			r, s, err := ecdsa.Sign(rand.Reader, a.Key, digest)
			if err != nil {
				return nil, err
			}
			out := make([]byte, 64)
			r.FillBytes(out[:32])
			s.FillBytes(out[32:])
			return out, nil
		})
	if err != nil {
		return "", "", err
	}
	c.store(key, t, now.Add(apnsTokenAge))
	return t, key, nil
}

type apnsAlert struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type apnsAPS struct {
	Alert    apnsAlert `json:"alert"`
	Sound    string    `json:"sound"`
	Badge    *int      `json:"badge,omitempty"`
	ThreadID string    `json:"thread-id,omitempty"`
}

type apnsPayload struct {
	APS            apnsAPS `json:"aps"`
	ConversationID string  `json:"yuva_conversation_id"`
}

// SendAPNs makes one attempt to deliver n to an iOS device token.
func (c *Client) SendAPNs(ctx context.Context, a APNs, deviceToken string, n Notification) Result {
	bearer, cacheKey, err := c.apnsToken(a)
	if err != nil {
		return Result{Err: err}
	}
	var badge *int
	if n.Badge > 0 {
		badge = &n.Badge
	}
	body, _ := json.Marshal(apnsPayload{
		APS:            apnsAPS{Alert: apnsAlert{Title: n.Title, Body: n.Body}, Sound: "default", Badge: badge, ThreadID: n.ConversationID},
		ConversationID: n.ConversationID,
	})
	base := or(c.APNsSandbox, APNsSandbox)
	if a.Production {
		base = or(c.APNsProduction, APNsProduction)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/3/device/"+url.PathEscape(deviceToken), bytes.NewReader(body))
	if err != nil {
		return Result{Err: err}
	}
	req.Header.Set("authorization", "bearer "+bearer)
	req.Header.Set("apns-topic", a.BundleID)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	req.Header.Set("apns-expiration", strconv.FormatInt(c.now().Add(24*time.Hour).Unix(), 10))
	if n.ConversationID != "" {
		req.Header.Set("apns-collapse-id", n.ConversationID)
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Result{Retry: true, Err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxAnswerBytes))
	if resp.StatusCode == http.StatusOK {
		return Result{}
	}
	var answer struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(raw, &answer)
	err = fmt.Errorf("APNs answered %d %s", resp.StatusCode, answer.Reason)
	switch {
	case resp.StatusCode == http.StatusGone, answer.Reason == "Unregistered":
		return Result{Gone: true, Err: err}
	case answer.Reason == "BadDeviceToken", answer.Reason == "DeviceTokenNotForTopic":
		return Result{BadToken: true, Err: err}
	case answer.Reason == "ExpiredProviderToken":
		c.forget(cacheKey)
		return Result{Retry: true, Err: err}
	case resp.StatusCode == http.StatusForbidden:
		c.forget(cacheKey)
		return Result{Err: err}
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return Result{Retry: true, Err: err}
	}
	return Result{Err: err}
}

func (c *Client) fcmToken(ctx context.Context, sa *ServiceAccount) (string, string, Result) {
	key := "fcm:" + keyFingerprint(sa.ClientEmail, sa.PrivateKeyID, sa.PrivateKey)
	if t, ok := c.cached(key); ok {
		return t, key, Result{}
	}
	tokenURL := or(c.TokenURL, GoogleTokenURL)
	now := c.now()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	if sa.PrivateKeyID != "" {
		header["kid"] = sa.PrivateKeyID
	}
	assertion, err := signedJWT(header,
		map[string]any{"iss": sa.ClientEmail, "scope": fcmScope, "aud": tokenURL, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()},
		func(digest []byte) ([]byte, error) {
			return rsa.SignPKCS1v15(rand.Reader, sa.key, crypto.SHA256, digest)
		})
	if err != nil {
		return "", "", Result{Err: err}
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", Result{Err: err}
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", Result{Retry: true, Err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxAnswerBytes))
	var answer struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	_ = json.Unmarshal(raw, &answer)
	if resp.StatusCode != http.StatusOK || answer.AccessToken == "" {
		err := fmt.Errorf("Google token endpoint answered %d %s", resp.StatusCode, answer.Error)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return "", "", Result{Retry: true, Err: err}
		}
		return "", "", Result{Err: err}
	}
	life := time.Duration(answer.ExpiresIn) * time.Second
	if life <= tokenEarly {
		life = tokenEarly + time.Minute
	}
	c.store(key, answer.AccessToken, now.Add(life-tokenEarly))
	return answer.AccessToken, key, Result{}
}

type fcmMessage struct {
	Message struct {
		Token        string            `json:"token"`
		Notification apnsAlert         `json:"notification"`
		Data         map[string]string `json:"data"`
		Android      struct {
			CollapseKey  string `json:"collapse_key,omitempty"`
			Priority     string `json:"priority"`
			Notification struct {
				Tag   string `json:"tag,omitempty"`
				Count *int   `json:"notification_count,omitempty"`
			} `json:"notification"`
		} `json:"android"`
	} `json:"message"`
}

// SendFCM makes one attempt to deliver n to an FCM registration token of projectID.
func (c *Client) SendFCM(ctx context.Context, sa *ServiceAccount, projectID, deviceToken string, n Notification) Result {
	bearer, cacheKey, r := c.fcmToken(ctx, sa)
	if r.Err != nil {
		return r
	}
	var m fcmMessage
	m.Message.Token = deviceToken
	m.Message.Notification = apnsAlert{Title: n.Title, Body: n.Body}
	m.Message.Data = map[string]string{"yuva_conversation_id": n.ConversationID}
	m.Message.Android.Priority = "HIGH"
	m.Message.Android.CollapseKey = n.ConversationID
	m.Message.Android.Notification.Tag = n.ConversationID
	if n.Badge > 0 {
		m.Message.Android.Notification.Count = &n.Badge
	}
	body, _ := json.Marshal(m)
	endpoint := or(c.FCM, FCMBase) + "/v1/projects/" + url.PathEscape(projectID) + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{Err: err}
	}
	req.Header.Set("authorization", "Bearer "+bearer)
	req.Header.Set("content-type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Result{Retry: true, Err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxAnswerBytes))
	if resp.StatusCode == http.StatusOK {
		return Result{}
	}
	var answer struct {
		Error struct {
			Status  string `json:"status"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &answer)
	code := answer.Error.Status
	for _, d := range answer.Error.Details {
		if d.ErrorCode != "" {
			code = d.ErrorCode
		}
	}
	err = fmt.Errorf("FCM answered %d %s", resp.StatusCode, code)
	switch {
	case code == "UNREGISTERED":
		return Result{Gone: true, Err: err}
	case resp.StatusCode == http.StatusUnauthorized:
		c.forget(cacheKey)
		return Result{Retry: true, Err: err}
	case resp.StatusCode == http.StatusForbidden:
		return Result{Err: err}
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return Result{Retry: true, Err: err}
	}
	return Result{Err: err}
}

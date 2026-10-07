package webhook

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The test vector of the Standard Webhooks reference libraries (libraries/go/webhook_test.go).
const (
	vectorSecret    = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	vectorID        = "msg_p5jXN8AQM9LWM0D4loKWxJek"
	vectorTimestamp = 1614265330
	vectorBody      = `{"test": 2432232314}`
	vectorSignature = "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="
)

func headers(id string, ts int64, sig string) http.Header {
	h := http.Header{}
	h.Set("webhook-id", id)
	h.Set("webhook-timestamp", fmt.Sprint(ts))
	h.Set("webhook-signature", sig)
	return h
}

func TestSignVector(t *testing.T) {
	got, err := Sign(vectorSecret, vectorID, time.Unix(vectorTimestamp, 0), []byte(vectorBody))
	if err != nil || got != vectorSignature {
		t.Fatalf("Sign = %q, %v; want %q", got, err, vectorSignature)
	}
	unprefixed, _ := Sign(strings.TrimPrefix(vectorSecret, "whsec_"), vectorID, time.Unix(vectorTimestamp, 0), []byte(vectorBody))
	if unprefixed != vectorSignature {
		t.Fatalf("secret without prefix: %q", unprefixed)
	}
}

func TestVerify(t *testing.T) {
	at := time.Unix(vectorTimestamp, 0)
	body := []byte(vectorBody)
	valid := headers(vectorID, vectorTimestamp, vectorSignature)
	if err := VerifyAt(vectorSecret, valid, body, 0, at); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if err := VerifyAt(vectorSecret, valid, body, 0, at.Add(DefaultTolerance)); err != nil {
		t.Fatalf("at the edge of the tolerance: %v", err)
	}
	cases := []struct {
		name string
		h    http.Header
		body string
		now  time.Time
		tol  time.Duration
		want error
	}{
		{"missing id", headers("", vectorTimestamp, vectorSignature), vectorBody, at, 0, ErrMissingHeaders},
		{"missing timestamp", func() http.Header { h := valid.Clone(); h.Del("webhook-timestamp"); return h }(), vectorBody, at, 0, ErrMissingHeaders},
		{"missing signature", headers(vectorID, vectorTimestamp, ""), vectorBody, at, 0, ErrMissingHeaders},
		{"bad timestamp", func() http.Header { h := valid.Clone(); h.Set("webhook-timestamp", "soon"); return h }(), vectorBody, at, 0, ErrInvalidTimestamp},
		{"too old", valid, vectorBody, at.Add(DefaultTolerance + time.Second), 0, ErrTimestampTooOld},
		{"too new", valid, vectorBody, at.Add(-DefaultTolerance - time.Second), 0, ErrTimestampTooNew},
		{"too old for a short tolerance", valid, vectorBody, at.Add(time.Minute + time.Second), time.Minute, ErrTimestampTooOld},
		{"wrong signature", headers(vectorID, vectorTimestamp, "v1,Ceo5qEr07ixe2NLpvHk3FH9bwy/WavXrAFQ/9tdO6mc="), vectorBody, at, 0, ErrInvalidSignature},
		{"partial signature", headers(vectorID, vectorTimestamp, "v1,"), vectorBody, at, 0, ErrInvalidSignature},
		{"other version", headers(vectorID, vectorTimestamp, strings.Replace(vectorSignature, "v1,", "v2,", 1)), vectorBody, at, 0, ErrInvalidSignature},
		{"tampered body", valid, `{"test": 2432232315}`, at, 0, ErrInvalidSignature},
		{"other id", headers("msg_other", vectorTimestamp, vectorSignature), vectorBody, at, 0, ErrInvalidSignature},
	}
	for _, c := range cases {
		if err := VerifyAt(vectorSecret, c.h, []byte(c.body), c.tol, c.now); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	multi := headers(vectorID, vectorTimestamp, strings.Join([]string{
		"v1,Ceo5qEr07ixe2NLpvHk3FH9bwy/WavXrAFQ/9tdO6mc=", "v1a,abc", vectorSignature, "v1,Ceo5qEr07ixe2NLpvHk3FH9bwy/WavXrAFQ/9tdO6mc=",
	}, " "))
	if err := VerifyAt(vectorSecret, multi, body, 0, at); err != nil {
		t.Fatalf("several signatures: %v", err)
	}
	if err := VerifyAt("whsec_", valid, body, 0, at); !errors.Is(err, ErrInvalidSecret) {
		t.Fatalf("empty secret: %v", err)
	}
	if err := VerifyAt("not base64!", valid, body, 0, at); !errors.Is(err, ErrInvalidSecret) {
		t.Fatalf("bad secret: %v", err)
	}
}

func TestVerifyNow(t *testing.T) {
	now := time.Now()
	body := []byte(`{"type":"message.created"}`)
	sig, err := Sign(vectorSecret, "msg_now", now, body)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(vectorSecret, headers("msg_now", now.Unix(), sig), body, 0); err != nil {
		t.Fatal(err)
	}
}

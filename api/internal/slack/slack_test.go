package slack

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestVerify(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	body := []byte(`{"type":"event_callback"}`)
	ts := strconv.FormatInt(now.Unix(), 10)
	sig := "v0=" + hex.EncodeToString(Sign("secret", ts, body))
	if err := Verify("secret", ts, sig, body, now); err != nil {
		t.Fatal(err)
	}
	if Verify("other", ts, sig, body, now) == nil {
		t.Fatal("another secret's signature accepted")
	}
	if Verify("secret", ts, sig, []byte(`{"type":"x"}`), now) == nil {
		t.Fatal("a changed body accepted")
	}
	if Verify("secret", ts, sig, body, now.Add(6*time.Minute)) == nil {
		t.Fatal("a replay outside the window accepted")
	}
}

func TestPlainTextAndEscape(t *testing.T) {
	got := PlainText("hi <@U1> see <https://a.example/x|this> &amp; <!here> in <#C1|general>", func(u string) string {
		if u == "U1" {
			return "Ayşe"
		}
		return ""
	})
	if want := "hi @Ayşe see this (https://a.example/x) & @here in #general"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := Escape("<!channel> & <https://x|y>"); got != "&lt;!channel&gt; &amp; &lt;https://x|y&gt;" {
		t.Fatalf("escape: %q", got)
	}
}

func TestRateLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		switch {
		case r.URL.Query().Get("long") != "" || r.FormValue("channel") == "long":
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
		case n == 1:
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			_, _ = w.Write([]byte(`{"ok":true,"ts":"1.2"}`))
		}
	}))
	defer srv.Close()
	var waited []time.Duration
	c := &Client{HTTP: srv.Client(), Base: srv.URL, Sleep: func(_ context.Context, d time.Duration) error { waited = append(waited, d); return nil }}
	ts, err := c.Post(context.Background(), "t", Message{Channel: "C1", Text: "x"})
	if err != nil || ts != "1.2" || len(waited) != 1 || waited[0] != 2*time.Second {
		t.Fatalf("short limit: %q %v %v", ts, err, waited)
	}
	_, err = c.Post(context.Background(), "t", Message{Channel: "long", Text: "x"})
	var se *Error
	if !errors.As(err, &se) || se.RetryAfter != time.Minute || len(waited) != 1 {
		t.Fatalf("long limit: %v %v", err, waited)
	}
}

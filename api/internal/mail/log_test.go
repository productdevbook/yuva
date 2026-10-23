package mail_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/mail"
)

func TestRedactAddresses(t *testing.T) {
	cases := map[string]string{
		"ayse@example.com": "*@example.com",
		"550 5.1.1 <ayse.yilmaz+x@mail.example.com.tr>: Recipient address rejected": "550 5.1.1 <*@mail.example.com.tr>: Recipient address rejected",
		"Ayşe <a@b.example>, bob@c.example":                                         "Ayşe <*@b.example>, *@c.example",
		"dial tcp 10.0.0.1:25: connection refused":                                  "dial tcp 10.0.0.1:25: connection refused",
	}
	for in, want := range cases {
		if got := mail.RedactAddresses(in); got != want {
			t.Errorf("RedactAddresses(%q) = %q, want %q", in, got, want)
		}
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

type failing struct{}

func (failing) Send(context.Context, mail.Message) error {
	return errors.New("550 <ayse@example.com> mailbox unavailable")
}

func TestMailLogsKeepAddressesOut(t *testing.T) {
	m := mail.Message{To: "ayse@example.com", Subject: "Your Yuva sign-in code: 123456", Text: "Your code is 123456."}
	buf := &syncBuffer{}
	log := slog.New(slog.NewTextHandler(buf, nil))
	if err := (mail.Log{Log: log}).Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if out := buf.String(); strings.Contains(out, "ayse@") || strings.Contains(out, "123456") || !strings.Contains(out, "*@example.com") {
		t.Fatalf("log mailer outside development: %s", out)
	}
	buf.Reset()
	code := m
	code.SignInCode = "123456"
	if err := (mail.Log{Log: log}).Send(context.Background(), code); err != nil {
		t.Fatal(err)
	}
	if out := buf.String(); strings.Count(out, "\n") != 1 || !strings.Contains(out, "code=123456") || !strings.Contains(out, "to=*@example.com") ||
		strings.Contains(out, "ayse@") || strings.Contains(out, "Your") {
		t.Fatalf("sign-in code outside development: %s", out)
	}
	buf.Reset()
	if err := (mail.Log{Log: log, Full: true}).Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if out := buf.String(); !strings.Contains(out, "ayse@example.com") || !strings.Contains(out, "Your code is 123456.") {
		t.Fatalf("log mailer in development: %s", out)
	}
	buf.Reset()
	if err := (mail.Async{Mailer: failing{}, Log: log, Timeout: time.Second}).Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(buf.String(), "mail not sent") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if out := buf.String(); !strings.Contains(out, "mail not sent") || strings.Contains(out, "ayse@") || !strings.Contains(out, "*@example.com") {
		t.Fatalf("failed send log: %s", out)
	}
}

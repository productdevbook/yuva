package email_test

import (
	"bytes"
	"mime"
	"net/mail"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/email"
)

func TestVerifyIngressMatchesEdgeWorker(t *testing.T) {
	raw, err := os.ReadFile("../../../edge/test/fixtures/plain.eml")
	if err != nil {
		t.Skip("edge fixture not present:", err)
	}
	const sig = "v1=2a6b3a29c5330e504f6ab4b1fb4a2c2c43900fa6a54f29f12425a8ccf0c6046a"
	at := time.Unix(1791364364, 0)
	if err := email.VerifyIngress("test-secret", "1791364364", "support@example.com", sig, raw, at.Add(time.Minute)); err != nil {
		t.Fatalf("edge signature rejected: %v", err)
	}
	if got := email.Sign("test-secret", "1791364364", "support@example.com", raw); got != sig {
		t.Fatalf("Sign = %s", got)
	}
	if err := email.VerifyIngress("test-secret", "1791364364", "support@example.com", sig, raw, at.Add(6*time.Minute)); err != email.ErrStaleTimestamp {
		t.Fatalf("replay outside the window: %v", err)
	}
	if err := email.VerifyIngress("test-secret", "1791364364", "support@example.com", sig, raw, at.Add(-6*time.Minute)); err != email.ErrStaleTimestamp {
		t.Fatalf("timestamp from the future: %v", err)
	}
	if err := email.VerifyIngress("other", "1791364364", "support@example.com", sig, raw, at); err != email.ErrBadSignature {
		t.Fatalf("wrong secret: %v", err)
	}
	if err := email.VerifyIngress("test-secret", "1791364364", "support@example.com", "v1=zz", raw, at); err != email.ErrBadSignature {
		t.Fatalf("malformed signature: %v", err)
	}
}

func TestBuildHeaders(t *testing.T) {
	refs := make([]string, 30)
	for i := range refs {
		refs[i] = "r" + string(rune('a'+i%26)) + "@example.org"
	}
	raw, err := email.Build(email.Outgoing{
		From: email.Address{Name: "Destek Ekibi", Email: "destek@example.com"}, ReplyTo: email.Address{Email: "destek@example.com"},
		To: email.Address{Name: "Ayşe", Email: "ayse@example.net"}, Subject: "Re: Sipariş #42",
		MessageID: "tok.rnd@example.com", InReplyTo: "parent@example.net", References: refs,
		Text: "Merhaba <script>", AutoSubmitted: true,
		Attachments: []email.Attachment{{Filename: "a.txt", ContentType: "text/plain", Data: []byte("hi")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if subject != "Re: Sipariş #42" {
		t.Errorf("Subject %q", subject)
	}
	if m.Header.Get("Message-ID") != "<tok.rnd@example.com>" || m.Header.Get("In-Reply-To") != "<parent@example.net>" {
		t.Errorf("ids: %v", m.Header)
	}
	if got := email.ParseIDList(m.Header.Get("References")); len(got) != 20 || got[0] != refs[0] || got[19] != refs[29] {
		t.Errorf("References %v", got)
	}
	if m.Header.Get("Auto-Submitted") != "auto-replied" || m.Header.Get("X-Auto-Response-Suppress") != "All" {
		t.Error("automatic message headers missing")
	}
	if !strings.HasPrefix(m.Header.Get("Content-Type"), "multipart/mixed") {
		t.Errorf("Content-Type %q", m.Header.Get("Content-Type"))
	}
	parsed, err := email.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(parsed.HTML, "Merhaba &lt;script&gt;") || len(parsed.Attachments) != 1 || !parsed.Auto {
		t.Errorf("round trip: html %q attachments %d auto %v", parsed.HTML, len(parsed.Attachments), parsed.Auto)
	}
}

func TestOurMessageIDs(t *testing.T) {
	token := email.NewConversationToken()
	id := email.NewMessageID(token, "Example.COM")
	got, domain, ok := email.TokenFromID("<" + strings.ToUpper(id) + ">")
	if !ok || got != token || domain != "example.com" {
		t.Fatalf("TokenFromID(%q) = %q %q %v", id, got, domain, ok)
	}
	if _, _, ok := email.TokenFromID("CAF1aB2cD3eF4gH5@mail.gmail.com"); ok {
		t.Fatal("a foreign id looks like ours")
	}
	for in, want := range map[string]string{"Invoice": "Re: Invoice", "RE: Re: Invoice": "Re: Invoice", "Ynt: Fatura": "Re: Fatura", "": ""} {
		if got := email.ReplySubject(in); got != want {
			t.Errorf("ReplySubject(%q) = %q", in, got)
		}
	}
}

func TestParseDSN(t *testing.T) {
	raw := "From: Mail Delivery System <MAILER-DAEMON@mx.example.net>\r\nTo: support@example.com\r\nSubject: Undelivered\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/report; report-type=delivery-status; boundary=r1\r\n\r\n" +
		"--r1\r\nContent-Type: text/plain\r\n\r\nSorry.\r\n" +
		"--r1\r\nContent-Type: message/delivery-status\r\n\r\nReporting-MTA: dns; mx.example.net\r\n\r\n" +
		"Final-Recipient: rfc822; Gone@Example.net\r\nAction: failed\r\nStatus: 5.1.1\r\nDiagnostic-Code: smtp; 550 5.1.1\r\n  user unknown\r\n\r\n" +
		"Final-Recipient: rfc822; slow@example.net\r\nAction: delayed\r\nStatus: 4.4.1\r\n\r\n" +
		"--r1\r\nContent-Type: message/rfc822\r\n\r\nFrom: support@example.com\r\nMessage-ID: <orig.id@example.com>\r\nSubject: Re: Q\r\n\r\nbody\r\n" +
		"--r1--\r\n"
	m, err := email.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if m.Bounce == nil || !m.Auto {
		t.Fatal("not recognized as a delivery report")
	}
	if m.Bounce.OriginalMessageID != "orig.id@example.com" {
		t.Errorf("original %q", m.Bounce.OriginalMessageID)
	}
	failed := m.Bounce.Failed()
	if len(m.Bounce.Recipients) != 2 || len(failed) != 1 || failed[0].Email != "gone@example.net" || !failed[0].Permanent() ||
		failed[0].Diagnostic != "550 5.1.1 user unknown" {
		t.Errorf("recipients %+v", m.Bounce.Recipients)
	}
}

func TestDMARCAndAutomatic(t *testing.T) {
	for in, want := range map[string]string{
		"mx.example.com; spf=pass; dkim=pass; dmarc=pass header.from=example.net":  email.DMARCPass,
		"mx.example.com; dmarc=fail (p=reject dis=reject) header.from=example.net": email.DMARCFail,
		"mx.example.com; dmarc=none":      email.DMARCNone,
		"mx.example.com; dmarc=temperror": email.DMARCUnknown,
		"":                                email.DMARCUnknown,
	} {
		if got := email.DMARCResult(in); got != want {
			t.Errorf("DMARCResult(%q) = %q, want %q", in, got, want)
		}
	}
	auto := []map[string]string{
		{"Auto-Submitted": "auto-generated"}, {"Precedence": "Bulk"}, {"Precedence": "auto_reply"}, {"X-Autoreply": "yes"},
		{"X-Auto-Response-Suppress": "OOF, AutoReply"}, {"List-Id": "<news.example.com>"}, {"Return-Path": "<>"},
	}
	for _, h := range auto {
		if !email.Automatic(h, "someone@example.net") {
			t.Errorf("%v not automatic", h)
		}
	}
	for _, h := range []map[string]string{{"Auto-Submitted": "no"}, {"Precedence": "first-class"}, {"X-Auto-Response-Suppress": "None"}, {}} {
		if email.Automatic(h, "someone@example.net") {
			t.Errorf("%v automatic", h)
		}
	}
	if !email.Automatic(map[string]string{}, "mailer-daemon@example.net") {
		t.Error("mailer-daemon not automatic")
	}
}

func TestHasRemoteImages(t *testing.T) {
	cases := map[string]bool{
		``: false,
		`<p>plain <a href="https://x.example">link</a></p>`: false,
		`<img src="cid:part1@x">`:                           false,
		`<img src="data:image/png;base64,AAAA">`:            false,
		`<img alt="no source">`:                             false,
		`<p><img src="https://t.example/p.gif"></p>`:        true,
		`<IMG SRC="HTTP://T.EXAMPLE/P.GIF">`:                true,
		`<img src="//t.example/p.gif">`:                     true,
		`<img src="/\t.example/p.gif">`:                     true,
	}
	for in, want := range cases {
		if got := email.HasRemoteImages(in); got != want {
			t.Errorf("HasRemoteImages(%q) = %v, want %v", in, got, want)
		}
	}
}

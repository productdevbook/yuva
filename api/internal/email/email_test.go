package email_test

import (
	"bytes"
	"fmt"
	"mime"
	"net/mail"
	"os"
	"slices"
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
	const (
		sig      = "v2=317d9bc511a3882d471ab816c963a77c7efea5fc8c83f43916ed701e14e83646"
		sigV1    = "v1=2a6b3a29c5330e504f6ab4b1fb4a2c2c43900fa6a54f29f12425a8ccf0c6046a"
		ts       = "1791364364"
		to, from = "support@example.com", "customer@example.org"
	)
	at := time.Unix(1791364364, 0)
	verify := func(secret, ts, to, from, sig string, at time.Time, acceptV1 bool) error {
		_, err := email.VerifyIngress(secret, ts, to, from, sig, raw, at, acceptV1)
		return err
	}
	if err := verify("test-secret", ts, to, from, sig, at.Add(time.Minute), false); err != nil {
		t.Fatalf("edge signature rejected: %v", err)
	}
	if got := email.Sign("test-secret", ts, to, from, raw); got != sig {
		t.Fatalf("Sign = %s", got)
	}
	if got := email.SignV1("test-secret", ts, to, raw); got != sigV1 {
		t.Fatalf("SignV1 = %s", got)
	}
	if err := verify("test-secret", ts, to, "attacker@example.net", sig, at, false); err != email.ErrBadSignature {
		t.Fatalf("envelope sender not covered: %v", err)
	}
	if err := verify("test-secret", ts, to, "", sig, at, false); err != email.ErrBadSignature {
		t.Fatalf("emptied envelope sender accepted: %v", err)
	}
	if err := verify("test-secret", ts, to, from, sigV1, at, false); err != email.ErrBadSignature {
		t.Fatalf("v1 accepted without the deprecation switch: %v", err)
	}
	c, err := email.VerifyIngress("test-secret", ts, to, "anything", sigV1, raw, at, true)
	if err != nil || !c.V1() {
		t.Fatalf("v1 with the switch: %v", err)
	}
	if err := verify("test-secret", ts, to, from, "v3="+sig[3:], at, true); err != email.ErrBadSignature {
		t.Fatalf("unknown version: %v", err)
	}
	if err := verify("test-secret", ts, to, from, sig, at.Add(6*time.Minute), false); err != email.ErrStaleTimestamp {
		t.Fatalf("replay outside the window: %v", err)
	}
	if err := verify("test-secret", ts, to, from, sig, at.Add(-6*time.Minute), false); err != email.ErrStaleTimestamp {
		t.Fatalf("timestamp from the future: %v", err)
	}
	if err := verify("other", ts, to, from, sig, at, false); err != email.ErrBadSignature {
		t.Fatalf("wrong secret: %v", err)
	}
	if err := verify("test-secret", ts, to, from, "v2=zz", at, false); err != email.ErrBadSignature {
		t.Fatalf("malformed signature: %v", err)
	}
	if _, err := email.StartIngress("test-secret", ts, to, from, sig, at.Add(time.Hour), false); err != email.ErrStaleTimestamp {
		t.Fatalf("stale timestamp must fail before the body: %v", err)
	}
	if _, err := email.StartIngress("test-secret", "", to, from, sig, at, false); err != email.ErrBadSignature {
		t.Fatalf("missing timestamp must fail before the body: %v", err)
	}
	streamed, err := email.StartIngress("test-secret", ts, to, from, sig, at, false)
	if err != nil {
		t.Fatal(err)
	}
	for chunk := range slices.Chunk(raw, 7) {
		_, _ = streamed.Write(chunk)
	}
	if err := streamed.Verify(); err != nil {
		t.Fatalf("streamed body: %v", err)
	}
}

func TestTrustedDMARC(t *testing.T) {
	for _, tc := range []struct{ header, id, want string }{
		{"mx.cloudflare.net; dkim=pass header.d=example.net; dmarc=pass header.from=example.net", "mx.cloudflare.net", email.DMARCPass},
		{"MX.Cloudflare.net; dmarc=fail (p=reject) header.from=example.net", "mx.cloudflare.net", email.DMARCFail},
		{"mx.cloudflare.net 1; dmarc=fail header.from=example.net", "mx.cloudflare.net", email.DMARCFail},
		{"attacker.example; dmarc=pass header.from=example.net", "mx.cloudflare.net", email.DMARCUnknown},
		{"mx.cloudflare.net.evil; dmarc=pass", "mx.cloudflare.net", email.DMARCUnknown},
		{"mx.cloudflare.net; dmarc=fail", "", email.DMARCUnknown},
		{"", "mx.cloudflare.net", email.DMARCUnknown},
	} {
		if got := email.TrustedDMARC(tc.header, tc.id); got != tc.want {
			t.Errorf("TrustedDMARC(%q, %q) = %q, want %q", tc.header, tc.id, got, tc.want)
		}
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

func TestParseBoundsThreadIDs(t *testing.T) {
	var refs strings.Builder
	for i := range 200000 {
		fmt.Fprintf(&refs, " <r%d@example.com>", i)
	}
	long := "<" + strings.Repeat("x", 1000) + "@example.com>"
	raw := "From: a@example.com\r\nTo: b@example.com\r\nSubject: s\r\nMessage-ID: <m@example.com>\r\n" +
		"In-Reply-To: <p1@example.com> <p2@example.com> <p3@example.com> <p4@example.com> <p5@example.com> <p6@example.com>\r\n" +
		"References:" + refs.String() + " " + long + "\r\n\r\nbody\r\n"
	start := time.Now()
	m, err := email.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("parsing took %v", took)
	}
	if len(m.References) != 50 || m.References[0] != "r0@example.com" || m.References[49] != "r199999@example.com" {
		t.Fatalf("References: %d, first %q, last %q", len(m.References), m.References[0], m.References[len(m.References)-1])
	}
	if len(m.InReplyTo) != 5 || m.InReplyTo[0] != "p1@example.com" {
		t.Fatalf("In-Reply-To: %v", m.InReplyTo)
	}
}

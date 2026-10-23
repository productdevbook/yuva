package email_test

import (
	"testing"

	"github.com/productdevbook/yuva/api/internal/email"
)

func TestTrustedDMARCReadsOnlyTheMethodResult(t *testing.T) {
	const id = "mx.cloudflare.net"
	for _, tc := range []struct{ name, header, want string }{
		{"envelope local part spells a pass", "mx.cloudflare.net; spf=pass smtp.mailfrom=dmarc=pass@attacker.example; dmarc=fail header.from=victim.example", email.DMARCFail},
		{"dkim identity spells a pass", "mx.cloudflare.net; dkim=pass header.i=dmarc=pass@attacker.example header.d=attacker.example header.s=dmarc=pass; dmarc=none header.from=victim.example", email.DMARCNone},
		{"quoted local part with a semicolon", `mx.cloudflare.net; spf=pass smtp.mailfrom="x;dmarc=pass"@attacker.example; dmarc=fail header.from=victim.example`, email.DMARCFail},
		{"unquoted semicolon in the envelope", "mx.cloudflare.net; spf=pass smtp.mailfrom=x;dmarc=pass@attacker.example; dmarc=none header.from=victim.example", email.DMARCUnknown},
		{"pass inside a comment", "mx.cloudflare.net; dmarc=fail (dmarc=pass) header.from=victim.example", email.DMARCFail},
		{"semicolon inside a comment", "mx.cloudflare.net; spf=pass (x; dmarc=pass) smtp.mailfrom=a@attacker.example; dmarc=none header.from=victim.example", email.DMARCNone},
		{"nested comment", "mx.cloudflare.net; spf=pass (a (b; dmarc=pass) c) smtp.mailfrom=a@attacker.example; dmarc=none", email.DMARCNone},
		{"comment before the method", "mx.cloudflare.net; (dmarc=fail) dmarc=pass header.from=example.net", email.DMARCPass},
		{"pass inside a quoted reason", `mx.cloudflare.net; dmarc=none reason="dmarc=pass; ok" header.from=victim.example`, email.DMARCNone},
		{"property after a method spells a pass", "mx.cloudflare.net; spf=pass smtp.mailfrom=a@attacker.example dmarc=pass", email.DMARCUnknown},
		{"no dmarc result", "mx.cloudflare.net; spf=pass smtp.mailfrom=a@example.net", email.DMARCUnknown},
		{"no result at all", "mx.cloudflare.net; none", email.DMARCUnknown},
		{"comment left open", "mx.cloudflare.net; dmarc=pass (open header.from=example.net", email.DMARCUnknown},
		{"quote left open", `mx.cloudflare.net; dkim=pass header.b="abc; dmarc=pass header.from=example.net`, email.DMARCUnknown},
		{"result that is not a keyword", "mx.cloudflare.net; dmarc=pass@attacker.example", email.DMARCUnknown},
		{"other result", "mx.cloudflare.net; dmarc=temperror header.from=example.net", email.DMARCUnknown},
		{"longer word", "mx.cloudflare.net; dmarc=passed header.from=example.net", email.DMARCUnknown},
		{"spaces and version", "mx.cloudflare.net 1; dmarc / 1 = pass header.from=example.net", email.DMARCPass},
		{"method in capitals", "mx.cloudflare.net; DMARC=Pass header.from=example.net", email.DMARCPass},
		{"two passes", "mx.cloudflare.net; dmarc=pass header.from=example.net; dmarc=pass header.from=example.net", email.DMARCPass},
		{"pass and none", "mx.cloudflare.net; dmarc=pass header.from=example.net; dmarc=none header.from=example.net", email.DMARCUnknown},
		{"pass, none and fail", "mx.cloudflare.net; dmarc=pass; dmarc=none; dmarc=fail", email.DMARCFail},
		{"trailing semicolon", "mx.cloudflare.net; dmarc=pass header.from=example.net;", email.DMARCPass},
		{"edge fixture layout", "mx.cloudflare.net; spf=pass smtp.mailfrom=example.org; dkim=pass header.d=example.org; dmarc=pass header.from=example.org", email.DMARCPass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := email.TrustedDMARC([]string{tc.header}, id); got != tc.want {
				t.Fatalf("TrustedDMARC(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestTrustedDMARCOnFoldedReceiverHeaders(t *testing.T) {
	const pass = "Authentication-Results: mx.cloudflare.net;\r\n" +
		"\tdkim=pass header.d=example.net header.s=s1 header.b=\"Wx4H+7E\";\r\n" +
		"\tdmarc=pass header.from=example.net policy.dmarc=none;\r\n" +
		"\tspf=none (mx.cloudflare.net: no SPF records found for postmaster@a1.mail.example.net) smtp.helo=a1.mail.example.net;\r\n" +
		"\tspf=pass (mx.cloudflare.net: domain of b-1@example.net designates 192.0.2.1 as permitted sender) smtp.mailfrom=b-1@example.net;\r\n" +
		"\tarc=none smtp.remote-ip=192.0.2.1\r\n"
	const fail = "Authentication-Results: mx.cloudflare.net;\r\n" +
		"\tdkim=pass header.d=attacker.example header.i=dmarc=pass@attacker.example header.b=\"abc\";\r\n" +
		"\tdmarc=fail reason=\"SPF not aligned (relaxed); dmarc=pass\" header.from=victim.example (policy=quarantine);\r\n" +
		"\tspf=pass (mx.cloudflare.net: domain of dmarc=pass@attacker.example designates 192.0.2.9 as permitted sender) smtp.mailfrom=dmarc=pass@attacker.example\r\n"
	for _, tc := range []struct{ name, header, want string }{
		{"pass", pass, email.DMARCPass},
		{"fail with sender-chosen identities", fail, email.DMARCFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := email.Parse([]byte(tc.header + "From: a@example.net\r\nTo: b@example.com\r\nSubject: x\r\n\r\nbody\r\n"))
			if err != nil {
				t.Fatal(err)
			}
			if got := email.TrustedDMARC(m.AllAuthResults, "mx.cloudflare.net"); got != tc.want {
				t.Fatalf("TrustedDMARC = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHeaderSenderMatchesParse(t *testing.T) {
	raw := []byte("Authentication-Results: mx.cloudflare.net;\r\n\tdkim=pass header.d=example.net;\r\n\tdmarc=pass header.from=example.net\r\n" +
		"Authentication-Results: mx.example.org; dmarc=fail\r\n" +
		"From: =?UTF-8?Q?Ay=C5=9Fe?= <Ayse@Example.NET>\r\nTo: b@example.com\r\nSubject: x\r\n" +
		"Content-Type: text/html\r\n\r\n<p>Hi</p>\r\n")
	from, ar := email.HeaderSender(raw)
	m, err := email.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if from != m.From.Email || len(ar) != 2 || email.TrustedDMARC(ar, "mx.cloudflare.net") != email.TrustedDMARC(m.AllAuthResults, "mx.cloudflare.net") {
		t.Fatalf("header %q %q, parse %q %q", from, ar, m.From.Email, m.AllAuthResults)
	}
	if email.TrustedDMARC(ar, "mx.cloudflare.net") != email.DMARCPass || m.Text != "Hi" {
		t.Fatalf("dmarc %q text %q", email.TrustedDMARC(ar, "mx.cloudflare.net"), m.Text)
	}
	if from, ar := email.HeaderSender([]byte("not a message")); from != "" || ar != nil {
		t.Fatalf("garbage: %q %q", from, ar)
	}
}

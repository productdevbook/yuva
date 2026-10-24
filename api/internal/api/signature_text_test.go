package api

import (
	"strings"
	"testing"
)

func TestSignatureFillEscapesMarkdown(t *testing.T) {
	a := signatureAuthor{name: `Ada_[Lovelace](x) <b>`, title: "#1 *lead* | `ops` ~ !"}
	got := renderHelp(a.fill("{{member.name}}, {{member.title}}"), helpImageSet{})
	if strings.Contains(got, "<a ") || strings.Contains(got, "<em>") || strings.Contains(got, "<code>") || strings.Contains(got, "<b>") || strings.Contains(got, "<h1") {
		t.Fatalf("a placeholder value was read as Markdown: %s", got)
	}
	if text := signatureText(got); text != "Ada_[Lovelace](x) <b>, #1 *lead* | `ops` ~ !" {
		t.Fatalf("text %q", text)
	}

	a = signatureAuthor{name: "www.example.com", title: "Lead\n- item\r\n# x\t1. a@b.co https://x.y +=-.:&\x00 q"}
	got = renderSignatureMarkdown(a.fill("{{member.name}}\n{{member.title}}"), helpImageSet{})
	for _, bad := range []string{"<a ", "<li", "<ol", "<ul", "<h1", "<em>", "<strong>"} {
		if strings.Contains(got, bad) {
			t.Fatalf("a placeholder value changed the Markdown structure (%s): %s", bad, got)
		}
	}
	if strings.Count(got, "<br") != 1 || strings.Count(got, "<p>") != 1 {
		t.Fatalf("a placeholder value broke the line: %s", got)
	}
	if text := signatureText(got); text != "www.example.com\nLead - item # x 1. a@b.co https://x.y +=-.:& q" {
		t.Fatalf("text %q", text)
	}
}

func TestSignatureHardWraps(t *testing.T) {
	got := renderSignatureMarkdown("Name\nTitle\nAcme\n\nPhone", helpImageSet{})
	if strings.Count(got, "<br") != 2 {
		t.Fatalf("single newlines were collapsed: %s", got)
	}
	if text := signatureText(got); text != "Name\nTitle\nAcme\nPhone" {
		t.Fatalf("text %q", text)
	}
}

func TestBatchSignatureLastMessageDecides(t *testing.T) {
	sig, text := "<p>Sig</p>", "Sig"
	signed := messageRow{SignatureHtml: &sig, SignatureText: &text}
	if _, _, ok := batchSignature([]messageRow{signed, {}}); ok {
		t.Fatal("an unsigned last message took an earlier message's signature")
	}
	if h, tx, ok := batchSignature([]messageRow{{}, signed}); !ok || h != sig || tx != text {
		t.Fatalf("signed last message: %q %q %v", h, tx, ok)
	}
	if _, _, ok := batchSignature(nil); ok {
		t.Fatal("an empty batch is signed")
	}
}

func TestSignedOwnHTML(t *testing.T) {
	text, h := signedOwnHTML("Hi.\n\n> old\n", "<html><body><p>Hi.</p><blockquote>old</blockquote></BODY></html>", "<div>card</div>", "Sig", "<p>Sig</p>")
	if text != "Hi.\n\n> old\n\n-- \nSig" {
		t.Fatalf("text %q", text)
	}
	if want := `<html><body><p>Hi.</p><blockquote>old</blockquote><div>card</div><div class="yuva-signature"><p>Sig</p></div></BODY></html>`; h != want {
		t.Fatalf("html %s", h)
	}
	text, h = signedOwnHTML("Hi.", "<p>Hi.</p>", "", "", `<img alt="">`)
	if text != "Hi." || h != `<p>Hi.</p><div class="yuva-signature"><img alt=""></div>` {
		t.Fatalf("image-only signature on a fragment: %q %s", text, h)
	}
}

func TestSignedText(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{"Hi.", "Hi.\n\n-- \nSig"},
		{"Hi.\n\n> old\n>\n\n> older\n", "Hi.\n\n-- \nSig\n\n> old\n>\n\n> older"},
		{"> only a quote", "-- \nSig\n\n> only a quote"},
		{"> quoted\n\nThen my answer.", "> quoted\n\nThen my answer.\n\n-- \nSig"},
	} {
		if got := signedText(c.body, "Sig"); got != c.want {
			t.Errorf("signedText(%q) = %q, want %q", c.body, got, c.want)
		}
	}
	if got := signedText("Hi.\n\n> old", ""); got != "Hi.\n\n> old" {
		t.Errorf("an image-only signature wrote a delimiter: %q", got)
	}
	html := signedHTML("Hi.\n> old", "<div>card</div>", "<p>Sig</p>")
	if i, j, k, l := strings.Index(html, "Hi."), strings.Index(html, "card"), strings.Index(html, "yuva-signature"), strings.Index(html, "&gt; old"); i < 0 || j < i || k < j || l < k {
		t.Fatalf("signedHTML order: %s", html)
	}
}

func TestSignatureText(t *testing.T) {
	got := signatureText(`<p>Deniz<br>Lead</p><ul><li>one</li><li>two</li></ul><p><a href="https://acme.example">https://acme.example</a> <a href="mailto:d@acme.example">d@acme.example</a> <a href="https://acme.example/x">site</a></p>`)
	want := "Deniz\nLead\none\ntwo\nhttps://acme.example d@acme.example site <https://acme.example/x>"
	if got != want {
		t.Fatalf("signatureText = %q, want %q", got, want)
	}
}

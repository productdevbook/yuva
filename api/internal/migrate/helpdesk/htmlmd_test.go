package helpdesk_test

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
)

func TestHTMLToMarkdown(t *testing.T) {
	base, _ := url.Parse("https://acme.zendesk.com")
	rw := helpdesk.Rewrite{
		Base:  base,
		Image: map[string]string{"https://acme.zendesk.com/hc/article_attachments/1/a.png": "https://yuva.example/i/aaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Link: func(href string) string {
			if href == "https://acme.zendesk.com/hc/en-us/articles/42-reset" {
				return "https://yuva.example/help/acme/en/articles/7-reset"
			}
			return ""
		},
	}
	cases := []struct{ name, in, want string }{
		{"paragraphs and emphasis", `<p>Hello <strong>bold</strong>, <em>soft</em> and <del>gone</del>.</p><p>Second</p>`, "Hello **bold**, *soft* and ~~gone~~.\n\nSecond\n"},
		{"space inside emphasis moves out", `<p>a<b> bold </b>b</p>`, "a **bold** b\n"},
		{"headings", `<h2>Title</h2><h3 id="x">Sub <a name="x"></a></h3>`, "## Title\n\n### Sub\n"},
		{"special characters", `<p>2 * 3 = 6 and [not a link] under_score</p>`, "2 \\* 3 = 6 and \\[not a link\\] under\\_score\n"},
		{"text that would start a block", `<p>- not a list</p><p>1. not either</p>`, "\\- not a list\n\n\\1. not either\n"},
		{"links", `<p><a href="/hc/en-us/articles/42-reset">reset</a> and <a href="https://example.org/a b(1)">out</a> <a href="javascript:x()">js</a> <a href="#top">top</a></p>`,
			"[reset](https://yuva.example/help/acme/en/articles/7-reset) and [out](https://example.org/a%20b%281%29) js [top](#top)\n"},
		{"stored image", `<p><img src="/hc/article_attachments/1/a.png" alt="A shot"></p>`, "![A shot](https://yuva.example/i/aaaaaaaaaaaaaaaaaaaaaaaaaa)\n"},
		{"unstored image stays a link", `<p><img src="https://cdn.example.org/b.png"></p>`, "[image](https://cdn.example.org/b.png)\n"},
		{"lists", `<ul><li>one</li><li>two<ul><li>nested</li></ul></li></ul><ol start="3"><li>three</li><li>four</li></ol>`, "- one\n- two\n  - nested\n\n3. three\n4. four\n"},
		{"code", `<p>Run <code>go test</code> now</p><pre><code class="language-sh">echo "hi"
ls</code></pre>`, "Run `go test` now\n\n```sh\necho \"hi\"\nls\n```\n"},
		{"fence longer than content", "<pre>a ``` b</pre>", "````\na ``` b\n````\n"},
		{"quote", `<blockquote><p>Mind the gap</p><p>twice</p></blockquote>`, "> Mind the gap\n>\n> twice\n"},
		{"table", `<table><thead><tr><th>Plan</th><th>Seats</th></tr></thead><tbody><tr><td>Free</td><td>1 | 2</td></tr></tbody></table>`, "| Plan | Seats |\n| --- | --- |\n| Free | 1 \\| 2 |\n"},
		{"table without head", `<table><tr><td>a</td><td>b</td></tr><tr><td>c</td></tr></table>`, "| a | b |\n| --- | --- |\n| c |  |\n"},
		{"line break", `<p>one<br>two</p>`, "one\\\ntwo\n"},
		{"rule and divs", `<div><div>in</div><hr><span>after</span></div>`, "in\n\n---\n\nafter\n"},
		{"scripts and widgets dropped", `<p>keep</p><script>alert(1)</script><style>p{}</style><button>no</button>`, "keep\n"},
		{"video becomes a link", `<iframe src="https://video.example.org/embed/9" title="Tour"></iframe>`, "[Tour](https://video.example.org/embed/9)\n"},
		{"empty", `<p> </p>`, ""},
	}
	for _, c := range cases {
		if got := helpdesk.HTMLToMarkdown(c.in, rw); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestHTMLImages(t *testing.T) {
	base, _ := url.Parse("https://acme.zendesk.com")
	got := helpdesk.HTMLImages(`<p><img src="/a.png"><img src="https://x.example.org/b.png"><img src="/a.png"><img src="data:image/png;base64,AAAA"><img></p>`, base)
	want := []string{"https://acme.zendesk.com/a.png", "https://x.example.org/b.png"}
	if !slices.Equal(got, want) {
		t.Errorf("images %v, want %v", got, want)
	}
}

func TestCutMarkdown(t *testing.T) {
	md := "one\n\ntwo\n\n```\ncode\n\nmore\n```\n\nthree\n"
	got, cut := helpdesk.CutMarkdown(md, 22)
	if !cut || got != "one\n\ntwo\n" {
		t.Errorf("cut inside a fence: %q %v", got, cut)
	}
	if got, cut := helpdesk.CutMarkdown(md, 1000); cut || got != md {
		t.Errorf("short body: %q %v", got, cut)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("no trailing newline")
	}
}

func TestFlattenHelpCollections(t *testing.T) {
	in := []helpdesk.HelpCollection{
		{ID: "c1", Name: "Billing"},
		{ID: "s1", ParentID: "c1", Name: "Invoices"},
		{ID: "s2", ParentID: "s1", Name: "Refunds"},
		{ID: "s3", ParentID: "s2", Name: "Late"},
	}
	out := helpdesk.FlattenHelpCollections(in)
	want := map[string][2]string{"c1": {"", "Billing"}, "s1": {"c1", "Invoices"}, "s2": {"c1", "Invoices / Refunds"}, "s3": {"c1", "Invoices / Refunds / Late"}}
	for _, c := range out {
		if w := want[c.ID]; c.ParentID != w[0] || c.Name != w[1] {
			t.Errorf("%s: parent %q name %q, want %v", c.ID, c.ParentID, c.Name, w)
		}
	}
}

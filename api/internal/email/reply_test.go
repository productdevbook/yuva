package email_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/email/reply"
)

func TestReplyFixtures(t *testing.T) {
	files, err := filepath.Glob("testdata/reply/*.eml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".eml")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(strings.TrimSuffix(f, ".eml") + ".txt")
			if err != nil {
				t.Fatal(err)
			}
			m, err := email.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			got, quoted := reply.Strip(m.Text)
			if got != strings.TrimSpace(string(want)) {
				t.Errorf("visible text\n got: %q\nwant: %q", got, strings.TrimSpace(string(want)))
			}
			if wantQuoted := !strings.HasPrefix(name, "new_") && !strings.HasPrefix(name, "forward_"); quoted != wantQuoted {
				t.Errorf("quoted = %v, want %v", quoted, wantQuoted)
			}
		})
	}
}

func TestStripKeepsWhollyQuotedText(t *testing.T) {
	in := "> only a quote\n> nothing else"
	got, quoted := reply.Strip(in)
	if got != in || quoted {
		t.Fatalf("got %q, %v", got, quoted)
	}
}

func TestStripScansABoundedPrefix(t *testing.T) {
	flood := strings.Repeat("From: x\nDate: y\n", 1<<20)
	start := time.Now()
	if got, quoted := reply.Strip(flood); len(got) != len(strings.TrimSpace(flood)) || quoted {
		t.Fatalf("got %d bytes, %v", len(got), quoted)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("stripping took %v", took)
	}
	quote := "Hello\n\nFrom: Ada\nSent: Monday\nTo: Bob\nSubject: Hi\n\n"
	if got, quoted := reply.Strip(quote + flood); got != "Hello" || !quoted {
		t.Fatalf("got %d bytes, %v", len(got), quoted)
	}
	long := strings.Repeat("plain line of text\n", 10000) + "\nOn Mon, Ada wrote:\n> old"
	if got, quoted := reply.Strip(long); got != strings.TrimSpace(long) || quoted {
		t.Fatalf("a quote past the scanned prefix hid text: %d bytes, %v", len(got), quoted)
	}
}

func TestStripHTMLQuotes(t *testing.T) {
	cases := map[string]struct{ in, keep, drop string }{
		"gmail": {
			in:   `<div dir="ltr">New text</div><div class="gmail_quote"><div class="gmail_attr">On Tue wrote:</div><blockquote class="gmail_quote">Old text</blockquote></div>`,
			keep: "New text", drop: "Old text",
		},
		"apple": {
			in:   `<html><body><div>Sure.</div><div><br><blockquote type="cite"><div>On 6 Oct 2026, Support wrote:</div><div>Old text</div></blockquote></div></body></html>`,
			keep: "Sure.", drop: "Old text",
		},
		"thunderbird": {
			in:   `<p>Thanks</p><div class="moz-cite-prefix">On 10/6/26 16:02, Support wrote:<br></div><blockquote type="cite" cite="mid:x@example.com">Old text</blockquote>`,
			keep: "Thanks", drop: "wrote",
		},
		"outlook": {
			in:   `<div>Done.</div><hr style="display:inline-block;width:98%" tabindex="-1"><div id="divRplyFwdMsg" dir="ltr"><b>From:</b> Support</div><div>Old text</div>`,
			keep: "Done.", drop: "Old text",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			out, removed := email.StripHTMLQuotes(c.in)
			if !removed || !strings.Contains(out, c.keep) || strings.Contains(out, c.drop) {
				t.Fatalf("removed=%v out=%q", removed, out)
			}
		})
	}
	if _, removed := email.StripHTMLQuotes(`<div>No quotes here</div>`); removed {
		t.Fatal("removed from a message without quotes")
	}
}

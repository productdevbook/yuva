package design

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const messageBlocks = `{"id": "logo", "type": "heading", "level": 2, "text": "Northwind"},
	{"id": "s", "type": "section", "background": "#eef2ff", "blocks": [{"id": "m", "type": "message"}]},
	{"id": "f", "type": "footer", "html": "<p>Hi {{ contact.first_name | \"there\" }}, we answer within a day.</p>"}`

func TestRenderMessageGolden(t *testing.T) {
	slot := &MessageSlot{
		Text:          "Hello Ada,\n\nyour order <#1042> & the invoice are on their way.\nTracking follows.\n\nThanks",
		CardsHTML:     `<table role="presentation"><tr><td><a href="https://help.example.com/a/1">Returns</a></td></tr></table>`,
		CardsText:     "Returns: https://help.example.com/a/1",
		SignatureHTML: "<p>Grace<br>Northwind support</p>",
		SignatureText: "Grace\nNorthwind support",
	}
	modes := map[string]Footer{"list": FooterList, "reply": FooterReply, "outreach": FooterOutreach}
	for mode, footer := range modes {
		for _, dark := range []bool{false, true} {
			for _, rtl := range []bool{false, true} {
				name := "message_" + mode
				th := theme
				if dark {
					name += "_dark"
				}
				if rtl {
					name += "_rtl"
					th = strings.Replace(theme, `"width": 600`, `"width": 600, "direction": "rtl"`, 1)
				}
				t.Run(name, func(t *testing.T) {
					in := input(doc(t, messageBlocks, th))
					in.Footer, in.Dark = footer, dark
					if footer != FooterList {
						in.Message = slot
						in.Values.ListName, in.Values.PreferencesURL = "", ""
					}
					if footer == FooterReply {
						in.Values.UnsubscribeURL = ""
					}
					out, err := Render(in)
					if err != nil {
						t.Fatal(err)
					}
					checkWellFormed(t, out.HTML)
					warn, _ := json.MarshalIndent(out.Warnings, "", "  ")
					for ext, body := range map[string]string{"html": out.HTML, "txt": out.Text, "warnings.json": string(warn) + "\n"} {
						golden(t, name+"."+ext, body)
					}
				})
			}
		}
	}
}

func golden(t *testing.T, name, body string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != body {
		t.Errorf("%s differs from its golden file (go test -update to accept)", path)
	}
}

func TestMessageSlot(t *testing.T) {
	in := input(doc(t, messageBlocks))
	in.Footer = FooterOutreach
	in.Message = &MessageSlot{Text: "One <b>\nline two\n\n\nNew paragraph", SignatureHTML: "<p>Sig</p>", SignatureText: "Sig"}
	out, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.HTML, "One &lt;b&gt;<br>line two</p>") || !strings.Contains(out.HTML, ">New paragraph</p>") {
		t.Errorf("body not escaped into paragraphs: %s", out.HTML)
	}
	if strings.Index(out.HTML, "New paragraph") > strings.Index(out.HTML, "<p>Sig</p>") || strings.Index(out.HTML, "<p>Sig</p>") > strings.Index(out.HTML, "we answer within a day") {
		t.Error("the signature is not between the body and the blocks after the message block")
	}
	if !strings.Contains(out.Text, "\n-- \nSig\n") {
		t.Errorf("text part lost the signature delimiter: %q", out.Text)
	}
	if !strings.Contains(out.Text, "Northwind\n\nOne <b>\nline two\n\nNew paragraph") || !strings.Contains(out.Text, "Unsubscribe: https://yuva.example.com/m/tok") {
		t.Errorf("text part: %q", out.Text)
	}
	if strings.Contains(out.HTML, "Manage preferences") || strings.Contains(out.Text, "subscribed to") || strings.Contains(out.HTML, "Your message appears here") {
		t.Errorf("outreach footer carries list words: %s", out.Text)
	}
	in.Footer, in.Sender.LegalName = FooterReply, ""
	out, _ = Render(in)
	if strings.Contains(out.Text, "Unsubscribe") || strings.Contains(out.Text, "12 Harbour Street") {
		t.Errorf("a reply without a legal name has a footer: %q", out.Text)
	}
	for _, w := range out.Warnings {
		if w.Code == "sender_identity_missing" {
			t.Error("a reply warns about the sender identity")
		}
	}
}

func TestMessageBlockRules(t *testing.T) {
	one := `{"id": "m", "type": "message"}`
	if is := validateBlocks(t, one+`, {"id": "s", "type": "section", "blocks": [{"id": "n", "type": "message"}]}`, Options{}); !slices.Contains(codes(is), "n:type:message_block_repeated") {
		t.Errorf("two message blocks: %v", codes(is))
	}
	inCols := `{"id": "c", "type": "columns", "columns": [{"width": 50, "blocks": [{"id": "m", "type": "message"}]}, {"width": 50, "blocks": []}]}`
	if is := validateBlocks(t, inCols, Options{}); !slices.Contains(codes(is), "m:type:nesting") {
		t.Errorf("message in columns: %v", codes(is))
	}
	if is := validateBlocks(t, `{"id": "m", "type": "message", "html": "<p>x</p>"}`, Options{}); len(is) == 0 {
		t.Error("a message block took text")
	}
	if is := validateBlocks(t, `{"id": "m", "type": "message", "padding": [8, 32, 8, 32], "background": "#ffffff", "align": "center", "hide": "none"}`, Options{}); len(is) != 0 {
		t.Errorf("base fields: %v", codes(is))
	}
	raw := `{"schema": 1, "theme": ` + theme + `, "locale": "en", "translations": {"de": {"blocks": {"m": "Hallo"}}}, "blocks": [` + one + `]}`
	d, is, err := Parse([]byte(raw))
	if err != nil || len(is) > 0 {
		t.Fatal(err, is)
	}
	if is := Validate(d, Options{}); !slices.Contains(codes(is), "m:translations.de.blocks.m:translation_block") {
		t.Errorf("translation entry: %v", codes(is))
	}
	for blocks, want := range map[string][2]any{
		one:    {1, false},
		inCols: {1, true},
		`{"id": "t", "type": "text", "html": "<p>x</p>"}`:                                         {0, false},
		`{"id": "s", "type": "section", "blocks": [` + one + `, {"id": "n", "type": "message"}]}`: {2, false},
	} {
		d, _, _ := Parse([]byte(`{"schema": 1, "theme": ` + theme + `, "blocks": [` + blocks + `]}`))
		if n, nested := d.HasMessageBlock(); n != want[0] || nested != want[1] {
			t.Errorf("%s: %d %v", blocks, n, nested)
		}
	}
}

func TestConversationFields(t *testing.T) {
	d := doc(t, `{"id": "t", "type": "text", "html": "<p>{{ list.name }} {{ contact.name }} {{ contact.attributes.plan | \"Free\" }}</p>"},
		{"id": "b", "type": "button", "label": "Leave", "url": "{{ unsubscribe_url }}"}, {"id": "m", "type": "message"}`)
	got := codes(d.FieldIssues(Values{}, ConversationFields))
	want := []string{"t::merge_field_unavailable", "t::merge_field_empty", "b::merge_field_unavailable"}
	if !slices.Equal(got, want) {
		t.Errorf("issues %v, want %v", got, want)
	}
	if is := d.FieldIssues(Values{ContactName: "Ada"}, ConversationFields); len(is) != 2 {
		t.Errorf("with a name: %v", codes(is))
	}
}

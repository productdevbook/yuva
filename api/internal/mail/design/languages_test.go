package design

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	versions := []string{"en", "es", "pt-BR", "zh-Hant", "tr"}
	cases := []struct {
		contact, want string
		matched       bool
	}{
		{"es-MX", "es", true},
		{"es_mx", "es", true},
		{"zh-Hant-TW", "zh-Hant", true},
		{"zh-hant", "zh-Hant", true},
		{"TR", "tr", true},
		{"pt", "pt-BR", true},
		{"pt-PT", "pt-BR", true},
		{"fr", "en", false},
		{"fr-CA", "en", false},
		{"", "en", false},
		{"not a tag", "en", false},
	}
	for _, c := range cases {
		if got, matched := Resolve(c.contact, versions, "en"); got != c.want || matched != c.matched {
			t.Errorf("Resolve(%q) = %q %v, want %q %v", c.contact, got, matched, c.want, c.matched)
		}
	}
	if got, matched := Resolve("es", []string{"en", "es-MX"}, "en"); got != "es-MX" || !matched {
		t.Errorf("es against es-MX: %q %v", got, matched)
	}
	if got, _ := Resolve("de", []string{"en", "tr"}, "tr"); got != "tr" {
		t.Errorf("fallback: %q", got)
	}
}

const versioned = `{"schema": 1, "locale": "en", "fallback_locale": "en", "preheader": "Our news", "theme": ` + theme + `, "blocks": [
  {"id": "h", "type": "heading", "level": 1, "text": "Hello {{ contact.first_name | \"there\" }}"},
  {"id": "s", "type": "section", "blocks": [
    {"id": "c", "type": "columns", "columns": [
      {"width": 50, "blocks": [{"id": "i", "type": "image", "image_id": "0190a3c4-7b2e-7d1f-9a3b-1c2d3e4f5a6b", "alt": "A boat", "link": "https://example.com/boat"}]},
      {"width": 50, "blocks": [{"id": "t", "type": "text", "html": "<p>Read <a href=\"https://example.com/news\">the news</a>.</p>"}]}
    ]}
  ]},
  {"id": "d", "type": "divider"},
  {"id": "b", "type": "button", "label": "Open", "url": "https://example.com/open"}
],
"translations": {
  "es": {"preheader": "Nuestras noticias", "blocks": {"h": "Hola {{ contact.first_name | \"amigo\" }}", "i": "Un barco", "t": "<p>Lee <a href=\"https://example.com/news\">las noticias</a>.</p>", "b": "Abrir"}},
  "ar": {"direction": "rtl", "blocks": {"h": "مرحبا"}}
}}`

func parsed(t *testing.T, raw string) Document {
	t.Helper()
	d, issues, err := Parse([]byte(raw))
	if err != nil || len(issues) > 0 {
		t.Fatalf("parse: %v %v", err, issues)
	}
	return d
}

func TestLocalizedChangesOnlyTexts(t *testing.T) {
	d := parsed(t, versioned)
	if is := Validate(d, Options{}); len(is) > 0 {
		t.Fatalf("validate: %v", is)
	}
	if v := d.Versions(); !slices.Equal(v, []string{"en", "ar", "es"}) {
		t.Fatalf("versions: %v", v)
	}
	want := d.Localized("en")
	if want.Translations != nil || want.Locale != "en" || want.Blocks[0].Text != d.Blocks[0].Text {
		t.Fatalf("base version: %+v", want)
	}
	want.Locale = "es"
	want.Preheader = "Nuestras noticias"
	want.Blocks[0].Text = `Hola {{ contact.first_name | "amigo" }}`
	cols := want.Blocks[1].Blocks[0].Columns
	cols[0].Blocks[0].Alt = "Un barco"
	cols[1].Blocks[0].HTML = `<p>Lee <a href="https://example.com/news">las noticias</a>.</p>`
	want.Blocks[3].Label = "Abrir"
	if got := d.Localized("es"); !bytes.Equal(Marshal(got), Marshal(want)) {
		t.Fatalf("es version:\n got %s\nwant %s", Marshal(got), Marshal(want))
	}
	if len(d.Translations) != 2 || d.Blocks[0].Text != `Hello {{ contact.first_name | "there" }}` {
		t.Fatal("Localized changed the document it was called on")
	}
	ar := d.Localized("ar")
	if ar.Theme.Direction != "rtl" || ar.Preheader != "Our news" || ar.Blocks[3].Label != "Open" {
		t.Fatalf("ar version: %s", Marshal(ar))
	}
	if m := d.Missing("ar"); !slices.Equal(m, []string{"i", "t", "b"}) {
		t.Fatalf("missing in ar: %v", m)
	}
	if m := d.Missing("es"); len(m) != 0 {
		t.Fatalf("missing in es: %v", m)
	}
	is := d.VersionIssues("ar")
	if len(is) != 4 || is[0].Code != "translation_missing" || is[0].Field != "translations.ar.blocks.i" || is[3].Code != "footer_language" || is[3].Detail != "ar" {
		t.Fatalf("ar issues: %v", is)
	}
}

func TestTranslationIssues(t *testing.T) {
	cases := map[string]struct{ translations, code, field string }{
		"unknown block": {`"es": {"blocks": {"zz": "Hola"}}`, "translation_block", "translations.es.blocks.zz"},
		"no text":       {`"es": {"blocks": {"d": "Hola"}}`, "translation_block", "translations.es.blocks.d"},
		"base locale":   {`"en": {"blocks": {}}`, "invalid_locale", "translations.en"},
		"bad tag":       {`"e s": {"blocks": {}}`, "invalid_locale", "translations.e s"},
		"bad html":      {`"es": {"blocks": {"t": "<script>x</script>"}}`, "invalid_rich_text", "translations.es.blocks.t"},
		"long button":   {`"es": {"blocks": {"b": "` + strings.Repeat("a", 81) + `"}}`, "too_long", "translations.es.blocks.b"},
	}
	for name, c := range cases {
		raw := strings.Replace(versioned, versioned[strings.Index(versioned, `"translations"`):], `"translations": {`+c.translations+`}}`, 1)
		d, issues, err := Parse([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		issues = append(issues, Validate(d, Options{})...)
		found := false
		for _, i := range issues {
			found = found || (i.Code == c.code && i.Field == c.field)
		}
		if !found {
			t.Errorf("%s: want %s at %s, got %v", name, c.code, c.field, issues)
		}
	}
	noBase := strings.Replace(versioned, `"locale": "en", `, "", 1)
	d, _, err := Parse([]byte(noBase))
	if err != nil {
		t.Fatal(err)
	}
	if is := Validate(d, Options{}); len(is) == 0 || is[0].Code != "locale_required" {
		t.Errorf("translations without a locale: %v", is)
	}
}

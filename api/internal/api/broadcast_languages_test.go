package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"testing"

	"github.com/productdevbook/yuva/api/internal/api"
)

func versionedDoc() map[string]any {
	doc := designDoc(
		map[string]any{"id": "h1", "type": "heading", "level": 1, "text": "Hello {{ contact.first_name | \"there\" }}"},
		map[string]any{"id": "t1", "type": "text", "html": `<p>Read <a href="https://acme.example/news">the news</a>.</p>`},
		map[string]any{"id": "b1", "type": "button", "label": "Open", "url": "https://acme.example/open"},
	)
	doc["locale"] = "en"
	doc["translations"] = map[string]any{
		"es": map[string]any{"blocks": map[string]any{"h1": "Hola {{ contact.first_name | \"amigo\" }}", "t1": `<p>Lee <a href="https://acme.example/news">las noticias</a>.</p>`, "b1": "Abrir"}},
		"pt": map[string]any{"blocks": map[string]any{"h1": "Olá", "t1": `<p>Leia <a href="https://acme.example/news">as notícias</a>.</p>`, "b1": "Abrir"}},
		"tr": map[string]any{"blocks": map[string]any{"h1": "Merhaba", "t1": `<p><a href="https://acme.example/news">Haberleri</a> okuyun.</p>`, "b1": "Aç"}},
	}
	return doc
}

func hasIssue(body map[string]any, key, code string) bool {
	items, _ := body[key].([]any)
	for _, it := range items {
		if it.(map[string]any)["code"] == code {
			return true
		}
	}
	return false
}

func TestDesignLanguages(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	o := bt.owner

	tpl := o.expect(http.StatusCreated, "POST", "/v1/email-designs", map[string]any{"name": "Three", "template": map[string]any{"slug": "newsletter", "locale": "en", "locales": []string{"tr", "de"}}})
	doc := tpl.body["document"].(map[string]any)
	if tr := doc["translations"].(map[string]any); doc["locale"] != "en" || len(tr) != 2 || tr["tr"] == nil || tr["de"] == nil || hasIssue(tpl.body, "warnings", "translation_missing") {
		t.Fatalf("newsletter in three languages: %s", tpl.raw)
	}
	o.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/email-designs", map[string]any{"name": "Same", "template": map[string]any{"slug": "newsletter", "locale": "en", "locales": []string{"en"}}})

	path := "/v1/email-designs/" + bt.design
	d := o.expect(http.StatusOK, "PUT", path, map[string]any{"name": "News", "document": versionedDoc(), "base_version": 1})
	if !hasIssue(d.body, "warnings", "footer_language") {
		t.Errorf("es and pt versions get the English footer: %s", d.raw)
	}
	plain := versionedDoc()
	delete(plain, "translations")
	plain["blocks"].([]map[string]any)[0]["text"] = "Hello again {{ contact.first_name | \"there\" }}"
	d = o.expect(http.StatusOK, "PUT", path, map[string]any{"name": "News", "document": plain, "base_version": 2})
	if tr, _ := d.body["document"].(map[string]any)["translations"].(map[string]any); len(tr) != 3 {
		t.Fatalf("a PUT without translations dropped them: %s", d.raw)
	}
	plain["translations"] = map[string]any{}
	d = o.expect(http.StatusOK, "PUT", path, map[string]any{"name": "News", "document": plain, "base_version": 3})
	if _, ok := d.body["document"].(map[string]any)["translations"]; ok {
		t.Fatalf("translations {} kept them: %s", d.raw)
	}
	bad := versionedDoc()
	bad["translations"].(map[string]any)["es"].(map[string]any)["blocks"].(map[string]any)["zz"] = "Nada"
	if r := o.expect(http.StatusUnprocessableEntity, "PUT", path, map[string]any{"name": "News", "document": bad, "base_version": 4}); !strings.Contains(string(r.raw), `"code":"translation_block"`) {
		t.Fatalf("an entry for an unknown block: %s", r.raw)
	}
	o.expect(http.StatusOK, "PUT", path, map[string]any{"name": "News", "document": versionedDoc(), "base_version": 4})

	render := func(locale string) response {
		return o.expect(http.StatusOK, "POST", "/v1/email-designs/render", map[string]any{"document": versionedDoc(), "list_id": bt.list, "locale": locale})
	}
	es := render("es-MX")
	if html := es.str("html"); es.body["locale"] != "es" || !strings.Contains(html, `lang="es"`) || !strings.Contains(html, "Hola Ada") || !strings.Contains(html, "Unsubscribe") || !hasIssue(es.body, "warnings", "footer_language") {
		t.Fatalf("render es: %v %v %s", es.body["locale"], es.body["warnings"], es.body["text"])
	}
	tr := render("tr")
	if html := tr.str("html"); tr.body["locale"] != "tr" || !strings.Contains(html, `lang="tr"`) || !strings.Contains(html, "Abonelikten çık") || hasIssue(tr.body, "warnings", "footer_language") {
		t.Fatalf("render tr: %v %v", tr.body["locale"], tr.body["warnings"])
	}
	if fr := render("fr"); fr.body["locale"] != "en" || !strings.Contains(fr.str("html"), "Hello Ada") {
		t.Fatalf("render fr falls back to en: %v", fr.body["locale"])
	}
	o.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/email-designs/render", map[string]any{"document": versionedDoc(), "locale": "not a tag"})

	cs := h.mcpSession(h.oauthToken(o, bt.ws, "Claude Code", testOrigin+"/mcp", []string{"broadcasts:read", "broadcasts:write"}))
	got, errText := callTool(t, cs, "update_design", map[string]any{"design_id": bt.design, "base_version": 5, "locale": "pt", "texts": map[string]any{"b1": "", "h1": "Olá {{ contact.first_name | \"amigo\" }}"}})
	if errText != "" {
		t.Fatal(errText)
	}
	if got["locales"].([]any)[0] != "en" || got["missing_texts"].(map[string]any)["pt"].([]any)[0] != "b1" {
		t.Fatalf("MCP update of the pt version: %v %v", got["locales"], got["missing_texts"])
	}
	if _, errText := callTool(t, cs, "update_design", map[string]any{"design_id": bt.design, "base_version": 6, "locale": "pt", "document": plain}); !strings.Contains(errText, "leave it out") {
		t.Fatalf("document with another version: %q", errText)
	}
	got, errText = callTool(t, cs, "update_design", map[string]any{"design_id": bt.design, "base_version": 6, "locale": "tr", "remove_locale": true})
	if errText != "" || len(got["locales"].([]any)) != 3 {
		t.Fatalf("remove tr: %v %q", got["locales"], errText)
	}
	got, errText = callTool(t, cs, "preview_design", map[string]any{"design_id": bt.design, "list_id": bt.list, "locale": "pt-PT"})
	if errText != "" || got["locale"] != "pt" || !strings.Contains(got["text"].(string), "Olá") {
		t.Fatalf("MCP preview of pt: %v %q", got, errText)
	}

	if _, errText := callTool(t, cs, "update_design", map[string]any{"design_id": bt.design, "base_version": 7, "fallback_locale": "es"}); errText != "" {
		t.Fatal(errText)
	}
	for i, tag := range []string{"es", "pt"} {
		if got, errText = callTool(t, cs, "update_design", map[string]any{"design_id": bt.design, "base_version": 8 + i, "locale": tag, "remove_locale": true}); errText != "" {
			t.Fatalf("remove %s: %q", tag, errText)
		}
	}
	if _, ok := got["locales"]; ok {
		t.Fatalf("locales after removing every version: %v", got["locales"])
	}
	single := versionedDoc()
	single["translations"] = map[string]any{}
	single["fallback_locale"] = "en"
	o.expect(http.StatusOK, "PUT", path, map[string]any{"name": "News", "document": single, "base_version": 10})

	other := newBroadcastTeam(t, h, bs)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", path, map[string]any{"name": "News", "document": versionedDoc(), "base_version": 11})
}

func TestBroadcastLanguages(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	o := bt.owner
	addr := func(tag string) string { return "success+" + tag + "@simulator.amazonses.com" }
	for _, tag := range []string{"pt", "es-MX", "fr", "tr"} {
		o.expect(http.StatusCreated, "POST", "/v1/lists/"+bt.list+"/subscriptions", map[string]any{
			"email": addr(strings.ToLower(tag)), "locale": tag, "consent": map[string]any{"confirmed": true},
		})
	}
	bt.subscribe(t, addr("none"))
	o.expect(http.StatusOK, "PUT", "/v1/email-designs/"+bt.design, map[string]any{"name": "News", "document": versionedDoc(), "base_version": 1})

	o.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/broadcasts", map[string]any{"inbox_id": bt.chatInbox, "name": "Bad", "translations": map[string]any{"e s": map[string]any{"subject": "x"}}})
	for i := 0; i < 5; i++ {
		o.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/broadcasts", map[string]any{"inbox_id": bt.chatInbox, "name": "Twice", "translations": map[string]any{"es": map[string]any{}, "ES": map[string]any{"subject": "X"}}})
	}
	id := bt.draft(t, map[string]any{"click_tracking": true, "translations": map[string]any{
		"es": map[string]any{"subject": "Noticias para {{ contact.first_name | \"ti\" }}"},
		"pt": map[string]any{"subject": "Notícias", "preheader": "Novidades"},
	}})
	path := "/v1/broadcasts/" + id
	it := reviewItem(o.expect(http.StatusOK, "GET", path+"/review", nil).body, "translations")
	if it["status"] != "warning" || !strings.Contains(it["detail"].(string), "tr") {
		t.Fatalf("review without a tr subject: %v", it)
	}
	counts := o.expect(http.StatusOK, "POST", path+"/audience", nil).body
	want := map[string][2]float64{"en": {2, 2}, "es": {1, 0}, "pt": {1, 0}, "tr": {1, 0}}
	byLocale := counts["by_locale"].([]any)
	if len(byLocale) != 4 || byLocale[0].(map[string]any)["locale"] != "en" {
		t.Fatalf("by_locale: %v", byLocale)
	}
	for _, l := range byLocale {
		l := l.(map[string]any)
		if w := want[l["locale"].(string)]; l["recipients"] != w[0] || l["fallback"] != w[1] {
			t.Errorf("by_locale %v, want %v", l, w)
		}
	}
	b := o.expect(http.StatusOK, "PATCH", path, map[string]any{"translations": map[string]any{
		"es": map[string]any{"subject": "Noticias para {{ contact.first_name | \"ti\" }}"},
		"pt": map[string]any{"subject": "Notícias", "preheader": "Novidades"},
		"tr": map[string]any{"subject": "Haberler"},
	}}).body
	if len(b["translations"].(map[string]any)) != 3 {
		t.Fatalf("translations after PATCH: %v", b["translations"])
	}
	if it := reviewItem(o.expect(http.StatusOK, "GET", path+"/review", nil).body, "translations"); it["detail"] != "the required footer is in English: es, pt" {
		t.Fatalf("review with every subject: %v", it)
	}

	bt.schedule(t, id)
	bt.run(t)
	o.expectProblem(http.StatusConflict, "not_draft", "PATCH", path, map[string]any{"translations": map[string]any{}})
	dec := new(mime.WordDecoder)
	subject := func(a string) string {
		ms := bs.to(a)
		if len(ms) != 1 {
			t.Fatalf("%s got %d mails", a, len(ms))
		}
		s, err := dec.DecodeHeader(ms[0].Header.Get("Subject"))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	for a, w := range map[string]string{addr("es-mx"): "Noticias para ti", addr("pt"): "Notícias", addr("tr"): "Haberler", addr("fr"): "News for you", addr("none"): "News for Reader"} {
		if s := subject(a); s != w {
			t.Errorf("subject to %s: %q, want %q", a, s, w)
		}
	}
	for a, w := range map[string]string{addr("es-mx"): "es", addr("fr"): "en", addr("none"): "en", addr("pt"): "pt"} {
		if d := deliveryOf(t, o, id, a); d["locale"] != w {
			t.Errorf("delivery to %s: locale %v, want %s", a, d["locale"], w)
		}
	}
	if ds := deliveriesOf(t, o, id, "&locale=es"); len(ds) != 1 || ds[0]["email"] != addr("es-mx") {
		t.Fatalf("deliveries of es: %v", ds)
	}

	var raw string
	for _, m := range bs.sent {
		if m.to[0] == addr("es-mx") {
			raw = strings.ReplaceAll(strings.ReplaceAll(string(m.raw), "=\r\n", ""), "=3D", "=")
		}
	}
	if !strings.Contains(raw, "Hola") || !strings.Contains(raw, `lang="es"`) {
		t.Fatal("the es-MX contact did not get the Spanish version")
	}
	click := regexp.MustCompile(`http://localhost:8080(/c/[A-Za-z0-9_-]+)`).FindStringSubmatch(raw)
	if click == nil {
		t.Fatal("no tracked link in the Spanish mail")
	}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := noFollow.Get(h.url + click[1])
	if err != nil || res.StatusCode != http.StatusFound {
		t.Fatalf("click: %v %v", res, err)
	}
	target := res.Header.Get("Location")

	es := o.expect(http.StatusOK, "GET", path+"/report?locale=es", nil).body
	if es["locale"] != "es" || es["counts"].(map[string]any)["clicked"] != float64(1) || es["counts"].(map[string]any)["recipients"] != float64(1) {
		t.Fatalf("report of es: %v %v", es["locale"], es["counts"])
	}
	en := o.expect(http.StatusOK, "GET", path+"/report?locale=en", nil).body
	if en["counts"].(map[string]any)["clicked"] != float64(0) || en["counts"].(map[string]any)["recipients"] != float64(2) {
		t.Fatalf("report of en: %v", en["counts"])
	}
	whole := o.expect(http.StatusOK, "GET", path+"/report", nil).body
	if _, ok := whole["locale"]; ok || len(whole["locales"].([]any)) != 4 {
		t.Fatalf("whole report: %v %v", whole["locale"], whole["locales"])
	}
	rows := 0
	for _, l := range whole["links"].([]any) {
		l := l.(map[string]any)
		if l["url"] == target {
			rows++
			if l["clicks"] != float64(1) {
				t.Errorf("clicks on %s in the whole report: %v", target, l["clicks"])
			}
		}
	}
	if rows != 1 {
		t.Fatalf("%s has %d rows in the whole report: %v", target, rows, whole["links"])
	}
	o.expectProblem(http.StatusBadRequest, "validation_failed", "GET", path+"/report?locale=de", nil)
	o.expectProblem(http.StatusBadRequest, "validation_failed", "GET", path+"/report?locale=not-a-tag!", nil)

	cs := h.mcpSession(h.oauthToken(o, bt.ws, "Claude Code", testOrigin+"/mcp", []string{"broadcasts:read", "broadcasts:write"}))
	got, errText := callTool(t, cs, "get_broadcast_report", map[string]any{"broadcast_id": id, "locale": "es"})
	if errText != "" || got["locale"] != "es" || got["counts"].(map[string]any)["clicked"] != float64(1) {
		t.Fatalf("MCP report of es: %v %q", got, errText)
	}
	got, errText = callTool(t, cs, "get_broadcast", map[string]any{"broadcast_id": id})
	if errText != "" || len(got["broadcast"].(map[string]any)["translations"].([]any)) != 3 {
		t.Fatalf("MCP broadcast translations: %v %q", got, errText)
	}

	other := newBroadcastTeam(t, h, bs)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", path+"/report?locale=es", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", path+"/deliveries?locale=es", nil)
}

func TestBroadcastTestInALanguage(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	o := bt.owner
	bt.subscribe(t, "success@simulator.amazonses.com")
	o.expect(http.StatusOK, "PUT", "/v1/email-designs/"+bt.design, map[string]any{"name": "News", "document": versionedDoc(), "base_version": 1})
	id := bt.draft(t, map[string]any{"translations": map[string]any{"tr": map[string]any{"subject": strings.Repeat("Haber ", 15)}}})
	if it := reviewItem(o.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body, "subject"); it["status"] != "warning" || !strings.HasSuffix(it["detail"].(string), "(tr)") {
		t.Fatalf("review of a long tr subject: %v", it)
	}
	o.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+id, map[string]any{"translations": map[string]any{"tr": map[string]any{"subject": "Haberler"}}})
	r := o.expect(http.StatusAccepted, "POST", "/v1/broadcasts/"+id+"/test", map[string]any{"locale": "tr-TR"})
	if r.body["locale"] != "tr" {
		t.Fatalf("test in tr: %s", r.raw)
	}
	o.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/broadcasts/"+id+"/test", map[string]any{"locale": "not a tag"})
	for _, j := range h.jobs("broadcast_test", bt.ws) {
		var a api.BroadcastTestArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			t.Fatal(err)
		}
		if err := h.srv.SendBroadcastTestMail(context.Background(), a, false); err != nil {
			t.Fatal(err)
		}
		h.dropJob(j.id)
	}
	if bs.count() != 1 {
		t.Fatalf("test sends: %d", bs.count())
	}
	m, _ := mail.ReadMessage(bytes.NewReader(bs.sent[0].raw))
	raw := strings.ReplaceAll(strings.ReplaceAll(string(bs.sent[0].raw), "=\r\n", ""), "=3D", "=")
	if m.Header.Get("Subject") != "[Test] Haberler" || !strings.Contains(raw, `lang="tr"`) {
		t.Fatalf("test mail in tr: %q", m.Header.Get("Subject"))
	}
}

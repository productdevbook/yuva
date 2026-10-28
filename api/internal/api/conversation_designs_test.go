package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jhillyerd/enmime/v2"
)

func conversationDoc() map[string]any {
	doc := designDoc(
		map[string]any{"id": "h1", "type": "heading", "level": 1, "text": "Hello {{ contact.first_name | \"there\" }}"},
		map[string]any{"id": "m1", "type": "message"},
		map[string]any{"id": "t1", "type": "text", "html": `<p>Read <a href="https://acme.example/news">the news</a>.</p>`},
	)
	doc["locale"] = "en"
	doc["translations"] = map[string]any{
		"es": map[string]any{"blocks": map[string]any{"h1": "Hola {{ contact.first_name | \"amigo\" }}", "t1": `<p>Lee <a href="https://acme.example/news">las noticias</a>.</p>`}},
	}
	return doc
}

func offeredDesign(t *testing.T, c *client, name string, inbox any) string {
	t.Helper()
	body := map[string]any{"name": name, "document": conversationDoc(), "for_conversations": true}
	if inbox != nil {
		body["inbox_id"] = inbox
	}
	r := c.expect(http.StatusCreated, "POST", "/v1/email-designs", body)
	if r.body["for_conversations"] != true {
		t.Fatalf("design not offered: %s", r.raw)
	}
	return r.str("id")
}

func sentParts(t *testing.T, h *harness) (*enmime.Envelope, map[string][]string) {
	t.Helper()
	sent, parsed := h.smtp.last(t)
	env, err := enmime.ReadEnvelope(bytes.NewReader(sent.raw))
	if err != nil {
		t.Fatal(err)
	}
	return env, parsed.Header
}

func TestDesignedReply(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	design := offeredDesign(t, et.owner, "Branded reply", et.inbox)

	first := newMessageID()
	r := h.ingest(et.address, buildMail(mailOpts{from: "rosa@example.net", to: et.address, subject: "Pedido", messageID: first, body: "¿Dónde está mi pedido?"}), nil)
	conv := r.str("conversation_id")
	if _, err := h.st.Pool.Exec(context.Background(), `UPDATE contacts SET locale = 'es-MX' WHERE workspace_id = $1 AND id = (SELECT contact_id FROM conversations WHERE id = $2)`, et.ws, conv); err != nil {
		t.Fatal(err)
	}

	preview := et.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/design-preview", map[string]any{"design_id": design})
	if preview.body["footer"] != "reply" || preview.body["locale"] != "es" || preview.body["subject"] != "Re: Pedido" || !strings.Contains(preview.str("html"), "Lee") {
		t.Fatalf("preview: %s", preview.raw)
	}

	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
		"kind": "message", "body": "Ya salió.\n\nLlega el lunes.", "design_id": design,
	})
	d := reply.body["design"].(map[string]any)
	if d["id"] != design || d["locale"] != "es" || d["version"] == nil || d["name"] != "Branded reply" || reply.str("body") != "Ya salió.\n\nLlega el lunes." {
		t.Fatalf("designed reply: %s", reply.raw)
	}
	h.sendQueued(t, et.ws, reply.str("id"))
	env, hdr := sentParts(t, h)
	if !strings.Contains(env.HTML, "Hola") || !strings.Contains(env.HTML, "Lee") || !strings.Contains(env.HTML, "<p") || !strings.Contains(env.HTML, "Llega el lunes.") {
		t.Fatalf("HTML part: %s", env.HTML)
	}
	if !strings.Contains(env.Text, "Ya salió.") || !strings.Contains(env.Text, "Lee las noticias") || !strings.Contains(env.Text, "Hola") {
		t.Fatalf("text part: %s", env.Text)
	}
	if hdr["In-Reply-To"][0] != "<"+first+">" || hdr["References"][0] != "<"+first+">" || len(hdr["List-Unsubscribe"]) != 0 || len(hdr["List-Id"]) != 0 {
		t.Fatalf("headers: %v", hdr)
	}
	if strings.Contains(env.HTML, "/m/") {
		t.Fatal("a reply carries an unsubscribe link")
	}
	mail := et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+reply.str("id")+"/email", nil)
	if !strings.Contains(mail.str("original_html"), "Llega el lunes.") || !strings.Contains(mail.str("full_text"), "Lee las noticias") {
		t.Fatalf("stored parts: %s", mail.raw)
	}

	draft := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
		"kind": "message", "body": "Más tarde.", "draft": true, "design_id": design, "design_locale": "en",
	})
	if dd := draft.body["design"].(map[string]any); dd["locale"] != "en" || dd["version"] != nil {
		t.Fatalf("draft design: %s", draft.raw)
	}
	another := offeredDesign(t, et.owner, "Another look", et.inbox)
	moved := et.owner.expect(http.StatusOK, "PATCH", "/v1/messages/"+draft.str("id"), map[string]any{"body": "Más tarde.", "design_id": another})
	if md := moved.body["design"].(map[string]any); md["id"] != another || md["locale"] != nil {
		t.Fatalf("another design kept the old design_locale: %s", moved.raw)
	}
	et.owner.expect(http.StatusOK, "PATCH", "/v1/messages/"+draft.str("id"), map[string]any{"body": "Más tarde.", "design_id": design, "design_locale": "en"})
	cur := et.owner.expect(http.StatusOK, "GET", "/v1/email-designs/"+design, nil)
	doc := conversationDoc()
	doc["blocks"].([]map[string]any)[0]["text"] = "Welcome back"
	updated := et.owner.expect(http.StatusOK, "PUT", "/v1/email-designs/"+design, map[string]any{
		"name": "Branded reply", "inbox_id": et.inbox, "document": doc, "base_version": cur.body["version"],
	})
	sent := et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+draft.str("id")+"/send", nil)
	if sd := sent.body["design"].(map[string]any); sd["version"] != updated.body["version"] || sd["locale"] != "en" {
		t.Fatalf("draft sent with version %v, design is at %v: %s", sd["version"], updated.body["version"], sent.raw)
	}
	h.sendQueued(t, et.ws, draft.str("id"))
	if env, _ := sentParts(t, h); !strings.Contains(env.HTML, "Welcome back") {
		t.Fatalf("draft not rendered at send time: %s", env.HTML)
	}

	et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+reply.str("id")+"/redact", nil)
	var text string
	var html *string
	if err := h.st.Pool.QueryRow(context.Background(), `SELECT full_text, full_html FROM message_emails WHERE workspace_id = $1 AND message_id = $2`, et.ws, reply.str("id")).Scan(&text, &html); err != nil || text != "" || html != nil {
		t.Fatalf("redaction kept the parts: %q %v %v", text, html, err)
	}

	et.owner.expectProblem(http.StatusBadRequest, "design_not_emailed", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "x", "design_id": design})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x", "html": "<p>x</p>", "design_id": design})
	et.owner.expectProblem(http.StatusBadRequest, "design_locale_unknown", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x", "design_id": design, "design_locale": "fr"})
	plain := et.owner.expect(http.StatusCreated, "POST", "/v1/email-designs", map[string]any{"name": "Newsletter", "document": designDoc(map[string]any{"id": "x1", "type": "heading", "level": 1, "text": "News"})})
	et.owner.expectProblem(http.StatusConflict, "design_unavailable", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x", "design_id": plain.str("id")})
	et.owner.expectProblem(http.StatusUnprocessableEntity, "invalid_design", "PUT", "/v1/email-designs/"+plain.str("id"), map[string]any{
		"name": "Newsletter", "document": designDoc(map[string]any{"id": "x1", "type": "heading", "level": 1, "text": "News"}), "base_version": plain.body["version"], "for_conversations": true,
	})
	listDoc := conversationDoc()
	listDoc["blocks"] = append(listDoc["blocks"].([]map[string]any), map[string]any{"id": "l1", "type": "text", "html": "<p>{{ list.name }}</p>"})
	listy := et.owner.expect(http.StatusCreated, "POST", "/v1/email-designs", map[string]any{"name": "Listy", "document": listDoc, "for_conversations": true})
	r2 := et.owner.expect(http.StatusUnprocessableEntity, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x", "design_id": listy.str("id")})
	if !strings.Contains(string(r2.raw), "merge_field_unavailable") {
		t.Fatalf("list.name in conversation mail: %s", r2.raw)
	}

	other := newEmailTeam(t, h, false)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+conv+"/design-preview", map[string]any{"design_id": design})
	oc := other.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": other.inbox, "contact_id": other.contact, "channel_id": other.channel, "subject": "Hi"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+oc.str("id")+"/messages", map[string]any{"kind": "message", "body": "x", "design_id": design})
}

func withSender(et emailTeam) {
	et.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+et.inbox, map[string]any{
		"sender": map[string]any{"legal_name": "Acme Ltd", "postal_address": "1 Main Street, Springfield"},
	})
}

func TestDesignedOutreach(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	design := offeredDesign(t, et.owner, "Hello", nil)
	start := func(contact string) string {
		return et.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": et.inbox, "contact_id": contact, "channel_id": et.channel, "subject": "News for you"}).str("id")
	}
	first := start(et.contact)
	et.owner.expectProblem(http.StatusConflict, "sender_identity_missing", "POST", "/v1/conversations/"+first+"/messages", map[string]any{"kind": "message", "body": "Hi", "design_id": design})
	et.owner.expectProblem(http.StatusConflict, "sender_identity_missing", "POST", "/v1/conversations/"+first+"/messages", map[string]any{"kind": "message", "body": "Hi", "draft": true, "design_id": design})
	withSender(et)

	blocked := et.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{"name": "Bea", "emails": []string{"bea@example.org"}}).str("id")
	if _, err := h.st.Pool.Exec(context.Background(), `INSERT INTO email_suppressions (workspace_id, email, reason, scope, created_at) VALUES ($1, 'bea@example.org', 'complaint', 'broadcasts', now())`, et.ws); err != nil {
		t.Fatal(err)
	}
	bc := start(blocked)
	et.owner.expectProblem(http.StatusConflict, "email_opted_out", "POST", "/v1/conversations/"+bc+"/messages", map[string]any{"kind": "message", "body": "Hi", "design_id": design})
	et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+bc+"/messages", map[string]any{"kind": "message", "body": "Hi"})

	conv := start(et.contact)
	preview := et.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/design-preview", map[string]any{"design_id": design, "body": "Hi"})
	if preview.body["footer"] != "outreach" || preview.body["locale"] != "en" {
		t.Fatalf("outreach preview: %s", preview.raw)
	}
	out := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "We have news.", "design_id": design})
	h.sendQueued(t, et.ws, out.str("id"))
	env, hdr := sentParts(t, h)
	if hdr["List-Unsubscribe-Post"][0] != "List-Unsubscribe=One-Click" || len(hdr["List-Id"]) != 0 || len(hdr["Precedence"]) != 0 {
		t.Fatalf("outreach headers: %v", hdr)
	}
	unsub := strings.Trim(hdr["List-Unsubscribe"][0], "<>")
	path := strings.TrimPrefix(unsub, "http://localhost:8080")
	if !strings.HasPrefix(path, "/m/") || !strings.Contains(env.HTML, "/m/") {
		t.Fatalf("unsubscribe link %q, html: %s", unsub, env.HTML)
	}
	if st, page := pageAt(t, h, "GET", path, nil); st != http.StatusOK || !strings.Contains(page, "Stop mail like this from") {
		t.Fatalf("outreach page: %d %s", st, page)
	}
	var n int
	if err := h.st.Pool.QueryRow(context.Background(), `SELECT count(*) FROM inbox_mail_optouts WHERE workspace_id = $1`, et.ws).Scan(&n); err != nil || n != 0 {
		t.Fatalf("opening the page wrote %d opt-outs (%v)", n, err)
	}
	optouts := func() int {
		var n int
		if err := h.st.Pool.QueryRow(context.Background(), `SELECT count(*) FROM inbox_mail_optouts WHERE workspace_id = $1`, et.ws).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if st, page := pageAt(t, h, "POST", path, url.Values{"action": {"all"}}); st != http.StatusOK || optouts() != 1 || !strings.Contains(page, `name="stop" value="undo"`) {
		t.Fatalf("unsubscribe from everything: %d, %d opt-outs, %s", st, optouts(), page)
	}
	if st, _ := pageAt(t, h, "POST", path, url.Values{"action": {"undo"}, "stop": {"undo"}}); st != http.StatusOK || optouts() != 0 {
		t.Fatalf("undo kept the opt-out: %d, %d opt-outs", st, optouts())
	}
	if st, _ := pageAt(t, h, "POST", path, url.Values{"List-Unsubscribe": {"One-Click"}}); st != http.StatusOK {
		t.Fatalf("one-click: %d", st)
	}
	var source string
	if err := h.st.Pool.QueryRow(context.Background(), `SELECT source FROM inbox_mail_optouts WHERE workspace_id = $1 AND inbox_id = $2 AND contact_id = $3`, et.ws, et.inbox, et.contact).Scan(&source); err != nil || source != "outreach" {
		t.Fatalf("one-click opt-out: %q %v", source, err)
	}
	et.owner.expectProblem(http.StatusConflict, "email_opted_out", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Again", "design_id": design})
	et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Plain still goes"})
	et.owner.expectProblem(http.StatusConflict, "email_opted_out", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Draft", "draft": true, "design_id": design})
}

func TestDesignedOutreachLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	withSender(et)
	design := offeredDesign(t, et.owner, "Hello", nil)
	conv := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": et.inbox, "contact_id": et.contact, "channel_id": et.channel, "subject": "Hi"}).str("id")
	for range 20 {
		et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Hi", "design_id": design})
	}
	et.owner.expectProblem(http.StatusTooManyRequests, "rate_limited", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Hi", "design_id": design})
	et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Plain"})
}

func TestConversationDesignsForAgents(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	et.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+et.inbox+"/members/"+et.agentID, nil)
	other := et.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Billing", "slug": "billing"}).body["inbox"].(map[string]any)["id"]
	mine := offeredDesign(t, et.owner, "Support look", et.inbox)
	shared := offeredDesign(t, et.owner, "Company look", nil)
	billing := offeredDesign(t, et.owner, "Billing look", other)
	hidden := et.owner.expect(http.StatusCreated, "POST", "/v1/email-designs", map[string]any{"name": "Newsletter", "document": conversationDoc()}).str("id")

	got := map[string]bool{}
	for _, it := range items(et.agent.expect(http.StatusOK, "GET", "/v1/email-designs?for_conversations=true", nil)) {
		got[it["id"].(string)] = true
	}
	if len(got) != 2 || !got[mine] || !got[shared] {
		t.Fatalf("agent sees %v", got)
	}
	et.agent.expectProblem(http.StatusForbidden, "forbidden", "GET", "/v1/email-designs", nil)
	et.agent.expect(http.StatusOK, "GET", "/v1/email-designs/"+mine, nil)
	et.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/email-designs/"+hidden, nil)
	et.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/email-designs/"+billing, nil)

	r := et.owner.expect(http.StatusUnprocessableEntity, "POST", "/v1/email-designs", map[string]any{"name": "No slot", "document": designDoc(), "for_conversations": true})
	if !strings.Contains(string(r.raw), "message_block_missing") {
		t.Fatalf("offered without a message block: %s", r.raw)
	}
}

func TestBroadcastReviewRefusesMessageBlock(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "success@simulator.amazonses.com")
	bt.owner.expect(http.StatusOK, "PUT", "/v1/email-designs/"+bt.design, map[string]any{"name": "News", "document": conversationDoc(), "base_version": 1})
	id := bt.draft(t, nil)
	if it := reviewItem(bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body, "message_block"); it == nil || it["status"] != "blocking" {
		t.Fatalf("review of a design with a message block: %v", it)
	}
	bt.owner.expectProblem(http.StatusConflict, "review_failed", "POST", "/v1/broadcasts/"+id+"/test", nil)
}

func TestMCPStartConversationWithDesign(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	withSender(et)
	design := offeredDesign(t, et.owner, "Hello", et.inbox)
	scopes := []string{"conversations:read", "conversations:write", "messages:write", "contacts:read"}
	cs := h.mcpSession(h.oauthToken(et.owner, et.ws, "Claude Code", testOrigin+"/mcp", scopes))
	listed, errText := callTool(t, cs, "list_designs", map[string]any{"for_conversations": true})
	if errText != "" || len(listed["items"].([]any)) != 1 {
		t.Fatalf("list_designs: %v %q", listed, errText)
	}
	out, errText := callTool(t, cs, "start_conversation", map[string]any{"inbox_id": et.inbox, "contact_id": et.contact, "channel_id": et.channel, "subject": "Welcome", "body": "Hello Ayşe", "design_id": design})
	if errText != "" {
		t.Fatal(errText)
	}
	msg := out["message"].(map[string]any)
	if msg["draft"] != true || msg["design"].(map[string]any)["id"] != design {
		t.Fatalf("start_conversation with a design: %v", out)
	}
	var stored string
	if err := h.st.Pool.QueryRow(context.Background(), `SELECT email_design_id::text FROM messages WHERE workspace_id = $1 AND id = $2`, et.ws, msg["id"]).Scan(&stored); err != nil || stored != design {
		t.Fatalf("draft's design %q %v", stored, err)
	}
	pv, errText := callTool(t, cs, "preview_message_design", map[string]any{"conversation_id": out["conversation"].(map[string]any)["id"], "design_id": design, "body": "Hi"})
	if errText != "" || pv["footer"] != "outreach" || pv["html_uri"] != nil || !strings.Contains(pv["text"].(string), "Hi") {
		t.Fatalf("preview_message_design: %v %q", pv, errText)
	}
}

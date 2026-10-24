package api_test

import (
	"context"
	"image/color"
	"net/http"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/store"
)

func (h *harness) sentParts(t *testing.T) (text, html string) {
	t.Helper()
	sent, _ := h.smtp.last(t)
	m, err := email.Parse(sent.raw)
	if err != nil {
		t.Fatal(err)
	}
	return m.Text, m.HTML
}

func mySignature(c *client, inbox string) map[string]any {
	for _, it := range c.expect(http.StatusOK, "GET", "/v1/me/signatures", nil).body["items"].([]any) {
		if s := it.(map[string]any); s["inbox_id"] == inbox {
			return s
		}
	}
	return nil
}

func TestMemberTitle(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	owner := tm.ownerMemberID(t)

	r := tm.agent.expect(http.StatusOK, "PATCH", "/v1/members/"+tm.agentID, map[string]any{"title": "  Support lead "})
	if r.str("title") != "Support lead" || r.str("role") != "agent" {
		t.Fatalf("own title: %s", r.raw)
	}
	if got := tm.owner.expect(http.StatusOK, "GET", "/v1/members/"+tm.agentID, nil).str("title"); got != "Support lead" {
		t.Fatalf("title read back %q", got)
	}
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	found := false
	for _, it := range tm.owner.expect(http.StatusOK, "GET", "/v1/inboxes/"+tm.inbox+"/members", nil).body["items"].([]any) {
		if m := it.(map[string]any); m["id"] == tm.agentID {
			found = m["title"] == "Support lead"
		}
	}
	if !found {
		t.Fatal("inbox members lack the title")
	}
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/members/"+owner, map[string]any{"title": "Boss"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/members/"+tm.agentID, map[string]any{"role": "admin"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/members/"+tm.agentID, map[string]any{"role": "agent", "title": "x"})
	tm.agent.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/members/"+tm.agentID, map[string]any{"title": strings.Repeat("a", 101)})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/members/"+tm.agentID, map[string]any{})

	r = tm.owner.expect(http.StatusOK, "PATCH", "/v1/members/"+tm.agentID, map[string]any{"title": ""})
	if r.str("title") != "" || r.str("role") != "agent" {
		t.Fatalf("owner clears the title: %s", r.raw)
	}
	r = tm.owner.expect(http.StatusOK, "PATCH", "/v1/members/"+tm.agentID, map[string]any{"role": "admin", "title": "Lead"})
	if r.str("title") != "Lead" || r.str("role") != "admin" {
		t.Fatalf("role and title: %s", r.raw)
	}
	tm.owner.expectProblem(http.StatusConflict, "last_owner", "PATCH", "/v1/members/"+owner, map[string]any{"role": "agent"})
}

func TestSignatures(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	o, a := et.owner, et.agent
	o.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+et.inbox+"/members/"+et.agentID, nil)
	a.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"name": "Deniz_Kaya"})
	a.expect(http.StatusOK, "PATCH", "/v1/members/"+et.agentID, map[string]any{"title": "Lead *support*"})

	logo := a.uploadImage("logo.png", "image/png", testPNG(t, 40, 20, color.RGBA{R: 200, A: 255}))
	if logo.status != http.StatusCreated {
		t.Fatalf("an agent uploads an image: %d %s", logo.status, logo.raw)
	}
	def := "{{member.name}}\n\n{{member.title}}\n\n![logo](" + logo.str("url") + ") ![x](https://tracker.example/x.png)"
	in := o.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+et.inbox, map[string]any{"signature": def})
	if in.str("signature") != def {
		t.Fatalf("inbox signature: %s", in.raw)
	}
	a.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/inboxes/"+et.inbox, map[string]any{"signature": "x"})
	o.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/inboxes/"+et.inbox, map[string]any{"signature": strings.Repeat("a", 4001)})

	sig := mySignature(a, et.inbox)
	html, _ := sig["html"].(string)
	if _, own := sig["body"]; own || !strings.Contains(html, "Deniz_Kaya") || strings.Contains(html, "<em>") ||
		!strings.Contains(html, `<img src="`+logo.str("url")+`"`) || strings.Contains(html, `src="https://tracker.example`) ||
		!strings.Contains(html, `href="https://tracker.example/x.png"`) {
		t.Fatalf("default signature for the agent: %v", sig)
	}
	if sig["text"] != "Deniz_Kaya\nLead *support*\nlogo x <https://tracker.example/x.png>" {
		t.Fatalf("signature text %q", sig["text"])
	}

	conv := h.ingest(et.address, buildMail(mailOpts{from: "sig@example.net", to: et.address, subject: "Invoice", messageID: newMessageID(), body: "Hello"}), nil).str("conversation_id")
	reply := a.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
		"kind": "message", "body": "Here it is.\n\n> Hello\n>\n> again",
	})
	if !strings.Contains(reply.str("signature_html"), "Deniz_Kaya") || strings.Contains(reply.str("body"), "Deniz") {
		t.Fatalf("reply signature: %s", reply.raw)
	}
	h.sendQueued(t, et.ws, reply.str("id"))
	text, htmlPart := h.sentParts(t)
	want := "Here it is.\n\n-- \nDeniz_Kaya\nLead *support*\nlogo x <https://tracker.example/x.png>\n\n> Hello\n>\n> again"
	if strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n")) != want {
		t.Fatalf("text part:\n%q\nwant\n%q", text, want)
	}
	if i, j, k := strings.Index(htmlPart, "Here it is."), strings.Index(htmlPart, `class="yuva-signature"`), strings.Index(htmlPart, "&gt; Hello"); i < 0 || j < i || k < j {
		t.Fatalf("html part order: %s", htmlPart)
	}
	if h.count("SELECT count(*) FROM email_images WHERE id = $1 AND sent_at IS NOT NULL", logo.str("id")) != 1 {
		t.Fatal("the signature's image is not marked sent")
	}

	plain := a.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "No signature.", "signature": false})
	if plain.body["signature_html"] != nil {
		t.Fatalf("signature: false still signed: %s", plain.raw)
	}
	h.sendQueued(t, et.ws, plain.str("id"))
	if text, htmlPart := h.sentParts(t); strings.Contains(text, "-- ") || strings.Contains(htmlPart, "yuva-signature") {
		t.Fatalf("unsigned mail carries a signature: %q %q", text, htmlPart)
	}
	note := a.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "internal"})
	if note.body["signature_html"] != nil {
		t.Fatalf("a note is signed: %s", note.raw)
	}

	draft := a.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Drafted.", "draft": true})
	if !strings.Contains(draft.str("signature_html"), "Deniz_Kaya") {
		t.Fatalf("draft snapshot: %s", draft.raw)
	}
	own := a.expect(http.StatusOK, "PUT", "/v1/me/signatures/"+et.inbox, map[string]any{"body": "Cheers, {{member.name}}"})
	if own.str("body") != "Cheers, {{member.name}}" || own.str("text") != "Cheers, Deniz_Kaya" {
		t.Fatalf("own signature: %s", own.raw)
	}
	sent := a.expect(http.StatusOK, "POST", "/v1/messages/"+draft.str("id")+"/send", nil)
	h.sendQueued(t, et.ws, sent.str("id"))
	if text, _ := h.sentParts(t); !strings.Contains(text, "Lead *support*") || strings.Contains(text, "Cheers") {
		t.Fatalf("the draft lost its snapshot: %q", text)
	}

	first := a.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "One."})
	second := a.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Two."})
	if err := h.srv.SendEmails(context.Background(), uuid.MustParse(et.ws), []uuid.UUID{uuid.MustParse(first.str("id")), uuid.MustParse(second.str("id"))}, false); err != nil {
		t.Fatal(err)
	}
	text, htmlPart = h.sentParts(t)
	if strings.Count(text, "Cheers, Deniz_Kaya") != 1 || strings.Count(htmlPart, "yuva-signature") != 1 || !strings.Contains(text, "One.\n\nTwo.") && !strings.Contains(text, "One.\r\n\r\nTwo.") {
		t.Fatalf("batch:\n%q\n%q", text, htmlPart)
	}
	first = a.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Signed."})
	second = a.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Unsigned.", "signature": false})
	if err := h.srv.SendEmails(context.Background(), uuid.MustParse(et.ws), []uuid.UUID{uuid.MustParse(first.str("id")), uuid.MustParse(second.str("id"))}, false); err != nil {
		t.Fatal(err)
	}
	if text, htmlPart = h.sentParts(t); strings.Contains(text, "-- ") || strings.Contains(htmlPart, "yuva-signature") {
		t.Fatalf("a batch ending unsigned went out signed:\n%q\n%q", text, htmlPart)
	}

	a.expect(http.StatusOK, "PUT", "/v1/me/signatures/"+et.inbox, map[string]any{"body": ""})
	if s := mySignature(a, et.inbox); s["body"] != "" || s["html"] != "" || s["text"] != "" {
		t.Fatalf("empty own signature: %v", s)
	}
	if r := a.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Bare."}); r.body["signature_html"] != nil {
		t.Fatalf("an empty own signature still signed: %s", r.raw)
	}
	a.expect(http.StatusNoContent, "DELETE", "/v1/me/signatures/"+et.inbox, nil)
	a.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/me/signatures/"+et.inbox, nil)
	if s := mySignature(a, et.inbox); s["body"] != nil || !strings.Contains(s["html"].(string), "Deniz_Kaya") {
		t.Fatalf("back to the default: %v", s)
	}

	api := o.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": et.inbox, "contact_id": et.contact, "subject": "API"}).str("id")
	if r := o.expect(http.StatusCreated, "POST", "/v1/conversations/"+api+"/messages", map[string]any{"kind": "message", "body": "Not mail."}); r.body["signature_html"] != nil {
		t.Fatalf("a reply that is not e-mailed is signed: %s", r.raw)
	}
	key := et.apiKey(h)
	if r := key.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Bot draft.", "draft": true}); r.body["signature_html"] != nil {
		t.Fatalf("an API key's reply is signed: %s", r.raw)
	}
	if r := key.do("GET", "/v1/me/signatures", nil); r.status != http.StatusForbidden {
		t.Fatalf("an API key reads signatures: %d %s", r.status, r.raw)
	}

	hidden := o.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Billing", "slug": "billing"}).body["inbox"].(map[string]any)["id"].(string)
	a.expectProblem(http.StatusNotFound, "not_found", "PUT", "/v1/me/signatures/"+hidden, map[string]any{"body": "x"})
	if mySignature(a, hidden) != nil || mySignature(o, hidden) == nil {
		t.Fatal("signatures list inboxes the member cannot see, or misses one they can")
	}
	other := newTeam(t, h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", "/v1/me/signatures/"+et.inbox, map[string]any{"body": "x"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/me/signatures/"+et.inbox, nil)
	if mySignature(other.owner, et.inbox) != nil {
		t.Fatal("another workspace lists this inbox's signature")
	}
}

func TestSignatureImagesAreKept(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	ws := uuid.MustParse(tm.ws)
	img := tm.agent.uploadImage("me.png", "image/png", testPNG(t, 30, 30, color.RGBA{G: 90, A: 255}))
	def := tm.owner.uploadImage("logo.png", "image/png", testPNG(t, 30, 30, color.RGBA{B: 90, A: 255}))
	tm.agent.expect(http.StatusOK, "PUT", "/v1/me/signatures/"+tm.inbox, map[string]any{"body": "![me](" + img.str("url") + ")"})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"signature": "![logo](" + def.str("url") + ")"})
	exists := func(id string) bool { return h.count("SELECT count(*) FROM email_images WHERE id = $1", id) == 1 }
	sweep := func() {
		t.Helper()
		if _, err := h.st.DeleteUnusedEmailImages(context.Background(), store.DeleteUnusedEmailImagesParams{WorkspaceID: ws, Before: time.Now().Add(time.Hour), Lim: 100}); err != nil {
			t.Fatal(err)
		}
	}
	sweep()
	if !exists(img.str("id")) || !exists(def.str("id")) {
		t.Fatal("the sweep deleted an image a signature uses")
	}
	r := tm.owner.expect(http.StatusConflict, "DELETE", "/v1/email-images/"+img.str("id")+"?force=true", nil)
	used, _ := r.body["used_by"].(map[string]any)
	mine, _ := used["member_signatures"].([]any)
	if r.str("code") != "image_in_use" || len(mine) != 1 || mine[0].(map[string]any)["member_id"] != tm.agentID || mine[0].(map[string]any)["inbox_id"] != tm.inbox {
		t.Fatalf("deleting a member signature's image: %s", r.raw)
	}
	r = tm.owner.expect(http.StatusConflict, "DELETE", "/v1/email-images/"+def.str("id")+"?force=true", nil)
	used, _ = r.body["used_by"].(map[string]any)
	inboxes, _ := used["inbox_signatures"].([]any)
	if r.str("code") != "image_in_use" || len(inboxes) != 1 || inboxes[0].(map[string]any)["id"] != tm.inbox {
		t.Fatalf("deleting an inbox signature's image: %s", r.raw)
	}
	tm.agent.expect(http.StatusNoContent, "DELETE", "/v1/me/signatures/"+tm.inbox, nil)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"signature": ""})
	sweep()
	if exists(img.str("id")) || exists(def.str("id")) {
		t.Fatal("images no signature uses were kept")
	}
}

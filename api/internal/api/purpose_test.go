package api_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

func mailInbox(c *client) string {
	c.h.t.Helper()
	r := c.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Personal mail", "slug": unique("mail"), "purpose": "mail"})
	in := r.body["inbox"].(map[string]any)
	if in["purpose"] != "mail" {
		c.h.t.Fatalf("created mail inbox: %s", r.raw)
	}
	return in["id"].(string)
}

func TestInboxPurposeRules(t *testing.T) {
	h := newHarness(t)
	ct := newChangelogTeam(t, h)
	other := newTeam(t, h)
	o := ct.owner

	if got := o.expect(http.StatusOK, "GET", "/v1/inboxes/"+ct.inbox, nil); got.body["purpose"] != "support" {
		t.Fatalf("default purpose: %s", got.raw)
	}
	o.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/inboxes", map[string]any{"name": "X", "slug": unique("x"), "purpose": "sales"})
	o.expectProblem(http.StatusConflict, "purpose_mismatch", "POST", "/v1/inboxes", map[string]any{"name": "X", "slug": unique("x"), "purpose": "mail", "assignment_mode": "round_robin"})
	mail := mailInbox(o)

	o.expectProblem(http.StatusConflict, "purpose_mismatch", "POST", "/v1/inboxes/"+mail+"/channels", map[string]any{
		"kind": "chat", "name": "Site", "chat": map[string]any{"allowed_origins": []string{"https://example.com"}},
	})
	o.expectProblem(http.StatusConflict, "purpose_mismatch", "POST", "/v1/inboxes/"+mail+"/channels", map[string]any{"kind": "app", "name": "App"})
	emailChannel := o.expect(http.StatusCreated, "POST", "/v1/inboxes/"+mail+"/channels", map[string]any{
		"kind": "email", "name": "Mail", "email": map[string]any{
			"address": unique("me") + "@example.com",
			"smtp":    map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"},
		},
	}).str("id")

	o.expectProblem(http.StatusConflict, "purpose_mismatch", "POST", "/v1/help-centers", map[string]any{"inbox_id": mail, "name": "Help", "slug": unique("help")})
	o.expectProblem(http.StatusConflict, "purpose_mismatch", "POST", "/v1/status-pages", map[string]any{"inbox_id": mail, "name": "Status", "slug": unique("st")})
	o.expectProblem(http.StatusConflict, "purpose_mismatch", "POST", "/v1/roadmap-boards", map[string]any{"inbox_id": mail, "channel_id": emailChannel, "name": "Roadmap", "slug": unique("rm")})
	o.expectProblem(http.StatusConflict, "purpose_mismatch", "POST", "/v1/changelogs", map[string]any{"inbox_id": mail, "name": "News", "slug": unique("news")})
	o.expectProblem(http.StatusConflict, "purpose_mismatch", "POST", "/v1/sla-policies", map[string]any{"name": "Mail", "inbox_ids": []string{mail}, "first_reply_minutes": 30})
	policy := o.expect(http.StatusCreated, "POST", "/v1/sla-policies", map[string]any{"name": "Support", "inbox_ids": []string{ct.inbox}, "first_reply_minutes": 30}).str("id")
	o.expectProblem(http.StatusConflict, "purpose_mismatch", "PATCH", "/v1/sla-policies/"+policy, map[string]any{"inbox_ids": []string{ct.inbox, mail}})
	o.expectProblem(http.StatusConflict, "purpose_mismatch", "PATCH", "/v1/inboxes/"+mail, map[string]any{"assignment_mode": "round_robin"})

	o.expectProblem(http.StatusConflict, "purpose_mismatch", "PATCH", "/v1/inboxes/"+ct.chatInbox, map[string]any{"purpose": "mail"})
	o.expectProblem(http.StatusConflict, "purpose_mismatch", "PATCH", "/v1/inboxes/"+ct.inbox, map[string]any{"purpose": "mail"})
	o.expect(http.StatusNoContent, "DELETE", "/v1/sla-policies/"+policy, nil)
	o.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+ct.inbox, map[string]any{"assignment_mode": "round_robin"})
	o.expectProblem(http.StatusConflict, "purpose_mismatch", "PATCH", "/v1/inboxes/"+ct.inbox, map[string]any{"purpose": "mail"})
	o.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+ct.inbox, map[string]any{"assignment_mode": "off"})
	if r := o.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+ct.inbox, map[string]any{"purpose": "mail"}); r.body["purpose"] != "mail" {
		t.Fatalf("became mail: %s", r.raw)
	}
	o.expectProblem(http.StatusConflict, "purpose_mismatch", "POST", "/v1/help-centers", map[string]any{"inbox_id": ct.inbox, "name": "Help", "slug": unique("help")})
	if r := o.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+ct.inbox, map[string]any{"purpose": "support"}); r.body["purpose"] != "support" {
		t.Fatalf("back to support: %s", r.raw)
	}
	o.expect(http.StatusCreated, "POST", "/v1/help-centers", map[string]any{"inbox_id": ct.inbox, "name": "Help", "slug": unique("help")})
	o.expectProblem(http.StatusConflict, "purpose_mismatch", "PATCH", "/v1/inboxes/"+ct.inbox, map[string]any{"purpose": "mail"})

	ct.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/inboxes/"+mail, map[string]any{"purpose": "support"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/inboxes/"+mail, map[string]any{"purpose": "support"})
	o.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/inboxes/"+mail, map[string]any{"purpose": "sales"})
}

func TestMailInboxLeftOutOfSupportFigures(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	key := tm.apiKey(h)
	mail := mailInbox(tm.owner)
	since := h.clock.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	open := func(inbox string) string {
		conv := key.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": inbox, "contact_id": tm.contact}).str("id")
		post(key, conv, map[string]any{"kind": "message", "direction": "in", "body": "hello"})
		return conv
	}
	tm.owner.expect(http.StatusCreated, "POST", "/v1/sla-policies", map[string]any{"name": "Everyone", "first_reply_minutes": 30})
	support, personal := open(tm.inbox), open(mail)
	if s := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+support, nil); s.body["sla"] == nil {
		t.Fatalf("support conversation without SLA: %s", s.raw)
	}
	if s := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+personal, nil); s.body["sla"] != nil {
		t.Fatalf("mail conversation with SLA: %s", s.raw)
	}
	h.clock.Advance(time.Minute)
	post(tm.owner, support, map[string]any{"kind": "message", "body": "on it"})
	post(tm.owner, personal, map[string]any{"kind": "message", "body": "see you friday"})

	stats := func(query string) map[string]any {
		return tm.owner.expect(http.StatusOK, "GET", "/v1/stats?since="+url.QueryEscape(since)+query, nil).body
	}
	if s := stats(""); s["replies"] != float64(1) || s["first_replies"] != float64(1) {
		t.Fatalf("workspace stats count mail: %v", s)
	}
	if s := stats("&inbox_id=" + mail); s["replies"] != float64(0) || s["first_replies"] != float64(0) {
		t.Fatalf("mail inbox stats: %v", s)
	}
	if in := homeBlock(homeOf(tm.owner), "inbox"); in["open"] != float64(1) {
		t.Fatalf("home counts mail: %v", in)
	}

	c := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+personal, nil)
	if c.body["inbox_purpose"] != "mail" {
		t.Fatalf("conversation of a mail inbox: %s", c.raw)
	}
	if got := itemIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/conversations?purpose=mail", nil)); !slices.Equal(got, []string{personal}) {
		t.Fatalf("purpose=mail lists %v", got)
	}
	if got := itemIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/conversations?purpose=support", nil)); !slices.Equal(got, []string{support}) {
		t.Fatalf("purpose=support lists %v", got)
	}
	if n := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts?purpose=mail", nil).body["all"]; n != float64(1) {
		t.Fatalf("mail counts %v", n)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?purpose=sales", nil)
}

func TestMoveIntoMailInbox(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	mail := mailInbox(ct.owner)
	cs := ct.session(h, map[string]any{})
	chat := cs.start("hi from the widget")
	ct.owner.expectProblem(http.StatusConflict, "purpose_mismatch", "POST", "/v1/conversations/"+chat+"/move", map[string]any{"inbox_id": mail})
	plain := ct.conversation(ct.owner)
	if r := ct.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+plain+"/move", map[string]any{"inbox_id": mail}); r.body["inbox_purpose"] != "mail" {
		t.Fatalf("moved: %s", r.raw)
	}
}

func TestMailBlockedByChatConversations(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	o := ct.owner
	cs := ct.session(h, map[string]any{})
	chat := cs.start("hi from the widget")
	plain := o.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Plain", "slug": unique("plain")}).body["inbox"].(map[string]any)["id"].(string)
	o.expect(http.StatusOK, "POST", "/v1/conversations/"+chat+"/move", map[string]any{"inbox_id": plain})

	refused := func(inbox, want string) string {
		t.Helper()
		r := o.do("PATCH", "/v1/inboxes/"+inbox, map[string]any{"purpose": "mail"})
		if r.status != http.StatusConflict || r.body["code"] != "purpose_mismatch" || !strings.Contains(r.body["detail"].(string), want) {
			t.Fatalf("PATCH %s to mail: %d %s", inbox, r.status, r.raw)
		}
		return r.body["detail"].(string)
	}
	refused(plain, "chat conversations")
	o.expect(http.StatusOK, "DELETE", "/v1/conversations/"+chat, nil)
	if r := o.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+plain, map[string]any{"purpose": "mail"}); r.body["purpose"] != "mail" {
		t.Fatalf("a trashed chat conversation still blocks: %s", r.raw)
	}

	cs.start("second question")
	o.expect(http.StatusNoContent, "DELETE", "/v1/channels/"+ct.channel, nil)
	if d := refused(ct.chatInbox, "chat conversations"); strings.Contains(d, "chat channel") {
		t.Fatalf("a deleted channel is still named: %s", d)
	}
}

func TestConversationStars(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	key := tm.apiKey(h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	conv, plain := tm.conversation(tm.owner), tm.conversation(tm.owner)
	path := "/v1/conversations/" + conv + "/star"
	after := lastEvent(tm.owner)

	tm.owner.expect(http.StatusNoContent, "PUT", path, nil)
	tm.owner.expect(http.StatusNoContent, "PUT", path, nil)
	if r := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil); r.body["starred"] != true {
		t.Fatalf("starred conversation: %s", r.raw)
	}
	if r := tm.agent.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil); r.body["starred"] != false {
		t.Fatalf("a star is per member: %s", r.raw)
	}
	if got := itemIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/conversations?starred=true", nil)); !slices.Equal(got, []string{conv}) {
		t.Fatalf("starred=true lists %v", got)
	}
	if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/conversations?starred=true", nil)); len(got) != 0 {
		t.Fatalf("agent starred lists %v", got)
	}
	if n := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil).body["starred"]; n != float64(1) {
		t.Fatalf("starred count %v", n)
	}

	var updated []map[string]any
	for _, e := range eventsOf(tm.owner, after, "conversation.updated") {
		if d, _ := e["data"].(map[string]any); d["id"] == conv {
			updated = append(updated, d)
		}
	}
	if len(updated) != 1 || updated[0]["starred"] != true {
		t.Fatalf("owner star events %v", updated)
	}
	for _, e := range feed(tm.agent, after) {
		if e["type"] == "conversation.star" || e["type"] == "conversation.updated" {
			t.Fatalf("the agent sees another member's star: %v", e)
		}
	}

	post(tm.owner, plain, map[string]any{"kind": "message", "body": "sent by me"})
	if got := itemIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/conversations?sent=mine", nil)); !slices.Equal(got, []string{plain}) {
		t.Fatalf("sent=mine lists %v", got)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?sent=all", nil)
	key.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?starred=true", nil)
	key.expectProblem(http.StatusForbidden, "member_session_required", "PUT", path, nil)
	if r := key.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil); r.body["starred"] != nil {
		t.Fatalf("a key sees starred: %s", r.raw)
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", path, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", path, nil)
	h.client().expectProblem(http.StatusUnauthorized, "unauthenticated", "PUT", path, nil)

	tm.owner.expect(http.StatusNoContent, "DELETE", path, nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", path, nil)
	if got := itemIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/conversations?starred=true", nil)); len(got) != 0 {
		t.Fatalf("after unstar %v", got)
	}
	if n := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil).body["starred"]; n != float64(0) {
		t.Fatalf("starred count after unstar %v", n)
	}
}

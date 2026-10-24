package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

func TestPersonalMailboxReadOnlyByItsOwner(t *testing.T) {
	t.Parallel()
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Webhooks.AllowPrivate = true
		d.Migrations = api.MigrationSettings{AllowPrivate: true, TestTimeout: 5 * time.Second}
	})
	tm := newTeam(t, h)
	mailOwner, mailOwnerID := tm.newAgent()
	adminEmail := unique("admin") + "@example.com"
	tm.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": adminEmail, "role": "admin"})
	admin := h.client()
	admin.signIn(adminEmail)
	key := tm.apiKey(h)

	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/inboxes", map[string]any{"name": "Theirs", "slug": unique("theirs"), "owner_member_id": mailOwnerID})
	key.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/inboxes", map[string]any{"name": "Keyed", "slug": unique("keyed"), "owner_member_id": mailOwnerID})
	mailOwner.expectProblem(http.StatusConflict, "purpose_mismatch", "POST", "/v1/inboxes", map[string]any{"name": "Mine", "slug": unique("mine"), "purpose": "support", "owner_member_id": mailOwnerID})
	created := mailOwner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Private mail", "slug": unique("private"), "owner_member_id": mailOwnerID})
	mailbox := created.body["inbox"].(map[string]any)
	inbox := mailbox["id"].(string)
	if mailbox["owner_member_id"] != mailOwnerID || mailbox["purpose"] != "mail" {
		t.Fatalf("created: %s", created.raw)
	}
	if shared := tm.owner.expect(http.StatusOK, "GET", "/v1/inboxes/"+tm.inbox, nil); shared.body["owner_member_id"] != nil || shared.body["mailbox"] != nil {
		t.Fatalf("shared inbox: %s", shared.raw)
	}

	address := unique("me") + "@example.com"
	mailOwner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+inbox+"/channels", map[string]any{
		"kind": "email", "name": "My mail", "email": map[string]any{
			"address": address, "smtp": map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"},
		},
	})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/inboxes/"+inbox+"/channels", nil)

	rcv := newReceiver(t)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": rcv.srv.URL, "events": []string{"conversation.created", "message.created", "contact.updated"}})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/webhooks", map[string]any{"url": rcv.srv.URL, "events": []string{"message.created"}, "inbox_id": inbox})

	r := h.ingest(address, []byte(fmt.Sprintf(attachmentMail, address, newMessageID())), nil)
	if r.status != http.StatusAccepted || r.str("status") != "stored" {
		t.Fatalf("ingest: %d %v", r.status, r.body)
	}
	conv, msg := r.str("conversation_id"), r.str("message_id")
	got := mailOwner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	contact := got.str("contact_id")
	var attachment string
	for _, m := range messages(mailOwner, conv) {
		if as, _ := m["attachments"].([]any); len(as) > 0 {
			attachment = as[0].(map[string]any)["id"].(string)
		}
	}
	if attachment == "" {
		t.Fatal("no attachment on the ingested mail")
	}
	post(mailOwner, conv, map[string]any{"kind": "note", "body": "my own note"})
	h.runWebhooks(tm.ws)
	if n := h.count("SELECT count(*) FROM webhook_deliveries WHERE workspace_id = $1 AND (conversation_id = $2 OR contact_id = $3)", tm.ws, conv, contact); n != 0 {
		t.Fatalf("%d webhook deliveries for the personal mailbox", n)
	}
	rcv.take()
	post(tm.owner, tm.conversation(tm.owner), map[string]any{"kind": "note", "body": "shared note"})
	h.runWebhooks(tm.ws)
	if len(rcv.take()) == 0 {
		t.Fatal("the shared inbox sent no webhook either")
	}

	replyDraft := mailOwner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "draft reply", "draft": true}).str("id")
	composed := mailOwner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": inbox, "to": []string{"success@simulator.amazonses.com"}, "subject": unique("Private draft"), "body": "Draft body", "draft": true,
	}).body["conversation"].(map[string]any)["id"].(string)

	reader, readerID := tm.newAgent(tm.inbox)
	sender, senderID := tm.newAgent(tm.inbox)
	mailOwner.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"name": "Mia Owner"})
	sender.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"name": "Sam Sender"})
	delegates := "/v1/inboxes/" + inbox + "/delegates/"
	readerWS := reader.dial("")
	readerWS.ready()
	for _, c := range []*client{tm.owner, admin, tm.agent} {
		c.expectProblem(http.StatusNotFound, "not_found", "PUT", delegates+readerID, map[string]any{"access": "read"})
		c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/inboxes/"+inbox+"/delegates", nil)
	}
	key.expectProblem(http.StatusForbidden, "member_session_required", "PUT", delegates+readerID, map[string]any{"access": "read"})
	mailOwner.expectProblem(http.StatusUnprocessableEntity, "self_delegation", "PUT", delegates+mailOwnerID, map[string]any{"access": "read"})
	mailOwner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", delegates+readerID, map[string]any{"access": "write"})
	mailOwner.expectProblem(http.StatusNotFound, "not_found", "PUT", delegates+uuid.New().String(), map[string]any{"access": "read"})
	tm.owner.expectProblem(http.StatusConflict, "personal_mailbox", "PUT", "/v1/inboxes/"+tm.inbox+"/delegates/"+readerID, map[string]any{"access": "read"})
	if g := mailOwner.expect(http.StatusOK, "PUT", delegates+readerID, map[string]any{"access": "read"}); g.str("access") != "read" || g.str("member_id") != readerID {
		t.Fatalf("grant: %s", g.raw)
	}
	var change map[string]any
	if err := json.Unmarshal(readerWS.nextOf("inbox_access.changed").Data, &change); err != nil || change["inbox_id"] != inbox || change["granted"] != true {
		t.Fatalf("reader's realtime on grant: %v %v", change, err)
	}
	mailOwner.expect(http.StatusOK, "PUT", delegates+senderID, map[string]any{"access": "read"})
	mailOwner.expect(http.StatusOK, "PUT", delegates+senderID, map[string]any{"access": "send"})
	mailOwner.expect(http.StatusOK, "PUT", delegates+senderID, map[string]any{"access": "send"})
	if l := mailOwner.expect(http.StatusOK, "GET", "/v1/inboxes/"+inbox+"/delegates", nil); len(l.body["items"].([]any)) != 2 || !strings.Contains(string(l.raw), `"send"`) {
		t.Fatalf("delegates: %s", l.raw)
	}
	for _, d := range []struct {
		c      *client
		access string
	}{{mailOwner, "owner"}, {reader, "read"}, {sender, "send"}} {
		got := d.c.expect(http.StatusOK, "GET", "/v1/inboxes/"+inbox, nil)
		if box, _ := got.body["mailbox"].(map[string]any); box == nil || box["access"] != d.access {
			t.Fatalf("%s's mailbox: %s", d.access, got.raw)
		}
	}
	for _, c := range []*client{reader, sender} {
		c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/inboxes/"+inbox+"/delegates", nil)
		c.expectProblem(http.StatusNotFound, "not_found", "PUT", delegates+tm.agentID, map[string]any{"access": "read"})
		c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/inboxes/"+inbox+"/channels", nil)
		c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/inboxes/"+inbox+"/members", nil)
		if c.do("PATCH", "/v1/inboxes/"+inbox, map[string]any{"timezone": "Europe/Berlin"}).status/100 == 2 {
			t.Fatal("a delegate changed the mailbox's settings")
		}
		if c.do("DELETE", "/v1/inboxes/"+inbox, nil).status/100 == 2 {
			t.Fatal("a delegate deleted the mailbox")
		}
	}
	readerGrant := h.client()
	readerGrant.bearer = h.oauthToken(reader, tm.ws, "Reader App", testOrigin, []string{"conversations:read", "conversations:write", "notes:write", "messages:write", "contacts:write", "contacts:read", "inboxes:read", "reports:read"})
	readerMCP := h.mcpSession(h.oauthToken(reader, tm.ws, "Reader Assistant", testOrigin+"/mcp", []string{"conversations:read", "contacts:read", "inboxes:read"}))

	ownerGrant := h.client()
	ownerGrant.bearer = h.oauthToken(mailOwner, tm.ws, "Own App", testOrigin, []string{"conversations:read", "conversations:write", "contacts:read", "inboxes:read", "reports:read"})
	otherGrant := h.client()
	otherGrant.bearer = h.oauthToken(tm.owner, tm.ws, "Owner App", testOrigin, []string{"conversations:read", "conversations:write", "contacts:read", "inboxes:read", "reports:read"})
	scopes := []string{"conversations:read", "contacts:read", "inboxes:read"}
	ownerMCP := h.mcpSession(h.oauthToken(mailOwner, tm.ws, "Own Assistant", testOrigin+"/mcp", scopes))
	otherMCP := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Owner Assistant", testOrigin+"/mcp", scopes))

	today := h.clock.Now().UTC().Format(time.DateOnly)
	ok := func(c *client, method, path string, body any) bool { return c.do(method, path, body).status/100 == 2 }
	has := func(c *client, path, needle string) bool {
		res := c.do("GET", path, nil)
		return res.status == http.StatusOK && strings.Contains(string(res.raw), needle)
	}
	const (
		none = iota
		reads
		sends
		owns
	)
	surfaces := []struct {
		name  string
		needs int
		sees  func(c *client) bool
	}{
		{"get conversation", reads, func(c *client) bool { return ok(c, "GET", "/v1/conversations/"+conv, nil) }},
		{"list conversations", reads, func(c *client) bool { return has(c, "/v1/conversations?limit=100", conv) }},
		{"counts", reads, func(c *client) bool { return has(c, "/v1/conversations/counts", inbox) }},
		{"messages", reads, func(c *client) bool { return ok(c, "GET", "/v1/conversations/"+conv+"/messages", nil) }},
		{"attachment", reads, func(c *client) bool { return ok(c, "GET", "/v1/attachments/"+attachment, nil) }},
		{"raw e-mail", reads, func(c *client) bool { return ok(c, "GET", "/v1/messages/"+msg+"/raw", nil) }},
		{"search", reads, func(c *client) bool { return has(c, "/v1/conversations?q=export", conv) }},
		{"contact", reads, func(c *client) bool { return ok(c, "GET", "/v1/contacts/"+contact, nil) }},
		{"contact list", reads, func(c *client) bool { return has(c, "/v1/contacts?limit=100", contact) }},
		{"contact's conversations", reads, func(c *client) bool { return has(c, "/v1/conversations?contact_id="+contact, conv) }},
		{"events", reads, func(c *client) bool { return has(c, "/v1/events?after=0&limit=500", conv) }},
		{"drafts", owns, func(c *client) bool { return has(c, "/v1/drafts", replyDraft) }},
		{"composed draft", owns, func(c *client) bool { return ok(c, "GET", "/v1/conversations/"+composed, nil) }},
		{"composed draft listed", owns, func(c *client) bool { return has(c, "/v1/conversations?limit=100", composed) }},
		{"report", reads, func(c *client) bool { return ok(c, "GET", "/v1/reports?from="+today+"&to="+today+"&inbox_id="+inbox, nil) }},
		{"bulk", sends, func(c *client) bool {
			res := c.do("POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{conv}, "status": "open"})
			return res.status == http.StatusOK && strings.Contains(fmt.Sprint(res.body["updated"]), conv)
		}},
	}
	viewers := []struct {
		name  string
		c     *client
		level int
	}{
		{"mailbox owner", mailOwner, owns},
		{"mailbox owner's grant", ownerGrant, owns},
		{"read delegate", reader, reads},
		{"read delegate's grant", readerGrant, reads},
		{"send delegate", sender, sends},
		{"workspace owner", tm.owner, none},
		{"admin", admin, none},
		{"another agent", tm.agent, none},
		{"api key", key, none},
		{"another member's grant", otherGrant, none},
	}
	for _, v := range viewers {
		for _, s := range surfaces {
			if got, want := s.sees(v.c), v.level >= s.needs; got != want {
				t.Errorf("%s, %s: sees %v, want %v", v.name, s.name, got, want)
			}
		}
		if v.level == none {
			if ok(v.c, "POST", "/v1/conversations/"+conv+"/move", map[string]any{"inbox_id": tm.inbox}) {
				t.Errorf("%s moved the conversation out of the personal mailbox", v.name)
			}
			if has(v.c, "/v1/home", conv) {
				t.Errorf("%s: Home shows the conversation", v.name)
			}
		}
	}
	for _, m := range []struct {
		name   string
		cs     *mcp.ClientSession
		reader bool
	}{{"mailbox owner", ownerMCP, true}, {"read delegate", readerMCP, true}, {"another member", otherMCP, false}} {
		if _, errText := callTool(t, m.cs, "get_conversation", map[string]any{"conversation_id": conv}); (errText == "") != m.reader {
			t.Errorf("MCP %s get_conversation: %q", m.name, errText)
		}
		out, _ := callTool(t, m.cs, "search_conversations", map[string]any{"query": "export"})
		if strings.Contains(fmt.Sprint(out), conv) != m.reader {
			t.Errorf("MCP %s search_conversations: %v", m.name, out)
		}
	}

	for _, c := range []*client{reader, readerGrant} {
		c.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "reader note"})
		c.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "reader reply"})
		c.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
		c.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/conversations/mail", map[string]any{
			"inbox_id": inbox, "to": []string{"success@simulator.amazonses.com"}, "subject": "From the reader", "body": "Hi",
		})
	}
	reader.expectProblem(http.StatusForbidden, "forbidden", "PUT", "/v1/conversations/"+conv+"/star", nil)
	sender.expect(http.StatusNoContent, "PUT", "/v1/conversations/"+conv+"/star", nil)
	post(sender, conv, map[string]any{"kind": "note", "body": "sender note"})
	sender.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "pending"})
	reply := sender.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "On Mia's behalf"})
	if author, _ := reply.body["author"].(map[string]any); reply.str("on_behalf_of_member_id") != mailOwnerID || author["member_id"] != senderID {
		t.Fatalf("delegate's reply: %s", reply.raw)
	}
	readerWS.until(messageIn(conv))
	for _, q := range []struct {
		c     *client
		query string
	}{{mailOwner, "in:sent"}, {mailOwner, "from:me"}, {sender, "in:sent"}, {reader, "has:attachment"}, {reader, "-in:trash"}} {
		if !has(q.c, "/v1/conversations?q="+url.QueryEscape(q.query), conv) {
			t.Errorf("search %q misses the delegate's conversation", q.query)
		}
	}
	h.sendQueued(t, tm.ws, reply.str("id"))
	sent, parsed := h.smtp.last(t)
	from, senderHeader := parsed.Header.Get("From"), parsed.Header.Get("Sender")
	if !strings.Contains(from, `"Mia Owner (My mail)"`) || !strings.Contains(from, address) || sent.from != address {
		t.Fatalf("From %q envelope %s", from, sent.from)
	}
	if !strings.HasPrefix(senderHeader, "Sam Sender <") || !strings.Contains(senderHeader, address) {
		t.Fatalf("Sender %q", senderHeader)
	}
	if own := personEmail(sender); strings.Contains(string(sent.raw), own) {
		t.Fatalf("the delegate's own address %s is in the mail", own)
	}
	if n := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1 AND target_id = $2 AND action = 'mailbox.delegate_granted'", tm.ws, inbox); n != 3 {
		t.Fatalf("%d grants audited, want 3", n)
	}

	mailOwner.expect(http.StatusNoContent, "DELETE", delegates+readerID, nil)
	mailOwner.expect(http.StatusNoContent, "DELETE", delegates+readerID, nil)
	if err := json.Unmarshal(readerWS.nextOf("inbox_access.changed").Data, &change); err != nil || change["inbox_id"] != inbox || change["granted"] != false {
		t.Fatalf("reader's realtime on revoke: %v %v", change, err)
	}
	reader.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv, nil)
	readerGrant.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv, nil)
	post(mailOwner, conv, map[string]any{"kind": "note", "body": "after the revoke"})
	shared := tm.conversation(tm.owner)
	tm.say(tm.owner, shared, "sentinel")
	for _, m := range readerWS.until(messageIn(shared)) {
		if m.ConversationID == conv {
			t.Fatalf("a revoked delegate's open connection got %s", m.Type)
		}
	}
	if n := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1 AND target_id = $2 AND action = 'mailbox.delegate_revoked'", tm.ws, inbox); n != 1 {
		t.Fatalf("%d revokes audited, want 1", n)
	}

	adminID := memberships(admin)[0].(map[string]any)["member_id"].(string)
	adminBox := admin.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Admin mail", "slug": unique("adminmail"), "owner_member_id": adminID}).body["inbox"].(map[string]any)["id"].(string)
	adminAddress := unique("adm") + "@example.com"
	adminChannel := admin.expect(http.StatusCreated, "POST", "/v1/inboxes/"+adminBox+"/channels", map[string]any{
		"kind": "email", "name": "Admin mail", "email": map[string]any{"address": adminAddress},
	}).str("id")
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/migrations", map[string]any{"preset": "gmail", "source_address": adminAddress, "inbox_id": adminBox})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/migrations", map[string]any{"preset": "gmail", "source_address": adminAddress, "channel_id": adminChannel})
	mig := admin.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{"preset": "gmail", "source_address": adminAddress, "channel_id": adminChannel}).str("id")
	sharedMig := tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{"preset": "gmail", "source_address": unique("shared") + "@example.com", "inbox_id": tm.inbox}).str("id")
	reached := func(c *client, method, path string, body any) bool { return c.do(method, path, body).status != http.StatusNotFound }
	migrationSurfaces := []struct {
		name string
		sees func(c *client) bool
	}{
		{"get", func(c *client) bool { return ok(c, "GET", "/v1/migrations/"+mig, nil) }},
		{"list", func(c *client) bool { return has(c, "/v1/migrations?limit=100", mig) }},
		{"folders", func(c *client) bool { return ok(c, "GET", "/v1/migrations/"+mig+"/folders", nil) }},
		{"errors", func(c *client) bool { return ok(c, "GET", "/v1/migrations/"+mig+"/errors", nil) }},
		{"preview", func(c *client) bool { return reached(c, "GET", "/v1/migrations/"+mig+"/preview", nil) }},
		{"events", func(c *client) bool { return has(c, "/v1/events?after=0&limit=500", mig) }},
		{"update", func(c *client) bool {
			return ok(c, "PATCH", "/v1/migrations/"+mig, map[string]any{"options": map[string]any{"close_after_days": 7}})
		}},
		{"start", func(c *client) bool { return reached(c, "POST", "/v1/migrations/"+mig+"/start", nil) }},
		{"finish", func(c *client) bool { return reached(c, "POST", "/v1/migrations/"+mig+"/finish", map[string]any{}) }},
		{"remove history", func(c *client) bool { return reached(c, "POST", "/v1/migrations/"+mig+"/remove-history", map[string]any{}) }},
	}
	for _, v := range []struct {
		name   string
		c      *client
		reader bool
	}{{"mailbox owner (admin)", admin, true}, {"workspace owner", tm.owner, false}, {"api key", key, false}} {
		for _, s := range migrationSurfaces {
			if got := s.sees(v.c); got != v.reader {
				t.Errorf("migration %s, %s: sees %v, want %v", s.name, v.name, got, v.reader)
			}
		}
		if !has(v.c, "/v1/migrations?limit=100", sharedMig) {
			t.Errorf("%s does not list the shared migration", v.name)
		}
	}
	admin.expectProblem(http.StatusConflict, "personal_mailbox", "PATCH", "/v1/migrations/"+mig, map[string]any{"inbox_id": tm.inbox})
	admin.expectProblem(http.StatusConflict, "personal_mailbox", "PATCH", "/v1/migrations/"+mig, map[string]any{"inbox_id": nil})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/migrations/"+sharedMig, map[string]any{"inbox_id": adminBox})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/migrations/"+sharedMig, map[string]any{"channel_id": adminChannel})
	admin.expectProblem(http.StatusConflict, "personal_mailbox", "PATCH", "/v1/migrations/"+sharedMig, map[string]any{
		"options": map[string]any{"inboxes": []map[string]any{{"folder": "Personal", "inbox_id": adminBox}}},
	})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/migrations/"+mig, nil)
	admin.expect(http.StatusNoContent, "DELETE", "/v1/migrations/"+mig, nil)
	left := admin.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{"preset": "gmail", "source_address": adminAddress, "channel_id": adminChannel}).str("id")
	admin.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+adminBox, nil)
	if n := h.count("SELECT count(*) FROM migrations WHERE workspace_id = $1 AND id = $2", tm.ws, left); n != 0 {
		t.Fatal("a migration into a deleted personal mailbox outlived it")
	}

	ownerID := tm.ownerMemberID(t)
	mailOwner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "look", "mentions": []string{ownerID}})

	meta := tm.owner.expect(http.StatusOK, "GET", "/v1/inboxes/"+inbox, nil)
	box, _ := meta.body["mailbox"].(map[string]any)
	if meta.body["owner_member_id"] != mailOwnerID || box == nil || box["conversations"] != float64(1) || !slices.Contains(box["addresses"].([]any), any(address)) || meta.str("signature") != "" {
		t.Fatalf("metadata: %s", meta.raw)
	}
	listed := tm.owner.expect(http.StatusOK, "GET", "/v1/inboxes", nil)
	if !strings.Contains(string(listed.raw), inbox) || strings.Contains(string(tm.agent.expect(http.StatusOK, "GET", "/v1/inboxes", nil).raw), inbox) {
		t.Fatalf("inbox list: owner %s", listed.raw)
	}
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/inboxes/"+inbox, nil)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+inbox, map[string]any{"name": "Renamed mail"})
	tm.owner.expectProblem(http.StatusConflict, "personal_mailbox", "PATCH", "/v1/inboxes/"+inbox, map[string]any{"timezone": "Europe/Istanbul"})
	key.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/inboxes/"+inbox, map[string]any{"name": "Key"})
	mailOwner.expectProblem(http.StatusConflict, "personal_mailbox", "PATCH", "/v1/inboxes/"+inbox, map[string]any{"purpose": "support"})
	mailOwner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+inbox, map[string]any{"timezone": "Europe/Istanbul"})
	tm.owner.expectProblem(http.StatusConflict, "personal_mailbox", "PUT", "/v1/inboxes/"+inbox+"/members/"+tm.agentID, nil)
	team := tm.newTeamOf("Mailers", []string{tm.agentID}, nil)
	tm.owner.expectProblem(http.StatusConflict, "personal_mailbox", "PUT", "/v1/teams/"+team+"/inboxes/"+inbox, nil)
	tm.owner.expectProblem(http.StatusConflict, "personal_mailbox", "POST", "/v1/api-keys", map[string]any{"name": "narrow", "scopes": []string{"conversations:read"}, "inbox_ids": []string{inbox}})
	removed := tm.owner.expect(http.StatusConflict, "DELETE", "/v1/members/"+mailOwnerID, nil)
	if removed.str("code") != "personal_mailbox" || !strings.Contains(fmt.Sprint(removed.body["inbox_ids"]), inbox) {
		t.Fatalf("remove member: %s", removed.raw)
	}
	key.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/inboxes/"+inbox, nil)
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/inboxes/"+inbox, nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+inbox, nil)
	if n := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1 AND target_id = $2 AND action IN ('inbox.created', 'inbox.deleted')", tm.ws, inbox); n != 2 {
		t.Fatalf("%d audit entries for the mailbox, want 2", n)
	}
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/members/"+mailOwnerID, nil)
}

func TestPersonalMailboxOwnersLeaveDeletedWorkspace(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := context.Background()
	tm := newTeam(t, h)
	second, secondID := tm.newAgent()
	for _, m := range []struct {
		c  *client
		id string
	}{{tm.agent, tm.agentID}, {second, secondID}} {
		m.c.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Mine", "slug": unique("mine"), "owner_member_id": m.id})
	}
	name := tm.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil).str("name")
	tm.owner.expect(http.StatusAccepted, "DELETE", "/v1/workspace", map[string]any{"name": name})

	agentEmail := personEmail(tm.agent)
	tm.agent.workspace = ""
	tm.agent.expect(http.StatusNoContent, "DELETE", "/v1/me", map[string]any{"email": agentEmail})
	if _, err := h.srv.OperatorDeletePerson(ctx, personEmail(second)); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT count(*) FROM inboxes WHERE workspace_id = $1 AND owner_member_id IS NOT NULL", tm.ws); n != 0 {
		t.Fatalf("%d personal mailboxes keep a deleted owner", n)
	}
	wsID := uuid.MustParse(tm.ws)
	for range 10 {
		p, err := h.srv.PurgeWorkspace(ctx, wsID, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if p.Done {
			break
		}
	}
	if n := h.count("SELECT count(*) FROM inboxes WHERE workspace_id = $1", tm.ws); n != 0 {
		t.Fatalf("%d inboxes left after the purge", n)
	}
}

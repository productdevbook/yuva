package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

func TestSearchOperators(t *testing.T) {
	t.Parallel()
	runSearchOperators(t)
}

func TestSearchOperatorsWithInlineAddressMatches(t *testing.T) {
	api.SetAddressMatchCap(1)
	t.Cleanup(func() { api.SetAddressMatchCap(5000) })
	runSearchOperators(t)
}

func runSearchOperators(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	et.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+et.inbox, map[string]any{"purpose": "mail"})
	other := newTeam(t, h)
	et.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+et.inbox+"/members/"+et.agentID, nil)
	key := et.apiKey(h)
	ctx := context.Background()

	var ownerEmail string
	if err := h.st.Pool.QueryRow(ctx, "SELECT p.email FROM members m JOIN people p ON p.id = m.person_id WHERE m.workspace_id = $1 AND m.role = 'owner'",
		uuid.MustParse(et.ws)).Scan(&ownerEmail); err != nil {
		t.Fatal(err)
	}

	alice := "alice@acme-test.example"
	a := h.ingest(et.address, buildMail(mailOpts{
		from: alice, to: et.address + ", " + ownerEmail, subject: "Quarterly invoice", messageID: newMessageID(), body: "please pay soon",
		headers: map[string]string{"Cc": "Carol <carol@cc-test.example>"},
	}), nil)
	convA := a.str("conversation_id")
	b := h.ingest(et.address, []byte(fmt.Sprintf(attachmentMail, et.address, newMessageID())), nil)
	convB := b.str("conversation_id")
	if convA == "" || convB == "" {
		t.Fatalf("ingest: %v %v", a.body, b.body)
	}
	h.clock.Advance(time.Minute)
	compose := func(c *client, subject string, extra map[string]any) (string, string) {
		body := map[string]any{"inbox_id": et.inbox, "to": []string{"success@simulator.amazonses.com"}, "subject": subject, "body": "body of " + subject}
		for k, v := range extra {
			body[k] = v
		}
		r := c.expect(http.StatusCreated, "POST", "/v1/conversations/mail", body)
		return r.body["conversation"].(map[string]any)["id"].(string), r.body["message"].(map[string]any)["id"].(string)
	}
	convC, _ := compose(et.owner, "Launch plan", nil)
	convD, _ := compose(et.owner, "Hidden notes", map[string]any{"draft": true})
	convE, _ := compose(et.owner, "Later", map[string]any{"send_at": h.clock.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
	convF, _ := compose(et.agent, "Agent secret", map[string]any{"draft": true})

	search := func(c *client, q string, extra ...string) []string {
		t.Helper()
		query := "?q=" + url.QueryEscape(q)
		for _, e := range extra {
			query += "&" + e
		}
		return itemIDs(c.expect(http.StatusOK, "GET", "/v1/conversations"+query, nil))
	}
	want := func(ids ...string) []string {
		out := slices.Clone(ids)
		slices.Sort(out)
		return out
	}
	check := func(c *client, q string, ids []string, extra ...string) {
		t.Helper()
		if got := search(c, q, extra...); !slices.Equal(got, want(ids...)) {
			t.Fatalf("q=%q %v: got %v, want %v", q, extra, got, want(ids...))
		}
	}
	o := et.owner

	check(o, "from:alice@acme-test", []string{convA})
	check(o, "from:ALICE", []string{convA})
	check(o, "from:customer", []string{convA})
	check(o, `from:"Ayşe"`, []string{convB})
	check(o, "from:%", nil)
	check(o, "from:me", []string{convC})
	check(o, "-from:me", []string{convA, convB, convD, convE})
	check(o, "to:me", []string{convA})
	check(o, "to:SIMULATOR.amazonses", []string{convC})
	check(o, "-to:simulator", []string{convA, convB, convD, convE})
	check(o, "cc:carol@cc-test", []string{convA})
	check(o, "cc:me", nil)
	check(o, "subject:invoice", []string{convA})
	check(o, `subject:"launch plan"`, []string{convC})
	check(o, "-subject:invoice -subject:plan", []string{convB, convD, convE})
	check(o, "please subject:invoice", []string{convA})
	check(o, "unrelated subject:invoice", nil)
	check(o, "from:alice has:attachment", nil)
	check(o, "has:attachment", []string{convB})
	check(o, "-has:attachment", []string{convA, convC, convD, convE})
	check(o, "ratio:3:1", nil)
	check(o, "invoice before:soon", nil)

	o.expect(http.StatusNoContent, "PUT", "/v1/conversations/"+convC+"/star", nil)
	check(o, "is:starred", []string{convC})
	check(o, "in:starred", []string{convC})
	check(o, "-is:starred", []string{convA, convB, convD, convE})
	check(o, "is:starred from:me", []string{convC})
	check(o, "is:starred from:alice", nil)
	check(et.agent, "is:starred", nil)

	check(o, "in:sent", []string{convC})
	check(o, "in:drafts", []string{convD})
	check(o, "in:scheduled", []string{convE})
	check(et.agent, "in:drafts", []string{convF})
	check(et.agent, "in:scheduled", nil)
	check(o, "subject:agent", nil)
	check(o, "subject:secret in:drafts", nil)
	check(et.agent, "subject:secret", []string{convF})
	check(o, "in:inbox", nil, "status=closed")

	for _, q := range []string{"from:me", "to:me", "is:starred", "in:starred", "in:sent", "in:drafts", "in:scheduled", "-in:sent"} {
		key.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?q="+url.QueryEscape(q), nil)
	}
	if got := search(key, "from:alice"); !slices.Equal(got, want(convA)) {
		t.Fatalf("key from:alice %v", got)
	}

	o.expect(http.StatusOK, "PATCH", "/v1/conversations/"+convB, map[string]any{"status": "closed"})
	check(o, "in:archive", []string{convB})
	check(o, "in:inbox", []string{convA})
	check(o, "-in:archive from:me", []string{convC})
	check(o, "in:archive", []string{convB}, "status=closed")
	check(o, "in:archive", nil, "status=open")
	o.expect(http.StatusOK, "PATCH", "/v1/conversations/"+convA, map[string]any{"status": "snoozed", "snooze_until": h.clock.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)})
	check(o, "in:snoozed", []string{convA})

	o.expect(http.StatusOK, "DELETE", "/v1/conversations/"+convB, nil)
	check(o, "in:trash", []string{convB})
	check(o, "has:attachment", nil)
	check(o, "in:archive", nil)
	et.agent.expectProblem(http.StatusForbidden, "forbidden", "GET", "/v1/conversations?q=in%3Atrash", nil)
	check(o, "-in:trash from:alice", []string{convA})

	stamp := time.Date(2026, 3, 10, 21, 30, 0, 0, time.UTC)
	if _, err := h.st.Pool.Exec(ctx, "UPDATE messages SET created_at = $3 WHERE workspace_id = $1 AND conversation_id = $2",
		uuid.MustParse(et.ws), uuid.MustParse(convA), stamp); err != nil {
		t.Fatal(err)
	}
	check(o, "subject:invoice after:2026-03-10", []string{convA})
	check(o, "subject:invoice after:2026-03-11", nil)
	check(o, "subject:invoice before:2026-03-11", []string{convA})
	check(o, "subject:invoice before:2026-03-10", nil)
	ist := "timezone=" + url.QueryEscape("Europe/Istanbul")
	check(o, "subject:invoice after:2026-03-11", []string{convA}, ist)
	check(o, "subject:invoice before:2026-03-11", nil, ist)
	check(o, "subject:invoice after:2026-03-10 before:2026-03-12", []string{convA}, ist)
	check(o, "subject:invoice -after:2026-03-11", nil, ist)
	check(o, "subject:invoice -before:2026-03-11", []string{convA}, ist)
	o.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?q=after%3A2026-03-11&timezone=Mars%2FBase", nil)
	if r := o.do("GET", "/v1/conversations?timezone=Mars%2FBase", nil); r.status != http.StatusBadRequest || r.body["field"] != "timezone" {
		t.Fatalf("bad timezone without dates: %d %s", r.status, r.raw)
	}
	check(o, "subject:invoice after:0000-01-01 before:99999-01-01", nil)

	support := o.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Support", "slug": unique("support")}).body["inbox"].(map[string]any)["id"].(string)
	convS := o.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": support, "contact_id": et.contact, "subject": "Support thing"}).str("id")
	o.expect(http.StatusNoContent, "PUT", "/v1/conversations/"+convS+"/star", nil)
	check(o, "is:starred", []string{convC, convS})
	check(o, "in:starred", []string{convC})
	o.expect(http.StatusOK, "PATCH", "/v1/conversations/"+convC, map[string]any{"status": "open"})
	check(o, "in:inbox", []string{convC})
	check(o, "-in:inbox is:starred", []string{convS})

	check(other.owner, "from:alice", nil)
	check(other.owner, "subject:invoice", nil)
	check(other.owner, "-from:alice", nil)

	o.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?q="+url.QueryEscape(strings.Repeat("is:starred ", 21)), nil)
	o.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?q="+url.QueryEscape(strings.Repeat("x", 201)), nil)

	cs := h.mcpSession(h.oauthToken(o, et.ws, "Claude Code", testOrigin+"/mcp", []string{"conversations:read", "inboxes:read"}))
	out, errText := callTool(t, cs, "search_conversations", map[string]any{"query": `from:alice subject:"quarterly invoice"`})
	items, _ := out["items"].([]any)
	if errText != "" || len(items) != 1 || items[0].(map[string]any)["id"] != convA {
		t.Fatalf("search_conversations with operators: %v %q", out, errText)
	}
	out, errText = callTool(t, cs, "search_conversations", map[string]any{"query": "-from:alice in:sent"})
	items, _ = out["items"].([]any)
	if errText != "" || len(items) != 1 || items[0].(map[string]any)["id"] != convC {
		t.Fatalf("search_conversations negated: %v %q", out, errText)
	}
}

func TestSearchOperatorsStayOutOfPersonalMailboxes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	mailOwner, mailOwnerID := tm.newAgent()
	inbox := mailOwner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Private mail", "slug": unique("private"), "owner_member_id": mailOwnerID}).
		body["inbox"].(map[string]any)["id"].(string)
	address := unique("me") + "@example.com"
	mailOwner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+inbox+"/channels", map[string]any{
		"kind": "email", "name": "My mail", "email": map[string]any{"address": address},
	})
	sender := unique("friend") + "@private-test.example"
	r := h.ingest(address, buildMail(mailOpts{from: sender, to: address, subject: "Private hello", messageID: newMessageID(), body: "just us"}), nil)
	conv := r.str("conversation_id")
	if conv == "" {
		t.Fatalf("ingest: %v", r.body)
	}
	for _, q := range []string{"from:" + sender, "to:" + address, "from:private-test", "subject:private", "-from:nobody"} {
		found := func(c *client) bool {
			return slices.Contains(itemIDs(c.expect(http.StatusOK, "GET", "/v1/conversations?q="+url.QueryEscape(q), nil)), conv)
		}
		if !found(mailOwner) {
			t.Fatalf("the owner does not find %q", q)
		}
		if found(tm.owner) || found(tm.agent) {
			t.Fatalf("another member finds the personal mailbox with %q", q)
		}
	}
}

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/productdevbook/yuva/api/internal/api"
)

func (tm team) agentWithEmail(inboxes ...string) (*client, string, string) {
	tm.owner.h.t.Helper()
	email := unique("agent") + "@example.com"
	tm.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": email, "role": "agent"})
	c := tm.owner.h.client()
	c.signIn(email)
	id := c.expect(http.StatusOK, "GET", "/v1/me", nil).body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)
	for _, in := range inboxes {
		tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+in+"/members/"+id, nil)
	}
	return c, id, email
}

func contains(field, value string) map[string]any {
	return map[string]any{"field": field, "operator": "contains", "value": value}
}

func runOutcomes(c *client, rule string) []string {
	var out []string
	for _, r := range ruleRuns(c, rule) {
		s := r["outcome"].(string)
		if e, _ := r["error"].(string); e != "" {
			s += ":" + e
		}
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

func TestMailFilters(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	tm := et.team
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"purpose": "mail"})
	reader, readerID, readerEmail := tm.agentWithEmail(tm.inbox)
	_, strangerID := tm.newAgent()
	support := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Helpdesk", "slug": unique("help")}).body["inbox"].(map[string]any)["id"].(string)
	label := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "Newsletters"}).str("id")

	base := func(conds []map[string]any, actions ...map[string]any) map[string]any {
		return map[string]any{"name": unique("filter"), "inbox_id": tm.inbox, "trigger": "mail_received", "conditions": conds, "actions": actions}
	}
	star := func(ids ...string) map[string]any { return map[string]any{"type": "star", "member_ids": ids} }
	news := []map[string]any{contains("from", "letters.example"), contains("list_id", "news.example.com")}
	for name, body := range map[string]map[string]any{
		"not a mail inbox":      {"name": "x", "inbox_id": support, "trigger": "mail_received", "actions": []map[string]any{{"type": "archive"}}},
		"no inbox":              {"name": "x", "trigger": "mail_received", "actions": []map[string]any{{"type": "archive"}}},
		"mail field elsewhere":  {"name": "x", "trigger": "message_received", "conditions": news, "actions": []map[string]any{{"type": "set_priority", "priority": "high"}}},
		"mail action elsewhere": {"name": "x", "trigger": "message_received", "actions": []map[string]any{{"type": "archive"}}},
		"short value":           base([]map[string]any{contains("from", "ab")}, map[string]any{"type": "archive"}),
		"long value":            base([]map[string]any{contains("subject", strings.Repeat("a", 201))}, map[string]any{"type": "archive"}),
		"address with space":    base([]map[string]any{contains("to", "a b c")}, map[string]any{"type": "archive"}),
		"attachment value":      base([]map[string]any{{"field": "has_attachment", "operator": "equals", "value": "yes"}}, map[string]any{"type": "archive"}),
		"attachment contains":   base([]map[string]any{{"field": "has_attachment", "operator": "contains", "value": "true"}}, map[string]any{"type": "archive"}),
		"forward and trash":     base(news, map[string]any{"type": "forward_to_member", "member_id": readerID}, map[string]any{"type": "trash"}),
		"archive and status":    base(news, map[string]any{"type": "archive"}, map[string]any{"type": "set_status", "status": "open"}),
		"archive and snooze":    base(news, map[string]any{"type": "archive"}, map[string]any{"type": "snooze", "snooze_minutes": 60}),
		"star nobody":           base(news, map[string]any{"type": "star"}),
		"star a non-reader":     base(news, star(strangerID)),
		"forward a non-reader":  base(news, map[string]any{"type": "forward_to_member", "member_id": strangerID}),
		"assign a non-reader":   base(news, map[string]any{"type": "assign_member", "member_id": strangerID}),
	} {
		res := tm.owner.do("POST", "/v1/automation-rules", body)
		if res.status != http.StatusBadRequest || res.str("code") != "validation_failed" {
			t.Errorf("%s: %d %s", name, res.status, res.raw)
		}
	}

	newsRule := tm.rule(tm.owner, base(news, star(readerID), map[string]any{"type": "add_labels", "label_ids": []string{label}},
		map[string]any{"type": "archive"}, map[string]any{"type": "forward_to_member", "member_id": readerID}))
	invoiceRule := tm.rule(tm.owner, base([]map[string]any{
		contains("to", et.address), contains("cc", "boss@example.org"), contains("subject", "invoice"),
		{"field": "has_attachment", "operator": "equals", "value": "false"},
	}, map[string]any{"type": "set_priority", "priority": "high"}, map[string]any{"type": "assign_member", "member_id": readerID}))
	trashRule := tm.rule(tm.owner, base([]map[string]any{{"field": "has_attachment", "operator": "equals", "value": "true"}}, map[string]any{"type": "trash"}))
	tm.rule(tm.owner, base([]map[string]any{{"field": "from", "operator": "not_contains", "value": "letters.example"}, contains("from", "nobody.example")}, map[string]any{"type": "archive"}))
	undeliveredRule := tm.rule(tm.owner, base([]map[string]any{contains("subject", "undeliver")}, map[string]any{"type": "set_priority", "priority": "low"}))
	noted := base([]map[string]any{contains("subject", "weekly")}, map[string]any{"type": "add_note", "body": "seen"}, map[string]any{"type": "assign_pool"})
	noted["enabled"] = false
	tm.rule(tm.owner, noted)

	first := newMessageID()
	r := h.ingest(et.address, buildMail(mailOpts{
		from: "weekly@letters.example.com", to: et.address, subject: "Weekly digest", messageID: first, body: "This week",
		headers: map[string]string{"List-Id": "Acme News <News.Example.COM>"},
	}), nil)
	if r.status != http.StatusAccepted {
		t.Fatalf("ingest: %d %v", r.status, r.body)
	}
	newsConv, newsMsg := r.str("conversation_id"), r.str("message_id")
	if n := h.count("SELECT count(*) FROM message_emails WHERE workspace_id = $1 AND message_id = $2 AND headers->>'List-Id' = 'Acme News <News.Example.COM>'", tm.ws, newsMsg); n != 1 {
		t.Fatalf("List-Id kept on %d rows", n)
	}
	var triggers []string
	for _, j := range h.automationJobs(tm.ws) {
		triggers = append(triggers, j.Trigger)
	}
	if !slices.Contains(triggers, "mail_received") {
		t.Fatalf("queued triggers %v", triggers)
	}
	h.runAutomations(tm.ws)
	got := reader.expect(http.StatusOK, "GET", "/v1/conversations/"+newsConv, nil)
	if got.body["starred"] != true || got.str("status") != "closed" || !slices.Contains(labelsOf(got), label) {
		t.Fatalf("after the news filter: %s", got.raw)
	}
	if tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+newsConv, nil).body["starred"] == true {
		t.Fatal("starred for a member the filter did not name")
	}
	forwards := h.jobs("mail_forwarded_notify", tm.ws)
	if len(forwards) != 1 {
		t.Fatalf("%d forward jobs", len(forwards))
	}
	var fwd api.MailForwardedArgs
	if err := json.Unmarshal(forwards[0].args, &fwd); err != nil {
		t.Fatal(err)
	}
	if fwd.MemberID.String() != readerID || fwd.MessageID.String() != newsMsg {
		t.Fatalf("forward args %+v", fwd)
	}
	before := len(h.mail.to(readerEmail))
	if err := h.srv.NotifyMailForwarded(context.Background(), fwd); err != nil {
		t.Fatal(err)
	}
	if sent := h.mail.to(readerEmail); len(sent) != before+1 {
		t.Fatalf("%d mail_forwarded e-mails", len(sent))
	}
	h.dropJob(forwards[0].id)
	if outcomes := runOutcomes(tm.owner, invoiceRule); len(outcomes) != 1 || outcomes[0] != "not_matched" {
		t.Fatalf("invoice rule on the newsletter: %v", outcomes)
	}

	sugg := tm.owner.expect(http.StatusOK, "GET", "/v1/messages/"+newsMsg+"/filter-suggestion", nil)
	conds, _ := sugg.body["conditions"].([]any)
	if sugg.str("trigger") != "mail_received" || sugg.str("inbox_id") != tm.inbox || sugg.str("name") != "news.example.com" || len(conds) != 2 ||
		conds[0].(map[string]any)["field"] != "from" || conds[1].(map[string]any)["field"] != "list_id" || len(sugg.body["actions"].([]any)) != 0 {
		t.Fatalf("suggestion: %s", sugg.raw)
	}
	other := newTeam(t, h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/messages/"+newsMsg+"/filter-suggestion", nil)
	support2 := tm.conversation(tm.owner)
	reply := post(tm.owner, support2, map[string]any{"kind": "note", "body": "internal"})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/messages/"+reply+"/filter-suggestion", nil)

	later := newMessageID()
	h.ingest(et.address, buildMail(mailOpts{
		from: "weekly@letters.example.com", to: et.address, subject: "Re: Weekly digest", messageID: later, body: "Again",
		headers: map[string]string{"List-Id": "Acme News <news.example.com>", "In-Reply-To": "<" + first + ">", "References": "<" + first + ">"},
	}), nil)
	jobs := h.automationJobs(tm.ws)
	h.runAutomations(tm.ws)
	if outcomes := runOutcomes(tm.owner, newsRule); len(outcomes) != 2 || outcomes[0] != "applied" || outcomes[1] != "applied" {
		t.Fatalf("news rule runs: %v", outcomes)
	}
	for _, j := range jobs {
		if err := h.srv.RunAutomations(context.Background(), j); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(ruleRuns(tm.owner, newsRule)); n != 2 {
		t.Fatalf("%d runs after running the same triggers again", n)
	}
	h.runAutomations(tm.ws)
	for _, j := range h.jobs("mail_forwarded_notify", tm.ws) {
		h.dropJob(j.id)
	}

	r = h.ingest(et.address, buildMail(mailOpts{
		from: "client@example.org", to: et.address + ", other@example.org", subject: "Invoice 42", messageID: newMessageID(), body: "Please pay",
		headers: map[string]string{"Cc": "Boss <boss@example.org>"},
	}), nil)
	invoiceConv := r.str("conversation_id")
	h.runAutomations(tm.ws)
	got = tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+invoiceConv, nil)
	if got.str("priority") != "high" || got.str("assignee_id") != readerID {
		t.Fatalf("after the invoice rule: %s", got.raw)
	}

	invoiceMsg := r.str("message_id")
	sugg = tm.owner.expect(http.StatusOK, "GET", "/v1/messages/"+invoiceMsg+"/filter-suggestion", nil)
	conds, _ = sugg.body["conditions"].([]any)
	if len(conds) != 2 || conds[1].(map[string]any)["field"] != "subject" || conds[1].(map[string]any)["value"] != "Invoice 42" {
		t.Fatalf("suggestion without a list: %s", sugg.raw)
	}
	draft := sugg.body
	draft["actions"] = []map[string]any{{"type": "set_priority", "priority": "urgent"}}
	draft["enabled"] = false
	tm.owner.expect(http.StatusCreated, "POST", "/v1/automation-rules", draft)
	long := h.ingest(et.address, buildMail(mailOpts{from: "verbose@example.org", to: et.address, subject: strings.Repeat("Long subject ", 20), messageID: newMessageID(), body: "x"}), nil)
	sugg = tm.owner.expect(http.StatusOK, "GET", "/v1/messages/"+long.str("message_id")+"/filter-suggestion", nil)
	if conds, _ = sugg.body["conditions"].([]any); len(conds) != 1 {
		t.Fatalf("suggestion kept a subject over 200 characters: %s", sugg.raw)
	}
	draft = sugg.body
	draft["actions"] = []map[string]any{{"type": "archive"}}
	draft["enabled"] = false
	tm.owner.expect(http.StatusCreated, "POST", "/v1/automation-rules", draft)
	h.runAutomations(tm.ws)

	post(tm.owner, invoiceConv, map[string]any{"kind": "message", "body": "Paid, invoice attached"})
	tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+invoiceConv+"/messages", map[string]any{"kind": "message", "body": "invoice draft", "draft": true})
	dsn := h.ingestFrom(et.address, "", dsnMail(et.address, newMessageID(), unique("gone")+"@example.net", "5.1.1"), nil)
	if dsn.str("conversation_id") == "" {
		t.Fatalf("unmatched delivery report not stored: %d %v", dsn.status, dsn.body)
	}
	h.runAutomations(tm.ws)
	if n := h.count("SELECT count(*) FROM automation_runs WHERE workspace_id = $1 AND conversation_id = $2 AND trigger = 'mail_received'", tm.ws, dsn.str("conversation_id")); n != 0 {
		t.Fatalf("%d mail filter runs on a delivery report", n)
	}
	spoofed := h.ingest(et.address, buildMail(mailOpts{
		from: "postmaster@example.org", to: et.address, subject: "Undelivered: your account", messageID: newMessageID(), body: "Click here",
		headers: map[string]string{"Return-Path": "<>"},
	}), nil)
	h.runAutomations(tm.ws)
	if n := h.count("SELECT count(*) FROM automation_runs WHERE workspace_id = $1 AND rule_id = $2 AND conversation_id = $3 AND outcome = 'applied'", tm.ws, undeliveredRule, spoofed.str("conversation_id")); n != 1 {
		t.Fatalf("a sender-made Return-Path: <> from postmaster skipped the filters (%d runs)", n)
	}
	invoiceRuns := 0
	for _, run := range ruleRuns(tm.owner, invoiceRule) {
		if run["conversation_id"] == invoiceConv {
			invoiceRuns++
		}
	}
	if invoiceRuns != 1 {
		t.Fatalf("%d invoice rule runs on its conversation", invoiceRuns)
	}

	r = h.ingest(et.address, fmt.Appendf(nil, attachmentMail, et.address, newMessageID()), nil)
	trashed := r.str("conversation_id")
	h.runAutomations(tm.ws)
	if n := h.count("SELECT count(*) FROM automation_runs WHERE workspace_id = $1 AND rule_id = $2 AND conversation_id = $3 AND outcome = 'applied'", tm.ws, trashRule, trashed); n != 1 {
		t.Fatalf("%d applied trash runs", n)
	}
	if n := h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND id = $2 AND trashed_at IS NOT NULL", tm.ws, trashed); n != 1 {
		t.Fatal("the attachment filter did not trash the conversation")
	}

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+tm.inbox+"/members/"+readerID, nil)
	h.ingest(et.address, buildMail(mailOpts{
		from: "weekly@letters.example.com", to: et.address, subject: "Another digest", messageID: newMessageID(), body: "More",
		headers: map[string]string{"List-Id": "<news.example.com>"},
	}), nil)
	h.runAutomations(tm.ws)
	if outcomes := runOutcomes(tm.owner, newsRule); !slices.Contains(outcomes, "failed:no_access") {
		t.Fatalf("news rule after the reader left: %v", outcomes)
	}
	if n := len(h.jobs("mail_forwarded_notify", tm.ws)); n != 0 {
		t.Fatalf("%d forwards to a member who left", n)
	}
}

func TestPersonalMailFilters(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	mailOwner, mailOwnerID := tm.newAgent()
	adminEmail := unique("admin") + "@example.com"
	tm.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": adminEmail, "role": "admin"})
	admin := h.client()
	admin.signIn(adminEmail)
	key := h.client()
	key.bearer = tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{
		"name": "rules", "scopes": []string{"automations:read", "automations:manage", "conversations:read"},
	}).str("secret")
	inbox := mailOwner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Private mail", "slug": unique("private"), "owner_member_id": mailOwnerID}).body["inbox"].(map[string]any)["id"].(string)
	address := unique("me") + "@example.com"
	mailOwner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+inbox+"/channels", map[string]any{
		"kind": "email", "name": "My mail", "email": map[string]any{
			"address": address, "smtp": map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"},
		},
	})

	workspaceRule := tm.rule(tm.owner, map[string]any{"name": "Everything urgent", "trigger": "message_received", "actions": []map[string]any{{"type": "set_priority", "priority": "urgent"}}})
	filter := map[string]any{"name": "Alerts", "inbox_id": inbox, "trigger": "mail_received", "conditions": []map[string]any{contains("from", "alerts@")}, "actions": []map[string]any{{"type": "star"}}}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/automation-rules", filter)
	admin.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/automation-rules", filter)
	key.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/automation-rules", filter)
	mailOwner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/automation-rules", map[string]any{"name": "x", "inbox_id": inbox, "trigger": "message_received", "actions": []map[string]any{{"type": "set_priority", "priority": "high"}}})
	for _, act := range []map[string]any{
		{"type": "star", "member_ids": []string{tm.agentID}},
		{"type": "forward_to_member", "member_id": tm.agentID},
		{"type": "assign_member", "member_id": tm.agentID},
	} {
		mailOwner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/automation-rules", map[string]any{"name": "x", "inbox_id": inbox, "trigger": "mail_received", "actions": []map[string]any{act}})
	}
	created := mailOwner.expect(http.StatusCreated, "POST", "/v1/automation-rules", filter)
	rule := created.str("id")
	acts := created.body["actions"].([]any)
	if ids := acts[0].(map[string]any)["member_ids"].([]any); len(ids) != 1 || ids[0] != mailOwnerID || created.body["position"] != float64(0) {
		t.Fatalf("created filter: %s", created.raw)
	}

	if !slices.Contains(itemIDs(mailOwner.expect(http.StatusOK, "GET", "/v1/automation-rules?inbox_id="+inbox, nil)), rule) {
		t.Fatal("the owner does not see their filter")
	}
	if slices.Contains(itemIDs(mailOwner.expect(http.StatusOK, "GET", "/v1/automation-rules", nil)), rule) {
		t.Fatal("the shared list carries a personal filter")
	}
	mailOwner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/automation-rules", map[string]any{"name": "x", "inbox_id": inbox, "trigger": "mail_received", "actions": []map[string]any{{"type": "add_note", "body": "hi"}}})
	for name, c := range map[string]*client{"workspace owner": tm.owner, "admin": admin, "key": key, "agent": tm.agent} {
		if ids := itemIDs(c.expect(http.StatusOK, "GET", "/v1/automation-rules", nil)); slices.Contains(ids, rule) {
			t.Errorf("%s lists the personal filter", name)
		}
		c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/automation-rules/"+rule, nil)
		c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/automation-rules?inbox_id="+inbox, nil)
		c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/automation-rules/"+rule+"/runs", nil)
		c.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/automation-rules/"+rule, map[string]any{"name": "Taken"})
		c.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/automation-rules/"+rule, nil)
	}
	adminID := admin.expect(http.StatusOK, "GET", "/v1/me", nil).body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)
	adminBox := admin.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Admin mail", "slug": unique("adm"), "owner_member_id": adminID}).body["inbox"].(map[string]any)["id"].(string)
	grant := h.client()
	grant.bearer = h.oauthToken(admin, tm.ws, "Own App", testOrigin, []string{"automations:read", "automations:manage"})
	adminRule := grant.expect(http.StatusCreated, "POST", "/v1/automation-rules", map[string]any{
		"name": "Receipts", "inbox_id": adminBox, "trigger": "mail_received", "conditions": []map[string]any{contains("subject", "receipt")}, "actions": []map[string]any{{"type": "archive"}},
	}).str("id")
	grant.expect(http.StatusOK, "PATCH", "/v1/automation-rules/"+adminRule, map[string]any{"name": "Receipts from apps"})
	grant.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/automation-rules/"+rule, nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/automation-rules/"+adminRule, nil)
	mailOwner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/automation-rules/"+adminRule, nil)
	mailOwner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/automation-rules/"+rule, map[string]any{"inbox_id": tm.inbox})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/automation-rules/"+workspaceRule, map[string]any{"inbox_id": inbox, "trigger": "mail_received"})
	tm.owner.expect(http.StatusOK, "PUT", "/v1/automation-rules/order", map[string]any{"ids": []string{workspaceRule}})
	mailOwner.expect(http.StatusOK, "PUT", "/v1/automation-rules/order", map[string]any{"ids": []string{rule}})
	if n := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1 AND target_id = $2 AND details::text <> '{}'", tm.ws, rule); n != 0 {
		t.Fatalf("%d audit rows of the personal filter with details", n)
	}
	mailOwner.expect(http.StatusCreated, "POST", "/v1/automation-rules", map[string]any{
		"name": "Spam", "inbox_id": inbox, "trigger": "mail_received", "conditions": []map[string]any{contains("from", "spammer.example")}, "actions": []map[string]any{{"type": "trash"}},
	})

	r := h.ingest(address, buildMail(mailOpts{from: "alerts@monitor.example.net", to: address, subject: "Disk full", messageID: newMessageID(), body: "90%"}), nil)
	conv, msg := r.str("conversation_id"), r.str("message_id")
	h.runAutomations(tm.ws)
	got := mailOwner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if got.body["starred"] != true || got.str("priority") == "urgent" {
		t.Fatalf("after the personal filter: %s", got.raw)
	}
	if outcomes := runOutcomes(mailOwner, rule); len(outcomes) != 1 || outcomes[0] != "applied" {
		t.Fatalf("filter runs: %v", outcomes)
	}
	if runs := ruleRuns(tm.owner, workspaceRule); len(runs) != 0 {
		t.Fatalf("the workspace rule ran on a personal mailbox: %v", runs)
	}

	junk := h.ingest(address, buildMail(mailOpts{from: "offers@spammer.example", to: address, subject: "Cheap", messageID: newMessageID(), body: "Buy"}), nil).str("conversation_id")
	h.runAutomations(tm.ws)
	if n := h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND id = $2 AND trashed_at IS NOT NULL", tm.ws, junk); n != 1 {
		t.Fatal("the personal filter did not trash")
	}
	if n := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1 AND target_id = $2 AND action = 'conversation.trashed' AND details::text = '{}'", tm.ws, junk); n != 1 {
		t.Fatalf("personal filter trash audit rows without details: %d", n)
	}

	mailOwner.expect(http.StatusOK, "GET", "/v1/messages/"+msg+"/filter-suggestion", nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/messages/"+msg+"/filter-suggestion", nil)
	key.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/messages/"+msg+"/filter-suggestion", nil)
	mailOwner.expect(http.StatusNoContent, "DELETE", "/v1/automation-rules/"+rule, nil)
}

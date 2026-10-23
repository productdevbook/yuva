package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

func TestFrontMigration(t *testing.T) {
	d := newDeskRun(t)
	address, channel := d.emailChannel(t, d.tm.inbox, "support")
	billingInbox := d.tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Billing", "slug": "billing"}).body["inbox"].(map[string]any)["id"].(string)
	d.emailChannel(t, billingInbox, "billing")
	id := d.tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{
		"source": "front", "preset": "front", "source_address": address, "channel_id": channel,
	}).str("id")
	path := "/v1/migrations/" + id
	token := helpdesktest.FrontToken

	if got := d.tm.owner.expect(http.StatusOK, "GET", path, nil).body["actions"].([]any); !slices.Contains(got, any("set_credentials")) || slices.Contains(got, any("sign_in")) {
		t.Fatalf("actions: %v", got)
	}
	d.tm.owner.expectProblem(http.StatusConflict, "migration_state", "POST", path+"/sign-in", map[string]any{"client_id": "x", "client_secret": "y"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token, "subdomain": "acme"})
	d.tm.owner.expectProblem(http.StatusConflict, "migration_state", "POST", path+"/test", nil)
	other := newTeam(t, d.h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", path+"/test", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", path, nil)
	d.tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token})

	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", map[string]any{"kind": "token", "token": "wrong-token"})
	if res := d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil); res.body["ok"] != false || d.state(id) != "draft" {
		t.Fatalf("a wrong token: %s", res.raw)
	}
	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token})
	m := d.tm.owner.expect(http.StatusOK, "GET", path, nil)
	if cred := m.body["credentials"].(map[string]any); cred["kind"] != "token" || cred["server"].(map[string]any)["host"] != helpdesk.FrontAPIHost || strings.Contains(string(m.raw), token) {
		t.Fatalf("credentials: %s", m.raw)
	}
	if res := d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil); res.body["ok"] != true || d.state(id) != "connected" {
		t.Fatalf("test: %s", d.tm.owner.expect(http.StatusOK, "GET", path, nil).raw)
	}
	folders := d.tm.owner.expect(http.StatusOK, "GET", path+"/folders", nil).body["items"].([]any)
	if len(folders) != 2 || folders[1].(map[string]any)["name"] != "Billing" || folders[1].(map[string]any)["address"] != "billing@front.example.com" {
		t.Fatalf("folders: %v", folders)
	}
	d.tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"options": map[string]any{
		"inboxes": []any{map[string]any{"folder": "Billing", "inbox_id": billingInbox}},
		"people":  []any{map[string]any{"address": "ece@front.example.com", "member_id": d.owner}},
	}})

	d.desk.FrontThrottle = 1
	d.tm.owner.expect(http.StatusOK, "POST", path+"/scan", nil)
	d.work(t, func() bool { return d.state(id) == "ready" })
	est := d.tm.owner.expect(http.StatusOK, "GET", path, nil).body["estimate"].(map[string]any)
	if num(est["conversations"]) != 4 || num(est["skipped"].(map[string]any)["trash"]) != 1 {
		t.Fatalf("dry run: %v", est)
	}

	d.tm.owner.expect(http.StatusOK, "POST", path+"/start", nil)
	d.work(t, func() bool { return d.state(id) == "syncing" })
	if n := d.h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND migration_id = $2", d.tm.ws, id); n != 4 {
		t.Fatalf("%d conversations, want 4", n)
	}
	conv := func(subject string) string {
		return d.str(t, "SELECT id::text FROM conversations WHERE workspace_id = $1 AND subject = $2", d.tm.ws, subject)
	}
	login, invoice, waiting, refund := conv("Cannot log in"), conv("Invoice question"), conv("Waiting on a reply"), conv("Refund")
	for c, want := range map[string]string{
		login:   "open " + d.tm.inbox + " " + channel + " " + d.owner,
		invoice: "closed " + d.tm.inbox + " " + channel + " -",
		waiting: "pending " + d.tm.inbox + " " + channel + " -",
		refund:  "closed " + billingInbox + " - -",
	} {
		got := d.str(t, "SELECT status || ' ' || inbox_id || ' ' || coalesce(channel_id::text, '-') || ' ' || coalesce(assignee_id::text, '-') FROM conversations WHERE workspace_id = $1 AND id = $2", d.tm.ws, c)
		if got != want {
			t.Errorf("conversation %s: %s, want %s", c, got, want)
		}
	}
	if got := d.str(t, "SELECT string_agg(kind || ':' || coalesce(direction, '-'), ' ' ORDER BY created_at) FROM messages WHERE workspace_id = $1 AND conversation_id = $2", d.tm.ws, login); got != "message:in note:- message:out" {
		t.Errorf("Cannot log in messages: %s", got)
	}
	if got := d.str(t, "SELECT string_agg(kind || ':' || coalesce(direction, '-'), ' ' ORDER BY created_at) FROM messages WHERE workspace_id = $1 AND conversation_id = $2", d.tm.ws, refund); got != "message:in note:-" {
		t.Errorf("Refund messages: %s", got)
	}
	if got := d.str(t, "SELECT string_agg(l.name, ',' ORDER BY l.name) FROM conversation_labels cl JOIN labels l ON l.workspace_id = cl.workspace_id AND l.id = cl.label_id WHERE cl.workspace_id = $1 AND cl.conversation_id = $2", d.tm.ws, login); got != "login,vip" {
		t.Errorf("labels: %s", got)
	}
	if got := d.str(t, "SELECT string_agg(filename, ',') FROM attachments WHERE workspace_id = $1", d.tm.ws); got != "notes.txt" {
		t.Errorf("attachments: %s", got)
	}
	if got := d.str(t, "SELECT c.attributes->>'front_id' || ' ' || (c.attributes->>'phone') FROM conversations v JOIN contacts c ON c.workspace_id = v.workspace_id AND c.id = v.contact_id WHERE v.workspace_id = $1 AND v.id = $2", d.tm.ws, login); got != "ayse@example.org +905550100200" {
		t.Errorf("contact attributes: %s", got)
	}

	at := d.h.clock.Now().Add(time.Minute)
	d.h.clock.Advance(3 * time.Minute)
	changed := fmt.Sprintf(`{"_total": 1, "_pagination": {"next": null}, "_results": [{"id": "cnv_1c", "subject": "Waiting on a reply", "status": "assigned",
		"status_category": "open", "is_closed": false, "created_at": 1760200000, "updated_at": %d, "assignee": null,
		"recipient": {"name": "Carol Ng", "handle": "carol@example.com", "role": "from"}, "tags": [{"id": "tag_late", "name": "late"}]}]}`, at.Unix())
	d.desk.Override("front/search.json", []byte(changed))
	var page map[string]any
	raw, _ := helpdesktest.Fixture("front/messages_cnv_1c.json")
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	page["_results"] = append(page["_results"].([]any), map[string]any{
		"id": "msg_9", "type": "email", "is_inbound": true, "created_at": at.Unix(), "draft_mode": nil, "author": nil,
		"recipients": []any{map[string]any{"name": "Carol Ng", "handle": "carol@example.com", "role": "from"}},
		"body":       "<p>Still waiting.</p>", "text": "Still waiting.", "attachments": []any{},
	})
	messages, _ := json.Marshal(page)
	d.desk.Override("front/messages_cnv_1c.json", messages)
	d.work(t, func() bool {
		return d.h.count("SELECT count(*) FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND kind <> 'event'", d.tm.ws, waiting) == 2
	})
	for range 2 {
		d.syncPass(t, id)
	}
	if n := d.h.count("SELECT count(*) FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND kind <> 'event'", d.tm.ws, waiting); n != 2 {
		t.Errorf("after sync the conversation has %d messages, want 2", n)
	}
	if n := d.h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND migration_id = $2", d.tm.ws, id); n != 4 {
		t.Errorf("%d conversations after sync, want 4", n)
	}

	reqs := d.desk.Requests()
	for _, r := range reqs {
		if r.Method != http.MethodGet || r.Host != helpdesk.FrontAPIHost {
			t.Errorf("%s %s%s was sent", r.Method, r.Host, r.Path)
		}
		if r.Auth != "Bearer "+token && r.Auth != "Bearer wrong-token" {
			t.Errorf("%s%s carried %q", r.Host, r.Path, r.Auth)
		}
	}
	d.checkRequests(t, helpdesk.FrontAPIHost, "", helpdesktest.FrontLimit/2, 0)
	for i, r := range reqs {
		n := 0
		for _, o := range reqs[:i+1] {
			if strings.HasPrefix(o.Path, "/conversations/search/") && o.At.After(r.At.Add(-time.Minute)) {
				n++
			}
		}
		if n > helpdesktest.FrontLimit*40/100 {
			t.Errorf("%d searches in the minute up to %s, want at most 40%% of %d", n, r.At.Format(time.TimeOnly), helpdesktest.FrontLimit)
		}
	}
	if !slices.ContainsFunc(reqs, func(r helpdesktest.Request) bool { return strings.HasPrefix(r.Path, "/conversations/search/") }) {
		t.Error("sync never searched")
	}

	d.tm.owner.expect(http.StatusOK, "POST", path+"/cancel", nil)
	if n := d.h.count("SELECT count(*) FROM migration_credentials WHERE workspace_id = $1", d.tm.ws); n != 0 {
		t.Errorf("%d credentials left after cancel", n)
	}
}

package api_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

func TestFreshdeskMigration(t *testing.T) {
	d := newDeskRun(t)
	address, channel := d.emailChannel(t, d.tm.inbox, "support")
	payInbox := d.tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Pay", "slug": "pay"}).body["inbox"].(map[string]any)["id"].(string)
	_, payChannel := d.emailChannel(t, payInbox, "pay")

	var preset map[string]any
	for _, p := range d.tm.owner.expect(http.StatusOK, "GET", "/v1/migrations/presets", nil).body["items"].([]any) {
		if p.(map[string]any)["id"] == "freshdesk" {
			preset = p.(map[string]any)
		}
	}
	if preset == nil || preset["auth"].(any) == nil || fmt.Sprint(preset["auth"]) != "[token]" || fmt.Sprint(preset["token_fields"]) != "[token subdomain]" ||
		preset["available"] != true || preset["guide"] != "migrate-freshdesk" {
		t.Fatalf("preset: %v", preset)
	}

	id := d.tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{
		"source": "freshdesk", "preset": "freshdesk", "source_address": address, "channel_id": channel,
	}).str("id")
	path := "/v1/migrations/" + id
	if got := d.tm.owner.expect(http.StatusOK, "GET", path, nil).body["actions"].([]any); !slices.Contains(got, any("set_credentials")) || slices.Contains(got, any("sign_in")) {
		t.Fatalf("actions: %v", got)
	}
	d.tm.owner.expectProblem(http.StatusConflict, "migration_state", "POST", path+"/sign-in", map[string]any{"client_id": "x", "client_secret": "y"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": helpdesktest.FreshdeskKey})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": helpdesktest.FreshdeskKey, "subdomain": "Not A Subdomain"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": helpdesktest.FreshdeskKey, "subdomain": "acme", "secret": "x"})
	other := newTeam(t, d.h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": helpdesktest.FreshdeskKey, "subdomain": "acme"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", path+"/test", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", path+"/folders", nil)
	d.tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": helpdesktest.FreshdeskKey, "subdomain": "acme"})

	connect := func(key string) response {
		d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", map[string]any{"kind": "token", "token": key, "subdomain": helpdesktest.FreshdeskSubdomain})
		return d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil)
	}
	for key, code := range map[string]string{helpdesktest.FreshdeskAgentKey: "not_admin", "a-wrong-key": "auth_failed"} {
		res := connect(key)
		steps := res.body["steps"].([]any)
		if res.body["ok"] != false || steps[len(steps)-1].(map[string]any)["code"] != code || d.state(id) != "draft" {
			t.Fatalf("key %s: %s", key, res.raw)
		}
	}
	res := connect(helpdesktest.FreshdeskKey)
	if res.body["ok"] != true || d.state(id) != "connected" {
		t.Fatalf("test: %s", res.raw)
	}
	got := d.tm.owner.expect(http.StatusOK, "GET", path, nil)
	cred := got.body["credentials"].(map[string]any)
	if cred["kind"] != "token" || cred["server"].(map[string]any)["host"] != "acme.freshdesk.com" || strings.Contains(string(got.raw), helpdesktest.FreshdeskKey) {
		t.Fatalf("credentials: %s", got.raw)
	}
	folders := d.tm.owner.expect(http.StatusOK, "GET", path+"/folders", nil).body["items"].([]any)
	if len(folders) != 3 || folders[0].(map[string]any)["name"] != "No product" || folders[1].(map[string]any)["name"] != "Acme Pay" ||
		folders[1].(map[string]any)["address"] != "pay@acme.example.com" {
		t.Fatalf("folders: %v", folders)
	}

	d.tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"options": map[string]any{
		"inboxes": []any{map[string]any{"folder": "Acme Pay", "inbox_id": payInbox}},
		"people":  []any{map[string]any{"address": "ece@acme.example.com", "member_id": d.owner}},
		"labels":  []any{map[string]any{"folder": "vip", "action": "none"}},
	}})
	d.tm.owner.expect(http.StatusOK, "POST", path+"/scan", nil)
	d.work(t, func() bool { return d.state(id) == "ready" })
	est := d.tm.owner.expect(http.StatusOK, "GET", path, nil).body["estimate"].(map[string]any)
	if num(est["conversations"]) != 6 || num(est["skipped"].(map[string]any)["spam"]) != 1 {
		t.Fatalf("dry run: %v", est)
	}

	d.tm.owner.expect(http.StatusOK, "POST", path+"/start", nil)
	d.work(t, func() bool { return d.state(id) == "syncing" })

	if n := d.h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND migration_id = $2", d.tm.ws, id); n != 6 {
		t.Fatalf("%d conversations, want 6", n)
	}
	conv := func(subject string) string {
		return d.str(t, "SELECT id::text FROM conversations WHERE workspace_id = $1 AND subject = $2", d.tm.ws, subject)
	}
	login, invoice, pricing, refund, vendor := conv("Cannot log in"), conv("Invoice question"), conv("Pricing for teams"), conv("Refund"), conv("Waiting on vendor")
	for c, want := range map[string]string{
		login:   "open " + payInbox + " " + payChannel + " " + d.owner,
		invoice: "closed " + d.tm.inbox + " " + channel + " -",
		pricing: "pending " + d.tm.inbox + " - -",
		refund:  "closed " + payInbox + " " + payChannel + " " + d.owner,
		vendor:  "open " + d.tm.inbox + " " + channel + " " + d.owner,
	} {
		got := d.str(t, "SELECT status || ' ' || inbox_id || ' ' || coalesce(channel_id::text, '-') || ' ' || coalesce(assignee_id::text, '-') FROM conversations WHERE workspace_id = $1 AND id = $2", d.tm.ws, c)
		if got != want {
			t.Errorf("conversation %s: %s, want %s", c, got, want)
		}
	}
	if got := d.str(t, "SELECT string_agg(kind || ':' || coalesce(direction, '-'), ' ' ORDER BY created_at, id) FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND kind <> 'event'", d.tm.ws, login); got !=
		"message:in message:out message:in note:- note:- message:in" {
		t.Errorf("Cannot log in: %s", got)
	}
	if got := d.str(t, "SELECT string_agg(direction, ' ') FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND kind = 'message'", d.tm.ws, invoice); got != "out" {
		t.Errorf("the outbound ticket: %s", got)
	}
	if got := d.str(t, "SELECT string_agg(filename, ',' ORDER BY filename) FROM attachments WHERE workspace_id = $1", d.tm.ws); got != "guide.txt,screenshot.png" {
		t.Errorf("attachments: %s", got)
	}
	if got := d.str(t, "SELECT string_agg(l.name, ',' ORDER BY l.name) FROM conversation_labels cl JOIN labels l ON l.workspace_id = cl.workspace_id AND l.id = cl.label_id WHERE cl.workspace_id = $1 AND cl.conversation_id = $2", d.tm.ws, login); got != "login" {
		t.Errorf("labels: %s", got)
	}
	if got := d.str(t, "SELECT c.attributes->>'freshdesk_id' || ' ' || (c.attributes->>'phone') FROM conversations v JOIN contacts c ON c.workspace_id = v.workspace_id AND c.id = v.contact_id WHERE v.workspace_id = $1 AND v.id = $2", d.tm.ws, login); got != "3001 +90 555 0100" {
		t.Errorf("requester attributes: %s", got)
	}
	if got := d.str(t, "SELECT c.attributes->>'freshdesk_id' FROM conversations v JOIN contacts c ON c.workspace_id = v.workspace_id AND c.id = v.contact_id WHERE v.workspace_id = $1 AND v.id = $2", d.tm.ws, pricing); got != "3003" {
		t.Errorf("a requester without an address: %s", got)
	}
	if n := d.h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1", other.ws); n != 0 {
		t.Errorf("%d conversations in another workspace", n)
	}

	at := d.h.clock.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	d.h.clock.Advance(3 * time.Minute)
	d.desk.Override("freshdesk/tickets.json", []byte(`[{"id": 103, "subject": "Pricing for teams", "status": 2, "source": 7, "requester_id": 3003, "responder_id": null,
		"product_id": 9002, "tags": ["sales"], "spam": false, "deleted": false, "created_at": "2026-09-03T08:00:00Z", "updated_at": "`+at+`",
		"requester": {"id": 3003, "name": "Mina Kaya", "email": null, "phone": null, "mobile": "+90 555 0103"}}]`))
	convs, err := helpdesktestFixture("freshdesk/conversations_103.json")
	if err != nil {
		t.Fatal(err)
	}
	convs = strings.Replace(convs, `}]}
]`, `}]},
 {"id": 2102, "ticket_id": 103, "incoming": true, "private": false, "source": 7, "user_id": 3003, "from_email": null,
  "body": "<div>And for ten people?</div>", "body_text": "And for ten people?", "created_at": "`+at+`", "updated_at": "`+at+`", "attachments": []}
]`, 1)
	d.desk.Override("freshdesk/conversations_103.json", []byte(convs))
	d.work(t, func() bool {
		return d.h.count("SELECT count(*) FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND kind <> 'event'", d.tm.ws, pricing) == 3
	})
	for range 3 {
		d.syncPass(t, id)
	}
	if n := d.h.count("SELECT count(*) FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND kind <> 'event'", d.tm.ws, pricing); n != 3 {
		t.Errorf("after sync Pricing has %d messages, want 3", n)
	}
	if got := d.str(t, "SELECT status FROM conversations WHERE workspace_id = $1 AND id = $2", d.tm.ws, pricing); got != "open" {
		t.Errorf("a customer's new message left the conversation %s", got)
	}

	basic := "Basic "
	for _, r := range d.desk.Requests() {
		switch r.Host {
		case "acme.freshdesk.com":
			if r.Method != http.MethodGet || !strings.HasPrefix(r.Auth, basic) {
				t.Errorf("%s %s with %q", r.Method, r.Path, r.Auth)
			}
		case helpdesktest.FreshdeskFilesHost:
			if r.Method != http.MethodGet || r.Auth != "" {
				t.Errorf("%s %s%s with %q", r.Method, r.Host, r.Path, r.Auth)
			}
		default:
			t.Errorf("a request reached %s", r.Host)
		}
	}
	d.checkRequests(t, "acme.freshdesk.com", "", helpdesktest.FreshdeskLimit/2, 0)

	d.tm.owner.expect(http.StatusOK, "POST", path+"/cancel", nil)
	if n := d.h.count("SELECT count(*) FROM migration_credentials WHERE workspace_id = $1", d.tm.ws); n != 0 {
		t.Errorf("%d credentials left after cancel", n)
	}
}

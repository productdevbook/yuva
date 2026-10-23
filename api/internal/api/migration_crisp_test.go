package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

func TestCrispMigration(t *testing.T) {
	d := newDeskRun(t)
	address, channel := d.emailChannel(t, d.tm.inbox, "support")
	id := d.tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{
		"source": "crisp", "preset": "crisp", "source_address": address, "channel_id": channel,
		"options": map[string]any{"people": []any{map[string]any{"address": "deniz@acme.example.com", "member_id": d.owner}}},
	}).str("id")
	path := "/v1/migrations/" + id
	token := map[string]any{"kind": "token", "token": helpdesktest.CrispIdentifier, "secret": helpdesktest.CrispKey, "website_id": helpdesktest.CrispWebsite}

	if got := d.tm.owner.expect(http.StatusOK, "GET", path, nil).body["actions"].([]any); !slices.Contains(got, any("set_credentials")) || slices.Contains(got, any("sign_in")) {
		t.Fatalf("actions: %v", got)
	}
	presets := d.tm.owner.expect(http.StatusOK, "GET", "/v1/migrations/presets", nil).body["items"].([]any)
	for _, p := range presets {
		p := p.(map[string]any)
		if p["id"] != "crisp" {
			continue
		}
		fields := p["token_fields"].([]any)
		if !slices.Equal(fields, []any{"token", "secret", "website_id"}) || p["auth"].([]any)[0] != "token" || num(p["budget"].(map[string]any)["requests_per_day"]) != 5000 {
			t.Errorf("preset: %v", p)
		}
	}
	without := func(key string) map[string]any {
		out := map[string]any{}
		for k, v := range token {
			if k != key {
				out[k] = v
			}
		}
		return out
	}
	d.tm.owner.expectProblem(http.StatusConflict, "migration_state", "POST", path+"/sign-in", map[string]any{"client_id": "x", "client_secret": "y"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", without("website_id"))
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", without("secret"))
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": "a", "secret": "b", "website_id": "not-a-uuid"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": "a", "secret": "b", "website_id": helpdesktest.CrispWebsite, "subdomain": "acme"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"username": "a", "password": "b", "website_id": helpdesktest.CrispWebsite})
	other := newTeam(t, d.h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", path+"/credentials", token)
	d.tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PUT", path+"/credentials", token)

	wrong := map[string]any{"kind": "token", "token": helpdesktest.CrispIdentifier, "secret": "stale", "website_id": helpdesktest.CrispWebsite}
	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", wrong)
	if res := d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil); res.body["ok"] != false {
		t.Fatalf("a stale key connected: %s", res.raw)
	}

	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", token)
	got := d.tm.owner.expect(http.StatusOK, "GET", path, nil)
	if cred := got.body["credentials"].(map[string]any); cred["kind"] != "token" || cred["server"].(map[string]any)["host"] != "api.crisp.chat" {
		t.Fatalf("credentials: %v", cred)
	}
	if strings.Contains(string(got.raw), helpdesktest.CrispKey) {
		t.Fatal("the key was answered")
	}
	if res := d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil); res.body["ok"] != true {
		t.Fatalf("test: %s", res.raw)
	}
	d.tm.owner.expect(http.StatusOK, "POST", path+"/scan", nil)
	d.work(t, func() bool { return d.state(id) == "ready" })
	est := d.tm.owner.expect(http.StatusOK, "GET", path, nil).body["estimate"].(map[string]any)
	if num(est["conversations"]) != 5 {
		t.Fatalf("dry run: %v", est)
	}
	d.tm.owner.expect(http.StatusOK, "POST", path+"/start", nil)
	d.work(t, func() bool { return d.state(id) == "syncing" })

	conv := func(subject string) string {
		return d.str(t, "SELECT id::text FROM conversations WHERE workspace_id = $1 AND subject = $2", d.tm.ws, subject)
	}
	login, invoice := conv("Cannot log in"), conv("Invoice question")
	if got := d.str(t, "SELECT status || ' ' || coalesce(assignee_id::text, '-') FROM conversations WHERE workspace_id = $1 AND id = $2", d.tm.ws, login); got != "open "+d.owner {
		t.Errorf("Cannot log in: %s", got)
	}
	if got := d.str(t, "SELECT status FROM conversations WHERE workspace_id = $1 AND id = $2", d.tm.ws, invoice); got != "closed" {
		t.Errorf("Invoice question: %s", got)
	}
	if got := d.str(t, "SELECT string_agg(kind || ':' || coalesce(direction, '-'), ' ' ORDER BY created_at, id) FROM messages WHERE workspace_id = $1 AND conversation_id = $2", d.tm.ws, login); got != "message:in message:in message:in message:in note:- message:out" {
		t.Errorf("Cannot log in messages: %s", got)
	}
	if n := d.h.count("SELECT count(DISTINCT author_contact_id) FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND direction = 'in'", d.tm.ws, invoice); n != 2 {
		t.Errorf("Invoice question has %d distinct senders, want its person and the second participant", n)
	}
	if n := d.h.count("SELECT count(*) FROM messages WHERE workspace_id = $1 AND conversation_id = $2", d.tm.ws, invoice); n != 3 {
		t.Errorf("Invoice question has %d messages, want 3 (the files outside Crisp's storage stay behind)", n)
	}
	if got := d.str(t, "SELECT string_agg(filename, ',' ORDER BY filename) FROM attachments WHERE workspace_id = $1", d.tm.ws); !strings.Contains(got, "login.png") || strings.Contains(got, "outside") {
		t.Errorf("attachments: %s", got)
	}
	if n := d.h.count("SELECT count(*) FROM contacts WHERE workspace_id = $1 AND name = 'visitor 4821'", d.tm.ws); n != 1 {
		t.Errorf("%d contacts for the anonymous visitor, want 1", n)
	}
	if got := d.str(t, "SELECT string_agg(l.name, ',' ORDER BY l.name) FROM conversation_labels cl JOIN labels l ON l.workspace_id = cl.workspace_id AND l.id = cl.label_id WHERE cl.workspace_id = $1 AND cl.conversation_id = $2", d.tm.ws, login); got != "login,vip" {
		t.Errorf("labels: %s", got)
	}

	before := d.h.count("SELECT count(*) FROM messages WHERE workspace_id = $1", d.tm.ws)
	d.syncPass(t, id)
	if after := d.h.count("SELECT count(*) FROM messages WHERE workspace_id = $1", d.tm.ws); after != before {
		t.Errorf("a sync pass changed the message count from %d to %d", before, after)
	}

	basic := crispBasic(helpdesktest.CrispIdentifier, helpdesktest.CrispKey)
	for _, r := range d.desk.Requests() {
		switch {
		case r.Method != http.MethodGet:
			t.Errorf("%s %s%s was sent", r.Method, r.Host, r.Path)
		case r.Host == "api.crisp.chat" && r.Auth != basic && r.Auth != crispBasic(helpdesktest.CrispIdentifier, "stale"):
			t.Errorf("%s was fetched with %q", r.Path, r.Auth)
		case r.Host != "api.crisp.chat" && r.Host != helpdesktest.CrispStorage:
			t.Errorf("a request reached %s%s", r.Host, r.Path)
		case r.Host == helpdesktest.CrispStorage && r.Auth != "":
			t.Errorf("the file on %s was fetched with %q", r.Host, r.Auth)
		}
	}
	d.checkRequests(t, "api.crisp.chat", "", 60, 0)
}

func crispBasic(user, password string) string {
	r, _ := http.NewRequest(http.MethodGet, "https://x.example", nil)
	r.SetBasicAuth(user, password)
	return r.Header.Get("Authorization")
}

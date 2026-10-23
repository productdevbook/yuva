package api_test

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/productdevbook/yuva/api/internal/api"
)

var migrationToolNames = []string{
	"detect_mail_source", "list_migrations", "get_migration", "create_migration", "update_migration", "test_migration_connection",
	"scan_migration", "preview_migration", "start_migration", "pause_migration", "resume_migration", "cancel_migration",
	"finish_migration", "retry_migration_items", "list_migration_errors", "send_migration_probe", "remove_migration_history",
}

func TestForwardingAddress(t *testing.T) {
	h := newHarnessWith(t, func(d *api.Deps) { d.Ingress.ForwardingDomain = testForwardingDomain })
	tm := newTeam(t, h)
	address := unique("help") + "@example.com"
	ch := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Help", "email": map[string]any{"address": address},
	})
	id := ch.str("id")
	first := ch.str("forwarding_address")
	if !regexp.MustCompile(`^[a-z0-9]{16}@` + regexp.QuoteMeta(testForwardingDomain) + `$`).MatchString(first) {
		t.Fatalf("forwarding address %q", first)
	}
	raw := mailText(unique("fwd")+"@example.org", "Lea Berg <lea@example.org>", address, "Forwarded to Yuva", time.Now(), "", "Hello through forwarding.")
	r := h.ingest(first, []byte(raw), nil)
	if r.status != http.StatusAccepted || r.str("status") != "stored" {
		t.Fatalf("mail at the forwarding address: %d %v", r.status, r.body)
	}
	if n := h.count("SELECT count(*) FROM conversations WHERE id = $1 AND channel_id = $2", r.str("conversation_id"), id); n != 1 {
		t.Fatal("mail at the forwarding address did not reach the channel")
	}
	if r := h.ingest(strings.ToUpper(strings.Split(first, "@")[0])+"+tag@"+testForwardingDomain, []byte(raw), nil); r.str("status") != "duplicate" {
		t.Fatalf("a tagged, upper-case forwarding address: %d %v", r.status, r.body)
	}

	other := newTeam(t, h)
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/channels/"+id+"/forwarding-address/rotate", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/channels/"+id+"/forwarding-address/rotate", nil)
	rotated := tm.owner.expect(http.StatusOK, "POST", "/v1/channels/"+id+"/forwarding-address/rotate", nil).str("forwarding_address")
	if rotated == first || !strings.HasSuffix(rotated, "@"+testForwardingDomain) {
		t.Fatalf("rotated %q from %q", rotated, first)
	}
	if r := h.ingest(first, []byte(raw), nil); r.status != http.StatusNotFound {
		t.Fatalf("the old forwarding address: %d", r.status)
	}
	chat := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "chat", "name": "Site", "chat": map[string]any{"allowed_origins": []string{"https://example.com"}},
	})
	if chat.body["forwarding_address"] != nil {
		t.Fatal("a chat channel has a forwarding address")
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/channels/"+chat.str("id")+"/forwarding-address/rotate", nil)

	plain := newHarness(t)
	pt := newTeam(t, plain)
	pc := pt.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+pt.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Help", "email": map[string]any{"address": unique("help") + "@example.com"},
	})
	if pc.body["forwarding_address"] != nil {
		t.Fatal("a forwarding address on a server without a forwarding domain")
	}
	pt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/channels/"+pc.str("id")+"/forwarding-address/rotate", nil)
}

func TestForwardingDomainCarriesAddresses(t *testing.T) {
	fwd := unique("fwd") + ".example"
	h := newHarnessWith(t, func(d *api.Deps) { d.Ingress.ForwardingDomain = fwd })
	tm := newTeam(t, h)
	create := func(address string) response {
		return tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
			"kind": "email", "name": address, "email": map[string]any{"address": address},
		})
	}
	channelOf := func(to string) string {
		t.Helper()
		raw := mailText(unique("m")+"@example.org", "Lea Berg <lea@example.org>", to, "Hello", time.Now(), "", "Hello.")
		r := h.ingest(to, []byte(raw), nil)
		if r.status != http.StatusAccepted || r.str("status") != "stored" {
			t.Fatalf("mail to %s: %d %v", to, r.status, r.body)
		}
		return tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+r.str("conversation_id"), nil).str("channel_id")
	}
	unknown := func(to string) {
		t.Helper()
		raw := mailText(unique("m")+"@example.org", "Lea Berg <lea@example.org>", to, "Hello", time.Now(), "", "Hello.")
		if r := h.ingest(to, []byte(raw), nil); r.status != http.StatusNotFound {
			t.Fatalf("mail to %s: %d %v", to, r.status, r.body)
		}
	}

	external := create(unique("help") + "@example.com")
	if got := channelOf(external.str("forwarding_address")); got != external.str("id") {
		t.Fatal("a forwarding token did not reach its channel")
	}
	unknown("zzzzzzzzzzzzzzzz@" + fwd)
	real := create("hello@" + fwd)
	if got := channelOf("hello@" + fwd); got != real.str("id") {
		t.Fatal("a channel address on the forwarding domain did not reach its channel")
	}
	if got := channelOf("hello+tag@" + fwd); got != real.str("id") {
		t.Fatal("a tagged channel address on the forwarding domain did not reach its channel")
	}
	unknown("zzzzzzzzzzzzzzzz@" + fwd)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "email", "name": "all", "email": map[string]any{"address": "*@" + fwd},
	})
	unknown("zzzzzzzzzzzzzzzz@" + fwd)
}

func TestMCPMigrationTools(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	resource := testOrigin + "/mcp"
	cs := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", resource, []string{"inboxes:read", "migrations:manage"}))
	tools := toolNames(t, cs)
	for _, name := range migrationToolNames {
		if !slices.Contains(tools, name) {
			t.Fatalf("%s not listed to an owner with migrations:manage", name)
		}
	}
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name == "remove_migration_history" && (tool.Annotations == nil || tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint) {
			t.Fatal("remove_migration_history is not marked destructive")
		}
	}
	prompts, err := cs.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(prompts.Prompts, func(p *mcp.Prompt) bool { return p.Name == "move_to_yuva" }) {
		t.Fatal("move_to_yuva not listed")
	}
	text := promptText(t, cs, "move_to_yuva", map[string]string{"address": "support@example.com"})
	if !strings.Contains(text, "connect_url") || !strings.Contains(text, "Never ask me for a password") || !strings.Contains(text, "only after I confirm") && !strings.Contains(text, "Only after I confirm") {
		t.Fatalf("move_to_yuva:\n%s", text)
	}

	address := unique("support") + "@example.com"
	ch := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Support", "email": map[string]any{"address": address},
	}).str("id")
	created, errText := callTool(t, cs, "create_migration", map[string]any{"address": address, "provider": "imap", "inbox_id": tm.inbox, "channel_id": ch})
	if errText != "" {
		t.Fatal(errText)
	}
	id := created["id"].(string)
	if created["state"] != "draft" || created["connect_url"] != testOrigin+"/migrations/"+id+"/connect?workspace="+tm.ws || created["connected"] != false {
		t.Fatalf("create_migration: %v", created)
	}
	got, errText := callTool(t, cs, "get_migration", map[string]any{"migration_id": id})
	if errText != "" || got["address"] != address || !slices.Contains(strs(got["actions"]), "set_credentials") {
		t.Fatalf("get_migration: %v %s", got, errText)
	}
	if _, errText := callTool(t, cs, "remove_migration_history", map[string]any{"migration_id": id, "confirm_count": 0}); !strings.Contains(errText, "completed or cancelled") {
		t.Fatalf("remove history of a draft: %q", errText)
	}
	if _, errText := callTool(t, cs, "finish_migration", map[string]any{"migration_id": id}); !strings.Contains(errText, "syncs") {
		t.Fatalf("finish a draft: %q", errText)
	}
	rr, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "yuva://migration/" + id})
	if err != nil || len(rr.Contents) != 1 || !strings.Contains(rr.Contents[0].Text, address) {
		t.Fatalf("resource: %v %v", rr, err)
	}

	other := newTeam(t, h)
	ocs := h.mcpSession(h.oauthToken(other.owner, other.ws, "Claude Code", resource, []string{"migrations:manage"}))
	if _, errText := callTool(t, ocs, "get_migration", map[string]any{"migration_id": id}); !strings.Contains(errText, "not_found") && !strings.Contains(errText, "no such migration") {
		t.Fatalf("another workspace's migration: %q", errText)
	}

	narrow := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", resource, []string{"inboxes:read"}))
	if tools := toolNames(t, narrow); slices.ContainsFunc(tools, func(n string) bool { return slices.Contains(migrationToolNames, n) }) {
		t.Fatalf("migration tools listed without migrations:manage: %v", tools)
	}
	if _, errText := callTool(t, narrow, "list_migrations", map[string]any{}); !strings.Contains(errText, "migrations:manage") {
		t.Fatalf("a migration tool without the scope: %q", errText)
	}
	agent := h.mcpSession(h.oauthToken(tm.agent, tm.ws, "Claude Code", resource, []string{"conversations:read"}))
	if tools := toolNames(t, agent); slices.ContainsFunc(tools, func(n string) bool { return slices.Contains(migrationToolNames, n) }) {
		t.Fatalf("migration tools listed to an agent: %v", tools)
	}
	if prompts, err := agent.ListPrompts(context.Background(), nil); err != nil || slices.ContainsFunc(prompts.Prompts, func(p *mcp.Prompt) bool { return p.Name == "move_to_yuva" }) {
		t.Fatalf("move_to_yuva listed to an agent: %v", err)
	}
}

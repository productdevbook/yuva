package api_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

func auditLog(t *testing.T, c *client, query string) []map[string]any {
	t.Helper()
	r := c.expect(http.StatusOK, "GET", "/v1/audit-log?limit=100"+query, nil)
	var out []map[string]any
	for _, it := range r.body["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func auditOf(entries []map[string]any, action string) []map[string]any {
	var out []map[string]any
	for _, e := range entries {
		if e["action"] == action {
			out = append(out, e)
		}
	}
	return out
}

func oneAudit(t *testing.T, entries []map[string]any, action string) map[string]any {
	t.Helper()
	found := auditOf(entries, action)
	if len(found) != 1 {
		t.Fatalf("%d %s entries in %v", len(found), action, entries)
	}
	return found[0]
}

func actorOf(e map[string]any) map[string]any   { return e["actor"].(map[string]any) }
func targetOf(e map[string]any) map[string]any  { return e["target"].(map[string]any) }
func detailsOf(e map[string]any) map[string]any { return e["details"].(map[string]any) }

func (h *harness) auditRows(ws string) string {
	h.t.Helper()
	var out string
	err := h.st.Pool.QueryRow(context.Background(),
		"SELECT coalesce(string_agg(action || ' ' || actor_name || ' ' || coalesce(via, '') || ' ' || details::text, E'\\n'), '') FROM audit_log WHERE workspace_id = $1", ws).Scan(&out)
	if err != nil {
		h.t.Fatal(err)
	}
	return out
}

func TestAuditLogActorsScopesAndIsolation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	ownerID := tm.ownerMemberID(t)

	start := auditLog(t, tm.owner, "")
	invited := oneAudit(t, start, "invite.created")
	if a := actorOf(invited); a["kind"] != "member" || a["id"] != ownerID || a["member_id"] != ownerID || a["name"] == "" || a["via"] != nil {
		t.Fatalf("invite actor %v", a)
	}
	if d := detailsOf(invited); d["role"] != "agent" || !strings.HasSuffix(d["email"].(string), "@example.com") || targetOf(invited)["type"] != "invite" {
		t.Fatalf("invite entry %v", invited)
	}
	joined := oneAudit(t, start, "member.joined")
	if actorOf(joined)["id"] != tm.agentID || targetOf(joined)["id"] != tm.agentID || detailsOf(joined)["role"] != "agent" {
		t.Fatalf("join entry %v", joined)
	}

	created := tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{
		"name": "Ops", "bot_name": "Ops bot", "scopes": []string{"webhooks:manage", "inboxes:manage", "audit:read"},
	})
	keyID := created.body["api_key"].(map[string]any)["id"].(string)
	keySecret := created.str("secret")
	bot := h.client()
	bot.bearer = keySecret
	hookURL := "https://hooks.example.com/services/T000/B000/tok3nInPath?token=q5ecretQuery"
	hook := bot.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": hookURL, "events": []string{"conversation.created"}})
	hookID := hook.body["endpoint"].(map[string]any)["id"].(string)
	hookSecret := hook.str("secret")
	rotatedHook := bot.expect(http.StatusOK, "POST", "/v1/webhooks/"+hookID+"/secret", nil).str("secret")
	bot.expect(http.StatusOK, "PATCH", "/v1/webhooks/"+hookID, map[string]any{"enabled": false})
	bot.expect(http.StatusOK, "PATCH", "/v1/webhooks/"+hookID, map[string]any{"description": "nothing to record"})

	app := h.client()
	app.bearer = h.oauthToken(tm.owner, tm.ws, "Audit Client", testOrigin, []string{"inboxes:manage", "audit:read"})
	identity := app.expect(http.StatusOK, "POST", "/v1/inboxes/"+tm.inbox+"/identity-secret", nil).str("identity_secret")

	address := unique("help") + "@example.com"
	smtpPassword := "Sm7p-" + unique("pw")
	channel := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Help", "email": map[string]any{"address": address,
			"smtp": map[string]any{"host": "smtp.example.com", "username": "help", "password": smtpPassword, "tls": "starttls"}},
	}).str("id")
	cs := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read", "inboxes:manage"}))
	if _, text := callTool(t, cs, "create_dkim_key", map[string]any{"channel_id": channel}); text != "" {
		t.Fatalf("create_dkim_key: %s", text)
	}

	all := auditLog(t, tm.owner, "")
	keyEntry := oneAudit(t, all, "api_key.created")
	if d := detailsOf(keyEntry); d["name"] != "Ops" || d["prefix"] == "" || len(d["scopes"].([]any)) != 3 || targetOf(keyEntry)["id"] != keyID {
		t.Fatalf("key entry %v", keyEntry)
	}
	for _, action := range []string{"webhook.created", "webhook.secret_rotated", "webhook.updated"} {
		e := oneAudit(t, all, action)
		if a := actorOf(e); a["kind"] != "api_key" || a["id"] != keyID || a["name"] != "Ops bot" || a["member_id"] != nil || targetOf(e)["id"] != hookID {
			t.Fatalf("%s entry %v", action, e)
		}
	}
	if d := detailsOf(oneAudit(t, all, "webhook.created")); d["host"] != "hooks.example.com" {
		t.Fatalf("webhook details %v", d)
	}
	if d := detailsOf(oneAudit(t, all, "webhook.updated")); fmt.Sprint(d["fields"]) != "[enabled]" {
		t.Fatalf("webhook update details %v", d)
	}
	rotated := oneAudit(t, all, "inbox.identity_secret_rotated")
	if a := actorOf(rotated); a["kind"] != "oauth_grant" || a["member_id"] != ownerID || a["via"] != "Audit Client" || a["name"] == "" || a["id"] == ownerID {
		t.Fatalf("token actor %v", a)
	}
	if e := oneAudit(t, all, "channel.dkim_key_created"); actorOf(e)["kind"] != "oauth_grant" || actorOf(e)["via"] != "Claude Code" || detailsOf(e)["selector"] != "yuva" {
		t.Fatalf("MCP entry %v", e)
	}
	if e := oneAudit(t, all, "channel.credentials_changed"); fmt.Sprint(detailsOf(e)["fields"]) != "[smtp]" || targetOf(e)["id"] != channel {
		t.Fatalf("credentials entry %v", e)
	}
	if grants := auditOf(all, "oauth_grant.created"); len(grants) != 2 || detailsOf(grants[1])["client"] != "Audit Client" {
		t.Fatalf("grant entries %v", grants)
	}
	for _, e := range all {
		if !strings.HasPrefix(e["action"].(string), targetOf(e)["type"].(string)+".") {
			t.Fatalf("action %v with target %v", e["action"], targetOf(e))
		}
	}

	byKey := auditLog(t, bot, "&actor_id="+keyID)
	if len(byKey) != 3 {
		t.Fatalf("%d entries by the key", len(byKey))
	}
	byOwner := auditLog(t, tm.owner, "&actor_id="+ownerID)
	if len(auditOf(byOwner, "inbox.identity_secret_rotated")) != 1 || len(auditOf(byOwner, "invite.created")) != 1 || len(auditOf(byOwner, "webhook.created")) != 0 {
		t.Fatalf("by the owner %v", byOwner)
	}
	if byGrant := auditLog(t, app, "&actor_kind=oauth_grant"); len(byGrant) != 2 {
		t.Fatalf("%d entries by connected apps", len(byGrant))
	}
	if picked := auditLog(t, tm.owner, "&action=webhook.created,api_key.created"); len(picked) != 2 {
		t.Fatalf("comma-separated actions %v", picked)
	}
	if picked := auditLog(t, tm.owner, "&action=webhook.created&action=member.joined"); len(picked) != 2 {
		t.Fatalf("repeated actions %v", picked)
	}
	at := h.clock.Now()
	if got := auditLog(t, tm.owner, "&since="+url.QueryEscape(at.Add(-time.Millisecond).Format(time.RFC3339Nano))); len(got) != len(all) {
		t.Fatalf("since %d of %d", len(got), len(all))
	}
	if got := auditLog(t, tm.owner, "&until="+url.QueryEscape(at.Add(-time.Millisecond).Format(time.RFC3339Nano))); len(got) != 0 {
		t.Fatalf("until %d", len(got))
	}
	h.clock.Advance(time.Hour)
	bot.expect(http.StatusNoContent, "DELETE", "/v1/webhooks/"+hookID, nil)
	if got := auditLog(t, tm.owner, "&since="+url.QueryEscape(at.Add(time.Minute).Format(time.RFC3339Nano))); len(got) != 1 || got[0]["action"] != "webhook.deleted" {
		t.Fatalf("after the hour %v", got)
	}
	if got := auditLog(t, tm.owner, ""); got[0]["action"] != "webhook.deleted" {
		t.Fatalf("not newest first: %v", got[0])
	}

	seen := map[string]bool{}
	cursor := ""
	for pages := 0; ; pages++ {
		r := tm.owner.expect(http.StatusOK, "GET", "/v1/audit-log?limit=2"+cursor, nil)
		for _, it := range r.body["items"].([]any) {
			id := it.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("entry %s twice", id)
			}
			seen[id] = true
		}
		next := r.str("next_cursor")
		if next == "" {
			break
		}
		if pages > 20 {
			t.Fatal("no end to the pages")
		}
		cursor = "&cursor=" + url.QueryEscape(next)
	}
	if len(seen) != len(all)+1 {
		t.Fatalf("paged %d of %d", len(seen), len(all)+1)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/audit-log?action=message.read", nil)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/audit-log?actor_kind=robot", nil)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/audit-log?limit=0", nil)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/audit-log?cursor=nonsense", nil)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/audit-log?since=2026-01-02T00:00:00Z&until=2026-01-01T00:00:00Z", nil)

	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "GET", "/v1/audit-log", nil)
	adminEmail := unique("admin") + "@example.com"
	tm.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": adminEmail, "role": "admin"})
	admin := h.client()
	admin.signIn(adminEmail)
	admin.expectProblem(http.StatusForbidden, "forbidden", "GET", "/v1/audit-log", nil)
	admin.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/api-keys", map[string]any{"name": "peek", "scopes": []string{"audit:read"}})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/api-keys", map[string]any{
		"name": "limited", "scopes": []string{"audit:read"}, "inbox_ids": []string{tm.inbox}})
	plain := h.client()
	plain.bearer = tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "everything"}).str("secret")
	if r := plain.do("GET", "/v1/audit-log", nil); r.status != http.StatusForbidden || r.str("code") != "insufficient_scope" || r.str("scope") != "audit:read" {
		t.Fatalf("a key made without naming scopes: %d %v", r.status, r.body)
	}
	reader := h.client()
	reader.bearer = h.oauthToken(tm.owner, tm.ws, "Reader", testOrigin, []string{"inboxes:read"})
	reader.expectProblem(http.StatusForbidden, "insufficient_scope", "GET", "/v1/audit-log", nil)

	other := newTeam(t, h)
	for _, e := range auditLog(t, other.owner, "") {
		if targetOf(e)["id"] == hookID || actorOf(e)["id"] == keyID || actorOf(e)["id"] == ownerID {
			t.Fatalf("another workspace sees %v", e)
		}
	}
	if got := auditLog(t, other.owner, "&actor_id="+keyID); len(got) != 0 {
		t.Fatalf("another workspace filters to %v", got)
	}
	tm.owner.workspace = other.ws
	tm.owner.expectProblem(http.StatusForbidden, "not_a_member", "GET", "/v1/audit-log", nil)
	tm.owner.workspace = ""
	bot.workspace = other.ws
	bot.expectProblem(http.StatusForbidden, "workspace_mismatch", "GET", "/v1/audit-log", nil)

	rows := h.auditRows(tm.ws)
	raw := string(tm.owner.expect(http.StatusOK, "GET", "/v1/audit-log?limit=100", nil).raw)
	for _, secret := range []string{keySecret, hookSecret, rotatedHook, identity, smtpPassword, "tok3nInPath", "q5ecretQuery", app.bearer} {
		if secret == "" {
			t.Fatal("a secret to look for is empty")
		}
		if strings.Contains(rows, secret) || strings.Contains(raw, secret) {
			t.Fatalf("the audit log holds %q", secret)
		}
	}
}

func TestAuditActionFamilies(t *testing.T) {
	t.Parallel()
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Ingress.ForwardingDomain = testForwardingDomain
		d.Broadcasts.Mode = "approval"
	})
	tm := newTeam(t, h)
	ctx := context.Background()
	ownerID := tm.ownerMemberID(t)

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/members/"+tm.agentID, map[string]any{"role": "admin"})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/members/"+tm.agentID, map[string]any{"role": "admin"})
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	invite := tm.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": unique("later") + "@example.com", "role": "agent"}).str("id")
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/invites/"+invite, nil)

	key := tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "Short lived", "scopes": []string{"inboxes:read"}}).body["api_key"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/api-keys/"+key, nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/api-keys/"+key, nil)
	cliKey, _, err := api.CreateWorkspaceAPIKey(ctx, h.st, uuid.MustParse(tm.ws), api.APIKeySpec{Name: "From the shell", Scopes: []string{"inboxes:read"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.RevokeAPIKeyByID(ctx, h.st, cliKey.ID, time.Now()); err != nil {
		t.Fatal(err)
	}

	address := unique("help") + "@example.com"
	email := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Help", "email": map[string]any{"address": address},
	}).str("id")
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+email, map[string]any{"name": "Help desk"})
	passwords := []string{"first-" + unique("pw"), "second-" + unique("pw")}
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+email, map[string]any{"email": map[string]any{"address": address,
		"broadcast_smtp": map[string]any{"host": "smtp.example.com", "username": "news", "password": passwords[0], "tls": "starttls"}}})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+email, map[string]any{"email": map[string]any{"address": address,
		"smtp": map[string]any{"host": "smtp.example.com", "username": "help", "password": passwords[1], "tls": "starttls"}}})
	tm.owner.expect(http.StatusOK, "POST", "/v1/channels/"+email+"/forwarding-address/rotate", nil)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/channels/"+email+"/dkim-key", map[string]any{"selector": "s1"})
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/channels/"+email+"/dkim-key", nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/channels/"+email+"/dkim-key", nil)
	chat := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "chat", "name": "Site", "chat": map[string]any{"allowed_origins": []string{"https://example.com"}},
	}).str("id")
	tm.owner.expect(http.StatusOK, "POST", "/v1/channels/"+chat+"/public-key", nil)

	tm.owner.expect(http.StatusOK, "POST", "/v1/inboxes/"+tm.inbox+"/identity-secret", nil)
	temp := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Temporary", "slug": "temporary"}).body["inbox"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+temp, nil)

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"retention_days": 30})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"retention_days": 30})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/workspace", map[string]any{"retention_days": 0})

	second := tm.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{"name": "Ben Kaya", "emails": []string{"ben@example.com"}}).str("id")
	tm.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+second+"/consent", nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+tm.contact, nil)
	list := tm.owner.expect(http.StatusCreated, "POST", "/v1/lists", map[string]any{"inbox_id": tm.inbox, "name": "Product news"}).str("id")
	tm.owner.expect(http.StatusOK, "GET", "/v1/lists/"+list+"/consent-export?format=json", nil)
	prop, _ := createProperty(tm.owner, map[string]any{"name": "Site", "domains": []string{"example.com"}, "timezone": "UTC", "inbox_id": tm.inbox})
	export := tm.owner.expect(http.StatusAccepted, "POST", "/v1/analytics/properties/"+prop+"/exports", map[string]any{"kind": "daily", "range": map[string]any{"period": "7d"}}).str("id")

	if _, err := h.st.Pool.Exec(ctx, `UPDATE workspaces SET broadcasts_status = 'approval_required' WHERE id = $1`, tm.ws); err != nil {
		t.Fatal(err)
	}
	tm.owner.expect(http.StatusAccepted, "POST", "/v1/workspace/broadcast-approval", map[string]any{
		"website": "https://acme.example", "sender_description": "Product news", "consent_description": "A form on the site"})
	if err := h.srv.OperatorDenyBroadcasts(ctx, uuid.MustParse(tm.ws), "tell us more"); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.OperatorApproveBroadcasts(ctx, uuid.MustParse(tm.ws), ""); err != nil {
		t.Fatal(err)
	}

	app := h.client()
	app.bearer = h.oauthToken(tm.owner, tm.ws, "Revoked App", testOrigin, []string{"inboxes:read"})
	var grant string
	for _, g := range tm.owner.expect(http.StatusOK, "GET", "/v1/oauth/grants", nil).body["items"].([]any) {
		grant = g.(map[string]any)["id"].(string)
	}
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/oauth/grants/"+grant, nil)

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/members/"+tm.agentID, nil)
	leaverEmail := unique("leaver") + "@example.com"
	tm.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": leaverEmail, "role": "agent"})
	leaver := h.client()
	leaver.signIn(leaverEmail)
	leaverID := leaver.expect(http.StatusOK, "GET", "/v1/me", nil).body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)
	leaver.expect(http.StatusNoContent, "DELETE", "/v1/me", map[string]any{"email": leaverEmail})

	all := auditLog(t, tm.owner, "")
	check := func(action, targetType, targetID string, details map[string]any) map[string]any {
		t.Helper()
		e := oneAudit(t, all, action)
		if targetOf(e)["type"] != targetType || (targetID != "" && targetOf(e)["id"] != targetID) {
			t.Fatalf("%s target %v", action, targetOf(e))
		}
		for k, v := range details {
			if fmt.Sprint(detailsOf(e)[k]) != fmt.Sprint(v) {
				t.Fatalf("%s details %v, want %s=%v", action, detailsOf(e), k, v)
			}
		}
		return e
	}
	check("member.role_changed", "member", tm.agentID, map[string]any{"from": "agent", "to": "admin"})
	check("member.inbox_access_granted", "member", tm.agentID, map[string]any{"inbox_id": tm.inbox, "inbox_name": "Support"})
	check("member.inbox_access_revoked", "member", tm.agentID, map[string]any{"inbox_name": "Support"})
	check("invite.deleted", "invite", invite, nil)
	if revoked := auditOf(all, "api_key.revoked"); len(revoked) != 2 {
		t.Fatalf("%d revocations", len(revoked))
	}
	for _, e := range auditOf(all, "api_key.created") {
		if detailsOf(e)["name"] == "From the shell" && (actorOf(e)["kind"] != "operator" || actorOf(e)["id"] != nil || actorOf(e)["name"] != nil) {
			t.Fatalf("command line key %v", e)
		}
	}
	creds := auditOf(all, "channel.credentials_changed")
	if len(creds) != 2 || fmt.Sprint(detailsOf(creds[0])["fields"]) != "[smtp]" || fmt.Sprint(detailsOf(creds[1])["fields"]) != "[broadcast_smtp]" {
		t.Fatalf("credential entries %v", creds)
	}
	check("channel.forwarding_address_rotated", "channel", email, nil)
	check("channel.dkim_key_created", "channel", email, map[string]any{"selector": "s1"})
	check("channel.dkim_key_deleted", "channel", email, nil)
	check("channel.key_rotated", "channel", chat, nil)
	check("inbox.identity_secret_rotated", "inbox", tm.inbox, nil)
	check("inbox.deleted", "inbox", temp, map[string]any{"name": "Temporary"})
	if e := check("workspace.retention_changed", "workspace", tm.ws, map[string]any{"to": 30}); detailsOf(e)["from"] != nil {
		t.Fatalf("retention from %v", detailsOf(e))
	}
	if d := detailsOf(check("contact.deleted", "contact", tm.contact, nil)); len(d) != 0 {
		t.Fatalf("contact details %v", d)
	}
	check("contact.consent_exported", "contact", second, map[string]any{"format": "csv"})
	check("list.consent_exported", "list", list, map[string]any{"name": "Product news", "format": "json"})
	check("analytics_export.created", "analytics_export", export, map[string]any{"property_id": prop, "kind": "daily", "format": "csv"})
	if e := check("workspace.broadcast_approval_requested", "workspace", tm.ws, nil); actorOf(e)["id"] != ownerID {
		t.Fatalf("approval request actor %v", actorOf(e))
	}
	if e := check("workspace.broadcasts_denied", "workspace", tm.ws, map[string]any{"reason": "tell us more"}); actorOf(e)["kind"] != "operator" {
		t.Fatalf("denial actor %v", actorOf(e))
	}
	if e := check("workspace.broadcasts_approved", "workspace", tm.ws, nil); actorOf(e)["kind"] != "operator" {
		t.Fatalf("approval actor %v", actorOf(e))
	}
	check("oauth_grant.revoked", "oauth_grant", grant, map[string]any{"client": "Revoked App"})
	removed := auditOf(all, "member.removed")
	if len(removed) != 2 || targetOf(removed[1])["id"] != tm.agentID || detailsOf(removed[1])["role"] != "admin" || detailsOf(removed[1])["reason"] != nil {
		t.Fatalf("removals %v", removed)
	}
	if e := removed[0]; targetOf(e)["id"] != leaverID || actorOf(e)["id"] != leaverID || detailsOf(e)["reason"] != "account_deleted" || actorOf(e)["name"] != leaverEmail {
		t.Fatalf("account deletion %v", e)
	}

	rows := h.auditRows(tm.ws)
	for _, secret := range append(passwords, "Ayşe", "ayse@example.com", "Ben Kaya", "ben@example.com") {
		if strings.Contains(rows, secret) {
			t.Fatalf("the audit log holds %q", secret)
		}
	}

	ws := tm.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil).str("name")
	tm.owner.expect(http.StatusAccepted, "DELETE", "/v1/workspace", map[string]any{"name": ws})
	if n := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1 AND action = 'workspace.deleted' AND actor_kind = 'member' AND actor_id = $2", tm.ws, ownerID); n != 1 {
		t.Fatalf("%d workspace deletions by the owner", n)
	}
	other := newTeam(t, h)
	if err := h.srv.OperatorDeleteWorkspace(ctx, uuid.MustParse(other.ws)); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1 AND action = 'workspace.deleted' AND actor_kind = 'operator' AND actor_id IS NULL", other.ws); n != 1 {
		t.Fatalf("%d workspace deletions by the operator", n)
	}
}

// TestAuditWriteSharesTheActionsTransaction blocks audit rows for one workspace: every action
// must then fail and leave nothing behind, and an action that fails writes no row.
func TestAuditWriteSharesTheActionsTransaction(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	key := tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "Kept", "scopes": []string{"inboxes:read"}}).body["api_key"].(map[string]any)["id"].(string)
	hook := tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": "https://hooks.example.com/x", "events": []string{"conversation.created"}}).body["endpoint"].(map[string]any)["id"].(string)

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	constraint := "audit_block_" + hex.EncodeToString(b)
	if _, err := h.st.Pool.Exec(ctx, fmt.Sprintf("ALTER TABLE audit_log ADD CONSTRAINT %s CHECK (workspace_id <> '%s') NOT VALID", constraint, tm.ws)); err != nil {
		t.Fatal(err)
	}
	dropped := false
	drop := func() {
		if !dropped {
			dropped = true
			if _, err := h.st.Pool.Exec(context.Background(), "ALTER TABLE audit_log DROP CONSTRAINT "+constraint); err != nil {
				t.Error(err)
			}
		}
	}
	t.Cleanup(drop)

	for _, c := range []struct{ method, path string }{
		{"DELETE", "/v1/api-keys/" + key},
		{"DELETE", "/v1/inboxes/" + tm.inbox},
		{"POST", "/v1/webhooks/" + hook + "/secret"},
		{"DELETE", "/v1/webhooks/" + hook},
		{"DELETE", "/v1/members/" + tm.agentID},
		{"DELETE", "/v1/contacts/" + tm.contact},
	} {
		if r := tm.owner.do(c.method, c.path, nil); r.status != http.StatusInternalServerError {
			t.Fatalf("%s %s with the audit row refused: %d", c.method, c.path, r.status)
		}
	}
	if r := tm.owner.do("PATCH", "/v1/workspace", map[string]any{"retention_days": 7}); r.status != http.StatusInternalServerError {
		t.Fatalf("retention with the audit row refused: %d", r.status)
	}
	if r := tm.owner.do("POST", "/v1/invites", map[string]any{"email": unique("x") + "@example.com", "role": "agent"}); r.status != http.StatusInternalServerError {
		t.Fatalf("invite with the audit row refused: %d", r.status)
	}
	if n := h.count("SELECT count(*) FROM api_keys WHERE id = $1 AND revoked_at IS NULL", key); n != 1 {
		t.Fatal("the key was revoked without its audit row")
	}
	tm.owner.expect(http.StatusOK, "GET", "/v1/inboxes/"+tm.inbox, nil)
	if n := h.count("SELECT count(*) FROM webhook_endpoints WHERE id = $1 AND previous_secret IS NULL", hook); n != 1 {
		t.Fatal("the webhook changed without its audit row")
	}
	tm.owner.expect(http.StatusOK, "GET", "/v1/members/"+tm.agentID, nil)
	tm.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+tm.contact, nil)
	if w := tm.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil); w.body["retention_days"] != nil {
		t.Fatalf("retention changed without its audit row: %v", w.body)
	}
	if n := h.count("SELECT count(*) FROM invites WHERE workspace_id = $1", tm.ws); n != 0 {
		t.Fatalf("%d invites saved without their audit row", n)
	}
	drop()

	before := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1", tm.ws)
	tm.owner.expectProblem(http.StatusConflict, "last_owner", "DELETE", "/v1/members/"+tm.ownerMemberID(t), nil)
	tm.owner.expectProblem(http.StatusConflict, "last_owner", "PATCH", "/v1/members/"+tm.ownerMemberID(t), map[string]any{"role": "agent"})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/api-keys/"+uuid.NewV7().String(), nil)
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "DELETE", "/v1/inboxes/"+tm.inbox, nil)
	if after := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1", tm.ws); after != before {
		t.Fatalf("failed actions wrote %d rows", after-before)
	}
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/api-keys/"+key, nil)
	if after := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1 AND action = 'api_key.revoked'", tm.ws); after != 1 {
		t.Fatalf("%d revocations after the block", after)
	}
}

// TestAuditRetentionSweep sweeps every workspace in the test database, so it runs before the
// parallel tests.
func TestAuditRetentionSweep(t *testing.T) {
	h := newHarnessWith(t, func(d *api.Deps) { d.AuditDays = 30 })
	tm := newTeam(t, h)
	other := newTeam(t, h)
	start := h.clock.Now()
	old := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1", tm.ws)
	if old == 0 {
		t.Fatal("no entries to sweep")
	}
	h.clock.Set(start.Add(20 * 24 * time.Hour))
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"retention_days": 90})
	h.clock.Set(start.Add(31 * 24 * time.Hour))
	if err := h.srv.CleanAudit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1", tm.ws); n != 1 {
		t.Fatalf("%d entries left, want the recent one", n)
	}
	if n := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1 AND action = 'workspace.retention_changed'", tm.ws); n != 1 {
		t.Fatal("the recent entry was swept")
	}
	if n := h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1", other.ws); n != 0 {
		t.Fatalf("%d old entries left in the other workspace", n)
	}
}

package api_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var setupTools = []string{"create_app_channel", "create_chat_channel", "create_inbox", "get_install_snippet", "list_channels", "update_app_channel", "update_chat_channel"}

func promptText(t *testing.T, cs *mcp.ClientSession, name string, args map[string]string) string {
	t.Helper()
	res, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	var out string
	for _, m := range res.Messages {
		if tc, ok := m.Content.(*mcp.TextContent); ok {
			out += tc.Text
		}
	}
	return out
}

func strs(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestMCPSetupToolsGoThroughTheAPI(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	token := h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read", "inboxes:manage"})
	cs := h.mcpSession(token)
	tools := toolNames(t, cs)
	for _, name := range setupTools {
		if !slices.Contains(tools, name) {
			t.Fatalf("%s not listed to an owner with inboxes:manage: %v", name, tools)
		}
	}

	inbox, errText := callTool(t, cs, "create_inbox", map[string]any{"name": "Örnek Işık", "locale": "tr", "mode": "live"})
	if errText != "" {
		t.Fatal(errText)
	}
	if inbox["slug"] != "ornek-isik" || inbox["default_locale"] != "tr" || inbox["mode"] != "live" || inbox["timezone"] != "UTC" {
		t.Fatalf("create_inbox: %v", inbox)
	}
	if again, errText := callTool(t, cs, "create_inbox", map[string]any{"name": "Örnek Işık", "locale": "en", "mode": "async"}); errText != "" || again["slug"] != "ornek-isik-2" {
		t.Fatalf("second inbox with the same name: %v %s", again, errText)
	}
	if _, errText := callTool(t, cs, "create_inbox", map[string]any{"name": "X", "slug": "ornek-isik", "locale": "en", "mode": "live"}); !strings.Contains(errText, "slug_taken") {
		t.Fatalf("a given slug that is taken: %q", errText)
	}
	if _, errText := callTool(t, cs, "create_inbox", map[string]any{"name": "X", "locale": "en", "mode": "live", "timezone": "Mars/Olympus"}); !strings.Contains(errText, "timezone must be an IANA time zone name") {
		t.Fatalf("bad timezone: %q", errText)
	}
	inboxID := inbox["id"].(string)

	if _, errText := callTool(t, cs, "create_chat_channel", map[string]any{"inbox_id": inboxID, "name": "Site", "origins": []string{"https://example.com/docs"}}); !strings.Contains(errText, "without a path") {
		t.Fatalf("origin with a path: %q", errText)
	}
	ch, errText := callTool(t, cs, "create_chat_channel", map[string]any{"inbox_id": inboxID, "name": "example.com", "origins": []string{"HTTPS://Example.com:443/", "https://example.com"}})
	if errText != "" {
		t.Fatal(errText)
	}
	if fmt.Sprint(ch["allowed_origins"]) != "[https://example.com]" || ch["allow_anonymous"] != true || ch["discussions"] != "off" ||
		!strings.HasPrefix(ch["public_key"].(string), "yuva_pk_") || ch["kind"] != "chat" {
		t.Fatalf("create_chat_channel: %v", ch)
	}
	chID := ch["id"].(string)
	api := tm.owner.expect(http.StatusOK, "GET", "/v1/channels/"+chID, nil).body["chat"].(map[string]any)
	if api["public_key"] != ch["public_key"] || api["allow_anonymous"] != true || api["discussions"] != "off" {
		t.Fatalf("channel in /v1: %v", api)
	}

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+chID, map[string]any{"chat": map[string]any{
		"allowed_origins": []string{"https://example.com"}, "allow_anonymous": true, "discussions": "off",
		"ask_email_offline": false, "launcher": map[string]any{"color": "#112233", "position": "left"},
	}})
	up, errText := callTool(t, cs, "update_chat_channel", map[string]any{
		"channel_id": chID, "add_origins": []string{"http://localhost:3000/"}, "remove_origins": []string{"https://EXAMPLE.com"},
		"discussions": "moderated", "greeting": "Hi there",
	})
	if errText != "" {
		t.Fatal(errText)
	}
	if fmt.Sprint(up["allowed_origins"]) != "[http://localhost:3000]" || up["discussions"] != "moderated" || up["greeting"] != "Hi there" || up["allow_anonymous"] != true {
		t.Fatalf("update_chat_channel: %v", up)
	}
	api = tm.owner.expect(http.StatusOK, "GET", "/v1/channels/"+chID, nil).body["chat"].(map[string]any)
	if api["ask_email_offline"] != false || fmt.Sprint(api["launcher"]) != "map[color:#112233 position:left]" {
		t.Fatalf("update_chat_channel changed what it was not given: %v", api)
	}
	if _, errText := callTool(t, cs, "update_chat_channel", map[string]any{"channel_id": chID, "remove_origins": []string{"http://localhost:3000"}}); !strings.Contains(errText, "1 to 20 origins") {
		t.Fatalf("removing the last origin: %q", errText)
	}
	if _, errText := callTool(t, cs, "update_chat_channel", map[string]any{"channel_id": chID, "add_origins": []string{"ftp://example.com"}}); !strings.Contains(errText, "allowed_origins must be origins") {
		t.Fatalf("bad origin: %q", errText)
	}

	list, errText := callTool(t, cs, "list_channels", map[string]any{"inbox_id": inboxID})
	if errText != "" {
		t.Fatal(errText)
	}
	if items := list["items"].([]any); len(items) != 1 || items[0].(map[string]any)["public_key"] != ch["public_key"] {
		t.Fatalf("list_channels: %v", list)
	}

	key := ch["public_key"].(string)
	for framework, want := range map[string][]string{
		"html":      {testOrigin + "/yuva.js", `channel="` + key + `"`, `server="` + testOrigin + `"`},
		"nuxt":      {`modules: ["useyuva/nuxt"]`, `channel: "` + key + `"`, "<YuvaChat />"},
		"react":     {`import("useyuva/chat")`, key},
		"nextjs":    {`"use client"`, key},
		"docs-html": {testOrigin + "/yuva-docs.js", "<yuva-discussions"},
	} {
		sn, errText := callTool(t, cs, "get_install_snippet", map[string]any{"channel_id": chID, "framework": framework})
		if errText != "" {
			t.Fatal(errText)
		}
		for _, w := range want {
			if !strings.Contains(sn["code"].(string), w) {
				t.Fatalf("%s snippet lacks %q:\n%s", framework, w, sn["code"])
			}
		}
		if (framework == "html" || framework == "docs-html") != (sn["install"] == nil) {
			t.Fatalf("%s install line: %v", framework, sn["install"])
		}
		if sn["placement"] == "" || !strings.HasPrefix(sn["guide_url"].(string), "https://github.com/productdevbook/yuva/blob/main/docs/") {
			t.Fatalf("%s snippet: %v", framework, sn)
		}
	}
	if sn, _ := callTool(t, cs, "get_install_snippet", map[string]any{"channel_id": chID, "framework": "docs-react"}); sn["purpose"] != "docs" {
		t.Fatalf("docs-react purpose: %v", sn["purpose"])
	}

	text := promptText(t, cs, "setup_live_chat", map[string]string{"site_url": "https://www.Example.com/pricing", "product_name": "Acme"})
	if !strings.Contains(text, "https://www.example.com") || !strings.Contains(text, "create_chat_channel") || strings.Contains(text, "reconnect") {
		t.Fatalf("setup_live_chat for an owner:\n%s", text)
	}

	other := newTeam(t, h)
	otherToken := h.oauthToken(other.owner, other.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read", "inboxes:manage"})
	ocs := h.mcpSession(otherToken)
	for name, args := range map[string]map[string]any{
		"list_channels":       {"inbox_id": inboxID},
		"create_chat_channel": {"inbox_id": inboxID, "name": "x", "origins": []string{"https://evil.example"}},
		"update_chat_channel": {"channel_id": chID, "add_origins": []string{"https://evil.example"}},
		"create_app_channel":  {"inbox_id": inboxID, "name": "x"},
		"update_app_channel":  {"channel_id": chID, "name": "x"},
		"get_install_snippet": {"channel_id": chID, "framework": "html"},
	} {
		if _, errText := callTool(t, ocs, name, args); !strings.Contains(errText, "not_found") {
			t.Fatalf("%s from another workspace: %q", name, errText)
		}
	}
}

func TestMCPSetupToolsNeedManageScopeAndRole(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	ch := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "chat", "name": "site", "chat": map[string]any{"allowed_origins": []string{"https://example.com"}},
	}).str("id")
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)

	reader := tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "reader", "scopes": []string{"inboxes:read"}}).str("secret")
	ownerRead := h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read"})
	agent := h.oauthToken(tm.agent, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read", "conversations:read"})
	for who, token := range map[string]string{"inboxes:read key": reader, "owner without inboxes:manage": ownerRead, "agent": agent} {
		cs := h.mcpSession(token)
		tools := toolNames(t, cs)
		if !slices.Contains(tools, "list_channels") || !slices.Contains(tools, "get_install_snippet") {
			t.Fatalf("%s: read setup tools missing: %v", who, tools)
		}
		for _, name := range []string{"create_inbox", "create_chat_channel", "update_chat_channel", "create_app_channel", "update_app_channel"} {
			if slices.Contains(tools, name) {
				t.Fatalf("%s lists %s", who, name)
			}
		}
		if _, errText := callTool(t, cs, "create_inbox", map[string]any{"name": "Sneaky", "locale": "en", "mode": "live"}); !strings.Contains(errText, "insufficient_scope") || !strings.Contains(errText, "reconnect Yuva") || !strings.Contains(errText, "inboxes:manage") {
			t.Fatalf("%s: create_inbox error %q", who, errText)
		}
		if _, errText := callTool(t, cs, "update_chat_channel", map[string]any{"channel_id": ch, "add_origins": []string{"https://evil.example"}}); !strings.Contains(errText, "inboxes:manage") {
			t.Fatalf("%s: update_chat_channel error %q", who, errText)
		}
		if text := promptText(t, cs, "setup_live_chat", map[string]string{"site_url": "example.com"}); !strings.Contains(text, "grant inboxes:manage") || strings.Contains(text, "call create_inbox") {
			t.Fatalf("%s: setup_live_chat:\n%s", who, text)
		}
	}
	if got := tm.owner.expect(http.StatusOK, "GET", "/v1/channels/"+ch, nil).body["chat"].(map[string]any)["allowed_origins"]; fmt.Sprint(got) != "[https://example.com]" {
		t.Fatalf("origins changed: %v", got)
	}
	if n := len(tm.owner.expect(http.StatusOK, "GET", "/v1/inboxes", nil).body["items"].([]any)); n != 1 {
		t.Fatalf("%d inboxes", n)
	}

	manager := tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "setup", "scopes": []string{"inboxes:read", "inboxes:manage"}}).str("secret")
	cs := h.mcpSession(manager)
	if tools := toolNames(t, cs); !slices.Contains(tools, "create_inbox") {
		t.Fatalf("key with inboxes:manage: %v", tools)
	}
	if _, errText := callTool(t, cs, "update_chat_channel", map[string]any{"channel_id": ch, "allow_anonymous": false}); errText != "" {
		t.Fatal(errText)
	}
	sn, errText := callTool(t, cs, "get_install_snippet", map[string]any{"channel_id": ch, "framework": "shadcn", "purpose": "docs"})
	if errText != "" || !strings.Contains(sn["install"].(string), "npx shadcn@latest add") || strs(sn["allowed_origins"])[0] != "https://example.com" {
		t.Fatalf("shadcn docs snippet: %v %s", sn, errText)
	}
}

func TestMCPAppChannelTools(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	manage := h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read", "inboxes:manage"})
	cs := h.mcpSession(manage)

	ch, errText := callTool(t, cs, "create_app_channel", map[string]any{"inbox_id": tm.inbox, "name": "Mobile", "platforms": []string{"ios"}, "allow_anonymous": true})
	if errText != "" {
		t.Fatal(errText)
	}
	if ch["kind"] != "app" || !strings.HasPrefix(ch["public_key"].(string), "yuva_pk_") || fmt.Sprint(ch["platforms"]) != "[ios]" || ch["allow_anonymous"] != true {
		t.Fatalf("create_app_channel: %v", ch)
	}
	id := ch["id"].(string)

	up, errText := callTool(t, cs, "update_app_channel", map[string]any{"channel_id": id, "platforms": []string{"ios", "android"}})
	if errText != "" {
		t.Fatal(errText)
	}
	if fmt.Sprint(up["platforms"]) != "[ios android]" || up["allow_anonymous"] != true || up["name"] != "Mobile" || up["public_key"] != ch["public_key"] {
		t.Fatalf("update_app_channel: %v", up)
	}
	api := tm.owner.expect(http.StatusOK, "GET", "/v1/channels/"+id, nil).body["app"].(map[string]any)
	if fmt.Sprint(api["platforms"]) != "[ios android]" || api["allow_anonymous"] != true {
		t.Fatalf("channel in /v1: %v", api)
	}

	chat := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "chat", "name": "site", "chat": map[string]any{"allowed_origins": []string{"https://example.com"}},
	}).str("id")
	if _, errText := callTool(t, cs, "update_app_channel", map[string]any{"channel_id": chat, "name": "x"}); !strings.Contains(errText, "app channels only") {
		t.Fatalf("update_app_channel on a chat channel: %q", errText)
	}

	mail, errText := callTool(t, cs, "create_inbox", map[string]any{"name": "Mail", "locale": "en", "mode": "async", "purpose": "mail"})
	if errText != "" {
		t.Fatal(errText)
	}
	if _, errText := callTool(t, cs, "create_app_channel", map[string]any{"inbox_id": mail["id"], "name": "App"}); !strings.Contains(errText, "purpose_mismatch") {
		t.Fatalf("app channel in a mail inbox: %q", errText)
	}

	read := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read"}))
	for _, name := range []string{"create_app_channel", "update_app_channel"} {
		if slices.Contains(toolNames(t, read), name) {
			t.Fatalf("inboxes:read lists %s", name)
		}
	}
	if _, errText := callTool(t, read, "update_app_channel", map[string]any{"channel_id": id, "name": "x"}); !strings.Contains(errText, "inboxes:manage") {
		t.Fatalf("missing scope: %q", errText)
	}

	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	agent := h.mcpSession(h.oauthToken(tm.agent, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read"}))
	if slices.Contains(toolNames(t, agent), "create_app_channel") {
		t.Fatal("an agent is offered create_app_channel")
	}
	if _, errText := callTool(t, agent, "create_app_channel", map[string]any{"inbox_id": tm.inbox, "name": "x"}); errText == "" {
		t.Fatal("an agent created an app channel")
	}

	other := newTeam(t, h)
	ocs := h.mcpSession(h.oauthToken(other.owner, other.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read", "inboxes:manage"}))
	if _, errText := callTool(t, ocs, "create_app_channel", map[string]any{"inbox_id": tm.inbox, "name": "x"}); !strings.Contains(errText, "not_found") {
		t.Fatalf("create from another workspace: %q", errText)
	}
	if _, errText := callTool(t, ocs, "update_app_channel", map[string]any{"channel_id": id, "name": "x"}); !strings.Contains(errText, "not_found") {
		t.Fatalf("update from another workspace: %q", errText)
	}
}

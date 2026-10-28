package api_test

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestMCPInitShowsTheWorkspaceAndTheCatalog(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "chat", "name": "Acme web", "chat": map[string]any{"allowed_origins": []string{"https://acme-shop.example.net"}},
	})
	cs := h.mcpSession(h.oauthToken(tm.agent, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"conversations:read"}))
	tools := toolNames(t, cs)
	for _, name := range []string{"get_setup_state", "report_yuva_issue"} {
		if !slices.Contains(tools, name) {
			t.Fatalf("%s not listed to any connection: %v", name, tools)
		}
	}
	if text := promptText(t, cs, "init", map[string]string{"goal": "live chat"}); !strings.Contains(text, "get_setup_state") || !strings.Contains(text, "The person wants: live chat") {
		t.Fatalf("init prompt: %s", text)
	}

	state, errText := callTool(t, cs, "get_setup_state", map[string]any{})
	if errText != "" {
		t.Fatal(errText)
	}
	if len(state["integrations"].([]any)) < 5 || state["server_version"] == nil {
		t.Fatalf("catalog: %v", state)
	}
	if hidden := strings.Join(strs(state["not_visible"]), "|"); !strings.Contains(hidden, "inboxes: grant inboxes:read") {
		t.Fatalf("a connection without inboxes:read is not told what to grant: %q", hidden)
	}

	owner := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read"}))
	state, errText = callTool(t, owner, "get_setup_state", map[string]any{})
	if errText != "" {
		t.Fatal(errText)
	}
	var origins []string
	for _, in := range state["inboxes"].([]any) {
		for _, ch := range in.(map[string]any)["channels"].([]any) {
			if o, ok := ch.(map[string]any)["allowed_origins"]; ok {
				origins = append(origins, strs(o)...)
			}
		}
	}
	if !slices.Contains(origins, "https://acme-shop.example.net") {
		t.Fatalf("channels of the inboxes: %v", state["inboxes"])
	}
}

func TestMCPReportIssueRemovesWhatIdentifiesTheProject(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"name": "Kestrel Support"})
	tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "chat", "name": "Kestrel site", "chat": map[string]any{"allowed_origins": []string{"https://kestrelhq.io"}},
	})
	cs := h.mcpSession(h.oauthToken(tm.agent, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"conversations:read"}))
	report, errText := callTool(t, cs, "report_yuva_issue", map[string]any{
		"title":         "Widget on kestrelhq.io stays blank for Kestrel Support",
		"area":          "widget",
		"what_happened": "On https://kestrelhq.io/pricing the chat for ayse@kestrelhq.io never opens. Key yuva_pk_AbCdEf123456 and token Bearer abcdefghijklmnop0123. Contact 0193a3c4-5d6e-7f80-9a1b-2c3d4e5f6a7b.",
		"error":         "TypeError at /Users/ayse/code/kestrel-web/src/app.tsx:12 from 203.0.113.7, see https://github.com/productdevbook/yuva/issues/1",
		"environment":   "Next.js 16, useyuva 0.0.12, node 24",
	})
	if errText != "" {
		t.Fatal(errText)
	}
	all := report["title"].(string) + "\n" + report["body"].(string) + "\n" + report["gh_command"].(string)
	for _, leak := range []string{"kestrelhq", "Kestrel Support", "Kestrel site", "ayse", "yuva_pk_AbCdEf123456", "abcdefghijklmnop0123", "0193a3c4", "203.0.113.7", "/Users/"} {
		if strings.Contains(strings.ToLower(all), strings.ToLower(leak)) {
			t.Fatalf("the report still carries %q:\n%s", leak, all)
		}
	}
	for _, kept := range []string{"https://github.com/productdevbook/yuva/issues/1", "Next.js 16", "useyuva 0.0.12", "### What happened", "### Error", "- Area: widget"} {
		if !strings.Contains(all, kept) {
			t.Fatalf("the report lost %q:\n%s", kept, all)
		}
	}
	if got := strings.Join(strs(report["redacted"]), "|"); !strings.Contains(got, `1 x name "Kestrel Support"`) || !strings.Contains(got, "<email>") || strings.Contains(got, "ayse") {
		t.Fatalf("redacted list: %q", got)
	}
	if report["redactions"].(float64) < 8 || report["repository"] != "productdevbook/yuva" ||
		!strings.HasPrefix(report["gh_command"].(string), "gh issue create --repo productdevbook/yuva") {
		t.Fatalf("report: %v", report)
	}
	u, err := url.Parse(report["new_issue_url"].(string))
	if err != nil || u.Host != "github.com" || u.Query().Get("title") != report["title"] {
		t.Fatalf("new_issue_url: %v %v", report["new_issue_url"], err)
	}
	if _, errText := callTool(t, cs, "report_yuva_issue", map[string]any{"title": "x"}); !strings.Contains(errText, "what_happened") {
		t.Fatalf("a report without what happened: %s", errText)
	}
}

func TestMCPReportIssueKeepsProductWordsAndShortNames(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"name": "Yuva"})
	tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "API", "slug": "api-desk"})
	tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Web", "slug": "web-desk"})
	tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Kestrel", "slug": "kestrel-desk"})
	cs := h.mcpSession(h.oauthToken(tm.agent, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"conversations:read"}))
	text := "Yuva identifies every signed-in user to Kestrel through the API; Yuva, panel and MCP tools list the inbox for the Web SDK and e-mail."
	report, errText := callTool(t, cs, "report_yuva_issue", map[string]any{"title": "Yuva API", "what_happened": text})
	if errText != "" {
		t.Fatal(errText)
	}
	want := strings.ReplaceAll(text, "Kestrel", "<name>")
	if !strings.Contains(report["body"].(string), want) {
		t.Fatalf("body:\n%s\nwant it to contain\n%s", report["body"], want)
	}
	if report["title"] != "Yuva API" || report["redactions"] != 1.0 || strings.Join(strs(report["redacted"]), "|") != `1 x name "Kestrel"` {
		t.Fatalf("report: %v", report)
	}
}

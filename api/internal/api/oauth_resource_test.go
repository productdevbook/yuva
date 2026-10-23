package api_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func (h *harness) authorize(clientName, resource string) *url.URL {
	h.t.Helper()
	redirect := "http://127.0.0.1:33418/callback"
	reg, _ := json.Marshal(map[string]any{"client_name": clientName, "redirect_uris": []string{redirect}})
	res, err := http.Post(h.url+"/oauth/register", "application/json", strings.NewReader(string(reg)))
	if err != nil {
		h.t.Fatal(err)
	}
	var registered struct {
		ClientID string `json:"client_id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&registered)
	res.Body.Close()
	sum := sha256.Sum256([]byte(strings.Repeat("v", 43)))
	q := url.Values{
		"response_type": {"code"}, "client_id": {registered.ClientID}, "redirect_uri": {redirect}, "state": {"s"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
		"resource": {resource}, "scope": {"conversations:read messages:write"},
	}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ares, err := noFollow.Get(h.url + "/oauth/authorize?" + q.Encode())
	if err != nil {
		h.t.Fatal(err)
	}
	ares.Body.Close()
	loc, err := url.Parse(ares.Header.Get("Location"))
	if err != nil || ares.StatusCode != http.StatusFound {
		h.t.Fatalf("authorize: %d %s", ares.StatusCode, ares.Header.Get("Location"))
	}
	return loc
}

func TestOAuthAPIResourceIsForMemberApps(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	r := h.ingest(et.address, buildMail(mailOpts{
		from: "customer@example.net", to: et.address, subject: "Refund", messageID: newMessageID(), body: "Can I get a refund?",
	}), nil)
	if r.status != http.StatusAccepted {
		t.Fatalf("ingest: %d %v", r.status, r.body)
	}
	conv := r.str("conversation_id")

	refused := h.authorize("Some MCP client", testOrigin)
	if refused.Query().Get("error") != "invalid_target" || refused.Query().Get("code") != "" {
		t.Fatalf("whole-API resource for a third-party client: %s", refused)
	}

	consent := h.authorize("Some MCP client", testOrigin+"/mcp")
	req := et.owner.expect(http.StatusOK, "GET", "/v1/oauth/requests/"+consent.Query().Get("request"), nil)
	var shown struct {
		FirstParty bool `json:"first_party"`
		Workspaces []struct {
			WorkspaceID string `json:"workspace_id"`
			MaySend     bool   `json:"may_send"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal(req.raw, &shown); err != nil {
		t.Fatal(err)
	}
	if shown.FirstParty || len(shown.Workspaces) != 1 || shown.Workspaces[0].MaySend {
		t.Fatalf("consent for a third-party client with bots_may_send off: %s", req.raw)
	}
	et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": true})
	if again := et.owner.expect(http.StatusOK, "GET", "/v1/oauth/requests/"+consent.Query().Get("request"), nil); !strings.Contains(string(again.raw), `"may_send":true`) {
		t.Fatalf("consent with bots_may_send on: %s", again.raw)
	}
	et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": false})

	scopes := []string{"conversations:read", "messages:write", "drafts:send"}
	app := h.oauthToken(et.owner, et.ws, "Member App", testOrigin, scopes)
	if tools := toolNames(t, h.mcpSession(app)); !slices.Contains(tools, "send_reply") {
		t.Fatalf("first-party app without send_reply: %v", tools)
	}

	stale := h.oauthToken(et.owner, et.ws, "Some MCP client", testOrigin, scopes)
	h.firstParty.Clear()
	cs := h.mcpSession(stale)
	if tools := toolNames(t, cs); slices.Contains(tools, "send_reply") || slices.Contains(tools, "send_draft") {
		t.Fatalf("whole-API token of a third-party client lists sending tools: %v", tools)
	}
	if _, errText := callTool(t, cs, "send_reply", map[string]any{"conversation_id": conv, "body": "sent without review"}); errText == "" {
		t.Fatal("send_reply ran without bots_may_send")
	}
	rest := h.client()
	rest.bearer = stale
	rest.expectProblem(http.StatusForbidden, "bot_sending_disabled", "POST", "/v1/conversations/"+conv+"/messages",
		map[string]any{"kind": "message", "body": "sent without review"})
	draft := rest.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages",
		map[string]any{"kind": "message", "body": "for review", "draft": true})
	rest.expectProblem(http.StatusForbidden, "bot_sending_disabled", "POST", "/v1/messages/"+draft.str("id")+"/send", nil)
	if jobs := h.jobs("email_send", et.ws); len(jobs) != 0 {
		t.Fatalf("e-mail queued: %d jobs", len(jobs))
	}
}

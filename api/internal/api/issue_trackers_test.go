package api_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/api"
)

type fakeIssue struct {
	repo, state, reason, title, body string
	number                           int
	linearID, identifier, stateType  string
}

// fakeTrackers answers like api.github.com (REST) and api.linear.app (GraphQL) for one token each.
type fakeTrackers struct {
	srv     *httptest.Server
	mu      sync.Mutex
	github  map[string]*fakeIssue
	linear  map[string]*fakeIssue
	created []*fakeIssue
	calls   int
	down    map[string]bool
	onUser  func()
}

const (
	fakeGitHubToken = "github_pat_fake_0123456789"
	fakeLinearKey   = "lin_api_fake_0123456789"
)

func newFakeTrackers(t *testing.T) *fakeTrackers {
	f := &fakeTrackers{github: map[string]*fakeIssue{}, linear: map[string]*fakeIssue{}, down: map[string]bool{}}
	mux := http.NewServeMux()
	issuePath := regexp.MustCompile(`^/github/repos/([^/]+/[^/]+)/issues(?:/([0-9]+))?$`)
	mux.HandleFunc("/github/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls++
		if r.Header.Get("Authorization") != "Bearer "+fakeGitHubToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}
		if r.URL.Path == "/github/user" {
			if hook := f.onUser; hook != nil {
				f.mu.Unlock()
				hook()
				f.mu.Lock()
			}
			_, _ = w.Write([]byte(`{"login":"octo-team"}`))
			return
		}
		m := issuePath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if f.down[m[1]] && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.Method == http.MethodPost {
			var in struct{ Title, Body string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			iss := &fakeIssue{repo: m[1], number: 100 + len(f.created), state: "open", title: in.Title, body: in.Body}
			f.github[strings.ToLower(m[1])+"#"+strconv.Itoa(iss.number)] = iss
			f.created = append(f.created, iss)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(f.githubJSON(iss))
			return
		}
		iss := f.github[strings.ToLower(m[1])+"#"+m[2]]
		if iss == nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
			return
		}
		_, _ = w.Write(f.githubJSON(iss))
	})
	mux.HandleFunc("POST /linear/graphql", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls++
		if r.Header.Get("Authorization") != fakeLinearKey {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"message":"Authentication required, not authenticated","extensions":{"code":"AUTHENTICATION_ERROR","type":"authentication error"}}]}`))
			return
		}
		var in struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch {
		case strings.Contains(in.Query, "organization"):
			_, _ = w.Write([]byte(`{"data":{"viewer":{"id":"u1"},"organization":{"name":"Acme"}}}`))
		case strings.Contains(in.Query, "teams("):
			if in.Variables["key"] == "ENG" {
				_, _ = w.Write([]byte(`{"data":{"teams":{"nodes":[{"id":"6f1c3c3e-0000-4000-8000-000000000001"}]}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"teams":{"nodes":[]}}}`))
		case strings.Contains(in.Query, "issueCreate"):
			input, _ := in.Variables["input"].(map[string]any)
			n := len(f.created) + 1
			iss := &fakeIssue{linearID: fmt.Sprintf("0b9d1a6e-0000-4000-8000-%012d", n), identifier: "ENG-" + strconv.Itoa(n), stateType: "backlog",
				title: fmt.Sprint(input["title"]), body: fmt.Sprint(input["description"])}
			f.linear[iss.linearID], f.linear[iss.identifier] = iss, iss
			f.created = append(f.created, iss)
			_, _ = w.Write([]byte(`{"data":{"issueCreate":{"success":true,"issue":` + f.linearJSON(iss) + `}}}`))
		default:
			iss := f.linear[fmt.Sprint(in.Variables["id"])]
			if iss == nil {
				_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"Entity not found: Issue","extensions":{"code":"INPUT_ERROR","type":"invalid input"}}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"issue":` + f.linearJSON(iss) + `}}`))
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTrackers) githubJSON(iss *fakeIssue) []byte {
	out := map[string]any{"number": iss.number, "html_url": "https://github.com/" + iss.repo + "/issues/" + strconv.Itoa(iss.number), "title": iss.title, "state": iss.state, "state_reason": nil}
	if iss.reason != "" {
		out["state_reason"] = iss.reason
	}
	b, _ := json.Marshal(out)
	return b
}

func (f *fakeTrackers) linearJSON(iss *fakeIssue) string {
	names := map[string]string{"backlog": "Backlog", "started": "In Progress", "completed": "Done", "canceled": "Canceled"}
	b, _ := json.Marshal(map[string]any{"id": iss.linearID, "identifier": iss.identifier, "title": iss.title, "url": "https://linear.app/acme/issue/" + iss.identifier + "/x",
		"state": map[string]any{"name": names[iss.stateType], "type": iss.stateType}})
	return string(b)
}

func (f *fakeTrackers) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func newIssueHarness(t *testing.T) (*harness, *fakeTrackers) {
	f := newFakeTrackers(t)
	h := newHarnessWith(t, func(d *api.Deps) {
		d.IssueTrackers = api.IssueTrackerSettings{GitHubAPI: f.srv.URL + "/github", LinearAPI: f.srv.URL + "/linear", HTTPClient: f.srv.Client()}
	})
	return h, f
}

func (h *harness) postIssueWebhook(integration string, header map[string]string, body []byte) int {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.url+"/ingress/issues/"+integration, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return res.StatusCode
}

func hexMAC(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func (h *harness) runIssueSyncs(ws string) int {
	h.t.Helper()
	jobs := h.jobs("issue_sync", ws)
	for _, j := range jobs {
		var a api.IssueSyncArgs
		_ = json.Unmarshal(j.args, &a)
		if err := h.srv.SyncIssue(context.Background(), a); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
	}
	return len(jobs)
}

func (h *harness) auditActions(c *client) []string {
	h.t.Helper()
	var out []string
	for _, e := range items(c.expect(http.StatusOK, "GET", "/v1/audit-log", nil)) {
		out = append(out, e["action"].(string))
	}
	return out
}

func TestIssueIntegrations(t *testing.T) {
	t.Parallel()
	h, _ := newIssueHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)

	tm.owner.expectProblem(http.StatusUnprocessableEntity, "tracker_auth_failed", "POST", "/v1/issue-integrations", map[string]any{"provider": "github", "token": "wrong"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/issue-integrations", map[string]any{"provider": "github", "token": fakeGitHubToken})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/issue-integrations", map[string]any{"provider": "linear", "token": fakeLinearKey, "api_url": "https://ghe.example.com/api/v3"})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/issue-integrations", map[string]any{"provider": "github", "token": fakeGitHubToken, "api_url": "https://127.0.0.1/api/v3"})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/issue-integrations", map[string]any{"provider": "github", "token": fakeGitHubToken, "default_target": "not a repo"})

	gh := tm.owner.expect(http.StatusCreated, "POST", "/v1/issue-integrations", map[string]any{"provider": "github", "token": fakeGitHubToken, "default_target": "acme/app"})
	if strings.Contains(string(gh.raw), fakeGitHubToken) {
		t.Fatalf("token answered: %s", gh.raw)
	}
	in := gh.body["integration"].(map[string]any)
	id := in["id"].(string)
	if in["account"] != "octo-team" || in["name"] != "GitHub · octo-team" || in["webhook_secret_set"] != true ||
		!strings.HasSuffix(in["webhook_url"].(string), "/ingress/issues/"+id) {
		t.Fatalf("integration: %s", gh.raw)
	}
	if s, _ := gh.body["webhook_secret"].(string); len(s) != 64 {
		t.Fatalf("made secret: %s", gh.raw)
	}
	lin := tm.owner.expect(http.StatusCreated, "POST", "/v1/issue-integrations", map[string]any{"provider": "linear", "token": fakeLinearKey, "webhook_secret": "lin_wh_0123456789abcdef"})
	if _, ok := lin.body["webhook_secret"]; ok {
		t.Fatalf("a secret the request sent was answered: %s", lin.raw)
	}

	list := tm.agent.expect(http.StatusOK, "GET", "/v1/issue-integrations", nil)
	if n := len(items(list)); n != 2 || strings.Contains(string(list.raw), fakeLinearKey) || strings.Contains(string(list.raw), "lin_wh_") {
		t.Fatalf("list: %s", list.raw)
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/issue-integrations/"+id, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/issue-integrations/"+id, map[string]any{"name": "Mine"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/issue-integrations/"+id, nil)
	if n := len(items(other.owner.expect(http.StatusOK, "GET", "/v1/issue-integrations", nil))); n != 0 {
		t.Fatalf("another workspace sees %d integrations", n)
	}

	tm.owner.expectProblem(http.StatusUnprocessableEntity, "tracker_auth_failed", "PATCH", "/v1/issue-integrations/"+id, map[string]any{"token": "wrong"})
	upd := tm.owner.expect(http.StatusOK, "PATCH", "/v1/issue-integrations/"+id, map[string]any{"token": fakeGitHubToken, "name": "Product", "default_target": nil})
	if upd.body["name"] != "Product" || upd.body["default_target"] != nil {
		t.Fatalf("update: %s", upd.raw)
	}
	rot := tm.owner.expect(http.StatusOK, "POST", "/v1/issue-integrations/"+id+"/webhook-secret", nil)
	if s, _ := rot.body["webhook_secret"].(string); len(s) != 64 || s == gh.body["webhook_secret"] {
		t.Fatalf("rotate: %s", rot.raw)
	}

	limited := tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "reader", "scopes": []string{"integrations:read"}})
	k := h.client()
	k.bearer = limited.str("secret")
	k.expect(http.StatusOK, "GET", "/v1/issue-integrations", nil)
	k.expectProblem(http.StatusForbidden, "insufficient_scope", "DELETE", "/v1/issue-integrations/"+id, nil)

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/issue-integrations/"+id, nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/issue-integrations/"+id, nil)
	acts := strings.Join(h.auditActions(tm.owner), ",")
	for _, want := range []string{"issue_integration.connected", "issue_integration.updated", "issue_integration.secret_rotated", "issue_integration.disconnected"} {
		if !strings.Contains(acts, want) {
			t.Fatalf("audit log lacks %s: %s", want, acts)
		}
	}
	var stored int
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM issue_integrations WHERE position('fake_0123'::bytea in token) > 0").Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("a token is stored in the clear: %d %v", stored, err)
	}
}

func TestIssueStateSetsFeedbackProgress(t *testing.T) {
	t.Parallel()
	h, f := newIssueHarness(t)
	rt := newRoadmapTeam(t, h, false)
	other := newRoadmapTeam(t, h, false)
	gh := rt.owner.expect(http.StatusCreated, "POST", "/v1/issue-integrations", map[string]any{"provider": "github", "token": fakeGitHubToken, "default_target": "acme/app"})
	integration := gh.body["integration"].(map[string]any)["id"].(string)
	secret := gh.body["webhook_secret"].(string)
	conv := rt.feedback(h)

	path := "/v1/conversations/" + conv + "/issues"
	rt.agent.expectProblem(http.StatusBadRequest, "validation_failed", "POST", path, map[string]any{"integration_id": integration})
	rt.agent.expectProblem(http.StatusBadRequest, "validation_failed", "POST", path, map[string]any{"integration_id": integration, "url": "https://example.com/x"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", path, map[string]any{"integration_id": integration, "title": "Dark mode"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", path, nil)
	otherConv := other.feedback(h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+otherConv+"/issues", map[string]any{"integration_id": integration, "title": "Dark mode"})
	plain := rt.conversation(rt.owner)
	rt.owner.expectProblem(http.StatusConflict, "not_feedback", "POST", "/v1/conversations/"+plain+"/issues", map[string]any{"integration_id": integration, "title": "x"})

	link := rt.agent.expect(http.StatusCreated, "POST", path, map[string]any{"integration_id": integration, "title": "Dark mode", "body": "Several customers ask for it."})
	if link.body["key"] != "acme/app#100" || link.body["progress"] != "planned" || link.body["state"] != "open" || link.body["conversation_id"] != conv {
		t.Fatalf("link: %s", link.raw)
	}
	made := f.created[0]
	if !strings.HasPrefix(made.body, "Several customers ask for it.\n\n---\nFrom Yuva: ") || !strings.Contains(made.body, "/conversations/"+conv) || strings.Contains(made.body, "@") {
		t.Fatalf("issue body: %q", made.body)
	}
	if got := rt.progress(conv); got != "planned" {
		t.Fatalf("progress after linking: %v", got)
	}
	again := rt.agent.expect(http.StatusOK, "POST", path, map[string]any{"integration_id": integration, "url": "https://github.com/acme/app/issues/100"})
	if again.body["id"] != link.body["id"] {
		t.Fatalf("linking again made another link: %s", again.raw)
	}
	c := rt.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if iss, _ := c.body["issues"].([]any); len(iss) != 1 || iss[0].(map[string]any)["key"] != "acme/app#100" {
		t.Fatalf("conversation issues: %s", c.raw)
	}

	f.set(func() { made.state, made.reason = "closed", "completed" })
	body := []byte(`{"action":"closed","issue":{"number":100},"repository":{"full_name":"acme/app"}}`)
	if st := h.postIssueWebhook(integration, map[string]string{"X-GitHub-Event": "issues", "X-Hub-Signature-256": "sha256=" + hexMAC("forged-secret", body)}, body); st != http.StatusUnauthorized {
		t.Fatalf("forged webhook: %d", st)
	}
	if st := h.postIssueWebhook(integration, map[string]string{"X-GitHub-Event": "issues"}, body); st != http.StatusUnauthorized {
		t.Fatalf("unsigned webhook: %d", st)
	}
	if n := len(h.jobs("issue_sync", rt.ws)); n != 0 {
		t.Fatalf("a refused webhook queued %d syncs", n)
	}
	if st := h.postIssueWebhook("0198c1d2-0000-7000-8000-000000000000", map[string]string{"X-GitHub-Event": "issues"}, body); st != http.StatusNotFound {
		t.Fatalf("unknown integration: %d", st)
	}
	if st := h.postIssueWebhook(integration, map[string]string{"X-GitHub-Event": "issues", "X-Hub-Signature-256": "sha256=" + hexMAC(secret, body)}, body); st != http.StatusAccepted {
		t.Fatalf("webhook: %d", st)
	}
	if n := h.runIssueSyncs(rt.ws); n != 1 {
		t.Fatalf("syncs %d", n)
	}
	if got := rt.progress(conv); got != "done" {
		t.Fatalf("progress after closing: %v", got)
	}
	evs := messages(rt.owner, conv)
	var viaIssue bool
	for _, m := range evs {
		if ev, ok := m["event"].(map[string]any); ok && ev["type"] == "feedback_progress" && ev["issue_link_id"] == link.body["id"] {
			viaIssue = true
		}
	}
	if !viaIssue {
		t.Fatal("no feedback_progress event names the issue link")
	}

	rt.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"feedback_progress": "in_progress"})
	edited := []byte(`{"action":"edited","issue":{"number":100},"repository":{"full_name":"acme/app"}}`)
	f.set(func() { made.title = "Dark mode for the app" })
	h.clock.Advance(time.Second)
	h.postIssueWebhook(integration, map[string]string{"X-GitHub-Event": "issues", "X-Hub-Signature-256": "sha256=" + hexMAC(secret, edited)}, edited)
	h.runIssueSyncs(rt.ws)
	if got := rt.progress(conv); got != "in_progress" {
		t.Fatalf("a progress set by hand did not hold: %v", got)
	}
	f.set(func() { made.state, made.reason = "closed", "not_planned" })
	h.clock.Advance(time.Second)
	h.postIssueWebhook(integration, map[string]string{"X-GitHub-Event": "issues", "X-Hub-Signature-256": "sha256=" + hexMAC(secret, body)}, body)
	h.runIssueSyncs(rt.ws)
	if got := rt.progress(conv); got != "wont_do" {
		t.Fatalf("progress after closing as not planned: %v", got)
	}

	f.set(func() { made.state, made.reason = "open", "reopened" })
	h.clock.Advance(2 * time.Hour)
	if n, err := h.srv.SweepIssues(context.Background()); err != nil || n < 1 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	h.runIssueSyncs(rt.ws)
	if got := rt.progress(conv); got != "planned" {
		t.Fatalf("progress after the sweep: %v", got)
	}

	var types []string
	for _, e := range rt.owner.expect(http.StatusOK, "GET", "/v1/events?after=0", nil).body["events"].([]any) {
		types = append(types, e.(map[string]any)["type"].(string))
	}
	if all := strings.Join(types, ","); !strings.Contains(all, "issue_link.created") || !strings.Contains(all, "issue_link.updated") {
		t.Fatalf("events: %s", all)
	}

	linkID := link.body["id"].(string)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", path+"/"+linkID, nil)
	rt.agent.expect(http.StatusNoContent, "DELETE", path+"/"+linkID, nil)
	rt.agent.expectProblem(http.StatusNotFound, "not_found", "DELETE", path+"/"+linkID, nil)
	if n := len(items(rt.owner.expect(http.StatusOK, "GET", path, nil))); n != 0 {
		t.Fatalf("links after unlinking: %d", n)
	}
	if got := rt.progress(conv); got != "planned" {
		t.Fatalf("unlinking changed the progress: %v", got)
	}
}

func TestLinearIssueOnIdeaShowsStateOnly(t *testing.T) {
	t.Parallel()
	h, f := newIssueHarness(t)
	rt := newRoadmapTeam(t, h, false)
	const secret = "lin_wh_0123456789abcdef"
	lin := rt.owner.expect(http.StatusCreated, "POST", "/v1/issue-integrations", map[string]any{"provider": "linear", "token": fakeLinearKey, "webhook_secret": secret})
	integration := lin.body["integration"].(map[string]any)["id"].(string)
	f.set(func() {
		iss := &fakeIssue{linearID: "0b9d1a6e-0000-4000-8000-0000000000aa", identifier: "ENG-7", stateType: "started", title: "Export to CSV"}
		f.linear[iss.linearID], f.linear[iss.identifier] = iss, iss
	})
	idea := rt.idea(rt.owner, "Export to CSV")
	conv := rt.feedback(h)
	rt.owner.expect(http.StatusNoContent, "PUT", "/v1/discussions/"+idea+"/conversations/"+conv, nil)

	path := "/v1/discussions/" + idea + "/issues"
	rt.agent.expectProblem(http.StatusUnprocessableEntity, "issue_not_found", "POST", path, map[string]any{"integration_id": integration, "url": "ENG-404"})
	link := rt.agent.expect(http.StatusCreated, "POST", path, map[string]any{"integration_id": integration, "url": "https://linear.app/acme/issue/ENG-7/export-to-csv"})
	if link.body["key"] != "ENG-7" || link.body["progress"] != "in_progress" || link.body["state"] != "In Progress" || link.body["idea_id"] != idea {
		t.Fatalf("link: %s", link.raw)
	}
	stage := func() any {
		t.Helper()
		return rt.owner.expect(http.StatusOK, "GET", "/v1/discussions/"+idea, nil).body["discussion"].(map[string]any)["stage"]
	}
	d := rt.owner.expect(http.StatusOK, "GET", "/v1/discussions/"+idea, nil).body["discussion"].(map[string]any)
	if d["stage"] != "ideas" || len(d["issues"].([]any)) != 1 {
		t.Fatalf("idea after linking: %v", d)
	}
	if n := len(rt.changes(idea)); n != 0 {
		t.Fatalf("linking an issue moved the idea %d times", n)
	}

	created := rt.agent.expect(http.StatusCreated, "POST", path, map[string]any{"integration_id": integration, "title": "CSV export follow-up", "target": "ENG"})
	if created.body["key"] != "ENG-1" || !strings.Contains(f.created[0].body, "/board/ideas/"+idea) {
		t.Fatalf("created: %s %q", created.raw, f.created[0].body)
	}
	rt.agent.expectProblem(http.StatusUnprocessableEntity, "issue_not_found", "POST", path, map[string]any{"integration_id": integration, "title": "x", "target": "OPS"})

	f.set(func() { f.linear["ENG-7"].stateType = "completed" })
	stale := []byte(fmt.Sprintf(`{"action":"update","type":"Issue","data":{"id":"0b9d1a6e-0000-4000-8000-0000000000aa"},"webhookTimestamp":%d}`, time.Now().Add(-5*time.Minute).UnixMilli()))
	if st := h.postIssueWebhook(integration, map[string]string{"Linear-Signature": hexMAC(secret, stale)}, stale); st != http.StatusUnauthorized {
		t.Fatalf("stale webhook: %d", st)
	}
	fresh := []byte(fmt.Sprintf(`{"action":"update","type":"Issue","data":{"id":"0b9d1a6e-0000-4000-8000-0000000000aa"},"webhookTimestamp":%d}`, time.Now().UnixMilli()))
	if st := h.postIssueWebhook(integration, map[string]string{"Linear-Signature": hexMAC("lin_wh_forged_forged", fresh)}, fresh); st != http.StatusUnauthorized {
		t.Fatalf("forged webhook: %d", st)
	}
	if st := h.postIssueWebhook(integration, map[string]string{"Linear-Signature": hexMAC(secret, fresh)}, fresh); st != http.StatusAccepted {
		t.Fatalf("webhook: %d", st)
	}
	h.clock.Advance(time.Second)
	h.runIssueSyncs(rt.ws)
	links := items(rt.owner.expect(http.StatusOK, "GET", path, nil))
	if links[0]["state"] != "Done" || links[0]["progress"] != "done" {
		t.Fatalf("idea's issue after completing: %v", links[0])
	}
	if st := stage(); st != "ideas" {
		t.Fatalf("an issue moved the idea to %v", st)
	}
	rt.runProgress(t)
	if got := rt.progress(conv); got != nil {
		t.Fatalf("the idea's linked feedback got progress %v from the issue", got)
	}
	rt.scopedKey("conversations:read").expectProblem(http.StatusForbidden, "insufficient_scope", "POST", path, map[string]any{"integration_id": integration, "url": "ENG-7"})
}

func TestIssueUpdateKeepsARacingSecretRotation(t *testing.T) {
	t.Parallel()
	h, f := newIssueHarness(t)
	tm := newTeam(t, h)
	gh := tm.owner.expect(http.StatusCreated, "POST", "/v1/issue-integrations", map[string]any{"provider": "github", "token": fakeGitHubToken})
	id := gh.body["integration"].(map[string]any)["id"].(string)
	var rotated string
	f.set(func() {
		f.onUser = func() {
			f.onUser = nil
			rotated = tm.owner.expect(http.StatusOK, "POST", "/v1/issue-integrations/"+id+"/webhook-secret", nil).str("webhook_secret")
		}
	})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/issue-integrations/"+id, map[string]any{"token": fakeGitHubToken, "name": "Renamed"})
	if rotated == "" {
		t.Fatal("the rotation did not run during the token check")
	}
	body := []byte(`{"zen":"Keep it logically awesome."}`)
	if st := h.postIssueWebhook(id, map[string]string{"X-GitHub-Event": "ping", "X-Hub-Signature-256": "sha256=" + hexMAC(rotated, body)}, body); st != http.StatusAccepted {
		t.Fatalf("the update undid the rotation: webhook signed with the rotated secret got %d", st)
	}
	if got := tm.owner.expect(http.StatusOK, "GET", "/v1/issue-integrations/"+id, nil).str("name"); got != "Renamed" {
		t.Fatalf("name %q", got)
	}
}

func TestIssueSweepGivesEveryIntegrationATurn(t *testing.T) {
	h, f := newIssueHarness(t)
	defer api.SetIssueSweepLimits(500, 1)()
	down := newRoadmapTeam(t, h, false)
	up := newRoadmapTeam(t, h, false)
	connect := func(rt roadmapTeam) string {
		return rt.owner.expect(http.StatusCreated, "POST", "/v1/issue-integrations", map[string]any{"provider": "github", "token": fakeGitHubToken, "default_target": "acme/" + rt.ws[len(rt.ws)-8:]}).
			body["integration"].(map[string]any)["id"].(string)
	}
	downInt, upInt := connect(down), connect(up)
	for range 3 {
		down.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+down.feedback(h)+"/issues", map[string]any{"integration_id": downInt, "title": "x"})
	}
	conv := up.feedback(h)
	up.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/issues", map[string]any{"integration_id": upInt, "title": "y"})
	made := f.created[len(f.created)-1]
	f.set(func() {
		f.down["acme/"+down.ws[len(down.ws)-8:]] = true
		made.state, made.reason = "closed", "completed"
	})

	h.clock.Advance(2 * time.Hour)
	if _, err := h.srv.SweepIssues(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(h.jobs("issue_sync", down.ws)); n != 1 {
		t.Fatalf("the failing integration took %d of the sweep", n)
	}
	if n := len(h.jobs("issue_sync", up.ws)); n != 1 {
		t.Fatalf("the healthy integration got %d", n)
	}
	h.runIssueSyncs(down.ws)
	h.runIssueSyncs(up.ws)
	if got := up.progress(conv); got != "done" {
		t.Fatalf("the healthy workspace's issue was not read: %v %s", got, up.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv+"/issues", nil).raw)
	}
	backoff := func() (int, time.Duration) {
		t.Helper()
		var n int
		var next time.Time
		if err := h.st.Pool.QueryRow(context.Background(), "SELECT sync_failures, next_sync_at FROM tracked_issues WHERE workspace_id = $1 AND sync_failures > 0", down.ws).Scan(&n, &next); err != nil {
			t.Fatal(err)
		}
		return n, next.Sub(h.clock.Now())
	}
	if n, wait := backoff(); n != 1 || wait < 14*time.Minute || wait > 16*time.Minute {
		t.Fatalf("after one failure: %d failures, next read in %v", n, wait)
	}
}

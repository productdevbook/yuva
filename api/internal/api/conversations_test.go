package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"testing"
	"time"
	"uuid"
)

type file struct {
	name, contentType string
	data              []byte
}

func (c *client) upload(path string, fields map[string]string, files []file) response {
	c.h.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	for _, f := range files {
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files"; filename=%q`, f.name))
		hdr.Set("Content-Type", f.contentType)
		pw, _ := w.CreatePart(hdr)
		_, _ = pw.Write(f.data)
	}
	_ = w.Close()
	req, _ := http.NewRequest("POST", c.h.url+path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Forwarded-For", c.ip)
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	if c.workspace != "" {
		req.Header.Set("Yuva-Workspace", c.workspace)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.h.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := response{status: res.StatusCode, raw: raw, header: res.Header}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

type team struct {
	ws      string
	owner   *client
	agent   *client
	agentID string
	inbox   string
	contact string
}

func newTeam(t *testing.T, h *harness) team {
	t.Helper()
	ownerEmail := unique("owner") + "@example.com"
	ws := h.bootstrap(ownerEmail, unique("ws"))
	owner := h.client()
	owner.signIn(ownerEmail)
	agentEmail := unique("agent") + "@example.com"
	owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": agentEmail, "role": "agent"})
	agent := h.client()
	agent.signIn(agentEmail)
	me := agent.expect(http.StatusOK, "GET", "/v1/me", nil)
	agentID := me.body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)
	inbox := owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Support", "slug": "support"})
	contact := owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{
		"name": "Ayşe Yılmaz", "emails": []string{"ayse@example.com"},
	})
	return team{
		ws: ws.WorkspaceID.String(), owner: owner, agent: agent, agentID: agentID,
		inbox: inbox.body["inbox"].(map[string]any)["id"].(string), contact: contact.str("id"),
	}
}

func (tm team) conversation(c *client) string {
	r := c.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact, "subject": "Help"})
	return r.str("id")
}

func messages(c *client, conv string) []map[string]any {
	r := c.expect(http.StatusOK, "GET", "/v1/conversations/"+conv+"/messages?limit=100", nil)
	var out []map[string]any
	for _, m := range r.body["items"].([]any) {
		out = append(out, m.(map[string]any))
	}
	return out
}

func TestInboxAccessControl(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	created := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Other", "slug": "other"})
	plain := created.str("identity_secret")
	other := uuid.MustParse(created.body["inbox"].(map[string]any)["id"].(string))
	var sealed []byte
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT identity_secret FROM inboxes WHERE id = $1", other).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	ws := uuid.MustParse(tm.ws)
	opened, err := h.secrets.Open(sealed, append(ws[:], other[:]...))
	if err != nil || string(opened) != plain || bytes.Contains(sealed, []byte(plain)) {
		t.Fatalf("identity secret not stored encrypted: %v", err)
	}
	if bytes.Contains(tm.owner.expect(http.StatusOK, "GET", "/v1/inboxes/"+other.String(), nil).raw, []byte(plain)) {
		t.Fatal("inbox body shows the identity secret")
	}
	conv := tm.conversation(tm.owner)
	msg := tm.owner.upload("/v1/conversations/"+conv+"/messages", map[string]string{"kind": "message", "body": "see file"},
		[]file{{"a.txt", "text/plain", []byte("hello")}})
	if msg.status != http.StatusCreated {
		t.Fatalf("upload: %d %s", msg.status, msg.raw)
	}
	att := msg.body["attachments"].([]any)[0].(map[string]any)["id"].(string)

	a := tm.agent
	if items := a.expect(http.StatusOK, "GET", "/v1/inboxes", nil).body["items"].([]any); len(items) != 0 {
		t.Fatalf("agent sees %d inboxes without access", len(items))
	}
	a.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/inboxes/"+tm.inbox, nil)
	a.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv, nil)
	a.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv+"/messages", nil)
	a.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "x"})
	a.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	a.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/attachments/"+att, nil)
	a.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations?inbox_id="+tm.inbox, nil)
	a.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact})
	if items := a.expect(http.StatusOK, "GET", "/v1/conversations", nil).body["items"].([]any); len(items) != 0 {
		t.Fatalf("agent lists %d conversations without access", len(items))
	}
	tm.owner.expect(http.StatusBadRequest, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": tm.agentID})
	a.expectProblem(http.StatusForbidden, "forbidden", "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)

	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	a.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	a.expect(http.StatusOK, "GET", "/v1/attachments/"+att, nil)
	if items := a.expect(http.StatusOK, "GET", "/v1/conversations", nil).body["items"].([]any); len(items) != 1 {
		t.Fatalf("agent lists %d conversations with access", len(items))
	}
	a.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "direction": "in", "body": "x"})

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	a.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv, nil)
}

func TestConversationCrossWorkspace(t *testing.T) {
	h := newHarness(t)
	a := newTeam(t, h)
	b := newTeam(t, h)
	convB := b.conversation(b.owner)
	labelB := b.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "vip"}).str("id")

	key := a.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "backend"}).str("secret")
	keyA := h.client()
	keyA.bearer = key
	for _, c := range []*client{a.owner, keyA} {
		c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/inboxes/"+b.inbox, nil)
		c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/contacts/"+b.contact, nil)
		c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+convB, nil)
		c.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/conversations/"+convB, map[string]any{"status": "closed"})
		c.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+convB+"/messages", map[string]any{"kind": "note", "body": "x"})
		c.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations", map[string]any{"inbox_id": b.inbox, "contact_id": a.contact})
		c.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations", map[string]any{"inbox_id": a.inbox, "contact_id": b.contact})
		c.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/contacts/"+b.contact, nil)
	}
	convA := a.conversation(a.owner)
	a.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/conversations/"+convA, map[string]any{"labels": []string{labelB}})
	a.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/conversations/"+convA, map[string]any{"assignee_id": b.agentID})
	a.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", "/v1/inboxes/"+b.inbox+"/members/"+a.agentID, nil)
	keyA.workspace = b.ws
	keyA.expectProblem(http.StatusForbidden, "workspace_mismatch", "GET", "/v1/conversations", nil)
	b.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+convB, nil)
}

func TestMessageClientIDIsIdempotent(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	conv := tm.conversation(tm.owner)
	body := map[string]any{"kind": "message", "body": "Merhaba", "client_id": "c-1"}
	first := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", body)
	again := tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/messages", body)
	if first.str("id") != again.str("id") {
		t.Fatalf("client_id repeated gave a new message: %s vs %s", first.raw, again.raw)
	}
	up := tm.owner.upload("/v1/conversations/"+conv+"/messages", map[string]string{"kind": "message", "body": "file", "client_id": "c-2"},
		[]file{{"a.txt", "text/plain", []byte("one")}})
	up2 := tm.owner.upload("/v1/conversations/"+conv+"/messages", map[string]string{"kind": "message", "body": "file", "client_id": "c-2"},
		[]file{{"a.txt", "text/plain", []byte("one")}})
	if up.status != http.StatusCreated || up2.status != http.StatusOK || up.str("id") != up2.str("id") {
		t.Fatalf("multipart client_id: %d %s / %d %s", up.status, up.raw, up2.status, up2.raw)
	}
	other := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+other+"/messages", body)
	if n := len(messages(tm.owner, conv)); n != 2 {
		t.Fatalf("%d messages, want 2", n)
	}
	var keys int
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM attachments a JOIN messages m ON m.id = a.message_id WHERE m.conversation_id = $1", uuid.MustParse(conv)).Scan(&keys); err != nil || keys != 1 {
		t.Fatalf("attachments stored: %d, %v", keys, err)
	}
}

func TestEventsOnAssignStatusAndLabels(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	conv := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	label := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "billing", "color": "#FF0000"}).str("id")

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": tm.agentID})
	tm.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed", "labels": []string{label}})
	tm.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"priority": "high"})
	r := tm.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": nil, "status": "open"})
	if _, ok := r.body["assignee_id"]; ok || r.str("status") != "open" {
		t.Fatalf("after unassign: %s", r.raw)
	}

	var got []string
	for _, m := range messages(tm.owner, conv) {
		if m["kind"] != "event" {
			continue
		}
		ev := m["event"].(map[string]any)
		author := m["author"].(map[string]any)
		got = append(got, fmt.Sprintf("%s:%v:%v", ev["type"], ev["status"], author["member_id"] != nil))
	}
	want := []string{"assigned:<nil>:true", "status_changed:closed:true", "labels_changed:<nil>:true", "unassigned:<nil>:true", "status_changed:open:true"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("events\n got %v\nwant %v", got, want)
	}
}

func TestConversationCursorPagination(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	want := map[string]bool{}
	for i := range 7 {
		want[tm.conversation(tm.owner)] = true
		if i%3 == 0 {
			h.clock.Advance(time.Second)
		}
	}
	seen := map[string]bool{}
	var order []string
	cursor := ""
	for page := 0; ; page++ {
		path := "/v1/conversations?limit=3"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		r := tm.owner.expect(http.StatusOK, "GET", path, nil)
		items := r.body["items"].([]any)
		if page == 1 {
			tm.conversation(tm.owner)
		}
		for _, it := range items {
			id := it.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("conversation %s returned twice", id)
			}
			seen[id] = true
			order = append(order, it.(map[string]any)["last_activity_at"].(string)+" "+id)
		}
		next, _ := r.body["next_cursor"].(string)
		if next == "" {
			break
		}
		cursor = next
	}
	for id := range want {
		if !seen[id] {
			t.Fatalf("conversation %s missing from pages", id)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("saw %d conversations, want %d (a conversation created mid-way must not shift pages)", len(seen), len(want))
	}
	for i := 1; i < len(order); i++ {
		if order[i-1][:19] < order[i][:19] {
			t.Fatalf("not ordered by last activity: %v", order)
		}
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?cursor=bogus", nil)
}

func TestAttachmentLimits(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	conv := tm.conversation(tm.owner)
	path := "/v1/conversations/" + conv + "/messages"
	fields := map[string]string{"kind": "message", "body": "files"}

	big := tm.owner.upload(path, fields, []file{{"big.txt", "text/plain", bytes.Repeat([]byte("x"), testAttachmentMaxBytes+1)}})
	if big.status != http.StatusRequestEntityTooLarge || big.str("code") != "attachment_too_large" {
		t.Fatalf("too large: %d %s", big.status, big.raw)
	}
	bad := tm.owner.upload(path, fields, []file{{"page.html", "text/html", []byte("<script>alert(1)</script>")}})
	if bad.status != http.StatusUnsupportedMediaType || bad.str("code") != "attachment_type_not_allowed" {
		t.Fatalf("type: %d %s", bad.status, bad.raw)
	}
	var many []file
	for i := range 11 {
		many = append(many, file{fmt.Sprintf("%d.txt", i), "text/plain", []byte("x")})
	}
	if r := tm.owner.upload(path, fields, many); r.status != http.StatusBadRequest {
		t.Fatalf("11 files: %d %s", r.status, r.raw)
	}
	if n := len(messages(tm.owner, conv)); n != 0 {
		t.Fatalf("rejected uploads wrote %d messages", n)
	}

	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 100)...)
	ok := tm.owner.upload(path, fields, []file{
		{"exact.txt", "text/plain", bytes.Repeat([]byte("y"), testAttachmentMaxBytes)},
		{"../../pic.png", "", png},
	})
	if ok.status != http.StatusCreated {
		t.Fatalf("upload: %d %s", ok.status, ok.raw)
	}
	atts := ok.body["attachments"].([]any)
	pic := atts[1].(map[string]any)
	if pic["filename"] != "pic.png" || pic["content_type"] != "image/png" {
		t.Fatalf("sniffed attachment: %v", pic)
	}
	dl := tm.owner.expect(http.StatusOK, "GET", "/v1/attachments/"+pic["id"].(string), nil)
	if !bytes.Equal(dl.raw, png) || dl.header.Get("Content-Type") != "image/png" ||
		dl.header.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(dl.header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("download headers %v, %d bytes", dl.header, len(dl.raw))
	}
}

func TestUsageCounters(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	conv := tm.conversation(tm.owner)
	tm.conversation(tm.owner)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "hi", "client_id": "u"})
	tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "hi", "client_id": "u"})
	tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "internal"})
	tm.owner.upload("/v1/conversations/"+conv+"/messages", map[string]string{"kind": "message", "body": "f"},
		[]file{{"a.txt", "text/plain", []byte("12345")}, {"b.txt", "text/plain", []byte("678")}})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})

	r := tm.owner.expect(http.StatusOK, "GET", "/v1/usage", nil)
	items := r.body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("usage: %s", r.raw)
	}
	u := items[0].(map[string]any)
	if u["month"] != h.clock.Now().UTC().Format("2006-01") || u["conversations"] != 2.0 || u["messages"] != 3.0 || u["attachment_bytes"] != 8.0 {
		t.Fatalf("usage: %v", u)
	}
	var stored int64
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT messages FROM usage_counters WHERE workspace_id = $1", uuid.MustParse(tm.ws)).Scan(&stored); err != nil || stored != 3 {
		t.Fatalf("stored counter %d, %v", stored, err)
	}
}

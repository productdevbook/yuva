package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"
)

func listItems(c *client, query string) []map[string]any {
	c.h.t.Helper()
	r := c.expect(http.StatusOK, "GET", "/v1/conversations"+query, nil)
	var out []map[string]any
	for _, it := range r.body["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func itemByID(items []map[string]any, id string) map[string]any {
	for _, it := range items {
		if it["id"] == id {
			return it
		}
	}
	return nil
}

func (tm team) apiKey(h *harness) *client {
	k := h.client()
	k.bearer = tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "backend"}).str("secret")
	return k
}

func post(c *client, conv string, body map[string]any) string {
	c.h.t.Helper()
	c.h.clock.Advance(time.Second)
	return c.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", body).str("id")
}

func TestConversationListItems(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	key := tm.apiKey(h)
	conv := tm.conversation(tm.owner)
	empty := tm.conversation(tm.owner)
	long := strings.Repeat("çok uzun ", 30)
	post(key, conv, map[string]any{"kind": "message", "direction": "in", "body": "  first\n\nline  "})
	post(tm.owner, conv, map[string]any{"kind": "message", "body": long})
	post(tm.owner, conv, map[string]any{"kind": "note", "body": "internal only"})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "pending"})

	nameless := tm.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{}).str("id")
	bare := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": nameless}).str("id")

	items := listItems(tm.owner, "")
	it := itemByID(items, conv)
	contact := it["contact"].(map[string]any)
	if contact["id"] != tm.contact || contact["name"] != "Ayşe Yılmaz" || contact["email"] != "ayse@example.com" {
		t.Fatalf("contact %v", contact)
	}
	last := it["last_message"].(map[string]any)
	text := last["text"].(string)
	want := []rune(strings.Join(strings.Fields(long), " "))
	if last["kind"] != "message" || last["author_type"] != "member" || !strings.HasSuffix(text, "…") ||
		len([]rune(text)) > 141 || !strings.HasPrefix(text, string(want[:100])) {
		t.Fatalf("preview %v", last)
	}
	if _, ok := itemByID(items, empty)["last_message"]; ok {
		t.Fatalf("conversation without messages has a preview: %v", itemByID(items, empty))
	}
	if c := itemByID(items, bare)["contact"].(map[string]any); c["id"] != nameless || c["name"] != "" {
		t.Fatalf("contact without e-mail %v", c)
	} else if _, ok := c["email"]; ok {
		t.Fatalf("contact without e-mail has one: %v", c)
	}
	for _, f := range []string{"inbox_id", "contact_id", "status", "labels", "last_activity_at"} {
		if _, ok := it[f]; !ok {
			t.Fatalf("list item lost %s: %v", f, it)
		}
	}

	byContact := listItems(tm.owner, "?contact_id="+nameless)
	if len(byContact) != 1 || byContact[0]["id"] != bare {
		t.Fatalf("contact_id filter: %v", byContact)
	}
	if n := len(listItems(tm.owner, "?contact_id="+tm.contact)); n != 2 {
		t.Fatalf("contact_id filter for the first contact: %d", n)
	}
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations?contact_id="+other.contact, nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations?contact_id="+uuid.New().String(), nil)
}

func TestUnreadAndReadCursor(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	key := tm.apiKey(h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	conv := tm.conversation(tm.owner)
	quiet := tm.conversation(tm.owner)
	unread := func(c *client, id string) bool {
		t.Helper()
		return itemByID(listItems(c, ""), id)["unread"].(bool)
	}
	if unread(tm.agent, quiet) || unread(tm.agent, conv) {
		t.Fatal("conversations without messages are unread")
	}
	first := post(key, conv, map[string]any{"kind": "message", "direction": "in", "body": "hello"})
	reply := post(tm.owner, conv, map[string]any{"kind": "message", "body": "hi, how can I help?"})
	if !unread(tm.agent, conv) || !unread(tm.owner, conv) || unread(key, conv) {
		t.Fatal("unread after a contact message: agent and owner should see it, the API key never")
	}

	tab1, tab2, ownerWS := tm.agent.dial(""), tm.agent.dial(""), tm.owner.dial("")
	tab1.ready()
	tab2.ready()
	ownerWS.ready()

	r := tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/read", map[string]any{"message_id": first})
	if r.str("last_read_message_id") != first || r.body["unread"] != true || r.str("member_id") != tm.agentID {
		t.Fatalf("read up to the first message: %s", r.raw)
	}
	r = tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/read", nil)
	if r.str("last_read_message_id") != reply || r.body["unread"] != false {
		t.Fatalf("read to the latest: %s", r.raw)
	}
	if unread(tm.agent, conv) || !unread(tm.owner, conv) {
		t.Fatal("reading is per member")
	}
	r = tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/read", map[string]any{"message_id": first})
	if r.str("last_read_message_id") != reply {
		t.Fatalf("the cursor moved backward: %s", r.raw)
	}
	tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/read", nil)

	tm.say(tm.owner, quiet, "marker")
	for _, tab := range []*wsClient{tab1, tab2} {
		var reads []string
		for _, m := range tab.until(messageIn(quiet)) {
			if m.Type != "conversation.read" {
				continue
			}
			var d struct {
				LastRead string `json:"last_read_message_id"`
				MemberID string `json:"member_id"`
			}
			_ = json.Unmarshal(m.Data, &d)
			if d.MemberID != tm.agentID || m.ConversationID != conv {
				t.Fatalf("read event %+v %s", m, m.Data)
			}
			reads = append(reads, d.LastRead)
		}
		if !slices.Equal(reads, []string{first, reply}) {
			t.Fatalf("read events %v, want only the two moves", reads)
		}
	}
	for _, m := range ownerWS.until(messageIn(quiet)) {
		if m.Type == "conversation.read" {
			t.Fatalf("the owner got the agent's read event: %s", m.Data)
		}
	}

	post(tm.owner, conv, map[string]any{"kind": "note", "body": "agent, take this"})
	if !unread(tm.agent, conv) {
		t.Fatal("a note from another member is unread")
	}
	tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/read", nil)
	post(tm.agent, conv, map[string]any{"kind": "message", "body": "on it"})
	tm.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	if unread(tm.agent, conv) {
		t.Fatal("own messages and events made the conversation unread")
	}

	r = tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+quiet+"/read", nil)
	if r.body["unread"] != false {
		t.Fatalf("read quiet: %s", r.raw)
	}
	tm.agent.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+conv+"/read", map[string]any{"message_id": uuid.New().String()})
	tm.agent.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+quiet+"/read", map[string]any{"message_id": first})
	key.expectProblem(http.StatusForbidden, "member_session_required", "POST", "/v1/conversations/"+conv+"/read", nil)
	other := newTeam(t, h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+conv+"/read", nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+conv+"/read", nil)
}

func TestConversationCounts(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	key := tm.apiKey(h)
	other := newTeam(t, h)
	other.conversation(other.owner)
	inboxB := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Billing", "slug": "billing"}).body["inbox"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	label := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "vip"}).str("id")
	ownerMe := tm.owner.expect(http.StatusOK, "GET", "/v1/me", nil).body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)

	open := func(inbox string, body map[string]any) string {
		body["inbox_id"], body["contact_id"] = inbox, tm.contact
		return tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", body).str("id")
	}
	open(tm.inbox, map[string]any{"assignee_id": tm.agentID, "labels": []string{label}})
	open(tm.inbox, map[string]any{})
	open(tm.inbox, map[string]any{"assignee_id": ownerMe})
	closed := open(tm.inbox, map[string]any{"labels": []string{label}})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+closed, map[string]any{"status": "closed"})
	open(inboxB, map[string]any{"labels": []string{label}})
	open(inboxB, map[string]any{"assignee_id": ownerMe})

	type counts struct {
		All, Mine, Unassigned int64
		Inboxes, Labels       []struct {
			ID    string `json:"id"`
			Count int64  `json:"count"`
		}
	}
	get := func(c *client) counts {
		t.Helper()
		r := c.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil)
		var out counts
		if err := json.Unmarshal(r.raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	byID := func(list []struct {
		ID    string `json:"id"`
		Count int64  `json:"count"`
	}) map[string]int64 {
		m := map[string]int64{}
		for _, x := range list {
			m[x.ID] = x.Count
		}
		return m
	}
	o := get(tm.owner)
	if o.All != 5 || o.Mine != 2 || o.Unassigned != 2 || len(o.Inboxes) != 2 ||
		byID(o.Inboxes)[tm.inbox] != 3 || byID(o.Inboxes)[inboxB] != 2 || len(o.Labels) != 1 || byID(o.Labels)[label] != 2 {
		t.Fatalf("owner counts %+v", o)
	}
	a := get(tm.agent)
	if a.All != 3 || a.Mine != 1 || a.Unassigned != 1 || len(a.Inboxes) != 1 || byID(a.Inboxes)[tm.inbox] != 3 || byID(a.Labels)[label] != 1 {
		t.Fatalf("agent counts %+v", a)
	}
	k := get(key)
	if k.All != 5 || k.Mine != 0 || k.Unassigned != 2 {
		t.Fatalf("key counts %+v", k)
	}
	if n := len(listItems(tm.owner, "?status=open&inbox_id="+tm.inbox)); int64(n) != byID(o.Inboxes)[tm.inbox] {
		t.Fatalf("counts disagree with the list: %d", n)
	}
}

func TestMessagesNewestFirst(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	conv := tm.conversation(tm.owner)
	var ids []string
	for i := range 5 {
		ids = append(ids, post(tm.owner, conv, map[string]any{"kind": "message", "body": "m" + string(rune('0'+i))}))
	}
	var got []string
	cursor := ""
	for {
		path := "/v1/conversations/" + conv + "/messages?order=desc&limit=2"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		r := tm.owner.expect(http.StatusOK, "GET", path, nil)
		for _, m := range r.body["items"].([]any) {
			got = append(got, m.(map[string]any)["id"].(string))
		}
		if cursor == "" {
			post(tm.owner, conv, map[string]any{"kind": "message", "body": "newer, must not shift pages"})
		}
		next, _ := r.body["next_cursor"].(string)
		if next == "" {
			break
		}
		cursor = next
	}
	want := slices.Clone(ids)
	slices.Reverse(want)
	if !slices.Equal(got, want) {
		t.Fatalf("desc pages %v, want %v", got, want)
	}
	asc := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv+"/messages?order=asc&limit=2", nil)
	if asc.body["items"].([]any)[0].(map[string]any)["id"] != ids[0] {
		t.Fatalf("asc: %s", asc.raw)
	}
	tm.owner.expect(http.StatusBadRequest, "GET", "/v1/conversations/"+conv+"/messages?order=sideways", nil)
}

func TestAttachmentWorkspaceQuery(t *testing.T) {
	h := newHarness(t)
	email := unique("both") + "@example.com"
	wsA := h.bootstrap(email, unique("ws")).WorkspaceID.String()
	b := newTeam(t, h)
	b.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": email, "role": "admin"})
	c := h.client()
	c.signIn(email)

	c.workspace = b.ws
	conv := b.conversation(c)
	up := c.upload("/v1/conversations/"+conv+"/messages", map[string]string{"kind": "message", "body": "file"}, []file{{"a.txt", "text/plain", []byte("hello")}})
	att := up.body["attachments"].([]any)[0].(map[string]any)["id"].(string)
	c.workspace = ""

	c.expectProblem(http.StatusBadRequest, "workspace_required", "GET", "/v1/attachments/"+att, nil)
	dl := c.expect(http.StatusOK, "GET", "/v1/attachments/"+att+"?workspace_id="+b.ws, nil)
	if string(dl.raw) != "hello" {
		t.Fatalf("download %q", dl.raw)
	}
	c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/attachments/"+att+"?workspace_id="+wsA, nil)
	c.expectProblem(http.StatusForbidden, "not_a_member", "GET", "/v1/attachments/"+att+"?workspace_id="+uuid.New().String(), nil)
	c.workspace = wsA
	c.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/attachments/"+att+"?workspace_id="+b.ws, nil)
	c.workspace = b.ws
	c.expect(http.StatusOK, "GET", "/v1/attachments/"+att+"?workspace_id="+b.ws, nil)
	c.workspace = ""
	c.expect(http.StatusBadRequest, "GET", "/v1/attachments/"+att+"?workspace_id=nope", nil)
}

func TestSignInKeepsOtherSessions(t *testing.T) {
	h := newHarness(t)
	email := unique("owner") + "@example.com"
	h.bootstrap(email, unique("ws"))
	older, newer := h.client(), h.client()
	older.signIn(email)
	h.clock.Advance(time.Hour)
	newer.signIn(email)
	older.expect(http.StatusOK, "GET", "/v1/me", nil)
	newer.expect(http.StatusOK, "GET", "/v1/me", nil)
	newer.expect(http.StatusNoContent, "POST", "/v1/auth/sign-out", nil)
	older.expect(http.StatusOK, "GET", "/v1/conversations", nil)
	newer.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)
}

func TestAttachmentContentMustMatchType(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	conv := tm.conversation(tm.owner)
	path := "/v1/conversations/" + conv + "/messages"
	fields := map[string]string{"kind": "message", "body": "files"}
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	jpeg := append([]byte("\xff\xd8\xff\xe0"), make([]byte, 64)...)

	for name, f := range map[string]file{
		"text as png":   {"a.png", "image/png", []byte("just some text")},
		"jpeg as png":   {"a.png", "image/png", jpeg},
		"binary as txt": {"a.txt", "text/plain", png},
	} {
		r := tm.owner.upload(path, fields, []file{f})
		if r.status != http.StatusUnsupportedMediaType || r.str("code") != "attachment_type_mismatch" {
			t.Fatalf("%s: %d %s", name, r.status, r.raw)
		}
	}
	if r := tm.owner.upload(path, fields, []file{{"a.pdf", "application/pdf", []byte("%PDF-1.4 x")}}); r.str("code") != "attachment_type_not_allowed" {
		t.Fatalf("disallowed type: %d %s", r.status, r.raw)
	}
	if n := len(messages(tm.owner, conv)); n != 0 {
		t.Fatalf("refused uploads wrote %d messages", n)
	}
	ok := tm.owner.upload(path, fields, []file{
		{"a.jpg", "image/jpg", jpeg},
		{"b.png", "image/png; name=b.png", png},
		{"c.txt", "", []byte("plain")},
	})
	if ok.status != http.StatusCreated {
		t.Fatalf("matching uploads: %d %s", ok.status, ok.raw)
	}
	var types []string
	for _, a := range ok.body["attachments"].([]any) {
		types = append(types, a.(map[string]any)["content_type"].(string))
	}
	if !slices.Equal(types, []string{"image/jpeg", "image/png", "text/plain"}) {
		t.Fatalf("stored types %v", types)
	}
}

func TestContactLookupNeedsInboxAccess(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+tm.contact, map[string]any{
		"external_ids": []map[string]any{{"inbox_id": tm.inbox, "external_id": "user-1"}},
	})
	path := "/v1/contacts/lookup?inbox_id=" + tm.inbox + "&external_id=user-1"
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", path, nil)
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/contacts/lookup?inbox_id="+tm.inbox+"&external_id=nobody", nil)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	if r := tm.agent.expect(http.StatusOK, "GET", path, nil); r.str("id") != tm.contact {
		t.Fatalf("lookup with access: %s", r.raw)
	}
	other := newTeam(t, h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", path, nil)
}

func TestSearchNegationExcludesConversation(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	refund := tm.conversation(tm.owner)
	late := tm.conversation(tm.owner)
	post(tm.owner, refund, map[string]any{"kind": "message", "body": "your invoice is paid"})
	post(tm.owner, refund, map[string]any{"kind": "note", "body": "customer wants a refund please"})
	post(tm.owner, late, map[string]any{"kind": "message", "body": "your invoice is late"})
	search := func(q string) []string {
		t.Helper()
		var ids []string
		for _, it := range listItems(tm.owner, "?q="+url.QueryEscape(q)) {
			ids = append(ids, it["id"].(string))
		}
		slices.Sort(ids)
		return ids
	}
	both := []string{refund, late}
	slices.Sort(both)
	for q, want := range map[string][]string{
		"invoice":                  both,
		"invoice -refund":          {late},
		"-refund":                  {late},
		`invoice -"refund please"`: {late},
		`invoice -"please refund"`: both,
		"invoice -late -refund":    nil,
		"-help":                    nil,
		"invoice -ayşe":            nil,
		"refund or late":           both,
		"paid -late":               {refund},
	} {
		if got := search(q); !slices.Equal(got, want) {
			t.Fatalf("q=%q: got %v, want %v", q, got, want)
		}
	}
}

func TestInboxCreatedAndDeletedEvents(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	ownerWS, agentWS := tm.owner.dial(""), tm.agent.dial("")
	ownerWS.ready()
	agentWS.ready()

	created := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Billing", "slug": "billing"}).body["inbox"].(map[string]any)
	inbox := created["id"].(string)
	m := ownerWS.next()
	var data map[string]any
	_ = json.Unmarshal(m.Data, &data)
	if m.Type != "inbox.created" || m.InboxID != inbox || data["slug"] != "billing" {
		t.Fatalf("owner got %+v %s", m, m.Data)
	}
	if strings.Contains(string(m.Data), "identity_secret") {
		t.Fatal("inbox.created carries the identity secret")
	}
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+inbox+"/members/"+tm.agentID, nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+inbox, nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/inboxes/"+inbox, nil)

	var types []string
	for _, m := range agentWS.until(func(m wsMessage) bool { return m.Type == "inbox.deleted" }) {
		types = append(types, m.Type+"@"+m.InboxID)
		if m.Type == "inbox.deleted" && !strings.Contains(string(m.Data), inbox) {
			t.Fatalf("inbox.deleted data %s", m.Data)
		}
	}
	if want := []string{"inbox_access.changed@" + inbox, "inbox.deleted@" + inbox}; !slices.Equal(types, want) {
		t.Fatalf("agent events %v, want %v", types, want)
	}
	if m := ownerWS.until(func(m wsMessage) bool { return m.Type == "inbox.deleted" }); m[len(m)-1].InboxID != inbox {
		t.Fatalf("owner inbox.deleted %+v", m)
	}

	hidden := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Hidden", "slug": "hidden"}).body["inbox"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+hidden, nil)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	for _, m := range agentWS.until(func(m wsMessage) bool { return m.Type == "inbox_access.changed" }) {
		if m.InboxID == hidden {
			t.Fatalf("agent got %s for an inbox it never saw", m.Type)
		}
	}
}

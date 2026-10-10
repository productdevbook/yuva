package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/coder/websocket"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/realtime"
)

type wsMessage struct {
	ID             int64           `json:"id"`
	Type           string          `json:"type"`
	WorkspaceID    string          `json:"workspace_id"`
	InboxID        string          `json:"inbox_id"`
	ConversationID string          `json:"conversation_id"`
	LastEventID    int64           `json:"last_event_id"`
	Data           json.RawMessage `json:"data"`
}

type wsClient struct {
	t    *testing.T
	conn *websocket.Conn
	msgs chan wsMessage
	err  chan error
}

func (c *client) dialStatus(query string, header http.Header) (*websocket.Conn, int, error) {
	c.h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !c.h.hub.Listening() {
		if time.Now().After(deadline) {
			c.h.t.Fatal("realtime listener did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if header == nil {
		header = http.Header{"Origin": {testOrigin}}
	}
	if c.bearer != "" {
		header.Set("Authorization", "Bearer "+c.bearer)
	}
	if c.workspace != "" {
		header.Set("Yuva-Workspace", c.workspace)
	}
	u := "ws" + strings.TrimPrefix(c.h.url, "http") + "/v1/realtime" + query
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, res, err := websocket.Dial(ctx, u, &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: header})
	status := 0
	if res != nil {
		status = res.StatusCode
	}
	return conn, status, err
}

func (c *client) dial(query string) *wsClient {
	c.h.t.Helper()
	conn, status, err := c.dialStatus(query, nil)
	if err != nil {
		c.h.t.Fatalf("dial %s: status %d: %v", query, status, err)
	}
	conn.SetReadLimit(1 << 20)
	w := &wsClient{t: c.h.t, conn: conn, msgs: make(chan wsMessage, 10000), err: make(chan error, 1)}
	go func() {
		for {
			_, b, err := conn.Read(context.Background())
			if err != nil {
				w.err <- err
				return
			}
			var m wsMessage
			if err := json.Unmarshal(b, &m); err != nil {
				w.err <- err
				return
			}
			w.msgs <- m
		}
	}()
	c.h.t.Cleanup(func() { _ = conn.CloseNow() })
	return w
}

// teamSignals are the notices about teammates and contacts' presence that member connections get;
// tests that do not look for them skip them.
var teamSignals = map[string]bool{"member.presence": true, "viewing": true, "contact.presence": true}

func (w *wsClient) next() wsMessage {
	w.t.Helper()
	for {
		m := w.nextAny()
		if !teamSignals[m.Type] {
			return m
		}
	}
}

// nextOf returns the next message of the given type, skipping others.
func (w *wsClient) nextOf(typ string) wsMessage {
	w.t.Helper()
	for {
		if m := w.nextAny(); m.Type == typ {
			return m
		}
	}
}

func (w *wsClient) nextAny() wsMessage {
	w.t.Helper()
	select {
	case m := <-w.msgs:
		return m
	case err := <-w.err:
		w.t.Fatalf("realtime read: %v", err)
	case <-time.After(10 * time.Second):
		w.t.Fatal("no realtime message within 10s")
	}
	return wsMessage{}
}

func (w *wsClient) ready() int64 {
	w.t.Helper()
	m := w.next()
	if m.Type != "ready" {
		w.t.Fatalf("first message %q, want ready", m.Type)
	}
	return m.LastEventID
}

func (w *wsClient) until(stop func(wsMessage) bool) []wsMessage {
	w.t.Helper()
	var out []wsMessage
	for {
		m := w.next()
		out = append(out, m)
		if stop(m) {
			return out
		}
	}
}

func (w *wsClient) close() { _ = w.conn.Close(websocket.StatusNormalClosure, "") }

func (h *harness) eventIDs(workspace string, after int64) []int64 {
	h.t.Helper()
	rows, err := h.st.Pool.Query(context.Background(), "SELECT id FROM events WHERE workspace_id = $1 AND id > $2 ORDER BY id", workspace, after)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, id)
	}
	return out
}

func eventIDsOf(ms []wsMessage) []int64 {
	var out []int64
	for _, m := range ms {
		if m.ID != 0 {
			out = append(out, m.ID)
		}
	}
	return out
}

func messageIn(conv string) func(wsMessage) bool {
	return func(m wsMessage) bool { return m.Type == "message.created" && m.ConversationID == conv }
}

func (tm team) say(c *client, conv, body string) {
	c.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "direction": "out", "body": body})
}

func TestRealtimeAccessFiltering(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	inboxB := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Billing", "slug": "billing"}).body["inbox"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	key := h.client()
	key.bearer = tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "backend"}).str("secret")

	owner, agent, keyWS, outsider := tm.owner.dial(""), tm.agent.dial(""), key.dial(""), other.owner.dial("")
	owner.ready()
	agent.ready()
	keyWS.ready()
	outsider.ready()

	convB := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": inboxB, "contact_id": tm.contact, "subject": "Invoice"}).str("id")
	tm.say(tm.owner, convB, "secret billing reply")
	convA := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+tm.contact, map[string]any{"name": "Ayşe Y."})
	tm.say(tm.owner, convA, "hello from A")

	for name, ws := range map[string]*wsClient{"owner": owner, "key": keyWS} {
		got := ws.until(messageIn(convA))
		var types []string
		for _, m := range got {
			types = append(types, m.Type+"@"+m.InboxID)
			if m.WorkspaceID != tm.ws {
				t.Fatalf("%s got an event of workspace %s", name, m.WorkspaceID)
			}
		}
		want := []string{"conversation.created@" + inboxB, "message.created@" + inboxB, "conversation.created@" + tm.inbox, "contact.updated@", "message.created@" + tm.inbox}
		if name == "owner" {
			want = slices.Insert(want, 2, "conversation.read@"+inboxB)
		}
		if !slices.Equal(types, want) {
			t.Fatalf("%s events %v, want %v", name, types, want)
		}
	}
	got := agent.until(messageIn(convA))
	var types []string
	for _, m := range got {
		types = append(types, m.Type+"@"+m.InboxID)
	}
	if want := []string{"conversation.created@" + tm.inbox, "contact.updated@", "message.created@" + tm.inbox}; !slices.Equal(types, want) {
		t.Fatalf("agent events %v, want %v", types, want)
	}

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	tm.say(tm.owner, convA, "agent may not see this")
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+inboxB+"/members/"+tm.agentID, nil)
	tm.say(tm.owner, convB, "agent sees this")
	got = agent.until(messageIn(convB))
	types = nil
	for _, m := range got {
		types = append(types, m.Type+"@"+m.InboxID)
	}
	if want := []string{"inbox_access.changed@" + tm.inbox, "inbox_access.changed@" + inboxB, "message.created@" + inboxB}; !slices.Equal(types, want) {
		t.Fatalf("agent events after access changes %v, want %v", types, want)
	}
	var change struct {
		MemberID string `json:"member_id"`
		Granted  bool   `json:"granted"`
	}
	_ = json.Unmarshal(got[0].Data, &change)
	if change.MemberID != tm.agentID || change.Granted {
		t.Fatalf("revocation payload %s", got[0].Data)
	}
	owner.until(messageIn(convB))

	other.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+other.contact, map[string]any{"name": "Mehmet"})
	m := outsider.next()
	if m.Type != "contact.updated" || m.WorkspaceID != other.ws {
		t.Fatalf("outsider got %+v, want only its own contact.updated", m)
	}
}

func TestRealtimeRefusals(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	if _, status, _ := h.client().dialStatus("", nil); status != http.StatusUnauthorized {
		t.Fatalf("anonymous: status %d", status)
	}
	if _, status, _ := tm.owner.dialStatus("", http.Header{"Origin": {"https://evil.example.com"}}); status != http.StatusForbidden {
		t.Fatalf("foreign origin: status %d", status)
	}
	if _, status, _ := tm.owner.dialStatus("?last_event_id=-1", nil); status != http.StatusBadRequest {
		t.Fatalf("negative last_event_id: status %d", status)
	}
	if _, status, _ := tm.owner.dialStatus("?workspace_id="+uuid.New().String(), nil); status != http.StatusForbidden {
		t.Fatalf("foreign workspace: status %d", status)
	}
	ws := tm.owner.dial("?workspace_id=" + tm.ws)
	ws.ready()
}

func TestRealtimeReplayHasNoGaps(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	conv := tm.conversation(tm.owner)
	first := tm.owner.dial("")
	start := first.ready()
	first.close()

	for i := range 5 {
		tm.say(tm.owner, conv, fmt.Sprintf("offline %d", i))
	}
	const live = 40
	done := make(chan struct{})
	go func() {
		defer close(done)
		writer := h.client()
		writer.http = tm.owner.http
		for i := range live {
			r := writer.do("POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": fmt.Sprintf("during %d", i)})
			if r.status != http.StatusCreated {
				t.Errorf("note %d: %d %s", i, r.status, r.raw)
				return
			}
		}
	}()
	time.Sleep(20 * time.Millisecond)
	ws := tm.owner.dial(fmt.Sprintf("?last_event_id=%d", start))
	<-done
	want := h.eventIDs(tm.ws, start)
	final := want[len(want)-1]
	got := ws.until(func(m wsMessage) bool { return m.ID == final })
	readies := 0
	for _, m := range got {
		if m.Type == "ready" {
			readies++
		}
	}
	if readies != 1 {
		t.Fatalf("%d ready messages", readies)
	}
	if ids := eventIDsOf(got); !slices.Equal(ids, want) {
		t.Fatalf("received ids %v\nwant %v", ids, want)
	}

	tm.say(tm.owner, conv, "after")
	m := ws.next()
	if m.Type != "message.created" || m.ID <= final {
		t.Fatalf("live event after replay: %+v", m)
	}
}

func TestRealtimeResyncRequired(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	conv := tm.conversation(tm.owner)
	ws := tm.owner.dial("")
	ws.ready()
	tm.say(tm.owner, conv, "old")
	old := ws.until(messageIn(conv))
	ws.close()
	oldID := old[len(old)-1].ID

	worker := &jobs.EventCleanupWorker{Queries: h.st.Queries, Now: func() time.Time { return time.Now().Add(jobs.EventRetention + time.Minute) }}
	if err := worker.Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if ids := h.eventIDs(tm.ws, 0); len(ids) != 0 {
		t.Fatalf("cleanup left events %v", ids)
	}
	tm.say(tm.owner, conv, "new")

	for _, q := range []string{fmt.Sprintf("?last_event_id=%d", oldID), "?last_event_id=999999999999"} {
		ws := tm.owner.dial(q)
		if m := ws.next(); m.Type != "resync_required" {
			t.Fatalf("%s: first message %q, want resync_required", q, m.Type)
		}
		last := ws.ready()
		if want := h.eventIDs(tm.ws, 0); last != want[len(want)-1] {
			t.Fatalf("%s: ready at %d, want %d", q, last, want[len(want)-1])
		}
		tm.say(tm.owner, conv, "live")
		if m := ws.next(); m.Type != "message.created" || m.ID <= last {
			t.Fatalf("%s: live event %+v", q, m)
		}
		ws.close()
	}
}

func TestRealtimeSlowConsumerIsDisconnected(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	conn, status, err := tm.owner.dialStatus("", nil)
	if err != nil {
		t.Fatalf("dial: %d %v", status, err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(-1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, _, err := conn.Read(ctx); err != nil {
		t.Fatal(err)
	}
	ws := uuid.MustParse(tm.ws)
	data, _ := json.Marshal(map[string]string{"pad": strings.Repeat("x", 64<<10)})
	published := make(chan struct{})
	go func() {
		defer close(published)
		for i := range 2000 {
			h.hub.Publish(realtime.Event{ID: int64(1<<40 + i), Type: realtime.ContactUpdated, WorkspaceID: ws, CreatedAt: time.Now(), Data: data})
		}
	}()
	select {
	case <-published:
	case <-time.After(5 * time.Second):
		t.Fatal("publishing to a slow consumer blocked the hub")
	}
	for {
		_, _, err := conn.Read(ctx)
		if err == nil {
			continue
		}
		var ce websocket.CloseError
		if !errors.As(err, &ce) || ce.Code != websocket.StatusTryAgainLater || ce.Reason != realtime.ReasonSlowConsumer {
			t.Fatalf("closed with %v, want 1013 slow_consumer", err)
		}
		return
	}
}

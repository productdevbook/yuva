package api_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
	"uuid"

	"github.com/coder/websocket"

	"github.com/productdevbook/yuva/api/internal/api"
)

func TestClientSocketCaps(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	cs := ct.session(h, map[string]any{})
	cs.start("hello")
	owner := ct.owner.dial("")
	owner.ready()

	var open []*wsClient
	refused, full := 0, 0
	for range 100 {
		w, status, err := dialContact(h, cs.token, ct.origin, "")
		if err != nil {
			if status != http.StatusTooManyRequests {
				t.Fatalf("dial: %d %v", status, err)
			}
			refused++
			continue
		}
		select {
		case m := <-w.msgs:
			if m.Type != "ready" {
				t.Fatalf("first frame %s", m.Type)
			}
			open = append(open, w)
		case err := <-w.err:
			var ce websocket.CloseError
			if !errors.As(err, &ce) || ce.Code != websocket.StatusTryAgainLater || ce.Reason != "too_many_connections;retry_after=30" {
				t.Fatalf("a socket over the cap ended with %v", err)
			}
			full++
		case <-time.After(5 * time.Second):
			t.Fatal("no first frame")
		}
	}
	if len(open) != 20 || full != 10 || refused != 70 {
		t.Fatalf("%d sockets open, %d closed over the cap and %d refused in a minute, want 20, 10 and 70", len(open), full, refused)
	}
	for _, w := range open {
		select {
		case err := <-w.err:
			t.Fatalf("an open socket was closed for a newer one: %v", err)
		default:
		}
	}
	if n := h.count("SELECT count(*) FROM realtime_connections WHERE workspace_id = $1 AND contact_id = $2", ct.ws, cs.contactID); n != 20 {
		t.Fatalf("%d connections of the session open, want 20", n)
	}
	open[0].close()
	time.Sleep(200 * time.Millisecond)
	h.clock.Advance(time.Minute)
	w, _, err := dialContact(h, cs.token, ct.origin, "")
	if err != nil {
		t.Fatal(err)
	}
	if m := w.next(); m.Type != "ready" {
		t.Fatalf("a socket after one closed: %s", m.Type)
	}
	w.close()
	presence := 0
	for {
		select {
		case m := <-owner.msgs:
			if m.Type == "contact.presence" {
				presence++
			}
			continue
		case <-time.After(2500 * time.Millisecond):
		}
		break
	}
	if presence > 1 {
		t.Fatalf("%d presence notices for one contact's 20 sockets, want 1", presence)
	}

	var head int64
	if err := h.st.Pool.QueryRow(t.Context(), "SELECT max(id) FROM events WHERE workspace_id = $1", ct.ws).Scan(&head); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.Pool.Exec(t.Context(), "INSERT INTO events (workspace_id, type, payload) SELECT $1, 'test.noise', '{}' FROM generate_series(1, 1200)", ct.ws); err != nil {
		t.Fatal(err)
	}
	for _, o := range open[1:] {
		o.close()
	}
	time.Sleep(200 * time.Millisecond)
	w, _, err = dialContact(h, cs.token, ct.origin, fmt.Sprintf("?last_event_id=%d", head))
	if err != nil {
		t.Fatal(err)
	}
	frames := w.until(func(m wsMessage) bool { return m.Type == "ready" })
	if len(frames) != 2 || frames[0].Type != "resync_required" || frames[1].LastEventID <= head+1200-1 {
		t.Fatalf("resuming 1,200 events back: %v", frames)
	}
	w.close()

	later := ct.session(h, map[string]any{"visitor_id": cs.visitor})
	if later.contactID != cs.contactID {
		t.Fatal("the visitor was not resumed")
	}
	w, _, err = dialContact(h, later.token, ct.origin, "?last_event_id=1")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range w.until(func(m wsMessage) bool { return m.Type == "ready" }) {
		if m.Type != "ready" {
			var d map[string]any
			_ = json.Unmarshal(m.Data, &d)
			t.Fatalf("a new session replayed %s from before it was created: %v", m.Type, d)
		}
	}
	w.close()
}

func TestContactPresenceCountsLoadedSockets(t *testing.T) {
	x := api.NewSockets()
	session, contact := uuid.New(), uuid.New()
	failing := x.Open(session, contact, "198.51.100.1")
	loaded := x.Open(session, contact, "198.51.100.1")
	if !x.Ready(loaded) {
		t.Fatal("the first socket that loaded did not announce the contact online")
	}
	if x.Close(failing) {
		t.Fatal("a socket that never loaded announced the contact offline")
	}
	second := x.Open(session, contact, "198.51.100.1")
	if x.Ready(second) {
		t.Fatal("a second loaded socket announced the contact online again")
	}
	if x.Close(loaded) {
		t.Fatal("offline while a loaded socket is still open")
	}
	if !x.Close(second) {
		t.Fatal("the last loaded socket did not announce the contact offline")
	}
}

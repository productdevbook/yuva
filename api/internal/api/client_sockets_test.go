package api_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestClientSocketCaps(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	cs := ct.session(h, map[string]any{})
	cs.start("hello")
	owner := ct.owner.dial("")
	owner.ready()

	var open []*wsClient
	refused := 0
	for range 100 {
		w, status, err := dialContact(h, cs.token, ct.origin, "")
		if err != nil {
			if status != http.StatusTooManyRequests {
				t.Fatalf("dial: %d %v", status, err)
			}
			refused++
			continue
		}
		w.ready()
		open = append(open, w)
	}
	if len(open) != 30 || refused != 70 {
		t.Fatalf("%d sockets accepted and %d refused in a minute, want 30 and 70", len(open), refused)
	}
	kicked := 0
	for _, w := range open[:len(open)-5] {
		select {
		case err := <-w.err:
			var ce websocket.CloseError
			if !errors.As(err, &ce) || ce.Code != websocket.StatusTryAgainLater || ce.Reason != "too_many_connections" {
				t.Fatalf("an older socket ended with %v", err)
			}
			kicked++
		case <-time.After(3 * time.Second):
			t.Fatal("an older socket of the session stayed open")
		}
	}
	for _, w := range open[len(open)-5:] {
		select {
		case err := <-w.err:
			t.Fatalf("one of the 5 newest sockets closed: %v", err)
		default:
		}
	}
	if n := h.count("SELECT count(*) FROM realtime_connections WHERE workspace_id = $1 AND contact_id = $2", ct.ws, cs.contactID); n != 5 {
		t.Fatalf("%d connections of the session open, want 5", n)
	}
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
		t.Fatalf("%d presence notices for one contact's 30 sockets, want 1", presence)
	}

	var head int64
	if err := h.st.Pool.QueryRow(t.Context(), "SELECT max(id) FROM events WHERE workspace_id = $1", ct.ws).Scan(&head); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.Pool.Exec(t.Context(), "INSERT INTO events (workspace_id, type, payload) SELECT $1, 'test.noise', '{}' FROM generate_series(1, 1200)", ct.ws); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Minute)
	w, _, err := dialContact(h, cs.token, ct.origin, fmt.Sprintf("?last_event_id=%d", head))
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

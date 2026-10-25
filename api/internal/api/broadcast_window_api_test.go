package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"
)

func setClock(c *clock, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = at
}

func window(start, end string) map[string]any { return map[string]any{"start": start, "end": end} }

func TestBroadcastTimeZoneAndSendAtLocal(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	setClock(h.clock, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "local@example.org")
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+bt.chatInbox, map[string]any{"timezone": "America/New_York"})

	b := bt.get(t, bt.draft(t, nil))
	if b["timezone"] != "America/New_York" || b["send_window"] != nil || b["window_opens_at"] != nil {
		t.Fatalf("a new broadcast takes the inbox's zone and no window: %v", b)
	}
	for _, bad := range []map[string]any{
		{"timezone": "+03:00"}, {"timezone": "Mars/Olympus"}, {"timezone": "Local"}, {"timezone": ""},
		{"send_window": window("05:00", "05:20")}, {"send_window": window("00:00", "24:00")}, {"send_window": window("5:00", "12:00")},
		{"send_window": window("24:00", "06:00")},
	} {
		body := map[string]any{"inbox_id": bt.chatInbox, "name": "Bad"}
		for k, v := range bad {
			body[k] = v
		}
		bt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/broadcasts", body)
	}

	id := bt.draft(t, map[string]any{"timezone": "Europe/Istanbul", "send_window": window("22:00", "06:00")})
	b = bt.get(t, id)
	if w := b["send_window"].(map[string]any); b["timezone"] != "Europe/Istanbul" || w["start"] != "22:00" || w["end"] != "06:00" {
		t.Fatalf("created with zone and window: %v", b)
	}
	b = bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+id, map[string]any{"send_window": nil, "name": "Istanbul news"}).body
	if b["send_window"] != nil || b["timezone"] != "Europe/Istanbul" || b["name"] != "Istanbul news" {
		t.Fatalf("send_window null on a draft: %v", b)
	}
	bt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/broadcasts/"+id, map[string]any{"timezone": "GMT+3"})

	path := "/v1/broadcasts/" + id + "/schedule"
	bt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", path, map[string]any{"send_at": "2026-11-02T06:00:00Z", "send_at_local": "2026-11-02T09:00"})
	for _, bad := range []string{"2026-11-02 09:00", "2026-11-02T09:00:00", "2026-11-02T09:00+03:00", "2026-13-02T09:00"} {
		bt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", path, map[string]any{"send_at_local": bad})
	}
	bt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", path, map[string]any{"send_at_local": "2027-02-02T09:00"})
	b = bt.owner.expect(http.StatusOK, "POST", path, map[string]any{"send_at_local": "2026-11-02T09:00"}).body
	if !timeOf(t, b["scheduled_at"]).Equal(time.Date(2026, 11, 2, 6, 0, 0, 0, time.UTC)) || b["state"] != "scheduled" {
		t.Fatalf("send_at_local 09:00 Istanbul: %v", b["scheduled_at"])
	}
	b = bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+id, map[string]any{"timezone": "Europe/Berlin"}).body
	if !timeOf(t, b["scheduled_at"]).Equal(time.Date(2026, 11, 2, 6, 0, 0, 0, time.UTC)) || b["timezone"] != "Europe/Berlin" {
		t.Fatalf("changing the zone moved scheduled_at: %v", b)
	}
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+id, map[string]any{"timezone": "Europe/Istanbul"})

	cs := h.mcpSession(h.oauthToken(bt.owner, bt.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read", "lists:read", "broadcasts:read", "broadcasts:write"}))
	got, errText := callTool(t, cs, "get_broadcast", map[string]any{"broadcast_id": id})
	if errText != "" {
		t.Fatal(errText)
	}
	if mb := got["broadcast"].(map[string]any); mb["scheduled_at_local"] != "2026-11-02T09:00" || mb["scheduled_at_offset"] != "GMT+3" || mb["timezone"] != "Europe/Istanbul" {
		t.Fatalf("MCP broadcast: %v", mb)
	}
	if _, errText := callTool(t, cs, "update_broadcast", map[string]any{"broadcast_id": id, "send_window_start": "05:00"}); !strings.Contains(errText, "together") {
		t.Fatalf("half a window through MCP: %q", errText)
	}
	got, errText = callTool(t, cs, "update_broadcast", map[string]any{"broadcast_id": id, "send_window_start": "05:00", "send_window_end": "12:00"})
	if errText != "" {
		t.Fatal(errText)
	}
	if w := got["broadcast"].(map[string]any)["send_window"].(map[string]any); w["start"] != "05:00" || w["end"] != "12:00" {
		t.Fatalf("window through MCP on a scheduled broadcast: %v", got)
	}
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+id, map[string]any{"send_window": window("14:00", "16:00")})
	it := reviewItem(bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body, "send_window")
	if it["status"] != "warning" || it["detail"] != "the window is closed at the scheduled time; sending starts at 14:00 Europe/Istanbul (2 Nov 2026 11:00 UTC)" {
		t.Fatalf("review of a scheduled broadcast whose window is closed then: %v", it)
	}
	if b := bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+id, map[string]any{"send_window": window("18:00", "00:00")}).body; b["send_window"].(map[string]any)["end"] != "24:00" {
		t.Fatalf("a window ending at 00:00: %v", b["send_window"])
	}
	got, errText = callTool(t, cs, "create_broadcast_draft", map[string]any{
		"inbox_id": bt.chatInbox, "name": "MCP draft", "subject": "Hi", "list_ids": []string{bt.list},
		"timezone": "Asia/Kolkata", "send_window_start": "09:00", "send_window_end": "17:30",
	})
	if errText != "" {
		t.Fatal(errText)
	}
	if got["timezone"] != "Asia/Kolkata" || got["send_window"].(map[string]any)["end"] != "17:30" {
		t.Fatalf("MCP draft with zone and window: %v", got)
	}
	if _, errText := callTool(t, cs, "update_broadcast", map[string]any{"broadcast_id": got["id"], "remove_send_window": true}); errText != "" {
		t.Fatal(errText)
	}
	if b := bt.get(t, got["id"].(string)); b["send_window"] != nil {
		t.Fatalf("remove_send_window: %v", b)
	}

	other := newBroadcastTeam(t, h, bs)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/broadcasts/"+id, map[string]any{"send_window": window("05:00", "12:00")})
}

func TestBroadcastSendWindowWhileSending(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	setClock(h.clock, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "s1@example.org", "s2@example.org", "s3@example.org")
	id := bt.draft(t, map[string]any{"timezone": "Europe/Istanbul", "send_window": window("05:00", "12:00"), "pace": map[string]any{"per_hour": 1}})
	path := "/v1/broadcasts/" + id
	review := bt.owner.expect(http.StatusOK, "GET", path+"/review", nil).body
	it := reviewItem(review, "send_window")
	if it["status"] != "warning" || it["detail"] != "the window is closed now; sending starts at 05:00 Europe/Istanbul (21 Oct 2026 02:00 UTC)" {
		t.Fatalf("review of a closed window: %v", it)
	}
	rt := bt.owner.dial("")
	rt.ready()
	bt.schedule(t, id)
	bt.run(t)
	for {
		var p map[string]any
		if err := json.Unmarshal(rt.nextOf("broadcast.progress").Data, &p); err != nil {
			t.Fatal(err)
		}
		if p["state"] == "sending" && p["window_opens_at"] != nil {
			if !timeOf(t, p["window_opens_at"]).Equal(time.Date(2026, 10, 21, 2, 0, 0, 0, time.UTC)) {
				t.Fatalf("progress while the window is closed: %v", p)
			}
			break
		}
	}
	b := bt.get(t, id)
	if bs.count() != 0 || b["state"] != "sending" || !timeOf(t, b["window_opens_at"]).Equal(time.Date(2026, 10, 21, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("outside the window: %d %v", bs.count(), b)
	}
	b = bt.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"send_window": window("14:00", "16:00")}).body
	if b["state"] != "sending" || b["window_opens_at"] != nil || b["send_window"].(map[string]any)["start"] != "14:00" {
		t.Fatalf("window changed while sending: %v", b)
	}
	ev := broadcastEvents(t, bt, id)
	if last := ev[len(ev)-1]["broadcast"].(map[string]any); last["send_window"].(map[string]any)["start"] != "14:00" {
		t.Fatalf("broadcast.updated for the window change: %v", last)
	}
	bt.owner.expectProblem(http.StatusConflict, "not_draft", "PATCH", path, map[string]any{"send_window": window("05:00", "12:00"), "name": "x"})
	bt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path, map[string]any{"send_window": window("14:00", "14:10")})
	bt.run(t)
	if bs.count() != 1 {
		t.Fatalf("inside the new window: %d", bs.count())
	}
	b = bt.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"timezone": "UTC", "pace": map[string]any{}}).body
	if b["timezone"] != "UTC" || !timeOf(t, b["window_opens_at"]).Equal(time.Date(2026, 10, 20, 14, 0, 0, 0, time.UTC)) {
		t.Fatalf("zone changed while sending: %v", b)
	}
	bt.run(t)
	if bs.count() != 1 {
		t.Fatalf("mail went out while the window in the new zone is closed: %d", bs.count())
	}
	b = bt.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"send_window": nil}).body
	if b["send_window"] != nil || b["window_opens_at"] != nil {
		t.Fatalf("window removed: %v", b)
	}
	if wait := bt.run(t); wait >= 0 {
		t.Fatalf("send left work: %v", wait)
	}
	if b := bt.get(t, id); b["state"] != "sent" || bs.count() != 3 {
		t.Fatalf("after removing the window: %d %v", bs.count(), b)
	}
	bt.owner.expectProblem(http.StatusConflict, "invalid_state", "PATCH", path, map[string]any{"send_window": window("05:00", "12:00")})
	bt.owner.expectProblem(http.StatusConflict, "invalid_state", "PATCH", path, map[string]any{"timezone": "Europe/Istanbul"})
}

// Two senders share one database across six local days that include a clock change; the clock
// moves to whatever either of them waits for next.
func TestBroadcastSendWindowTwoSenders(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	setClock(h.clock, time.Date(2027, 3, 25, 0, 0, 0, 0, berlin))
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribeMany(t, "w", 500)
	id := bt.draft(t, map[string]any{"timezone": "Europe/Berlin", "send_window": window("05:00", "12:00"), "pace": map[string]any{"per_day": 100}})
	review := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body
	if review["days"] != float64(5) || review["duration_seconds"] != float64(5*3600+4*86400+99*252) {
		t.Fatalf("estimate with a window: %v", review)
	}
	bt.schedule(t, id)
	bt.run(t)
	if bs.count() != 0 {
		t.Fatalf("mail before the window opened: %d", bs.count())
	}
	ctx := context.Background()
	ws, ch := uuid.MustParse(bt.ws), uuid.MustParse(bt.emailChannel)
	end := time.Date(2027, 3, 31, 0, 0, 0, 0, berlin)
	for rounds := 0; h.clock.Now().Before(end); rounds++ {
		if rounds > 20000 {
			t.Fatal("too many rounds")
		}
		var wg sync.WaitGroup
		waits, errs := make([]time.Duration, 2), make([]error, 2)
		for i := range 2 {
			wg.Go(func() { waits[i], errs[i] = h.srv.SendBroadcasts(ctx, ws, ch, time.Minute) })
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if waits[0] == 0 || waits[1] == 0 {
			continue
		}
		if waits[0] < 0 && waits[1] < 0 {
			break
		}
		step := max(waits[0], waits[1])
		for _, w := range waits {
			if w > 0 {
				step = min(step, w)
			}
		}
		h.clock.Advance(step)
	}
	var state string
	if err := h.st.Pool.QueryRow(ctx, `SELECT state FROM broadcasts WHERE workspace_id = $1 AND id = $2`, ws, id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	rows, err := h.st.Pool.Query(ctx, `SELECT sent_at FROM broadcast_deliveries WHERE workspace_id = $1 AND broadcast_id = $2 AND sent_at IS NOT NULL ORDER BY sent_at`, ws, id)
	if err != nil {
		t.Fatal(err)
	}
	var sent []time.Time
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			t.Fatal(err)
		}
		sent = append(sent, at)
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	seen := map[string]bool{}
	bs.mu.Lock()
	for _, m := range bs.sent {
		seen[m.to[0]] = true
	}
	mails := len(bs.sent)
	bs.mu.Unlock()
	if state != "sent" || len(sent) != 500 || mails != 500 || len(seen) != 500 {
		t.Fatalf("state %s, %d deliveries sent, %d mails to %d addresses", state, len(sent), mails, len(seen))
	}
	perDay := map[string]int{}
	for i, at := range sent {
		l := at.In(berlin)
		if m := l.Hour()*60 + l.Minute(); m < 5*60 || m >= 12*60 {
			t.Fatalf("mail at %v, outside 05:00–12:00 Berlin", l)
		}
		perDay[l.Format("2006-01-02")]++
		if i > 0 && at.Sub(sent[i-1]) < 252*time.Second {
			t.Fatalf("two mails %v apart at %v: a slot was taken twice", at.Sub(sent[i-1]), l)
		}
	}
	for _, day := range []string{"2027-03-25", "2027-03-26", "2027-03-27", "2027-03-28", "2027-03-29"} {
		if perDay[day] != 100 {
			t.Fatalf("mails per local day: %v", perDay)
		}
	}
	if first := sent[300].UTC(); !first.Equal(time.Date(2027, 3, 28, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("on the day the clocks change the window opened at %v, want 03:00 UTC", first)
	}
}

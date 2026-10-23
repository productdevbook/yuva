package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
	"uuid"
)

type forgetCase struct {
	h       *harness
	tm      team
	rcv     *receiver
	key     *client
	hook    string
	conv    string
	secret  string
	pending string
}

func newForgetCase(t *testing.T, h *harness, tag string) forgetCase {
	t.Helper()
	tm := newTeam(t, h)
	rcv := newReceiver(t)
	hook := tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{
		"url": rcv.srv.URL, "events": []string{"message.created", "contact.updated"},
	}).body["endpoint"].(map[string]any)["id"].(string)
	f := forgetCase{h: h, tm: tm, rcv: rcv, hook: hook, secret: "kept-secret-" + tag, pending: "pending-secret-" + tag}
	f.conv = tm.conversation(tm.owner)
	key := tm.apiKey(h)
	f.key = key
	key.expect(http.StatusCreated, "POST", "/v1/conversations/"+f.conv+"/messages", map[string]any{"kind": "message", "direction": "in", "body": f.secret})
	h.runWebhooks(tm.ws)
	if got := rcv.take(); len(got) != 1 || !strings.Contains(string(got[0].body), f.secret) {
		t.Fatalf("first delivery: %v", got)
	}
	rcv.setStatus(http.StatusInternalServerError)
	key.expect(http.StatusCreated, "POST", "/v1/conversations/"+f.conv+"/messages", map[string]any{"kind": "message", "direction": "in", "body": f.pending})
	if retries := h.runWebhooks(tm.ws); retries != 1 {
		t.Fatalf("%d retries for the failing delivery", retries)
	}
	rcv.take()
	rcv.setStatus(http.StatusOK)
	feed := tm.owner.expect(http.StatusOK, "GET", "/v1/events?after=0&limit=500", nil)
	if !strings.Contains(string(feed.raw), f.secret) || !strings.Contains(string(feed.raw), f.pending) {
		t.Fatalf("events before the deletion: %s", feed.raw)
	}
	return f
}

func (f forgetCase) deliveries(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, it := range f.key.expect(http.StatusOK, "GET", "/v1/webhooks/"+f.hook+"/deliveries", nil).body["items"].([]any) {
		out = append(out, it.(map[string]any)["id"].(string))
	}
	return out
}

func (f forgetCase) checkForgotten(t *testing.T, deliveries []string, extra ...string) {
	t.Helper()
	feed := f.key.expect(http.StatusOK, "GET", "/v1/events?after=0&limit=500", nil)
	for _, s := range append([]string{f.secret, f.pending}, extra...) {
		if strings.Contains(string(feed.raw), s) {
			t.Fatalf("event feed still holds %q: %s", s, feed.raw)
		}
		if n := f.h.count("SELECT count(*) FROM webhook_deliveries WHERE workspace_id = $1 AND strpos(payload, $2) > 0", f.tm.ws, s); n != 0 {
			t.Fatalf("%d deliveries still hold %q", n, s)
		}
	}
	for _, d := range deliveries {
		f.key.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/webhooks/"+f.hook+"/deliveries/"+d, nil)
		f.key.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/webhooks/"+f.hook+"/deliveries/"+d+"/redeliver", nil)
	}
	for _, d := range deliveries {
		if retry, err := f.h.srv.DeliverWebhook(context.Background(), uuid.MustParse(f.tm.ws), uuid.MustParse(d), 2, 5, false); err != nil || retry {
			t.Fatalf("deliver a deleted delivery: %v %v", retry, err)
		}
	}
	if got := f.rcv.take(); len(got) != 0 {
		t.Fatalf("a delivery still went out after the deletion: %s", got[0].body)
	}
}

func TestDeletionForgetsEventsAndWebhookCopies(t *testing.T) {
	h := privateHarness(t)
	bystander := newForgetCase(t, h, "bystander")
	bystanderDeliveries := bystander.deliveries(t)

	t.Run("contact", func(t *testing.T) {
		f := newForgetCase(t, h, "contact")
		f.tm.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+f.tm.contact, map[string]any{"name": "Forgotten Person"})
		h.runWebhooks(f.tm.ws)
		if got := f.rcv.take(); len(got) != 1 || got[0].typ != "contact.updated" {
			t.Fatalf("contact.updated: %v", got)
		}
		ds := f.deliveries(t)
		if len(ds) != 3 {
			t.Fatalf("%d deliveries", len(ds))
		}
		bystander.tm.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/contacts/"+f.tm.contact, nil)
		f.tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+f.tm.contact, nil)
		f.checkForgotten(t, ds, "Forgotten Person")
	})

	t.Run("inbox", func(t *testing.T) {
		f := newForgetCase(t, h, "inbox")
		ds := f.deliveries(t)
		bystander.tm.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/inboxes/"+f.tm.inbox, nil)
		f.tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+f.tm.inbox, nil)
		f.checkForgotten(t, ds)
	})

	t.Run("retention", func(t *testing.T) {
		f := newForgetCase(t, h, "retention")
		ds := f.deliveries(t)
		f.tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+f.conv, map[string]any{"status": "closed"})
		f.tm.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"retention_days": 1})
		h.clock.Advance(25 * time.Hour)
		if err := h.srv.ApplyRetention(context.Background()); err != nil {
			t.Fatal(err)
		}
		f.tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+f.conv, nil)
		f.checkForgotten(t, ds)
	})

	t.Run("trash purge", func(t *testing.T) {
		f := newForgetCase(t, h, "trash")
		ds := f.deliveries(t)
		f.tm.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+f.conv, nil)
		f.tm.owner.expect(http.StatusOK, "GET", "/v1/webhooks/"+f.hook+"/deliveries/"+ds[0], nil)
		h.clock.Advance(31 * 24 * time.Hour)
		if err := h.srv.PurgeTrash(context.Background(), time.Minute); err != nil {
			t.Fatal(err)
		}
		f.key.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+f.conv, nil)
		f.checkForgotten(t, ds)
	})

	feed := bystander.key.expect(http.StatusOK, "GET", "/v1/events?after=0&limit=500", nil)
	if !strings.Contains(string(feed.raw), bystander.secret) {
		t.Fatalf("another workspace's events went: %s", feed.raw)
	}
	for _, d := range bystanderDeliveries {
		bystander.key.expect(http.StatusOK, "GET", "/v1/webhooks/"+bystander.hook+"/deliveries/"+d, nil)
	}
}

package api_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/store"
)

// The server and the job client here use River tables in a schema of their own, so the running
// client never takes another test's jobs.
func TestHangingWebhooksLeaveAssignmentPrompt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	schema := strings.ReplaceAll(unique("river"), "-", "_")
	var queued *store.Store
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Webhooks.AllowPrivate = true
		if _, err := d.Store.Pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = d.Store.Pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
		m, err := rivermigrate.New(riverpgxv5.New(d.Store.Pool), &rivermigrate.Config{Schema: schema, Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
			t.Fatal(err)
		}
		dsn := os.Getenv("YUVA_TEST_DATABASE_URL")
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		if queued, err = store.Open(ctx, dsn+sep+"search_path="+schema+",public"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(queued.Close)
		d.Store = queued
	})
	tm := newTeam(t, h)

	var hanging atomic.Int32
	stop := make(chan struct{})
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hanging.Add(1)
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	}))
	t.Cleanup(endpoint.Close)
	t.Cleanup(func() { close(stop) })
	for range 4 {
		tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": endpoint.URL, "events": []string{"conversation.created"}})
	}

	workers := river.NewWorkers()
	h.srv.AddWorkers(workers)
	client, err := river.NewClient(riverpgxv5.New(queued.Pool), &river.Config{
		Logger: slog.New(slog.DiscardHandler), Queues: jobs.Queues(2, 0, 0), Workers: workers,
		FetchCooldown: 10 * time.Millisecond, FetchPollInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = client.StopAndCancel(stopCtx)
	})

	for range 15 {
		tm.conversation(tm.owner)
	}
	deadline := time.Now().Add(15 * time.Second)
	for hanging.Load() < jobs.WebhookWorkers {
		if time.Now().After(deadline) {
			t.Fatalf("%d deliveries reached the endpoint", hanging.Load())
		}
		time.Sleep(20 * time.Millisecond)
	}

	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	h.online(tm.ws, tm.agentID)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"assignment_mode": "round_robin"})
	start := time.Now()
	conv := tm.conversation(tm.owner)
	for assigneeOf(tm.owner, conv) != tm.agentID {
		if time.Since(start) > 5*time.Second {
			t.Fatalf("not assigned %s after the conversation was created, with %d deliveries hanging", time.Since(start), hanging.Load())
		}
		time.Sleep(20 * time.Millisecond)
	}
	var held int
	if err := queued.Pool.QueryRow(ctx, "SELECT count(*) FROM river_job WHERE kind = 'webhook_delivery' AND state IN ('available', 'running') AND queue = $1", jobs.WebhookQueue).Scan(&held); err != nil {
		t.Fatal(err)
	}
	if held <= jobs.WebhookWorkers {
		t.Fatalf("only %d deliveries were still waiting or running", held)
	}
}

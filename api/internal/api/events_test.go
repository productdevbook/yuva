package api

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

func TestEventsCommitWithTheChange(t *testing.T) {
	dsn := os.Getenv("YUVA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("YUVA_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.MigrateUp(ctx, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	res, err := Bootstrap(ctx, st, BootstrapInput{Email: "tx-" + randomToken(6) + "@example.com", Workspace: "tx", AllowExisting: true})
	if err != nil {
		t.Fatal(err)
	}
	ws := res.WorkspaceID
	listener, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close(ctx)
	if _, err := listener.Exec(ctx, "LISTEN "+realtime.Channel); err != nil {
		t.Fatal(err)
	}
	s := New(Deps{Log: slog.New(slog.DiscardHandler), Store: st})
	count := func() int {
		var n int
		if err := st.Pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE workspace_id = $1", ws).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	notified := func() string {
		wctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		for {
			n, err := listener.WaitForNotification(wctx)
			if err != nil {
				return ""
			}
			if len(n.Payload) > len(ws.String()) && n.Payload[:len(ws.String())] == ws.String() {
				return n.Payload
			}
		}
	}

	boom := errors.New("boom")
	err = st.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.CreateLabel(ctx, store.CreateLabelParams{ID: newID(), WorkspaceID: ws, Name: "rolled back", Color: "#ff0000", Now: time.Now()}); err != nil {
			return err
		}
		if _, err := writeEvents(ctx, q, ws, []pendingEvent{{typ: realtime.ContactUpdated, data: []byte(`{}`)}}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("transaction error %v", err)
	}
	if n := count(); n != 0 {
		t.Fatalf("%d events after a rolled-back change", n)
	}
	if p := notified(); p != "" {
		t.Fatalf("notification %q after a rolled-back change", p)
	}

	err = s.inTx(ctx, ws, func(q *store.Queries, ev *eventBatch) error {
		ev.add(realtime.ContactUpdated, nil, nil, map[string]string{"id": "x"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 1 {
		t.Fatalf("%d events after a committed change, want 1", n)
	}
	if p := notified(); p == "" {
		t.Fatal("no notification after commit")
	}
}

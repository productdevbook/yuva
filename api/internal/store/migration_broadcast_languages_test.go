package store

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBroadcastLanguagesMigrationKeepsClicks(t *testing.T) {
	dsn := os.Getenv("YUVA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("YUVA_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := "yuva_mig_" + uuid.New().String()[:8]
	if _, err := admin.Exec(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, `DROP DATABASE "`+name+`" WITH (FORCE)`) })

	pcfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pcfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	st := &Store{Pool: pool, Queries: New(pool)}
	p, err := st.migrations(slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err := p.UpTo(ctx, 141); err != nil {
		t.Fatal(err)
	}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	ws, inbox, contact, broadcast, delivery := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO workspaces (id, name) VALUES ($1, 'W')`, ws)
	exec(`INSERT INTO inboxes (id, workspace_id, name, slug, identity_secret, created_at, updated_at) VALUES ($1, $2, 'I', $3, '\x00', now(), now())`, inbox, ws, uuid.New().String())
	exec(`INSERT INTO contacts (id, workspace_id, name, attributes, created_at, updated_at) VALUES ($1, $2, '', '{}', now(), now())`, contact, ws)
	exec(`INSERT INTO broadcasts (id, workspace_id, inbox_id, name, state, click_tracking, created_at, updated_at) VALUES ($1, $2, $3, 'Old', 'sending', true, now(), now())`, broadcast, ws, inbox)
	exec(`INSERT INTO broadcast_links (workspace_id, broadcast_id, position, url) VALUES ($1, $2, 0, 'https://example.com/a'), ($1, $2, 1, 'https://example.com/b')`, ws, broadcast)
	exec(`INSERT INTO broadcast_deliveries (id, workspace_id, broadcast_id, contact_id, email, email_token, header_message_id, created_at)
		VALUES ($1, $2, $3, $4, 'success@simulator.amazonses.com', 'abcdefghijklmnopqrstuvwx', '<old@example.com>', now())`, delivery, ws, broadcast, contact)

	if _, err := p.UpTo(ctx, 142); err != nil {
		t.Fatal(err)
	}
	d, err := st.GetBroadcastDelivery(ctx, GetBroadcastDeliveryParams{WorkspaceID: ws, ID: delivery})
	if err != nil {
		t.Fatal(err)
	}
	if d.Locale != nil {
		t.Fatalf("an old delivery has a version: %s", *d.Locale)
	}
	link, err := st.GetBroadcastLink(ctx, GetBroadcastLinkParams{WorkspaceID: ws, BroadcastID: broadcast, Locale: "", Position: 1})
	if err != nil || link.Url != "https://example.com/b" {
		t.Fatalf("the old link no longer resolves: %v %v", link, err)
	}
	if err := st.CountBroadcastLinkClick(ctx, CountBroadcastLinkClickParams{WorkspaceID: ws, BroadcastID: broadcast, Locale: link.Locale, Position: link.Position}); err != nil {
		t.Fatal(err)
	}
	links, err := st.ListBroadcastLinks(ctx, ListBroadcastLinksParams{WorkspaceID: ws, BroadcastID: broadcast})
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links[0].Url != "https://example.com/b" || links[0].Clicks != 1 || links[1].Clicks != 0 {
		t.Fatalf("links after a click: %+v", links)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO broadcast_links (workspace_id, broadcast_id, locale, position, url) VALUES ($1, $2, 'es', 1, 'https://example.com/b')`, ws, broadcast); err != nil {
		t.Fatalf("a version's link beside the old one: %v", err)
	}
	if _, err := p.DownTo(ctx, 141); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM broadcast_links WHERE broadcast_id = $1`, broadcast).Scan(&n); err != nil || n != 2 {
		t.Fatalf("links after down: %d %v", n, err)
	}
}

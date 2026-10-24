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

func TestWorkspaceFocusMigration(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 130); err != nil {
		t.Fatal(err)
	}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	workspace := func(purposes ...string) uuid.UUID {
		ws := uuid.New()
		exec(`INSERT INTO workspaces (id, name) VALUES ($1, 'W')`, ws)
		for _, purpose := range purposes {
			exec(`INSERT INTO inboxes (id, workspace_id, name, slug, identity_secret, created_at, updated_at, purpose) VALUES ($1, $2, 'I', $3, '\x00', now(), now(), $4)`,
				uuid.New(), ws, uuid.New().String(), purpose)
		}
		return ws
	}
	empty, support, mail, mixed := workspace(), workspace("support"), workspace("mail"), workspace("support", "mail")

	if _, err := p.UpTo(ctx, 131); err != nil {
		t.Fatal(err)
	}
	for ws, want := range map[uuid.UUID]string{empty: "support", support: "support", mail: "both", mixed: "both"} {
		var got *string
		if err := pool.QueryRow(ctx, `SELECT focus FROM workspaces WHERE id = $1`, ws).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got == nil || *got != want {
			t.Errorf("workspace %s: focus %v, want %s", ws, got, want)
		}
	}
	fresh := workspace("mail")
	var got *string
	if err := pool.QueryRow(ctx, `SELECT focus FROM workspaces WHERE id = $1`, fresh).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("a workspace made after the migration has focus %s", *got)
	}
	if _, err := pool.Exec(ctx, `UPDATE workspaces SET focus = 'sales' WHERE id = $1`, fresh); err == nil {
		t.Fatal("focus accepts an unknown value")
	}
}

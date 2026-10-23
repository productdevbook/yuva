package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

type migrationRows struct {
	h  *harness
	ws string
}

func (r migrationRows) seed(t *testing.T, id, state, email string) {
	t.Helper()
	ctx := context.Background()
	meta := `{"id":"t1","subject":"Refund for Ayse","requester":{"id":"u1","name":"Ayse Yilmaz","email":"` + email + `"},"from":{"name":"Ayse Yilmaz","email":"` + email + `"}}`
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"UPDATE migrations SET state = $3, live_from = now(), verified_at = now() WHERE workspace_id = $1 AND id = $2", []any{r.ws, id, state}},
		{"INSERT INTO migration_threads (workspace_id, migration_id, thread_key, last_at, items, done, plan) VALUES ($1, $2, 'zendesk:t1', now(), 2, 0, $3::jsonb)",
			[]any{r.ws, id, `{"contact":{"name":"Ayse Yilmaz","email":"` + email + `"},"subject":"Refund for Ayse"}`}},
		{"INSERT INTO migration_items (workspace_id, migration_id, source_id, thread_key, dedupe_key, received_at, size_bytes, state, in_copy, meta) VALUES ($1, $2, 'zendesk:t1', 'zendesk:t1', 'd1', now(), 10, 'planned', true, $3::jsonb)",
			[]any{r.ws, id, meta}},
		{"INSERT INTO migration_items (workspace_id, migration_id, source_id, thread_key, dedupe_key, received_at, size_bytes, state, in_copy, meta) VALUES ($1, $2, 'zendesk:t2', 'zendesk:t2', 'd2', now(), 10, 'imported', true, $3::jsonb)",
			[]any{r.ws, id, `{"id":"t2","subject":"Other","requester":{"id":"u2","name":"Mehmet","email":"someone.else@example.com"}}`}},
		{"INSERT INTO migration_plan_names (workspace_id, migration_id, email, name) VALUES ($1, $2, $3, 'Ayse Yilmaz')", []any{r.ws, id, email}},
		{"INSERT INTO migration_errors (workspace_id, migration_id, code, item_subject, created_at) VALUES ($1, $2, 'gone', 'Refund for Ayse', now())", []any{r.ws, id}},
	} {
		if _, err := r.h.st.Pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
}

func (r migrationRows) personal(id string) int {
	return r.h.count(`SELECT (SELECT count(*) FROM migration_items WHERE workspace_id = $1 AND migration_id = $2 AND meta <> '{}')
		+ (SELECT count(*) FROM migration_threads WHERE workspace_id = $1 AND migration_id = $2 AND plan <> '{}')
		+ (SELECT count(*) FROM migration_plan_names WHERE workspace_id = $1 AND migration_id = $2)
		+ (SELECT count(*) FROM migration_errors WHERE workspace_id = $1 AND migration_id = $2 AND item_subject <> '')`, r.ws, id)
}

func TestMigrationRowsForgetPeopleWhenItEnds(t *testing.T) {
	h := newMigrationHarness(t)
	mt := newMigrationTeam(t, h)
	rows := migrationRows{h: h, ws: mt.ws}
	const email = "ayse@example.com"

	done := mt.create(map[string]any{"preset": "imap", "source_address": "support@example.com"})
	rows.seed(t, done, "syncing", email)
	if rows.personal(done) == 0 {
		t.Fatal("nothing seeded")
	}
	mt.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+done+"/finish", map[string]any{"force": true})
	if n := rows.personal(done); n != 0 {
		t.Fatalf("personal rows after finishing: %d", n)
	}
	if n := h.count("SELECT count(*) FROM migration_items WHERE workspace_id = $1 AND migration_id = $2", mt.ws, done); n != 2 {
		t.Fatalf("item rows kept for the counts: %d", n)
	}

	cancelled := mt.create(map[string]any{"preset": "imap", "source_address": "support2@example.com"})
	rows.seed(t, cancelled, "copying", email)
	mt.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+cancelled+"/cancel", nil)
	if n := rows.personal(cancelled); n != 0 {
		t.Fatalf("personal rows after cancelling: %d", n)
	}

	other := newMigrationTeam(t, h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/migrations/"+cancelled+"/cancel", nil)
}

func TestContactDeletionClearsRunningMigrationRows(t *testing.T) {
	h := newMigrationHarness(t)
	mt := newMigrationTeam(t, h)
	rows := migrationRows{h: h, ws: mt.ws}
	var email string
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT email FROM contact_emails WHERE workspace_id = $1 AND contact_id = $2", mt.ws, mt.contact).Scan(&email); err != nil {
		t.Fatal(err)
	}
	running := mt.create(map[string]any{"preset": "imap", "source_address": "support@example.com"})
	rows.seed(t, running, "copying", strings.ToLower(email))
	if _, err := h.st.Pool.Exec(context.Background(), `INSERT INTO migration_items (workspace_id, migration_id, source_id, thread_key, dedupe_key, received_at, size_bytes, state, in_copy, meta)
		VALUES ($1, $2, 'zendesk:t3', 'zendesk:t1', 'd3', now(), 10, 'failed', true, jsonb_build_object('to', jsonb_build_array(jsonb_build_object('email', upper($3::text)))))`, mt.ws, running, email); err != nil {
		t.Fatal(err)
	}
	lookalike := mt.create(map[string]any{"preset": "imap", "source_address": "support3@example.com"})
	rows.seed(t, lookalike, "copying", "x"+strings.ToLower(email))
	longer := mt.create(map[string]any{"preset": "imap", "source_address": "support4@example.com"})
	rows.seed(t, longer, "copying", strings.ToLower(email)+".tr")
	elsewhere := newMigrationTeam(t, h)
	elsewhereID := elsewhere.create(map[string]any{"preset": "imap", "source_address": "support@example.com"})
	migrationRows{h: h, ws: elsewhere.ws}.seed(t, elsewhereID, "copying", strings.ToLower(email))

	mt.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+mt.contact, nil)
	if n := h.count(`SELECT count(*) FROM migration_items WHERE workspace_id = $1 AND migration_id = $2 AND meta::text LIKE '%Ayse%'`, mt.ws, running); n != 0 {
		t.Fatalf("items still name the contact: %d", n)
	}
	if n := h.count("SELECT count(*) FROM migration_plan_names WHERE workspace_id = $1 AND email = $2", mt.ws, strings.ToLower(email)); n != 0 {
		t.Fatalf("plan names kept: %d", n)
	}
	if n := h.count("SELECT count(*) FROM migration_threads WHERE workspace_id = $1 AND migration_id = $2 AND plan <> '{}'", mt.ws, running); n != 0 {
		t.Fatalf("thread plans kept: %d", n)
	}
	if n := h.count("SELECT count(*) FROM migration_items WHERE workspace_id = $1 AND migration_id = $2 AND state = 'skipped' AND reason = 'contact_deleted'", mt.ws, running); n != 2 {
		t.Fatalf("planned item of the contact not skipped: %d", n)
	}
	if n := h.count("SELECT count(*) FROM migration_items WHERE workspace_id = $1 AND migration_id = $2 AND source_id = 'zendesk:t3' AND state = 'skipped'", mt.ws, running); n != 1 {
		t.Fatalf("failed item of the contact left to retry: %d", n)
	}
	for _, id := range []string{lookalike, longer} {
		if n := rows.personal(id); n != 5 {
			t.Fatalf("a similar address was cleared: %d", n)
		}
		if n := h.count("SELECT count(*) FROM migration_items WHERE workspace_id = $1 AND migration_id = $2 AND state = 'planned'", mt.ws, id); n != 1 {
			t.Fatalf("a similar address's item was skipped: %d", n)
		}
	}
	if n := h.count("SELECT done FROM migration_threads WHERE workspace_id = $1 AND migration_id = $2", mt.ws, running); n != 1 {
		t.Fatalf("thread done %d", n)
	}
	if n := h.count("SELECT count(*) FROM migration_items WHERE workspace_id = $1 AND migration_id = $2 AND meta <> '{}'", mt.ws, running); n != 1 {
		t.Fatalf("another person's item was cleared: %d", n)
	}
	if n := rows.personal(running); n == 0 {
		t.Fatal("the error subject and the other person's rows should stay while it runs")
	}
	if n := (migrationRows{h: h, ws: elsewhere.ws}).personal(elsewhereID); n != 5 {
		t.Fatalf("another workspace's rows touched: %d", n)
	}
}

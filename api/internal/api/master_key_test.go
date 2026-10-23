package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
	"slices"
	"testing"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/productdevbook/yuva/api/internal/secret"
	"github.com/productdevbook/yuva/api/internal/store"
)

func freshStore(t *testing.T) *store.Store {
	t.Helper()
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
	t.Cleanup(func() { admin.Close(ctx) })
	name := "yuva_mk_" + uuid.New().String()[:8]
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
	t.Cleanup(pool.Close)
	st := &store.Store{Pool: pool, Queries: store.New(pool)}
	if err := st.MigrateUp(ctx, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	return st
}

func randomMasterKey() string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	return base64.StdEncoding.EncodeToString(raw)
}

type sealedFixture struct {
	column string
	read   string
	ws, id uuid.UUID
	ctx    []byte
	plain  string
}

func TestMasterKeyRotation(t *testing.T) {
	st := freshStore(t)
	ctx := context.Background()
	keyA, keyB, keyC := randomMasterKey(), randomMasterKey(), randomMasterKey()
	serverWith := func(cur, prev string) *Server {
		t.Helper()
		k, err := secret.ParseKeys(cur, prev)
		if err != nil {
			t.Fatal(err)
		}
		return New(Deps{Log: slog.New(slog.DiscardHandler), Store: st, Secrets: k})
	}
	onlyA := serverWith(keyA, "")
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := st.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	workspace := func() uuid.UUID {
		t.Helper()
		res, err := Bootstrap(ctx, st, BootstrapInput{Email: "mk-" + randomToken(6) + "@example.com", Workspace: "mk", AllowExisting: true})
		if err != nil {
			t.Fatal(err)
		}
		return res.WorkspaceID
	}
	ws, legacyWS := workspace(), workspace()

	var fixtures []sealedFixture
	seal := func(column, read string, ws, id uuid.UUID, context []byte) []byte {
		plain := "secret of " + column + " " + randomToken(4)
		fixtures = append(fixtures, sealedFixture{column: column, read: read, ws: ws, id: id, ctx: context, plain: plain})
		return onlyA.secrets.Seal([]byte(plain), context)
	}
	inbox, email, app, hook := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	page, component, monitor, migration, property, center, imp, sso, issue := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()

	exec(`INSERT INTO inboxes (id, workspace_id, name, slug, identity_secret, created_at, updated_at) VALUES ($1, $2, 'I', 'i', $3, now(), now())`,
		inbox, ws, seal("inboxes.identity_secret", `SELECT identity_secret FROM inboxes WHERE workspace_id = $1 AND id = $2`, ws, inbox, identitySecretContext(ws, inbox)))
	exec(`INSERT INTO channels (id, workspace_id, inbox_id, kind, name, created_at, updated_at) VALUES ($1, $3, $4, 'email', 'E', now(), now()), ($2, $3, $4, 'app', 'A', now(), now())`,
		email, app, ws, inbox)
	exec(`INSERT INTO email_channels (workspace_id, channel_id, address, smtp_password, broadcast_smtp_password, dkim_private_key) VALUES ($1, $2, 'help@example.com', $3, $4, $5)`,
		ws, email,
		seal("email_channels.smtp_password", `SELECT smtp_password FROM email_channels WHERE workspace_id = $1 AND channel_id = $2`, ws, email, smtpSecretContext(ws, email)),
		seal("email_channels.broadcast_smtp_password", `SELECT broadcast_smtp_password FROM email_channels WHERE workspace_id = $1 AND channel_id = $2`, ws, email, broadcastSMTPSecretContext(ws, email)),
		seal("email_channels.dkim_private_key", `SELECT dkim_private_key FROM email_channels WHERE workspace_id = $1 AND channel_id = $2`, ws, email, dkimSecretContext(ws, email)))
	exec(`INSERT INTO app_push_settings (workspace_id, channel_id, updated_at, apns_key, apns_key_id, apns_team_id, apns_bundle_id, fcm_credentials, fcm_project_id)
		VALUES ($1, $2, now(), $3, 'ABCDEFGHIJ', 'ABCDEFGHIJ', 'com.example.app', $4, 'example')`, ws, app,
		seal("app_push_settings.apns_key", `SELECT apns_key FROM app_push_settings WHERE workspace_id = $1 AND channel_id = $2`, ws, app, apnsSecretContext(ws, app)),
		seal("app_push_settings.fcm_credentials", `SELECT fcm_credentials FROM app_push_settings WHERE workspace_id = $1 AND channel_id = $2`, ws, app, fcmSecretContext(ws, app)))
	exec(`INSERT INTO webhook_endpoints (id, workspace_id, url, events, secret, previous_secret, created_at, updated_at)
		VALUES ($1, $2, 'https://example.com/hook', '{conversation.created}', $3, $4, now(), now())`, hook, ws,
		seal("webhook_endpoints.secret", `SELECT secret FROM webhook_endpoints WHERE workspace_id = $1 AND id = $2`, ws, hook, webhookSecretContext(ws, hook)),
		seal("webhook_endpoints.previous_secret", `SELECT previous_secret FROM webhook_endpoints WHERE workspace_id = $1 AND id = $2`, ws, hook, webhookSecretContext(ws, hook)))
	exec(`INSERT INTO status_pages (id, workspace_id, inbox_id, name, slug, locale, timezone, changed_at, created_at, updated_at) VALUES ($1, $2, $3, 'P', 'mk-page', 'en', 'UTC', now(), now(), now())`, page, ws, inbox)
	exec(`INSERT INTO status_components (id, workspace_id, page_id, name, state, created_at, updated_at) VALUES ($1, $2, $3, 'C', 'operational', now(), now())`, component, ws, page)
	exec(`INSERT INTO status_monitors (id, workspace_id, page_id, component_id, kind, name, failure_state, failure_threshold, recovery_threshold, on_failure, state,
		target, interval_seconds, timeout_seconds, http_method, status_min, status_max, cert_warn_days, http_headers, changed_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'http', 'M', 'major_outage', 1, 1, 'none', 'pending', 'https://example.com', 60, 10, 'GET', 200, 299, 14, $5, now(), now(), now())`,
		monitor, ws, page, component,
		seal("status_monitors.http_headers", `SELECT http_headers FROM status_monitors WHERE workspace_id = $1 AND id = $2`, ws, monitor, headerSecretContext(ws, monitor)))
	exec(`INSERT INTO migrations (id, workspace_id, source, created_at, updated_at) VALUES ($1, $2, 'imap', now(), now())`, migration, ws)
	exec(`INSERT INTO migration_credentials (workspace_id, migration_id, kind, sealed, host, port, tls, created_at, updated_at) VALUES ($1, $2, 'password', $3, 'imap.example.com', 993, 'tls', now(), now())`,
		ws, migration, seal("migration_credentials.sealed", `SELECT sealed FROM migration_credentials WHERE workspace_id = $1 AND migration_id = $2`, ws, migration, migrationSecretContext(ws, migration)))
	exec(`INSERT INTO migration_sign_ins (workspace_id, migration_id, state_hash, sealed, host, expires_at, created_at) VALUES ($1, $2, repeat('a', 64), $3, 'login.example.com', now() + interval '1 hour', now())`,
		ws, migration, seal("migration_sign_ins.sealed", `SELECT sealed FROM migration_sign_ins WHERE workspace_id = $1 AND migration_id = $2`, ws, migration, migrationSecretContext(ws, migration)))
	exec(`INSERT INTO analytics_properties (id, workspace_id, name, public_key, domains, visitor_key, created_at, updated_at) VALUES ($1, $2, 'Site', $3, '{example.com}', $4, now(), now())`,
		property, ws, "yuva_pk_"+randomToken(8),
		seal("analytics_properties.visitor_key", `SELECT visitor_key FROM analytics_properties WHERE workspace_id = $1 AND id = $2`, ws, property, analyticsSecretContext(ws, property)))
	exec(`INSERT INTO help_centers (id, workspace_id, inbox_id, name, slug, default_locale, locales, changed_at, created_at, updated_at) VALUES ($1, $2, $3, 'H', 'mk-help', 'en', '{en}', now(), now(), now())`, center, ws, inbox)
	exec(`INSERT INTO help_imports (id, workspace_id, center_id, source, state, sealed, created_at, updated_at) VALUES ($1, $2, $3, 'helpscout', 'queued', $4, now(), now())`,
		imp, ws, center, seal("help_imports.sealed", `SELECT sealed FROM help_imports WHERE workspace_id = $1 AND id = $2`, ws, imp, helpImportSecretContext(ws, imp)))
	exec(`INSERT INTO slack_installations (workspace_id, team_id, team_name, app_id, bot_user_id, bot_token, scopes, created_at, updated_at) VALUES ($1, 'T1', 'Team', 'A1', 'U1', $2, '{chat:write}', now(), now())`,
		ws, seal("slack_installations.bot_token", `SELECT bot_token FROM slack_installations WHERE workspace_id = $1 AND $2::uuid IS NOT NULL`, ws, ws, slackTokenContext(ws)))
	exec(`INSERT INTO sso_settings (workspace_id, id, protocol, oidc_issuer, oidc_client_id, oidc_client_secret, created_at, updated_at) VALUES ($1, $2, 'oidc', 'https://id.example.com', 'yuva', $3, now(), now())`,
		ws, sso, seal("sso_settings.oidc_client_secret", `SELECT oidc_client_secret FROM sso_settings WHERE workspace_id = $1 AND id = $2`, ws, sso, ssoSecretContext(ws, sso)))
	exec(`INSERT INTO issue_integrations (id, workspace_id, provider, name, account, token, webhook_secret, created_at, updated_at) VALUES ($1, $2, 'github', 'GitHub', 'example', $3, $4, now(), now())`,
		issue, ws,
		seal("issue_integrations.token", `SELECT token FROM issue_integrations WHERE workspace_id = $1 AND id = $2`, ws, issue, issueTokenContext(ws, issue)),
		seal("issue_integrations.webhook_secret", `SELECT webhook_secret FROM issue_integrations WHERE workspace_id = $1 AND id = $2`, ws, issue, issueSecretContext(ws, issue)))

	if err := onlyA.rememberOptOuts(ctx, st.Queries, ws, []string{"Gone@example.com"}, optOutErasure); err != nil {
		t.Fatal(err)
	}
	legacyKey, err := secret.ParseKey(keyA)
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO email_optouts (workspace_id, email_hash, reason, created_at) VALUES ($1, $2, $3, now())`,
		legacyWS, optOutHash(legacyKey.Derive(optOutKeyInfo+legacyWS.String()), "legacy@example.com"), optOutErasure)
	codeHash := verifyCodeHash(onlyA.secrets.Derive(verifyCodeInfo), email, "123456")

	registered := map[string]bool{}
	for _, c := range sealedColumns {
		registered[c.Table+"."+c.Column] = true
	}
	covered := map[string]bool{"workspace_keys.optout_key": true}
	for _, f := range fixtures {
		covered[f.column] = true
	}
	for c := range registered {
		if !covered[c] {
			t.Errorf("no fixture for sealed column %s", c)
		}
	}
	notSealed := []string{
		"analytics_salts.salt", "analytics_shares.token_hash", "api_keys.secret_hash", "chat_visitors.visitor_hash",
		"contact_sessions.token_hash", "email_channels.dkim_public_key", "email_channels.verify_code_hash",
		"email_confirmations.token_hash", "email_images.sha256", "email_optouts.email_hash", "help_import_items.content_hash",
		"idempotency_keys.body", "idempotency_keys.body_sha256", "login_codes.code_hash", "oauth_codes.code_hash",
		"oauth_tokens.token_hash", "passkeys.credential_id", "people.webauthn_handle", "rate_limits.key", "rate_slots.key",
		"reader_tokens.token_hash", "saml_assertions.id_hash", "sessions.token_hash", "slack_link_codes.code_hash",
		"sso_logins.state_hash", "sso_settings.saml_certificates", "status_monitors.token_hash", "webauthn_ceremonies.id_hash",
	}
	rows, err := st.Pool.Query(ctx, `SELECT table_name || '.' || column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND udt_name IN ('bytea', '_bytea') AND table_name NOT LIKE 'river\_%'`)
	if err != nil {
		t.Fatal(err)
	}
	columns, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range columns {
		if !registered[c] && !slices.Contains(notSealed, c) {
			t.Errorf("bytea column %s is neither in sealedColumns nor known to hold no sealed value", c)
		}
	}

	readsWith := func(s *Server) {
		t.Helper()
		for _, f := range fixtures {
			var sealed []byte
			if err := st.Pool.QueryRow(ctx, f.read, f.ws, f.id).Scan(&sealed); err != nil {
				t.Fatalf("%s: %v", f.column, err)
			}
			plain, err := s.secrets.Open(sealed, f.ctx)
			if err != nil || string(plain) != f.plain {
				t.Errorf("%s: %q %v", f.column, plain, err)
			}
		}
		for w, addr := range map[uuid.UUID]string{ws: "gone@example.com", legacyWS: "LEGACY@example.com"} {
			if opted, err := s.optedOut(ctx, st.Queries, w, addr); err != nil || !opted {
				t.Errorf("opt-out of %s: %v %v", addr, opted, err)
			}
		}
	}
	report := func(rep MasterKeyReport, err error) map[string]SealedReport {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]SealedReport{}
		for _, c := range rep.Columns {
			out[c.Column] = c
		}
		return out
	}

	rotating := serverWith(keyB, keyA)
	readsWith(rotating)
	if !rotating.anyDerived(verifyCodeInfo, func(k []byte) bool { return slices.Equal(codeHash, verifyCodeHash(k, email, "123456")) }) {
		t.Error("an address code sent before the rotation is refused during it")
	}
	if v := rotating.emailProofValues(ws, "example.com"); len(v) != 2 || v[1] != onlyA.emailProofValues(ws, "example.com")[0] || v[0] == v[1] {
		t.Errorf("proof values during the rotation: %v", v)
	}
	rep, err := rotating.CheckMasterKey(ctx, 1, nil)
	if rep.UnkeptOptOutKeys != 1 || rep.Clean() {
		t.Errorf("check before rotating: %d unkept opt-out keys, clean %v", rep.UnkeptOptOutKeys, rep.Clean())
	}
	for name, c := range report(rep, err) {
		if c.Previous != 1 || c.Current != 0 || len(c.Unreadable) != 0 {
			t.Errorf("check before rotating, %s: %+v", name, c)
		}
	}

	for name, c := range report(rotating.RotateMasterKey(ctx, 1, nil)) {
		want := SealedReport{Column: name, Previous: 1}
		if name == "workspace_keys.optout_key" {
			want.Current = 1
		}
		if c.Previous != want.Previous || c.Current != want.Current || len(c.Unreadable) != 0 {
			t.Errorf("rotate, %s: %+v", name, c)
		}
	}
	rep, err = rotating.CheckMasterKey(ctx, 500, nil)
	if !rep.Clean() {
		t.Errorf("check after rotating: %+v", rep)
	}
	for name, c := range report(rep, err) {
		if c.Current == 0 {
			t.Errorf("check after rotating, %s: %+v", name, c)
		}
	}

	onlyB := serverWith(keyB, "")
	readsWith(onlyB)
	rep, err = onlyB.CheckMasterKey(ctx, 500, nil)
	if err != nil || !rep.Clean() {
		t.Errorf("check with the new key alone: %+v %v", rep, err)
	}
	if onlyB.anyDerived(verifyCodeInfo, func(k []byte) bool { return slices.Equal(codeHash, verifyCodeHash(k, email, "123456")) }) {
		t.Error("an address code made under the removed key still checks out")
	}
	f := fixtures[0]
	var sealed []byte
	if err := st.Pool.QueryRow(ctx, f.read, f.ws, f.id).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if _, err := onlyA.secrets.Open(sealed, f.ctx); err == nil {
		t.Error("a re-sealed value still opens with the old key")
	}

	stray, err := secret.ParseKey(keyC)
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE webhook_endpoints SET secret = $3 WHERE workspace_id = $1 AND id = $2`, ws, hook, stray.Seal([]byte("lost"), webhookSecretContext(ws, hook)))
	rep, err = onlyB.CheckMasterKey(ctx, 500, nil)
	if c := report(rep, err)["webhook_endpoints.secret"]; rep.Clean() || len(c.Unreadable) != 1 || c.Unreadable[0] != "workspace "+ws.String()+" row "+hook.String() {
		t.Errorf("check with a value under neither key: %+v", c)
	}
	if _, err := onlyB.RotateMasterKey(ctx, 500, nil); err == nil {
		t.Error("rotate without a previous key")
	}
	exec(`UPDATE workspace_keys SET optout_key = $2 WHERE workspace_id = $1`, ws, stray.Seal([]byte("lost"), optOutKeyContext(ws)))
	rep, err = serverWith(randomMasterKey(), keyB).RotateMasterKey(ctx, 500, nil)
	if c := report(rep, err)["workspace_keys.optout_key"]; len(c.Unreadable) != 1 || c.Previous != 1 {
		t.Errorf("rotate with an opt-out key under neither key: %+v", c)
	}

	fresh := workspace()
	rolledBack := errors.New("roll back")
	if err := st.InTx(ctx, func(q *store.Queries) error {
		if err := onlyB.rememberOptOuts(ctx, q, fresh, []string{"first@example.com"}, optOutErasure); err != nil {
			return err
		}
		return rolledBack
	}); err != rolledBack {
		t.Fatal(err)
	}
	if err := onlyB.rememberOptOuts(ctx, st.Queries, fresh, []string{"second@example.com"}, optOutErasure); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetOptOutKey(ctx, fresh); err != nil {
		t.Errorf("opt-out key after a rolled back first opt-out: %v", err)
	}
}

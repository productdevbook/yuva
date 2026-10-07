package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("YUVA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("YUVA_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := migrateUp(ctx, st, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	return st
}

func uniqueName(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

func bootstrapWorkspace(t *testing.T, st *store.Store, name string) api.BootstrapResult {
	t.Helper()
	res, err := api.Bootstrap(context.Background(), st, api.BootstrapInput{
		Email: uniqueName("cli") + "@example.com", Workspace: name, AllowExisting: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func runAPIKey(t *testing.T, st *store.Store, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := apiKey(context.Background(), st, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func workspaceStatus(t *testing.T, url, secret string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url+"/v1/workspace", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	return res.StatusCode
}

func TestAPIKeyCommand(t *testing.T) {
	st := testStore(t)
	log := slog.New(slog.DiscardHandler)
	ts := httptest.NewServer(api.New(api.Deps{Log: log, Store: st, Version: "test", Mailer: mail.Log{Log: log}}).Handler())
	t.Cleanup(ts.Close)

	name := uniqueName("cli-ws")
	ws := bootstrapWorkspace(t, st, name)
	other := bootstrapWorkspace(t, st, uniqueName("cli-other"))

	stdout, _, err := runAPIKey(t, st, "create", "--workspace", name, "--name", "backend")
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.TrimSuffix(stdout, "\n")
	if secret == "" || strings.ContainsAny(secret, " \t\n") {
		t.Fatalf("create must print only the secret, got %q", stdout)
	}
	if got := workspaceStatus(t, ts.URL, secret); got != http.StatusOK {
		t.Fatalf("new key: status %d", got)
	}

	byID, _, err := runAPIKey(t, st, "create", "--workspace", ws.WorkspaceID.String(), "--name", "by id")
	if err != nil || strings.TrimSpace(byID) == secret {
		t.Fatalf("create by id: %q %v", byID, err)
	}
	if _, _, err := runAPIKey(t, st, "create", "--workspace", name, "--name", "  "); err == nil {
		t.Fatal("blank name accepted")
	}

	list, _, err := runAPIKey(t, st, "list", "--workspace", name)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(list, secret) || strings.Contains(list, strings.TrimSpace(byID)) {
		t.Fatal("listing contains a secret")
	}
	keys, err := st.ListAPIKeys(context.Background(), ws.WorkspaceID)
	if err != nil || len(keys) != 2 {
		t.Fatalf("keys: %v %v", keys, err)
	}
	for _, k := range keys {
		if !strings.Contains(list, k.ID.String()) || !strings.Contains(list, k.Prefix) {
			t.Fatalf("listing misses %s:\n%s", k.ID, list)
		}
		if k.WorkspaceID != ws.WorkspaceID || k.CreatedBy != nil {
			t.Fatalf("key %s: workspace %s, created by %v", k.ID, k.WorkspaceID, k.CreatedBy)
		}
	}
	otherList, _, err := runAPIKey(t, st, "list", "--workspace", other.WorkspaceID.String())
	if err != nil || strings.Contains(otherList, keys[0].ID.String()) {
		t.Fatalf("other workspace listing: %q %v", otherList, err)
	}

	revoked := keys[0]
	if !strings.HasPrefix(secret, revoked.Prefix+"_") {
		revoked = keys[1]
	}
	if out, _, err := runAPIKey(t, st, "revoke", revoked.ID.String()); err != nil || out != "" {
		t.Fatalf("revoke: %q %v", out, err)
	}
	if got := workspaceStatus(t, ts.URL, secret); got != http.StatusUnauthorized {
		t.Fatalf("revoked key: status %d", got)
	}
	if got := workspaceStatus(t, ts.URL, strings.TrimSpace(byID)); got != http.StatusOK {
		t.Fatalf("other key after revoke: status %d", got)
	}
	if _, _, err := runAPIKey(t, st, "revoke", revoked.ID.String()); err != nil {
		t.Fatalf("revoking twice: %v", err)
	}
	if _, _, err := runAPIKey(t, st, "revoke", "not-an-id"); err == nil {
		t.Fatal("bad id accepted")
	}
	if _, _, err := runAPIKey(t, st, "revoke", other.MemberID.String()); err == nil {
		t.Fatal("unknown id accepted")
	}
}

func TestAPIKeyCommandRefusesAmbiguousName(t *testing.T) {
	st := testStore(t)
	name := uniqueName("cli-twin")
	a := bootstrapWorkspace(t, st, name)
	b := bootstrapWorkspace(t, st, name)

	stdout, _, err := runAPIKey(t, st, "create", "--workspace", name, "--name", "backend")
	if err == nil || stdout != "" {
		t.Fatalf("ambiguous name: %q %v", stdout, err)
	}
	if !strings.Contains(err.Error(), a.WorkspaceID.String()) || !strings.Contains(err.Error(), b.WorkspaceID.String()) {
		t.Fatalf("error does not name the candidates: %v", err)
	}
	for _, ws := range []api.BootstrapResult{a, b} {
		if keys, err := st.ListAPIKeys(context.Background(), ws.WorkspaceID); err != nil || len(keys) != 0 {
			t.Fatalf("key created despite ambiguity: %v %v", keys, err)
		}
	}
	if _, _, err := runAPIKey(t, st, "list", "--workspace", name); err == nil {
		t.Fatal("list accepted an ambiguous name")
	}
	if _, _, err := runAPIKey(t, st, "create", "--workspace", uniqueName("missing"), "--name", "x"); err == nil {
		t.Fatal("unknown workspace accepted")
	}
	if _, _, err := runAPIKey(t, st, "create", "--workspace", a.WorkspaceID.String(), "--name", "x"); err != nil {
		t.Fatalf("id of an ambiguous name: %v", err)
	}
}

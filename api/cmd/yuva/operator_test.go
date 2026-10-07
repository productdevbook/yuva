package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/secret"
	"github.com/productdevbook/yuva/api/internal/store"
)

const testMasterKey = "eXV2YS1kZXYtb25seS1tYXN0ZXIta2V5LTMyYnl0ZXM="

func operatorServer(t *testing.T, st *store.Store) (*api.Server, *secret.Key) {
	t.Helper()
	key, err := secret.ParseKey(testMasterKey)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.DiscardHandler)
	return api.New(api.Deps{Log: log, Store: st, Version: "test", Mailer: mail.Log{Log: log}, Secrets: key}), key
}

func runInbox(t *testing.T, st *store.Store, srv *api.Server, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := inboxCommand(context.Background(), st, srv, args, &stdout, &stderr)
	return stdout.String(), err
}

func runChannel(t *testing.T, st *store.Store, srv *api.Server, stdin string, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := channelCommand(context.Background(), st, srv, args, strings.NewReader(stdin), &stdout, &stderr)
	return stdout.String(), err
}

func TestInboxAndChannelCommands(t *testing.T) {
	st := testStore(t)
	srv, key := operatorServer(t, st)
	ctx := context.Background()
	name := uniqueName("cli-inbox")
	ws := bootstrapWorkspace(t, st, name)
	other := bootstrapWorkspace(t, st, uniqueName("cli-inbox-other"))

	out, err := runInbox(t, st, srv, "create", "--workspace", name, "--name", "Support Desk", "--locale", "tr",
		"--timezone", "Europe/Istanbul", "--mode", "async", "--expected-reply-minutes", "1440")
	if err != nil {
		t.Fatal(err)
	}
	inboxID, err := uuid.Parse(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("create must print the inbox id, got %q", out)
	}
	in, err := st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws.WorkspaceID, ID: inboxID})
	if err != nil {
		t.Fatal(err)
	}
	if in.Slug != "support-desk" || in.DefaultLocale != "tr" || in.Timezone != "Europe/Istanbul" || in.Mode != "async" ||
		in.ExpectedReplyMinutes == nil || *in.ExpectedReplyMinutes != 1440 || len(in.IdentitySecret) == 0 {
		t.Fatalf("inbox stored as %+v", in)
	}
	events, err := st.ListEventsAfter(ctx, store.ListEventsAfterParams{WorkspaceID: ws.WorkspaceID, After: 0, Lim: 100})
	if err != nil {
		t.Fatal(err)
	}
	created := false
	for _, e := range events {
		created = created || (e.Type == "inbox.created" && e.InboxID != nil && *e.InboxID == inboxID)
	}
	if !created {
		t.Fatal("no inbox.created event")
	}

	if _, err := runInbox(t, st, srv, "create", "--workspace", name, "--name", "Support Desk"); err == nil {
		t.Fatal("duplicate slug accepted")
	}
	if _, err := runInbox(t, st, srv, "create", "--workspace", name, "--name", "Bad", "--timezone", "Mars/Olympus"); err == nil {
		t.Fatal("bad time zone accepted")
	}
	if _, err := runInbox(t, st, srv, "create", "--workspace", name, "--name", "Bad", "--mode", "sometimes"); err == nil {
		t.Fatal("bad mode accepted")
	}

	list, err := runInbox(t, st, srv, "list", "--workspace", name)
	if err != nil || !strings.Contains(list, inboxID.String()) || !strings.Contains(list, "support-desk") {
		t.Fatalf("list: %q %v", list, err)
	}
	if list, err := runInbox(t, st, srv, "list", "--workspace", other.WorkspaceID.String()); err != nil || strings.Contains(list, inboxID.String()) {
		t.Fatalf("other workspace list: %q %v", list, err)
	}

	address := uniqueName("help") + "@example.com"
	pwFile := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(pwFile, []byte("smtp-secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = runChannel(t, st, srv, "", "create-email", "--workspace", name, "--inbox", "support-desk", "--name", "Help",
		"--address", strings.ToUpper(address), "--display-name", "Help Desk", "--smtp-host", "smtp.example.com",
		"--smtp-port", "587", "--tls", "starttls", "--smtp-username", "user", "--smtp-password-file", pwFile)
	if err != nil {
		t.Fatal(err)
	}
	channelID, err := uuid.Parse(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("create-email must print the channel id, got %q", out)
	}
	e, err := st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws.WorkspaceID, ChannelID: channelID})
	if err != nil {
		t.Fatal(err)
	}
	if e.Address != address || e.DisplayName != "Help Desk" || e.SmtpHost != "smtp.example.com" || e.SmtpPort != 587 ||
		e.SmtpTls != "starttls" || e.SmtpUsername != "user" || e.AutoReplyEnabled {
		t.Fatalf("email channel stored as %+v", e)
	}
	if bytes.Contains(e.SmtpPassword, []byte("smtp-secret-value")) {
		t.Fatal("SMTP password stored in the clear")
	}
	sealedFor := append(append([]byte("smtp:"), ws.WorkspaceID[:]...), channelID[:]...)
	if plain, err := key.Open(e.SmtpPassword, sealedFor); err != nil || string(plain) != "smtp-secret-value" {
		t.Fatalf("SMTP password does not open: %q %v", plain, err)
	}

	stdinAddress := uniqueName("stdin") + "@example.com"
	out, err = runChannel(t, st, srv, "from-stdin\n", "create-email", "--workspace", name, "--inbox", inboxID.String(),
		"--name", "Stdin", "--address", stdinAddress, "--smtp-host", "smtp.example.com", "--smtp-username", "user",
		"--smtp-password-file", "-")
	if err != nil {
		t.Fatal(err)
	}
	stdinID := uuid.MustParse(strings.TrimSpace(out))
	e, err = st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws.WorkspaceID, ChannelID: stdinID})
	if err != nil {
		t.Fatal(err)
	}
	sealedFor = append(append([]byte("smtp:"), ws.WorkspaceID[:]...), stdinID[:]...)
	if plain, err := key.Open(e.SmtpPassword, sealedFor); err != nil || string(plain) != "from-stdin" {
		t.Fatalf("stdin password: %q %v", plain, err)
	}

	if _, err := runChannel(t, st, srv, "", "create-email", "--workspace", name, "--inbox", "support-desk", "--name", "Again",
		"--address", address); err == nil {
		t.Fatal("address taken twice")
	}
	catchAll := "*@" + uniqueName("domain") + ".example"
	out, err = runChannel(t, st, srv, "", "create-email", "--workspace", name, "--inbox", "support-desk", "--name", "Everything else",
		"--address", catchAll, "--smtp-host", "smtp.example.com")
	if err != nil {
		t.Fatal(err)
	}
	e, err = st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws.WorkspaceID, ChannelID: uuid.MustParse(strings.TrimSpace(out))})
	if err != nil || e.Address != catchAll {
		t.Fatalf("catch-all channel stored as %+v %v", e, err)
	}
	if _, err := runChannel(t, st, srv, "", "create-email", "--workspace", name, "--inbox", "support-desk", "--name", "Twice",
		"--address", catchAll); err == nil {
		t.Fatal("second catch-all for a domain accepted")
	}
	if _, err := runChannel(t, st, srv, "", "create-email", "--workspace", name, "--inbox", "support-desk", "--name", "Plain",
		"--address", uniqueName("plain")+"@example.com", "--smtp-host", "smtp.example.com", "--tls", "none",
		"--smtp-username", "user"); err == nil {
		t.Fatal("username over an unencrypted connection accepted")
	}
	if _, err := runChannel(t, st, srv, "", "create-email", "--workspace", name, "--inbox", "support-desk", "--name", "Port",
		"--address", uniqueName("port")+"@example.com", "--smtp-port", "25"); err == nil {
		t.Fatal("SMTP flags without a host accepted")
	}
	if _, err := runChannel(t, st, srv, "", "create-email", "--workspace", other.WorkspaceID.String(), "--inbox", inboxID.String(),
		"--name", "Foreign", "--address", uniqueName("foreign")+"@example.com"); err == nil {
		t.Fatal("inbox of another workspace accepted")
	}

	list, err = runChannel(t, st, srv, "", "list", "--workspace", name, "--inbox", "support-desk")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, channelID.String()) || !strings.Contains(list, address) || !strings.Contains(list, stdinID.String()) {
		t.Fatalf("channel list misses a channel:\n%s", list)
	}
	if strings.Contains(list, "smtp-secret-value") || strings.Contains(list, "from-stdin") {
		t.Fatal("channel list shows a password")
	}
}

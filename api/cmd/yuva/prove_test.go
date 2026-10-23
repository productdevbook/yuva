package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/store"
)

func runProve(t *testing.T, st *store.Store, args ...string) (string, error) {
	t.Helper()
	srv, _ := operatorServer(t, st)
	var stdout, stderr bytes.Buffer
	err := channelCommand(context.Background(), st, srv, append([]string{"prove"}, args...), strings.NewReader(""), &stdout, &stderr)
	return stdout.String(), err
}

func TestChannelProve(t *testing.T) {
	st := testStore(t)
	srv, _ := operatorServer(t, st)
	ctx := context.Background()
	nameA, nameB := uniqueName("cli-prove-a"), uniqueName("cli-prove-b")
	a, b := bootstrapWorkspace(t, st, nameA), bootstrapWorkspace(t, st, nameB)
	address := uniqueName("help") + "@example.com"
	email := func(ws, addr string) uuid.UUID {
		t.Helper()
		out, err := runChannel(t, st, srv, "", "create-email", "--workspace", ws, "--inbox", "mail", "--name", addr, "--address", addr)
		if err != nil {
			t.Fatal(err)
		}
		id, err := uuid.Parse(strings.Fields(out)[0])
		if err != nil {
			t.Fatalf("create-email printed %q", out)
		}
		return id
	}
	for _, ws := range []string{nameA, nameB} {
		if _, err := runInbox(t, st, srv, "create", "--workspace", ws, "--name", "Mail"); err != nil {
			t.Fatal(err)
		}
	}
	chA, chB := email(nameA, address), email(nameB, address)
	verified := func(ws, ch uuid.UUID) *string {
		t.Helper()
		e, err := st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws, ChannelID: ch})
		if err != nil {
			t.Fatal(err)
		}
		return e.VerifiedBy
	}

	if _, err := runProve(t, st, "--channel", address); err == nil || !strings.Contains(err.Error(), "--workspace") {
		t.Fatalf("an address two workspaces claim, without --workspace: %v", err)
	}
	out, err := runProve(t, st, "--channel", chA.String())
	if err == nil || !strings.Contains(out, "would prove") || verified(a.WorkspaceID, chA) != nil {
		t.Fatalf("without --yes: %v %q", err, out)
	}
	if out, err = runProve(t, st, "--channel", chA.String(), "--yes"); err != nil || !strings.Contains(out, "proved") {
		t.Fatalf("prove: %v %q", err, out)
	}
	if by := verified(a.WorkspaceID, chA); by == nil || *by != "operator" {
		t.Fatalf("verified_by %v", by)
	}
	var actor string
	if err := st.Pool.QueryRow(ctx, "SELECT actor_kind FROM audit_log WHERE workspace_id = $1 AND action = 'channel.address_proven' AND target_id = $2",
		a.WorkspaceID, chA).Scan(&actor); err != nil || actor != "operator" {
		t.Fatalf("audit: %q %v", actor, err)
	}

	out, err = runProve(t, st, "--channel", address, "--workspace", nameB, "--yes")
	if err == nil || !strings.Contains(out, "refused: another workspace proved") || verified(b.WorkspaceID, chB) != nil {
		t.Fatalf("an address another workspace proved: %v %q", err, out)
	}
	if by := verified(a.WorkspaceID, chA); by == nil {
		t.Fatal("the refusal took the address from its holder")
	}

	shared, own := email(nameB, uniqueName("legacy")+"@example.com"), email(nameB, uniqueName("sales")+"@example.com")
	if _, err := st.Pool.Exec(ctx, "UPDATE email_channels SET address = '*@gmail.com' WHERE channel_id = $1 AND NOT EXISTS (SELECT 1 FROM email_channels WHERE workspace_id = $2 AND address = '*@gmail.com')",
		shared, b.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	out, err = runProve(t, st, "--all-unproven", "--workspace", nameB)
	if err == nil || !strings.Contains(out, own.String()) || !strings.Contains(out, "refused: a catch-all on a shared mail domain") {
		t.Fatalf("listing: %v %q", err, out)
	}
	if _, err := runProve(t, st, "--all-unproven"); err == nil {
		t.Fatal("--all-unproven without --workspace accepted")
	}
	if out, err = runProve(t, st, "--all-unproven", "--workspace", nameB, "--yes"); err != nil {
		t.Fatalf("all unproven: %v %q", err, out)
	}
	if verified(b.WorkspaceID, own) == nil || verified(b.WorkspaceID, shared) != nil || verified(b.WorkspaceID, chB) != nil {
		t.Fatalf("after --all-unproven: %q", out)
	}
}

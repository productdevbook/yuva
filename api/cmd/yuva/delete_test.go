package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/store"
)

func TestWorkspaceAndPersonDeleteCommands(t *testing.T) {
	st := testStore(t)
	srv, _ := operatorServer(t, st)
	ctx := context.Background()
	name := uniqueName("cli-delete")
	ws := bootstrapWorkspace(t, st, name)
	other := bootstrapWorkspace(t, st, uniqueName("cli-delete-other"))
	owner, err := st.GetPerson(ctx, ws.PersonID)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer

	if err := personCommand(ctx, srv, []string{"delete", "--email", owner.Email, "--yes"}, &stderr); err == nil || !strings.Contains(err.Error(), "last_owner") {
		t.Fatalf("deleting the last owner: %v", err)
	}
	if err := workspaceCommand(ctx, st, srv, []string{"delete", "--workspace", name}, &stderr); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("delete without --yes: %v", err)
	}
	if live, err := st.IsWorkspaceLive(ctx, ws.WorkspaceID); err != nil || !live {
		t.Fatalf("workspace closed without --yes: %v %v", live, err)
	}
	if err := workspaceCommand(ctx, st, srv, []string{"delete", "--workspace", name, "--yes"}, &stderr); err != nil {
		t.Fatal(err)
	}
	if live, err := st.IsWorkspaceLive(ctx, ws.WorkspaceID); err != nil || live {
		t.Fatalf("workspace still live: %v %v", live, err)
	}
	if _, err := api.FindWorkspace(ctx, st, name); err == nil {
		t.Fatal("a deleted workspace is still found by name")
	}
	if err := workspaceCommand(ctx, st, srv, []string{"delete", "--workspace", ws.WorkspaceID.String(), "--yes"}, &stderr); err == nil {
		t.Fatal("deleting a deleted workspace again succeeded")
	}
	if live, err := st.IsWorkspaceLive(ctx, other.WorkspaceID); err != nil || !live {
		t.Fatalf("other workspace: %v %v", live, err)
	}

	if err := personCommand(ctx, srv, []string{"delete", "--email", owner.Email}, &stderr); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("person delete without --yes: %v", err)
	}
	if err := personCommand(ctx, srv, []string{"delete", "--email", strings.ToUpper(owner.Email), "--yes"}, &stderr); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetPerson(ctx, ws.PersonID); !store.IsNotFound(err) {
		t.Fatalf("person after delete: %v", err)
	}
	if err := personCommand(ctx, srv, []string{"delete", "--email", owner.Email, "--yes"}, &stderr); err == nil {
		t.Fatal("deleting an unknown person succeeded")
	}
}

package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const prefixAlphabet = "abcdefghijkmnopqrstuvwxyz23456789"

func apiKeyBody(k store.ApiKey) oas.ApiKey {
	return oas.ApiKey{
		Id: k.ID, Name: k.Name, Prefix: k.Prefix, CreatedBy: k.CreatedBy, CreatedAt: k.CreatedAt,
		LastUsedAt: k.LastUsedAt, RevokedAt: k.RevokedAt,
	}
}

func newAPIKey() (prefix, secret string) {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	var sb strings.Builder
	sb.WriteString(apiKeyPrefixHead)
	for _, c := range b {
		sb.WriteByte(prefixAlphabet[int(c)%len(prefixAlphabet)])
	}
	prefix = sb.String()
	return prefix, prefix + "_" + randomToken(32)
}

func (s *Server) ListApiKeys(ctx context.Context, _ oas.ListApiKeysRequestObject) (oas.ListApiKeysResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	rows, err := s.st.ListAPIKeys(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	out := oas.ListApiKeys200JSONResponse{Items: make([]oas.ApiKey, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, apiKeyBody(r))
	}
	return out, nil
}

func (s *Server) CreateApiKey(ctx context.Context, req oas.CreateApiKeyRequestObject) (oas.CreateApiKeyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	name, ok := apiKeyName(req.Body.Name)
	if !ok {
		return nil, errValidation("name must be 1 to 200 characters")
	}
	k, secret, err := createAPIKey(ctx, s.st.Queries, p.workspaceID, name, &p.memberID)
	if err != nil {
		return nil, err
	}
	return oas.CreateApiKey201JSONResponse{ApiKey: apiKeyBody(k), Secret: secret}, nil
}

func apiKeyName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	return name, name != "" && len([]rune(name)) <= 200
}

func createAPIKey(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, name string, createdBy *uuid.UUID) (store.ApiKey, string, error) {
	prefix, secret := newAPIKey()
	k, err := q.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		ID: uuid.New(), WorkspaceID: workspaceID, Name: name, Prefix: prefix,
		SecretHash: hashSecret(secret), CreatedBy: createdBy,
	})
	if err != nil {
		return store.ApiKey{}, "", err
	}
	return k, secret, nil
}

// FindWorkspace resolves a workspace id or an exact workspace name for the command line.
func FindWorkspace(ctx context.Context, st *store.Store, ref string) (store.Workspace, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return store.Workspace{}, errors.New("--workspace is required")
	}
	if id, err := uuid.Parse(ref); err == nil {
		ws, err := st.GetWorkspace(ctx, id)
		if store.IsNotFound(err) {
			return store.Workspace{}, fmt.Errorf("no workspace with id %s", id)
		}
		if err == nil && ws.DeletedAt != nil {
			return store.Workspace{}, fmt.Errorf("workspace %s is being deleted", id)
		}
		return ws, err
	}
	matches, err := st.ListWorkspacesByName(ctx, ref)
	if err != nil {
		return store.Workspace{}, err
	}
	switch len(matches) {
	case 0:
		return store.Workspace{}, fmt.Errorf("no workspace named %q", ref)
	case 1:
		return matches[0], nil
	}
	ids := make([]string, len(matches))
	for i, ws := range matches {
		ids[i] = ws.ID.String()
	}
	return store.Workspace{}, fmt.Errorf("%d workspaces are named %q; pass one of their ids: %s", len(matches), ref, strings.Join(ids, ", "))
}

// CreateWorkspaceAPIKey makes an API key for the command line; the secret is returned only here.
func CreateWorkspaceAPIKey(ctx context.Context, st *store.Store, workspaceID uuid.UUID, name string) (store.ApiKey, string, error) {
	name, ok := apiKeyName(name)
	if !ok {
		return store.ApiKey{}, "", errors.New("--name must be 1 to 200 characters")
	}
	return createAPIKey(ctx, st.Queries, workspaceID, name, nil)
}

// RevokeAPIKeyByID revokes a key of any workspace for the command line.
func RevokeAPIKeyByID(ctx context.Context, st *store.Store, id uuid.UUID, now time.Time) (store.ApiKey, error) {
	k, err := st.GetAPIKeyByID(ctx, id)
	if store.IsNotFound(err) {
		return store.ApiKey{}, fmt.Errorf("no API key with id %s", id)
	}
	if err != nil {
		return store.ApiKey{}, err
	}
	if _, err := st.RevokeAPIKey(ctx, store.RevokeAPIKeyParams{Now: now, WorkspaceID: k.WorkspaceID, ID: k.ID}); err != nil {
		return store.ApiKey{}, err
	}
	return k, nil
}

func (s *Server) RevokeApiKey(ctx context.Context, req oas.RevokeApiKeyRequestObject) (oas.RevokeApiKeyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	n, err := s.st.RevokeAPIKey(ctx, store.RevokeAPIKeyParams{Now: s.now(), WorkspaceID: p.workspaceID, ID: req.ApiKeyId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errNotFound
	}
	return oas.RevokeApiKey204Response{}, nil
}

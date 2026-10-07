package api

import (
	"context"
	"crypto/rand"
	"strings"
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
	name := strings.TrimSpace(req.Body.Name)
	if name == "" || len([]rune(name)) > 200 {
		return nil, errValidation("name must be 1 to 200 characters")
	}
	prefix, secret := newAPIKey()
	k, err := s.st.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		ID: uuid.New(), WorkspaceID: p.workspaceID, Name: name, Prefix: prefix,
		SecretHash: hashSecret(secret), CreatedBy: &p.memberID,
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateApiKey201JSONResponse{ApiKey: apiKeyBody(k), Secret: secret}, nil
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

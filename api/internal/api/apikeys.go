package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const prefixAlphabet = "abcdefghijkmnopqrstuvwxyz23456789"

func apiKeyBody(k store.ApiKey, inboxes []uuid.UUID) oas.ApiKey {
	out := oas.ApiKey{
		Id: k.ID, Name: k.Name, Prefix: k.Prefix, CreatedBy: k.CreatedBy, CreatedAt: k.CreatedAt,
		LastUsedAt: k.LastUsedAt, RevokedAt: k.RevokedAt, ExpiresAt: k.ExpiresAt, BotName: k.BotName,
		BotAvatarUrl: k.BotAvatarUrl, Scopes: make([]oas.ApiKeyScope, len(k.Scopes)),
	}
	for i, sc := range k.Scopes {
		out.Scopes[i] = oas.ApiKeyScope(sc)
	}
	if k.InboxLimited {
		if inboxes == nil {
			inboxes = []uuid.UUID{}
		}
		out.InboxIds = &inboxes
	}
	return out
}

func apiKeyInboxes(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	rows, err := q.ListAPIKeyInboxes(ctx, store.ListAPIKeyInboxesParams{WorkspaceID: workspaceID, ApiKeyIds: ids})
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID][]uuid.UUID{}
	for _, r := range rows {
		out[r.ApiKeyID] = append(out[r.ApiKeyID], r.InboxID)
	}
	return out, nil
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
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	inboxes, err := apiKeyInboxes(ctx, s.st.Queries, p.workspaceID, ids)
	if err != nil {
		return nil, err
	}
	out := oas.ListApiKeys200JSONResponse{Items: make([]oas.ApiKey, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, apiKeyBody(r, inboxes[r.ID]))
	}
	return out, nil
}

// APIKeySpec is what a new key is made with; nil Scopes means every scope.
type APIKeySpec struct {
	Name         string
	Scopes       []string
	Inboxes      []uuid.UUID
	ExpiresAt    *time.Time
	BotName      *string
	BotAvatarURL *string
}

func (s *Server) CreateApiKey(ctx context.Context, req oas.CreateApiKeyRequestObject) (oas.CreateApiKeyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	spec := APIKeySpec{Name: b.Name, ExpiresAt: b.ExpiresAt, BotName: b.BotName, BotAvatarURL: b.BotAvatarUrl}
	if b.Scopes != nil {
		spec.Scopes = make([]string, len(*b.Scopes))
		for i, sc := range *b.Scopes {
			spec.Scopes[i] = string(sc)
		}
	}
	if b.InboxIds != nil {
		spec.Inboxes = *b.InboxIds
		if len(spec.Inboxes) == 0 {
			return nil, errValidation("inbox_ids must name at least one inbox; leave it out for every inbox")
		}
	}
	var (
		k      store.ApiKey
		secret string
	)
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		var err error
		k, secret, err = createAPIKey(ctx, q, p.workspaceID, spec, &p.memberID, s.now())
		return err
	})
	if err != nil {
		var invalid apiKeySpecError
		if errors.As(err, &invalid) {
			return nil, errValidation(string(invalid))
		}
		return nil, err
	}
	inboxes, err := apiKeyInboxes(ctx, s.st.Queries, p.workspaceID, []uuid.UUID{k.ID})
	if err != nil {
		return nil, err
	}
	return oas.CreateApiKey201JSONResponse{ApiKey: apiKeyBody(k, inboxes[k.ID]), Secret: secret}, nil
}

func (s *Server) UpdateApiKey(ctx context.Context, req oas.UpdateApiKeyRequestObject) (oas.UpdateApiKeyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	k, err := s.st.GetAPIKey(ctx, store.GetAPIKeyParams{WorkspaceID: p.workspaceID, ID: req.ApiKeyId})
	if store.IsNotFound(err) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	arg := store.UpdateAPIKeyParams{WorkspaceID: p.workspaceID, ID: k.ID, Name: k.Name, BotName: k.BotName, BotAvatarUrl: k.BotAvatarUrl}
	if b.Name != nil {
		name, ok := apiKeyName(*b.Name)
		if !ok {
			return nil, errValidation("name must be 1 to 200 characters")
		}
		arg.Name = name
	}
	if b.BotName.IsSpecified() {
		arg.BotName = nil
		if !b.BotName.IsNull() {
			name, ok := apiKeyName(b.BotName.MustGet())
			if !ok {
				return nil, errValidation("bot_name must be 1 to 200 characters")
			}
			arg.BotName = &name
		}
	}
	if b.BotAvatarUrl.IsSpecified() {
		arg.BotAvatarUrl = nil
		if !b.BotAvatarUrl.IsNull() {
			u, ok := botAvatarURL(b.BotAvatarUrl.MustGet())
			if !ok {
				return nil, errValidation("bot_avatar_url must be an https URL of at most 2000 characters")
			}
			arg.BotAvatarUrl = &u
		}
	}
	k, err = s.st.UpdateAPIKey(ctx, arg)
	if err != nil {
		return nil, err
	}
	inboxes, err := apiKeyInboxes(ctx, s.st.Queries, p.workspaceID, []uuid.UUID{k.ID})
	if err != nil {
		return nil, err
	}
	return oas.UpdateApiKey200JSONResponse(apiKeyBody(k, inboxes[k.ID])), nil
}

func apiKeyName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	return name, name != "" && len([]rune(name)) <= 200
}

func botAvatarURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	return raw, err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && len(raw) <= 2000
}

type apiKeySpecError string

func (e apiKeySpecError) Error() string { return string(e) }

func createAPIKey(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, spec APIKeySpec, createdBy *uuid.UUID, now time.Time) (store.ApiKey, string, error) {
	name, ok := apiKeyName(spec.Name)
	if !ok {
		return store.ApiKey{}, "", apiKeySpecError("name must be 1 to 200 characters")
	}
	var scopes []string
	for _, sc := range allScopes {
		if spec.Scopes == nil || slices.Contains(spec.Scopes, string(sc)) {
			scopes = append(scopes, string(sc))
		}
	}
	for _, sc := range spec.Scopes {
		if !oas.ApiKeyScope(sc).Valid() {
			return store.ApiKey{}, "", apiKeySpecError("unknown scope " + sc)
		}
	}
	if len(scopes) == 0 {
		return store.ApiKey{}, "", apiKeySpecError("scopes must name at least one scope")
	}
	inboxes := slices.Clone(spec.Inboxes)
	slices.SortFunc(inboxes, func(a, b uuid.UUID) int { return a.Compare(b) })
	inboxes = slices.Compact(inboxes)
	if len(inboxes) > 100 {
		return store.ApiKey{}, "", apiKeySpecError("a key can be limited to at most 100 inboxes")
	}
	if len(inboxes) > 0 {
		for _, sc := range unlimitedOnlyScopes {
			if slices.Contains(scopes, string(sc)) {
				return store.ApiKey{}, "", apiKeySpecError("a key limited to inboxes cannot hold " + string(sc))
			}
		}
	}
	if spec.ExpiresAt != nil && !spec.ExpiresAt.After(now) {
		return store.ApiKey{}, "", apiKeySpecError("expires_at must be in the future")
	}
	var botName, avatar *string
	if spec.BotName != nil {
		n, ok := apiKeyName(*spec.BotName)
		if !ok {
			return store.ApiKey{}, "", apiKeySpecError("bot_name must be 1 to 200 characters")
		}
		botName = &n
	}
	if spec.BotAvatarURL != nil {
		u, ok := botAvatarURL(*spec.BotAvatarURL)
		if !ok {
			return store.ApiKey{}, "", apiKeySpecError("bot_avatar_url must be an https URL of at most 2000 characters")
		}
		avatar = &u
	}
	for _, id := range inboxes {
		if _, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: workspaceID, ID: id}); store.IsNotFound(err) {
			return store.ApiKey{}, "", apiKeySpecError("inbox_ids names an inbox that does not exist")
		} else if err != nil {
			return store.ApiKey{}, "", err
		}
	}
	prefix, secret := newAPIKey()
	k, err := q.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		ID: uuid.New(), WorkspaceID: workspaceID, Name: name, Prefix: prefix,
		SecretHash: hashSecret(secret), CreatedBy: createdBy, Scopes: scopes, InboxLimited: len(inboxes) > 0,
		ExpiresAt: spec.ExpiresAt, BotName: botName, BotAvatarUrl: avatar,
	})
	if err != nil {
		return store.ApiKey{}, "", err
	}
	for _, id := range inboxes {
		if err := q.AddAPIKeyInbox(ctx, store.AddAPIKeyInboxParams{WorkspaceID: workspaceID, ApiKeyID: k.ID, InboxID: id}); err != nil {
			return store.ApiKey{}, "", err
		}
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
func CreateWorkspaceAPIKey(ctx context.Context, st *store.Store, workspaceID uuid.UUID, spec APIKeySpec) (store.ApiKey, string, error) {
	var (
		k      store.ApiKey
		secret string
	)
	err := st.InTx(ctx, func(q *store.Queries) error {
		var err error
		k, secret, err = createAPIKey(ctx, q, workspaceID, spec, nil, time.Now())
		return err
	})
	return k, secret, err
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

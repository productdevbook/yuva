package api

import (
	"context"
	"encoding/json"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxChannelSettingsBytes = 64 << 10

func channelBody(c store.Channel) oas.Channel {
	out := oas.Channel{
		Id: c.ID, InboxId: c.InboxID, Kind: oas.ChannelKind(c.Kind), Name: c.Name,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Settings: oas.ChannelSettings{},
	}
	_ = json.Unmarshal(c.Settings, &out.Settings)
	return out
}

func channelSettings(v *oas.ChannelSettings) ([]byte, error) {
	if v == nil || *v == nil {
		return []byte("{}"), nil
	}
	b := mustJSON(*v)
	if len(b) > maxChannelSettingsBytes {
		return nil, errValidation("settings must be at most 64 KiB")
	}
	return b, nil
}

func (s *Server) ListChannels(ctx context.Context, req oas.ListChannelsRequestObject) (oas.ListChannelsResponseObject, error) {
	p := principalFrom(ctx)
	if _, err := visibleInbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	}
	rows, err := s.st.ListChannels(ctx, store.ListChannelsParams{WorkspaceID: p.workspaceID, InboxID: req.InboxId})
	if err != nil {
		return nil, err
	}
	out := oas.ListChannels200JSONResponse{Items: make([]oas.Channel, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, channelBody(r))
	}
	return out, nil
}

func (s *Server) CreateChannel(ctx context.Context, req oas.CreateChannelRequestObject) (oas.CreateChannelResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	if !req.Body.Kind.Valid() {
		return nil, errValidation("kind must be email, chat, app or api")
	}
	name, err := trimmed(req.Body.Name, 1, 200, "name")
	if err != nil {
		return nil, err
	}
	settings, err := channelSettings(req.Body.Settings)
	if err != nil {
		return nil, err
	}
	if _, err := visibleInbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	}
	c, err := s.st.CreateChannel(ctx, store.CreateChannelParams{
		ID: newID(), WorkspaceID: p.workspaceID, InboxID: req.InboxId, Kind: string(req.Body.Kind),
		Name: name, Settings: settings, Now: s.now(),
	})
	if store.IsForeignKeyViolation(err) {
		return nil, errInboxGone
	}
	if err != nil {
		return nil, err
	}
	return oas.CreateChannel201JSONResponse(channelBody(c)), nil
}

func (s *Server) visibleChannel(ctx context.Context, p principal, id oas.ChannelId) (store.Channel, error) {
	c, err := s.st.GetChannel(ctx, store.GetChannelParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return c, errChannelGone
	}
	if err != nil {
		return c, err
	}
	ok, err := canSeeInbox(ctx, s.st.Queries, p, c.InboxID)
	if err != nil {
		return c, err
	}
	if !ok {
		return c, errChannelGone
	}
	return c, nil
}

func (s *Server) GetChannel(ctx context.Context, req oas.GetChannelRequestObject) (oas.GetChannelResponseObject, error) {
	c, err := s.visibleChannel(ctx, principalFrom(ctx), req.ChannelId)
	if err != nil {
		return nil, err
	}
	return oas.GetChannel200JSONResponse(channelBody(c)), nil
}

func (s *Server) UpdateChannel(ctx context.Context, req oas.UpdateChannelRequestObject) (oas.UpdateChannelResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	cur, err := s.visibleChannel(ctx, p, req.ChannelId)
	if err != nil {
		return nil, err
	}
	name, settings := cur.Name, cur.Settings
	if req.Body.Name != nil {
		if name, err = trimmed(*req.Body.Name, 1, 200, "name"); err != nil {
			return nil, err
		}
	}
	if req.Body.Settings != nil {
		if settings, err = channelSettings(req.Body.Settings); err != nil {
			return nil, err
		}
	}
	c, err := s.st.UpdateChannel(ctx, store.UpdateChannelParams{WorkspaceID: p.workspaceID, ID: cur.ID, Name: name, Settings: settings, Now: s.now()})
	if store.IsNotFound(err) {
		return nil, errChannelGone
	}
	if err != nil {
		return nil, err
	}
	return oas.UpdateChannel200JSONResponse(channelBody(c)), nil
}

func (s *Server) DeleteChannel(ctx context.Context, req oas.DeleteChannelRequestObject) (oas.DeleteChannelResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	if _, err := s.visibleChannel(ctx, p, req.ChannelId); err != nil {
		return nil, err
	}
	if err := s.st.DeleteChannel(ctx, store.DeleteChannelParams{WorkspaceID: p.workspaceID, ID: req.ChannelId}); err != nil {
		return nil, err
	}
	return oas.DeleteChannel204Response{}, nil
}

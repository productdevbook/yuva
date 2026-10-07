package api

import (
	"context"
	"encoding/json"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxChannelSettingsBytes = 64 << 10

func channelBody(c store.Channel, e *store.EmailChannel, chat *store.ChatChannel) oas.Channel {
	out := oas.Channel{
		Id: c.ID, InboxId: c.InboxID, Kind: oas.ChannelKind(c.Kind), Name: c.Name,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Settings: oas.ChannelSettings{},
	}
	_ = json.Unmarshal(c.Settings, &out.Settings)
	if e != nil {
		out.Email = emailChannelBody(*e)
	}
	if chat != nil {
		out.Chat = chatChannelBody(*chat)
	}
	return out
}

func (s *Server) oneChannelBody(ctx context.Context, q *store.Queries, c store.Channel) (oas.Channel, error) {
	if c.Kind == string(oas.ChannelKindChat) {
		chat, err := q.GetChatChannel(ctx, store.GetChatChannelParams{WorkspaceID: c.WorkspaceID, ChannelID: c.ID})
		if store.IsNotFound(err) {
			return channelBody(c, nil, nil), nil
		}
		if err != nil {
			return oas.Channel{}, err
		}
		return channelBody(c, nil, &chat), nil
	}
	if c.Kind != string(oas.ChannelKindEmail) {
		return channelBody(c, nil, nil), nil
	}
	e, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: c.WorkspaceID, ChannelID: c.ID})
	if store.IsNotFound(err) {
		return channelBody(c, nil, nil), nil
	}
	if err != nil {
		return oas.Channel{}, err
	}
	return channelBody(c, &e, nil), nil
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
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	emails, err := emailChannelsByID(ctx, s.st.Queries, p.workspaceID, ids)
	if err != nil {
		return nil, err
	}
	chats, err := chatChannelsByID(ctx, s.st.Queries, p.workspaceID, ids)
	if err != nil {
		return nil, err
	}
	out := oas.ListChannels200JSONResponse{Items: make([]oas.Channel, 0, len(rows))}
	for _, r := range rows {
		var e *store.EmailChannel
		if v, ok := emails[r.ID]; ok {
			e = &v
		}
		var chat *store.ChatChannel
		if v, ok := chats[r.ID]; ok {
			chat = &v
		}
		out.Items = append(out.Items, channelBody(r, e, chat))
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
	isEmail := req.Body.Kind == oas.ChannelKindEmail
	if isEmail && req.Body.Email == nil {
		return nil, errEmailRequired
	}
	if !isEmail && req.Body.Email != nil {
		return nil, errEmailNotAllowed
	}
	isChat := req.Body.Kind == oas.ChannelKindChat
	if isChat && req.Body.Chat == nil {
		return nil, errChatRequired
	}
	if !isChat && req.Body.Chat != nil {
		return nil, errChatNotAllowed
	}
	id := newID()
	var emailArg store.CreateEmailChannelParams
	if isEmail {
		if emailArg, err = s.emailChannelParams(p.workspaceID, id, req.Body.Email, nil); err != nil {
			return nil, err
		}
	}
	var chatArg store.SaveChatChannelParams
	if isChat {
		if chatArg, err = chatChannelParams(p.workspaceID, id, req.Body.Chat, nil); err != nil {
			return nil, err
		}
	}
	if _, err := visibleInbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	}
	var out oas.Channel
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		c, err := q.CreateChannel(ctx, store.CreateChannelParams{
			ID: id, WorkspaceID: p.workspaceID, InboxID: req.InboxId, Kind: string(req.Body.Kind),
			Name: name, Settings: settings, Now: s.now(),
		})
		if store.IsForeignKeyViolation(err) {
			return errInboxGone
		}
		if err != nil {
			return err
		}
		if isChat {
			chat, err := q.SaveChatChannel(ctx, chatArg)
			if err != nil {
				return err
			}
			out = channelBody(c, nil, &chat)
			return nil
		}
		if !isEmail {
			out = channelBody(c, nil, nil)
			return nil
		}
		e, err := saveEmailChannel(ctx, q, emailArg)
		if err != nil {
			return err
		}
		out = channelBody(c, &e, nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateChannel201JSONResponse(out), nil
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
	out, err := s.oneChannelBody(ctx, s.st.Queries, c)
	if err != nil {
		return nil, err
	}
	return oas.GetChannel200JSONResponse(out), nil
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
	if req.Body.Email != nil && cur.Kind != string(oas.ChannelKindEmail) {
		return nil, errEmailNotAllowed
	}
	if req.Body.Chat != nil && cur.Kind != string(oas.ChannelKindChat) {
		return nil, errChatNotAllowed
	}
	var out oas.Channel
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		c, err := q.UpdateChannel(ctx, store.UpdateChannelParams{WorkspaceID: p.workspaceID, ID: cur.ID, Name: name, Settings: settings, Now: s.now()})
		if store.IsNotFound(err) {
			return errChannelGone
		}
		if err != nil {
			return err
		}
		if req.Body.Email != nil {
			var prev *store.EmailChannel
			e, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: p.workspaceID, ChannelID: c.ID})
			if err == nil {
				prev = &e
			} else if !store.IsNotFound(err) {
				return err
			}
			arg, err := s.emailChannelParams(p.workspaceID, c.ID, req.Body.Email, prev)
			if err != nil {
				return err
			}
			if _, err := saveEmailChannel(ctx, q, arg); err != nil {
				return err
			}
		}
		if req.Body.Chat != nil {
			var prev *store.ChatChannel
			cc, err := q.GetChatChannel(ctx, store.GetChatChannelParams{WorkspaceID: p.workspaceID, ChannelID: c.ID})
			if err == nil {
				prev = &cc
			} else if !store.IsNotFound(err) {
				return err
			}
			arg, err := chatChannelParams(p.workspaceID, c.ID, req.Body.Chat, prev)
			if err != nil {
				return err
			}
			if _, err := q.SaveChatChannel(ctx, arg); err != nil {
				return err
			}
		}
		out, err = s.oneChannelBody(ctx, q, c)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateChannel200JSONResponse(out), nil
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

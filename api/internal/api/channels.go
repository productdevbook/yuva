package api

import (
	"context"
	"encoding/json"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxChannelSettingsBytes = 64 << 10

func (s *Server) channelBody(c store.Channel, e *store.EmailChannel, chat *store.ChatChannel, t *store.SesTenant) oas.Channel {
	out := oas.Channel{
		Id: c.ID, InboxId: c.InboxID, Kind: oas.ChannelKind(c.Kind), Name: c.Name,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Settings: oas.ChannelSettings{}, Attachments: s.channelAttachmentsBody(c),
	}
	_ = json.Unmarshal(c.Settings, &out.Settings)
	if e != nil {
		out.Email = emailChannelBody(*e)
		out.Email.Ses = s.emailSesBody(*e, t)
		out.Email.Verification = s.emailVerificationBody(*e)
		if addr := s.forwardingAddress(*e); addr != "" {
			out.ForwardingAddress = &addr
		}
	}
	if chat != nil && c.Kind == string(oas.ChannelKindApp) {
		out.App = appChannelBody(*chat)
	} else if chat != nil {
		out.Chat = chatChannelBody(*chat)
	}
	return out
}

func (s *Server) oneChannelBody(ctx context.Context, q *store.Queries, c store.Channel) (oas.Channel, error) {
	if hasPublicKey(c.Kind) {
		chat, err := q.GetChatChannel(ctx, store.GetChatChannelParams{WorkspaceID: c.WorkspaceID, ChannelID: c.ID})
		if store.IsNotFound(err) {
			return s.channelBody(c, nil, nil, nil), nil
		}
		if err != nil {
			return oas.Channel{}, err
		}
		out := s.channelBody(c, nil, &chat, nil)
		return out, fillAppPush(ctx, q, c.WorkspaceID, &out)
	}
	if c.Kind != string(oas.ChannelKindEmail) {
		return s.channelBody(c, nil, nil, nil), nil
	}
	e, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: c.WorkspaceID, ChannelID: c.ID})
	if store.IsNotFound(err) {
		return s.channelBody(c, nil, nil, nil), nil
	}
	if err != nil {
		return oas.Channel{}, err
	}
	return s.oneChannelBodyWith(ctx, q, c, e)
}

func (s *Server) oneChannelBodyWith(ctx context.Context, q *store.Queries, c store.Channel, e store.EmailChannel) (oas.Channel, error) {
	t, err := s.channelTenant(ctx, q, e)
	if err != nil {
		return oas.Channel{}, err
	}
	return s.channelBody(c, &e, nil, t), nil
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
	tenant, err := inboxTenant(ctx, s.st.Queries, p.workspaceID, req.InboxId)
	if err != nil {
		return nil, err
	}
	pushes, err := s.st.ListAppPushSettings(ctx, store.ListAppPushSettingsParams{WorkspaceID: p.workspaceID, ChannelIds: ids})
	if err != nil {
		return nil, err
	}
	pushByID := make(map[uuid.UUID]*store.AppPushSetting, len(pushes))
	for i := range pushes {
		pushByID[pushes[i].ChannelID] = &pushes[i]
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
		ch := s.channelBody(r, e, chat, tenant)
		if ch.App != nil {
			ch.App.Push = appPushBody(pushByID[r.ID])
		}
		redactChannelSES(p, &ch)
		out.Items = append(out.Items, ch)
	}
	return out, nil
}

func (s *Server) CreateChannel(ctx context.Context, req oas.CreateChannelRequestObject) (oas.CreateChannelResponseObject, error) {
	p := principalFrom(ctx)
	if err := s.requireChannelManager(ctx, p, req.InboxId, uuid.Nil()); err != nil {
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
	limits, err := s.channelAttachmentsInput(req.Body.Attachments)
	if err != nil {
		return nil, err
	}
	isEmail := req.Body.Kind == oas.ChannelKindEmail
	if isEmail && req.Body.Email == nil {
		return nil, errEmailRequired
	}
	if isEmail && req.Body.Email.Dkim != nil && req.Body.Email.Dkim.Enabled {
		return nil, errDkimNoKey
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
	isApp := req.Body.Kind == oas.ChannelKindApp
	if !isApp && req.Body.App != nil {
		return nil, errAppNotAllowed
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
	if isApp {
		if chatArg, err = appChannelParams(p.workspaceID, id, req.Body.App, nil); err != nil {
			return nil, err
		}
	}
	if _, err := visibleInbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	}
	var out oas.Channel
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if !isEmail {
			if err := supportInbox(ctx, q, p.workspaceID, req.InboxId, "a "+string(req.Body.Kind)+" channel, only e-mail channels"); err != nil {
				return err
			}
		}
		c, err := q.CreateChannel(ctx, store.CreateChannelParams{
			ID: id, WorkspaceID: p.workspaceID, InboxID: req.InboxId, Kind: string(req.Body.Kind),
			Name: name, Settings: settings, AttachmentMaxBytes: limits.maxBytes, AttachmentTypes: limits.types, Now: s.now(),
		})
		if store.IsForeignKeyViolation(err) {
			return errInboxGone
		}
		if err != nil {
			return err
		}
		if isChat || isApp {
			chat, err := q.SaveChatChannel(ctx, chatArg)
			if err != nil {
				return err
			}
			if isApp && req.Body.App != nil {
				if err := s.saveAppPush(ctx, q, p, c.ID, req.Body.App.Push); err != nil {
					return err
				}
			}
			out = s.channelBody(c, nil, &chat, nil)
			return fillAppPush(ctx, q, p.workspaceID, &out)
		}
		if !isEmail {
			out = s.channelBody(c, nil, nil, nil)
			return nil
		}
		e, err := saveEmailChannel(ctx, q, emailArg)
		if err != nil {
			return err
		}
		bc, err := s.emailBroadcastParams(p.workspaceID, id, req.Body.Email, nil)
		if err != nil {
			return err
		}
		if bc != nil {
			if e, err = q.SetEmailChannelBroadcast(ctx, *bc); err != nil {
				return err
			}
		}
		if err := s.auditCredentials(ctx, q, p, nil, e); err != nil {
			return err
		}
		if e, err = applySESInput(ctx, q, e, req.Body.Email.Ses); err != nil {
			return err
		}
		if e, err = s.sesChannelSaved(ctx, q, events, nil, e, c.InboxID); err != nil {
			return err
		}
		t, err := inboxTenant(ctx, q, p.workspaceID, c.InboxID)
		if err != nil {
			return err
		}
		out = s.channelBody(c, &e, nil, t)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateChannel201JSONResponse(out), nil
}

// requireChannelManager is requireManagerOrKey, and also lets the owner of a personal mailbox,
// whatever their role, manage its channels. channelID names the inbox when set.
func (s *Server) requireChannelManager(ctx context.Context, p principal, inboxID, channelID uuid.UUID) error {
	err := requireManagerOrKey(p)
	if err == nil || p.isKey() {
		return err
	}
	if channelID != uuid.Nil() {
		c, cerr := s.st.GetChannel(ctx, store.GetChannelParams{WorkspaceID: p.workspaceID, ID: channelID})
		if cerr != nil {
			return err
		}
		inboxID = c.InboxID
	}
	in, ierr := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: p.workspaceID, ID: inboxID})
	if ierr != nil || in.OwnerMemberID == nil || !canReadPersonal(p, in) {
		return err
	}
	return nil
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
	redactChannelSES(principalFrom(ctx), &out)
	return oas.GetChannel200JSONResponse(out), nil
}

func (s *Server) UpdateChannel(ctx context.Context, req oas.UpdateChannelRequestObject) (oas.UpdateChannelResponseObject, error) {
	p := principalFrom(ctx)
	if err := s.requireChannelManager(ctx, p, uuid.Nil(), req.ChannelId); err != nil {
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
	limits, err := s.channelAttachmentsInput(req.Body.Attachments)
	if err != nil {
		return nil, err
	}
	if req.Body.Email != nil && cur.Kind != string(oas.ChannelKindEmail) {
		return nil, errEmailNotAllowed
	}
	if req.Body.Chat != nil && cur.Kind != string(oas.ChannelKindChat) {
		return nil, errChatNotAllowed
	}
	if req.Body.App != nil && cur.Kind != string(oas.ChannelKindApp) {
		return nil, errAppNotAllowed
	}
	var out oas.Channel
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := q.UpdateChannel(ctx, store.UpdateChannelParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Name: name, Settings: settings, Now: s.now(),
			SetAttachmentMaxBytes: limits.setMaxBytes, AttachmentMaxBytes: limits.maxBytes,
			SetAttachmentTypes: limits.setTypes, AttachmentTypes: limits.types,
		})
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
			saved, err := saveEmailChannel(ctx, q, arg)
			if err != nil {
				return err
			}
			bc, err := s.emailBroadcastParams(p.workspaceID, c.ID, req.Body.Email, prev)
			if err != nil {
				return err
			}
			if bc != nil {
				if saved, err = q.SetEmailChannelBroadcast(ctx, *bc); err != nil {
					return err
				}
			}
			if err := s.auditCredentials(ctx, q, p, prev, saved); err != nil {
				return err
			}
			if err := s.applyDkimInput(ctx, q, prev, saved, req.Body.Email.Dkim); err != nil {
				return err
			}
			if saved, err = applySESInput(ctx, q, saved, req.Body.Email.Ses); err != nil {
				return err
			}
			if _, err = s.sesChannelSaved(ctx, q, events, prev, saved, c.InboxID); err != nil {
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
		if req.Body.App != nil {
			cc, err := q.GetChatChannel(ctx, store.GetChatChannelParams{WorkspaceID: p.workspaceID, ChannelID: c.ID})
			if err != nil && !store.IsNotFound(err) {
				return err
			}
			var prev *store.ChatChannel
			if err == nil {
				prev = &cc
			}
			arg, err := appChannelParams(p.workspaceID, c.ID, req.Body.App, prev)
			if err != nil {
				return err
			}
			if _, err := q.SaveChatChannel(ctx, arg); err != nil {
				return err
			}
			if err := s.saveAppPush(ctx, q, p, c.ID, req.Body.App.Push); err != nil {
				return err
			}
		}
		out, err = s.oneChannelBody(ctx, q, c)
		return err
	})
	if err != nil {
		return nil, err
	}
	if cur.Kind == string(oas.ChannelKindChat) || cur.Kind == string(oas.ChannelKindApp) {
		inbox := cur.InboxID
		s.signal(ctx, realtime.Event{Type: realtime.ChannelUpdated, WorkspaceID: p.workspaceID, InboxID: &inbox, Data: mustJSON(channelRef{ID: cur.ID})})
	}
	return oas.UpdateChannel200JSONResponse(out), nil
}

func (s *Server) DeleteChannel(ctx context.Context, req oas.DeleteChannelRequestObject) (oas.DeleteChannelResponseObject, error) {
	p := principalFrom(ctx)
	if err := s.requireChannelManager(ctx, p, uuid.Nil(), req.ChannelId); err != nil {
		return nil, err
	}
	c, err := s.visibleChannel(ctx, p, req.ChannelId)
	if err != nil {
		return nil, err
	}
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if _, err := q.ChannelRoadmapBoard(ctx, store.ChannelRoadmapBoardParams{WorkspaceID: p.workspaceID, ChannelID: c.ID}); err == nil {
			return errChannelHasBoard
		} else if !store.IsNotFound(err) {
			return err
		}
		e, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: p.workspaceID, ChannelID: c.ID})
		found := err == nil
		if err != nil && !store.IsNotFound(err) {
			return err
		}
		if err := q.DeleteChannel(ctx, store.DeleteChannelParams{WorkspaceID: p.workspaceID, ID: c.ID}); err != nil {
			return err
		}
		if found && s.sesOn() && (channelUsesSES(e) || e.SesState != sesStateNone) {
			events.jobOnce(SESTenantSyncArgs{WorkspaceID: p.workspaceID, InboxID: c.InboxID}, nil)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteChannel204Response{}, nil
}

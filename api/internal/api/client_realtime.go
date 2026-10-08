package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"
	"uuid"

	"github.com/coder/websocket"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

type clientFrame struct {
	ID             int64      `json:"id,omitempty"`
	Type           string     `json:"type"`
	ConversationID *uuid.UUID `json:"conversation_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	Data           any        `json:"data"`
}

const (
	clientPresenceFrame = "presence"
	clientInboxFrame    = "inbox.updated"
	// statusUnsent marks a conversation whose status a resuming connection may have missed, so the
	// next change, replayed or live, is sent even when it equals the current status.
	statusUnsent = ""
)

type channelRef struct {
	ID uuid.UUID `json:"id"`
}

type closeStream struct {
	code   websocket.StatusCode
	reason string
}

func (c *closeStream) Error() string { return c.reason }

func (s *Server) serveClientRealtime(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		writeProblem(w, errNotFound)
		return
	}
	if !s.hub.Listening() {
		writeProblem(w, errRealtimeUnavailable)
		return
	}
	var resumeFrom *int64
	if v := r.URL.Query().Get("last_event_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id < 0 {
			writeProblem(w, errValidation("last_event_id must be a non-negative integer"))
			return
		}
		resumeFrom = &id
	}
	token := websocketToken(r)
	cp, err := s.resolveContact(r.Context(), token)
	if err != nil {
		var e *apiError
		if errors.As(err, &e) {
			writeProblem(w, e)
			return
		}
		s.log.ErrorContext(r.Context(), "client realtime auth", slog.Any("error", err))
		writeProblem(w, errInternal)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true, Subprotocols: []string{wsProtocol}})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	code, reason, err := s.clientStream(conn, r, cp, token, resumeFrom)
	if err != nil && r.Context().Err() == nil {
		s.log.WarnContext(r.Context(), "client realtime stream ended", slog.Any("error", err))
	}
	if code != 0 {
		_ = conn.Close(code, reason)
	}
}

func (s *Server) clientStream(conn *websocket.Conn, r *http.Request, cp contactPrincipal, token string, resumeFrom *int64) (websocket.StatusCode, string, error) {
	ctx := conn.CloseRead(r.Context())
	sub := s.hub.Subscribe(cp.workspaceID)
	defer s.hub.Unsubscribe(sub)
	f := &clientFilter{s: s, cp: cp, names: map[uuid.UUID]string{}, readAt: map[uuid.UUID]time.Time{}}
	if err := f.load(ctx, resumeFrom != nil); err != nil {
		return websocket.StatusInternalError, "internal", err
	}
	connID := newID()
	if err := s.st.OpenConnection(ctx, store.OpenConnectionParams{ID: connID, WorkspaceID: cp.workspaceID, ContactID: &cp.contactID, Now: s.now()}); err != nil {
		return websocket.StatusInternalError, "internal", err
	}
	defer func() {
		bg := context.WithoutCancel(ctx)
		// Last seen moves before the connection goes, so a continuity check never sees the contact
		// neither connected nor recently seen.
		if err := s.st.SeeContactSession(bg, store.SeeContactSessionParams{WorkspaceID: cp.workspaceID, ID: cp.sessionID, Now: s.now()}); err != nil {
			s.log.WarnContext(bg, "client realtime close", slog.Any("error", err))
		}
		if err := s.st.CloseConnection(bg, store.CloseConnectionParams{WorkspaceID: cp.workspaceID, ID: connID}); err != nil {
			s.log.WarnContext(bg, "client realtime close", slog.Any("error", err))
		}
		s.scheduleContactContinuity(bg, cp.workspaceID, cp.contactID)
	}()
	send := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		wctx, cancel := context.WithTimeout(ctx, realtimeWriteTimeout)
		defer cancel()
		return conn.Write(wctx, websocket.MessageText, b)
	}
	deliver := func(e realtime.Event) error {
		frames, err := f.transform(ctx, e)
		if err != nil {
			return err
		}
		for _, fr := range frames {
			if err := send(fr); err != nil {
				return err
			}
		}
		return nil
	}
	finish := func(err error) (websocket.StatusCode, string, error) {
		var cs *closeStream
		if errors.As(err, &cs) {
			return cs.code, cs.reason, nil
		}
		return 0, "", err
	}

	last, err := s.st.LastEventID(ctx, cp.workspaceID)
	if err != nil {
		return websocket.StatusInternalError, "internal", err
	}
	if resumeFrom != nil {
		known := *resumeFrom == 0
		if !known {
			if known, err = s.st.EventExists(ctx, store.EventExistsParams{WorkspaceID: cp.workspaceID, ID: *resumeFrom}); err != nil {
				return websocket.StatusInternalError, "internal", err
			}
		}
		if !known {
			if err := send(realtimeResync{Type: "resync_required"}); err != nil {
				return 0, "", err
			}
		} else {
			last = *resumeFrom
			for {
				rows, err := s.st.ListEventsAfter(ctx, store.ListEventsAfterParams{WorkspaceID: cp.workspaceID, After: last, Lim: replayPageSize})
				if err != nil {
					return websocket.StatusInternalError, "internal", err
				}
				for _, row := range rows {
					last = row.ID
					if err := deliver(realtime.FromRow(row)); err != nil {
						return finish(err)
					}
				}
				if len(rows) < replayPageSize {
					break
				}
			}
		}
	}
	if err := send(realtimeReady{Type: "ready", LastEventID: last}); err != nil {
		return 0, "", err
	}
	if err := deliver(realtime.Event{Type: realtime.PresenceHint, WorkspaceID: cp.workspaceID}); err != nil {
		return finish(err)
	}

	tick := time.NewTicker(realtimeHeartbeat)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return 0, "", nil
		case <-sub.Done():
			switch sub.Reason() {
			case realtime.ReasonSlowConsumer:
				return websocket.StatusTryAgainLater, realtime.ReasonSlowConsumer, nil
			default:
				return websocket.StatusServiceRestart, realtime.ReasonRestart, nil
			}
		case e := <-sub.Events():
			if !e.Ephemeral() {
				if e.ID <= last {
					continue
				}
				last = e.ID
			}
			if err := deliver(e); err != nil {
				return finish(err)
			}
		case <-tick.C:
			pctx, cancel := context.WithTimeout(ctx, realtimeWriteTimeout)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return 0, "", err
			}
			if _, err := s.resolveContact(ctx, token); err != nil {
				var e *apiError
				if errors.As(err, &e) {
					return websocket.StatusPolicyViolation, e.Code, nil
				}
				s.log.WarnContext(ctx, "client realtime re-check", slog.Any("error", err))
				continue
			}
			if err := s.st.SeeConnection(ctx, store.SeeConnectionParams{WorkspaceID: cp.workspaceID, ID: connID, Now: s.now()}); err != nil {
				s.log.WarnContext(ctx, "client realtime presence", slog.Any("error", err))
			}
			if err := deliver(realtime.Event{Type: realtime.PresenceHint, WorkspaceID: cp.workspaceID}); err != nil {
				return finish(err)
			}
		}
	}
}

// clientFilter turns workspace events into what a contact may see: their own conversations in
// the session's inbox, messages without notes or internal events, the status of conversations,
// and for live inboxes members' typing, read positions and presence.
type clientFilter struct {
	s        *Server
	cp       contactPrincipal
	inbox    store.Inbox
	chat     store.ChatChannel
	settings []byte
	convs    map[uuid.UUID]string
	names    map[uuid.UUID]string
	readAt   map[uuid.UUID]time.Time
	presence []byte
}

func (f *clientFilter) load(ctx context.Context, resuming bool) error {
	in, err := f.s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: f.cp.workspaceID, ID: f.cp.inboxID})
	if err != nil {
		return err
	}
	f.inbox, f.chat = in, f.cp.chat
	if _, err := f.inboxFrames(ctx); err != nil {
		return err
	}
	rows, err := f.s.st.ListContactConversationIDs(ctx, store.ListContactConversationIDsParams{
		WorkspaceID: f.cp.workspaceID, InboxID: f.cp.inboxID, ContactID: f.cp.contactID,
	})
	if err != nil {
		return err
	}
	f.convs = make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		f.convs[r.ID] = r.Status
		if resuming {
			f.convs[r.ID] = statusUnsent
		}
	}
	return nil
}

func (f *clientFilter) live() bool { return f.inbox.Mode == string(oas.Live) }

func (f *clientFilter) memberName(ctx context.Context, id *uuid.UUID) (string, error) {
	if id == nil {
		return "", nil
	}
	if n, ok := f.names[*id]; ok {
		return n, nil
	}
	names, err := memberNames(ctx, f.s.st.Queries, f.cp.workspaceID, []uuid.UUID{*id})
	if err != nil {
		return "", err
	}
	f.names[*id] = names[*id]
	return names[*id], nil
}

func (f *clientFilter) clientMessage(ctx context.Context, m oas.Message) (oas.ClientMessage, error) {
	name, err := f.memberName(ctx, m.Author.MemberId)
	if err != nil {
		return oas.ClientMessage{}, err
	}
	out := oas.ClientMessage{
		Id: m.Id, ConversationId: m.ConversationId, Direction: oas.In, Body: m.Body, Html: m.Html, CreatedAt: m.CreatedAt,
		Attachments: make([]oas.ClientAttachment, 0, len(m.Attachments)),
	}
	if m.Direction != nil {
		out.Direction = *m.Direction
	}
	out.Author = clientAuthor(m.Author, map[uuid.UUID]string{derefID(m.Author.MemberId): name})
	if m.Author.Type == oas.AuthorTypeContact {
		out.ClientId = m.ClientId
	}
	for _, a := range m.Attachments {
		out.Attachments = append(out.Attachments, oas.ClientAttachment{
			Id: a.Id, Filename: a.Filename, ContentType: a.ContentType, Size: a.Size, ContentId: a.ContentId, Inline: a.Inline,
		})
	}
	return out, nil
}

func derefID(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil()
	}
	return *id
}

func (f *clientFilter) presenceFrames(ctx context.Context) ([]any, error) {
	if !f.live() {
		return nil, nil
	}
	p, err := f.s.presence(ctx, f.s.st.Queries, f.inbox)
	if err != nil || p == nil {
		return nil, err
	}
	b := mustJSON(p)
	if string(b) == string(f.presence) {
		return nil, nil
	}
	f.presence = b
	return []any{clientFrame{Type: clientPresenceFrame, CreatedAt: f.s.now(), Data: p}}, nil
}

// inboxFrames sends the inbox's public settings when they differ from what this connection last
// saw; the first call only records them.
func (f *clientFilter) inboxFrames(ctx context.Context) ([]any, error) {
	in, err := f.s.clientInbox(ctx, f.s.st.Queries, f.inbox, f.chat)
	if err != nil {
		return nil, err
	}
	b := mustJSON(in)
	first := f.settings == nil
	if string(b) == string(f.settings) {
		return nil, nil
	}
	f.settings = b
	if first {
		return nil, nil
	}
	return []any{oas.ClientInboxUpdatedEvent{Type: oas.ClientInboxUpdatedEventType(clientInboxFrame), CreatedAt: f.s.now(), Data: in}}, nil
}

func (f *clientFilter) transform(ctx context.Context, e realtime.Event) ([]any, error) {
	ownConv := false
	if e.ConversationID != nil {
		_, ownConv = f.convs[*e.ConversationID]
	}
	frameAs := func(typ string, data any) []any {
		return []any{clientFrame{ID: e.ID, Type: typ, ConversationID: e.ConversationID, CreatedAt: e.CreatedAt, Data: data}}
	}
	frame := func(data any) []any { return frameAs(e.Type, data) }
	switch e.Type {
	case realtime.ConversationCreated, realtime.ConversationUpdated:
		var c oas.Conversation
		if err := json.Unmarshal(e.Data, &c); err != nil {
			return nil, err
		}
		if c.InboxId != f.cp.inboxID {
			delete(f.convs, c.Id)
			return nil, nil
		}
		prev, known := f.convs[c.Id]
		if c.ContactId != f.cp.contactID {
			delete(f.convs, c.Id)
			return nil, nil
		}
		f.convs[c.Id] = string(c.Status)
		if !known || e.Type == realtime.ConversationCreated {
			return frameAs(realtime.ConversationCreated, oas.ClientConversation{
				Id: c.Id, Subject: c.Subject, Status: c.Status, LastMessageAt: c.LastMessageAt, CreatedAt: c.CreatedAt,
			}), nil
		}
		if prev != statusUnsent && prev == string(c.Status) {
			return nil, nil
		}
		return frame(oas.ClientConversationStatus{Id: c.Id, Status: c.Status, UpdatedAt: c.UpdatedAt}), nil
	case realtime.MessageCreated, realtime.MessageUpdated:
		if !ownConv {
			return nil, nil
		}
		var m oas.Message
		if err := json.Unmarshal(e.Data, &m); err != nil {
			return nil, err
		}
		if m.Kind != oas.MessageKindMessage {
			return nil, nil
		}
		cm, err := f.clientMessage(ctx, m)
		if err != nil {
			return nil, err
		}
		return frame(cm), nil
	case realtime.ConversationRead:
		if !ownConv || !f.live() {
			return nil, nil
		}
		var rd oas.ConversationRead
		if err := json.Unmarshal(e.Data, &rd); err != nil {
			return nil, err
		}
		if rd.LastReadMessageId == nil {
			return nil, nil
		}
		pos, err := f.s.st.GetMessagePosition(ctx, store.GetMessagePositionParams{WorkspaceID: f.cp.workspaceID, ConversationID: *e.ConversationID, ID: *rd.LastReadMessageId})
		if store.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !pos.CreatedAt.After(f.readAt[*e.ConversationID]) {
			return nil, nil
		}
		f.readAt[*e.ConversationID] = pos.CreatedAt
		return frameAs("read", oas.ClientRead{ConversationId: *e.ConversationID, ReadAt: pos.CreatedAt}), nil
	case realtime.Typing:
		if !ownConv || !f.live() {
			return nil, nil
		}
		var ty oas.Typing
		if err := json.Unmarshal(e.Data, &ty); err != nil {
			return nil, err
		}
		if ty.Author.Type != oas.TypingAuthorTypeMember {
			return nil, nil
		}
		name := ""
		if ty.Author.Name != nil {
			name = *ty.Author.Name
		}
		ini := initials(name)
		return frame(oas.ClientTyping{
			ConversationId: ty.ConversationId, Typing: ty.Typing,
			Author: oas.ClientMessageAuthor{Type: oas.AuthorTypeMember, Name: &name, Initials: &ini},
		}), nil
	case realtime.ContactDeleted:
		var ref oas.ContactRef
		if err := json.Unmarshal(e.Data, &ref); err != nil {
			return nil, err
		}
		if ref.Id == f.cp.contactID {
			return nil, &closeStream{websocket.StatusPolicyViolation, "unauthenticated"}
		}
		return nil, nil
	case realtime.InboxDeleted:
		if e.InboxID != nil && *e.InboxID == f.cp.inboxID {
			return nil, &closeStream{websocket.StatusPolicyViolation, "unauthenticated"}
		}
		return nil, nil
	case realtime.InboxUpdated:
		if e.InboxID == nil || *e.InboxID != f.cp.inboxID {
			return nil, nil
		}
		in, err := f.s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: f.cp.workspaceID, ID: f.cp.inboxID})
		if err != nil {
			return nil, err
		}
		f.inbox = in
		frames, err := f.presenceFrames(ctx)
		if err != nil {
			return nil, err
		}
		more, err := f.inboxFrames(ctx)
		return append(frames, more...), err
	case realtime.ChannelUpdated:
		var ref channelRef
		if err := json.Unmarshal(e.Data, &ref); err != nil {
			return nil, err
		}
		if ref.ID != f.cp.channelID {
			return nil, nil
		}
		ch, err := f.s.st.GetSessionChannel(ctx, store.GetSessionChannelParams{WorkspaceID: f.cp.workspaceID, ChannelID: f.cp.channelID})
		if store.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		f.chat = sessionChat(ch)
		return f.inboxFrames(ctx)
	case realtime.PresenceHint:
		return f.presenceFrames(ctx)
	}
	return nil, nil
}

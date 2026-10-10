package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/coder/websocket"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	realtimeFrameLimit   = 1024
	realtimeHeartbeat    = 30 * time.Second
	realtimeWriteTimeout = 10 * time.Second
	replayPageSize       = 500
)

var errRealtimeUnavailable = problem(http.StatusServiceUnavailable, "realtime_unavailable", "the event feed is reconnecting; try again shortly")

var errOriginNotAllowed = problem(http.StatusForbidden, "origin_not_allowed", "this origin may not open a realtime connection")

type realtimeReady struct {
	Type        string `json:"type"`
	LastEventID int64  `json:"last_event_id"`
}

type realtimeResync struct {
	Type string `json:"type"`
}

func (s *Server) originAllowed(origin string) bool {
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	got := strings.ToLower(u.Scheme + "://" + u.Host)
	allowed := []string{s.auth.PublicURL}
	if s.webauthn != nil {
		allowed = append(allowed, s.webauthn.Config.RPOrigins...)
	}
	for _, a := range allowed {
		if au, err := url.Parse(a); err == nil && strings.ToLower(au.Scheme+"://"+au.Host) == got {
			return true
		}
	}
	return false
}

func (s *Server) serveRealtime(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		writeProblem(w, errNotFound)
		return
	}
	if !s.hub.Listening() {
		writeProblem(w, errRealtimeUnavailable)
		return
	}
	query := r.URL.Query()
	var resumeFrom *int64
	if v := query.Get("last_event_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id < 0 {
			writeProblem(w, errValidation("last_event_id must be a non-negative integer"))
			return
		}
		resumeFrom = &id
	}
	if err := workspaceFromQuery(r); err != nil {
		writeProblem(w, err.(*apiError))
		return
	}
	if _, bearer := bearerToken(r); !bearer && !s.originAllowed(r.Header.Get("Origin")) {
		writeProblem(w, errOriginNotAllowed)
		return
	}
	p, err := s.resolvePrincipal(r.Context(), r, accessMemberOrKey)
	if err == nil {
		err = requireScope(p, oas.ConversationsRead)
	}
	if err != nil {
		var e *apiError
		if errors.As(err, &e) {
			writeProblem(w, e)
			return
		}
		s.log.ErrorContext(r.Context(), "realtime auth", slog.Any("error", err))
		writeProblem(w, errInternal)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	code, reason, err := s.stream(conn, r, p, resumeFrom)
	if err != nil && r.Context().Err() == nil {
		s.log.WarnContext(r.Context(), "realtime stream ended", slog.Any("error", err))
	}
	if code != 0 {
		_ = conn.Close(code, reason)
	}
}

func (s *Server) stream(conn *websocket.Conn, r *http.Request, p principal, resumeFrom *int64) (websocket.StatusCode, string, error) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	sub := s.hub.Subscribe(p.workspaceID)
	defer s.hub.Unsubscribe(sub)
	var connID uuid.UUID
	if !p.isKey() {
		connID = newID()
		memberID := p.memberID
		if err := s.st.OpenConnection(ctx, store.OpenConnectionParams{ID: connID, WorkspaceID: p.workspaceID, MemberID: &memberID, Now: s.now()}); err != nil {
			return websocket.StatusInternalError, "internal", err
		}
		s.presenceHint(ctx, p.workspaceID)
		s.memberPresence(ctx, p.workspaceID, memberID)
		defer func() {
			bg := context.WithoutCancel(ctx)
			if err := s.st.CloseConnection(bg, store.CloseConnectionParams{WorkspaceID: p.workspaceID, ID: connID}); err != nil {
				s.log.WarnContext(bg, "realtime close", slog.Any("error", err))
			}
			s.presenceHint(bg, p.workspaceID)
			s.memberPresence(bg, p.workspaceID, memberID)
		}()
	}
	direct := make(chan realtime.Event, viewersBuffer)
	go s.readMemberFrames(ctx, cancel, conn, p, connID, direct)
	f := &eventFilter{p: p}
	if err := f.reload(ctx, s.st.Queries); err != nil {
		return websocket.StatusInternalError, "internal", err
	}
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
		ok, err := f.allows(ctx, s.st.Queries, e)
		if err != nil || !ok {
			return err
		}
		return send(e)
	}

	last, err := s.st.LastEventID(ctx, p.workspaceID)
	if err != nil {
		return websocket.StatusInternalError, "internal", err
	}
	if resumeFrom != nil {
		known := *resumeFrom == 0
		if !known {
			if known, err = s.st.EventExists(ctx, store.EventExistsParams{WorkspaceID: p.workspaceID, ID: *resumeFrom}); err != nil {
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
				rows, err := s.st.ListEventsAfter(ctx, store.ListEventsAfterParams{WorkspaceID: p.workspaceID, After: last, Lim: replayPageSize})
				if err != nil {
					return websocket.StatusInternalError, "internal", err
				}
				for _, row := range rows {
					last = row.ID
					if err := deliver(realtime.FromRow(row)); err != nil {
						return 0, "", err
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
		case e := <-direct:
			if err := deliver(e); err != nil {
				return 0, "", err
			}
		case e := <-sub.Events():
			if !e.Ephemeral() {
				if e.ID <= last {
					continue
				}
				last = e.ID
			}
			if err := deliver(e); err != nil {
				return 0, "", err
			}
		case <-tick.C:
			pctx, cancel := context.WithTimeout(ctx, realtimeWriteTimeout)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return 0, "", err
			}
			if connID != uuid.Nil() {
				if err := s.st.SeeConnection(ctx, store.SeeConnectionParams{WorkspaceID: p.workspaceID, ID: connID, Now: s.now()}); err != nil {
					s.log.WarnContext(ctx, "realtime presence", slog.Any("error", err))
				}
			}
			next, err := s.resolvePrincipal(ctx, r, accessMemberOrKey)
			var e *apiError
			if errors.As(err, &e) {
				if e.Status == http.StatusUnauthorized {
					return websocket.StatusPolicyViolation, "unauthenticated", nil
				}
				return websocket.StatusPolicyViolation, "forbidden", nil
			}
			if err != nil {
				s.log.WarnContext(ctx, "realtime re-check", slog.Any("error", err))
				continue
			}
			f.p = next
			if err := f.reload(ctx, s.st.Queries); err != nil {
				s.log.WarnContext(ctx, "realtime re-check", slog.Any("error", err))
			}
		}
	}
}

type eventFilter struct {
	p       principal
	inboxes map[uuid.UUID]bool
}

func (f *eventFilter) reload(ctx context.Context, q *store.Queries) error {
	if f.p.seesAllInboxes() {
		f.inboxes = nil
		return nil
	}
	ids, err := q.ListViewerInboxIDs(ctx, store.ListViewerInboxIDsParams{WorkspaceID: f.p.workspaceID, ViewerID: f.p.viewerID()})
	if err != nil {
		return err
	}
	f.inboxes = make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		f.inboxes[id] = true
	}
	return nil
}

func (f *eventFilter) allows(ctx context.Context, q *store.Queries, e realtime.Event) (bool, error) {
	if e.Type == realtime.InboxAccessChanged {
		var ch oas.InboxAccessChange
		if err := json.Unmarshal(e.Data, &ch); err != nil {
			return false, err
		}
		if !f.p.isKey() && ch.MemberId == f.p.memberID {
			return true, f.reload(ctx, q)
		}
		return f.p.seesAllInboxes(), nil
	}
	switch e.Type {
	case realtime.PresenceHint, realtime.ChannelUpdated:
		return false, nil
	case realtime.MemberPresence:
		return !f.p.isKey(), nil
	case realtime.Viewing:
		var v oas.Viewing
		if err := json.Unmarshal(e.Data, &v); err != nil {
			return false, err
		}
		if f.p.isKey() || v.MemberId == f.p.memberID {
			return false, nil
		}
	case realtime.ContactUpdated, realtime.ContactDeleted, realtime.ContactPresence:
		if requireScope(f.p, oas.ContactsRead) != nil {
			return false, nil
		}
	case realtime.Typing:
		var ty oas.Typing
		if err := json.Unmarshal(e.Data, &ty); err != nil {
			return false, err
		}
		if !f.p.isKey() && ty.Author.MemberId != nil && *ty.Author.MemberId == f.p.memberID {
			return false, nil
		}
	}
	if e.Type == realtime.ConversationRead || e.Type == realtime.ConversationPin {
		var r struct {
			MemberID uuid.UUID `json:"member_id"`
		}
		if err := json.Unmarshal(e.Data, &r); err != nil {
			return false, err
		}
		return !f.p.isKey() && r.MemberID == f.p.memberID, nil
	}
	if f.p.seesAllInboxes() {
		return true, nil
	}
	if e.Type == realtime.ContactUpdated {
		var c struct {
			ID uuid.UUID `json:"id"`
		}
		if err := json.Unmarshal(e.Data, &c); err != nil {
			return false, err
		}
		return q.ContactVisibleToViewer(ctx, store.ContactVisibleToViewerParams{WorkspaceID: f.p.workspaceID, ViewerID: f.p.viewerID(), ContactID: c.ID})
	}
	if e.Type == realtime.ContactPresence {
		var c oas.ContactPresence
		if err := json.Unmarshal(e.Data, &c); err != nil {
			return false, err
		}
		return q.ContactVisibleToViewer(ctx, store.ContactVisibleToViewerParams{WorkspaceID: f.p.workspaceID, ViewerID: f.p.viewerID(), ContactID: c.ContactId})
	}
	if e.InboxID == nil {
		return true, nil
	}
	ok := f.inboxes[*e.InboxID]
	if e.Type == realtime.InboxDeleted {
		delete(f.inboxes, *e.InboxID)
	}
	return ok, nil
}

type realtimeClientFrame struct {
	Type           string     `json:"type"`
	ConversationID *uuid.UUID `json:"conversation_id"`
}

// readMemberFrames reads what the panel sends: `viewing` records the conversation a member's
// connection shows (none when the member cannot see it), so notifications about it stay quiet,
// tells the other members who can see it and sends this connection the members already there;
// anything else is ignored. The connection ends when reading fails, and its conversation is then
// left.
func (s *Server) readMemberFrames(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, p principal, connID uuid.UUID, direct chan<- realtime.Event) {
	defer cancel()
	var shown *store.Conversation
	defer func() {
		if shown == nil {
			return
		}
		bg := context.WithoutCancel(ctx)
		if err := s.st.SetConnectionViewing(bg, store.SetConnectionViewingParams{WorkspaceID: p.workspaceID, ID: connID, Now: s.now()}); err != nil {
			s.log.WarnContext(bg, "realtime viewing", slog.Any("error", err))
		}
		s.viewingChanged(bg, *shown, p.memberID)
	}()
	conn.SetReadLimit(realtimeFrameLimit)
	for {
		typ, b, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var f realtimeClientFrame
		if typ != websocket.MessageText || connID == uuid.Nil() || json.Unmarshal(b, &f) != nil || f.Type != "viewing" {
			continue
		}
		prev := shown
		shown = nil
		if f.ConversationID != nil {
			if c, err := visibleConversation(ctx, s.st.Queries, p, *f.ConversationID, false); err == nil {
				shown = &c
			}
		}
		var viewing *uuid.UUID
		if shown != nil {
			viewing = &shown.ID
		}
		if err := s.st.SetConnectionViewing(ctx, store.SetConnectionViewingParams{
			WorkspaceID: p.workspaceID, ID: connID, ConversationID: viewing, Now: s.now(),
		}); err != nil {
			if ctx.Err() == nil {
				s.log.WarnContext(ctx, "realtime viewing", slog.Any("error", err))
			}
			shown = prev
			continue
		}
		if prev != nil && (shown == nil || prev.ID != shown.ID) {
			s.viewingChanged(ctx, *prev, p.memberID)
		}
		if shown == nil {
			continue
		}
		if prev == nil || prev.ID != shown.ID {
			s.viewingChanged(ctx, *shown, p.memberID)
		}
		s.sendViewers(ctx, *shown, p.memberID, direct)
	}
}

const viewersBuffer = 32

// viewingChanged tells the members who can see a conversation whether a member still has it open
// on any connection.
func (s *Server) viewingChanged(ctx context.Context, c store.Conversation, memberID uuid.UUID) {
	viewers, err := s.st.ListViewingMembers(ctx, store.ListViewingMembersParams{WorkspaceID: c.WorkspaceID, ConversationID: &c.ID, FreshAfter: s.now().Add(-presenceFresh)})
	if err != nil {
		s.log.WarnContext(ctx, "realtime viewing", slog.Any("error", err))
		return
	}
	s.signal(ctx, viewingEvent(c, memberID, containsID(viewers, memberID)))
}

// sendViewers gives a connection that opened a conversation the other members already there.
func (s *Server) sendViewers(ctx context.Context, c store.Conversation, memberID uuid.UUID, direct chan<- realtime.Event) {
	viewers, err := s.st.ListViewingMembers(ctx, store.ListViewingMembersParams{WorkspaceID: c.WorkspaceID, ConversationID: &c.ID, FreshAfter: s.now().Add(-presenceFresh)})
	if err != nil {
		s.log.WarnContext(ctx, "realtime viewing", slog.Any("error", err))
		return
	}
	for _, id := range viewers {
		if id == memberID {
			continue
		}
		if ok, err := memberHasInbox(ctx, s.st.Queries, c.WorkspaceID, c.InboxID, id); err != nil || !ok {
			continue
		}
		e := viewingEvent(c, id, true)
		e.CreatedAt = s.now()
		select {
		case direct <- e:
		default:
			return
		}
	}
}

func viewingEvent(c store.Conversation, memberID uuid.UUID, viewing bool) realtime.Event {
	return realtime.Event{
		Type: realtime.Viewing, WorkspaceID: c.WorkspaceID, InboxID: &c.InboxID, ConversationID: &c.ID,
		Data: mustJSON(oas.Viewing{MemberId: memberID, ConversationId: c.ID, Viewing: viewing}),
	}
}

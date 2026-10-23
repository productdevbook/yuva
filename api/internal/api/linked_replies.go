package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const linkedReplyAttempts = 8

type LinkedReplyArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	ReplyID        uuid.UUID `json:"reply_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
}

func (LinkedReplyArgs) Kind() string { return "linked_reply" }

func (LinkedReplyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: linkedReplyAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// StatusReplyArgs is the job kind linked replies had before ideas shared them; it stays for one
// release so jobs queued under it finish.
type StatusReplyArgs LinkedReplyArgs

func (StatusReplyArgs) Kind() string { return "status_reply" }

func (StatusReplyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: linkedReplyAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type linkedReplyWorker struct {
	river.WorkerDefaults[LinkedReplyArgs]
	s *Server
}

func (w *linkedReplyWorker) Work(ctx context.Context, job *river.Job[LinkedReplyArgs]) error {
	return w.s.workLinkedReply(ctx, job.Args, job.Attempt >= job.MaxAttempts)
}

type statusReplyWorker struct {
	river.WorkerDefaults[StatusReplyArgs]
	s *Server
}

func (w *statusReplyWorker) Work(ctx context.Context, job *river.Job[StatusReplyArgs]) error {
	return w.s.workLinkedReply(ctx, LinkedReplyArgs(job.Args), job.Attempt >= job.MaxAttempts)
}

func (s *Server) workLinkedReply(ctx context.Context, a LinkedReplyArgs, last bool) error {
	err := s.RunLinkedReply(ctx, a)
	if err != nil && last {
		s.finishLinkedReply(context.WithoutCancel(ctx), a, outcomeFailed, reasonGaveUp)
	}
	return err
}

// A retried job finds the message it already posted through this client id instead of sending it again.
func linkedReplyClientID(reply uuid.UUID) string { return "status-reply:" + reply.String() }

const (
	outcomePending = "pending"
	outcomeSent    = "sent"
	outcomeSkipped = "skipped"
	outcomeFailed  = "failed"

	reasonClosed      = "closed"
	reasonUnlinked    = "unlinked"
	reasonConvDeleted = "conversation_deleted"
	reasonRevoked     = "revoked"
	reasonGaveUp      = "gave_up"
)

var (
	maxIncidentLinks     = 5000
	maxIdeaLinks         = 5000
	statusReplyBatchSize = 500
)

func errTooManyLinks() *apiError {
	return problem(http.StatusUnprocessableEntity, "too_many_links", "an incident takes at most "+strconv.Itoa(maxIncidentLinks)+" linked conversations")
}

func errTooManyIdeaLinks() *apiError {
	return problem(http.StatusUnprocessableEntity, "too_many_links", "an idea takes at most "+strconv.Itoa(maxIdeaLinks)+" linked conversations")
}

var errLinkedReplyGone = problem(http.StatusNotFound, "not_found", "no such reply")

type linkedReplyTargets struct {
	incident, idea *uuid.UUID
	conversations  []uuid.UUID
}

func (s *Server) createLinkedReply(ctx context.Context, p principal, b *oas.LinkedReplyCreate, resolve func(q *store.Queries) (linkedReplyTargets, error)) (store.LinkedReply, error) {
	var reply store.LinkedReply
	if err := requireScope(p, oas.MessagesWrite); err != nil {
		return reply, err
	}
	closeAfter := b.Close != nil && *b.Close
	if closeAfter {
		if err := requireScope(p, oas.ConversationsWrite); err != nil {
			return reply, err
		}
	}
	if p.deliversAsBot() && !p.botsMaySend {
		return reply, errBotSendingDisabled
	}
	if len([]rune(b.Body)) > maxMessageBodyRunes {
		return reply, errValidation("body must be at most 65536 characters")
	}
	if strings.TrimSpace(b.Body) == "" {
		return reply, errValidation("body is required")
	}
	err := s.st.InTxRaw(ctx, func(tx pgx.Tx, q *store.Queries) error {
		t, err := resolve(q)
		if err != nil {
			return err
		}
		ids := t.conversations
		member, key := statusAuthor(p)
		var grant *uuid.UUID
		if p.isGrant() {
			g := p.grantID
			grant = &g
		}
		reply, err = q.CreateLinkedReply(ctx, store.CreateLinkedReplyParams{
			WorkspaceID: p.workspaceID, ID: newID(), IncidentID: t.incident, IdeaID: t.idea, Body: b.Body, Close: closeAfter, MemberID: member,
			ApiKeyID: key, GrantID: grant, Via: p.via, McpOnly: p.mcpOnly, Queued: int32(len(ids)), CreatedAt: s.now(),
		})
		if err != nil {
			return err
		}
		for len(ids) > 0 {
			batch := ids[:min(statusReplyBatchSize, len(ids))]
			ids = ids[len(batch):]
			if err := q.AddLinkedReplyTargets(ctx, store.AddLinkedReplyTargetsParams{WorkspaceID: p.workspaceID, ReplyID: reply.ID, ConversationIds: batch}); err != nil {
				return err
			}
			jobs := make([]river.InsertManyParams, len(batch))
			for i, id := range batch {
				jobs[i] = river.InsertManyParams{Args: LinkedReplyArgs{WorkspaceID: p.workspaceID, ReplyID: reply.ID, ConversationID: id}}
			}
			if _, err := s.jobs.InsertManyTx(ctx, tx, jobs); err != nil {
				return err
			}
		}
		return nil
	})
	return reply, err
}

func (s *Server) linkedReplyReport(ctx context.Context, p principal, reply store.LinkedReply, limit int32, cursor *string) (oas.LinkedReply, error) {
	_, after, err := decodeCursor(cursor)
	if err != nil {
		return oas.LinkedReply{}, err
	}
	counts, err := s.st.CountLinkedReplyTargets(ctx, store.CountLinkedReplyTargetsParams{
		WorkspaceID: p.workspaceID, ReplyID: reply.ID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return oas.LinkedReply{}, err
	}
	rows, err := s.st.ListLinkedReplyTargetOutcomes(ctx, store.ListLinkedReplyTargetOutcomesParams{
		WorkspaceID: p.workspaceID, ReplyID: reply.ID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), CursorID: after, Lim: limit + 1,
	})
	if err != nil {
		return oas.LinkedReply{}, err
	}
	out := oas.LinkedReply{
		Id: reply.ID, IncidentId: reply.IncidentID, IdeaId: reply.IdeaID, Close: reply.Close, Queued: reply.Queued, CreatedAt: reply.CreatedAt,
		Items: []oas.LinkedReplyTarget{},
	}
	for _, c := range counts {
		switch c.Outcome {
		case outcomePending:
			out.Counts.Pending = c.N
		case outcomeSent:
			out.Counts.Sent = c.N
		case outcomeSkipped:
			out.Counts.Skipped = c.N
		case outcomeFailed:
			out.Counts.Failed = c.N
		}
	}
	if len(rows) > int(limit) {
		rows = rows[:limit]
		cur := encodeCursor(reply.CreatedAt, rows[len(rows)-1].ConversationID)
		out.NextCursor = &cur
	}
	for _, r := range rows {
		out.Items = append(out.Items, oas.LinkedReplyTarget{
			ConversationId: r.ConversationID, Outcome: oas.LinkedReplyOutcome(r.Outcome), Reason: r.Reason, FinishedAt: r.FinishedAt,
		})
	}
	return out, nil
}

func (s *Server) ReplyToStatusIncidentConversations(ctx context.Context, req oas.ReplyToStatusIncidentConversationsRequestObject) (oas.ReplyToStatusIncidentConversationsResponseObject, error) {
	p := principalFrom(ctx)
	reply, err := s.createLinkedReply(ctx, p, req.Body, func(q *store.Queries) (linkedReplyTargets, error) {
		inc, err := visibleStatusIncident(ctx, q, p, req.PageId, req.IncidentId)
		if err != nil {
			return linkedReplyTargets{}, err
		}
		ids, err := q.ListStatusReplyTargets(ctx, store.ListStatusReplyTargetsParams{
			WorkspaceID: p.workspaceID, IncidentID: inc.ID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
		})
		return linkedReplyTargets{incident: &inc.ID, conversations: ids}, err
	})
	if err != nil {
		return nil, err
	}
	return oas.ReplyToStatusIncidentConversations202JSONResponse{Id: reply.ID, Queued: reply.Queued}, nil
}

func (s *Server) GetStatusIncidentReply(ctx context.Context, req oas.GetStatusIncidentReplyRequestObject) (oas.GetStatusIncidentReplyResponseObject, error) {
	p := principalFrom(ctx)
	limit, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	inc, err := visibleStatusIncident(ctx, s.st.Queries, p, req.PageId, req.IncidentId)
	if err != nil {
		return nil, err
	}
	reply, err := s.st.GetIncidentLinkedReply(ctx, store.GetIncidentLinkedReplyParams{WorkspaceID: p.workspaceID, IncidentID: &inc.ID, ID: req.ReplyId})
	if store.IsNotFound(err) {
		return nil, errLinkedReplyGone
	}
	if err != nil {
		return nil, err
	}
	out, err := s.linkedReplyReport(ctx, p, reply, limit, req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	return oas.GetStatusIncidentReply200JSONResponse(out), nil
}

func (s *Server) ReplyToIdeaConversations(ctx context.Context, req oas.ReplyToIdeaConversationsRequestObject) (oas.ReplyToIdeaConversationsResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	reply, err := s.createLinkedReply(ctx, p, req.Body, func(q *store.Queries) (linkedReplyTargets, error) {
		t, err := visibleIdea(ctx, q, p, req.DiscussionId, false)
		if err != nil {
			return linkedReplyTargets{}, err
		}
		ids, err := q.ListIdeaReplyTargets(ctx, store.ListIdeaReplyTargetsParams{
			WorkspaceID: p.workspaceID, IdeaID: t.d.ID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
		})
		return linkedReplyTargets{idea: &t.d.ID, conversations: ids}, err
	})
	if err != nil {
		return nil, err
	}
	return oas.ReplyToIdeaConversations202JSONResponse{Id: reply.ID, Queued: reply.Queued}, nil
}

func (s *Server) GetIdeaLinkedReply(ctx context.Context, req oas.GetIdeaLinkedReplyRequestObject) (oas.GetIdeaLinkedReplyResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	limit, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	t, err := visibleIdea(ctx, s.st.Queries, p, req.DiscussionId, false)
	if err != nil {
		return nil, err
	}
	reply, err := s.st.GetIdeaLinkedReply(ctx, store.GetIdeaLinkedReplyParams{WorkspaceID: p.workspaceID, IdeaID: &t.d.ID, ID: req.ReplyId})
	if store.IsNotFound(err) {
		return nil, errLinkedReplyGone
	}
	if err != nil {
		return nil, err
	}
	out, err := s.linkedReplyReport(ctx, p, reply, limit, req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	return oas.GetIdeaLinkedReply200JSONResponse(out), nil
}

func (s *Server) callerPrincipal(ctx context.Context, ws uuid.UUID, member, apiKey, grantID *uuid.UUID, via string, mcpOnly bool) (principal, bool, error) {
	w, err := s.st.GetWorkspace(ctx, ws)
	if store.IsNotFound(err) || (err == nil && w.DeletedAt != nil) {
		return principal{}, false, nil
	}
	if err != nil {
		return principal{}, false, err
	}
	p := principal{workspaceID: ws, via: via, mcpOnly: mcpOnly, botsMaySend: w.BotsMaySend, broadcastsByKeys: w.BroadcastsByKeys}
	switch {
	case apiKey != nil:
		k, err := s.st.GetAPIKey(ctx, store.GetAPIKeyParams{WorkspaceID: ws, ID: *apiKey})
		if store.IsNotFound(err) {
			return p, false, nil
		}
		if err != nil {
			return p, false, err
		}
		if k.RevokedAt != nil || (k.ExpiresAt != nil && !s.now().Before(*k.ExpiresAt)) {
			return p, false, nil
		}
		p.keyID, p.scopes, p.limited = k.ID, k.Scopes, k.InboxLimited
	case member != nil:
		m, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: ws, ID: *member})
		if store.IsNotFound(err) {
			return p, false, nil
		}
		if err != nil {
			return p, false, err
		}
		p.memberID, p.role = m.ID, m.Role
		if grantID != nil {
			g, err := s.st.GetOAuthGrant(ctx, store.GetOAuthGrantParams{WorkspaceID: ws, ID: *grantID})
			if store.IsNotFound(err) {
				return p, false, nil
			}
			if err != nil {
				return p, false, err
			}
			p.grantID, p.scopes = g.ID, g.Scopes
		}
	default:
		return p, false, nil
	}
	return p, true, nil
}

func (s *Server) finishLinkedReply(ctx context.Context, a LinkedReplyArgs, outcome, reason string) {
	var r *string
	if reason != "" {
		r = &reason
	}
	now := s.now()
	err := s.st.FinishLinkedReplyTarget(ctx, store.FinishLinkedReplyTargetParams{
		WorkspaceID: a.WorkspaceID, ReplyID: a.ReplyID, ConversationID: a.ConversationID, Outcome: outcome, Reason: r, Now: &now,
	})
	if err != nil {
		s.log.WarnContext(ctx, "linked reply outcome not recorded", slog.String("conversation_id", a.ConversationID.String()), slog.Any("error", err))
	}
}

func (s *Server) RunStatusReply(ctx context.Context, a StatusReplyArgs) error {
	return s.RunLinkedReply(ctx, LinkedReplyArgs(a))
}

func (s *Server) RunLinkedReply(ctx context.Context, a LinkedReplyArgs) error {
	pending, err := s.st.LinkedReplyTargetOutcome(ctx, store.LinkedReplyTargetOutcomeParams{WorkspaceID: a.WorkspaceID, ReplyID: a.ReplyID, ConversationID: a.ConversationID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil || pending != outcomePending {
		return err
	}
	outcome, reason, err := s.sendLinkedReply(ctx, a)
	if err != nil {
		return err
	}
	s.finishLinkedReply(ctx, a, outcome, reason)
	return nil
}

func (s *Server) sendLinkedReply(ctx context.Context, a LinkedReplyArgs) (outcome, reason string, err error) {
	reply, err := s.st.GetLinkedReply(ctx, store.GetLinkedReplyParams{WorkspaceID: a.WorkspaceID, ID: a.ReplyID})
	if store.IsNotFound(err) {
		return outcomeSkipped, reasonConvDeleted, nil
	}
	if err != nil {
		return "", "", err
	}
	p, ok, err := s.callerPrincipal(ctx, reply.WorkspaceID, reply.MemberID, reply.ApiKeyID, reply.GrantID, reply.Via, reply.McpOnly)
	if err != nil {
		return "", "", err
	}
	if !ok {
		return outcomeSkipped, reasonRevoked, nil
	}
	source := oas.RoadmapWrite
	if reply.IncidentID != nil {
		source = oas.StatusManage
	}
	needs := []oas.ApiKeyScope{source, oas.MessagesWrite}
	if reply.Close {
		needs = append(needs, oas.ConversationsWrite)
	}
	for _, sc := range needs {
		if err := requireScope(p, sc); err != nil {
			return outcomeFailed, err.(*apiError).Code, nil
		}
	}
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: a.WorkspaceID, ID: a.ConversationID})
	if store.IsNotFound(err) || (err == nil && c.TrashedAt != nil) {
		return outcomeSkipped, reasonConvDeleted, nil
	}
	if err != nil {
		return "", "", err
	}
	if c.Status == string(oas.ConversationStatusClosed) {
		return outcomeSkipped, reasonClosed, nil
	}
	var linked bool
	if reply.IncidentID != nil {
		linked, err = s.st.StatusIncidentConversationLinked(ctx, store.StatusIncidentConversationLinkedParams{
			WorkspaceID: a.WorkspaceID, IncidentID: *reply.IncidentID, ConversationID: a.ConversationID,
		})
	} else {
		linked, err = s.st.IdeaConversationLinked(ctx, store.IdeaConversationLinkedParams{
			WorkspaceID: a.WorkspaceID, IdeaID: *reply.IdeaID, ConversationID: a.ConversationID,
		})
	}
	if err != nil {
		return "", "", err
	}
	if !linked {
		return outcomeSkipped, reasonUnlinked, nil
	}
	clientID := linkedReplyClientID(a.ReplyID)
	actx := context.WithValue(ctx, principalKey, p)
	_, err = s.CreateMessage(actx, oas.CreateMessageRequestObject{ConversationId: a.ConversationID, JSONBody: &oas.CreateMessageJSONRequestBody{
		Kind: oas.MessageCreateKindMessage, Body: &reply.Body, ClientId: &clientID,
	}})
	if err == nil && reply.Close {
		closed := oas.ConversationStatusClosed
		_, err = s.UpdateConversation(actx, oas.UpdateConversationRequestObject{ConversationId: a.ConversationID, Body: &oas.ConversationUpdate{Status: &closed}})
		if ae, ok := errors.AsType[*apiError](err); ok && ae.Code == "field_required" {
			s.log.InfoContext(ctx, "linked reply left the conversation open", slog.String("conversation_id", a.ConversationID.String()), slog.String("field", ae.Field))
			err = nil
		}
	}
	var e *apiError
	if errors.As(err, &e) && e.Status < 500 && e.Status != http.StatusTooManyRequests && e.Status != http.StatusConflict {
		s.log.WarnContext(ctx, "linked reply not sent", slog.String("conversation_id", a.ConversationID.String()), slog.String("code", e.Code))
		return outcomeFailed, e.Code, nil
	}
	if err != nil {
		return "", "", err
	}
	return outcomeSent, "", nil
}

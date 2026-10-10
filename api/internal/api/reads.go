package api

import (
	"context"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

func (s *Server) MarkConversationRead(ctx context.Context, req oas.MarkConversationReadRequestObject) (oas.MarkConversationReadResponseObject, error) {
	p := principalFrom(ctx)
	var out oas.ConversationRead
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := visibleConversation(ctx, q, p, req.ConversationId, false)
		if err != nil {
			return err
		}
		out = oas.ConversationRead{ConversationId: c.ID, MemberId: p.memberID}
		var (
			msgID uuid.UUID
			msgAt time.Time
		)
		if req.Body != nil && req.Body.MessageId != nil {
			m, err := q.GetMessagePosition(ctx, store.GetMessagePositionParams{WorkspaceID: p.workspaceID, ConversationID: c.ID, ID: *req.Body.MessageId})
			if store.IsNotFound(err) {
				return errValidation("message_id must be a message of the conversation")
			}
			if err != nil {
				return err
			}
			msgID, msgAt = m.ID, m.CreatedAt
		} else {
			m, err := q.GetLatestMessagePosition(ctx, store.GetLatestMessagePositionParams{WorkspaceID: p.workspaceID, ConversationID: c.ID})
			if store.IsNotFound(err) {
				cleared, err := q.ClearMarkedUnread(ctx, store.ClearMarkedUnreadParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationID: c.ID})
				if err == nil && cleared > 0 {
					events.conversation(realtime.ConversationRead, c, out)
				}
				return err
			}
			if err != nil {
				return err
			}
			msgID, msgAt = m.ID, m.CreatedAt
		}
		out, err = s.moveReadCursor(ctx, q, events, p.workspaceID, p.memberID, c, msgID, msgAt)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.MarkConversationRead200JSONResponse(out), nil
}

func (s *Server) moveReadCursor(ctx context.Context, q *store.Queries, events *eventBatch, workspaceID, memberID uuid.UUID, c store.Conversation, msgID uuid.UUID, msgAt time.Time) (oas.ConversationRead, error) {
	out := oas.ConversationRead{ConversationId: c.ID, MemberId: memberID}
	read, err := q.MarkConversationRead(ctx, store.MarkConversationReadParams{
		WorkspaceID: workspaceID, MemberID: memberID, ConversationID: c.ID, MessageID: msgID, MessageAt: msgAt, Now: s.now(),
	})
	moved := err == nil
	if store.IsNotFound(err) {
		read, err = q.GetConversationRead(ctx, store.GetConversationReadParams{WorkspaceID: workspaceID, MemberID: memberID, ConversationID: c.ID})
	}
	if err != nil {
		return out, err
	}
	out.LastReadMessageId = &read.LastReadMessageID
	cleared, err := q.ClearMarkedUnread(ctx, store.ClearMarkedUnreadParams{WorkspaceID: workspaceID, MemberID: memberID, ConversationID: c.ID})
	if err != nil {
		return out, err
	}
	unread, err := q.ListUnreadConversations(ctx, store.ListUnreadConversationsParams{WorkspaceID: workspaceID, MemberID: memberID, ConversationIds: []uuid.UUID{c.ID}})
	if err != nil {
		return out, err
	}
	out.Unread = len(unread) > 0
	if moved || cleared > 0 {
		events.conversation(realtime.ConversationRead, c, out)
	}
	return out, nil
}

func (s *Server) MarkConversationUnread(ctx context.Context, req oas.MarkConversationUnreadRequestObject) (oas.MarkConversationUnreadResponseObject, error) {
	p := principalFrom(ctx)
	var out oas.ConversationRead
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := visibleConversation(ctx, q, p, req.ConversationId, false)
		if err != nil {
			return err
		}
		if err := q.MarkConversationUnread(ctx, store.MarkConversationUnreadParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationID: c.ID, Now: s.now()}); err != nil {
			return err
		}
		out = oas.ConversationRead{ConversationId: c.ID, MemberId: p.memberID, Unread: true}
		read, err := q.GetConversationRead(ctx, store.GetConversationReadParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationID: c.ID})
		if err == nil {
			out.LastReadMessageId = &read.LastReadMessageID
		} else if !store.IsNotFound(err) {
			return err
		}
		events.conversation(realtime.ConversationRead, c, out)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.MarkConversationUnread200JSONResponse(out), nil
}

func (s *Server) PinConversation(ctx context.Context, req oas.PinConversationRequestObject) (oas.PinConversationResponseObject, error) {
	p := principalFrom(ctx)
	var out oas.ConversationPin
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := visibleConversation(ctx, q, p, req.ConversationId, false)
		if err != nil {
			return err
		}
		at, err := q.PinConversation(ctx, store.PinConversationParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationID: c.ID, Now: s.now()})
		pinned := err == nil
		if store.IsNotFound(err) {
			at, err = q.GetConversationPin(ctx, store.GetConversationPinParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationID: c.ID})
		}
		if err != nil {
			return err
		}
		out = oas.ConversationPin{ConversationId: c.ID, MemberId: p.memberID, PinnedAt: &at}
		if pinned {
			events.conversation(realtime.ConversationPin, c, out)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.PinConversation200JSONResponse(out), nil
}

func (s *Server) UnpinConversation(ctx context.Context, req oas.UnpinConversationRequestObject) (oas.UnpinConversationResponseObject, error) {
	p := principalFrom(ctx)
	var out oas.ConversationPin
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := visibleConversation(ctx, q, p, req.ConversationId, false)
		if err != nil {
			return err
		}
		out = oas.ConversationPin{ConversationId: c.ID, MemberId: p.memberID}
		n, err := q.UnpinConversation(ctx, store.UnpinConversationParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationID: c.ID})
		if err != nil {
			return err
		}
		if n > 0 {
			events.conversation(realtime.ConversationPin, c, out)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.UnpinConversation200JSONResponse(out), nil
}

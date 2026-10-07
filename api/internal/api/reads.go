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
				return nil
			}
			if err != nil {
				return err
			}
			msgID, msgAt = m.ID, m.CreatedAt
		}
		read, err := q.MarkConversationRead(ctx, store.MarkConversationReadParams{
			WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationID: c.ID, MessageID: msgID, MessageAt: msgAt, Now: s.now(),
		})
		moved := err == nil
		if store.IsNotFound(err) {
			read, err = q.GetConversationRead(ctx, store.GetConversationReadParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationID: c.ID})
		}
		if err != nil {
			return err
		}
		out.LastReadMessageId = &read.LastReadMessageID
		unread, err := q.ListUnreadConversations(ctx, store.ListUnreadConversationsParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationIds: []uuid.UUID{c.ID}})
		if err != nil {
			return err
		}
		out.Unread = len(unread) > 0
		if moved {
			events.conversation(realtime.ConversationRead, c, out)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.MarkConversationRead200JSONResponse(out), nil
}

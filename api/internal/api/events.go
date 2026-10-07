package api

import (
	"context"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

type pendingEvent struct {
	typ            string
	inboxID        *uuid.UUID
	conversationID *uuid.UUID
	data           []byte
}

type eventBatch struct {
	items []pendingEvent
}

func (b *eventBatch) add(typ string, inboxID, conversationID *uuid.UUID, data any) {
	b.items = append(b.items, pendingEvent{typ: typ, inboxID: inboxID, conversationID: conversationID, data: mustJSON(data)})
}

func (b *eventBatch) conversation(typ string, c store.Conversation, data any) {
	inbox, conv := c.InboxID, c.ID
	b.add(typ, &inbox, &conv, data)
}

// inTx writes the queued events last, so the per-workspace event lock is the final lock a
// transaction takes and event ids follow commit order within a workspace.
func (s *Server) inTx(ctx context.Context, workspaceID uuid.UUID, fn func(q *store.Queries, ev *eventBatch) error) error {
	return s.st.InTx(ctx, func(q *store.Queries) error {
		var ev eventBatch
		if err := fn(q, &ev); err != nil {
			return err
		}
		return writeEvents(ctx, q, workspaceID, ev.items)
	})
}

func writeEvents(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, items []pendingEvent) error {
	if len(items) == 0 {
		return nil
	}
	if err := q.LockEventStream(ctx, workspaceID); err != nil {
		return err
	}
	for _, e := range items {
		id, err := q.InsertEvent(ctx, store.InsertEventParams{
			WorkspaceID: workspaceID, Type: e.typ, InboxID: e.inboxID, ConversationID: e.conversationID, Payload: e.data,
		})
		if err != nil {
			return err
		}
		if err := q.NotifyEvent(ctx, store.NotifyEventParams{Channel: realtime.Channel, WorkspaceID: workspaceID, ID: id}); err != nil {
			return err
		}
	}
	return nil
}

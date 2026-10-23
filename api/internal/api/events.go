package api

import (
	"context"
	"slices"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

type pendingEvent struct {
	typ            string
	inboxID        *uuid.UUID
	conversationID *uuid.UUID
	data           []byte
	deleted        *deletedContact
}

type boardRef struct{ ws, inbox, board uuid.UUID }

type eventBatch struct {
	items []pendingEvent
	jobs  []river.InsertManyParams
	// boards are roadmap boards whose public side changed; open panels hear of it after the commit.
	boards []boardRef
	// quiet drops events and keeps jobs: imports write no event or webhook per row.
	quiet bool
	// rule is the automation rule whose run wrote the batch; its events never trigger a rule.
	rule *oas.AutomationRuleRef
}

func (b *eventBatch) job(args river.JobArgs, opts *river.InsertOpts) {
	b.jobs = append(b.jobs, river.InsertManyParams{Args: args, InsertOpts: opts})
}

// jobOnce queues a job unless the batch already holds one with the same arguments; River refuses
// two unique jobs with equal arguments in one insert.
func (b *eventBatch) jobOnce(args river.JobArgs, opts *river.InsertOpts) {
	for _, j := range b.jobs {
		if j.Args == args {
			return
		}
	}
	b.job(args, opts)
}

func (b *eventBatch) add(typ string, inboxID, conversationID *uuid.UUID, data any) {
	if b.quiet {
		return
	}
	b.items = append(b.items, pendingEvent{typ: typ, inboxID: inboxID, conversationID: conversationID, data: mustJSON(data)})
}

// conversationAt queues a conversation event at position i, before events queued after i.
func (b *eventBatch) conversationAt(i int, typ string, c store.Conversation, data any) {
	if b.quiet {
		return
	}
	inbox, conv := c.InboxID, c.ID
	b.items = slices.Insert(b.items, i, pendingEvent{typ: typ, inboxID: &inbox, conversationID: &conv, data: mustJSON(data)})
}

func (b *eventBatch) conversation(typ string, c store.Conversation, data any) {
	inbox, conv := c.InboxID, c.ID
	b.add(typ, &inbox, &conv, data)
}

// inTx writes the queued events last, so the per-workspace event lock is the final lock a
// transaction takes and event ids follow commit order within a workspace.
func (s *Server) inTx(ctx context.Context, workspaceID uuid.UUID, fn func(q *store.Queries, ev *eventBatch) error) error {
	var boards []boardRef
	err := s.st.InTxRaw(ctx, func(tx pgx.Tx, q *store.Queries) error {
		var ev eventBatch
		if err := fn(q, &ev); err != nil {
			return err
		}
		if err := s.syncSLA(ctx, q, workspaceID, &ev); err != nil {
			return err
		}
		boards = ev.boards
		if len(ev.jobs) > 0 {
			if _, err := s.jobs.InsertManyTx(ctx, tx, ev.jobs); err != nil {
				return err
			}
		}
		ids, err := writeEvents(ctx, q, workspaceID, ev.items)
		if err != nil {
			return err
		}
		if err := s.queueWebhooks(ctx, tx, q, workspaceID, ev.items, ids); err != nil {
			return err
		}
		if err := s.queueAutomations(ctx, tx, q, workspaceID, &ev, ids); err != nil {
			return err
		}
		return s.queueNotifications(ctx, tx, workspaceID, ev.items)
	})
	if err == nil {
		seen := map[uuid.UUID]bool{}
		for _, b := range boards {
			if !seen[b.board] {
				seen[b.board] = true
				s.boardSignal(ctx, b.ws, b.inbox, b.board)
			}
		}
	}
	return err
}

func writeEvents(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, items []pendingEvent) ([]int64, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if err := q.LockEventStream(ctx, workspaceID); err != nil {
		return nil, err
	}
	ids := make([]int64, len(items))
	for i, e := range items {
		id, err := q.InsertEvent(ctx, store.InsertEventParams{
			WorkspaceID: workspaceID, Type: e.typ, InboxID: e.inboxID, ConversationID: e.conversationID, Payload: e.data,
		})
		if err != nil {
			return nil, err
		}
		if err := q.NotifyEvent(ctx, store.NotifyEventParams{Channel: realtime.Channel, WorkspaceID: workspaceID, ID: id}); err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}

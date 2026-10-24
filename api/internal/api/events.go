package api

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
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
	contactID      *uuid.UUID
	data           []byte
	deleted        *deletedContact
	// noRules marks an event a rule caused later, such as the end of a snooze it set.
	noRules bool
	// bounce marks a delivery report ingress trusted (null envelope sender, DSN); it runs no mail filter.
	bounce bool
	// onlyMember: the conversation is still this member's draft; no one else, no webhook, rule or notification.
	onlyMember *uuid.UUID
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
	// assign marks a batch automatic assignment wrote; it frees no member for waiting conversations.
	assign bool
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
	b.addFor(typ, inboxID, conversationID, nil, data)
}

func (b *eventBatch) addFor(typ string, inboxID, conversationID, onlyMember *uuid.UUID, data any) {
	if b.quiet {
		return
	}
	raw := mustJSON(data)
	b.items = append(b.items, pendingEvent{typ: typ, inboxID: inboxID, conversationID: conversationID, contactID: eventContact(typ, data, raw), data: raw, onlyMember: onlyMember})
}

// eventContact is the contact an event is about: the contact of a contact event, the author of a
// contact's message.
func eventContact(typ string, data any, raw []byte) *uuid.UUID {
	switch m := data.(type) {
	case oas.Message:
		return m.Author.ContactId
	case *oas.Message:
		return m.Author.ContactId
	}
	if !strings.HasPrefix(typ, "contact.") {
		return nil
	}
	var ref struct {
		ID *uuid.UUID `json:"id"`
	}
	_ = json.Unmarshal(raw, &ref)
	return ref.ID
}

// conversationAt queues a conversation event at position i, before events queued after i.
func (b *eventBatch) conversationAt(i int, typ string, c store.Conversation, data any) {
	if b.quiet {
		return
	}
	inbox, conv := c.InboxID, c.ID
	raw := mustJSON(data)
	b.items = slices.Insert(b.items, i, pendingEvent{typ: typ, inboxID: &inbox, conversationID: &conv, contactID: eventContact(typ, data, raw), data: raw, onlyMember: c.DraftOf})
}

func (b *eventBatch) conversation(typ string, c store.Conversation, data any) {
	inbox, conv := c.InboxID, c.ID
	b.addFor(typ, &inbox, &conv, c.DraftOf, data)
}

// inTx writes the queued events last, so the per-workspace event lock is the final lock a
// transaction takes and event ids follow commit order within a workspace.
func (s *Server) inTx(ctx context.Context, workspaceID uuid.UUID, fn func(q *store.Queries, ev *eventBatch) error) error {
	return s.inTxWith(ctx, workspaceID, func(_ pgx.Tx, q *store.Queries, ev *eventBatch) error { return fn(q, ev) })
}

// inTxWith is inTx for work that also needs the transaction itself.
func (s *Server) inTxWith(ctx context.Context, workspaceID uuid.UUID, fn func(tx pgx.Tx, q *store.Queries, ev *eventBatch) error) error {
	var boards []boardRef
	err := s.st.InTxRaw(ctx, func(tx pgx.Tx, q *store.Queries) error {
		var ev eventBatch
		if err := fn(tx, q, &ev); err != nil {
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
		if err := s.queueAssignWaiting(ctx, tx, q, workspaceID, &ev); err != nil {
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
			WorkspaceID: workspaceID, Type: e.typ, InboxID: e.inboxID, ConversationID: e.conversationID, ContactID: e.contactID, Payload: e.data,
			OnlyMember: e.onlyMember,
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

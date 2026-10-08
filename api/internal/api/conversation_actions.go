package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxBulkConversations = 100

var errNoEmailChannel = problem(http.StatusConflict, "no_email_channel", "the inbox has no e-mail channel to continue this e-mail conversation")

func (s *Server) BulkUpdateConversations(ctx context.Context, req oas.BulkUpdateConversationsRequestObject) (oas.BulkUpdateConversationsResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	if len(b.ConversationIds) == 0 || len(b.ConversationIds) > maxBulkConversations {
		return nil, errValidation("conversation_ids must name 1 to 100 conversations")
	}
	if b.Status == nil && b.SnoozeUntil == nil && !b.AssigneeId.IsSpecified() && b.AddLabels == nil && b.RemoveLabels == nil {
		return nil, errValidation("name at least one change: status, assignee_id, add_labels or remove_labels")
	}
	upd := oas.ConversationUpdate{Status: b.Status, SnoozeUntil: b.SnoozeUntil, AssigneeId: b.AssigneeId}
	if b.Status != nil && !b.Status.Valid() {
		return nil, errValidation("status must be open, pending, snoozed or closed")
	}
	snoozed := b.Status != nil && *b.Status == oas.ConversationStatusSnoozed
	if snoozed && (b.SnoozeUntil == nil || !b.SnoozeUntil.After(s.now())) {
		return nil, errValidation("snoozed needs a snooze_until in the future")
	}
	if !snoozed && b.SnoozeUntil != nil {
		return nil, errValidation("snooze_until is only allowed with status snoozed")
	}
	if b.AssigneeId.IsSpecified() && !b.AssigneeId.IsNull() {
		if _, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: p.workspaceID, ID: b.AssigneeId.MustGet()}); store.IsNotFound(err) {
			return nil, errValidation("assignee_id must be a member of the workspace")
		} else if err != nil {
			return nil, err
		}
	}
	var add, remove []uuid.UUID
	var err error
	if b.AddLabels != nil {
		if add, err = labelSet(ctx, s.st.Queries, p.workspaceID, *b.AddLabels); err != nil {
			return nil, err
		}
	}
	if b.RemoveLabels != nil {
		if remove, err = labelSet(ctx, s.st.Queries, p.workspaceID, *b.RemoveLabels); err != nil {
			return nil, err
		}
	}
	if slices.ContainsFunc(add, func(l uuid.UUID) bool { return slices.Contains(remove, l) }) {
		return nil, errValidation("a label cannot be both added and removed")
	}
	out := oas.ConversationBulkResult{Updated: []oas.Conversation{}, Failed: []oas.ConversationBulkFailure{}}
	seen := map[uuid.UUID]bool{}
	for _, id := range b.ConversationIds {
		if seen[id] {
			continue
		}
		seen[id] = true
		var c oas.Conversation
		err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
			var err error
			c, err = s.changeConversation(ctx, q, events, p, id, upd, add, remove)
			return err
		})
		var ae *apiError
		switch {
		case err == nil:
			out.Updated = append(out.Updated, c)
		case errors.As(err, &ae) && ae.Status < http.StatusInternalServerError:
			f := oas.ConversationBulkFailure{Id: id, Status: int32(ae.Status), Code: ae.Code}
			if ae.Detail != "" {
				d := ae.Detail
				f.Detail = &d
			}
			out.Failed = append(out.Failed, f)
		default:
			return nil, err
		}
	}
	return oas.BulkUpdateConversations200JSONResponse(out), nil
}

func (s *Server) MoveConversation(ctx context.Context, req oas.MoveConversationRequestObject) (oas.MoveConversationResponseObject, error) {
	p := principalFrom(ctx)
	var out oas.Conversation
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := visibleConversation(ctx, q, p, req.ConversationId, true)
		if err != nil {
			return err
		}
		target, err := visibleInbox(ctx, q, p, req.Body.InboxId)
		if err != nil {
			return err
		}
		if target.ID == cur.InboxID {
			return errValidation("the conversation is already in this inbox")
		}
		next := cur
		next.InboxID = target.ID
		next.ChannelID = nil
		if cur.ChannelID != nil {
			ch, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: p.workspaceID, ID: *cur.ChannelID})
			if err != nil {
				return err
			}
			if ch.Kind == string(oas.ChannelKindEmail) {
				ech, err := q.InboxEmailChannel(ctx, store.InboxEmailChannelParams{WorkspaceID: p.workspaceID, InboxID: target.ID})
				if store.IsNotFound(err) {
					return errNoEmailChannel
				}
				if err != nil {
					return err
				}
				if _, err := sendingAddress(ech, next); err != nil {
					return err
				}
				next.ChannelID = &ech.ChannelID
			} else {
				same, err := q.InboxChannelOfKind(ctx, store.InboxChannelOfKindParams{WorkspaceID: p.workspaceID, InboxID: target.ID, Kind: ch.Kind})
				if err != nil && !store.IsNotFound(err) {
					return err
				}
				if err == nil {
					next.ChannelID = &same.ID
				}
			}
		}
		if cur.AssigneeID != nil {
			ok, err := memberHasInbox(ctx, q, p.workspaceID, target.ID, *cur.AssigneeID)
			if err != nil {
				return err
			}
			if !ok {
				next.AssigneeID = nil
			}
		}
		now := s.now()
		moved, err := q.MoveConversation(ctx, store.MoveConversationParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, InboxID: target.ID, ChannelID: next.ChannelID, AssigneeID: next.AssigneeID, Now: now,
		})
		if err != nil {
			return err
		}
		if out, err = oneConversation(ctx, q, moved); err != nil {
			return err
		}
		from := cur.InboxID
		events.add(realtime.ConversationMoved, &from, &moved.ID, oas.ConversationMoved{Id: moved.ID, InboxId: target.ID, PreviousInboxId: from})
		events.conversation(realtime.ConversationUpdated, moved, out)
		if err := s.writeEvent(ctx, q, events, p, moved, oas.MessageEvent{Type: oas.Moved, InboxId: &target.ID, PreviousInboxId: &from}, now); err != nil {
			return err
		}
		return s.recordChanges(ctx, q, events, p, cur, moved, nil, nil, now)
	})
	if err != nil {
		return nil, err
	}
	return oas.MoveConversation200JSONResponse(out), nil
}

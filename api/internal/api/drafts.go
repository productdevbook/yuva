package api

import (
	"context"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

var errNotADraft = problem(http.StatusConflict, "not_a_draft", "the message is not a draft")

// dispatchOutgoing hands a delivered outgoing message to e-mail (plan) or, for a member's reply in
// a chat that continues by e-mail, to continuity.
func (s *Server) dispatchOutgoing(ctx context.Context, q *store.Queries, events *eventBatch, p principal, c store.Conversation, plan *emailPlan, msg messageRow, now time.Time) (*store.ListMessageEmailsRow, error) {
	if plan != nil {
		return s.queueEmail(ctx, q, events, plan, msg)
	}
	if p.isKey() || c.ChannelID == nil {
		return nil, nil
	}
	ch, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: p.workspaceID, ID: *c.ChannelID})
	if err != nil {
		return nil, err
	}
	if emailsReplies(c, ch.Kind) {
		s.scheduleContinuity(events, c, now)
	}
	return nil, nil
}

// lockDraft finds a message the caller can see, locks its conversation and the message, and
// refuses anything that is not a draft.
func lockDraft(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (store.Conversation, error) {
	m, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return store.Conversation{}, errMessageGone
	}
	if err != nil {
		return store.Conversation{}, err
	}
	c, err := visibleConversation(ctx, q, p, m.ConversationID, true)
	if err == errConversationGone {
		return c, errMessageGone
	}
	if err != nil {
		return c, err
	}
	lm, err := q.LockMessage(ctx, store.LockMessageParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return c, errMessageGone
	}
	if err != nil {
		return c, err
	}
	if !lm.Draft {
		return c, errNotADraft
	}
	return c, nil
}

func (s *Server) UpdateMessage(ctx context.Context, req oas.UpdateMessageRequestObject) (oas.UpdateMessageResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	var out oas.Message
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := lockDraft(ctx, q, p, req.MessageId)
		if err != nil {
			return err
		}
		cur, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: p.workspaceID, ID: req.MessageId})
		if err != nil {
			return err
		}
		body, html := cur.Body, cur.Html
		if b.Body != nil {
			body = *b.Body
		}
		if b.Html.IsSpecified() {
			html = nil
			if !b.Html.IsNull() {
				v := b.Html.MustGet()
				if len(v) > maxMessageHTMLBytes {
					return errValidation("html must be at most 256 KiB")
				}
				if clean := strings.TrimSpace(s.sanitize.Sanitize(v)); clean != "" {
					html = &clean
				}
			}
		}
		atts, err := messageAttachments(ctx, q, p.workspaceID, []uuid.UUID{cur.ID})
		if err != nil {
			return err
		}
		if len([]rune(body)) > maxMessageBodyRunes {
			return errValidation("body must be at most 65536 characters")
		}
		if strings.TrimSpace(body) == "" && len(atts[cur.ID]) == 0 {
			return errValidation("body is required unless files are attached")
		}
		m, err := q.UpdateDraft(ctx, store.UpdateDraftParams{WorkspaceID: p.workspaceID, ID: cur.ID, Body: body, Html: html})
		if err != nil {
			return err
		}
		out = messageBody(messageRow(m), atts[cur.ID])
		events.conversation(realtime.DraftUpdated, c, out)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateMessage200JSONResponse(out), nil
}

func (s *Server) DeleteMessage(ctx context.Context, req oas.DeleteMessageRequestObject) (oas.DeleteMessageResponseObject, error) {
	p := principalFrom(ctx)
	var keys []string
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := lockDraft(ctx, q, p, req.MessageId)
		if err != nil {
			return err
		}
		m, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: p.workspaceID, ID: req.MessageId})
		if err != nil {
			return err
		}
		atts, err := q.ListAttachmentsOfMessage(ctx, store.ListAttachmentsOfMessageParams{WorkspaceID: p.workspaceID, MessageID: m.ID})
		if err != nil {
			return err
		}
		for _, a := range atts {
			keys = append(keys, a.StorageKey)
		}
		if _, err := q.DeleteDraft(ctx, store.DeleteDraftParams{WorkspaceID: p.workspaceID, ID: m.ID}); err != nil {
			return err
		}
		events.conversation(realtime.DraftDeleted, c, messageBody(messageRow(m), atts))
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.deleteObjects(ctx, keys)
	return oas.DeleteMessage204Response{}, nil
}

func (s *Server) SendMessage(ctx context.Context, req oas.SendMessageRequestObject) (oas.SendMessageResponseObject, error) {
	p := principalFrom(ctx)
	if p.isKey() && !p.botsMaySend {
		return nil, errBotSendingDisabled
	}
	var out oas.Message
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := lockDraft(ctx, q, p, req.MessageId)
		if err != nil {
			return err
		}
		plan, err := s.planEmail(ctx, q, c)
		if err != nil {
			return err
		}
		now := s.now()
		arg := store.SendDraftParams{WorkspaceID: p.workspaceID, ID: req.MessageId, Now: now}
		if p.isKey() {
			arg.SentByApiKeyID = &p.keyID
		} else {
			arg.SentByMemberID = &p.memberID
		}
		if plan != nil {
			queued := deliveryQueued
			arg.DeliveryState = &queued
		}
		sent, err := q.SendDraft(ctx, arg)
		if err != nil {
			return err
		}
		msg := messageRow(sent)
		summary, err := s.dispatchOutgoing(ctx, q, events, p, c, plan, msg, now)
		if err != nil {
			return err
		}
		atts, err := q.ListAttachmentsOfMessage(ctx, store.ListAttachmentsOfMessageParams{WorkspaceID: p.workspaceID, MessageID: msg.ID})
		if err != nil {
			return err
		}
		var total int64
		for _, a := range atts {
			total += a.SizeBytes
		}
		if err := q.TouchConversation(ctx, store.TouchConversationParams{WorkspaceID: p.workspaceID, ID: c.ID, Now: now, IsMessage: true}); err != nil {
			return err
		}
		if err := s.addUsage(ctx, q, p.workspaceID, 0, 1, total); err != nil {
			return err
		}
		out = withEmail(messageBody(msg, atts), summary)
		events.conversation(realtime.MessageCreated, c, out)
		if !p.isKey() {
			if _, err := s.moveReadCursor(ctx, q, events, p.workspaceID, p.memberID, c, msg.ID, msg.CreatedAt); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.SendMessage200JSONResponse(out), nil
}

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

// draftHidden mirrors draft_visible_to in SQL: a member's draft is seen only by its author.
func draftHidden(p principal, m store.GetMessageRow) bool {
	if !m.Draft || m.AuthorType != string(oas.AuthorTypeMember) {
		return false
	}
	viewer := p.memberRef()
	return viewer == nil || m.AuthorMemberID == nil || *m.AuthorMemberID != *viewer
}

// dispatchOutgoing hands a delivered outgoing message to e-mail (plan) or, for a member's reply in
// a chat that continues by e-mail, to continuity.
func (s *Server) dispatchOutgoing(ctx context.Context, q *store.Queries, events *eventBatch, p principal, c store.Conversation, plan *emailPlan, msg messageRow, now time.Time) (*store.ListMessageEmailsRow, error) {
	if plan != nil {
		return s.queueEmail(ctx, q, events, plan, msg)
	}
	if err := s.scheduleAppPush(ctx, q, events, c); err != nil {
		return nil, err
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
	if draftHidden(p, m) {
		return store.Conversation{}, errMessageGone
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
	if at, ok := ctx.Value(scheduledSendKey).(time.Time); ok && (lm.ScheduledAt == nil || !lm.ScheduledAt.Equal(at)) {
		return c, errScheduleStale
	}
	return c, nil
}

func (s *Server) UpdateMessage(ctx context.Context, req oas.UpdateMessageRequestObject) (oas.UpdateMessageResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	var sendAt time.Time
	if b.SendAt != nil {
		var err error
		if sendAt, err = s.checkSendAt(p, *b.SendAt); err != nil {
			return nil, err
		}
	}
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
		forward := cur.Kind == string(oas.MessageKindForward)
		if forward && b.Html.IsSpecified() && !b.Html.IsNull() {
			return errForwardNoteFormat
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
		if strings.TrimSpace(body) == "" && len(atts[cur.ID]) == 0 && !forward {
			return errValidation("body is required unless files are attached")
		}
		m, err := q.UpdateDraft(ctx, store.UpdateDraftParams{WorkspaceID: p.workspaceID, ID: cur.ID, Body: body, Html: html})
		if err != nil {
			return err
		}
		row := messageRow(m)
		if b.SendAt != nil {
			if err := ownDraft(p, cur); err != nil {
				return err
			}
			r, err := q.RescheduleDraft(ctx, store.RescheduleDraftParams{WorkspaceID: p.workspaceID, ID: cur.ID, SendAt: sendAt, MemberID: p.memberID})
			if err != nil {
				return err
			}
			row = messageRow(r)
			queueScheduledSend(events, p.workspaceID, cur.ID, sendAt)
		}
		out = messageBody(row, atts[cur.ID])
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
		if c.DraftOf != nil {
			keys, err = s.deleteDraftConversation(ctx, q, events, c)
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

func (s *Server) deleteDraftConversation(ctx context.Context, q *store.Queries, events *eventBatch, c store.Conversation) ([]string, error) {
	ids := []uuid.UUID{c.ID}
	if err := forgetConversations(ctx, q, c.WorkspaceID, ids); err != nil {
		return nil, err
	}
	keys, err := q.ListConversationsStorageKeys(ctx, store.ListConversationsStorageKeysParams{WorkspaceID: c.WorkspaceID, Ids: ids})
	if err != nil {
		return nil, err
	}
	raw, err := q.ListConversationsRawKeys(ctx, store.ListConversationsRawKeysParams{WorkspaceID: c.WorkspaceID, Ids: ids})
	if err != nil {
		return nil, err
	}
	if _, err := q.DeleteConversations(ctx, store.DeleteConversationsParams{WorkspaceID: c.WorkspaceID, Ids: ids}); err != nil {
		return nil, err
	}
	events.addFor(realtime.ConversationDeleted, &c.InboxID, &c.ID, c.DraftOf, oas.ConversationDeleted{Id: c.ID, InboxId: c.InboxID, ContactId: c.ContactID})
	return append(keys, raw...), nil
}

func (s *Server) SendMessage(ctx context.Context, req oas.SendMessageRequestObject) (oas.SendMessageResponseObject, error) {
	p := principalFrom(ctx)
	if p.deliversAsBot() && !p.botsMaySend {
		return nil, errBotSendingDisabled
	}
	var cc, bcc copyList
	var from *string
	if req.Body != nil {
		cc, bcc = copiesOf(req.Body.Cc), copiesOf(req.Body.Bcc)
		from = (*string)(req.Body.FromAddress)
	}
	if from != nil && (p.isKey() || p.deliversAsBot()) {
		return nil, errFromNotMember
	}
	if req.Body != nil && req.Body.SendAt != nil {
		return s.scheduleDraft(ctx, p, req.MessageId, *req.Body.SendAt, cc, bcc, from)
	}
	if m, err := s.st.GetMessage(ctx, store.GetMessageParams{WorkspaceID: p.workspaceID, ID: req.MessageId}); err == nil && m.Kind == string(oas.MessageKindForward) {
		if from != nil {
			return nil, errValidation("a forward goes from the channel's address and takes no from_address")
		}
		return s.sendForwardDraft(ctx, p, req)
	} else if err == nil && m.Draft {
		if d, err := s.st.GetComposedDraft(ctx, store.GetComposedDraftParams{WorkspaceID: p.workspaceID, MessageID: m.ID}); err == nil {
			if from != nil {
				if err := s.checkDraftFrom(ctx, p, m.ConversationID, *from); err != nil {
					return nil, err
				}
			}
			to, ccList, bccList := composedDraftLists(d, cc, bcc)
			if err := s.takeComposeRecipients(ctx, p, len(to)+len(ccList)+len(bccList)); err != nil {
				return nil, err
			}
		}
	}
	var out oas.Message
	var inbox uuid.UUID
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := lockDraft(ctx, q, p, req.MessageId)
		if err != nil {
			return err
		}
		inbox = c.InboxID
		var plan *emailPlan
		composed, err := q.GetComposedDraft(ctx, store.GetComposedDraftParams{WorkspaceID: p.workspaceID, MessageID: req.MessageId})
		isComposed := err == nil
		switch {
		case isComposed:
			plan, err = s.composedDraftPlan(ctx, q, c, composed, cc, bcc)
		case store.IsNotFound(err):
			plan, err = s.planEmail(ctx, q, c)
		}
		if err != nil {
			return err
		}
		if isComposed {
			if err := addComposedRecipients(ctx, q, events, c, plan, s.now()); err != nil {
				return err
			}
		} else if plan != nil {
			if err := s.setCopies(ctx, q, c, plan, cc, bcc); err != nil {
				return err
			}
		} else if cc.set || bcc.set {
			return errCopiesNotEmailed
		}
		if plan == nil && from != nil {
			return errFromNotEmailed
		}
		if plan != nil {
			if err := s.applyFromAddress(ctx, q, p, plan, from); err != nil {
				return err
			}
		}
		now := s.now()
		arg := store.SendDraftParams{WorkspaceID: p.workspaceID, ID: req.MessageId, Now: now, SentVia: p.viaClient()}
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
		if c, err = revealDraftConversation(ctx, q, events, c); err != nil {
			return err
		}
		summary, err := s.dispatchOutgoing(ctx, q, events, p, c, plan, msg, now)
		if err != nil {
			return err
		}
		if err := keepCopiesAsParticipants(ctx, q, events, c, plan, now); err != nil {
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
		out = withBcc(out, summary)
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
	if out.Cards != nil {
		s.countHelpSent(ctx, p.workspaceID, inbox, *out.Cards)
	}
	return oas.SendMessage200JSONResponse(out), nil
}

func (s *Server) ListDrafts(ctx context.Context, req oas.ListDraftsRequestObject) (oas.ListDraftsResponseObject, error) {
	p := principalFrom(ctx)
	prm := req.Params
	lim, err := pageSize(prm.Limit)
	if err != nil {
		return nil, err
	}
	at, cid, err := decodeCursor(prm.Cursor)
	if err != nil {
		return nil, err
	}
	var kind *string
	if prm.Author != nil {
		if !prm.Author.Valid() {
			return nil, errValidation("author must be bot, assistant or member")
		}
		k := string(*prm.Author)
		kind = &k
	}
	if prm.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *prm.InboxId); err != nil {
			return nil, err
		}
	}
	mine := prm.Mine != nil && *prm.Mine
	rows, err := s.st.ListDrafts(ctx, store.ListDraftsParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), ViewerMemberID: p.memberRef(),
		InboxID: prm.InboxId, ConversationID: prm.ConversationId, Mine: mine, Scheduled: prm.Scheduled,
		AuthorKind: kind, CursorAt: at, CursorID: cid, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	total, err := s.st.CountDrafts(ctx, store.CountDraftsParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), ViewerMemberID: p.memberRef(),
		InboxID: prm.InboxId, ConversationID: prm.ConversationId, Mine: mine, Scheduled: prm.Scheduled, AuthorKind: kind,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		next = &c
	}
	ids := make([]uuid.UUID, len(rows))
	convIDs := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i], convIDs[i] = r.ID, r.ConversationID
	}
	atts, err := messageAttachments(ctx, s.st.Queries, p.workspaceID, ids)
	if err != nil {
		return nil, err
	}
	convs, err := s.conversationSummaries(ctx, p.workspaceID, convIDs)
	if err != nil {
		return nil, err
	}
	items := make([]oas.DraftItem, len(rows))
	for i, r := range rows {
		items[i] = oas.DraftItem{Draft: messageBody(messageRow(r), atts[r.ID]), Conversation: convs[r.ConversationID]}
	}
	return oas.ListDrafts200JSONResponse{Items: items, NextCursor: next, Total: total}, nil
}

func (s *Server) checkDraftFrom(ctx context.Context, p principal, conv uuid.UUID, from string) error {
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: p.workspaceID, ID: conv})
	if err != nil {
		return err
	}
	if c.ChannelID == nil {
		return errComposeNoChannel
	}
	ch, err := s.st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: p.workspaceID, ChannelID: *c.ChannelID})
	if store.IsNotFound(err) {
		return errComposeNoChannel
	}
	if err != nil {
		return err
	}
	_, err = s.fromMemberAddress(ctx, s.st.Queries, p, ch, from)
	return err
}

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

var (
	limitComposePerActor    = limit{60, time.Minute}
	limitComposeRecipients  = limit{300, time.Hour}
	errComposeNoChannel     = problem(http.StatusConflict, "email_not_configured", "the inbox has no e-mail channel with an SMTP account")
	errComposeChannel       = errValidation("channel_id must be an e-mail channel of the inbox")
	errComposeNoRecipient   = problem(http.StatusConflict, "no_recipient", "the draft has no To address left to send to; its recipients were erased")
	errComposeRepeated      = errors.New("client_id stored by a concurrent request")
	composeRecipientsBudget = fmt.Sprintf("new mail goes to at most %d recipients an hour per member or key", limitComposeRecipients.n)
)

var composeFormFields = []string{"inbox_id", "channel_id", "to", "cc", "bcc", "subject", "body", "html", "signature", "draft", "client_id", "from_address", "send_at"}

// composeChannel is the e-mail channel a new mail of the inbox goes out through: the one named, or
// the inbox's first with an SMTP account, an exact address before a catch-all.
func composeChannel(ctx context.Context, q *store.Queries, ws, inboxID uuid.UUID, channelID *uuid.UUID) (store.EmailChannel, error) {
	if channelID == nil {
		ch, err := q.InboxSendingEmailChannel(ctx, store.InboxSendingEmailChannelParams{WorkspaceID: ws, InboxID: inboxID})
		if store.IsNotFound(err) {
			return ch, errComposeNoChannel
		}
		return ch, err
	}
	row, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: ws, ID: *channelID})
	if store.IsNotFound(err) || (err == nil && row.InboxID != inboxID) {
		return store.EmailChannel{}, errComposeChannel
	}
	if err != nil {
		return store.EmailChannel{}, err
	}
	ch, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws, ChannelID: *channelID})
	if store.IsNotFound(err) {
		return ch, errComposeChannel
	}
	if err != nil {
		return ch, err
	}
	if ch.SmtpHost == "" {
		return ch, errEmailNotConfigured
	}
	return ch, nil
}

// composeRecipients checks the To, Cc and Bcc of a new mail: at most maxCopies each, valid
// addresses that are not this workspace's, lower-cased, an address named twice kept in its first
// list, none undeliverable.
func (s *Server) composeRecipients(ctx context.Context, q *store.Queries, ws uuid.UUID, to, cc, bcc []string) ([]string, []string, []string, error) {
	fields := []string{"to", "cc", "bcc"}
	var (
		lists [3][]string
		taken []string
	)
	for i, in := range [][]string{to, cc, bcc} {
		if len(in) > maxCopies {
			return nil, nil, nil, errValidation(fmt.Sprintf("%s can name at most %d addresses", fields[i], maxCopies))
		}
		for _, v := range in {
			addr, err := s.copyAddress(ctx, ws, fields[i], v)
			if err != nil {
				return nil, nil, nil, err
			}
			if slices.Contains(taken, addr) {
				continue
			}
			taken = append(taken, addr)
			lists[i] = append(lists[i], addr)
		}
	}
	suppressed, err := q.SuppressedEmails(ctx, store.SuppressedEmailsParams{WorkspaceID: ws, Emails: taken})
	if err != nil {
		return nil, nil, nil, err
	}
	for i, list := range lists {
		for _, addr := range list {
			if slices.Contains(suppressed, addr) {
				return nil, nil, nil, undeliverableCopy(fields[i], addr)
			}
		}
	}
	return lists[0], lists[1], lists[2], nil
}

func (s *Server) takeComposeRecipients(ctx context.Context, p principal, n int) error {
	if ok, wait := s.limits.takeAll(ctx, "compose_recipients:"+p.viewerID().String(), limitComposeRecipients, n); !ok {
		e := rateLimited(wait)
		e.Detail = composeRecipientsBudget + "; " + e.Detail
		return e
	}
	return nil
}

// composedPlan is the plan of a new mail: every To in To, the Cc, the Bcc only in the envelope,
// the subject as written and no parent.
func (s *Server) composedPlan(ctx context.Context, q *store.Queries, c store.Conversation, ch store.EmailChannel, subject string, to, cc, bcc []string) (*emailPlan, error) {
	plan, err := s.emailPlanFor(ctx, q, c, ch, to[0])
	if err != nil {
		return nil, err
	}
	plan.subject, plan.toAll, plan.cc, plan.bcc = subject, to, cc, bcc
	plan.inReplyTo, plan.references = nil, nil
	return plan, nil
}

// composedDraftLists are the recipients a composed draft is sent to: those stored with it, Cc and
// Bcc replaced when given.
func composedDraftLists(d store.ComposedMail, cc, bcc copyList) ([]string, []string, []string) {
	ccList, bccList := d.CcAddresses, d.BccAddresses
	if cc.set {
		ccList = cc.addrs
	}
	if bcc.set {
		bccList = bcc.addrs
	}
	return d.ToAddresses, ccList, bccList
}

// composedDraftPlan is the plan for sending a composed draft, its recipients checked again.
func (s *Server) composedDraftPlan(ctx context.Context, q *store.Queries, c store.Conversation, d store.ComposedMail, cc, bcc copyList) (*emailPlan, error) {
	if c.ChannelID == nil {
		return nil, errComposeNoChannel
	}
	ch, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: c.WorkspaceID, ChannelID: *c.ChannelID})
	if store.IsNotFound(err) {
		return nil, errComposeNoChannel
	}
	if err != nil {
		return nil, err
	}
	if ch.SmtpHost == "" {
		return nil, errEmailNotConfigured
	}
	toIn, ccIn, bccIn := composedDraftLists(d, cc, bcc)
	if len(toIn) == 0 {
		return nil, errComposeNoRecipient
	}
	to, ccList, bccList, err := s.composeRecipients(ctx, q, c.WorkspaceID, toIn, ccIn, bccIn)
	if err != nil {
		return nil, err
	}
	return s.composedPlan(ctx, q, c, ch, d.Subject, to, ccList, bccList)
}

// addComposedRecipients makes the further To and the Cc of a new mail contacts when unknown and
// active participants of c.
func addComposedRecipients(ctx context.Context, q *store.Queries, events *eventBatch, c store.Conversation, plan *emailPlan, now time.Time) error {
	var changed int64
	for _, addr := range slices.Concat(plan.toAll[1:], plan.cc) {
		if _, err := contactByAddress(ctx, q, c.WorkspaceID, addr, now); err != nil {
			return err
		}
		n, err := putParticipant(ctx, q, c, addr, participantActive, participantCc, now)
		if err != nil {
			return err
		}
		changed += n
	}
	if changed == 0 {
		return nil
	}
	return emitParticipants(ctx, q, events, c)
}

// contactByAddress is the workspace's contact with addr, or a new one, as ingress makes for an
// unknown sender.
func contactByAddress(ctx context.Context, q *store.Queries, ws uuid.UUID, addr string, now time.Time) (uuid.UUID, error) {
	id, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: addr})
	if err == nil || !store.IsNotFound(err) {
		return id, err
	}
	r, err := q.CreateContact(ctx, store.CreateContactParams{ID: newID(), WorkspaceID: ws, Name: "", Attributes: []byte("{}"), Now: now})
	if err != nil {
		return uuid.Nil(), err
	}
	if err := q.AddContactEmail(ctx, store.AddContactEmailParams{WorkspaceID: ws, ContactID: r.ID, Email: addr, Position: 0}); err != nil {
		return uuid.Nil(), err
	}
	return r.ID, q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: ws, ID: r.ID})
}

// composedMail is the answer for a stored composed message.
func (s *Server) composedMail(ctx context.Context, q *store.Queries, p principal, messageID uuid.UUID) (oas.ComposedMail, error) {
	m, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: p.workspaceID, ID: messageID})
	if store.IsNotFound(err) {
		return oas.ComposedMail{}, errMessageGone
	}
	if err != nil {
		return oas.ComposedMail{}, err
	}
	c, err := readableConversation(ctx, q, p, m.ConversationID)
	if err != nil {
		return oas.ComposedMail{}, err
	}
	conv, err := oneConversation(ctx, q, c)
	if err != nil {
		return oas.ComposedMail{}, err
	}
	atts, err := messageAttachments(ctx, q, p.workspaceID, []uuid.UUID{m.ID})
	if err != nil {
		return oas.ComposedMail{}, err
	}
	emails, err := emailSummaries(ctx, q, p.workspaceID, []uuid.UUID{m.ID})
	if err != nil {
		return oas.ComposedMail{}, err
	}
	msgs := []oas.Message{withBcc(withEmail(messageBody(messageRow(m), atts[m.ID]), emails[m.ID]), emails[m.ID])}
	if _, err := attachRecipients(ctx, q, p, msgs); err != nil {
		return oas.ComposedMail{}, err
	}
	return oas.ComposedMail{Conversation: conv, Message: msgs[0]}, nil
}

// repeatedCompose is the stored answer for a client_id already used in the inbox, or nil.
func (s *Server) repeatedCompose(ctx context.Context, p principal, inboxID uuid.UUID, clientID *string) (*oas.ComposedMail, error) {
	if clientID == nil {
		return nil, nil
	}
	id, err := s.st.GetComposedMail(ctx, store.GetComposedMailParams{WorkspaceID: p.workspaceID, InboxID: inboxID, ClientID: clientID})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out, err := s.composedMail(ctx, s.st.Queries, p, id)
	if err != nil {
		return nil, err
	}
	return &out, s.finishComposed(ctx, p, &out)
}

func (s *Server) finishComposed(ctx context.Context, p principal, out *oas.ComposedMail) error {
	if err := s.fillSLA(ctx, s.st.Queries, p.workspaceID, &out.Conversation); err != nil {
		return err
	}
	starred, err := memberStarred(ctx, s.st.Queries, p, []uuid.UUID{out.Conversation.Id})
	if err != nil {
		return err
	}
	out.Conversation.Starred = starredFor(p, starred[out.Conversation.Id])
	return nil
}

func (s *Server) ComposeMail(ctx context.Context, req oas.ComposeMailRequestObject) (oas.ComposeMailResponseObject, error) {
	p := principalFrom(ctx)
	var (
		in        *messageInput
		inboxID   uuid.UUID
		channelID *uuid.UUID
		err       error
	)
	switch {
	case req.JSONBody != nil:
		b := req.JSONBody
		in = &messageInput{html: b.Html, clientID: b.ClientId, subject: &b.Subject, draft: b.Draft != nil && *b.Draft, unsigned: b.Signature != nil && !*b.Signature}
		if b.Body != nil {
			in.body = *b.Body
		}
		in.to, in.cc, in.bcc = copiesOf(&b.To), copiesOf(b.Cc), copiesOf(b.Bcc)
		in.from = (*string)(b.FromAddress)
		in.sendAt, in.undrafted = b.SendAt, b.Draft != nil && !*b.Draft
		inboxID, channelID = b.InboxId, b.ChannelId
	case req.MultipartBody != nil:
		if in, err = s.readMultipart(req.MultipartBody, p.workspaceID, composeFormFields, s.serverLimits()); err != nil {
			return nil, err
		}
		if inboxID, err = uuid.Parse(strings.TrimSpace(in.extra["inbox_id"])); err != nil {
			in.close()
			return nil, errValidation("inbox_id must be an inbox id")
		}
		if v, ok := in.extra["channel_id"]; ok {
			id, err := uuid.Parse(strings.TrimSpace(v))
			if err != nil {
				in.close()
				return nil, errComposeChannel
			}
			channelID = &id
		}
	default:
		return nil, errValidation("send application/json or multipart/form-data")
	}
	defer in.close()
	in.kind, in.direction = string(oas.MessageKindMessage), string(oas.Out)
	toIn, ccIn, bccIn := in.to.addrs, in.cc.addrs, in.bcc.addrs
	in.to, in.cc, in.bcc = copyList{}, copyList{}, copyList{}
	if in.subject == nil {
		return nil, errValidation("subject is required")
	}
	subject, err := trimmed(*in.subject, 1, 500, "subject")
	if err != nil {
		return nil, err
	}
	if err := s.validateMessage(p, in); err != nil {
		return nil, err
	}
	if len(toIn) == 0 {
		return nil, errValidation("to needs at least one address")
	}
	if _, err := visibleInbox(ctx, s.st.Queries, p, inboxID); err != nil {
		return nil, err
	}
	if prev, err := s.repeatedCompose(ctx, p, inboxID, in.clientID); err != nil || prev != nil {
		if err != nil {
			return nil, err
		}
		return oas.ComposeMail200JSONResponse(*prev), nil
	}
	to, cc, bcc, err := s.composeRecipients(ctx, s.st.Queries, p.workspaceID, toIn, ccIn, bccIn)
	if err != nil {
		return nil, err
	}
	if len(in.files) > 0 || (in.from != nil && !in.draft) {
		ch, err := composeChannel(ctx, s.st.Queries, p.workspaceID, inboxID, channelID)
		if err != nil {
			return nil, err
		}
		if in.from != nil {
			if _, err := s.fromMemberAddress(ctx, s.st.Queries, p, ch, *in.from); err != nil {
				return nil, err
			}
		}
		limits, err := s.channelLimits(ctx, s.st.Queries, p.workspaceID, &ch.ChannelID)
		if err != nil {
			return nil, err
		}
		for _, f := range in.files {
			if f.size > limits.maxBytes {
				return nil, limits.tooLarge()
			}
			if !limits.allows(f.contentType) {
				return nil, limits.typeRefused()
			}
		}
	}
	if err := s.rateLimit(ctx, rateCheck{"compose:" + p.viewerID().String(), limitComposePerActor}); err != nil {
		return nil, err
	}
	if !in.draft {
		if err := s.takeComposeRecipients(ctx, p, len(to)+len(cc)+len(bcc)); err != nil {
			return nil, err
		}
	}
	var stored []string
	for _, f := range in.files {
		if err := s.objects.Put(ctx, f.key, f.file, f.size, f.contentType); err != nil {
			s.deleteObjects(ctx, stored)
			return nil, err
		}
		stored = append(stored, f.key)
	}
	var out oas.ComposedMail
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		inbox, err := visibleInbox(ctx, q, p, inboxID)
		if err != nil {
			return err
		}
		ch, err := composeChannel(ctx, q, p.workspaceID, inbox.ID, channelID)
		if err != nil {
			return err
		}
		now := s.now()
		contactID, err := contactByAddress(ctx, q, p.workspaceID, to[0], now)
		if err != nil {
			return err
		}
		var assignee *uuid.UUID
		if inbox.Purpose == string(oas.InboxPurposeMail) && !p.isKey() {
			id := p.memberID
			assignee = &id
		}
		chID := ch.ChannelID
		c, err := q.CreateConversation(ctx, store.CreateConversationParams{
			ID: newID(), WorkspaceID: p.workspaceID, InboxID: inbox.ID, ContactID: contactID, ChannelID: &chID,
			Subject: subject, Priority: string(oas.Normal), AssigneeID: assignee, Now: now,
		})
		if err != nil {
			return err
		}
		authorType, _, _ := authorFor(p)
		private := in.draft && authorType == string(oas.AuthorTypeMember)
		if private {
			author := p.memberID
			if err := q.SetConversationDraftOf(ctx, store.SetConversationDraftOfParams{WorkspaceID: p.workspaceID, ID: c.ID, DraftOf: &author}); err != nil {
				return err
			}
			c.DraftOf = &author
		}
		if inbox.Purpose == string(oas.InboxPurposeMail) {
			// Mail one wrote waits under Sent; the first reply reopens it into the Inbox.
			if c, err = q.UpdateConversation(ctx, store.UpdateConversationParams{
				WorkspaceID: c.WorkspaceID, ID: c.ID, Subject: c.Subject, Status: string(oas.ConversationStatusPending), Priority: c.Priority,
				AssigneeID: c.AssigneeID, Spam: c.Spam, FeedbackProgress: c.FeedbackProgress, Now: now,
			}); err != nil {
				return err
			}
		}
		body, err := conversationBody(ctx, q, c, nil)
		if err != nil {
			return err
		}
		events.conversation(realtime.ConversationCreated, c, body)
		before := c
		before.AssigneeID = nil
		if err := s.recordChanges(ctx, q, events, p, before, c, nil, nil, now); err != nil {
			return err
		}
		if err := s.addUsage(ctx, q, p.workspaceID, 1, 0, 0); err != nil {
			return err
		}
		var plan *emailPlan
		if !in.draft {
			if plan, err = s.composedPlan(ctx, q, c, ch, subject, to, cc, bcc); err != nil {
				return err
			}
			if err := s.applyFromAddress(ctx, q, p, plan, in.from); err != nil {
				return err
			}
		}
		arg := store.CreateMessageParams{
			ID: newID(), WorkspaceID: p.workspaceID, ConversationID: c.ID, Kind: in.kind, Direction: &in.direction, Body: in.body,
			Html: in.html, ClientID: in.clientID, CreatedAt: now, Draft: in.draft, Via: p.viaClient(),
		}
		arg.AuthorType, arg.AuthorMemberID, arg.AuthorApiKeyID = authorFor(p)
		if plan != nil {
			queued := deliveryQueued
			arg.DeliveryState = &queued
		}
		if err := s.signMessage(ctx, q, p, c, in, plan != nil, &arg); err != nil {
			return err
		}
		msg, err := q.CreateMessage(ctx, arg)
		if err != nil {
			return err
		}
		var summary *store.ListMessageEmailsRow
		if plan != nil {
			if summary, err = s.dispatchOutgoing(ctx, q, events, p, c, plan, msg, now); err != nil {
				return err
			}
			if err := addComposedRecipients(ctx, q, events, c, plan, now); err != nil {
				return err
			}
		}
		var (
			atts  []store.Attachment
			total int64
		)
		for _, f := range in.files {
			a, err := q.CreateAttachment(ctx, store.CreateAttachmentParams{
				ID: f.id, WorkspaceID: p.workspaceID, ConversationID: c.ID, MessageID: msg.ID, StorageKey: f.key,
				Filename: f.filename, ContentType: f.contentType, SizeBytes: f.size, CreatedAt: now,
			})
			if err != nil {
				return err
			}
			atts = append(atts, a)
			total += f.size
		}
		composed := store.CreateComposedMailParams{
			WorkspaceID: p.workspaceID, MessageID: msg.ID, InboxID: inbox.ID, ClientID: in.clientID,
			ToAddresses: to, CcAddresses: nonNil(cc), BccAddresses: nonNil(bcc), Subject: subject, CreatedAt: now,
		}
		n, err := q.CreateComposedMail(ctx, composed)
		if err != nil {
			return err
		}
		if n == 0 {
			return errComposeRepeated
		}
		if in.draft {
			if in.sendAt != nil {
				if msg, err = s.scheduleLocked(ctx, q, events, p, c, msg.Kind, msg.ID, *in.sendAt, copyList{}, copyList{}, in.from); err != nil {
					return err
				}
			}
			events.conversation(realtime.DraftCreated, c, messageBody(msg, atts))
		} else {
			if err := q.TouchConversation(ctx, store.TouchConversationParams{WorkspaceID: p.workspaceID, ID: c.ID, Now: now, IsMessage: true}); err != nil {
				return err
			}
			if err := s.addUsage(ctx, q, p.workspaceID, 0, 1, total); err != nil {
				return err
			}
			events.conversation(realtime.MessageCreated, c, withEmail(messageBody(msg, atts), summary))
			if arg.AuthorMemberID != nil {
				if _, err := s.moveReadCursor(ctx, q, events, p.workspaceID, *arg.AuthorMemberID, c, msg.ID, msg.CreatedAt); err != nil {
					return err
				}
			}
		}
		if c, err = q.GetConversation(ctx, store.GetConversationParams{WorkspaceID: p.workspaceID, ID: c.ID}); err != nil {
			return err
		}
		if out.Conversation, err = oneConversation(ctx, q, c); err != nil {
			return err
		}
		out.Message = withRecipients(withBcc(withEmail(messageBody(msg, atts), summary), summary), p, composed.ToAddresses, composed.CcAddresses, composed.BccAddresses)
		return nil
	})
	if err != nil {
		s.deleteObjects(ctx, stored)
	}
	if errors.Is(err, errComposeRepeated) {
		prev, err := s.repeatedCompose(ctx, p, inboxID, in.clientID)
		if err != nil {
			return nil, err
		}
		if prev == nil {
			return nil, errComposeRepeated
		}
		return oas.ComposeMail200JSONResponse(*prev), nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.finishComposed(ctx, p, &out); err != nil {
		return nil, err
	}
	return oas.ComposeMail201JSONResponse(out), nil
}

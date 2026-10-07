package api

import (
	"context"
	"errors"
	"log/slog"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	defaultChatEmailDelay  = 5 * time.Minute
	continuitySubjectRunes = 60
)

type ContinuityArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
}

func (ContinuityArgs) Kind() string { return "chat_continuity" }

type continuityWorker struct {
	river.WorkerDefaults[ContinuityArgs]
	s *Server
}

func (w *continuityWorker) Work(ctx context.Context, job *river.Job[ContinuityArgs]) error {
	next, err := w.s.CheckContinuity(ctx, job.Args.WorkspaceID, job.Args.ConversationID)
	if err != nil || next == nil {
		return err
	}
	_, err = w.s.jobs.Insert(ctx, job.Args, &river.InsertOpts{ScheduledAt: *next})
	return err
}

func (s *Server) continuityOpts(at time.Time) *river.InsertOpts {
	return &river.InsertOpts{ScheduledAt: at, UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: s.chat.EmailDelay}}
}

// scheduleContinuity checks a chat conversation once the delay has passed after a member's reply.
func (s *Server) scheduleContinuity(events *eventBatch, c store.Conversation, at time.Time) {
	events.job(ContinuityArgs{WorkspaceID: c.WorkspaceID, ConversationID: c.ID}, s.continuityOpts(at.Add(s.chat.EmailDelay)))
}

// scheduleContactContinuity runs after a contact's last connection closes, for the replies they
// have not read.
func (s *Server) scheduleContactContinuity(ctx context.Context, workspaceID, contactID uuid.UUID) {
	ids, err := s.st.ListConversationsWithPendingReplies(ctx, store.ListConversationsWithPendingRepliesParams{WorkspaceID: workspaceID, ContactID: contactID})
	if err != nil {
		s.log.WarnContext(ctx, "continuity", slog.Any("error", err))
		return
	}
	at := s.now().Add(s.chat.EmailDelay)
	for _, id := range ids {
		if _, err := s.jobs.Insert(ctx, ContinuityArgs{WorkspaceID: workspaceID, ConversationID: id}, s.continuityOpts(at)); err != nil {
			s.log.WarnContext(ctx, "continuity", slog.Any("error", err))
		}
	}
}

func later(a time.Time, b *time.Time) time.Time {
	if b != nil && b.After(a) {
		return *b
	}
	return a
}

// CheckContinuity e-mails a chat contact the members' replies they have not read, once they have
// been gone for the chat e-mail delay and the oldest unread reply is that old, at most once per
// delay. It returns when to check again, or nil.
func (s *Server) CheckContinuity(ctx context.Context, workspaceID, conversationID uuid.UUID) (*time.Time, error) {
	now, delay := s.now(), s.chat.EmailDelay
	var next *time.Time
	err := s.inTx(ctx, workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := q.LockConversation(ctx, store.LockConversationParams{WorkspaceID: workspaceID, ID: conversationID})
		if store.IsNotFound(err) || (err == nil && c.ChannelID == nil) {
			return nil
		}
		if err != nil {
			return err
		}
		ch, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: workspaceID, ID: *c.ChannelID})
		if store.IsNotFound(err) || (err == nil && !emailsReplies(c, ch.Kind)) {
			return nil
		}
		if err != nil {
			return err
		}
		pending, err := q.ListPendingReplies(ctx, store.ListPendingRepliesParams{WorkspaceID: workspaceID, ConversationID: c.ID})
		if err != nil || len(pending) == 0 {
			return err
		}
		connected, err := q.ContactConnected(ctx, store.ContactConnectedParams{WorkspaceID: workspaceID, ContactID: &c.ContactID, FreshAfter: now.Add(-presenceFresh)})
		if err != nil {
			return err
		}
		if connected {
			at := now.Add(delay)
			next = &at
			return nil
		}
		lastSeen, err := q.ContactLastSeen(ctx, store.ContactLastSeenParams{WorkspaceID: workspaceID, ContactID: c.ContactID})
		if err != nil {
			return err
		}
		due := later(pending[0].CreatedAt.Add(delay), new(lastSeen.Add(delay)))
		if c.ContinuitySentAt != nil {
			due = later(due, new(c.ContinuitySentAt.Add(delay)))
		}
		if due.After(now) {
			next = &due
			return nil
		}
		to, err := continuityAddress(ctx, q, c)
		if err != nil || to == "" {
			return err
		}
		ech, err := q.InboxEmailChannel(ctx, store.InboxEmailChannelParams{WorkspaceID: workspaceID, InboxID: c.InboxID})
		if store.IsNotFound(err) || (err == nil && ech.SmtpHost == "") {
			return nil
		}
		if err != nil {
			return err
		}
		suppressed, err := q.IsEmailSuppressed(ctx, store.IsEmailSuppressedParams{WorkspaceID: workspaceID, Email: to})
		if err != nil || suppressed {
			return err
		}
		plan, err := s.emailPlanFor(ctx, q, c, ech, to)
		if err != nil {
			var apiErr *apiError
			if errors.As(err, &apiErr) {
				return nil
			}
			return err
		}
		if c.Subject == "" {
			if plan.subject, err = continuitySubject(ctx, q, c); err != nil {
				return err
			}
		}
		ids := make([]uuid.UUID, len(pending))
		for i, p := range pending {
			ids[i] = p.ID
			m, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: workspaceID, ID: p.ID})
			if err != nil {
				return err
			}
			if err := s.recordOutboundEmail(ctx, q, plan, messageRow(m)); err != nil {
				return err
			}
			if err := s.setDeliveryTx(ctx, q, events, workspaceID, p.ID, deliveryQueued, nil); err != nil {
				return err
			}
		}
		events.job(EmailSendArgs{WorkspaceID: workspaceID, MessageID: ids[len(ids)-1], Batch: ids}, &river.InsertOpts{MaxAttempts: emailSendAttempts})
		return q.ClaimContinuity(ctx, store.ClaimContinuityParams{
			WorkspaceID: workspaceID, ID: c.ID, Through: pending[len(pending)-1].CreatedAt, Now: now,
		})
	})
	return next, err
}

// continuitySubject names a conversation without a subject by its inbox and the start of its
// first message.
func continuitySubject(ctx context.Context, q *store.Queries, c store.Conversation) (string, error) {
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: c.WorkspaceID, ID: c.InboxID})
	if err != nil {
		return "", err
	}
	body, err := q.FirstMessageBody(ctx, store.FirstMessageBodyParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID})
	if store.IsNotFound(err) {
		return in.Name, nil
	}
	if err != nil {
		return "", err
	}
	return in.Name + " — " + excerpt(body, continuitySubjectRunes), nil
}

// continuityAddress is a verified address of the conversation's contact, else the address they
// typed in the widget.
func continuityAddress(ctx context.Context, q *store.Queries, c store.Conversation) (string, error) {
	to, err := q.ContactReplyAddress(ctx, store.ContactReplyAddressParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID})
	if err == nil || !store.IsNotFound(err) {
		return to, err
	}
	emails, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: c.WorkspaceID, ContactIds: []uuid.UUID{c.ContactID}})
	if err != nil {
		return "", err
	}
	if len(emails) > 0 {
		return emails[0].Email, nil
	}
	ct, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: c.WorkspaceID, ID: c.ContactID})
	if err != nil || ct.TypedEmail == nil {
		return "", err
	}
	return *ct.TypedEmail, nil
}

// claimTypedEmail makes the address a chat contact typed one of their addresses once mail from it
// answers our e-mail to that very address, before the sender is matched to a contact.
func (s *Server) claimTypedEmail(ctx context.Context, q *store.Queries, events *eventBatch, ws, inboxID uuid.UUID, m *email.Message, sender string) error {
	if _, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: sender}); !store.IsNotFound(err) {
		return err
	}
	ids := append(append([]string{}, m.InReplyTo...), m.References...)
	if len(ids) == 0 {
		return nil
	}
	contactID, err := q.FindThreadSentTo(ctx, store.FindThreadSentToParams{WorkspaceID: ws, InboxID: inboxID, Ids: ids, Address: sender})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	ct, err := q.LockContact(ctx, store.LockContactParams{WorkspaceID: ws, ID: contactID})
	if err != nil || ct.TypedEmail == nil || *ct.TypedEmail != sender {
		return err
	}
	n, err := q.CountContactEmails(ctx, store.CountContactEmailsParams{WorkspaceID: ws, ContactID: ct.ID})
	if err != nil {
		return err
	}
	if err := q.AddContactEmail(ctx, store.AddContactEmailParams{WorkspaceID: ws, ContactID: ct.ID, Email: sender, Position: int32(n)}); err != nil {
		return err
	}
	r, err := q.SetContactTypedEmail(ctx, store.SetContactTypedEmailParams{WorkspaceID: ws, ID: ct.ID, Now: s.now()})
	if err != nil {
		return err
	}
	if err := q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: ws, ID: ct.ID}); err != nil {
		return err
	}
	body, err := s.contactBody(ctx, q, ws, contactRow(r))
	if err != nil {
		return err
	}
	events.add(realtime.ContactUpdated, nil, nil, body)
	return nil
}

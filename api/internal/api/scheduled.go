package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	scheduledSendAttempts = 10
	maxSendAhead          = 366 * 24 * time.Hour
)

var (
	errInvalidSendAt     = problem(http.StatusBadRequest, "invalid_send_at", "send_at must be at least 1 minute and at most 366 days ahead")
	errSendAtNeedsMember = errValidation("send_at needs a member session")
	errScheduleNotOwn    = errValidation("only your own draft can be scheduled")
	errScheduleStale     = errors.New("the scheduled send was moved, cancelled or done")
)

type scheduledSendCtx int

// scheduledSendKey carries the send_at a scheduled send was queued for into lockDraft, which sends
// only while the locked draft is still scheduled for exactly that time.
const scheduledSendKey scheduledSendCtx = 0

func (s *Server) checkSendAt(p principal, at time.Time) (time.Time, error) {
	if p.scoped() || p.memberRef() == nil {
		return time.Time{}, errSendAtNeedsMember
	}
	now := s.now()
	if at.Before(now.Add(time.Minute)) || at.After(now.Add(maxSendAhead)) {
		return time.Time{}, errInvalidSendAt
	}
	return at.UTC().Truncate(time.Microsecond), nil
}

func ownDraft(p principal, m store.GetMessageRow) error {
	if m.AuthorType != string(oas.AuthorTypeMember) || m.AuthorMemberID == nil || *m.AuthorMemberID != p.memberID {
		return errScheduleNotOwn
	}
	return nil
}

type ScheduledSendArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	MessageID   uuid.UUID `json:"message_id"`
	SendAt      time.Time `json:"send_at"`
}

func (ScheduledSendArgs) Kind() string { return "scheduled_send" }

func queueScheduledSend(events *eventBatch, ws, id uuid.UUID, at time.Time) {
	events.jobOnce(ScheduledSendArgs{WorkspaceID: ws, MessageID: id, SendAt: at}, &river.InsertOpts{
		ScheduledAt: at, MaxAttempts: scheduledSendAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled,
		}},
	})
}

func copyAddrs(l copyList) []string {
	if !l.set {
		return nil
	}
	return nonNil(l.addrs)
}

func (s *Server) cleanCopies(ctx context.Context, ws uuid.UUID, cc, bcc *copyList) error {
	if len(cc.addrs) > maxCopies || len(bcc.addrs) > maxCopies {
		return errValidation("cc and bcc can name at most 50 addresses each")
	}
	for i, l := range []*copyList{cc, bcc} {
		field := []string{"cc", "bcc"}[i]
		for j, v := range l.addrs {
			a, err := s.copyAddress(ctx, ws, field, v)
			if err != nil {
				return err
			}
			l.addrs[j] = a
		}
	}
	return nil
}

func (s *Server) scheduleLocked(ctx context.Context, q *store.Queries, events *eventBatch, p principal, c store.Conversation, kind string, id uuid.UUID, sendAt time.Time, cc, bcc copyList, from *string) (messageRow, error) {
	if kind == string(oas.MessageKindForward) {
		if cc.set || bcc.set {
			return messageRow{}, errForwardNoCopies
		}
		if from != nil {
			return messageRow{}, errValidation("a forward goes from the channel's address and takes no from_address")
		}
	}
	if cc.set || bcc.set || from != nil {
		notEmailed := errFromNotEmailed
		if cc.set || bcc.set {
			notEmailed = errCopiesNotEmailed
		}
		if c.ChannelID == nil {
			return messageRow{}, notEmailed
		}
		ch, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: p.workspaceID, ChannelID: *c.ChannelID})
		if store.IsNotFound(err) {
			return messageRow{}, notEmailed
		}
		if err != nil {
			return messageRow{}, err
		}
		if from != nil {
			if _, err := s.fromMemberAddress(ctx, q, p, ch, *from); err != nil {
				return messageRow{}, err
			}
		}
	}
	if cc.set || bcc.set {
		d, err := q.GetComposedDraft(ctx, store.GetComposedDraftParams{WorkspaceID: p.workspaceID, MessageID: id})
		if err != nil && !store.IsNotFound(err) {
			return messageRow{}, err
		}
		if err == nil {
			if err := s.keepComposedCopies(ctx, q, p, d, cc, bcc); err != nil {
				return messageRow{}, err
			}
			cc, bcc = copyList{}, copyList{}
		}
	}
	m, err := q.ScheduleDraft(ctx, store.ScheduleDraftParams{
		WorkspaceID: p.workspaceID, ID: id, SendAt: sendAt, MemberID: p.memberID,
		Cc: copyAddrs(cc), Bcc: copyAddrs(bcc), FromAddress: from,
	})
	if err != nil {
		return messageRow{}, err
	}
	queueScheduledSend(events, p.workspaceID, id, sendAt)
	return messageRow(m), nil
}

// keepComposedCopies writes the Cc and Bcc given for a new mail's scheduled send into its stored
// lists, so its recipients always show what goes.
func (s *Server) keepComposedCopies(ctx context.Context, q *store.Queries, p principal, d store.ComposedMail, cc, bcc copyList) error {
	ccList, bccList := d.CcAddresses, d.BccAddresses
	if cc.set {
		ccList = cc.addrs
	}
	if bcc.set {
		bccList = bcc.addrs
	}
	to, ccList, bccList, err := s.composeRecipients(ctx, q, p.workspaceID, d.ToAddresses, ccList, bccList)
	if err != nil {
		return err
	}
	_, err = q.UpdateComposedDraft(ctx, store.UpdateComposedDraftParams{
		WorkspaceID: p.workspaceID, MessageID: d.MessageID, ToAddresses: to, CcAddresses: nonNil(ccList), BccAddresses: nonNil(bccList), Subject: d.Subject,
	})
	return err
}

func (s *Server) scheduleDraft(ctx context.Context, p principal, id uuid.UUID, at time.Time, cc, bcc copyList, from *string) (oas.SendMessageResponseObject, error) {
	sendAt, err := s.checkSendAt(p, at)
	if err != nil {
		return nil, err
	}
	if err := s.cleanCopies(ctx, p.workspaceID, &cc, &bcc); err != nil {
		return nil, err
	}
	var out oas.Message
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := lockDraft(ctx, q, p, id)
		if err != nil {
			return err
		}
		cur, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: p.workspaceID, ID: id})
		if err != nil {
			return err
		}
		if err := ownDraft(p, cur); err != nil {
			return err
		}
		m, err := s.scheduleLocked(ctx, q, events, p, c, cur.Kind, id, sendAt, cc, bcc, from)
		if err != nil {
			return err
		}
		atts, err := messageAttachments(ctx, q, p.workspaceID, []uuid.UUID{id})
		if err != nil {
			return err
		}
		out = messageBody(m, atts[id])
		events.conversation(realtime.DraftUpdated, c, out)
		msgs := []oas.Message{out}
		if _, err := attachRecipients(ctx, q, p, msgs); err != nil {
			return err
		}
		out = msgs[0]
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.SendMessage200JSONResponse(out), nil
}

func (s *Server) UnscheduleMessage(ctx context.Context, req oas.UnscheduleMessageRequestObject) (oas.UnscheduleMessageResponseObject, error) {
	p := principalFrom(ctx)
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
		atts, err := messageAttachments(ctx, q, p.workspaceID, []uuid.UUID{cur.ID})
		if err != nil {
			return err
		}
		if cur.ScheduledAt == nil {
			out = messageBody(messageRow(cur), atts[cur.ID])
		} else {
			m, err := q.UnscheduleDraft(ctx, store.UnscheduleDraftParams{WorkspaceID: p.workspaceID, ID: cur.ID})
			if err != nil {
				return err
			}
			out = messageBody(messageRow(m), atts[cur.ID])
			events.conversation(realtime.DraftUpdated, c, out)
		}
		msgs := []oas.Message{out}
		if _, err := attachRecipients(ctx, q, p, msgs); err != nil {
			return err
		}
		out = msgs[0]
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.UnscheduleMessage200JSONResponse(out), nil
}

// memberDraftAuthor names the member whose draft a draft event carries; such an event reaches only
// that member and never a webhook. A member draft whose author left names no one.
func memberDraftAuthor(typ string, data []byte) (uuid.UUID, bool) {
	if typ != realtime.DraftCreated && typ != realtime.DraftUpdated && typ != realtime.DraftDeleted {
		return uuid.UUID{}, false
	}
	var m struct {
		Author oas.MessageAuthor `json:"author"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return uuid.UUID{}, true
	}
	if m.Author.Type != oas.AuthorTypeMember {
		return uuid.UUID{}, false
	}
	if m.Author.MemberId == nil {
		return uuid.UUID{}, true
	}
	return *m.Author.MemberId, true
}

// freeClientID drops a client_id another member's private draft holds: the new message is created
// without it rather than answering with that draft.
func freeClientID(ctx context.Context, q *store.Queries, ws, conv uuid.UUID, id *string) (*string, error) {
	taken, err := q.ClientIDTaken(ctx, store.ClientIDTakenParams{WorkspaceID: ws, ConversationID: conv, ClientID: id})
	if err != nil || taken {
		return nil, err
	}
	return id, nil
}

func revealDraftConversation(ctx context.Context, q *store.Queries, events *eventBatch, c store.Conversation) (store.Conversation, error) {
	if c.DraftOf == nil {
		return c, nil
	}
	if err := q.SetConversationDraftOf(ctx, store.SetConversationDraftOfParams{WorkspaceID: c.WorkspaceID, ID: c.ID}); err != nil {
		return c, err
	}
	c.DraftOf = nil
	body, err := conversationBody(ctx, q, c, nil)
	if err != nil {
		return c, err
	}
	events.conversation(realtime.ConversationCreated, c, body)
	return c, nil
}

type scheduledSendWorker struct {
	river.WorkerDefaults[ScheduledSendArgs]
	s *Server
}

func (w *scheduledSendWorker) Work(ctx context.Context, job *river.Job[ScheduledSendArgs]) error {
	return w.s.SendScheduled(ctx, job.Args, job.Attempt >= job.MaxAttempts)
}

func (w *scheduledSendWorker) Timeout(*river.Job[ScheduledSendArgs]) time.Duration {
	return 2 * time.Minute
}

// lockDraft refuses unless the draft is still scheduled for a.SendAt, so a moved draft or a racing worker sends nothing.
func (s *Server) SendScheduled(ctx context.Context, a ScheduledSendArgs, lastAttempt bool) error {
	if live, err := s.workspaceLive(ctx, a.WorkspaceID); err != nil || !live {
		return err
	}
	sc, err := s.st.GetScheduledSend(ctx, store.GetScheduledSendParams{WorkspaceID: a.WorkspaceID, ID: a.MessageID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if sc.ScheduledAt == nil || !sc.ScheduledAt.Equal(a.SendAt) {
		return nil
	}
	if sc.ScheduledBy == nil {
		return s.failScheduled(ctx, a, nil, "member_inactive", "the member who scheduled it is no longer in the workspace")
	}
	m, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: a.WorkspaceID, ID: *sc.ScheduledBy})
	if store.IsNotFound(err) {
		return s.failScheduled(ctx, a, nil, "member_inactive", "the member who scheduled it is no longer in the workspace")
	}
	if err != nil {
		return err
	}
	p := principal{workspaceID: a.WorkspaceID, memberID: m.ID, personID: m.PersonID, role: m.Role}
	msg, err := s.st.GetMessage(ctx, store.GetMessageParams{WorkspaceID: a.WorkspaceID, ID: a.MessageID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	c, err := visibleConversationOrTrashed(ctx, s.st.Queries, p, msg.ConversationID, false)
	if errors.Is(err, errConversationGone) {
		return s.failScheduled(ctx, a, &m.ID, "inbox_access_lost", "you can no longer see the conversation's inbox")
	}
	if err != nil {
		return err
	}
	if c.TrashedAt != nil {
		return s.failScheduled(ctx, a, &m.ID, "conversation_trashed", "the conversation is in the trash")
	}
	body := &oas.MessageSend{FromAddress: (*oas.Email)(sc.ScheduledFrom)}
	if sc.ScheduledCc != nil {
		body.Cc = emailsOf(sc.ScheduledCc)
	}
	if sc.ScheduledBcc != nil {
		body.Bcc = emailsOf(sc.ScheduledBcc)
	}
	sctx := context.WithValue(context.WithValue(ctx, principalKey, p), scheduledSendKey, a.SendAt)
	_, err = s.SendMessage(sctx, oas.SendMessageRequestObject{MessageId: a.MessageID, Body: body})
	if err == nil || errors.Is(err, errScheduleStale) || errors.Is(err, errNotADraft) {
		return nil
	}
	if errors.Is(err, errMessageGone) || errors.Is(err, errConversationGone) {
		return s.failScheduled(ctx, a, &m.ID, "conversation_unavailable", "the conversation is no longer available to you")
	}
	var apiErr *apiError
	transient := !errors.As(err, &apiErr) || apiErr.Status == http.StatusTooManyRequests || apiErr.Status >= 500
	if transient && !lastAttempt {
		return err
	}
	code, detail := "send_failed", "the message could not be sent"
	if apiErr != nil {
		code, detail = apiErr.Code, apiErr.Detail
	}
	s.log.WarnContext(ctx, "scheduled send failed", slog.String("code", code), slog.Any("error", err))
	return s.failScheduled(ctx, a, &m.ID, code, detail)
}

func emailsOf(addrs []string) *[]oas.Email {
	out := make([]oas.Email, len(addrs))
	for i, a := range addrs {
		out[i] = oas.Email(a)
	}
	return &out
}

// failScheduled turns the draft back into a plain one with send_error, while it is still scheduled
// for a.SendAt, and notifies the member who scheduled it.
func (s *Server) failScheduled(ctx context.Context, a ScheduledSendArgs, member *uuid.UUID, code, detail string) error {
	failed := false
	err := s.inTx(ctx, a.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		lm, err := q.LockMessage(ctx, store.LockMessageParams{WorkspaceID: a.WorkspaceID, ID: a.MessageID})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !lm.Draft || lm.ScheduledAt == nil || !lm.ScheduledAt.Equal(a.SendAt) {
			return nil
		}
		m, err := q.UnscheduleDraft(ctx, store.UnscheduleDraftParams{
			WorkspaceID: a.WorkspaceID, ID: a.MessageID, SendError: mustJSON(oas.MessageSendError{Code: code, Detail: detail}),
		})
		if err != nil {
			return err
		}
		c, err := q.GetConversation(ctx, store.GetConversationParams{WorkspaceID: a.WorkspaceID, ID: lm.ConversationID})
		if err != nil {
			return err
		}
		atts, err := messageAttachments(ctx, q, a.WorkspaceID, []uuid.UUID{m.ID})
		if err != nil {
			return err
		}
		events.conversation(realtime.DraftUpdated, c, messageBody(messageRow(m), atts[m.ID]))
		if member != nil {
			events.job(ScheduledFailedArgs{WorkspaceID: a.WorkspaceID, MessageID: a.MessageID, MemberID: *member, Detail: detail}, nil)
		}
		failed = true
		return nil
	})
	if err == nil && failed {
		s.log.InfoContext(ctx, "scheduled send kept as draft", slog.String("message_id", a.MessageID.String()), slog.String("code", code))
	}
	return err
}

type ScheduledFailedArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	MessageID   uuid.UUID `json:"message_id"`
	MemberID    uuid.UUID `json:"member_id"`
	Detail      string    `json:"detail"`
}

func (ScheduledFailedArgs) Kind() string { return "scheduled_failed_notify" }

type scheduledFailedWorker struct {
	river.WorkerDefaults[ScheduledFailedArgs]
	s *Server
}

func (w *scheduledFailedWorker) Work(ctx context.Context, job *river.Job[ScheduledFailedArgs]) error {
	return w.s.NotifyScheduledFailed(ctx, job.Args)
}

type scheduledFailedMailData struct {
	Inbox, Subject, Reason, URL, SettingsURL string
}

// NotifyScheduledFailed tells the member whose scheduled message could not go, by push and e-mail
// as they chose for scheduled_send_failed.
func (s *Server) NotifyScheduledFailed(ctx context.Context, a ScheduledFailedArgs) error {
	ws := a.WorkspaceID
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return err
	}
	msg, err := s.st.GetMessage(ctx, store.GetMessageParams{WorkspaceID: ws, ID: a.MessageID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: msg.ConversationID})
	if err != nil {
		return err
	}
	r, err := s.st.GetNotificationMember(ctx, store.GetNotificationMemberParams{WorkspaceID: ws, ID: a.MemberID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: c.InboxID})
	if err != nil {
		return err
	}
	overrides, err := s.st.ListInboxNotificationsOfInbox(ctx, store.ListInboxNotificationsOfInboxParams{WorkspaceID: ws, InboxID: in.ID})
	if err != nil {
		return err
	}
	events := applyNotificationEvents(roleNotificationDefaults(r.Role), parseNotificationUpdate(r.NotificationEvents))
	for _, o := range overrides {
		if o.MemberID == r.ID {
			events = applyNotificationEvents(events, parseNotificationUpdate(o.Events))
		}
	}
	ch := notificationChannelsFor(events, oas.ScheduledSendFailed)
	data := scheduledFailedMailData{
		Inbox: in.Name, Subject: c.Subject, Reason: a.Detail,
		URL:         s.auth.PublicURL + conversationPath(ws, c.ID),
		SettingsURL: s.auth.PublicURL + "/settings/notifications?workspace_id=" + ws.String(),
	}
	if ch.Push && s.push != nil {
		subs, err := s.st.ListLivePushSubscriptionIDs(ctx, store.ListLivePushSubscriptionIDsParams{PersonID: r.PersonID, Now: s.now(), WorkspaceID: &ws})
		if err != nil {
			return err
		}
		if len(subs) > 0 {
			m, err := mail.Render("push_scheduled_failed", r.Locale, data)
			if err != nil {
				return err
			}
			conv, inbox := c.ID, in.ID
			payload := oas.PushNotification{
				Event: oas.ScheduledSendFailed, Title: m.Subject, Body: strings.TrimSpace(m.Text), Url: conversationPath(ws, c.ID),
				Tag: "scheduled-" + msg.ID.String(), ConversationId: &conv, WorkspaceId: &ws, InboxId: &inbox,
			}
			jobs := make([]river.InsertManyParams, 0, len(subs))
			for _, id := range subs {
				jobs = append(jobs, river.InsertManyParams{Args: s.pushArgs(&ws, id, payload, in, c), InsertOpts: pushOpts()})
			}
			if _, err := s.jobs.InsertMany(ctx, jobs); err != nil {
				return err
			}
		}
	}
	if ch.Email {
		m, err := mail.Render("scheduled_failed", r.Locale, data)
		if err != nil {
			return err
		}
		m.To = r.Email
		if err := s.mailer.Send(ctx, m); err != nil {
			s.log.WarnContext(ctx, "scheduled send e-mail", slog.Any("error", err))
		}
	}
	return nil
}

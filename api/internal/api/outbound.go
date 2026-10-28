package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/riverqueue/river"
	"go.opentelemetry.io/otel/attribute"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/telemetry"
)

const (
	deliveryQueued = "queued"
	deliverySent   = "sent"
	deliveryFailed = "failed"

	emailSendAttempts = 8
	maxDeliveryError  = 500
)

var (
	errEmailUndeliverable = problem(http.StatusConflict, "email_undeliverable", "the contact's e-mail address bounced or complained; clear it on the contact to send again")
	errEmailNotConfigured = problem(http.StatusConflict, "email_not_configured", "the conversation's e-mail channel has no SMTP account")
	errEmailNoSender      = problem(http.StatusConflict, "email_no_sender", "the catch-all channel has no address to send this conversation from; set its from_address")
)

type emailPlan struct {
	channel         store.EmailChannel
	channelID       uuid.UUID
	from            email.Address
	to              string
	toAll           []string
	cc              []string
	bcc             []string
	subject         string
	headerID        string
	inReplyTo       *string
	references      []string
	auto            bool
	memberAddressID *uuid.UUID
	design          *designedMail
}

func (p *emailPlan) recipients() []string {
	if len(p.toAll) > 0 {
		return p.toAll
	}
	return []string{p.to}
}

// planEmail decides whether an outgoing message in c goes out by e-mail and prepares its headers.
// It returns nil when the conversation is not an e-mail conversation or the contact has no address.
func (s *Server) planEmail(ctx context.Context, q *store.Queries, c store.Conversation) (*emailPlan, error) {
	if c.ChannelID == nil {
		return nil, nil
	}
	ch, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: c.WorkspaceID, ChannelID: *c.ChannelID})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	to, err := q.ContactReplyAddress(ctx, store.ContactReplyAddressParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID})
	if store.IsNotFound(err) {
		emails, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: c.WorkspaceID, ContactIds: []uuid.UUID{c.ContactID}})
		if err != nil {
			return nil, err
		}
		if len(emails) == 0 {
			return nil, nil
		}
		to = emails[0].Email
	} else if err != nil {
		return nil, err
	}
	if ch.SmtpHost == "" {
		return nil, errEmailNotConfigured
	}
	suppressed, err := q.IsEmailSuppressed(ctx, store.IsEmailSuppressedParams{WorkspaceID: c.WorkspaceID, Email: to})
	if err != nil {
		return nil, err
	}
	if suppressed {
		return nil, errEmailUndeliverable
	}
	return s.emailPlanFor(ctx, q, c, ch, to)
}

// sendingAddress is the From address of mail in c: for a catch-all channel the address of its
// domain that the contact wrote to.
func sendingAddress(ch store.EmailChannel, c store.Conversation) (string, error) {
	if isCatchAll(ch.Address) {
		if c.EmailAddress != nil && email.Domain(*c.EmailAddress) == email.Domain(ch.Address) && !isCatchAll(*c.EmailAddress) {
			return *c.EmailAddress, nil
		}
		if ch.FromAddress != nil {
			return *ch.FromAddress, nil
		}
		return "", errEmailNoSender
	}
	if ch.FromAddress != nil {
		return *ch.FromAddress, nil
	}
	return ch.Address, nil
}

// emailPlanFor prepares the headers of a message to `to` in c, sent through the e-mail channel ch
// and threaded with the conversation's earlier mail.
func (s *Server) emailPlanFor(ctx context.Context, q *store.Queries, c store.Conversation, ch store.EmailChannel, to string) (*emailPlan, error) {
	fromAddr, err := sendingAddress(ch, c)
	if err != nil {
		return nil, err
	}
	token, err := q.SetConversationEmailToken(ctx, store.SetConversationEmailTokenParams{
		WorkspaceID: c.WorkspaceID, ID: c.ID, Token: email.NewConversationToken(),
	})
	if err != nil {
		return nil, err
	}
	name := ch.DisplayName
	if name == "" {
		chRow, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: c.WorkspaceID, ID: ch.ChannelID})
		if err != nil {
			return nil, err
		}
		name = chRow.Name
	}
	plan := &emailPlan{
		channel: ch, channelID: ch.ChannelID, from: email.Address{Name: name, Email: fromAddr}, to: to,
		subject:  email.ReplySubject(c.Subject),
		headerID: email.NewMessageID(token, email.Domain(fromAddr)),
	}
	if plan.subject == "" {
		plan.subject = name
	}
	last, err := q.LatestThreadEmail(ctx, store.LatestThreadEmailParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID, ExcludeMessageID: uuid.Nil()})
	if err == nil {
		id := last.HeaderMessageID
		plan.inReplyTo = &id
		parents := last.ReferencesIds
		if len(parents) == 0 && last.InReplyTo != nil {
			parents = []string{*last.InReplyTo}
		}
		plan.references = email.TrimReferences(append(append([]string{}, parents...), id))
	} else if !store.IsNotFound(err) {
		return nil, err
	}
	return plan, nil
}

func (s *Server) recordOutboundEmail(ctx context.Context, q *store.Queries, plan *emailPlan, msg messageRow) error {
	refs := plan.references
	if refs == nil {
		refs = []string{}
	}
	chID := plan.channelID
	arg := store.CreateMessageEmailParams{
		WorkspaceID: msg.WorkspaceID, MessageID: msg.ID, ConversationID: msg.ConversationID, ChannelID: &chID,
		Direction: string(oas.Out), HeaderMessageID: plan.headerID, InReplyTo: plan.inReplyTo, ReferencesIds: refs,
		FromAddress: plan.from.Email, ToAddresses: plan.recipients(), CcAddresses: nonNil(plan.cc), Subject: plan.subject,
		FullText: msg.Body, FullHtml: msg.Html, Headers: []byte("{}"), Dmarc: email.DMARCUnknown, Auto: plan.auto,
		CreatedAt: msg.CreatedAt,
	}
	if d := plan.design; d != nil {
		arg.FullText, arg.FullHtml = d.text, &d.html
		arg.DesignID, arg.DesignVersion, arg.DesignLocale, arg.Outreach = &d.id, &d.version, &d.locale, &d.outreach
	}
	if err := q.CreateMessageEmail(ctx, arg); err != nil {
		return err
	}
	if plan.memberAddressID != nil {
		if err := q.SetMessageEmailMemberAddress(ctx, store.SetMessageEmailMemberAddressParams{
			WorkspaceID: msg.WorkspaceID, MessageID: msg.ID, MemberAddressID: plan.memberAddressID,
		}); err != nil {
			return err
		}
	}
	if len(plan.bcc) == 0 {
		return nil
	}
	return q.SetMessageEmailBcc(ctx, store.SetMessageEmailBccParams{WorkspaceID: msg.WorkspaceID, MessageID: msg.ID, BccAddresses: plan.bcc})
}

func (s *Server) queueEmail(ctx context.Context, q *store.Queries, events *eventBatch, plan *emailPlan, msg messageRow) (*store.ListMessageEmailsRow, error) {
	if err := s.recordOutboundEmail(ctx, q, plan, msg); err != nil {
		return nil, err
	}
	events.job(EmailSendArgs{WorkspaceID: msg.WorkspaceID, MessageID: msg.ID}, &river.InsertOpts{MaxAttempts: emailSendAttempts})
	out := &store.ListMessageEmailsRow{
		MessageID: msg.ID, Direction: string(oas.Out), HeaderMessageID: plan.headerID, FromAddress: plan.from.Email,
		ToAddresses: plan.recipients(), CcAddresses: plan.cc, BccAddresses: plan.bcc, Subject: plan.subject, Auto: plan.auto,
		Dmarc: email.DMARCUnknown,
	}
	if d := plan.design; d != nil {
		out.DesignID, out.DesignVersion, out.DesignLocale, out.DesignName = &d.id, &d.version, &d.locale, d.name
	}
	return out, nil
}

func emailSummaries(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]*store.ListMessageEmailsRow, error) {
	out := map[uuid.UUID]*store.ListMessageEmailsRow{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListMessageEmails(ctx, store.ListMessageEmailsParams{WorkspaceID: workspaceID, MessageIds: ids})
	if err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].MessageID] = &rows[i]
	}
	return out, nil
}

func emailList(in []string) []oas.Email {
	out := make([]oas.Email, len(in))
	for i, e := range in {
		out[i] = oas.Email(e)
	}
	return out
}

func withEmail(m oas.Message, e *store.ListMessageEmailsRow) oas.Message {
	if e == nil {
		return m
	}
	out := &oas.MessageEmail{
		MessageId: e.HeaderMessageID, From: oas.Email(e.FromAddress), To: emailList(e.ToAddresses), Quoted: e.Quoted,
		Raw: e.HasRaw, Auto: e.Auto, Dmarc: oas.MessageEmailDmarc(e.Dmarc), HasRemoteImages: m.Html != nil && email.HasRemoteImages(*m.Html),
		UnverifiedSender: e.UnverifiedSender,
	}
	if len(e.CcAddresses) > 0 {
		cc := emailList(e.CcAddresses)
		out.Cc = &cc
	}
	if e.Subject != "" {
		subject := e.Subject
		out.Subject = &subject
	}
	m.Email = out
	if d := emailDesign(e); d != nil {
		m.Design = d
	}
	return m
}

// withBcc adds the hidden recipients, which only answers to /v1 requests carry: events feed
// webhooks and the contact's stream, and must never show them.
func withBcc(m oas.Message, e *store.ListMessageEmailsRow) oas.Message {
	if e == nil || m.Email == nil || len(e.BccAddresses) == 0 {
		return m
	}
	bcc := emailList(e.BccAddresses)
	out := *m.Email
	out.Bcc = &bcc
	m.Email = &out
	return m
}

type EmailSendArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	MessageID   uuid.UUID `json:"message_id"`
	// Batch lists the messages sent together in one e-mail, MessageID last; empty for one message.
	Batch []uuid.UUID `json:"batch,omitempty"`
}

func (EmailSendArgs) Kind() string { return "email_send" }

func (EmailSendArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{Queue: jobs.EmailQueue} }

type emailSendWorker struct {
	river.WorkerDefaults[EmailSendArgs]
	s *Server
}

func (w *emailSendWorker) Work(ctx context.Context, job *river.Job[EmailSendArgs]) error {
	ids := job.Args.Batch
	if len(ids) == 0 {
		ids = []uuid.UUID{job.Args.MessageID}
	}
	return w.s.SendEmails(ctx, job.Args.WorkspaceID, ids, job.Attempt >= job.MaxAttempts)
}

func (w *emailSendWorker) Timeout(*river.Job[EmailSendArgs]) time.Duration { return 2 * time.Minute }

func (s *Server) AddWorkers(workers *river.Workers) {
	river.AddWorker(workers, &emailSendWorker{s: s})
	river.AddWorker(workers, &continuityWorker{s: s})
	river.AddWorker(workers, &webhookFanoutWorker{s: s})
	river.AddWorker(workers, &webhookDeliveryWorker{s: s})
	river.AddWorker(workers, &notifyWorker{s: s})
	river.AddWorker(workers, &pushWorker{s: s})
	river.AddWorker(workers, &appPushCheckWorker{s: s})
	river.AddWorker(workers, &appPushSendWorker{s: s})
	river.AddWorker(workers, &slackEventWorker{s: s})
	river.AddWorker(workers, &slackPostWorker{s: s})
	river.AddWorker(workers, &slackScrubWorker{s: s})
	river.AddWorker(workers, &slackSLAWorker{s: s})
	river.AddWorker(workers, &notificationEmailWorker{s: s})
	river.AddWorker(workers, &retentionWorker{s: s})
	river.AddWorker(workers, &workspaceDeleteWorker{s: s})
	river.AddWorker(workers, &trashEmptyWorker{s: s})
	river.AddWorker(workers, &storageDeleteWorker{s: s})
	river.AddWorker(workers, &ratingRequestWorker{s: s})
	river.AddWorker(workers, &presenceSweepWorker{s: s})
	river.AddWorker(workers, &discussionDigestWorker{s: s})
	river.AddWorker(workers, &operatorDigestWorker{s: s})
	river.AddWorker(workers, &addressLossMailWorker{s: s})
	river.AddWorker(workers, &listMailWorker{s: s})
	river.AddWorker(workers, &listImportWorker{s: s})
	river.AddWorker(workers, &helpImportWorker{s: s})
	river.AddWorker(workers, &customFieldClearWorker{s: s})
	river.AddWorker(workers, &broadcastStartWorker{s: s})
	river.AddWorker(workers, &broadcastSendWorker{s: s})
	river.AddWorker(workers, &broadcastTestWorker{s: s})
	river.AddWorker(workers, &broadcastSweepWorker{s: s})
	river.AddWorker(workers, &broadcastRepliesWorker{s: s})
	river.AddWorker(workers, &deliverabilityProbeWorker{s: s})
	river.AddWorker(workers, &migrationScanWorker{s: s})
	river.AddWorker(workers, &migrationCopyWorker{s: s})
	river.AddWorker(workers, &migrationWatchdogWorker{s: s})
	river.AddWorker(workers, &migrationSyncWorker{s: s})
	river.AddWorker(workers, &migrationRemoveWorker{s: s})
	river.AddWorker(workers, &migrationProbeWorker{s: s})
	river.AddWorker(workers, &statusMaintenanceWorker{s: s})
	river.AddWorker(workers, &statusMonitorSweepWorker{s: s})
	river.AddWorker(workers, &statusChecksCleanupWorker{s: s})
	river.AddWorker(workers, &statusCheckWorker{s: s})
	river.AddWorker(workers, &statusRoundWorker{s: s})
	river.AddWorker(workers, &statusMonitorNotifyWorker{s: s})
	river.AddWorker(workers, &statusMonitorNotifyMemberWorker{s: s})
	river.AddWorker(workers, &statusReplyWorker{s: s})
	river.AddWorker(workers, &linkedReplyWorker{s: s})
	river.AddWorker(workers, &roadmapProgressWorker{s: s})
	river.AddWorker(workers, &issueSyncWorker{s: s})
	river.AddWorker(workers, &issueSweepWorker{s: s})
	river.AddWorker(workers, &statusNoticeWorker{s: s})
	river.AddWorker(workers, &statusNoticeSweepWorker{s: s})
	river.AddWorker(workers, &changelogPublishWorker{s: s})
	river.AddWorker(workers, &changelogSweepWorker{s: s})
	river.AddWorker(workers, &rateLimitSweepWorker{s: s})
	river.AddWorker(workers, &slaCheckWorker{s: s})
	river.AddWorker(workers, &scheduledSendWorker{s: s})
	river.AddWorker(workers, &scheduledFailedWorker{s: s})
	river.AddWorker(workers, &mailForwardedWorker{s: s})
	river.AddWorker(workers, &slaNotifyWorker{s: s})
	river.AddWorker(workers, &slaSweepWorker{s: s})
	river.AddWorker(workers, &snoozeWakeWorker{s: s})
	river.AddWorker(workers, &automationRunWorker{s: s})
	river.AddWorker(workers, &automationIdleSweepWorker{s: s})
	river.AddWorker(workers, &autoAssignWorker{s: s})
	river.AddWorker(workers, &assignWaitingWorker{s: s})
	river.AddWorker(workers, &assignSweepWorker{s: s})
	river.AddWorker(workers, &reportRollupWorker{s: s})
	river.AddWorker(workers, &reportRollupScanWorker{s: s})
	river.AddWorker(workers, &ssoDomainCheckWorker{s: s})
	river.AddWorker(workers, &ssoDomainSweepWorker{s: s})
	s.addStatusDomainWorkers(workers)
	s.addAnalyticsWorkers(workers)
	s.addSESWorkers(workers)
}

func truncateRunes(v string, n int) string {
	if utf8.RuneCountInString(v) <= n {
		return v
	}
	r := []rune(v)
	return string(r[:n])
}

// SendEmail delivers a queued outgoing message through its channel's SMTP account. A temporary
// failure is returned so the job is retried, unless it is the last attempt.
func (s *Server) SendEmail(ctx context.Context, workspaceID, messageID uuid.UUID, lastAttempt bool) error {
	return s.SendEmails(ctx, workspaceID, []uuid.UUID{messageID}, lastAttempt)
}

// SendEmails delivers queued outgoing messages as one e-mail, with the headers stored for the
// last of them.
func (s *Server) SendEmails(ctx context.Context, workspaceID uuid.UUID, ids []uuid.UUID, lastAttempt bool) error {
	if live, err := s.workspaceLive(ctx, workspaceID); err != nil || !live {
		return err
	}
	var msgs []messageRow
	// Read under the messages' locks so a redaction in progress is waited for; a redacted message
	// is left out of its batch and the others still go. One redacted while the e-mail is on its
	// way is recorded as sent, as it was.
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.LockMessagesForSend(ctx, store.LockMessagesForSendParams{WorkspaceID: workspaceID, Ids: ids}); err != nil {
			return err
		}
		var last uuid.UUID
		for _, id := range ids {
			msg, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: workspaceID, ID: id})
			if store.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			if msg.RedactedAt != nil {
				continue
			}
			last = msg.ID
			if msg.DeliveryState != nil && *msg.DeliveryState == deliveryQueued {
				msgs = append(msgs, messageRow(msg))
			}
		}
		if len(msgs) == 0 || msgs[len(msgs)-1].ID != last {
			msgs = nil
		}
		return nil
	})
	if err != nil || len(msgs) == 0 {
		return err
	}
	set := func(state string, detail *string) error {
		return s.inTx(ctx, workspaceID, func(q *store.Queries, events *eventBatch) error {
			for _, m := range msgs {
				if err := s.setDeliveryTx(ctx, q, events, workspaceID, m.ID, state, detail); err != nil {
					return err
				}
			}
			return nil
		})
	}
	relayID, sendErr := s.sendQueued(ctx, workspaceID, msgs)
	if sendErr == nil {
		return s.inTx(ctx, workspaceID, func(q *store.Queries, events *eventBatch) error {
			if relayID != "" {
				if err := q.SetRelayMessageID(ctx, store.SetRelayMessageIDParams{WorkspaceID: workspaceID, MessageID: msgs[len(msgs)-1].ID, RelayMessageID: &relayID}); err != nil {
					return err
				}
			}
			for _, m := range msgs {
				if err := s.setDeliveryTx(ctx, q, events, workspaceID, m.ID, deliverySent, nil); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if !email.IsPermanent(sendErr) && !lastAttempt {
		redacted := mail.RedactError(sendErr)
		s.log.WarnContext(ctx, "email send failed, retrying", slog.String("message_id", ids[len(ids)-1].String()), slog.Any("error", redacted))
		return redacted
	}
	detail := truncateRunes(sendErr.Error(), maxDeliveryError)
	return set(deliveryFailed, &detail)
}

// sendQueued returns the Message-ID the relay put on the mail in place of ours, or "".
func (s *Server) sendQueued(ctx context.Context, workspaceID uuid.UUID, msgs []messageRow) (string, error) {
	msg := msgs[len(msgs)-1]
	me, err := s.st.GetMessageEmail(ctx, store.GetMessageEmailParams{WorkspaceID: workspaceID, MessageID: msg.ID})
	if err != nil {
		return "", fmt.Errorf("message e-mail: %w", err)
	}
	if me.ChannelID == nil {
		return "", &email.PermanentError{Err: errors.New("the e-mail channel was removed")}
	}
	ch, err := s.st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: workspaceID, ChannelID: *me.ChannelID})
	if store.IsNotFound(err) {
		return "", &email.PermanentError{Err: errors.New("the e-mail channel was removed")}
	}
	if err != nil {
		return "", err
	}
	if ch.SmtpHost == "" {
		return "", &email.PermanentError{Err: errors.New("the e-mail channel has no SMTP account")}
	}
	if msg.Kind == string(oas.MessageKindForward) {
		return s.sendForward(ctx, workspaceID, msg, me, ch)
	}
	if len(me.ToAddresses) == 0 {
		return "", &email.PermanentError{Err: errors.New("no recipient")}
	}
	suppressed, err := s.st.SuppressedEmails(ctx, store.SuppressedEmailsParams{
		WorkspaceID: workspaceID, Emails: slices.Concat(me.ToAddresses, me.CcAddresses, me.BccAddresses),
	})
	if err != nil {
		return "", err
	}
	var (
		rcpts      []string
		to, alsoTo []email.Address
		cc         []email.Address
	)
	for i, list := range [][]string{me.ToAddresses, me.CcAddresses, me.BccAddresses} {
		for _, addr := range list {
			if slices.Contains(rcpts, addr) || slices.Contains(suppressed, addr) {
				continue
			}
			rcpts = append(rcpts, addr)
			switch {
			case i == 0 && len(to) == 0:
				to = append(to, email.Address{Email: addr})
			case i == 0:
				alsoTo = append(alsoTo, email.Address{Email: addr})
			case i == 1:
				cc = append(cc, email.Address{Email: addr})
			}
		}
	}
	if len(to) == 0 {
		return "", &email.PermanentError{Err: errors.New("the recipient address is undeliverable")}
	}
	cfg, err := s.channelSMTP(workspaceID, ch)
	if err != nil {
		return "", err
	}
	conv, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: workspaceID, ID: msg.ConversationID})
	if err != nil {
		return "", err
	}
	if to[0].Email == me.ToAddresses[0] {
		if names, err := s.st.ListContactSummaries(ctx, store.ListContactSummariesParams{WorkspaceID: workspaceID, Ids: []uuid.UUID{conv.ContactID}}); err == nil && len(names) == 1 {
			to[0].Name = names[0].Name
		}
	}
	channelName := ch.DisplayName
	if channelName == "" {
		if c, err := s.st.GetChannel(ctx, store.GetChannelParams{WorkspaceID: workspaceID, ID: ch.ChannelID}); err == nil {
			channelName = c.Name
		}
	}
	replyTo := ch.Address
	if isCatchAll(replyTo) {
		replyTo = me.FromAddress
	}
	envelope := me.FromAddress
	var fromName string
	if me.MemberAddressID != nil {
		if fromName, envelope, err = s.memberSender(ctx, workspaceID, ch, conv, msg, me); err != nil {
			return "", err
		}
		if isCatchAll(ch.Address) {
			replyTo = envelope
		}
	} else if fromName, err = s.replyFromName(ctx, workspaceID, conv, msg, channelName); err != nil {
		return "", err
	}
	out := email.Outgoing{
		From:          email.Address{Name: fromName, Email: me.FromAddress},
		ReplyTo:       email.Address{Name: channelName, Email: replyTo},
		To:            to[0],
		AlsoTo:        alsoTo,
		Cc:            cc,
		Subject:       me.Subject,
		MessageID:     me.HeaderMessageID,
		Text:          batchText(msgs, false),
		Date:          msg.CreatedAt,
		References:    me.ReferencesIds,
		AutoSubmitted: me.Auto,
	}
	if msg.OnBehalfOfMemberID != nil {
		if out.Sender, err = s.delegateSender(ctx, workspaceID, ch, conv, msg); err != nil {
			return "", err
		}
	}
	if me.InReplyTo != nil {
		out.InReplyTo = *me.InReplyTo
	}
	if me.FullHtml != nil && len(msgs) == 1 {
		out.HTML = *me.FullHtml
	}
	designed := me.DesignVersion != nil && me.FullHtml != nil
	blocks := batchCards(msgs)
	if designed {
		out.Text = me.FullText
	} else if sigHTML, sigText, ok := batchSignature(msgs); ok {
		if out.HTML == "" {
			out.Text = signedText(out.Text, sigText)
			out.HTML = signedHTML(batchText(msgs, true), blocks, sigHTML)
		} else {
			out.Text, out.HTML = signedOwnHTML(out.Text, out.HTML, blocks, sigText, sigHTML)
		}
	} else if blocks != "" {
		if out.HTML == "" {
			out.HTML = email.TextToHTML(batchText(msgs, true))
		}
		out.HTML += blocks
	}
	for _, m := range msgs {
		atts, err := s.st.ListAttachmentsOfMessage(ctx, store.ListAttachmentsOfMessageParams{WorkspaceID: workspaceID, MessageID: m.ID})
		if err != nil {
			return "", err
		}
		for _, a := range atts {
			data, err := s.readObject(ctx, a.StorageKey)
			if err != nil {
				return "", err
			}
			out.Attachments = append(out.Attachments, email.Attachment{Filename: a.Filename, ContentType: a.ContentType, Data: data})
		}
	}
	if out.Headers, err = s.sesHeaders(ctx, ch, cfg); err != nil {
		return "", err
	}
	if designed && me.Outreach {
		manage := s.outreachURL(conv)
		out.Headers = append(out.Headers, email.Header{Name: "List-Unsubscribe", Value: "<" + manage + ">"},
			email.Header{Name: "List-Unsubscribe-Post", Value: "List-Unsubscribe=One-Click"})
	}
	raw, err := email.Build(out)
	if err != nil {
		return "", &email.PermanentError{Err: err}
	}
	sendCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	sendCtx, span := telemetry.Start(sendCtx, "mail.send", telemetry.WorkspaceKey.String(workspaceID.String()), telemetry.InboxKey.String(conv.InboxID.String()),
		attribute.Int("yuva.recipients", len(rcpts)), attribute.Int("yuva.message_bytes", len(raw)))
	relayID, err := email.SendRelayed(sendCtx, s.sender, cfg, envelope, rcpts, raw)
	telemetry.End(span, err)
	return relayID, err
}

func sendingMember(msg messageRow) *uuid.UUID {
	if msg.SentByApiKeyID != nil {
		return nil
	}
	if msg.SentByMemberID != nil {
		return msg.SentByMemberID
	}
	if msg.AuthorType == string(oas.AuthorTypeMember) {
		return msg.AuthorMemberID
	}
	return nil
}

// fromMember is whose name the From: carries: the mailbox owner for a delegate's mail, otherwise
// the member who sent it.
func fromMember(msg messageRow) *uuid.UUID {
	if msg.OnBehalfOfMemberID != nil {
		return msg.OnBehalfOfMemberID
	}
	return sendingMember(msg)
}

// delegateSender names the delegate who wrote msg, with the channel's own sending address so the
// Sender: stays on the From: domain and never shows the delegate's own address.
func (s *Server) delegateSender(ctx context.Context, ws uuid.UUID, ch store.EmailChannel, conv store.Conversation, msg messageRow) (email.Address, error) {
	member := sendingMember(msg)
	if member == nil {
		return email.Address{}, nil
	}
	addr, err := sendingAddress(ch, conv)
	if err != nil {
		return email.Address{}, &email.PermanentError{Err: err}
	}
	name, err := memberName(ctx, s.st.Queries, ws, *member)
	return email.Address{Name: name, Email: addr}, err
}

func (s *Server) replyFromName(ctx context.Context, ws uuid.UUID, conv store.Conversation, msg messageRow, channelName string) (string, error) {
	fromName := channelName
	if msg.AuthorType == string(oas.AuthorTypeBot) && msg.BotName != "" {
		fromName = msg.BotName
	}
	member := fromMember(msg)
	if member == nil {
		return fromName, nil
	}
	inbox, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: conv.InboxID})
	if err != nil {
		return "", err
	}
	if inbox.Purpose != string(oas.InboxPurposeMail) {
		return fromName, nil
	}
	return memberChannelName(ctx, s.st.Queries, ws, *member, channelName)
}

func memberChannelName(ctx context.Context, q *store.Queries, ws, member uuid.UUID, channelName string) (string, error) {
	name, err := memberName(ctx, q, ws, member)
	if err != nil || name == "" {
		return channelName, err
	}
	return name + " (" + channelName + ")", nil
}

// The envelope stays the channel's sending address so bounces come back to the channel; a mail
// whose address may no longer send fails for good rather than going out from another address.
func (s *Server) memberSender(ctx context.Context, ws uuid.UUID, ch store.EmailChannel, conv store.Conversation, msg messageRow, me store.MessageEmail) (string, string, error) {
	a, err := s.st.GetMemberAddressByID(ctx, store.GetMemberAddressByIDParams{WorkspaceID: ws, ID: *me.MemberAddressID})
	if store.IsNotFound(err) {
		return "", "", &email.PermanentError{Err: errors.New("the sending address " + me.FromAddress + " was removed")}
	}
	if err != nil {
		return "", "", err
	}
	if member := fromMember(msg); a.VerifiedAt == nil || a.Address != me.FromAddress || member == nil || *member != a.MemberID {
		return "", "", &email.PermanentError{Err: errors.New("the sending address " + me.FromAddress + " is no longer the sender's verified address")}
	}
	why, err := s.memberAddressUnusable(ctx, s.st.Queries, ch, a.Address)
	if err != nil {
		return "", "", err
	}
	if why != "" {
		return "", "", &email.PermanentError{Err: errors.New("the sending address " + a.Address + " may no longer send through this channel: " + why)}
	}
	envelope, err := sendingAddress(ch, conv)
	if err != nil {
		return "", "", &email.PermanentError{Err: err}
	}
	name, err := memberName(ctx, s.st.Queries, ws, a.MemberID)
	return name, envelope, err
}

func (s *Server) channelSMTP(workspaceID uuid.UUID, ch store.EmailChannel) (email.SMTPConfig, error) {
	var password string
	if len(ch.SmtpPassword) > 0 {
		plain, err := s.secrets.Open(ch.SmtpPassword, smtpSecretContext(workspaceID, ch.ChannelID))
		if err != nil {
			return email.SMTPConfig{}, &email.PermanentError{Err: errors.New("the SMTP password cannot be decrypted with this master key")}
		}
		password = string(plain)
	}
	return email.SMTPConfig{Host: ch.SmtpHost, Port: int(ch.SmtpPort), Username: ch.SmtpUsername, Password: password, TLS: ch.SmtpTls}, nil
}

// batchText joins the messages' bodies; plain leaves out the article lines that the HTML part draws as cards.
func batchText(msgs []messageRow, plain bool) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		body := m.Body
		if plain {
			body = withoutHelpCards(body, helpCardsOf(m.HelpCards))
		}
		if b := strings.TrimSpace(body); b != "" {
			parts = append(parts, b)
		}
	}
	return strings.Join(parts, "\n\n")
}

func (s *Server) readObject(ctx context.Context, key string) ([]byte, error) {
	r, err := s.objects.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func (s *Server) setDeliveryTx(ctx context.Context, q *store.Queries, events *eventBatch, workspaceID, messageID uuid.UUID, state string, detail *string) error {
	m, err := q.SetMessageDelivery(ctx, store.SetMessageDeliveryParams{
		WorkspaceID: workspaceID, ID: messageID, State: state, Error: detail, Now: s.now(),
	})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	c, err := q.GetConversation(ctx, store.GetConversationParams{WorkspaceID: workspaceID, ID: m.ConversationID})
	if err != nil {
		return err
	}
	atts, err := messageAttachments(ctx, q, workspaceID, []uuid.UUID{m.ID})
	if err != nil {
		return err
	}
	ems, err := emailSummaries(ctx, q, workspaceID, []uuid.UUID{m.ID})
	if err != nil {
		return err
	}
	events.conversation(realtime.MessageUpdated, c, withEmail(messageBody(messageRow(m), atts[m.ID]), ems[m.ID]))
	return nil
}

func (s *Server) GetMessageEmail(ctx context.Context, req oas.GetMessageEmailRequestObject) (oas.GetMessageEmailResponseObject, error) {
	p := principalFrom(ctx)
	me, err := s.visibleMessageEmail(ctx, p, req.MessageId)
	if err != nil {
		return nil, err
	}
	original := s.originalHTML(ctx, me)
	if me.DesignVersion != nil && me.FullHtml != nil {
		original = me.FullHtml
		me.FullHtml = new(s.sanitize.Sanitize(*me.FullHtml))
	}
	out := oas.GetMessageEmail200JSONResponse{
		MessageId: me.HeaderMessageID, FullText: me.FullText, FullHtml: me.FullHtml, OriginalHtml: original, InReplyTo: me.InReplyTo,
		AuthenticationResults: me.AuthenticationResults, Headers: map[string]string{},
		HasRemoteImages: me.FullHtml != nil && email.HasRemoteImages(*me.FullHtml) || original != nil && email.HasRemoteImages(*original),
	}
	if len(me.ReferencesIds) > 0 {
		refs := me.ReferencesIds
		out.References = &refs
	}
	_ = json.Unmarshal(me.Headers, &out.Headers)
	return out, nil
}

func (s *Server) originalHTML(ctx context.Context, me store.MessageEmail) *string {
	if me.RawKey == nil || me.RawSize != nil && *me.RawSize > MaxIngressBytes {
		return nil
	}
	body, err := s.objects.Open(ctx, *me.RawKey)
	if err != nil {
		return nil
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, MaxIngressBytes+1))
	if err != nil || len(raw) > MaxIngressBytes {
		return nil
	}
	m, err := email.Parse(raw)
	if err != nil || strings.TrimSpace(m.HTML) == "" || len(m.HTML) > maxFullHTMLBytes {
		return nil
	}
	clean := strings.TrimSpace(s.original.Sanitize(m.HTML))
	if clean == "" || len(clean) > maxFullHTMLBytes {
		return nil
	}
	return &clean
}

func (s *Server) visibleMessageEmail(ctx context.Context, p principal, messageID uuid.UUID) (store.MessageEmail, error) {
	me, err := s.st.GetMessageEmail(ctx, store.GetMessageEmailParams{WorkspaceID: p.workspaceID, MessageID: messageID})
	if store.IsNotFound(err) {
		return me, errMessageGone
	}
	if err != nil {
		return me, err
	}
	if _, err := readableConversation(ctx, s.st.Queries, p, me.ConversationID); err != nil {
		return me, errMessageGone
	}
	return me, nil
}

func (s *Server) DownloadMessageRaw(ctx context.Context, req oas.DownloadMessageRawRequestObject) (oas.DownloadMessageRawResponseObject, error) {
	p := principalFrom(ctx)
	me, err := s.visibleMessageEmail(ctx, p, req.MessageId)
	if err != nil {
		return nil, err
	}
	if me.RawKey == nil {
		return nil, errMessageGone
	}
	body, err := s.objects.Open(ctx, *me.RawKey)
	if err != nil {
		return nil, errMessageGone
	}
	disposition := `attachment; filename="message-` + me.MessageID.String() + `.eml"`
	nosniff := "nosniff"
	var size int64
	if me.RawSize != nil {
		size = *me.RawSize
	}
	return oas.DownloadMessageRaw200Messagerfc822Response{
		Body: body, ContentLength: size,
		Headers: oas.DownloadMessageRaw200ResponseHeaders{ContentDisposition: &disposition, XContentTypeOptions: &nosniff},
	}, nil
}

var errMessageGone = problem(http.StatusNotFound, "not_found", "no such message")

func normalizedAddressList(in []email.Address) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		if a.Email != "" {
			out = append(out, strings.ToLower(a.Email))
		}
	}
	return out
}

func batchCards(msgs []messageRow) string {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(helpCardsHTML(helpCardsOf(m.HelpCards)))
	}
	return b.String()
}

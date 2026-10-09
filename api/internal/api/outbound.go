package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
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
	channel    store.EmailChannel
	channelID  uuid.UUID
	from       email.Address
	to         string
	subject    string
	headerID   string
	inReplyTo  *string
	references []string
	auto       bool
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
	return q.CreateMessageEmail(ctx, store.CreateMessageEmailParams{
		WorkspaceID: msg.WorkspaceID, MessageID: msg.ID, ConversationID: msg.ConversationID, ChannelID: &chID,
		Direction: string(oas.Out), HeaderMessageID: plan.headerID, InReplyTo: plan.inReplyTo, ReferencesIds: refs,
		FromAddress: plan.from.Email, ToAddresses: []string{plan.to}, CcAddresses: []string{}, Subject: plan.subject,
		FullText: msg.Body, FullHtml: msg.Html, Headers: []byte("{}"), Dmarc: email.DMARCUnknown, Auto: plan.auto,
		CreatedAt: msg.CreatedAt,
	})
}

func (s *Server) queueEmail(ctx context.Context, q *store.Queries, events *eventBatch, plan *emailPlan, msg messageRow) (*store.ListMessageEmailsRow, error) {
	if err := s.recordOutboundEmail(ctx, q, plan, msg); err != nil {
		return nil, err
	}
	events.job(EmailSendArgs{WorkspaceID: msg.WorkspaceID, MessageID: msg.ID}, &river.InsertOpts{MaxAttempts: emailSendAttempts})
	return &store.ListMessageEmailsRow{
		MessageID: msg.ID, Direction: string(oas.Out), HeaderMessageID: plan.headerID, FromAddress: plan.from.Email,
		ToAddresses: []string{plan.to}, Subject: plan.subject, Auto: plan.auto, Dmarc: email.DMARCUnknown,
	}, nil
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
	return m
}

type EmailSendArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	MessageID   uuid.UUID `json:"message_id"`
	// Batch lists the messages sent together in one e-mail, MessageID last; empty for one message.
	Batch []uuid.UUID `json:"batch,omitempty"`
}

func (EmailSendArgs) Kind() string { return "email_send" }

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
	river.AddWorker(workers, &notificationEmailWorker{s: s})
	river.AddWorker(workers, &retentionWorker{s: s})
	river.AddWorker(workers, &workspaceDeleteWorker{s: s})
	river.AddWorker(workers, &ratingRequestWorker{s: s})
	river.AddWorker(workers, &presenceSweepWorker{s: s})
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
	for _, id := range ids {
		msg, err := s.st.GetMessage(ctx, store.GetMessageParams{WorkspaceID: workspaceID, ID: id})
		if store.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		if msg.DeliveryState != nil && *msg.DeliveryState == deliveryQueued {
			msgs = append(msgs, messageRow(msg))
		}
	}
	if len(msgs) == 0 || msgs[len(msgs)-1].ID != ids[len(ids)-1] {
		return nil
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
	sendErr := s.sendQueued(ctx, workspaceID, msgs)
	if sendErr == nil {
		return set(deliverySent, nil)
	}
	if !email.IsPermanent(sendErr) && !lastAttempt {
		s.log.WarnContext(ctx, "email send failed, retrying", slog.String("message_id", ids[len(ids)-1].String()), slog.Any("error", sendErr))
		return sendErr
	}
	detail := truncateRunes(sendErr.Error(), maxDeliveryError)
	return set(deliveryFailed, &detail)
}

func (s *Server) sendQueued(ctx context.Context, workspaceID uuid.UUID, msgs []messageRow) error {
	msg := msgs[len(msgs)-1]
	me, err := s.st.GetMessageEmail(ctx, store.GetMessageEmailParams{WorkspaceID: workspaceID, MessageID: msg.ID})
	if err != nil {
		return fmt.Errorf("message e-mail: %w", err)
	}
	if me.ChannelID == nil {
		return &email.PermanentError{Err: errors.New("the e-mail channel was removed")}
	}
	ch, err := s.st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: workspaceID, ChannelID: *me.ChannelID})
	if store.IsNotFound(err) {
		return &email.PermanentError{Err: errors.New("the e-mail channel was removed")}
	}
	if err != nil {
		return err
	}
	if ch.SmtpHost == "" {
		return &email.PermanentError{Err: errors.New("the e-mail channel has no SMTP account")}
	}
	if len(me.ToAddresses) == 0 {
		return &email.PermanentError{Err: errors.New("no recipient")}
	}
	to := me.ToAddresses[0]
	suppressed, err := s.st.IsEmailSuppressed(ctx, store.IsEmailSuppressedParams{WorkspaceID: workspaceID, Email: to})
	if err != nil {
		return err
	}
	if suppressed {
		return &email.PermanentError{Err: errors.New("the recipient address is undeliverable")}
	}
	var password string
	if len(ch.SmtpPassword) > 0 {
		plain, err := s.secrets.Open(ch.SmtpPassword, smtpSecretContext(workspaceID, ch.ChannelID))
		if err != nil {
			return &email.PermanentError{Err: errors.New("the SMTP password cannot be decrypted with this master key")}
		}
		password = string(plain)
	}
	conv, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: workspaceID, ID: msg.ConversationID})
	if err != nil {
		return err
	}
	toName := ""
	if names, err := s.st.ListContactSummaries(ctx, store.ListContactSummariesParams{WorkspaceID: workspaceID, Ids: []uuid.UUID{conv.ContactID}}); err == nil && len(names) == 1 {
		toName = names[0].Name
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
	fromName := channelName
	if msg.AuthorType == string(oas.AuthorTypeBot) && msg.BotName != "" {
		fromName = msg.BotName
	}
	out := email.Outgoing{
		From:          email.Address{Name: fromName, Email: me.FromAddress},
		ReplyTo:       email.Address{Name: channelName, Email: replyTo},
		To:            email.Address{Name: toName, Email: to},
		Subject:       me.Subject,
		MessageID:     me.HeaderMessageID,
		Text:          batchText(msgs),
		Date:          msg.CreatedAt,
		References:    me.ReferencesIds,
		AutoSubmitted: me.Auto,
	}
	if me.InReplyTo != nil {
		out.InReplyTo = *me.InReplyTo
	}
	if msg.Html != nil && len(msgs) == 1 {
		out.HTML = *msg.Html
	}
	for _, m := range msgs {
		atts, err := s.st.ListAttachmentsOfMessage(ctx, store.ListAttachmentsOfMessageParams{WorkspaceID: workspaceID, MessageID: m.ID})
		if err != nil {
			return err
		}
		for _, a := range atts {
			data, err := s.readObject(ctx, a.StorageKey)
			if err != nil {
				return err
			}
			out.Attachments = append(out.Attachments, email.Attachment{Filename: a.Filename, ContentType: a.ContentType, Data: data})
		}
	}
	raw, err := email.Build(out)
	if err != nil {
		return &email.PermanentError{Err: err}
	}
	cfg := email.SMTPConfig{Host: ch.SmtpHost, Port: int(ch.SmtpPort), Username: ch.SmtpUsername, Password: password, TLS: ch.SmtpTls}
	sendCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	return s.sender.Send(sendCtx, cfg, me.FromAddress, []string{to}, raw)
}

func batchText(msgs []messageRow) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if b := strings.TrimSpace(m.Body); b != "" {
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
	out := oas.GetMessageEmail200JSONResponse{
		MessageId: me.HeaderMessageID, FullText: me.FullText, FullHtml: me.FullHtml, InReplyTo: me.InReplyTo,
		AuthenticationResults: me.AuthenticationResults, Headers: map[string]string{},
		HasRemoteImages: me.FullHtml != nil && email.HasRemoteImages(*me.FullHtml),
	}
	if len(me.ReferencesIds) > 0 {
		refs := me.ReferencesIds
		out.References = &refs
	}
	_ = json.Unmarshal(me.Headers, &out.Headers)
	return out, nil
}

func (s *Server) visibleMessageEmail(ctx context.Context, p principal, messageID uuid.UUID) (store.MessageEmail, error) {
	me, err := s.st.GetMessageEmail(ctx, store.GetMessageEmailParams{WorkspaceID: p.workspaceID, MessageID: messageID})
	if store.IsNotFound(err) {
		return me, errMessageGone
	}
	if err != nil {
		return me, err
	}
	if _, err := visibleConversation(ctx, s.st.Queries, p, me.ConversationID, false); err != nil {
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

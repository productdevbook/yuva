package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/attribute"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/telemetry"
)

const (
	maxForwardRecipients      = 10
	maxForwardedMessages      = 100
	maxForwardAttachmentBytes = 20 << 20
	forwardSeparator          = "---------- Forwarded message ----------"
)

var (
	limitForwardPerActor     = limit{20, time.Hour}
	limitForwardPerWorkspace = limit{200, time.Hour}

	errForwardTooLarge   = problem(http.StatusRequestEntityTooLarge, "forward_too_large", "the forwarded files are larger than 20 MiB together; forward without attachments")
	errForwardMessage    = errValidation("message_id must be a message to or from the contact in this conversation")
	errNothingToForward  = errValidation("the conversation has no message to forward")
	errForwardNoCopies   = errValidation("a forward goes to its recipients only; it takes neither cc nor bcc")
	errForwardNoteFormat = errValidation("a forward's note is plain text; send body without html")
)

// forwardAddresses checks the recipients a caller names: 1 to 10 valid addresses that the
// workspace neither receives nor sends mail for, without repeats.
func (s *Server) forwardAddresses(ctx context.Context, ws uuid.UUID, in []oas.Email) ([]string, error) {
	if len(in) == 0 {
		return nil, errValidation("to needs at least one address")
	}
	if len(in) > maxForwardRecipients {
		return nil, errValidation("to can name at most 10 addresses")
	}
	var out []string
	for _, v := range in {
		addr, err := s.copyAddress(ctx, ws, "to", string(v))
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, addr) {
			out = append(out, addr)
		}
	}
	return out, nil
}

// checkForwardRecipients refuses the conversation's contact (a reply reaches them) and addresses
// that bounced or complained.
func checkForwardRecipients(ctx context.Context, q *store.Queries, c store.Conversation, to []string) error {
	own, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: c.WorkspaceID, ContactIds: []uuid.UUID{c.ContactID}})
	if err != nil {
		return err
	}
	parts, err := q.ListConversationParticipants(ctx, store.ListConversationParticipantsParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID})
	if err != nil {
		return err
	}
	contact, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: c.WorkspaceID, ID: c.ContactID})
	if err != nil {
		return err
	}
	for _, addr := range to {
		typed := contact.TypedEmail != nil && strings.EqualFold(*contact.TypedEmail, addr)
		if typed || slices.ContainsFunc(own, func(e store.ListContactEmailsRow) bool { return e.Email == addr }) {
			return errValidation("to: " + addr + " is the conversation's contact; reply instead")
		}
		if slices.ContainsFunc(parts, func(r store.ListConversationParticipantsRow) bool {
			return r.Email == addr && r.Role == participantCc && r.Status == participantActive
		}) {
			return errValidation("to: " + addr + " is copied on this conversation and already gets its replies; reply instead")
		}
		suppressed, err := q.IsEmailSuppressed(ctx, store.IsEmailSuppressedParams{WorkspaceID: c.WorkspaceID, Email: addr})
		if err != nil {
			return err
		}
		if suppressed {
			return problem(http.StatusConflict, "email_undeliverable", "to: "+addr+" bounced or complained; clear it on the contact to send again")
		}
	}
	return nil
}

func (s *Server) forwardLimit(ctx context.Context, p principal) error {
	return s.rateLimit(ctx,
		rateCheck{"forward:" + p.viewerID().String(), limitForwardPerActor},
		rateCheck{"forward-workspace:" + p.workspaceID.String(), limitForwardPerWorkspace})
}

// forwardSpec names what a forward of c carries: the one message, or the latest messages to and
// from the contact, and whether their files fit.
func (s *Server) forwardSpec(ctx context.Context, q *store.Queries, c store.Conversation, messageID *uuid.UUID, to []string, attachments bool, now time.Time) (oas.MessageForward, error) {
	spec := oas.MessageForward{Scope: oas.MessageForwardScopeConversation, To: emailList(to), Attachments: attachments}
	if messageID != nil {
		m, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: c.WorkspaceID, ID: *messageID})
		if store.IsNotFound(err) {
			return spec, errForwardMessage
		}
		if err != nil {
			return spec, err
		}
		if m.ConversationID != c.ID || m.Kind != string(oas.MessageKindMessage) || m.Draft || m.RedactedAt != nil {
			return spec, errForwardMessage
		}
		spec.Scope, spec.MessageIds = oas.MessageForwardScopeMessage, []uuid.UUID{m.ID}
	} else {
		ids, err := q.ListForwardableMessageIDs(ctx, store.ListForwardableMessageIDsParams{
			WorkspaceID: c.WorkspaceID, ConversationID: c.ID, Before: now, Lim: maxForwardedMessages,
		})
		if err != nil {
			return spec, err
		}
		if len(ids) == 0 {
			return spec, errNothingToForward
		}
		spec.MessageIds = ids
	}
	if attachments {
		atts, err := q.ListAttachments(ctx, store.ListAttachmentsParams{WorkspaceID: c.WorkspaceID, MessageIds: spec.MessageIds})
		if err != nil {
			return spec, err
		}
		var total int64
		for _, a := range atts {
			total += a.SizeBytes
		}
		if total > maxForwardAttachmentBytes {
			return spec, errForwardTooLarge
		}
	}
	return spec, nil
}

// forwardChannel is the e-mail channel a forward of c goes out through: the conversation's own,
// else the inbox's.
func forwardChannel(ctx context.Context, q *store.Queries, c store.Conversation) (store.EmailChannel, error) {
	if c.ChannelID != nil {
		ch, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: c.WorkspaceID, ChannelID: *c.ChannelID})
		if err == nil || !store.IsNotFound(err) {
			return ch, err
		}
	}
	ch, err := q.InboxEmailChannel(ctx, store.InboxEmailChannelParams{WorkspaceID: c.WorkspaceID, InboxID: c.InboxID})
	if store.IsNotFound(err) {
		return ch, errEmailNotConfigured
	}
	return ch, err
}

// forwardPlan prepares a forward's headers: a new Message-ID of the conversation that names no
// earlier mail, so recipients learn nothing of the thread's ids.
func (s *Server) forwardPlan(ctx context.Context, q *store.Queries, c store.Conversation, to []string) (*emailPlan, error) {
	ch, err := forwardChannel(ctx, q, c)
	if err != nil {
		return nil, err
	}
	if ch.SmtpHost == "" {
		return nil, errEmailNotConfigured
	}
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
	name, err := s.channelName(ctx, q, ch)
	if err != nil {
		return nil, err
	}
	subject := email.ForwardSubject(c.Subject)
	if subject == "" {
		subject = email.ForwardSubject(name)
	}
	return &emailPlan{
		channel: ch, channelID: ch.ChannelID, from: email.Address{Name: name, Email: fromAddr}, to: to[0], toAll: to,
		subject: subject, headerID: email.NewMessageID(token, email.Domain(fromAddr)),
	}, nil
}

func (s *Server) channelName(ctx context.Context, q *store.Queries, ch store.EmailChannel) (string, error) {
	if ch.DisplayName != "" {
		return ch.DisplayName, nil
	}
	row, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: ch.WorkspaceID, ID: ch.ChannelID})
	if err != nil {
		return "", err
	}
	return row.Name, nil
}

func (s *Server) ForwardConversation(ctx context.Context, req oas.ForwardConversationRequestObject) (oas.ForwardConversationResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	draft := b.Draft != nil && *b.Draft
	if err := requireScope(p, oas.ConversationsRead); err != nil {
		return nil, err
	}
	if err := requireScope(p, oas.MessagesWrite); err != nil {
		return nil, err
	}
	if !draft && p.deliversAsBot() && !p.botsMaySend {
		return nil, errBotSendingDisabled
	}
	note := ""
	if b.Note != nil {
		note = *b.Note
	}
	if len([]rune(note)) > maxMessageBodyRunes {
		return nil, errValidation("note must be at most 65536 characters")
	}
	to, err := s.forwardAddresses(ctx, p.workspaceID, b.To)
	if err != nil {
		return nil, err
	}
	attachments := b.Attachments == nil || *b.Attachments
	c, err := visibleConversation(ctx, s.st.Queries, p, req.ConversationId, false)
	if err != nil {
		return nil, err
	}
	if err := checkForwardRecipients(ctx, s.st.Queries, c, to); err != nil {
		return nil, err
	}
	if !draft {
		if err := s.forwardLimit(ctx, p); err != nil {
			return nil, err
		}
	}
	var out oas.Message
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := visibleConversation(ctx, q, p, req.ConversationId, true)
		if err != nil {
			return err
		}
		if err := checkForwardRecipients(ctx, q, c, to); err != nil {
			return err
		}
		now := s.now()
		spec, err := s.forwardSpec(ctx, q, c, b.MessageId, to, attachments, now)
		if err != nil {
			return err
		}
		var plan *emailPlan
		if !draft {
			if plan, err = s.forwardPlan(ctx, q, c, to); err != nil {
				return err
			}
		}
		direction := string(oas.Out)
		arg := store.CreateMessageParams{
			ID: newID(), WorkspaceID: p.workspaceID, ConversationID: c.ID, Kind: string(oas.MessageKindForward), Direction: &direction,
			Body: note, CreatedAt: now, Draft: draft, Via: p.viaClient(), Forward: mustJSON(spec),
		}
		arg.AuthorType, arg.AuthorMemberID, arg.AuthorApiKeyID = authorFor(p)
		if plan != nil {
			queued := deliveryQueued
			arg.DeliveryState = &queued
		}
		msg, err := q.CreateMessage(ctx, arg)
		if err != nil {
			return err
		}
		if draft {
			out = messageBody(msg, nil)
			events.conversation(realtime.DraftCreated, c, out)
			return nil
		}
		out, err = s.deliverForward(ctx, q, events, p, c, plan, msg, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.ForwardConversation201JSONResponse(out), nil
}

// deliverForward queues a forward's e-mail and announces it to members.
func (s *Server) deliverForward(ctx context.Context, q *store.Queries, events *eventBatch, p principal, c store.Conversation, plan *emailPlan, msg messageRow, now time.Time) (oas.Message, error) {
	summary, err := s.queueEmail(ctx, q, events, plan, msg)
	if err != nil {
		return oas.Message{}, err
	}
	if err := q.TouchConversation(ctx, store.TouchConversationParams{WorkspaceID: c.WorkspaceID, ID: c.ID, Now: now}); err != nil {
		return oas.Message{}, err
	}
	if err := s.addUsage(ctx, q, c.WorkspaceID, 0, 1, 0); err != nil {
		return oas.Message{}, err
	}
	out := withEmail(messageBody(msg, nil), summary)
	events.conversation(realtime.MessageCreated, c, out)
	if !p.isKey() && msg.AuthorMemberID != nil {
		if _, err := s.moveReadCursor(ctx, q, events, c.WorkspaceID, p.memberID, c, msg.ID, msg.CreatedAt); err != nil {
			return out, err
		}
	}
	return out, nil
}

func forwardOf(m messageRow) (oas.MessageForward, bool) {
	var spec oas.MessageForward
	if len(m.Forward) == 0 || json.Unmarshal(m.Forward, &spec) != nil {
		return spec, false
	}
	return spec, true
}

// sendForwardDraft sends a draft forward: its recipients, message and files are checked again, and
// a forward of the conversation takes the conversation as it is now.
func (s *Server) sendForwardDraft(ctx context.Context, p principal, req oas.SendMessageRequestObject) (oas.SendMessageResponseObject, error) {
	if req.Body != nil && (req.Body.Cc != nil || req.Body.Bcc != nil) {
		return nil, errForwardNoCopies
	}
	if err := requireScope(p, oas.ConversationsRead); err != nil {
		return nil, err
	}
	cur, err := s.st.GetMessage(ctx, store.GetMessageParams{WorkspaceID: p.workspaceID, ID: req.MessageId})
	if err != nil {
		return nil, err
	}
	if draftHidden(p, cur) {
		return nil, errMessageGone
	}
	c, err := visibleConversation(ctx, s.st.Queries, p, cur.ConversationID, false)
	if err == errConversationGone {
		return nil, errMessageGone
	}
	if err != nil {
		return nil, err
	}
	if !cur.Draft {
		return nil, errNotADraft
	}
	spec, ok := forwardOf(messageRow(cur))
	if !ok {
		return nil, errNothingToForward
	}
	to, err := s.forwardAddresses(ctx, p.workspaceID, spec.To)
	if err != nil {
		return nil, err
	}
	if err := checkForwardRecipients(ctx, s.st.Queries, c, to); err != nil {
		return nil, err
	}
	if err := s.forwardLimit(ctx, p); err != nil {
		return nil, err
	}
	var out oas.Message
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := lockDraft(ctx, q, p, req.MessageId)
		if err != nil {
			return err
		}
		if err := checkForwardRecipients(ctx, q, c, to); err != nil {
			return err
		}
		now := s.now()
		var one *uuid.UUID
		if spec.Scope == oas.MessageForwardScopeMessage && len(spec.MessageIds) == 1 {
			one = &spec.MessageIds[0]
		}
		fresh, err := s.forwardSpec(ctx, q, c, one, to, spec.Attachments, now)
		if err != nil {
			return err
		}
		plan, err := s.forwardPlan(ctx, q, c, to)
		if err != nil {
			return err
		}
		if err := q.SetMessageForward(ctx, store.SetMessageForwardParams{WorkspaceID: p.workspaceID, ID: req.MessageId, Forward: mustJSON(fresh)}); err != nil {
			return err
		}
		queued := deliveryQueued
		arg := store.SendDraftParams{WorkspaceID: p.workspaceID, ID: req.MessageId, Now: now, SentVia: p.viaClient(), DeliveryState: &queued}
		if p.isKey() {
			arg.SentByApiKeyID = &p.keyID
		} else {
			arg.SentByMemberID = &p.memberID
		}
		sent, err := q.SendDraft(ctx, arg)
		if err != nil {
			return err
		}
		if c, err = revealDraftConversation(ctx, q, events, c); err != nil {
			return err
		}
		out, err = s.deliverForward(ctx, q, events, p, c, plan, messageRow(sent), now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.SendMessage200JSONResponse(out), nil
}

// sendForward e-mails a queued forward to its recipients that can still receive mail.
func (s *Server) sendForward(ctx context.Context, workspaceID uuid.UUID, msg messageRow, me store.MessageEmail, ch store.EmailChannel) (string, error) {
	spec, ok := forwardOf(msg)
	if !ok {
		return "", &email.PermanentError{Err: errors.New("the forward names nothing to send")}
	}
	var rcpts []string
	for _, addr := range me.ToAddresses {
		suppressed, err := s.st.IsEmailSuppressed(ctx, store.IsEmailSuppressedParams{WorkspaceID: workspaceID, Email: addr})
		if err != nil {
			return "", err
		}
		if !suppressed && !slices.Contains(rcpts, addr) {
			rcpts = append(rcpts, addr)
		}
	}
	if len(rcpts) == 0 {
		return "", &email.PermanentError{Err: errors.New("every recipient address is undeliverable")}
	}
	cfg, err := s.channelSMTP(workspaceID, ch)
	if err != nil {
		return "", err
	}
	conv, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: workspaceID, ID: msg.ConversationID})
	if err != nil {
		return "", err
	}
	channelName, err := s.channelName(ctx, s.st.Queries, ch)
	if err != nil {
		return "", err
	}
	fromName := channelName
	sender := msg.AuthorMemberID
	if msg.SentByMemberID != nil {
		sender = msg.SentByMemberID
	}
	switch {
	case msg.SentByApiKeyID != nil:
		if msg.SentByBotName != "" {
			fromName = msg.SentByBotName
		}
	case sender != nil:
		inbox, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: workspaceID, ID: conv.InboxID})
		if err != nil {
			return "", err
		}
		if inbox.Purpose == string(oas.InboxPurposeMail) {
			if fromName, err = memberChannelName(ctx, s.st.Queries, workspaceID, *sender, channelName); err != nil {
				return "", err
			}
			break
		}
		name, err := memberName(ctx, s.st.Queries, workspaceID, *sender)
		if err != nil {
			return "", err
		}
		if name != "" {
			fromName = name
		}
	case msg.AuthorType == string(oas.AuthorTypeBot) && msg.BotName != "":
		fromName = msg.BotName
	}
	replyTo := ch.Address
	if isCatchAll(replyTo) {
		replyTo = me.FromAddress
	}
	text, html, files, err := s.composeForward(ctx, workspaceID, conv, spec, msg.Body, channelName)
	if err != nil {
		return "", err
	}
	out := email.Outgoing{
		From:        email.Address{Name: fromName, Email: me.FromAddress},
		ReplyTo:     email.Address{Name: channelName, Email: replyTo},
		To:          email.Address{Email: rcpts[0]},
		Subject:     me.Subject,
		MessageID:   me.HeaderMessageID,
		Text:        text,
		HTML:        html,
		Date:        msg.CreatedAt,
		Attachments: files,
	}
	for _, addr := range rcpts[1:] {
		out.AlsoTo = append(out.AlsoTo, email.Address{Email: addr})
	}
	if out.Headers, err = s.sesHeaders(ctx, ch, cfg); err != nil {
		return "", err
	}
	raw, err := email.Build(out)
	if err != nil {
		return "", &email.PermanentError{Err: err}
	}
	sendCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	sendCtx, span := telemetry.Start(sendCtx, "mail.send", telemetry.WorkspaceKey.String(workspaceID.String()), telemetry.InboxKey.String(conv.InboxID.String()),
		attribute.Int("yuva.recipients", len(rcpts)), attribute.Int("yuva.message_bytes", len(raw)))
	relayID, err := email.SendRelayed(sendCtx, s.sender, cfg, me.FromAddress, rcpts, raw)
	telemetry.End(span, err)
	return relayID, err
}

// composeForward writes the forward's text and HTML: the note, then each forwarded message that is
// still there and not redacted under a header naming its sender, date and subject. Everything is
// written as text and escaped into the HTML, so nothing of the original markup goes out.
func (s *Server) composeForward(ctx context.Context, ws uuid.UUID, c store.Conversation, spec oas.MessageForward, note, channelName string) (string, string, []email.Attachment, error) {
	var msgs []store.GetMessageRow
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.LockMessagesShared(ctx, store.LockMessagesSharedParams{WorkspaceID: ws, Ids: spec.MessageIds}); err != nil {
			return err
		}
		for _, id := range spec.MessageIds {
			m, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: ws, ID: id})
			if store.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			if m.ConversationID != c.ID || m.Kind != string(oas.MessageKindMessage) || m.Draft || m.RedactedAt != nil {
				continue
			}
			msgs = append(msgs, m)
		}
		return nil
	})
	if err != nil {
		return "", "", nil, err
	}
	ids := make([]uuid.UUID, len(msgs))
	var contacts []uuid.UUID
	for i, m := range msgs {
		ids[i] = m.ID
		if m.AuthorContactID != nil && !slices.Contains(contacts, *m.AuthorContactID) {
			contacts = append(contacts, *m.AuthorContactID)
		}
	}
	emails, err := emailSummaries(ctx, s.st.Queries, ws, ids)
	if err != nil {
		return "", "", nil, err
	}
	names := map[uuid.UUID]store.ListContactSummariesRow{}
	if len(contacts) > 0 {
		rows, err := s.st.ListContactSummaries(ctx, store.ListContactSummariesParams{WorkspaceID: ws, Ids: contacts})
		if err != nil {
			return "", "", nil, err
		}
		for _, r := range rows {
			names[r.ID] = r
		}
	}
	var text, html strings.Builder
	if note = strings.TrimSpace(note); note != "" {
		text.WriteString(note)
		text.WriteString("\n\n")
		html.WriteString(email.TextToHTML(note))
		html.WriteString("<br>\n")
	}
	for i, m := range msgs {
		e := emails[m.ID]
		body := m.Body
		if spec.Scope == oas.MessageForwardScopeMessage && e != nil && e.Direction == string(oas.In) {
			if full, err := s.st.GetMessageEmail(ctx, store.GetMessageEmailParams{WorkspaceID: ws, MessageID: m.ID}); err == nil && strings.TrimSpace(full.FullText) != "" {
				body = full.FullText
			}
		}
		head := forwardHeader(m, e, names, channelName)
		if i > 0 {
			text.WriteString("\n\n")
		}
		text.WriteString(head)
		text.WriteString("\n\n")
		text.WriteString(strings.TrimSpace(body))
		html.WriteString(`<div>`)
		html.WriteString(email.TextToHTML(head))
		html.WriteString(`</div><blockquote style="margin:0 0 0 .8ex;border-left:1px solid #ccc;padding-left:1ex">`)
		html.WriteString(email.TextToHTML(strings.TrimSpace(body)))
		html.WriteString("</blockquote>\n")
	}
	var files []email.Attachment
	if spec.Attachments && len(ids) > 0 {
		atts, err := s.st.ListAttachments(ctx, store.ListAttachmentsParams{WorkspaceID: ws, MessageIds: ids})
		if err != nil {
			return "", "", nil, err
		}
		var total int64
		for _, a := range atts {
			if total += a.SizeBytes; total > maxForwardAttachmentBytes {
				return "", "", nil, &email.PermanentError{Err: errors.New("the forwarded files are larger than 20 MiB together")}
			}
			data, err := s.readObject(ctx, a.StorageKey)
			if err != nil {
				return "", "", nil, err
			}
			files = append(files, email.Attachment{Filename: a.Filename, ContentType: a.ContentType, Data: data})
		}
	}
	return text.String(), html.String(), files, nil
}

func forwardHeader(m store.GetMessageRow, e *store.ListMessageEmailsRow, names map[uuid.UUID]store.ListContactSummariesRow, channelName string) string {
	var from email.Address
	if m.Direction != nil && *m.Direction == string(oas.In) {
		if m.AuthorContactID != nil {
			from = email.Address{Name: names[*m.AuthorContactID].Name, Email: names[*m.AuthorContactID].Email}
		}
	} else {
		from.Name = channelName
	}
	if e != nil && e.FromAddress != "" {
		from.Email = e.FromAddress
	}
	lines := []string{forwardSeparator}
	switch {
	case from.Name != "" && from.Email != "":
		lines = append(lines, "From: "+from.Name+" <"+from.Email+">")
	case from.Email != "":
		lines = append(lines, "From: "+from.Email)
	case from.Name != "":
		lines = append(lines, "From: "+from.Name)
	}
	lines = append(lines, "Date: "+m.CreatedAt.UTC().Format("Mon, 2 Jan 2006 15:04:05 -0700"))
	if e != nil {
		if e.Subject != "" {
			lines = append(lines, "Subject: "+e.Subject)
		}
		if len(e.ToAddresses) > 0 {
			lines = append(lines, "To: "+strings.Join(e.ToAddresses, ", "))
		}
		if len(e.CcAddresses) > 0 {
			lines = append(lines, "Cc: "+strings.Join(e.CcAddresses, ", "))
		}
	}
	return strings.Join(lines, "\n")
}

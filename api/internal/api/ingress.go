package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/email/reply"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	MaxIngressBytes           = 25 << 20
	newConversationsPerHour   = 20
	defaultSenderHourlyCap    = 500
	maxFullHTMLBytes          = 2 << 20
	defaultIngestConcurrency  = 8
	defaultIngressReadTimeout = 30 * time.Second
)

type IngestError struct {
	Status int
	Code   oas.IngressRejectionCode
	Reason string
}

func (e *IngestError) Error() string { return string(e.Code) + ": " + e.Reason }

var (
	errIngestNotConfigured = &IngestError{http.StatusServiceUnavailable, oas.NotConfigured, "Mail for this server is not set up yet"}
	errIngestUnavailable   = &IngestError{http.StatusServiceUnavailable, oas.Unavailable, "Temporary failure, try again later"}
	errIngestBusy          = &IngestError{http.StatusServiceUnavailable, oas.Unavailable, "Busy, try again later"}
	errIngestTooLarge      = &IngestError{http.StatusRequestEntityTooLarge, oas.TooLarge, "Message too large"}
	errIngestBadSignature  = &IngestError{http.StatusUnauthorized, oas.BadSignature, "Rejected by recipient server"}
	errIngestStale         = &IngestError{http.StatusUnauthorized, oas.StaleTimestamp, "Rejected by recipient server"}
	errIngestUnknown       = &IngestError{http.StatusNotFound, oas.UnknownRecipient, "No such recipient"}
	errIngestBlocked       = &IngestError{http.StatusForbidden, oas.BlockedSender, "Messages from this sender are not accepted"}
	errIngestMalformed     = &IngestError{http.StatusBadRequest, oas.Malformed, "The message could not be read"}
	errIngestRateLimited   = &IngestError{http.StatusTooManyRequests, oas.RateLimited, "Too many messages from this sender in the last hour; this message was not accepted"}
)

const (
	IngestStored    = string(oas.IngressResultStatusStored)
	IngestDuplicate = string(oas.IngressResultStatusDuplicate)
	IngestBounce    = string(oas.IngressResultStatusBounce)
	IngestDropped   = string(oas.IngressResultStatusDropped)
)

type IngestResult struct {
	Status         string
	ConversationID *uuid.UUID
	MessageID      *uuid.UUID
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeIngestError(w http.ResponseWriter, e *IngestError) {
	writeJSON(w, e.Status, oas.IngressRejection{Code: e.Code, Reason: e.Reason})
}

func (s *Server) serveIngressEmail(w http.ResponseWriter, r *http.Request) {
	if s.ingress.Secret == "" {
		writeIngestError(w, errIngestNotConfigured)
		return
	}
	if r.ContentLength > MaxIngressBytes {
		writeIngestError(w, errIngestTooLarge)
		return
	}
	to, from := r.Header.Get("X-Yuva-Envelope-To"), r.Header.Get("X-Yuva-Envelope-From")
	legacy := email.Legacy{V1: s.ingress.AcceptV1, V2: s.ingress.AcceptV2}
	check, err := email.StartIngress(s.ingress.Secret, r.Header.Get("X-Yuva-Timestamp"), to, from, r.Header.Get("X-Yuva-Signature"), r.ContentLength, s.now(), legacy)
	switch {
	case errors.Is(err, email.ErrStaleTimestamp):
		writeIngestError(w, errIngestStale)
		return
	case err != nil:
		writeIngestError(w, errIngestBadSignature)
		return
	}
	select {
	case s.ingestQ <- struct{}{}:
		defer func() { <-s.ingestQ }()
	default:
		writeIngestError(w, errIngestBusy)
		return
	}
	readTimeout := s.ingress.ReadTimeout
	if readTimeout <= 0 {
		readTimeout = defaultIngressReadTimeout
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(readTimeout))
	body, err := io.ReadAll(io.TeeReader(io.LimitReader(r.Body, MaxIngressBytes+1), check))
	// An expired deadline would cancel the request context once the server reads ahead for the next request.
	_ = rc.SetReadDeadline(time.Time{})
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || len(body) > MaxIngressBytes {
		writeIngestError(w, errIngestTooLarge)
		return
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		writeIngestError(w, errIngestUnavailable)
		return
	}
	if err != nil {
		writeIngestError(w, errIngestMalformed)
		return
	}
	if err := check.Verify(); err != nil {
		writeIngestError(w, errIngestBadSignature)
		return
	}
	res, err := s.IngestEmail(r.Context(), to, from, !check.V1(), body)
	var ie *IngestError
	if errors.As(err, &ie) {
		writeIngestError(w, ie)
		return
	}
	if err != nil {
		s.log.ErrorContext(r.Context(), "ingest email", slog.Any("error", err))
		writeIngestError(w, errIngestUnavailable)
		return
	}
	out := oas.IngressResult{Status: oas.IngressResultStatus(res.Status), ConversationId: res.ConversationID, MessageId: res.MessageID}
	status := http.StatusAccepted
	if res.Status == IngestDuplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, out)
}

// findEmailChannel returns the channel for the envelope recipient and the recipient address itself,
// normalized: on the forwarding domain the channel whose token is the local part, else the exact
// address, then local@ for local+tag@, then the domain's catch-all channel. forwarded reports a
// recipient that matched a forwarding token.
func (s *Server) findEmailChannel(ctx context.Context, to string) (ch store.FindEmailChannelByAddressRow, addr string, forwarded bool, err error) {
	addr = strings.ToLower(strings.Trim(strings.TrimSpace(to), "<>"))
	local, domain, ok := strings.Cut(addr, "@")
	if ok && s.ingress.ForwardingDomain != "" && domain == s.ingress.ForwardingDomain {
		token, _, _ := strings.Cut(local, "+")
		row, err := s.st.FindEmailChannelByForwardToken(ctx, token)
		if err == nil {
			return store.FindEmailChannelByAddressRow(row), addr, true, nil
		}
		if !store.IsNotFound(err) {
			return ch, addr, false, err
		}
	}
	ch, err = s.st.FindEmailChannelByAddress(ctx, addr)
	if store.IsNotFound(err) {
		if base, _, tagged := strings.Cut(local, "+"); ok && tagged {
			ch, err = s.st.FindEmailChannelByAddress(ctx, base+"@"+domain)
		}
	}
	if store.IsNotFound(err) && ok && local != "" && local != catchAllLocal && domain != "" {
		ch, err = s.st.FindEmailChannelByAddress(ctx, catchAllLocal+"@"+domain)
	}
	if store.IsNotFound(err) {
		return ch, addr, false, errIngestUnknown
	}
	return ch, addr, false, err
}

func (s *Server) ownSender(ctx context.Context, ch store.FindEmailChannelByAddressRow, sender string) (bool, error) {
	if sender == "" {
		return false, nil
	}
	if sender == ch.Address || (ch.FromAddress != nil && sender == *ch.FromAddress) || slices.Contains(s.ingress.OwnAddresses, sender) {
		return true, nil
	}
	return s.st.IsEmailChannelSender(ctx, sender)
}

func nullSender(envelopeFrom string) bool {
	v := strings.TrimSpace(envelopeFrom)
	return v == "" || v == "<>"
}

// ownMessageID reports mail that carries a Message-ID we generated for this channel's domain:
// our own outbound mail coming back.
func ownMessageID(id string, ch store.FindEmailChannelByAddressRow) bool {
	_, domain, ok := email.TokenFromID(id)
	if !ok {
		return false
	}
	if domain == email.Domain(ch.Address) {
		return true
	}
	return ch.FromAddress != nil && domain == email.Domain(*ch.FromAddress)
}

type storedFile struct {
	id          uuid.UUID
	key         string
	filename    string
	contentType string
	size        int64
	contentID   *string
	inline      bool
}

func (s *Server) inboundAttachments(ctx context.Context, workspaceID uuid.UUID, atts []email.Attachment) ([]storedFile, error) {
	out, _, err := s.keepAttachments(ctx, workspaceID, atts, func(int) uuid.UUID { return newID() })
	return out, err
}

// keepAttachments stores the attachments the server keeps, at most 50, under the ids idFor
// gives by position, and reports how many it left out by size or type.
func (s *Server) keepAttachments(ctx context.Context, workspaceID uuid.UUID, atts []email.Attachment, idFor func(int) uuid.UUID) ([]storedFile, int, error) {
	var out []storedFile
	skipped := 0
	for i, a := range atts {
		if len(out) == 50 {
			skipped += len(atts) - i
			break
		}
		size := int64(len(a.Data))
		if size == 0 {
			continue
		}
		if size > s.attach.MaxBytes {
			skipped++
			continue
		}
		ct, err := s.contentType(a.ContentType, bytes.NewReader(a.Data))
		if err != nil {
			var apiErr *apiError
			if errors.As(err, &apiErr) {
				skipped++
				continue
			}
			return out, skipped, err
		}
		name := a.Filename
		if name == "" {
			name = "attachment-" + strconv.Itoa(i+1)
			if exts, _ := mime.ExtensionsByType(ct); len(exts) > 0 {
				name += exts[0]
			}
		}
		f := storedFile{id: idFor(i), filename: cleanFilename(name), contentType: ct, size: size, inline: a.Inline}
		if cid := truncateRunes(a.ContentID, 998); cid != "" {
			f.contentID = &cid
		}
		f.key = workspaceID.String() + "/attachments/" + f.id.String()
		if err := s.objects.Put(ctx, f.key, bytes.NewReader(a.Data), size, ct); err != nil {
			return out, skipped, err
		}
		out = append(out, f)
	}
	return out, skipped, nil
}

// mailContent is what ingest stores of a parsed message besides its addresses: the visible
// text and HTML with quotes removed and sanitised, and the full message.
type mailContent struct {
	visible   string
	html      *string
	fullHTML  *string
	quoted    bool
	subject   string
	headers   []byte
	inReplyTo *string
	refs      []string
}

func (s *Server) mailContent(m *email.Message) mailContent {
	visible, textQuoted := reply.Strip(m.Text)
	out := mailContent{visible: truncateRunes(visible, maxMessageBodyRunes), quoted: textQuoted}
	if m.HTML != "" {
		stripped, removed := email.StripHTMLQuotes(m.HTML)
		out.quoted = out.quoted || removed
		out.html = s.sanitizeHTML(stripped, maxMessageHTMLBytes)
	}
	out.fullHTML = s.sanitizeHTML(m.HTML, maxFullHTMLBytes)
	out.subject = truncateRunes(strings.Join(strings.Fields(m.Subject), " "), 500)
	out.headers, _ = json.Marshal(m.Headers)
	if len(m.InReplyTo) > 0 {
		out.inReplyTo = &m.InReplyTo[0]
	}
	out.refs = m.References
	if out.refs == nil {
		out.refs = []string{}
	}
	return out
}

func (s *Server) sanitizeHTML(v string, limit int) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	clean := strings.TrimSpace(s.sanitize.Sanitize(v))
	if clean == "" || len(clean) > limit {
		return nil
	}
	return &clean
}

// IngestEmail stores one inbound message for the channel that receives mail at envelopeTo. It is
// called by /ingress/email after the signature check and by `yuva ingest-email`. An envelope sender
// that is not fromTrusted (a deprecated v1 signature does not cover it) never marks the message as
// a delivery report or automatic.
func (s *Server) IngestEmail(ctx context.Context, envelopeTo, envelopeFrom string, fromTrusted bool, raw []byte) (IngestResult, error) {
	if len(raw) > MaxIngressBytes {
		return IngestResult{}, errIngestTooLarge
	}
	ch, recipient, forwarded, err := s.findEmailChannel(ctx, envelopeTo)
	if err != nil {
		return IngestResult{}, err
	}
	m, err := email.Parse(raw)
	if err != nil {
		return IngestResult{}, errIngestMalformed
	}
	if probe, err := s.takeProbe(ctx, ch.WorkspaceID, m); err != nil {
		return IngestResult{}, err
	} else if probe {
		return IngestResult{Status: IngestDropped}, nil
	}
	route := verifiedDirect
	if forwarded {
		route = verifiedForwarding
	}
	if probe, err := s.takeMigrationProbe(ctx, ch, m, route); err != nil {
		return IngestResult{}, err
	} else if probe {
		return IngestResult{Status: IngestDropped}, nil
	}
	if forwarded {
		if taken, err := s.takeForwardingConfirmation(ctx, ch, m); err != nil {
			return IngestResult{}, err
		} else if taken {
			return IngestResult{Status: IngestDropped}, nil
		}
	}
	return s.ingestToChannel(ctx, ch, recipient, envelopeFrom, fromTrusted, raw, m, ingestOptions{})
}

// ingestOptions: noGreeting leaves out the channel's auto-reply, for new mail catch-up sync found
// in a mailbox whose provider may have answered it already.
type ingestOptions struct {
	noGreeting bool
}

// ingestToChannel stores a parsed inbound message for the channel as live mail.
func (s *Server) ingestToChannel(ctx context.Context, ch store.FindEmailChannelByAddressRow, recipient, envelopeFrom string, fromTrusted bool, raw []byte, m *email.Message, opts ingestOptions) (IngestResult, error) {
	sender := m.From.Email
	if sender == "" {
		if a, ok := email.ParseAddress(envelopeFrom); ok {
			sender = a.Email
		}
	}
	if own, err := s.ownSender(ctx, ch, sender); err != nil {
		return IngestResult{}, err
	} else if own {
		return IngestResult{Status: IngestDropped}, nil
	}
	if err := s.refuseBlockedSender(ctx, ch.WorkspaceID, sender); err != nil {
		return IngestResult{}, err
	}
	nullFrom := fromTrusted && nullSender(envelopeFrom)
	if m.Feedback != nil {
		res, handled, err := s.applyFeedbackReport(ctx, ch, m)
		if err != nil || handled {
			return res, err
		}
	}
	if m.Bounce != nil && nullFrom {
		res, handled, err := s.applyDSN(ctx, ch, m)
		if err != nil || handled {
			return res, err
		}
		if sender == "" {
			return IngestResult{Status: IngestDropped}, nil
		}
	}
	if sender == "" {
		return IngestResult{}, errIngestMalformed
	}
	auto := m.Auto || nullFrom || ownMessageID(m.MessageID, ch)
	if auto {
		if counted, err := s.broadcastAutoReply(ctx, ch, m); err != nil || counted {
			return IngestResult{Status: IngestDropped}, err
		}
	}
	ws := ch.WorkspaceID
	headerID := m.MessageID
	if headerID == "" {
		headerID = email.SynthesizedID(raw)
	}
	if prev, err := s.st.FindInboundEmailByHeader(ctx, store.FindInboundEmailByHeaderParams{WorkspaceID: ws, ChannelID: &ch.ChannelID, HeaderMessageID: headerID}); err == nil {
		return IngestResult{Status: IngestDuplicate, ConversationID: &prev.ConversationID, MessageID: &prev.MessageID}, nil
	} else if !store.IsNotFound(err) {
		return IngestResult{}, err
	}

	rawKey := ws.String() + "/raw/" + newID().String() + ".eml"
	if err := s.objects.Put(ctx, rawKey, bytes.NewReader(raw), int64(len(raw)), "message/rfc822"); err != nil {
		return IngestResult{}, err
	}
	stored := []string{rawKey}
	files, err := s.inboundAttachments(ctx, ws, m.Attachments)
	for _, f := range files {
		stored = append(stored, f.key)
	}
	if err != nil {
		s.deleteObjects(ctx, stored)
		return IngestResult{}, err
	}

	content := s.mailContent(m)
	subject := content.subject
	var authResults *string
	if m.AuthResults != "" {
		authResults = &m.AuthResults
	}
	dmarc := email.TrustedDMARC(m.AllAuthResults, s.ingress.AuthservID)
	spam := dmarc == email.DMARCFail
	rawSize := int64(len(raw))

	var res IngestResult
	err = s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
		now := s.now()
		contact, unverified, err := s.contactForMail(ctx, q, ch, m, sender, spam, now)
		if err != nil {
			return err
		}
		if err := s.checkSenderCap(ctx, q, ws, contact.ID, now); err != nil {
			return err
		}
		var convAddress *string
		if isCatchAll(ch.Address) {
			convAddress = &recipient
		}
		conv, isNew, err := s.threadFor(ctx, q, events, ch, m, contact.ID, subject, spam, convAddress, now)
		if err != nil {
			return err
		}
		direction := string(oas.In)
		msg, err := q.CreateMessage(ctx, store.CreateMessageParams{
			ID: newID(), WorkspaceID: ws, ConversationID: conv.ID, Kind: string(oas.MessageKindMessage), Direction: &direction,
			AuthorType: string(oas.AuthorTypeContact), AuthorContactID: &contact.ID, Body: content.visible, Html: content.html, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		summary := store.ListMessageEmailsRow{
			MessageID: msg.ID, Direction: direction, HeaderMessageID: headerID, FromAddress: sender,
			ToAddresses: normalizedAddressList(m.To), CcAddresses: normalizedAddressList(m.Cc), Subject: subject,
			Quoted: content.quoted, HasRaw: true, Auto: auto, Dmarc: dmarc, UnverifiedSender: unverified,
		}
		if err := q.CreateMessageEmail(ctx, store.CreateMessageEmailParams{
			WorkspaceID: ws, MessageID: msg.ID, ConversationID: conv.ID, ChannelID: &ch.ChannelID, Direction: direction,
			HeaderMessageID: headerID, InReplyTo: content.inReplyTo, ReferencesIds: content.refs, FromAddress: sender,
			ToAddresses: summary.ToAddresses, CcAddresses: summary.CcAddresses, Subject: subject, FullText: m.Text,
			FullHtml: content.fullHTML, Quoted: summary.Quoted, Headers: content.headers, RawKey: &rawKey,
			RawSize: &rawSize, AuthenticationResults: authResults, Dmarc: dmarc, Auto: auto, CreatedAt: now,
		}); err != nil {
			return err
		}
		var atts []store.Attachment
		var total int64
		for _, f := range files {
			a, err := q.CreateAttachment(ctx, store.CreateAttachmentParams{
				ID: f.id, WorkspaceID: ws, ConversationID: conv.ID, MessageID: msg.ID, StorageKey: f.key,
				Filename: f.filename, ContentType: f.contentType, SizeBytes: f.size, ContentID: f.contentID, Inline: f.inline, CreatedAt: now,
			})
			if err != nil {
				return err
			}
			atts = append(atts, a)
			total += f.size
		}
		if err := q.TouchConversation(ctx, store.TouchConversationParams{WorkspaceID: ws, ID: conv.ID, Now: now, IsMessage: true}); err != nil {
			return err
		}
		newConversations := int64(0)
		if isNew {
			newConversations = 1
		}
		if err := s.addUsage(ctx, q, ws, newConversations, 1, total); err != nil {
			return err
		}
		events.conversation(realtime.MessageCreated, conv, withEmail(messageBody(messageRow(msg), atts), &summary))
		res = IngestResult{Status: IngestStored, ConversationID: &conv.ID, MessageID: &msg.ID}
		if isNew && !auto && !spam && ch.AutoReplyEnabled && !opts.noGreeting {
			return s.autoReply(ctx, q, events, ch, conv, contact.ID, now)
		}
		return nil
	})
	if err != nil {
		s.deleteObjects(ctx, stored)
		if store.IsUniqueViolation(err) {
			if prev, err := s.st.FindInboundEmailByHeader(ctx, store.FindInboundEmailByHeaderParams{WorkspaceID: ws, ChannelID: &ch.ChannelID, HeaderMessageID: headerID}); err == nil {
				return IngestResult{Status: IngestDuplicate, ConversationID: &prev.ConversationID, MessageID: &prev.MessageID}, nil
			}
		}
		return IngestResult{}, err
	}
	return res, nil
}

func (s *Server) refuseBlockedSender(ctx context.Context, ws uuid.UUID, addr string) error {
	if addr == "" {
		return nil
	}
	id, err := s.st.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: addr})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	c, err := s.st.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: id})
	if err != nil {
		return err
	}
	if c.Blocked {
		return errIngestBlocked
	}
	return nil
}

func (s *Server) contactForSender(ctx context.Context, q *store.Queries, ws uuid.UUID, addr, name string, now time.Time) (contactRow, error) {
	id, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: addr})
	if err == nil {
		c, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: id})
		if err != nil {
			return c, err
		}
		if c.Blocked {
			return c, errIngestBlocked
		}
		return c, nil
	}
	if !store.IsNotFound(err) {
		return contactRow{}, err
	}
	r, err := q.CreateContact(ctx, store.CreateContactParams{
		ID: newID(), WorkspaceID: ws, Name: truncateRunes(strings.TrimSpace(name), 200), Attributes: []byte("{}"), Now: now,
	})
	if err != nil {
		return contactRow{}, err
	}
	if err := q.AddContactEmail(ctx, store.AddContactEmailParams{WorkspaceID: ws, ContactID: r.ID, Email: addr, Position: 0}); err != nil {
		return contactRow{}, err
	}
	if err := q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: ws, ID: r.ID}); err != nil {
		return contactRow{}, err
	}
	return contactRow(r), nil
}

// threadOf is the conversation of the inbox that the mail's In-Reply-To and References name, by
// stored Message-IDs and then by the conversation token inside our own Message-IDs, preferring
// one of contactID.
func threadOf(ctx context.Context, q *store.Queries, ch store.FindEmailChannelByAddressRow, m *email.Message, contactID uuid.UUID) (*store.Conversation, error) {
	ws := ch.WorkspaceID
	ids := slices.Concat(m.InReplyTo, m.References)
	if len(ids) == 0 {
		return nil, nil
	}
	c, err := q.FindConversationByHeaders(ctx, store.FindConversationByHeadersParams{WorkspaceID: ws, InboxID: ch.InboxID, Ids: ids, ContactID: contactID})
	if err == nil {
		return &c, nil
	}
	if !store.IsNotFound(err) {
		return nil, err
	}
	var tokens []string
	for _, id := range ids {
		if t, _, ok := email.TokenFromID(id); ok {
			tokens = append(tokens, t)
		}
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	c, err = q.FindConversationByEmailToken(ctx, store.FindConversationByEmailTokenParams{WorkspaceID: ws, InboxID: ch.InboxID, Tokens: tokens, ContactID: contactID})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// contactForMail is the contact the mail is from: the contact with the sender's address, else the
// contact of the thread it answers when that contact typed the address in the widget and has not
// confirmed it (the message joins their conversation from an unverified address and links
// nothing; confirming the address later makes it theirs), else a new contact. Mail that fails
// DMARC never takes the typed-address path.
func (s *Server) contactForMail(ctx context.Context, q *store.Queries, ch store.FindEmailChannelByAddressRow, m *email.Message, sender string, spam bool, now time.Time) (contactRow, bool, error) {
	ws := ch.WorkspaceID
	if _, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: sender}); err == nil || !store.IsNotFound(err) || spam {
		c, err := s.contactForSender(ctx, q, ws, sender, m.From.Name, now)
		return c, false, err
	}
	thread, err := threadOf(ctx, q, ch, m, uuid.Nil())
	if err != nil {
		return contactRow{}, false, err
	}
	if thread != nil {
		c, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: thread.ContactID})
		if err != nil {
			return c, false, err
		}
		if c.TypedEmail != nil && strings.EqualFold(*c.TypedEmail, sender) {
			if c.Blocked {
				return c, false, errIngestBlocked
			}
			return c, true, nil
		}
	}
	c, err := s.contactForSender(ctx, q, ws, sender, m.From.Name, now)
	return c, false, err
}

// threadFor finds the conversation a reply belongs to (threadOf), or starts a new one. A thread
// that belongs to another contact is never joined, and mail that fails DMARC never joins a
// conversation not flagged spam: the sender gets its own conversation that points to it.
func (s *Server) threadFor(ctx context.Context, q *store.Queries, events *eventBatch, ch store.FindEmailChannelByAddressRow, m *email.Message, contactID uuid.UUID, subject string, spam bool, address *string, now time.Time) (store.Conversation, bool, error) {
	ws := ch.WorkspaceID
	found, err := threadOf(ctx, q, ch, m, contactID)
	if err != nil {
		return store.Conversation{}, false, err
	}
	var related *uuid.UUID
	if found != nil && (found.ContactID != contactID || (spam && !found.Spam)) {
		related = &found.ID
		found = nil
	}
	var (
		delivery *store.BroadcastDelivery
		notice   *store.StatusNoticeDelivery
	)
	if found == nil && related == nil && !spam {
		if delivery, err = deliveryOfTokens(ctx, q, ch, m, contactID); err != nil {
			return store.Conversation{}, false, err
		}
		if delivery == nil {
			if notice, err = noticeDeliveryOfTokens(ctx, q, ch, m, contactID); err != nil {
				return store.Conversation{}, false, err
			}
		}
		if notice != nil && notice.ContactID == contactID && notice.ConversationID != nil {
			c, err := q.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: *notice.ConversationID})
			if err == nil && c.ContactID == contactID && c.InboxID == ch.InboxID && c.TrashedAt == nil {
				found, notice = &c, nil
			} else if err != nil && !store.IsNotFound(err) {
				return c, false, err
			}
		}
		if delivery != nil && delivery.ContactID == contactID && delivery.ConversationID != nil {
			c, err := q.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: *delivery.ConversationID})
			if err == nil && c.ContactID == contactID && c.InboxID == ch.InboxID && c.TrashedAt == nil {
				found, delivery = &c, nil
			} else if err != nil && !store.IsNotFound(err) {
				return c, false, err
			}
		}
	}
	if found == nil {
		n, err := q.CountRecentConversations(ctx, store.CountRecentConversationsParams{
			WorkspaceID: ws, ChannelID: &ch.ChannelID, ContactID: contactID, Since: now.Add(-time.Hour),
		})
		if err != nil {
			return store.Conversation{}, false, err
		}
		if n >= newConversationsPerHour {
			latest, err := q.LatestContactConversation(ctx, store.LatestContactConversationParams{WorkspaceID: ws, ChannelID: &ch.ChannelID, ContactID: contactID})
			if err != nil {
				return latest, false, err
			}
			if !spam || latest.Spam {
				found, delivery, notice = &latest, nil, nil
			}
		}
	}
	if found != nil {
		c, err := s.reopenConversation(ctx, q, events, ws, found.ID, now)
		return c, false, err
	}
	c, err := q.CreateConversation(ctx, store.CreateConversationParams{
		ID: newID(), WorkspaceID: ws, InboxID: ch.InboxID, ContactID: contactID, ChannelID: &ch.ChannelID,
		Subject: subject, Priority: string(oas.Normal), Spam: spam, RelatedConversationID: related,
		EmailAddress: address, Now: now,
	})
	if err != nil {
		return c, false, err
	}
	var reply func() error
	if delivery != nil {
		created := c
		if c, err = q.SetConversationBroadcast(ctx, store.SetConversationBroadcastParams{WorkspaceID: ws, ID: c.ID, BroadcastID: &delivery.BroadcastID}); err != nil {
			return created, false, err
		}
		d := *delivery
		reply = func() error {
			_, err := s.broadcastReplyStart(ctx, q, events, c, d, contactID, now)
			return err
		}
	}
	if notice != nil {
		if err := s.noticeReplyStart(ctx, q, c, *notice, contactID, now); err != nil {
			return c, false, err
		}
	}
	events.conversation(realtime.ConversationCreated, c, conversationBody(c, nil))
	if reply != nil {
		if err := reply(); err != nil {
			return c, false, err
		}
	}
	return c, true, nil
}

func (s *Server) checkSenderCap(ctx context.Context, q *store.Queries, ws, contactID uuid.UUID, now time.Time) error {
	limit := s.ingress.SenderHourlyCap
	if limit <= 0 {
		limit = defaultSenderHourlyCap
	}
	n, err := q.CountRecentInboundEmails(ctx, store.CountRecentInboundEmailsParams{WorkspaceID: ws, ContactID: contactID, Since: now.Add(-time.Hour)})
	if err != nil {
		return err
	}
	if n >= int64(limit) {
		return errIngestRateLimited
	}
	return nil
}

func (s *Server) systemEvent(ctx context.Context, q *store.Queries, events *eventBatch, c store.Conversation, ev oas.MessageEvent, at time.Time) error {
	msg, err := q.CreateMessage(ctx, store.CreateMessageParams{
		ID: newID(), WorkspaceID: c.WorkspaceID, ConversationID: c.ID, Kind: string(oas.MessageKindEvent),
		AuthorType: string(oas.AuthorTypeSystem), Event: mustJSON(ev), CreatedAt: at,
	})
	if err != nil {
		return err
	}
	events.conversation(realtime.MessageCreated, c, messageBody(msg, nil))
	return nil
}

func (s *Server) autoReply(ctx context.Context, q *store.Queries, events *eventBatch, ch store.FindEmailChannelByAddressRow, c store.Conversation, contactID uuid.UUID, now time.Time) error {
	plan, err := s.planEmail(ctx, q, c)
	var apiErr *apiError
	if errors.As(err, &apiErr) || (err == nil && plan == nil) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = q.ClaimAutoReply(ctx, store.ClaimAutoReplyParams{
		WorkspaceID: c.WorkspaceID, ChannelID: ch.ChannelID, ContactID: contactID, Now: now,
		NotAfter: now.Add(-time.Duration(ch.AutoReplyIntervalHours) * time.Hour),
	})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	plan.auto = true
	direction, queued := string(oas.Out), deliveryQueued
	at := now.Add(time.Microsecond)
	msg, err := q.CreateMessage(ctx, store.CreateMessageParams{
		ID: newID(), WorkspaceID: c.WorkspaceID, ConversationID: c.ID, Kind: string(oas.MessageKindMessage),
		Direction: &direction, AuthorType: string(oas.AuthorTypeSystem), Body: ch.AutoReplyText, CreatedAt: at,
		DeliveryState: &queued,
	})
	if err != nil {
		return err
	}
	summary, err := s.queueEmail(ctx, q, events, plan, msg)
	if err != nil {
		return err
	}
	if err := q.TouchConversation(ctx, store.TouchConversationParams{WorkspaceID: c.WorkspaceID, ID: c.ID, Now: at, IsMessage: true}); err != nil {
		return err
	}
	if err := s.addUsage(ctx, q, c.WorkspaceID, 0, 1, 0); err != nil {
		return err
	}
	events.conversation(realtime.MessageCreated, c, withEmail(messageBody(msg, nil), summary))
	return nil
}

// reopenConversation opens a conversation a contact wrote in again, with the events of the change.
func (s *Server) reopenConversation(ctx context.Context, q *store.Queries, events *eventBatch, ws, id uuid.UUID, now time.Time) (store.Conversation, error) {
	c, err := q.LockConversation(ctx, store.LockConversationParams{WorkspaceID: ws, ID: id})
	if err != nil {
		return c, err
	}
	if c.Status == string(oas.ConversationStatusOpen) {
		return c, nil
	}
	updated, err := q.UpdateConversation(ctx, store.UpdateConversationParams{
		WorkspaceID: ws, ID: c.ID, Subject: c.Subject, Status: string(oas.ConversationStatusOpen), Priority: c.Priority,
		AssigneeID: c.AssigneeID, Spam: c.Spam, FeedbackProgress: c.FeedbackProgress, Now: now,
	})
	if err != nil {
		return c, err
	}
	body, err := oneConversation(ctx, q, updated)
	if err != nil {
		return c, err
	}
	events.conversation(realtime.ConversationUpdated, updated, body)
	st, prev := oas.ConversationStatusOpen, oas.ConversationStatus(c.Status)
	if err := s.systemEvent(ctx, q, events, updated, oas.MessageEvent{Type: oas.StatusChanged, Status: &st, PreviousStatus: &prev}, now); err != nil {
		return c, err
	}
	return updated, nil
}

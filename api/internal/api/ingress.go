package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
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
	MaxIngressBytes         = 25 << 20
	newConversationsPerHour = 20
	maxFullHTMLBytes        = 2 << 20
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
	errIngestTooLarge      = &IngestError{http.StatusRequestEntityTooLarge, oas.TooLarge, "Message too large"}
	errIngestBadSignature  = &IngestError{http.StatusUnauthorized, oas.BadSignature, "Rejected by recipient server"}
	errIngestStale         = &IngestError{http.StatusUnauthorized, oas.StaleTimestamp, "Rejected by recipient server"}
	errIngestUnknown       = &IngestError{http.StatusNotFound, oas.UnknownRecipient, "No such recipient"}
	errIngestBlocked       = &IngestError{http.StatusForbidden, oas.BlockedSender, "Messages from this sender are not accepted"}
	errIngestMalformed     = &IngestError{http.StatusBadRequest, oas.Malformed, "The message could not be read"}
	errIngestRateLimited   = &IngestError{http.StatusTooManyRequests, oas.RateLimited, "Too many new conversations from this sender in the last hour; this message was not accepted"}
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
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxIngressBytes+1))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || len(body) > MaxIngressBytes {
		writeIngestError(w, errIngestTooLarge)
		return
	}
	if err != nil {
		writeIngestError(w, errIngestMalformed)
		return
	}
	to := r.Header.Get("X-Yuva-Envelope-To")
	switch err := email.VerifyIngress(s.ingress.Secret, r.Header.Get("X-Yuva-Timestamp"), to, r.Header.Get("X-Yuva-Signature"), body, s.now()); {
	case errors.Is(err, email.ErrStaleTimestamp):
		writeIngestError(w, errIngestStale)
		return
	case err != nil:
		writeIngestError(w, errIngestBadSignature)
		return
	}
	res, err := s.IngestEmail(r.Context(), to, r.Header.Get("X-Yuva-Envelope-From"), body)
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

func (s *Server) findEmailChannel(ctx context.Context, to string) (store.FindEmailChannelByAddressRow, error) {
	addr := strings.ToLower(strings.Trim(strings.TrimSpace(to), "<>"))
	ch, err := s.st.FindEmailChannelByAddress(ctx, addr)
	if store.IsNotFound(err) {
		local, domain, ok := strings.Cut(addr, "@")
		if base, _, tagged := strings.Cut(local, "+"); ok && tagged {
			ch, err = s.st.FindEmailChannelByAddress(ctx, base+"@"+domain)
		}
	}
	if store.IsNotFound(err) {
		return ch, errIngestUnknown
	}
	return ch, err
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
	var out []storedFile
	for i, a := range atts {
		if len(out) == 50 {
			break
		}
		size := int64(len(a.Data))
		if size == 0 || size > s.attach.MaxBytes {
			continue
		}
		ct, err := s.contentType(a.ContentType, bytes.NewReader(a.Data))
		if err != nil {
			var apiErr *apiError
			if errors.As(err, &apiErr) {
				continue
			}
			return out, err
		}
		name := a.Filename
		if name == "" {
			name = "attachment-" + strconv.Itoa(i+1)
			if exts, _ := mime.ExtensionsByType(ct); len(exts) > 0 {
				name += exts[0]
			}
		}
		f := storedFile{id: newID(), filename: cleanFilename(name), contentType: ct, size: size, inline: a.Inline}
		if cid := truncateRunes(a.ContentID, 998); cid != "" {
			f.contentID = &cid
		}
		f.key = workspaceID.String() + "/attachments/" + f.id.String()
		if err := s.objects.Put(ctx, f.key, bytes.NewReader(a.Data), size, ct); err != nil {
			return out, err
		}
		out = append(out, f)
	}
	return out, nil
}

func synthesizedID(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256-" + hex.EncodeToString(sum[:20]) + "@yuva.invalid"
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
// called by /ingress/email after the signature check and by `yuva ingest-email`.
func (s *Server) IngestEmail(ctx context.Context, envelopeTo, envelopeFrom string, raw []byte) (IngestResult, error) {
	if len(raw) > MaxIngressBytes {
		return IngestResult{}, errIngestTooLarge
	}
	ch, err := s.findEmailChannel(ctx, envelopeTo)
	if err != nil {
		return IngestResult{}, err
	}
	m, err := email.Parse(raw)
	if err != nil {
		return IngestResult{}, errIngestMalformed
	}
	sender := m.From.Email
	if sender == "" {
		if a, ok := email.ParseAddress(envelopeFrom); ok {
			sender = a.Email
		}
	}
	if sender == ch.Address || (ch.FromAddress != nil && sender == *ch.FromAddress) {
		return IngestResult{Status: IngestDropped}, nil
	}
	if err := s.refuseBlockedSender(ctx, ch.WorkspaceID, sender); err != nil {
		return IngestResult{}, err
	}
	if m.Bounce != nil {
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
	auto := m.Auto || nullSender(envelopeFrom) || ownMessageID(m.MessageID, ch)
	ws := ch.WorkspaceID
	headerID := m.MessageID
	if headerID == "" {
		headerID = synthesizedID(raw)
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

	visible, textQuoted := reply.Strip(m.Text)
	visible = truncateRunes(visible, maxMessageBodyRunes)
	var html *string
	htmlQuoted := false
	if m.HTML != "" {
		stripped, removed := email.StripHTMLQuotes(m.HTML)
		htmlQuoted = removed
		html = s.sanitizeHTML(stripped, maxMessageHTMLBytes)
	}
	subject := truncateRunes(strings.Join(strings.Fields(m.Subject), " "), 500)
	headers, _ := json.Marshal(m.Headers)
	var authResults *string
	if m.AuthResults != "" {
		authResults = &m.AuthResults
	}
	spam := m.DMARC == email.DMARCFail
	rawSize := int64(len(raw))

	var res IngestResult
	err = s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
		now := s.now()
		contact, err := s.contactForSender(ctx, q, ws, sender, m.From.Name, now)
		if err != nil {
			return err
		}
		conv, isNew, err := s.threadFor(ctx, q, events, ch, m, contact.ID, subject, spam, now)
		if err != nil {
			return err
		}
		direction := string(oas.In)
		msg, err := q.CreateMessage(ctx, store.CreateMessageParams{
			ID: newID(), WorkspaceID: ws, ConversationID: conv.ID, Kind: string(oas.MessageKindMessage), Direction: &direction,
			AuthorType: string(oas.AuthorTypeContact), AuthorContactID: &contact.ID, Body: visible, Html: html, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		var inReplyTo *string
		if len(m.InReplyTo) > 0 {
			inReplyTo = &m.InReplyTo[0]
		}
		refs := m.References
		if refs == nil {
			refs = []string{}
		}
		summary := store.ListMessageEmailsRow{
			MessageID: msg.ID, Direction: direction, HeaderMessageID: headerID, FromAddress: sender,
			ToAddresses: normalizedAddressList(m.To), CcAddresses: normalizedAddressList(m.Cc), Subject: subject,
			Quoted: textQuoted || htmlQuoted, HasRaw: true, Auto: auto, Dmarc: m.DMARC,
		}
		if err := q.CreateMessageEmail(ctx, store.CreateMessageEmailParams{
			WorkspaceID: ws, MessageID: msg.ID, ConversationID: conv.ID, ChannelID: &ch.ChannelID, Direction: direction,
			HeaderMessageID: headerID, InReplyTo: inReplyTo, ReferencesIds: refs, FromAddress: sender,
			ToAddresses: summary.ToAddresses, CcAddresses: summary.CcAddresses, Subject: subject, FullText: m.Text,
			FullHtml: s.sanitizeHTML(m.HTML, maxFullHTMLBytes), Quoted: summary.Quoted, Headers: headers, RawKey: &rawKey,
			RawSize: &rawSize, AuthenticationResults: authResults, Dmarc: m.DMARC, Auto: auto, CreatedAt: now,
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
		if isNew && !auto && !spam && ch.AutoReplyEnabled {
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

// threadFor finds the conversation a reply belongs to, by the stored Message-IDs it names and then
// by the conversation token inside our own Message-IDs, or starts a new one. A thread that belongs
// to another contact is never joined: the sender gets its own conversation that points to it.
func (s *Server) threadFor(ctx context.Context, q *store.Queries, events *eventBatch, ch store.FindEmailChannelByAddressRow, m *email.Message, contactID uuid.UUID, subject string, spam bool, now time.Time) (store.Conversation, bool, error) {
	ws := ch.WorkspaceID
	ids := slices.Concat(m.InReplyTo, m.References)
	var found *store.Conversation
	if len(ids) > 0 {
		c, err := q.FindConversationByHeaders(ctx, store.FindConversationByHeadersParams{WorkspaceID: ws, InboxID: ch.InboxID, Ids: ids, ContactID: contactID})
		if err == nil {
			found = &c
		} else if !store.IsNotFound(err) {
			return c, false, err
		}
	}
	if found == nil {
		var tokens []string
		for _, id := range ids {
			if t, _, ok := email.TokenFromID(id); ok {
				tokens = append(tokens, t)
			}
		}
		if len(tokens) > 0 {
			c, err := q.FindConversationByEmailToken(ctx, store.FindConversationByEmailTokenParams{WorkspaceID: ws, InboxID: ch.InboxID, Tokens: tokens, ContactID: contactID})
			if err == nil {
				found = &c
			} else if !store.IsNotFound(err) {
				return c, false, err
			}
		}
	}
	var related *uuid.UUID
	if found != nil && found.ContactID != contactID {
		related = &found.ID
		found = nil
	}
	if found != nil {
		c, err := q.LockConversation(ctx, store.LockConversationParams{WorkspaceID: ws, ID: found.ID})
		if err != nil {
			return c, false, err
		}
		if c.Status == string(oas.Open) {
			return c, false, nil
		}
		updated, err := q.UpdateConversation(ctx, store.UpdateConversationParams{
			WorkspaceID: ws, ID: c.ID, Subject: c.Subject, Status: string(oas.Open), Priority: c.Priority,
			AssigneeID: c.AssigneeID, Spam: c.Spam, Now: now,
		})
		if err != nil {
			return c, false, err
		}
		body, err := oneConversation(ctx, q, updated)
		if err != nil {
			return c, false, err
		}
		events.conversation(realtime.ConversationUpdated, updated, body)
		st, prev := oas.Open, oas.ConversationStatus(c.Status)
		if err := s.systemEvent(ctx, q, events, updated, oas.MessageEvent{Type: oas.StatusChanged, Status: &st, PreviousStatus: &prev}, now); err != nil {
			return c, false, err
		}
		return updated, false, nil
	}
	n, err := q.CountRecentConversations(ctx, store.CountRecentConversationsParams{
		WorkspaceID: ws, ChannelID: &ch.ChannelID, ContactID: contactID, Since: now.Add(-time.Hour),
	})
	if err != nil {
		return store.Conversation{}, false, err
	}
	if n >= newConversationsPerHour {
		return store.Conversation{}, false, errIngestRateLimited
	}
	c, err := q.CreateConversation(ctx, store.CreateConversationParams{
		ID: newID(), WorkspaceID: ws, InboxID: ch.InboxID, ContactID: contactID, ChannelID: &ch.ChannelID,
		Subject: subject, Priority: string(oas.Normal), Spam: spam, RelatedConversationID: related, Now: now,
	})
	if err != nil {
		return c, false, err
	}
	events.conversation(realtime.ConversationCreated, c, conversationBody(c, nil))
	return c, true, nil
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

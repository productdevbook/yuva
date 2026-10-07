package api

import (
	"context"
	"errors"
	"mime"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/storage"
	"github.com/productdevbook/yuva/api/internal/store"
)

var (
	clientMessageFields      = []string{"body", "client_id"}
	clientConversationFields = []string{"subject", "body", "client_id"}
)

func clientAttachmentBody(a store.Attachment) oas.ClientAttachment {
	return oas.ClientAttachment{
		Id: a.ID, Filename: a.Filename, ContentType: a.ContentType, Size: a.SizeBytes, ContentId: a.ContentID, Inline: a.Inline,
	}
}

func clientAuthor(authorType string, memberID *uuid.UUID, names map[uuid.UUID]string) oas.ClientMessageAuthor {
	out := oas.ClientMessageAuthor{Type: oas.AuthorType(authorType)}
	if authorType == string(oas.AuthorTypeMember) {
		var name string
		if memberID != nil {
			name = names[*memberID]
		}
		ini := initials(name)
		out.Name, out.Initials = &name, &ini
	}
	return out
}

func clientMessageBody(m messageRow, atts []store.Attachment, names map[uuid.UUID]string) oas.ClientMessage {
	out := oas.ClientMessage{
		Id: m.ID, ConversationId: m.ConversationID, Direction: oas.In, Body: m.Body, Html: m.Html, CreatedAt: m.CreatedAt,
		Author: clientAuthor(m.AuthorType, m.AuthorMemberID, names), Attachments: make([]oas.ClientAttachment, 0, len(atts)),
	}
	if m.Direction != nil {
		out.Direction = oas.Direction(*m.Direction)
	}
	if m.AuthorType == string(oas.AuthorTypeContact) {
		out.ClientId = m.ClientID
	}
	for _, a := range atts {
		out.Attachments = append(out.Attachments, clientAttachmentBody(a))
	}
	return out
}

func memberNames(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListMemberNames(ctx, store.ListMemberNamesParams{WorkspaceID: workspaceID, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, nil
}

func clientConversationBody(c store.Conversation) oas.ClientConversation {
	return oas.ClientConversation{
		Id: c.ID, Subject: c.Subject, Status: oas.ConversationStatus(c.Status), LastMessageAt: c.LastMessageAt, CreatedAt: c.CreatedAt,
	}
}

func clientConversation(ctx context.Context, q *store.Queries, cp contactPrincipal, id uuid.UUID, lock bool) (store.Conversation, error) {
	var (
		c   store.Conversation
		err error
	)
	if lock {
		c, err = q.LockConversation(ctx, store.LockConversationParams{WorkspaceID: cp.workspaceID, ID: id})
	} else {
		c, err = q.GetConversation(ctx, store.GetConversationParams{WorkspaceID: cp.workspaceID, ID: id})
	}
	if store.IsNotFound(err) || (err == nil && (c.InboxID != cp.inboxID || c.ContactID != cp.contactID)) {
		return c, errConversationGone
	}
	return c, err
}

func clientConversationItems(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, rows []store.Conversation) ([]oas.ClientConversation, error) {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	previews, err := q.ListConversationPreviews(ctx, store.ListConversationPreviewsParams{WorkspaceID: workspaceID, ConversationIds: ids})
	if err != nil {
		return nil, err
	}
	byConv := map[uuid.UUID]*oas.ClientMessagePreview{}
	for _, m := range previews {
		byConv[m.ConversationID] = &oas.ClientMessagePreview{Id: m.ID, AuthorType: oas.AuthorType(m.AuthorType), Text: previewText(m.Body), CreatedAt: m.CreatedAt}
	}
	unreadIDs, err := q.ListUnreadForContact(ctx, store.ListUnreadForContactParams{WorkspaceID: workspaceID, ConversationIds: ids})
	if err != nil {
		return nil, err
	}
	unread := map[uuid.UUID]bool{}
	for _, id := range unreadIDs {
		unread[id] = true
	}
	reads, err := q.ListMemberReadPositions(ctx, store.ListMemberReadPositionsParams{WorkspaceID: workspaceID, ConversationIds: ids})
	if err != nil {
		return nil, err
	}
	readAt := map[uuid.UUID]*time.Time{}
	for _, r := range reads {
		readAt[r.ConversationID] = &r.ReadAt
	}
	out := make([]oas.ClientConversation, len(rows))
	for i, r := range rows {
		out[i] = clientConversationBody(r)
		out[i].LastMessage, out[i].Unread, out[i].LastReadByMemberAt = byConv[r.ID], unread[r.ID], readAt[r.ID]
	}
	return out, nil
}

func (s *Server) ListClientConversations(ctx context.Context, req oas.ListClientConversationsRequestObject) (oas.ListClientConversationsResponseObject, error) {
	cp := contactFrom(ctx)
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListContactConversations(ctx, store.ListContactConversationsParams{
		WorkspaceID: cp.workspaceID, InboxID: cp.inboxID, ContactID: cp.contactID, CursorAt: at, CursorID: id, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		last := rows[lim-1]
		pos := last.CreatedAt
		if last.LastMessageAt != nil {
			pos = *last.LastMessageAt
		}
		c := encodeCursor(pos, last.ID)
		next = &c
	}
	items, err := clientConversationItems(ctx, s.st.Queries, cp.workspaceID, rows)
	if err != nil {
		return nil, err
	}
	return oas.ListClientConversations200JSONResponse{Items: items, NextCursor: next}, nil
}

func (s *Server) GetClientConversation(ctx context.Context, req oas.GetClientConversationRequestObject) (oas.GetClientConversationResponseObject, error) {
	cp := contactFrom(ctx)
	c, err := clientConversation(ctx, s.st.Queries, cp, req.ConversationId, false)
	if err != nil {
		return nil, err
	}
	items, err := clientConversationItems(ctx, s.st.Queries, cp.workspaceID, []store.Conversation{c})
	if err != nil {
		return nil, err
	}
	return oas.GetClientConversation200JSONResponse(items[0]), nil
}

func (s *Server) ListClientMessages(ctx context.Context, req oas.ListClientMessagesRequestObject) (oas.ListClientMessagesResponseObject, error) {
	cp := contactFrom(ctx)
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	c, err := clientConversation(ctx, s.st.Queries, cp, req.ConversationId, false)
	if err != nil {
		return nil, err
	}
	var rows []messageRow
	switch order := req.Params.Order; {
	case order == nil || *order == oas.ListClientMessagesParamsOrderAsc:
		var asc []store.ListPublicMessagesRow
		asc, err = s.st.ListPublicMessages(ctx, store.ListPublicMessagesParams{
			WorkspaceID: cp.workspaceID, ConversationID: c.ID, CursorAt: at, CursorID: id, Lim: lim + 1,
		})
		for _, r := range asc {
			rows = append(rows, messageRow(r))
		}
	case *order == oas.ListClientMessagesParamsOrderDesc:
		var desc []store.ListPublicMessagesDescRow
		desc, err = s.st.ListPublicMessagesDesc(ctx, store.ListPublicMessagesDescParams{
			WorkspaceID: cp.workspaceID, ConversationID: c.ID, CursorAt: at, CursorID: id, Lim: lim + 1,
		})
		for _, r := range desc {
			rows = append(rows, messageRow(r))
		}
	default:
		return nil, errValidation("order must be asc or desc")
	}
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		cur := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		next = &cur
	}
	out, err := s.clientMessages(ctx, s.st.Queries, cp.workspaceID, rows)
	if err != nil {
		return nil, err
	}
	return oas.ListClientMessages200JSONResponse{Items: out, NextCursor: next}, nil
}

func (s *Server) clientMessages(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, rows []messageRow) ([]oas.ClientMessage, error) {
	ids := make([]uuid.UUID, len(rows))
	var members []uuid.UUID
	for i, r := range rows {
		ids[i] = r.ID
		if r.AuthorMemberID != nil {
			members = append(members, *r.AuthorMemberID)
		}
	}
	atts, err := messageAttachments(ctx, q, workspaceID, ids)
	if err != nil {
		return nil, err
	}
	names, err := memberNames(ctx, q, workspaceID, members)
	if err != nil {
		return nil, err
	}
	out := make([]oas.ClientMessage, len(rows))
	for i, r := range rows {
		out[i] = clientMessageBody(r, atts[r.ID], names)
	}
	return out, nil
}

func (s *Server) validateClientInput(in *messageInput) error {
	if len([]rune(in.body)) > maxMessageBodyRunes {
		return errValidation("body must be at most 65536 characters")
	}
	if strings.TrimSpace(in.body) == "" && len(in.files) == 0 {
		return errValidation("body is required unless files are attached")
	}
	if in.clientID != nil {
		id, err := trimmed(*in.clientID, 1, 200, "client_id")
		if err != nil {
			return err
		}
		in.clientID = &id
	}
	if in.subject != nil {
		v, err := trimmed(*in.subject, 0, 500, "subject")
		if err != nil {
			return err
		}
		in.subject = &v
	}
	return nil
}

func (s *Server) putUploads(ctx context.Context, in *messageInput) ([]string, error) {
	var stored []string
	for _, f := range in.files {
		if err := s.objects.Put(ctx, f.key, f.file, f.size, f.contentType); err != nil {
			s.deleteObjects(ctx, stored)
			return nil, err
		}
		stored = append(stored, f.key)
	}
	return stored, nil
}

// addContactMessage writes the contact's message into c, reopening c when it is not open.
func (s *Server) addContactMessage(ctx context.Context, q *store.Queries, events *eventBatch, cp contactPrincipal, c store.Conversation, in *messageInput, isNew bool) (messageRow, []store.Attachment, error) {
	now := s.now()
	direction := string(oas.In)
	msg, err := q.CreateMessage(ctx, store.CreateMessageParams{
		ID: newID(), WorkspaceID: cp.workspaceID, ConversationID: c.ID, Kind: string(oas.MessageKindMessage), Direction: &direction,
		AuthorType: string(oas.AuthorTypeContact), AuthorContactID: &cp.contactID, Body: in.body, ClientID: in.clientID, CreatedAt: now,
	})
	if err != nil {
		return msg, nil, err
	}
	var (
		atts  []store.Attachment
		total int64
	)
	for _, f := range in.files {
		a, err := q.CreateAttachment(ctx, store.CreateAttachmentParams{
			ID: f.id, WorkspaceID: cp.workspaceID, ConversationID: c.ID, MessageID: msg.ID, StorageKey: f.key,
			Filename: f.filename, ContentType: f.contentType, SizeBytes: f.size, CreatedAt: now,
		})
		if err != nil {
			return msg, nil, err
		}
		atts = append(atts, a)
		total += f.size
	}
	if c.Status != string(oas.Open) {
		updated, err := q.UpdateConversation(ctx, store.UpdateConversationParams{
			WorkspaceID: cp.workspaceID, ID: c.ID, Subject: c.Subject, Status: string(oas.Open), Priority: c.Priority,
			AssigneeID: c.AssigneeID, Spam: c.Spam, Now: now,
		})
		if err != nil {
			return msg, nil, err
		}
		body, err := oneConversation(ctx, q, updated)
		if err != nil {
			return msg, nil, err
		}
		events.conversation(realtime.ConversationUpdated, updated, body)
		st, prev := oas.Open, oas.ConversationStatus(c.Status)
		if err := s.systemEvent(ctx, q, events, updated, oas.MessageEvent{Type: oas.StatusChanged, Status: &st, PreviousStatus: &prev}, now.Add(time.Microsecond)); err != nil {
			return msg, nil, err
		}
	}
	if err := q.TouchConversation(ctx, store.TouchConversationParams{WorkspaceID: cp.workspaceID, ID: c.ID, Now: now, IsMessage: true}); err != nil {
		return msg, nil, err
	}
	var convs int64
	if isNew {
		convs = 1
	}
	if err := s.addUsage(ctx, q, cp.workspaceID, convs, 1, total); err != nil {
		return msg, nil, err
	}
	events.conversation(realtime.MessageCreated, c, messageBody(msg, atts))
	return msg, atts, nil
}

func (s *Server) CreateClientConversation(ctx context.Context, req oas.CreateClientConversationRequestObject) (oas.CreateClientConversationResponseObject, error) {
	cp := contactFrom(ctx)
	if err := s.rateLimit(s.writeChecks(ctx, cp)...); err != nil {
		return nil, err
	}
	var in *messageInput
	switch {
	case req.JSONBody != nil:
		b := req.JSONBody
		in = &messageInput{clientID: b.ClientId, subject: b.Subject}
		if b.Body != nil {
			in.body = *b.Body
		}
	case req.MultipartBody != nil:
		var err error
		if in, err = s.readMultipart(req.MultipartBody, cp.workspaceID, clientConversationFields); err != nil {
			return nil, err
		}
	default:
		return nil, errValidation("send application/json or multipart/form-data")
	}
	defer in.close()
	if err := s.validateClientInput(in); err != nil {
		return nil, err
	}
	if in.clientID != nil {
		prev, err := s.st.FindContactMessageByClientID(ctx, store.FindContactMessageByClientIDParams{
			WorkspaceID: cp.workspaceID, InboxID: cp.inboxID, ContactID: cp.contactID, ClientID: *in.clientID,
		})
		if err == nil {
			out, err := s.clientConversationCreated(ctx, cp, prev.ConversationID, prev.ID)
			if err != nil {
				return nil, err
			}
			return oas.CreateClientConversation201JSONResponse(out), nil
		}
		if !store.IsNotFound(err) {
			return nil, err
		}
	}
	stored, err := s.putUploads(ctx, in)
	if err != nil {
		return nil, err
	}
	var convID, msgID uuid.UUID
	err = s.inTx(ctx, cp.workspaceID, func(q *store.Queries, events *eventBatch) error {
		subject := ""
		if in.subject != nil {
			subject = *in.subject
		}
		channelID := cp.channelID
		c, err := q.CreateConversation(ctx, store.CreateConversationParams{
			ID: newID(), WorkspaceID: cp.workspaceID, InboxID: cp.inboxID, ContactID: cp.contactID, ChannelID: &channelID,
			Subject: subject, Priority: string(oas.Normal), Now: s.now(),
		})
		if err != nil {
			return err
		}
		events.conversation(realtime.ConversationCreated, c, conversationBody(c, nil))
		msg, _, err := s.addContactMessage(ctx, q, events, cp, c, in, true)
		convID, msgID = c.ID, msg.ID
		return err
	})
	if err != nil {
		s.deleteObjects(ctx, stored)
		return nil, err
	}
	out, err := s.clientConversationCreated(ctx, cp, convID, msgID)
	if err != nil {
		return nil, err
	}
	return oas.CreateClientConversation201JSONResponse(out), nil
}

func (s *Server) clientConversationCreated(ctx context.Context, cp contactPrincipal, convID, msgID uuid.UUID) (oas.ClientConversationCreated, error) {
	var out oas.ClientConversationCreated
	c, err := clientConversation(ctx, s.st.Queries, cp, convID, false)
	if err != nil {
		return out, err
	}
	items, err := clientConversationItems(ctx, s.st.Queries, cp.workspaceID, []store.Conversation{c})
	if err != nil {
		return out, err
	}
	m, err := s.st.GetMessage(ctx, store.GetMessageParams{WorkspaceID: cp.workspaceID, ID: msgID})
	if err != nil {
		return out, err
	}
	msgs, err := s.clientMessages(ctx, s.st.Queries, cp.workspaceID, []messageRow{messageRow(m)})
	if err != nil {
		return out, err
	}
	out.Conversation, out.Message = items[0], msgs[0]
	return out, nil
}

func (s *Server) CreateClientMessage(ctx context.Context, req oas.CreateClientMessageRequestObject) (oas.CreateClientMessageResponseObject, error) {
	cp := contactFrom(ctx)
	if err := s.rateLimit(s.writeChecks(ctx, cp)...); err != nil {
		return nil, err
	}
	if _, err := clientConversation(ctx, s.st.Queries, cp, req.ConversationId, false); err != nil {
		return nil, err
	}
	var in *messageInput
	switch {
	case req.JSONBody != nil:
		b := req.JSONBody
		in = &messageInput{clientID: b.ClientId}
		if b.Body != nil {
			in.body = *b.Body
		}
	case req.MultipartBody != nil:
		var err error
		if in, err = s.readMultipart(req.MultipartBody, cp.workspaceID, clientMessageFields); err != nil {
			return nil, err
		}
	default:
		return nil, errValidation("send application/json or multipart/form-data")
	}
	defer in.close()
	if err := s.validateClientInput(in); err != nil {
		return nil, err
	}
	stored, err := s.putUploads(ctx, in)
	if err != nil {
		return nil, err
	}
	var (
		msg     messageRow
		atts    []store.Attachment
		created = true
	)
	err = s.inTx(ctx, cp.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := clientConversation(ctx, q, cp, req.ConversationId, true)
		if err != nil {
			return err
		}
		if in.clientID != nil {
			prev, err := q.GetMessageByClientID(ctx, store.GetMessageByClientIDParams{WorkspaceID: cp.workspaceID, ConversationID: c.ID, ClientID: in.clientID})
			if err == nil {
				msg, created = messageRow(prev), false
				m, err := messageAttachments(ctx, q, cp.workspaceID, []uuid.UUID{prev.ID})
				atts = m[prev.ID]
				return err
			}
			if !store.IsNotFound(err) {
				return err
			}
		}
		msg, atts, err = s.addContactMessage(ctx, q, events, cp, c, in, false)
		return err
	})
	if err != nil || !created {
		s.deleteObjects(ctx, stored)
	}
	if err != nil {
		return nil, err
	}
	body := clientMessageBody(msg, atts, nil)
	if !created {
		return oas.CreateClientMessage200JSONResponse(body), nil
	}
	return oas.CreateClientMessage201JSONResponse(body), nil
}

func (s *Server) MarkClientConversationRead(ctx context.Context, req oas.MarkClientConversationReadRequestObject) (oas.MarkClientConversationReadResponseObject, error) {
	cp := contactFrom(ctx)
	c, err := clientConversation(ctx, s.st.Queries, cp, req.ConversationId, false)
	if err != nil {
		return nil, err
	}
	out := oas.ClientReadState{ConversationId: c.ID}
	var (
		msgID uuid.UUID
		msgAt time.Time
	)
	if req.Body != nil && req.Body.MessageId != nil {
		m, err := s.st.GetMessage(ctx, store.GetMessageParams{WorkspaceID: cp.workspaceID, ID: *req.Body.MessageId})
		if store.IsNotFound(err) || (err == nil && (m.ConversationID != c.ID || m.Kind != string(oas.MessageKindMessage))) {
			return nil, errValidation("message_id must be a message of the conversation")
		}
		if err != nil {
			return nil, err
		}
		msgID, msgAt = m.ID, m.CreatedAt
	} else {
		m, err := s.st.GetLatestPublicMessagePosition(ctx, store.GetLatestPublicMessagePositionParams{WorkspaceID: cp.workspaceID, ConversationID: c.ID})
		if store.IsNotFound(err) {
			return oas.MarkClientConversationRead200JSONResponse(out), nil
		}
		if err != nil {
			return nil, err
		}
		msgID, msgAt = m.ID, m.CreatedAt
	}
	read, err := s.st.MarkContactRead(ctx, store.MarkContactReadParams{
		WorkspaceID: cp.workspaceID, ConversationID: c.ID, MessageID: msgID, MessageAt: msgAt, Now: s.now(),
	})
	if store.IsNotFound(err) {
		read, err = s.st.GetContactRead(ctx, store.GetContactReadParams{WorkspaceID: cp.workspaceID, ConversationID: c.ID})
	}
	if err != nil {
		return nil, err
	}
	out.LastReadMessageId = &read.LastReadMessageID
	unread, err := s.st.ListUnreadForContact(ctx, store.ListUnreadForContactParams{WorkspaceID: cp.workspaceID, ConversationIds: []uuid.UUID{c.ID}})
	if err != nil {
		return nil, err
	}
	out.Unread = len(unread) > 0
	return oas.MarkClientConversationRead200JSONResponse(out), nil
}

func (s *Server) SetClientTyping(ctx context.Context, req oas.SetClientTypingRequestObject) (oas.SetClientTypingResponseObject, error) {
	cp := contactFrom(ctx)
	if err := s.rateLimit(rateCheck{"typing:" + cp.sessionID.String(), limitTypingPerSession}); err != nil {
		return nil, err
	}
	c, err := clientConversation(ctx, s.st.Queries, cp, req.ConversationId, false)
	if err != nil {
		return nil, err
	}
	typing := req.Body == nil || req.Body.Typing == nil || *req.Body.Typing
	contactID := cp.contactID
	data := oas.Typing{ConversationId: c.ID, Typing: typing, Author: oas.TypingAuthor{Type: oas.TypingAuthorTypeContact, ContactId: &contactID}}
	inbox, conv := c.InboxID, c.ID
	s.signal(ctx, realtime.Event{Type: realtime.Typing, WorkspaceID: cp.workspaceID, InboxID: &inbox, ConversationID: &conv, Data: mustJSON(data)})
	return oas.SetClientTyping204Response{}, nil
}

func (s *Server) DownloadClientAttachment(ctx context.Context, req oas.DownloadClientAttachmentRequestObject) (oas.DownloadClientAttachmentResponseObject, error) {
	cp := contactFrom(ctx)
	a, err := s.st.GetClientAttachment(ctx, store.GetClientAttachmentParams{
		WorkspaceID: cp.workspaceID, ID: req.AttachmentId, InboxID: cp.inboxID, ContactID: cp.contactID,
	})
	if store.IsNotFound(err) {
		return nil, errAttachmentGone
	}
	if err != nil {
		return nil, err
	}
	body, err := s.objects.Open(ctx, a.StorageKey)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, errAttachmentGone
	}
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": a.Filename})
	if disposition == "" {
		disposition = "attachment"
	}
	nosniff := "nosniff"
	return oas.DownloadClientAttachment200AsteriskResponse{
		Body: body, ContentType: a.ContentType, ContentLength: a.SizeBytes,
		Headers: oas.DownloadClientAttachment200ResponseHeaders{ContentDisposition: &disposition, XContentTypeOptions: &nosniff},
	}, nil
}

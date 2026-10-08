package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"unicode"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/storage"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxAttachmentsPerMessage = 10
	maxMessageBodyRunes      = 65536
	maxMessageHTMLBytes      = 262144
	maxFormFieldBytes        = maxMessageHTMLBytes + 1
)

var (
	errIncomingNeedsKey   = problem(http.StatusForbidden, "forbidden", "only an API key can post an incoming message on behalf of the contact")
	errAttachmentTooLarge = problem(http.StatusRequestEntityTooLarge, "attachment_too_large", "an attachment is larger than the server allows")
	errAttachmentType     = problem(http.StatusUnsupportedMediaType, "attachment_type_not_allowed", "an attachment has a content type the server does not allow")
	errAttachmentMismatch = problem(http.StatusUnsupportedMediaType, "attachment_type_mismatch", "an attachment's content does not match its declared content type")
	errTooManyAttachments = errValidation("at most 10 files per message")
	errBotSendingDisabled = problem(http.StatusForbidden, "bot_sending_disabled", "this workspace does not let API keys and MCP clients deliver messages; post a draft for a member to send")
	errDraftKind          = errValidation("only an outgoing message can be a draft")
)

type messageRow = store.CreateMessageRow

type upload struct {
	id          uuid.UUID
	key         string
	filename    string
	contentType string
	size        int64
	file        *os.File
}

type messageInput struct {
	kind      string
	direction string
	body      string
	html      *string
	clientID  *string
	subject   *string
	draft     bool
	extra     map[string]string
	files     []*upload
}

var memberFormFields = []string{"kind", "direction", "body", "html", "client_id", "draft"}

func (in *messageInput) close() {
	for _, f := range in.files {
		f.file.Close()
		os.Remove(f.file.Name())
	}
}

func (s *Server) validateMessage(p principal, in *messageInput) error {
	switch in.kind {
	case string(oas.MessageKindMessage):
		if in.direction == "" {
			in.direction = string(oas.Out)
		}
		if in.direction != string(oas.In) && in.direction != string(oas.Out) {
			return errValidation("direction must be in or out")
		}
		if in.direction == string(oas.In) && !p.isKey() {
			return errIncomingNeedsKey
		}
		if err := requireScope(p, oas.MessagesWrite); err != nil {
			return err
		}
		if in.direction == string(oas.In) && in.draft {
			return errDraftKind
		}
		if in.direction == string(oas.Out) && !in.draft && p.deliversAsBot() && !p.botsMaySend {
			return errBotSendingDisabled
		}
	case string(oas.MessageKindNote):
		if in.direction != "" {
			return errValidation("a note has no direction")
		}
		if err := requireScope(p, oas.NotesWrite); err != nil {
			return err
		}
		if in.draft {
			return errDraftKind
		}
	default:
		return errValidation("kind must be message or note")
	}
	if len([]rune(in.body)) > maxMessageBodyRunes {
		return errValidation("body must be at most 65536 characters")
	}
	if strings.TrimSpace(in.body) == "" && len(in.files) == 0 {
		return errValidation("body is required unless files are attached")
	}
	if in.html != nil {
		if len(*in.html) > maxMessageHTMLBytes {
			return errValidation("html must be at most 256 KiB")
		}
		clean := strings.TrimSpace(s.sanitize.Sanitize(*in.html))
		in.html = nil
		if clean != "" {
			in.html = &clean
		}
	}
	if in.clientID != nil {
		id, err := trimmed(*in.clientID, 1, 200, "client_id")
		if err != nil {
			return err
		}
		in.clientID = &id
	}
	return nil
}

func (s *Server) typeAllowed(ct string) bool {
	for _, t := range s.attach.Types {
		if t == ct || (strings.HasSuffix(t, "/*") && strings.HasPrefix(ct, strings.TrimSuffix(t, "*"))) {
			return true
		}
	}
	return false
}

func cleanFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '"' {
			return -1
		}
		return r
	}, name))
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	for len(name) > 255 {
		r := []rune(name)
		name = string(r[:len(r)-1])
	}
	return name
}

func (s *Server) spool(workspaceID uuid.UUID, part *multipart.Part) (*upload, error) {
	f, err := os.CreateTemp("", "yuva-upload-*")
	if err != nil {
		return nil, err
	}
	u := &upload{id: newID(), filename: cleanFilename(part.FileName()), file: f}
	u.key = workspaceID.String() + "/attachments/" + u.id.String()
	n, err := io.Copy(f, io.LimitReader(part, s.attach.MaxBytes+1))
	if err == nil && n > s.attach.MaxBytes {
		err = errAttachmentTooLarge
	}
	if err == nil {
		u.size = n
		u.contentType, err = s.contentType(part.Header.Get("Content-Type"), f)
	}
	if err == nil {
		_, err = f.Seek(0, io.SeekStart)
	}
	if err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, err
	}
	return u, nil
}

func (s *Server) contentType(declared string, f io.ReaderAt) (string, error) {
	head := make([]byte, 512)
	n, _ := f.ReadAt(head, 0)
	sniffed := sniffType(head[:n])
	ct, _, err := mime.ParseMediaType(declared)
	ct = canonicalType(strings.ToLower(ct))
	switch {
	case err != nil || ct == "" || ct == octetStream:
		ct = sniffed
	case !contentMatches(ct, sniffed):
		if !s.typeAllowed(ct) {
			return "", errAttachmentType
		}
		return "", errAttachmentMismatch
	}
	if !s.typeAllowed(ct) {
		return "", errAttachmentType
	}
	return ct, nil
}

func (s *Server) readMultipart(r *multipart.Reader, workspaceID uuid.UUID, fields []string) (*messageInput, error) {
	in := &messageInput{}
	for {
		part, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			return in, nil
		}
		if err != nil {
			in.close()
			return nil, uploadError(err)
		}
		if part.FileName() != "" || part.FormName() == "files" {
			if len(in.files) == maxAttachmentsPerMessage {
				in.close()
				return nil, errTooManyAttachments
			}
			u, err := s.spool(workspaceID, part)
			if err != nil {
				in.close()
				return nil, uploadError(err)
			}
			in.files = append(in.files, u)
			continue
		}
		b, err := io.ReadAll(io.LimitReader(part, maxFormFieldBytes+1))
		if err != nil {
			in.close()
			return nil, uploadError(err)
		}
		if len(b) > maxFormFieldBytes {
			in.close()
			return nil, errValidation(part.FormName() + " is too long")
		}
		v := string(b)
		if !slices.Contains(fields, part.FormName()) {
			in.close()
			return nil, errValidation("unknown form field " + part.FormName())
		}
		switch part.FormName() {
		case "kind":
			in.kind = v
		case "direction":
			in.direction = v
		case "body":
			in.body = v
		case "html":
			in.html = &v
		case "client_id":
			in.clientID = &v
		case "draft":
			switch v {
			case "true":
				in.draft = true
			case "false":
			default:
				in.close()
				return nil, errValidation("draft must be true or false")
			}
		case "subject":
			in.subject = &v
		default:
			if in.extra == nil {
				in.extra = map[string]string{}
			}
			in.extra[part.FormName()] = v
		}
	}
}

func uploadError(err error) error {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return err
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return errAttachmentTooLarge
	}
	if errors.Is(err, multipart.ErrMessageTooLarge) || strings.Contains(err.Error(), "multipart") {
		return errValidation("the multipart body is not valid")
	}
	return err
}

func attachmentBody(a store.Attachment) oas.Attachment {
	return oas.Attachment{
		Id: a.ID, Filename: a.Filename, ContentType: a.ContentType, Size: a.SizeBytes, ContentId: a.ContentID,
		Inline: a.Inline, CreatedAt: a.CreatedAt,
	}
}

func botAuthor(id *uuid.UUID, name, avatar string) oas.MessageAuthor {
	out := oas.MessageAuthor{Type: oas.AuthorTypeBot, ApiKeyId: id, Name: &name}
	if avatar != "" {
		out.AvatarUrl = &avatar
	}
	return out
}

func messageAuthor(m messageRow) oas.MessageAuthor {
	if m.AuthorType == string(oas.AuthorTypeBot) {
		return botAuthor(m.AuthorApiKeyID, m.BotName, m.BotAvatarUrl)
	}
	return oas.MessageAuthor{Type: oas.AuthorType(m.AuthorType), MemberId: m.AuthorMemberID, ContactId: m.AuthorContactID, Via: m.Via}
}

func messageBody(m messageRow, atts []store.Attachment) oas.Message {
	out := oas.Message{
		Id: m.ID, ConversationId: m.ConversationID, Kind: oas.MessageKind(m.Kind), Body: m.Body, Html: m.Html,
		ClientId: m.ClientID, CreatedAt: m.CreatedAt, Attachments: make([]oas.Attachment, 0, len(atts)), Draft: m.Draft,
		Author: messageAuthor(m),
	}
	switch {
	case m.SentByApiKeyID != nil:
		by := botAuthor(m.SentByApiKeyID, m.SentByBotName, "")
		out.SentBy = &by
	case m.SentByMemberID != nil:
		out.SentBy = &oas.MessageAuthor{Type: oas.AuthorTypeMember, MemberId: m.SentByMemberID, Via: m.SentVia}
	}
	if m.Direction != nil {
		d := oas.Direction(*m.Direction)
		out.Direction = &d
	}
	if m.Event != nil {
		var ev oas.MessageEvent
		if json.Unmarshal(m.Event, &ev) == nil {
			out.Event = &ev
		}
	}
	for _, a := range atts {
		out.Attachments = append(out.Attachments, attachmentBody(a))
	}
	if m.DeliveryState != nil && m.DeliveryUpdatedAt != nil {
		out.Delivery = &oas.MessageDelivery{
			Channel: oas.MessageDeliveryChannelEmail, State: oas.MessageDeliveryState(*m.DeliveryState),
			Error: m.DeliveryError, UpdatedAt: *m.DeliveryUpdatedAt,
		}
	}
	return out
}

func messageAttachments(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID][]store.Attachment, error) {
	rows, err := q.ListAttachments(ctx, store.ListAttachmentsParams{WorkspaceID: workspaceID, MessageIds: ids})
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID][]store.Attachment{}
	for _, r := range rows {
		out[r.MessageID] = append(out[r.MessageID], r)
	}
	return out, nil
}

func (s *Server) ListMessages(ctx context.Context, req oas.ListMessagesRequestObject) (oas.ListMessagesResponseObject, error) {
	p := principalFrom(ctx)
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	c, err := visibleConversation(ctx, s.st.Queries, p, req.ConversationId, false)
	if err != nil {
		return nil, err
	}
	var rows []store.ListMessagesRow
	switch order := req.Params.Order; {
	case order == nil || *order == oas.ListMessagesParamsOrderAsc:
		rows, err = s.st.ListMessages(ctx, store.ListMessagesParams{
			WorkspaceID: p.workspaceID, ConversationID: c.ID, CursorAt: at, CursorID: id, Lim: lim + 1,
		})
	case *order == oas.ListMessagesParamsOrderDesc:
		var desc []store.ListMessagesDescRow
		desc, err = s.st.ListMessagesDesc(ctx, store.ListMessagesDescParams{
			WorkspaceID: p.workspaceID, ConversationID: c.ID, CursorAt: at, CursorID: id, Lim: lim + 1,
		})
		for _, r := range desc {
			rows = append(rows, store.ListMessagesRow(r))
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
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	atts, err := messageAttachments(ctx, s.st.Queries, p.workspaceID, ids)
	if err != nil {
		return nil, err
	}
	emails, err := emailSummaries(ctx, s.st.Queries, p.workspaceID, ids)
	if err != nil {
		return nil, err
	}
	out := oas.ListMessages200JSONResponse{Items: make([]oas.Message, len(rows)), NextCursor: next}
	for i, r := range rows {
		out.Items[i] = withEmail(messageBody(messageRow(r), atts[r.ID]), emails[r.ID])
	}
	return out, nil
}

func (s *Server) CreateMessage(ctx context.Context, req oas.CreateMessageRequestObject) (oas.CreateMessageResponseObject, error) {
	p := principalFrom(ctx)
	if _, err := visibleConversation(ctx, s.st.Queries, p, req.ConversationId, false); err != nil {
		return nil, err
	}
	var in *messageInput
	switch {
	case req.JSONBody != nil:
		b := req.JSONBody
		in = &messageInput{kind: string(b.Kind), html: b.Html, clientID: b.ClientId, draft: b.Draft != nil && *b.Draft}
		if b.Direction != nil {
			in.direction = string(*b.Direction)
		}
		if b.Body != nil {
			in.body = *b.Body
		}
	case req.MultipartBody != nil:
		var err error
		if in, err = s.readMultipart(req.MultipartBody, p.workspaceID, memberFormFields); err != nil {
			return nil, err
		}
	default:
		return nil, errValidation("send application/json or multipart/form-data")
	}
	defer in.close()
	if err := s.validateMessage(p, in); err != nil {
		return nil, err
	}
	var stored []string
	for _, f := range in.files {
		if err := s.objects.Put(ctx, f.key, f.file, f.size, f.contentType); err != nil {
			s.deleteObjects(ctx, stored)
			return nil, err
		}
		stored = append(stored, f.key)
	}
	var (
		msg     messageRow
		atts    []store.Attachment
		summary *store.ListMessageEmailsRow
		created = true
	)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := visibleConversation(ctx, q, p, req.ConversationId, true)
		if err != nil {
			return err
		}
		if in.clientID != nil {
			prev, err := q.GetMessageByClientID(ctx, store.GetMessageByClientIDParams{WorkspaceID: p.workspaceID, ConversationID: c.ID, ClientID: in.clientID})
			if err == nil {
				msg, created = messageRow(prev), false
				m, err := messageAttachments(ctx, q, p.workspaceID, []uuid.UUID{prev.ID})
				atts = m[prev.ID]
				if err != nil {
					return err
				}
				em, err := emailSummaries(ctx, q, p.workspaceID, []uuid.UUID{prev.ID})
				summary = em[prev.ID]
				return err
			}
			if !store.IsNotFound(err) {
				return err
			}
		}
		now := s.now()
		deliver := in.kind == string(oas.MessageKindMessage) && in.direction == string(oas.Out) && !in.draft
		var plan *emailPlan
		if deliver {
			if plan, err = s.planEmail(ctx, q, c); err != nil {
				return err
			}
		}
		arg := store.CreateMessageParams{
			ID: newID(), WorkspaceID: p.workspaceID, ConversationID: c.ID, Kind: in.kind, Body: in.body,
			Html: in.html, ClientID: in.clientID, CreatedAt: now, Draft: in.draft, Via: p.viaClient(),
		}
		if in.kind == string(oas.MessageKindMessage) {
			arg.Direction = &in.direction
		}
		if in.direction == string(oas.In) {
			arg.AuthorType, arg.AuthorContactID = string(oas.AuthorTypeContact), &c.ContactID
		} else {
			arg.AuthorType, arg.AuthorMemberID, arg.AuthorApiKeyID = authorFor(p)
		}
		if plan != nil {
			queued := deliveryQueued
			arg.DeliveryState = &queued
		}
		if msg, err = q.CreateMessage(ctx, arg); err != nil {
			return err
		}
		if deliver {
			if summary, err = s.dispatchOutgoing(ctx, q, events, p, c, plan, msg, now); err != nil {
				return err
			}
		}
		var total int64
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
		if in.draft {
			events.conversation(realtime.DraftCreated, c, messageBody(msg, atts))
			return nil
		}
		if err := q.TouchConversation(ctx, store.TouchConversationParams{
			WorkspaceID: p.workspaceID, ID: c.ID, Now: now, IsMessage: in.kind == string(oas.MessageKindMessage),
		}); err != nil {
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
		return nil
	})
	if err != nil || !created {
		s.deleteObjects(ctx, stored)
	}
	if err != nil {
		return nil, err
	}
	if !created {
		return oas.CreateMessage200JSONResponse(withEmail(messageBody(msg, atts), summary)), nil
	}
	return oas.CreateMessage201JSONResponse(withEmail(messageBody(msg, atts), summary)), nil
}

func (s *Server) DownloadAttachment(ctx context.Context, req oas.DownloadAttachmentRequestObject) (oas.DownloadAttachmentResponseObject, error) {
	p := principalFrom(ctx)
	a, err := s.st.GetAttachment(ctx, store.GetAttachmentParams{WorkspaceID: p.workspaceID, ID: req.AttachmentId})
	if store.IsNotFound(err) {
		return nil, errAttachmentGone
	}
	if err != nil {
		return nil, err
	}
	ok, err := canSeeInbox(ctx, s.st.Queries, p, a.InboxID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errAttachmentGone
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
	return oas.DownloadAttachment200AsteriskResponse{
		Body: body, ContentType: a.ContentType, ContentLength: a.SizeBytes,
		Headers: oas.DownloadAttachment200ResponseHeaders{ContentDisposition: &disposition, XContentTypeOptions: &nosniff},
	}, nil
}

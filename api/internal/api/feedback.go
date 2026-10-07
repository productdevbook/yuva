package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

var feedbackCategories = []oas.FeedbackCategory{oas.Bug, oas.Idea, oas.Praise, oas.Other}

var (
	errFeedbackChannel     = problem(http.StatusForbidden, "forbidden", "feedback is sent from app channels")
	errFeedbackNeedsKey    = problem(http.StatusForbidden, "forbidden", "only an API key can post feedback for a contact")
	clientFeedbackFields   = []string{"category", "subject", "body", "client_id", "allow_email", "email", "app_version", "build", "os", "os_version", "device_model", "locale", "screen", "installation_id"}
	feedbackFieldMaxLength = map[string]int{
		"app_version": 50, "build": 50, "os": 50, "os_version": 50, "device_model": 100, "locale": 35, "screen": 200, "installation_id": 200,
	}
)

func feedbackFieldsOf(f oas.FeedbackFields) map[string]*string {
	return map[string]*string{
		"app_version": f.AppVersion, "build": f.Build, "os": f.Os, "os_version": f.OsVersion, "device_model": f.DeviceModel,
		"locale": f.Locale, "screen": f.Screen, "installation_id": f.InstallationId,
	}
}

func feedbackOf(category oas.FeedbackCategory, allowEmail bool, fields map[string]*string) (oas.Feedback, error) {
	out := oas.Feedback{Category: category, AllowEmail: allowEmail}
	if !category.Valid() {
		return out, errValidation("category must be bug, idea, praise or other")
	}
	clean := map[string]*string{}
	for name, v := range fields {
		if v == nil {
			continue
		}
		t, err := trimmed(*v, 0, feedbackFieldMaxLength[name], name)
		if err != nil {
			return out, err
		}
		if t != "" {
			clean[name] = &t
		}
	}
	out.AppVersion, out.Build, out.Os, out.OsVersion = clean["app_version"], clean["build"], clean["os"], clean["os_version"]
	out.DeviceModel, out.Locale, out.Screen, out.InstallationId = clean["device_model"], clean["locale"], clean["screen"], clean["installation_id"]
	return out, nil
}

func conversationFeedback(c store.Conversation) *oas.Feedback {
	if c.Kind != string(oas.ConversationKindFeedback) || c.Feedback == nil {
		return nil
	}
	var f oas.Feedback
	if json.Unmarshal(c.Feedback, &f) != nil {
		return nil
	}
	return &f
}

// emailsReplies tells whether members' replies the contact has not read go out by e-mail: always
// for chat, and for feedback whose contact allowed it.
func emailsReplies(c store.Conversation, channelKind string) bool {
	switch channelKind {
	case string(oas.ChannelKindChat):
		return true
	case string(oas.ChannelKindApp), string(oas.ChannelKindApi):
		f := conversationFeedback(c)
		return f != nil && f.AllowEmail
	}
	return false
}

func (s *Server) CreateClientFeedback(ctx context.Context, req oas.CreateClientFeedbackRequestObject) (oas.CreateClientFeedbackResponseObject, error) {
	cp := contactFrom(ctx)
	if cp.kind != string(oas.ChannelKindApp) {
		return nil, errFeedbackChannel
	}
	if err := s.rateLimit(s.writeChecks(ctx, cp)...); err != nil {
		return nil, err
	}
	var (
		in         *messageInput
		category   oas.FeedbackCategory
		allowEmail bool
		email      *oas.Email
		fields     map[string]*string
	)
	switch {
	case req.JSONBody != nil:
		b := req.JSONBody
		in = &messageInput{clientID: b.ClientId, subject: b.Subject}
		if b.Body != nil {
			in.body = *b.Body
		}
		category, email = b.Category, b.Email
		allowEmail = b.AllowEmail != nil && *b.AllowEmail
		fields = feedbackFieldsOf(oas.FeedbackFields{
			AppVersion: b.AppVersion, Build: b.Build, Os: b.Os, OsVersion: b.OsVersion, DeviceModel: b.DeviceModel,
			Locale: b.Locale, Screen: b.Screen, InstallationId: b.InstallationId,
		})
	case req.MultipartBody != nil:
		var err error
		if in, err = s.readMultipart(req.MultipartBody, cp.workspaceID, clientFeedbackFields); err != nil {
			return nil, err
		}
		category = oas.FeedbackCategory(in.extra["category"])
		if v, ok := in.extra["allow_email"]; ok {
			if allowEmail, err = strconv.ParseBool(v); err != nil {
				in.close()
				return nil, errValidation("allow_email must be true or false")
			}
		}
		if v, ok := in.extra["email"]; ok && strings.TrimSpace(v) != "" {
			e := oas.Email(v)
			email = &e
		}
		fields = map[string]*string{}
		for name := range feedbackFieldMaxLength {
			if v, ok := in.extra[name]; ok {
				fields[name] = &v
			}
		}
	default:
		return nil, errValidation("send application/json or multipart/form-data")
	}
	defer in.close()
	if err := s.validateClientInput(in); err != nil {
		return nil, err
	}
	fb, err := feedbackOf(category, allowEmail, fields)
	if err != nil {
		return nil, err
	}
	var addr *string
	if email != nil {
		a, err := normalizeEmail(*email)
		if err != nil {
			return nil, err
		}
		addr = &a
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
			return oas.CreateClientFeedback201JSONResponse(out), nil
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
		if fb.AllowEmail && addr != nil {
			r, err := q.SetContactTypedEmail(ctx, store.SetContactTypedEmailParams{WorkspaceID: cp.workspaceID, ID: cp.contactID, TypedEmail: addr, Now: s.now()})
			if err != nil {
				return err
			}
			body, err := s.contactBody(ctx, q, cp.workspaceID, contactRow(r))
			if err != nil {
				return err
			}
			events.add(realtime.ContactUpdated, nil, nil, body)
		}
		c, err := s.createFeedbackConversation(ctx, q, events, cp.workspaceID, cp.inboxID, cp.contactID, cp.channelID, in.subject, fb)
		if err != nil {
			return err
		}
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
	return oas.CreateClientFeedback201JSONResponse(out), nil
}

func (s *Server) createFeedbackConversation(ctx context.Context, q *store.Queries, events *eventBatch, ws, inboxID, contactID, channelID uuid.UUID, subject *string, fb oas.Feedback) (store.Conversation, error) {
	subj := ""
	if subject != nil {
		subj = *subject
	}
	kind := string(oas.ConversationKindFeedback)
	c, err := q.CreateConversation(ctx, store.CreateConversationParams{
		ID: newID(), WorkspaceID: ws, InboxID: inboxID, ContactID: contactID, ChannelID: &channelID,
		Subject: subj, Priority: string(oas.Normal), Kind: &kind, Feedback: mustJSON(fb), Now: s.now(),
	})
	if err != nil {
		return c, err
	}
	events.conversation(realtime.ConversationCreated, c, conversationBody(c, nil))
	return c, nil
}

func (s *Server) CreateFeedback(ctx context.Context, req oas.CreateFeedbackRequestObject) (oas.CreateFeedbackResponseObject, error) {
	p := principalFrom(ctx)
	if !p.isKey() {
		return nil, errFeedbackNeedsKey
	}
	b := req.Body
	in := &messageInput{body: b.Body, clientID: b.ClientId, subject: b.Subject}
	if err := s.validateClientInput(in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(b.Body) == "" {
		return nil, errValidation("body is required")
	}
	fb, err := feedbackOf(b.Category, b.AllowEmail != nil && *b.AllowEmail, feedbackFieldsOf(oas.FeedbackFields{
		AppVersion: b.AppVersion, Build: b.Build, Os: b.Os, OsVersion: b.OsVersion, DeviceModel: b.DeviceModel,
		Locale: b.Locale, Screen: b.Screen, InstallationId: b.InstallationId,
	}))
	if err != nil {
		return nil, err
	}
	externalID, err := trimmed(b.Contact.ExternalId, 1, 200, "contact.external_id")
	if err != nil {
		return nil, err
	}
	var name, addr string
	if b.Contact.Name != nil {
		if name, err = trimmed(*b.Contact.Name, 0, 200, "contact.name"); err != nil {
			return nil, err
		}
	}
	if b.Contact.Email != nil {
		if addr, err = normalizeEmail(*b.Contact.Email); err != nil {
			return nil, err
		}
	}
	inbox, err := visibleInbox(ctx, s.st.Queries, p, b.InboxId)
	if err != nil {
		return nil, err
	}
	var (
		out     oas.FeedbackCreated
		created = true
	)
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		contact, err := s.feedbackContact(ctx, q, events, inbox, externalID, name, addr)
		if err != nil {
			return err
		}
		if out.Contact, err = s.contactBody(ctx, q, p.workspaceID, contact); err != nil {
			return err
		}
		if in.clientID != nil {
			prev, err := q.FindContactMessageByClientID(ctx, store.FindContactMessageByClientIDParams{
				WorkspaceID: p.workspaceID, InboxID: inbox.ID, ContactID: contact.ID, ClientID: *in.clientID,
			})
			if err == nil {
				created = false
				c, err := q.GetConversation(ctx, store.GetConversationParams{WorkspaceID: p.workspaceID, ID: prev.ConversationID})
				if err != nil {
					return err
				}
				if out.Conversation, err = oneConversation(ctx, q, c); err != nil {
					return err
				}
				m, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: p.workspaceID, ID: prev.ID})
				if err != nil {
					return err
				}
				atts, err := messageAttachments(ctx, q, p.workspaceID, []uuid.UUID{m.ID})
				out.Message = messageBody(messageRow(m), atts[m.ID])
				return err
			}
			if !store.IsNotFound(err) {
				return err
			}
		}
		ch, err := s.inboxAPIChannel(ctx, q, inbox)
		if err != nil {
			return err
		}
		c, err := s.createFeedbackConversation(ctx, q, events, p.workspaceID, inbox.ID, contact.ID, ch.ID, in.subject, fb)
		if err != nil {
			return err
		}
		cp := contactPrincipal{workspaceID: p.workspaceID, inboxID: inbox.ID, channelID: ch.ID, contactID: contact.ID}
		msg, atts, err := s.addContactMessage(ctx, q, events, cp, c, in, true)
		if err != nil {
			return err
		}
		out.Message = messageBody(msg, atts)
		out.Conversation, err = oneConversation(ctx, q, c)
		return err
	})
	if err != nil {
		return nil, err
	}
	if !created {
		return oas.CreateFeedback200JSONResponse(out), nil
	}
	return oas.CreateFeedback201JSONResponse(out), nil
}

// inboxAPIChannel returns the inbox's oldest api channel, creating one named "API" on first use.
func (s *Server) inboxAPIChannel(ctx context.Context, q *store.Queries, inbox store.Inbox) (store.Channel, error) {
	arg := store.InboxAPIChannelParams{WorkspaceID: inbox.WorkspaceID, InboxID: inbox.ID}
	ch, err := q.InboxAPIChannel(ctx, arg)
	if !store.IsNotFound(err) {
		return ch, err
	}
	if _, err := q.LockInbox(ctx, store.LockInboxParams{WorkspaceID: inbox.WorkspaceID, ID: inbox.ID}); err != nil {
		if store.IsNotFound(err) {
			return ch, errInboxGone
		}
		return ch, err
	}
	ch, err = q.InboxAPIChannel(ctx, arg)
	if !store.IsNotFound(err) {
		return ch, err
	}
	return q.CreateChannel(ctx, store.CreateChannelParams{
		ID: newID(), WorkspaceID: inbox.WorkspaceID, InboxID: inbox.ID, Kind: string(oas.ChannelKindApi),
		Name: "API", Settings: []byte("{}"), Now: s.now(),
	})
}

// feedbackContact finds the host's user by external id in the inbox, then by e-mail, or creates
// them, and saves the name and address the host sent.
func (s *Server) feedbackContact(ctx context.Context, q *store.Queries, events *eventBatch, inbox store.Inbox, externalID, name, addr string) (contactRow, error) {
	ws, now := inbox.WorkspaceID, s.now()
	changed := false
	id, err := q.GetContactIDByExternalID(ctx, store.GetContactIDByExternalIDParams{WorkspaceID: ws, InboxID: inbox.ID, ExternalID: externalID})
	if store.IsNotFound(err) && addr != "" {
		id, err = q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: addr})
		if err == nil {
			err = q.AddContactExternalID(ctx, store.AddContactExternalIDParams{WorkspaceID: ws, InboxID: inbox.ID, ExternalID: externalID, ContactID: id})
			changed = true
		}
	}
	if store.IsNotFound(err) {
		r, cerr := q.CreateContact(ctx, store.CreateContactParams{ID: newID(), WorkspaceID: ws, Name: name, Attributes: []byte("{}"), Now: now})
		if cerr != nil {
			return contactRow{}, cerr
		}
		id = r.ID
		err = q.AddContactExternalID(ctx, store.AddContactExternalIDParams{WorkspaceID: ws, InboxID: inbox.ID, ExternalID: externalID, ContactID: id})
		changed = true
	}
	if err != nil {
		return contactRow{}, err
	}
	cur, err := q.LockContact(ctx, store.LockContactParams{WorkspaceID: ws, ID: id})
	if err != nil {
		return contactRow{}, err
	}
	if cur.Blocked {
		return contactRow{}, errContactBlocked
	}
	if addr != "" {
		owner, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: addr})
		if store.IsNotFound(err) {
			n, err := q.CountContactEmails(ctx, store.CountContactEmailsParams{WorkspaceID: ws, ContactID: id})
			if err != nil {
				return contactRow{}, err
			}
			if err := q.AddContactEmail(ctx, store.AddContactEmailParams{WorkspaceID: ws, ContactID: id, Email: addr, Position: int32(n)}); err != nil {
				return contactRow{}, err
			}
			changed = true
		} else if err != nil {
			return contactRow{}, err
		} else if owner != id {
			s.log.InfoContext(ctx, "feedback e-mail belongs to another contact", "contact_id", id, "owner_id", owner)
		}
	}
	row := cur
	if name != "" && name != cur.Name {
		r, err := q.UpdateContact(ctx, store.UpdateContactParams{WorkspaceID: ws, ID: id, Name: name, Attributes: cur.Attributes, Blocked: cur.Blocked, Now: now})
		if err != nil {
			return contactRow{}, err
		}
		row, changed = store.LockContactRow(r), true
	}
	if !changed {
		return contactRow(row), nil
	}
	if err := q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: ws, ID: id}); err != nil {
		return contactRow{}, err
	}
	body, err := s.contactBody(ctx, q, ws, contactRow(row))
	if err != nil {
		return contactRow{}, err
	}
	events.add(realtime.ContactUpdated, nil, nil, body)
	return contactRow(row), nil
}

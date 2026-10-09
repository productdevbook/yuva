package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const visitorPrefix = "yuva_v_"

var (
	errUnknownChannelKey  = problem(http.StatusNotFound, "not_found", "no chat or app channel has this key")
	errAnonymousForbidden = problem(http.StatusForbidden, "anonymous_not_allowed", "this channel needs an identity token")
	errAnonymousLimit     = problem(http.StatusTooManyRequests, "anonymous_limit", "too many new visitors from this address in the last hour; try again later")
)

func identitySecretContext(workspaceID, inboxID uuid.UUID) []byte {
	return append(append([]byte{}, workspaceID[:]...), inboxID[:]...)
}

func (s *Server) CreateClientSession(ctx context.Context, req oas.CreateClientSessionRequestObject) (oas.CreateClientSessionResponseObject, error) {
	b := req.Body
	ip := rateIP(s.clientIP(requestFrom(ctx)))
	if err := s.rateLimit(rateCheck{"session:ip:" + ip, limitSessionPerIP}); err != nil {
		return nil, err
	}
	key := strings.TrimSpace(b.ChannelKey)
	if key == "" || len(key) > 200 {
		return nil, errValidation("channel_key is required")
	}
	ch, err := s.st.FindChatChannelByKey(ctx, key)
	if store.IsNotFound(err) {
		return nil, errUnknownChannelKey
	}
	if err != nil {
		return nil, err
	}
	if err := s.rateLimit(rateCheck{"session:channel:" + ch.ChannelID.String(), limitSessionPerChannel}); err != nil {
		return nil, err
	}
	if !s.originAllowedFor(originFrom(ctx), ch.Kind, ch.AllowedOrigins) {
		return nil, errOriginRefused
	}
	ws := ch.WorkspaceID
	inbox, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: ch.InboxID})
	if err != nil {
		return nil, err
	}
	var claims *identityClaims
	if b.IdentityToken != nil && strings.TrimSpace(*b.IdentityToken) != "" {
		secret, err := s.secrets.Open(inbox.IdentitySecret, identitySecretContext(ws, inbox.ID))
		if err != nil {
			return nil, err
		}
		c, err := verifyIdentityToken(strings.TrimSpace(*b.IdentityToken), secret, s.now())
		if err != nil {
			return nil, err
		}
		claims = &c
	} else if !ch.AllowAnonymous {
		return nil, errAnonymousForbidden
	}
	var visitor string
	if b.VisitorId != nil {
		visitor = strings.TrimSpace(*b.VisitorId)
	}
	chat := chatChannelOf(ch)
	var out oas.ClientSession
	for attempt := 0; ; attempt++ {
		out, err = s.startContactSession(ctx, chat, inbox, claims, visitor)
		if err == nil || attempt == 1 || !store.IsUniqueViolation(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return oas.CreateClientSession201JSONResponse(out), nil
}

func chatChannelOf(ch store.FindChatChannelByKeyRow) store.ChatChannel {
	return store.ChatChannel{
		WorkspaceID: ch.WorkspaceID, ChannelID: ch.ChannelID, PublicKey: ch.PublicKey, AllowedOrigins: ch.AllowedOrigins,
		AllowAnonymous: ch.AllowAnonymous, AskEmailOffline: ch.AskEmailOffline, Greeting: ch.Greeting,
		LauncherPosition: ch.LauncherPosition, LauncherColor: ch.LauncherColor, Platforms: ch.Platforms,
	}
}

func (s *Server) GetClientChannel(ctx context.Context, req oas.GetClientChannelRequestObject) (oas.GetClientChannelResponseObject, error) {
	if err := s.rateLimit(rateCheck{"channel:ip:" + rateIP(s.clientIP(requestFrom(ctx))), limitChannelPerIP}); err != nil {
		return nil, err
	}
	key := strings.TrimSpace(req.ChannelKey)
	if key == "" || len(key) > 200 {
		return nil, errUnknownChannelKey
	}
	ch, err := s.st.FindChatChannelByKey(ctx, key)
	if store.IsNotFound(err) {
		return nil, errUnknownChannelKey
	}
	if err != nil {
		return nil, err
	}
	if !s.originAllowedFor(originFrom(ctx), ch.Kind, ch.AllowedOrigins) {
		return nil, errOriginRefused
	}
	inbox, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ch.WorkspaceID, ID: ch.InboxID})
	if err != nil {
		return nil, err
	}
	out, err := s.clientInbox(ctx, s.st.Queries, inbox, chatChannelOf(ch))
	if err != nil {
		return nil, err
	}
	return oas.GetClientChannel200JSONResponse(out), nil
}

func (s *Server) startContactSession(ctx context.Context, chat store.ChatChannel, inbox store.Inbox, claims *identityClaims, visitor string) (oas.ClientSession, error) {
	ws := chat.WorkspaceID
	var out oas.ClientSession
	err := s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
		var (
			contact contactRow
			err     error
		)
		if claims != nil && claims.jti != "" {
			if err := s.useTokenID(ctx, q, inbox, *claims); err != nil {
				return err
			}
		}
		if claims != nil {
			contact, err = s.identifyContact(ctx, q, events, inbox, *claims, visitor)
		} else {
			contact, visitor, err = s.anonymousContact(ctx, q, chat.ChannelID, inbox, visitor)
			out.VisitorId = &visitor
		}
		if err != nil {
			return err
		}
		if contact.Blocked {
			return errContactBlocked
		}
		now := s.now()
		token := contactTokenPrefix + randomToken(32)
		sess, err := q.CreateContactSession(ctx, store.CreateContactSessionParams{
			ID: newID(), WorkspaceID: ws, ChannelID: chat.ChannelID, InboxID: inbox.ID, ContactID: contact.ID,
			TokenHash: hashSecret(token), Identified: claims != nil, Now: now, ExpiresAt: now.Add(contactSessionTTL),
		})
		if err != nil {
			return err
		}
		out.Token, out.ExpiresAt = token, sess.ExpiresAt
		if out.Contact, err = clientContactBody(ctx, q, contact, claims != nil); err != nil {
			return err
		}
		out.Inbox, err = s.clientInbox(ctx, q, inbox, chat)
		return err
	})
	return out, err
}

var errTokenReused = errIdentity("the token's jti was already used")

// useTokenID makes a token that carries a jti single-use within its lifetime.
func (s *Server) useTokenID(ctx context.Context, q *store.Queries, inbox store.Inbox, c identityClaims) error {
	now := s.now()
	if err := q.DeleteExpiredIdentityTokenIDs(ctx, store.DeleteExpiredIdentityTokenIDsParams{WorkspaceID: inbox.WorkspaceID, InboxID: inbox.ID, Now: now.Add(-identityLeeway)}); err != nil {
		return err
	}
	n, err := q.UseIdentityTokenID(ctx, store.UseIdentityTokenIDParams{WorkspaceID: inbox.WorkspaceID, InboxID: inbox.ID, Jti: c.jti, ExpiresAt: c.exp})
	if err != nil {
		return err
	}
	if n == 0 {
		return errTokenReused
	}
	return nil
}

func (s *Server) anonymousContact(ctx context.Context, q *store.Queries, channelID uuid.UUID, inbox store.Inbox, visitor string) (contactRow, string, error) {
	ws := inbox.WorkspaceID
	if visitor != "" {
		id, err := q.GetChatVisitor(ctx, store.GetChatVisitorParams{WorkspaceID: ws, InboxID: inbox.ID, VisitorHash: hashSecret(visitor)})
		if err == nil {
			c, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: id})
			return c, visitor, err
		}
		if !store.IsNotFound(err) {
			return contactRow{}, "", err
		}
	}
	now := s.now()
	key := "anonymous:" + channelID.String() + ":" + rateIP(s.clientIP(requestFrom(ctx)))
	if !s.limits.allow(key, limit{s.chat.AnonymousContactsPerHour, time.Hour}, now) {
		return contactRow{}, "", errAnonymousLimit
	}
	r, err := q.CreateContact(ctx, store.CreateContactParams{ID: newID(), WorkspaceID: ws, Attributes: []byte("{}"), Now: now})
	if err != nil {
		return contactRow{}, "", err
	}
	visitor = visitorPrefix + randomToken(24)
	if err := q.CreateChatVisitor(ctx, store.CreateChatVisitorParams{
		WorkspaceID: ws, InboxID: inbox.ID, VisitorHash: hashSecret(visitor), ContactID: r.ID, CreatedAt: now,
	}); err != nil {
		return contactRow{}, "", err
	}
	return contactRow(r), visitor, nil
}

// identifyContact finds the contact of a verified identity token: by external id, then by the
// token's verified e-mail on a contact without an external id in this inbox, then the anonymous
// visitor of this browser, or a new contact. An anonymous visitor of this browser that is not that
// contact is merged into it.
func (s *Server) identifyContact(ctx context.Context, q *store.Queries, events *eventBatch, inbox store.Inbox, c identityClaims, visitor string) (contactRow, error) {
	ws := inbox.WorkspaceID
	now := s.now()
	var visitorContact *uuid.UUID
	if visitor != "" {
		id, err := q.GetChatVisitor(ctx, store.GetChatVisitorParams{WorkspaceID: ws, InboxID: inbox.ID, VisitorHash: hashSecret(visitor)})
		if err == nil {
			inInbox, _, err := externalIDPlaces(ctx, q, ws, inbox.ID, id)
			if err != nil {
				return contactRow{}, err
			}
			if !inInbox {
				visitorContact = &id
			}
		} else if !store.IsNotFound(err) {
			return contactRow{}, err
		}
	}
	addExternal, fillOnly := false, false
	id, err := q.GetContactIDByExternalID(ctx, store.GetContactIDByExternalIDParams{WorkspaceID: ws, InboxID: inbox.ID, ExternalID: c.sub})
	if store.IsNotFound(err) && c.emailVerified {
		owner, oerr := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: c.email})
		if oerr != nil && !store.IsNotFound(oerr) {
			return contactRow{}, oerr
		}
		if oerr == nil {
			inInbox, elsewhere, xerr := externalIDPlaces(ctx, q, ws, inbox.ID, owner)
			if xerr != nil {
				return contactRow{}, xerr
			}
			if !inInbox {
				addExternal, fillOnly, id, err = true, elsewhere, owner, nil
			}
		}
	}
	if store.IsNotFound(err) && visitorContact != nil {
		addExternal, id, err = true, *visitorContact, nil
	}
	if store.IsNotFound(err) {
		r, cerr := q.CreateContact(ctx, store.CreateContactParams{ID: newID(), WorkspaceID: ws, Attributes: []byte("{}"), Now: now})
		if cerr != nil {
			return contactRow{}, cerr
		}
		addExternal, id, err = true, r.ID, nil
	}
	if err != nil {
		return contactRow{}, err
	}
	if addExternal {
		if err := q.AddContactExternalID(ctx, store.AddContactExternalIDParams{WorkspaceID: ws, InboxID: inbox.ID, ExternalID: c.sub, ContactID: id}); err != nil {
			return contactRow{}, err
		}
	}
	cur, err := q.LockContact(ctx, store.LockContactParams{WorkspaceID: ws, ID: id})
	if err != nil {
		return contactRow{}, err
	}
	if visitorContact != nil && *visitorContact != id {
		if err := s.mergeContact(ctx, q, events, ws, *visitorContact, id); err != nil {
			return contactRow{}, err
		}
	} else if visitorContact != nil {
		if err := q.DeleteChatVisitor(ctx, store.DeleteChatVisitorParams{WorkspaceID: ws, InboxID: inbox.ID, VisitorHash: hashSecret(visitor)}); err != nil {
			return contactRow{}, err
		}
	}
	if c.email != "" {
		owner, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: c.email})
		switch {
		case err != nil && !store.IsNotFound(err):
			return contactRow{}, err
		case err == nil && owner != id:
			s.log.InfoContext(ctx, "identity token e-mail belongs to another contact", "contact_id", id, "owner_id", owner)
		case err == nil:
		case c.emailVerified:
			n, err := q.CountContactEmails(ctx, store.CountContactEmailsParams{WorkspaceID: ws, ContactID: id})
			if err != nil {
				return contactRow{}, err
			}
			if err := q.AddContactEmail(ctx, store.AddContactEmailParams{WorkspaceID: ws, ContactID: id, Email: c.email, Position: int32(n)}); err != nil {
				return contactRow{}, err
			}
		case cur.TypedEmail == nil || *cur.TypedEmail != c.email:
			r, err := q.SetContactTypedEmail(ctx, store.SetContactTypedEmailParams{WorkspaceID: ws, ID: id, TypedEmail: &c.email, Now: now})
			if err != nil {
				return contactRow{}, err
			}
			cur = store.LockContactRow(r)
		}
	}
	name, locale, attrs := cur.Name, cur.Locale, cur.Attributes
	if c.name != nil && *c.name != "" && (!fillOnly || name == "") {
		name = *c.name
	}
	if c.locale != nil && (!fillOnly || locale == nil) {
		locale = c.locale
	}
	if c.attrs != nil {
		merged := map[string]any{}
		_ = json.Unmarshal(cur.Attributes, &merged)
		var add map[string]any
		_ = json.Unmarshal(c.attrs, &add)
		for k, v := range add {
			if _, taken := merged[k]; !fillOnly || !taken {
				merged[k] = v
			}
		}
		if attrs = mustJSON(merged); len(attrs) > maxAttributesBytes {
			attrs = c.attrs
			if fillOnly {
				attrs = cur.Attributes
			}
		}
	}
	r, err := q.SetContactIdentity(ctx, store.SetContactIdentityParams{WorkspaceID: ws, ID: id, Name: name, Locale: locale, Attributes: attrs, Now: now})
	if err != nil {
		return contactRow{}, err
	}
	if err := q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: ws, ID: id}); err != nil {
		return contactRow{}, err
	}
	body, err := s.contactBody(ctx, q, ws, contactRow(r))
	if err != nil {
		return contactRow{}, err
	}
	events.add(realtime.ContactUpdated, nil, nil, body)
	return contactRow(r), nil
}

// externalIDPlaces reports whether a contact has an external id in the inbox and whether it has
// any in another inbox.
func externalIDPlaces(ctx context.Context, q *store.Queries, ws, inboxID, contactID uuid.UUID) (inInbox, elsewhere bool, err error) {
	rows, err := q.ListContactExternalIDs(ctx, store.ListContactExternalIDsParams{WorkspaceID: ws, ContactIds: []uuid.UUID{contactID}})
	for _, r := range rows {
		if r.InboxID == inboxID {
			inInbox = true
		} else {
			elsewhere = true
		}
	}
	return inInbox, elsewhere, err
}

// mergeContact moves an anonymous visitor's conversations, messages and verified addresses to the
// identified contact and deletes the visitor's contact.
func (s *Server) mergeContact(ctx context.Context, q *store.Queries, events *eventBatch, ws, from, to uuid.UUID) error {
	now := s.now()
	moved, err := q.MoveContactConversations(ctx, store.MoveContactConversationsParams{WorkspaceID: ws, FromContact: from, ToContact: to, Now: now})
	if err != nil {
		return err
	}
	if err := q.MoveContactMessages(ctx, store.MoveContactMessagesParams{WorkspaceID: ws, FromContact: &from, ToContact: &to}); err != nil {
		return err
	}
	if err := q.MoveContactEmails(ctx, store.MoveContactEmailsParams{WorkspaceID: ws, FromContact: from, ToContact: to}); err != nil {
		return err
	}
	for _, row := range moved {
		c := store.Conversation(row)
		body, err := oneConversation(ctx, q, c)
		if err != nil {
			return err
		}
		events.conversation(realtime.ConversationUpdated, c, body)
	}
	if _, err := q.DeleteContact(ctx, store.DeleteContactParams{WorkspaceID: ws, ID: from}); err != nil {
		return err
	}
	events.add(realtime.ContactDeleted, nil, nil, oas.ContactRef{Id: from})
	return nil
}

func clientContactBody(ctx context.Context, q *store.Queries, c contactRow, identified bool) (oas.ClientContact, error) {
	out := oas.ClientContact{Id: c.ID, Name: c.Name, Identified: identified}
	emails, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: c.WorkspaceID, ContactIds: []uuid.UUID{c.ID}})
	if err != nil {
		return out, err
	}
	if len(emails) > 0 {
		e := oas.Email(emails[0].Email)
		out.Email = &e
	}
	if c.TypedEmail != nil {
		e := oas.Email(*c.TypedEmail)
		out.TypedEmail = &e
	}
	return out, nil
}

func (s *Server) clientInbox(ctx context.Context, q *store.Queries, in store.Inbox, chat store.ChatChannel) (oas.ClientInbox, error) {
	b := inboxBody(in)
	out := oas.ClientInbox{
		Id: in.ID, Name: in.Name, Branding: b.Branding, DefaultLocale: in.DefaultLocale, Timezone: in.Timezone,
		Mode: b.Mode, BusinessHours: b.BusinessHours, OpenNow: inboxOpen(in, s.now()),
		Chat: oas.ClientChatSettings{
			Greeting: chat.Greeting, LauncherPosition: oas.Right, LauncherColor: chat.LauncherColor,
			AskEmailOffline: chat.AskEmailOffline, AllowAnonymous: chat.AllowAnonymous,
		},
		FeedbackCategories: feedbackCategories,
		AskForRating:       in.AskForRating,
	}
	if out.Chat.Greeting == "" && b.Branding.Greeting != nil {
		out.Chat.Greeting = *b.Branding.Greeting
	}
	if out.Chat.LauncherColor == nil {
		out.Chat.LauncherColor = b.Branding.Color
	}
	if chat.LauncherPosition != nil {
		out.Chat.LauncherPosition = oas.ChatLauncherPosition(*chat.LauncherPosition)
	}
	if in.Mode == string(oas.Async) {
		out.ExpectedReplyMinutes = in.ExpectedReplyMinutes
	}
	var err error
	out.Presence, err = s.presence(ctx, q, in)
	return out, err
}

func (s *Server) clientSessionInfo(ctx context.Context, cp contactPrincipal) (oas.ClientSessionInfo, error) {
	var out oas.ClientSessionInfo
	c, err := s.st.GetContact(ctx, store.GetContactParams{WorkspaceID: cp.workspaceID, ID: cp.contactID})
	if err != nil {
		return out, err
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: cp.workspaceID, ID: cp.inboxID})
	if err != nil {
		return out, err
	}
	out.ExpiresAt = cp.expiresAt
	if out.Contact, err = clientContactBody(ctx, s.st.Queries, c, cp.identified); err != nil {
		return out, err
	}
	out.Inbox, err = s.clientInbox(ctx, s.st.Queries, in, cp.chat)
	return out, err
}

func (s *Server) GetClientSession(ctx context.Context, _ oas.GetClientSessionRequestObject) (oas.GetClientSessionResponseObject, error) {
	out, err := s.clientSessionInfo(ctx, contactFrom(ctx))
	if err != nil {
		return nil, err
	}
	return oas.GetClientSession200JSONResponse(out), nil
}

func (s *Server) DeleteClientSession(ctx context.Context, _ oas.DeleteClientSessionRequestObject) (oas.DeleteClientSessionResponseObject, error) {
	cp := contactFrom(ctx)
	if err := s.st.DeleteContactSession(ctx, store.DeleteContactSessionParams{WorkspaceID: cp.workspaceID, ID: cp.sessionID}); err != nil {
		return nil, err
	}
	return oas.DeleteClientSession204Response{}, nil
}

func (s *Server) SetClientContactEmail(ctx context.Context, req oas.SetClientContactEmailRequestObject) (oas.SetClientContactEmailResponseObject, error) {
	cp := contactFrom(ctx)
	if !cp.chat.AskEmailOffline {
		return nil, problem(http.StatusForbidden, "forbidden", "this channel does not ask for an e-mail address")
	}
	if err := s.rateLimit(s.writeChecks(ctx, cp)...); err != nil {
		return nil, err
	}
	addr, err := normalizeEmail(req.Body.Email)
	if err != nil {
		return nil, err
	}
	var (
		out     oas.ClientContact
		confirm *mail.Message
	)
	err = s.inTx(ctx, cp.workspaceID, func(q *store.Queries, events *eventBatch) error {
		r, err := q.SetContactTypedEmail(ctx, store.SetContactTypedEmailParams{WorkspaceID: cp.workspaceID, ID: cp.contactID, TypedEmail: &addr, Now: s.now()})
		if err != nil {
			return err
		}
		if confirm, err = s.confirmTypedEmail(ctx, q, cp, addr); err != nil {
			return err
		}
		if out, err = clientContactBody(ctx, q, contactRow(r), cp.identified); err != nil {
			return err
		}
		body, err := s.contactBody(ctx, q, cp.workspaceID, contactRow(r))
		if err != nil {
			return err
		}
		events.add(realtime.ContactUpdated, nil, nil, body)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if confirm != nil {
		s.sendMail(ctx, *confirm)
	}
	return oas.SetClientContactEmail200JSONResponse(out), nil
}

func (s *Server) confirmTypedEmail(ctx context.Context, q *store.Queries, cp contactPrincipal, addr string) (*mail.Message, error) {
	inbox, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: cp.workspaceID, ID: cp.inboxID})
	if err != nil {
		return nil, err
	}
	return s.requestEmailConfirmation(ctx, q, inbox, cp.contactID, addr)
}

func (s *Server) writeChecks(ctx context.Context, cp contactPrincipal) []rateCheck {
	return []rateCheck{
		{"write:ip:" + rateIP(s.clientIP(requestFrom(ctx))), limitWritePerIP},
		{"write:channel:" + cp.channelID.String(), limitWritePerChannel},
	}
}

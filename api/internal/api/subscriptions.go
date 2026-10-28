package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	eventSubscriptionCreated = string(oas.SubscriptionEventTypeSubscriptionCreated)
	eventSubscriptionUpdated = string(oas.SubscriptionEventTypeSubscriptionUpdated)

	subPending      = string(oas.SubscriptionStatePending)
	subSubscribed   = string(oas.SubscriptionStateSubscribed)
	subUnsubscribed = string(oas.SubscriptionStateUnsubscribed)

	consentRequested        = string(oas.ConsentEventKindRequested)
	consentConfirmationSent = string(oas.ConsentEventKindConfirmationSent)
	consentConfirmed        = string(oas.ConsentEventKindConfirmed)
	consentSubscribed       = string(oas.ConsentEventKindSubscribed)
	consentUnsubscribed     = string(oas.ConsentEventKindUnsubscribed)
	consentComplained       = string(oas.ConsentEventKindComplained)

	optOutAll       = "unsubscribed_all"
	optOutComplaint = "complaint"
	optOutErasure   = "erasure_after_optout"

	pendingKept       = 30 * 24 * time.Hour
	unsubscribedKept  = 3 * 365 * 24 * time.Hour
	maxConfirmations  = 10
	maxAddressOctets  = 254
	consentExportPage = 1000
)

var (
	errSubscriptionGone     = problem(http.StatusNotFound, "not_found", "no such subscription")
	errSubUnsubscribed      = problem(http.StatusConflict, "unsubscribed", "the address unsubscribed from this list; only the person can come back, by asking on a form or in the app and confirming the mail they get")
	errSubOptedOut          = problem(http.StatusConflict, "opted_out", "the address objected to mail from this workspace; only the person can subscribe it again")
	errSubSuppressed        = problem(http.StatusConflict, "suppressed", "mail to this address bounced or was reported as spam")
	errInvalidEmail         = problem(http.StatusBadRequest, "invalid_email", "email is not a valid address")
	errConfirmationLimit    = problem(http.StatusTooManyRequests, "confirmation_limit", "this address got as many confirmation mails as allowed for now")
	errConfirmationRequired = problem(http.StatusConflict, "confirmation_required", "only the person can subscribe, by confirming the mail they get")
	errNotPending           = problem(http.StatusConflict, "not_pending", "the subscription is not waiting for a confirmation")
)

var (
	limitConfirmPerAddress     = limit{3, 24 * time.Hour}
	limitConfirmPerAddressList = limit{1, 10 * time.Minute}
	limitConfirmPerChannel     = limit{1000, 24 * time.Hour}
	limitConfirmPerSub         = limit{3, 24 * time.Hour}
)

// subscriptionAddress accepts a bare address (no display name) of at most 254 octets with a
// dotted domain, lowercased.
func subscriptionAddress(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	a, err := mail.ParseAddress(raw)
	if err != nil || !strings.EqualFold(a.Address, raw) || len(a.Address) > maxAddressOctets {
		return "", errInvalidEmail
	}
	_, domain, _ := strings.Cut(a.Address, "@")
	if !strings.Contains(strings.Trim(domain, "."), ".") {
		return "", errInvalidEmail
	}
	return strings.ToLower(a.Address), nil
}

// optOutHashes keys addresses to a workspace under its opt-out key, so a row proves an objection
// without keeping the address.
func (s *Server) optOutHashes(ctx context.Context, q *store.Queries, ws uuid.UUID, emails ...string) ([][]byte, error) {
	key, err := s.optOutKey(ctx, q, ws, false)
	if err != nil {
		return nil, err
	}
	out := make([][]byte, len(emails))
	for i, e := range emails {
		out[i] = optOutHash(key, e)
	}
	return out, nil
}

func (s *Server) rememberOptOuts(ctx context.Context, q *store.Queries, ws uuid.UUID, emails []string, reason string) error {
	if len(emails) == 0 {
		return nil
	}
	key, err := s.optOutKey(ctx, q, ws, true)
	if err != nil {
		return err
	}
	for _, e := range emails {
		if err := q.AddOptOut(ctx, store.AddOptOutParams{WorkspaceID: ws, EmailHash: optOutHash(key, e), Reason: reason, Now: s.now()}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) optedOut(ctx context.Context, q *store.Queries, ws uuid.UUID, email string) (bool, error) {
	h, err := s.optOutHashes(ctx, q, ws, email)
	if err != nil {
		return false, err
	}
	return q.IsOptedOut(ctx, store.IsOptedOutParams{WorkspaceID: ws, EmailHash: h[0]})
}

func (s *Server) forgetOptOut(ctx context.Context, q *store.Queries, ws uuid.UUID, email string) error {
	h, err := s.optOutHashes(ctx, q, ws, email)
	if err != nil {
		return err
	}
	return q.DeleteOptOut(ctx, store.DeleteOptOutParams{WorkspaceID: ws, EmailHash: h[0]})
}

// rememberContactObjections keeps, before a contact is deleted, the addresses that unsubscribed
// from a list or complained, so nobody can add them back.
func (s *Server) rememberContactObjections(ctx context.Context, q *store.Queries, ws, contactID uuid.UUID) error {
	emails, err := q.ContactObjectionEmails(ctx, store.ContactObjectionEmailsParams{WorkspaceID: ws, ContactID: contactID})
	if err != nil {
		return err
	}
	return s.rememberOptOuts(ctx, q, ws, emails, optOutErasure)
}

func subscriptionBody(sub store.ListSubscription, name string) oas.Subscription {
	out := oas.Subscription{
		Id: sub.ID, ListId: sub.ListID, Contact: oas.SubscriptionContact{Id: sub.ContactID, Name: name}, Email: oas.Email(sub.Email),
		State: oas.SubscriptionState(sub.State), Source: oas.SubscriptionSource(sub.Source), ConsentAt: sub.ConsentAt,
		ConfirmedAt: sub.ConfirmedAt, ConfirmationSentAt: sub.ConfirmationSentAt, UnsubscribedAt: sub.UnsubscribedAt,
		CreatedAt: sub.CreatedAt, UpdatedAt: sub.UpdatedAt,
	}
	if sub.UnsubscribeReason != nil && sub.State == subUnsubscribed {
		r := oas.UnsubscribeReason(*sub.UnsubscribeReason)
		out.UnsubscribeReason = &r
	}
	return out
}

func subscriptionBodies(ctx context.Context, q *store.Queries, ws uuid.UUID, subs []store.ListSubscription) ([]oas.Subscription, error) {
	out := make([]oas.Subscription, len(subs))
	if len(subs) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, len(subs))
	for i, sub := range subs {
		ids[i] = sub.ContactID
	}
	rows, err := q.ListContactSummaries(ctx, store.ListContactSummariesParams{WorkspaceID: ws, Ids: ids})
	if err != nil {
		return nil, err
	}
	names := map[uuid.UUID]string{}
	for _, r := range rows {
		names[r.ID] = r.Name
	}
	subIDs := make([]uuid.UUID, len(subs))
	for i, sub := range subs {
		subIDs[i] = sub.ID
	}
	chosen, err := q.ListSubscriptionComponents(ctx, store.ListSubscriptionComponentsParams{WorkspaceID: ws, SubscriptionIds: subIDs})
	if err != nil {
		return nil, err
	}
	comps := map[uuid.UUID][]uuid.UUID{}
	for _, c := range chosen {
		comps[c.SubscriptionID] = append(comps[c.SubscriptionID], c.ComponentID)
	}
	for i, sub := range subs {
		out[i] = subscriptionBody(sub, names[sub.ContactID])
		if c, ok := comps[sub.ID]; ok {
			out[i].Components = &c
		} else if sub.ChoseComponents {
			out[i].Components = &[]uuid.UUID{}
		}
	}
	return out, nil
}

func oneSubscription(ctx context.Context, q *store.Queries, sub store.ListSubscription) (oas.Subscription, error) {
	out, err := subscriptionBodies(ctx, q, sub.WorkspaceID, []store.ListSubscription{sub})
	if err != nil {
		return oas.Subscription{}, err
	}
	return out[0], nil
}

// consentProof is what a consent event records about who acted and how.
type consentProof struct {
	source                          string
	ip, userAgent, pageURL, text    *string
	memberID, apiKeyID, broadcastID *uuid.UUID
	importID                        *uuid.UUID
	detail                          map[string]any
}

func principalSource(p principal) string {
	if p.scoped() {
		return string(oas.SubscriptionSourceApi)
	}
	return string(oas.SubscriptionSourceMember)
}

func principalProof(p principal, source string) consentProof {
	pr := consentProof{source: source, detail: map[string]any{}}
	if p.isKey() {
		pr.apiKeyID = &p.keyID
	} else {
		pr.memberID = &p.memberID
	}
	if p.via != "" {
		pr.detail["oauth_client"] = p.via
	}
	return pr
}

// requestProof records the person's own request: IP address and user agent.
func (s *Server) requestProof(r *http.Request, source string) consentProof {
	pr := consentProof{source: source, detail: map[string]any{}}
	if r == nil {
		return pr
	}
	ip := s.clientIP(r)
	pr.ip = &ip
	if ua := truncateRunes(strings.ToValidUTF8(r.UserAgent(), "\uFFFD"), 300); ua != "" {
		pr.userAgent = &ua
	}
	return pr
}

func optionalText(v string, n int) *string {
	v = truncateRunes(strings.TrimSpace(v), n)
	if v == "" {
		return nil
	}
	return &v
}

func (s *Server) consentEvent(ctx context.Context, q *store.Queries, sub store.ListSubscription, kind string, pr consentProof) error {
	detail := pr.detail
	if detail == nil {
		detail = map[string]any{}
	}
	clip := func(v *string, n int) *string {
		if v == nil {
			return nil
		}
		return optionalText(*v, n)
	}
	return q.AddConsentEvent(ctx, store.AddConsentEventParams{
		ID: newID(), WorkspaceID: sub.WorkspaceID, SubscriptionID: sub.ID, Kind: kind, Source: pr.source,
		Ip: clip(pr.ip, 64), UserAgent: clip(pr.userAgent, 300), PageUrl: clip(pr.pageURL, 2048), ConsentText: clip(pr.text, 2000),
		MemberID: pr.memberID, ApiKeyID: pr.apiKeyID, ImportID: pr.importID, BroadcastID: pr.broadcastID, Detail: mustJSON(detail), CreatedAt: s.now(),
	})
}

func emitSubscription(ctx context.Context, q *store.Queries, events *eventBatch, list store.MailingList, sub store.ListSubscription, previous *string) error {
	if events.quiet {
		return nil
	}
	body, err := oneSubscription(ctx, q, sub)
	if err != nil {
		return err
	}
	data := oas.SubscriptionEventData{Subscription: body, List: listRef(list)}
	typ := eventSubscriptionCreated
	if previous != nil {
		typ = eventSubscriptionUpdated
		st := oas.SubscriptionState(*previous)
		data.PreviousState = &st
	}
	inbox := list.InboxID
	events.add(typ, &inbox, nil, data)
	return nil
}

func (s *Server) newSubscription(ctx context.Context, q *store.Queries, events *eventBatch, list store.MailingList, contactID uuid.UUID, email, state, source string, consentAt *time.Time) (store.ListSubscription, error) {
	sub, err := q.CreateSubscription(ctx, store.CreateSubscriptionParams{
		ID: newID(), WorkspaceID: list.WorkspaceID, ListID: list.ID, ContactID: contactID, Email: email, State: state,
		Source: source, ConsentAt: consentAt, Now: s.now(),
	})
	if err != nil {
		return sub, err
	}
	if err := s.subscriptionConfirmed(ctx, q, events, list, sub, ""); err != nil {
		return sub, err
	}
	return sub, emitSubscription(ctx, q, events, list, sub, nil)
}

func (s *Server) saveSubscription(ctx context.Context, q *store.Queries, events *eventBatch, list store.MailingList, sub store.ListSubscription, previous string) (store.ListSubscription, error) {
	out, err := q.SaveSubscriptionState(ctx, store.SaveSubscriptionStateParams{
		WorkspaceID: sub.WorkspaceID, ID: sub.ID, State: sub.State, ConsentAt: sub.ConsentAt, ConfirmedAt: sub.ConfirmedAt,
		UnsubscribedAt: sub.UnsubscribedAt, UnsubscribeReason: sub.UnsubscribeReason, UnsubscribeBroadcastID: sub.UnsubscribeBroadcastID,
		Now: s.now(),
	})
	if err != nil {
		return out, err
	}
	if previous == out.State {
		return out, nil
	}
	if err := s.subscriptionConfirmed(ctx, q, events, list, out, previous); err != nil {
		return out, err
	}
	return out, emitSubscription(ctx, q, events, list, out, &previous)
}

// unsubscribe ends a subscription at once; kind is `unsubscribed` or `complained`.
func (s *Server) unsubscribe(ctx context.Context, q *store.Queries, events *eventBatch, list store.MailingList, sub store.ListSubscription, reason, kind string, pr consentProof) (store.ListSubscription, error) {
	if sub.State == subUnsubscribed {
		return sub, nil
	}
	prev := sub.State
	sub.State, sub.UnsubscribedAt, sub.UnsubscribeReason, sub.UnsubscribeBroadcastID = subUnsubscribed, new(s.now()), &reason, pr.broadcastID
	out, err := s.saveSubscription(ctx, q, events, list, sub, prev)
	if err != nil {
		return out, err
	}
	if pr.broadcastID != nil {
		if err := s.unsubscribedThrough(ctx, q, sub.WorkspaceID, *pr.broadcastID, sub.ContactID); err != nil {
			return out, err
		}
	}
	return out, s.consentEvent(ctx, q, out, kind, pr)
}

// subscribeNow makes a subscription `subscribed` without a confirmation mail, recording the
// given proof, and sends the list's welcome mail.
func (s *Server) subscribeNow(ctx context.Context, q *store.Queries, events *eventBatch, list store.MailingList, sub store.ListSubscription, consentAt time.Time, pr consentProof) (store.ListSubscription, error) {
	out, err := s.markSubscribed(ctx, q, events, list, sub, consentAt, pr)
	if err != nil {
		return out, err
	}
	s.queueWelcome(events, list, out)
	return out, nil
}

// markSubscribed is subscribeNow without the welcome mail.
func (s *Server) markSubscribed(ctx context.Context, q *store.Queries, events *eventBatch, list store.MailingList, sub store.ListSubscription, consentAt time.Time, pr consentProof) (store.ListSubscription, error) {
	prev := sub.State
	sub.State, sub.ConsentAt, sub.UnsubscribedAt = subSubscribed, &consentAt, nil
	out, err := s.saveSubscription(ctx, q, events, list, sub, prev)
	if err != nil {
		return out, err
	}
	return out, s.consentEvent(ctx, q, out, consentSubscribed, pr)
}

// confirmListSubscription is the person confirming by mail: the subscription becomes `subscribed`,
// an objection they made before is lifted and the address joins their contact when nobody has it
// and that contact is nobody's identity. Clicking the link proves the address receives mail, not
// who it belongs to, so an identified contact earns an address only through confirmTypedEmail.
func (s *Server) confirmListSubscription(ctx context.Context, q *store.Queries, events *eventBatch, list store.MailingList, sub store.ListSubscription, pr consentProof) (store.ListSubscription, error) {
	now := s.now()
	prev := sub.State
	sub.State, sub.ConsentAt, sub.ConfirmedAt, sub.UnsubscribedAt = subSubscribed, &now, &now, nil
	out, err := s.saveSubscription(ctx, q, events, list, sub, prev)
	if err != nil {
		return out, err
	}
	if err := s.consentEvent(ctx, q, out, consentConfirmed, pr); err != nil {
		return out, err
	}
	if list.Kind != listKindStatus {
		if err := s.forgetOptOut(ctx, q, sub.WorkspaceID, sub.Email); err != nil {
			return out, err
		}
	}
	if _, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: sub.WorkspaceID, Email: sub.Email}); store.IsNotFound(err) {
		own, err := q.ContactUnidentified(ctx, store.ContactUnidentifiedParams{WorkspaceID: sub.WorkspaceID, ID: sub.ContactID})
		if err != nil {
			return out, err
		}
		if own {
			n, err := q.CountContactEmails(ctx, store.CountContactEmailsParams{WorkspaceID: sub.WorkspaceID, ContactID: sub.ContactID})
			if err != nil {
				return out, err
			}
			if err := q.AddContactEmail(ctx, store.AddContactEmailParams{WorkspaceID: sub.WorkspaceID, ContactID: sub.ContactID, Email: sub.Email, Position: int32(n)}); err != nil {
				return out, err
			}
		}
	} else if err != nil {
		return out, err
	}
	s.queueWelcome(events, list, out)
	return out, nil
}

// objected says whether the person unsubscribed from the list and has not confirmed since: an
// `unsubscribed` subscription, or a `pending` one they asked for again.
func objected(sub store.ListSubscription) bool {
	return sub.State == subUnsubscribed || (sub.State == subPending && sub.UnsubscribeReason != nil)
}

// requestAgain moves an `unsubscribed` subscription to `pending`: only a confirmation by the
// person can make it `subscribed` again. The unsubscribe reason stays until then.
func (s *Server) requestAgain(ctx context.Context, q *store.Queries, events *eventBatch, list store.MailingList, sub store.ListSubscription) (store.ListSubscription, error) {
	if sub.State != subUnsubscribed {
		return sub, nil
	}
	sub.State, sub.UnsubscribedAt, sub.ConsentAt = subPending, nil, nil
	return s.saveSubscription(ctx, q, events, list, sub, subUnsubscribed)
}

// confirmationAllowed counts a confirmation mail against the per-address limits and, for forms,
// the channel's daily limit, in transaction q: whether a mail is due depends on what it read.
func (s *Server) confirmationAllowed(ctx context.Context, q *store.Queries, ws, listID uuid.UUID, email string, channel *uuid.UUID) bool {
	checks := []rateCheck{
		{"list-confirm:address:" + ws.String() + ":" + email, limitConfirmPerAddress},
		{"list-confirm:address-list:" + listID.String() + ":" + email, limitConfirmPerAddressList},
	}
	if channel != nil {
		checks = append(checks, rateCheck{"list-confirm:channel:" + channel.String(), limitConfirmPerChannel})
	}
	return s.rateLimitTx(ctx, q, checks...) == nil
}

func (s *Server) queueConfirmation(ctx context.Context, q *store.Queries, events *eventBatch, sub store.ListSubscription) error {
	return s.queueConfirmationAt(ctx, q, events, sub, time.Time{})
}

// queueConfirmationAt queues the confirmation mail to go out at the given time; zero is now.
func (s *Server) queueConfirmationAt(ctx context.Context, q *store.Queries, events *eventBatch, sub store.ListSubscription, at time.Time) error {
	if err := q.MarkConfirmationSent(ctx, store.MarkConfirmationSentParams{WorkspaceID: sub.WorkspaceID, ID: sub.ID, Now: s.now()}); err != nil {
		return err
	}
	opts := listMailOpts()
	opts.ScheduledAt = at
	events.job(ListMailArgs{WorkspaceID: sub.WorkspaceID, SubscriptionID: sub.ID, Mail: listMailConfirm}, opts)
	return nil
}

func (s *Server) queueWelcome(events *eventBatch, list store.MailingList, sub store.ListSubscription) {
	if list.Welcome {
		events.job(ListMailArgs{WorkspaceID: sub.WorkspaceID, SubscriptionID: sub.ID, Mail: listMailWelcome}, listMailOpts())
	}
}

func visibleSubscription(ctx context.Context, q *store.Queries, p principal, listID, id uuid.UUID, lock bool) (store.MailingList, store.ListSubscription, error) {
	list, err := visibleList(ctx, q, p, listID, false)
	if err != nil {
		return list, store.ListSubscription{}, err
	}
	get := q.GetSubscription
	if lock {
		get = func(ctx context.Context, arg store.GetSubscriptionParams) (store.ListSubscription, error) {
			return q.LockSubscription(ctx, store.LockSubscriptionParams(arg))
		}
	}
	sub, err := get(ctx, store.GetSubscriptionParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) || (err == nil && sub.ListID != list.ID) {
		return list, sub, errSubscriptionGone
	}
	return list, sub, err
}

func (s *Server) ListSubscriptions(ctx context.Context, req oas.ListSubscriptionsRequestObject) (oas.ListSubscriptionsResponseObject, error) {
	p := principalFrom(ctx)
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	var state, q *string
	if req.Params.State != nil {
		if !req.Params.State.Valid() {
			return nil, errValidation("state must be pending, subscribed or unsubscribed")
		}
		state = new(string(*req.Params.State))
	}
	if req.Params.Q != nil {
		if v := strings.TrimSpace(*req.Params.Q); v != "" {
			q = &v
		}
	}
	list, err := visibleList(ctx, s.st.Queries, p, req.ListId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListSubscriptions(ctx, store.ListSubscriptionsParams{
		WorkspaceID: p.workspaceID, ListID: list.ID, State: state, Q: q, CursorAt: at, CursorID: id, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		next = &c
	}
	items, err := subscriptionBodies(ctx, s.st.Queries, p.workspaceID, rows)
	if err != nil {
		return nil, err
	}
	return oas.ListSubscriptions200JSONResponse{Items: items, NextCursor: next}, nil
}

// subscriber finds or creates the contact a host backend, member or import names, and the
// address to subscribe; created reports a new contact.
func (s *Server) subscriber(ctx context.Context, q *store.Queries, events *eventBatch, p principal, list store.MailingList, b *oas.SubscriptionCreate) (uuid.UUID, string, bool, error) {
	ws := p.workspaceID
	var email string
	if b.Email != nil {
		var err error
		if email, err = subscriptionAddress(*b.Email); err != nil {
			return uuid.Nil(), "", false, err
		}
	}
	var externalID string
	if b.ExternalId != nil {
		externalID = strings.TrimSpace(*b.ExternalId)
		if externalID == "" || len([]rune(externalID)) > 200 {
			return uuid.Nil(), "", false, errValidation("external_id must be 1 to 200 characters")
		}
	}
	name := ""
	if b.Name != nil {
		var err error
		if name, err = trimmed(*b.Name, 0, 200, "name"); err != nil {
			return uuid.Nil(), "", false, err
		}
	}
	if b.Locale != nil && !languagePattern.MatchString(string(*b.Locale)) {
		return uuid.Nil(), "", false, errValidation("locale must be a BCP 47 language tag")
	}
	var attrs []byte
	if b.Attributes != nil {
		attrs = mustJSON(*b.Attributes)
		if len(attrs) > maxAttributesBytes {
			return uuid.Nil(), "", false, errValidation("attributes must be at most 16 KiB")
		}
	}
	var contactID *uuid.UUID
	switch {
	case b.ContactId != nil:
		if found, err := q.ContactExists(ctx, store.ContactExistsParams{WorkspaceID: ws, ID: *b.ContactId}); err != nil {
			return uuid.Nil(), "", false, err
		} else if !found {
			return uuid.Nil(), "", false, errContactGone
		}
		if err := visibleContact(ctx, q, p, *b.ContactId); err != nil {
			return uuid.Nil(), "", false, err
		}
		contactID = b.ContactId
	case externalID != "":
		id, err := q.GetContactIDByExternalID(ctx, store.GetContactIDByExternalIDParams{WorkspaceID: ws, InboxID: list.InboxID, ExternalID: externalID})
		if err == nil {
			contactID = &id
		} else if !store.IsNotFound(err) {
			return uuid.Nil(), "", false, err
		}
	}
	if contactID == nil && email == "" {
		return uuid.Nil(), "", false, errValidation("send email, contact_id or external_id")
	}
	if contactID == nil {
		id, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: email})
		if store.IsNotFound(err) {
			id, err = q.SubscriptionContactByEmail(ctx, store.SubscriptionContactByEmailParams{WorkspaceID: ws, Email: email})
		}
		if err == nil {
			contactID = &id
		} else if !store.IsNotFound(err) {
			return uuid.Nil(), "", false, err
		}
	}
	if contactID == nil {
		if attrs == nil {
			attrs = []byte("{}")
		}
		c, err := q.CreateContact(ctx, store.CreateContactParams{ID: newID(), WorkspaceID: ws, Name: name, Attributes: attrs, CreatedBy: p.creator(), Now: s.now()})
		if err != nil {
			return uuid.Nil(), "", false, err
		}
		if b.Locale != nil {
			if _, err := q.SetContactIdentity(ctx, store.SetContactIdentityParams{
				WorkspaceID: ws, ID: c.ID, Name: name, Locale: new(string(*b.Locale)), Attributes: attrs, Now: s.now(),
			}); err != nil {
				return uuid.Nil(), "", false, err
			}
		}
		if err := q.AddContactEmail(ctx, store.AddContactEmailParams{WorkspaceID: ws, ContactID: c.ID, Email: email, Position: 0}); err != nil {
			return uuid.Nil(), "", false, err
		}
		if externalID != "" {
			if err := s.addExternalID(ctx, q, events, ws, list.InboxID, c.ID, externalID, true); err != nil {
				return uuid.Nil(), "", false, err
			}
		}
		if err := s.attributesWritten(ctx, q, events, ws, c.ID, []uuid.UUID{list.InboxID}, nil, c.Attributes); err != nil {
			return uuid.Nil(), "", false, err
		}
		if err := q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: ws, ID: c.ID}); err != nil {
			return uuid.Nil(), "", false, err
		}
		return c.ID, email, true, nil
	}
	cur, err := q.LockContact(ctx, store.LockContactParams{WorkspaceID: ws, ID: *contactID})
	if err != nil {
		return uuid.Nil(), "", false, err
	}
	if (name != "" && cur.Name == "") || (b.Locale != nil && cur.Locale == nil) {
		n, loc := cur.Name, cur.Locale
		if n == "" {
			n = name
		}
		if loc == nil && b.Locale != nil {
			loc = new(string(*b.Locale))
		}
		if _, err := q.SetContactIdentity(ctx, store.SetContactIdentityParams{WorkspaceID: ws, ID: cur.ID, Name: n, Locale: loc, Attributes: cur.Attributes, Now: s.now()}); err != nil {
			return uuid.Nil(), "", false, err
		}
	}
	emails, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: ws, ContactIds: []uuid.UUID{cur.ID}})
	if err != nil {
		return uuid.Nil(), "", false, err
	}
	if email == "" {
		if len(emails) == 0 {
			return uuid.Nil(), "", false, problem(http.StatusBadRequest, "invalid_email", "the contact has no e-mail address; send email")
		}
		return cur.ID, emails[0].Email, false, nil
	}
	if _, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: email}); store.IsNotFound(err) {
		if err := q.AddContactEmail(ctx, store.AddContactEmailParams{WorkspaceID: ws, ContactID: cur.ID, Email: email, Position: int32(len(emails))}); err != nil {
			return uuid.Nil(), "", false, err
		}
	} else if err != nil {
		return uuid.Nil(), "", false, err
	}
	if externalID != "" {
		inInbox, _, err := externalIDPlaces(ctx, q, ws, list.InboxID, cur.ID)
		if err != nil {
			return uuid.Nil(), "", false, err
		}
		if !inInbox {
			if err := s.addExternalID(ctx, q, events, ws, list.InboxID, cur.ID, externalID, true); err != nil {
				return uuid.Nil(), "", false, err
			}
		}
	}
	return cur.ID, email, false, nil
}

func (s *Server) CreateSubscription(ctx context.Context, req oas.CreateSubscriptionRequestObject) (oas.CreateSubscriptionResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	c := b.Consent
	if c.ObtainedAt != nil && c.ObtainedAt.After(s.now().Add(time.Minute)) {
		return nil, errValidation("consent.obtained_at must not be in the future")
	}
	var (
		out     store.ListSubscription
		created bool
		status  bool
	)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		list, err := visibleList(ctx, q, p, req.ListId, false)
		if err != nil {
			return err
		}
		if err := requireManagerOrKey(p); err != nil {
			return err
		}
		if b.Attributes != nil {
			if err := checkContactFields(ctx, q, p.workspaceID, *b.Attributes); err != nil {
				return err
			}
		}
		contactID, email, _, err := s.subscriber(ctx, q, events, p, list, b)
		if err != nil {
			return err
		}
		if suppressed, err := q.IsEmailSuppressedAnyScope(ctx, store.IsEmailSuppressedAnyScopeParams{WorkspaceID: p.workspaceID, Email: email}); err != nil {
			return err
		} else if suppressed {
			return errSubSuppressed
		}
		status = list.Kind == listKindStatus
		confirmed := c.Confirmed && !status
		if !status {
			if opted, err := s.optedOut(ctx, q, p.workspaceID, email); err != nil {
				return err
			} else if opted {
				return errSubOptedOut
			}
		}
		pr := principalProof(p, principalSource(p))
		pr.ip, pr.pageURL, pr.text = c.Ip, c.PageUrl, c.Text
		if c.Reference != nil {
			pr.detail["reference"] = truncateRunes(*c.Reference, 500)
		}
		consentAt := s.now()
		if c.ObtainedAt != nil {
			pr.detail["obtained_at"] = c.ObtainedAt.UTC().Format(time.RFC3339)
			consentAt = *c.ObtainedAt
		}
		cur, err := q.LockSubscriptionByEmail(ctx, store.LockSubscriptionByEmailParams{WorkspaceID: p.workspaceID, ListID: list.ID, Email: email})
		if err != nil && !store.IsNotFound(err) {
			return err
		}
		if err == nil {
			switch {
			case cur.State == subSubscribed:
				out = cur
			case cur.State == subUnsubscribed || (objected(cur) && confirmed):
				return errSubUnsubscribed
			case confirmed:
				out, err = s.subscribeNow(ctx, q, events, list, cur, consentAt, pr)
			default:
				out = cur
			}
			return err
		}
		created = true
		if confirmed {
			if out, err = s.newSubscription(ctx, q, events, list, contactID, email, subSubscribed, principalSource(p), &consentAt); err != nil {
				return err
			}
			if err := s.consentEvent(ctx, q, out, consentSubscribed, pr); err != nil {
				return err
			}
			s.queueWelcome(events, list, out)
			return nil
		}
		if !s.confirmationAllowed(ctx, q, p.workspaceID, list.ID, email, nil) {
			return errConfirmationLimit
		}
		if out, err = s.newSubscription(ctx, q, events, list, contactID, email, subPending, principalSource(p), nil); err != nil {
			return err
		}
		if err := s.consentEvent(ctx, q, out, consentRequested, pr); err != nil {
			return err
		}
		return s.queueConfirmation(ctx, q, events, out)
	})
	if err != nil {
		return nil, err
	}
	body, err := oneSubscription(ctx, s.st.Queries, out)
	if err != nil {
		return nil, err
	}
	if created && status {
		return oas.CreateSubscription202JSONResponse(body), nil
	}
	if created {
		return oas.CreateSubscription201JSONResponse(body), nil
	}
	return oas.CreateSubscription200JSONResponse(body), nil
}

func (s *Server) GetSubscription(ctx context.Context, req oas.GetSubscriptionRequestObject) (oas.GetSubscriptionResponseObject, error) {
	p := principalFrom(ctx)
	_, sub, err := visibleSubscription(ctx, s.st.Queries, p, req.ListId, req.SubscriptionId, false)
	if err != nil {
		return nil, err
	}
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	body, err := oneSubscription(ctx, s.st.Queries, sub)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListConsentEvents(ctx, store.ListConsentEventsParams{WorkspaceID: p.workspaceID, SubscriptionID: sub.ID})
	if err != nil {
		return nil, err
	}
	out := oas.GetSubscription200JSONResponse{
		Id: body.Id, ListId: body.ListId, Contact: body.Contact, Email: body.Email, State: body.State, Source: body.Source,
		ConsentAt: body.ConsentAt, ConfirmedAt: body.ConfirmedAt, ConfirmationSentAt: body.ConfirmationSentAt,
		UnsubscribedAt: body.UnsubscribedAt, UnsubscribeReason: body.UnsubscribeReason, CreatedAt: body.CreatedAt, UpdatedAt: body.UpdatedAt,
		Components: body.Components,
		Events:     make([]oas.ConsentEvent, len(rows)),
	}
	for i, e := range rows {
		out.Events[i] = consentEventBody(e)
	}
	return out, nil
}

func consentEventBody(e store.ConsentEvent) oas.ConsentEvent {
	out := oas.ConsentEvent{
		Id: e.ID, SubscriptionId: e.SubscriptionID, Kind: oas.ConsentEventKind(e.Kind), Source: e.Source, Ip: e.Ip,
		UserAgent: e.UserAgent, PageUrl: e.PageUrl, ConsentText: e.ConsentText, MemberId: e.MemberID, ApiKeyId: e.ApiKeyID,
		BroadcastId: e.BroadcastID, ImportId: e.ImportID, CreatedAt: e.CreatedAt, Detail: map[string]any{},
	}
	_ = json.Unmarshal(e.Detail, &out.Detail)
	return out
}

func (s *Server) UpdateSubscription(ctx context.Context, req oas.UpdateSubscriptionRequestObject) (oas.UpdateSubscriptionResponseObject, error) {
	p := principalFrom(ctx)
	if !req.Body.State.Valid() {
		return nil, errValidation("state must be pending, subscribed or unsubscribed")
	}
	var out store.ListSubscription
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		var err error
		out, err = s.setSubscriptionState(ctx, q, events, p, req.ListId, req.SubscriptionId, req.Body.State)
		return err
	})
	if err != nil {
		return nil, err
	}
	body, err := oneSubscription(ctx, s.st.Queries, out)
	if err != nil {
		return nil, err
	}
	return oas.UpdateSubscription200JSONResponse(body), nil
}

func (s *Server) setSubscriptionState(ctx context.Context, q *store.Queries, events *eventBatch, p principal, listID, id uuid.UUID, state oas.SubscriptionState) (store.ListSubscription, error) {
	list, sub, err := visibleSubscription(ctx, q, p, listID, id, true)
	if err != nil {
		return sub, err
	}
	if err := requireManagerOrKey(p); err != nil {
		return sub, err
	}
	if string(state) != subUnsubscribed {
		if string(state) == sub.State && sub.State == subPending {
			return sub, nil
		}
		return sub, errConfirmationRequired
	}
	reason := string(oas.UnsubscribeReasonMember)
	if p.scoped() {
		reason = string(oas.UnsubscribeReasonApi)
	}
	return s.unsubscribe(ctx, q, events, list, sub, reason, consentUnsubscribed, principalProof(p, reason))
}

func (s *Server) DeleteSubscription(ctx context.Context, req oas.DeleteSubscriptionRequestObject) (oas.DeleteSubscriptionResponseObject, error) {
	p := principalFrom(ctx)
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		return s.deleteSubscription(ctx, q, p, req.ListId, req.SubscriptionId)
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteSubscription204Response{}, nil
}

func (s *Server) deleteSubscription(ctx context.Context, q *store.Queries, p principal, listID, id uuid.UUID) error {
	_, sub, err := visibleSubscription(ctx, q, p, listID, id, true)
	if err != nil {
		return err
	}
	if err := requireManagerOrKey(p); err != nil {
		return err
	}
	list, err := q.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: p.workspaceID, ID: sub.ListID})
	if err != nil {
		return err
	}
	if objected(sub) && list.Kind != listKindStatus {
		if err := s.rememberOptOuts(ctx, q, p.workspaceID, []string{sub.Email}, optOutErasure); err != nil {
			return err
		}
	}
	_, err = q.DeleteSubscription(ctx, store.DeleteSubscriptionParams{WorkspaceID: p.workspaceID, ID: sub.ID})
	return err
}

func (s *Server) ResendSubscriptionConfirmation(ctx context.Context, req oas.ResendSubscriptionConfirmationRequestObject) (oas.ResendSubscriptionConfirmationResponseObject, error) {
	p := principalFrom(ctx)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		list, sub, err := visibleSubscription(ctx, q, p, req.ListId, req.SubscriptionId, true)
		if err != nil {
			return err
		}
		if err := requireManagerOrKey(p); err != nil {
			return err
		}
		if sub.State != subPending {
			return errNotPending
		}
		if list.ChannelID == nil {
			return errEmailChannelRequired
		}
		if err := listChannel(ctx, q, p.workspaceID, list.InboxID, *list.ChannelID); err != nil {
			return err
		}
		if sub.ConfirmationsSent >= maxConfirmations || s.rateLimitTx(ctx, q, rateCheck{"list-confirm:subscription:" + sub.ID.String(), limitConfirmPerSub}) != nil {
			return errConfirmationLimit
		}
		return s.queueConfirmation(ctx, q, events, sub)
	})
	if err != nil {
		return nil, err
	}
	return oas.ResendSubscriptionConfirmation202Response{}, nil
}

func (s *Server) ListContactSubscriptions(ctx context.Context, req oas.ListContactSubscriptionsRequestObject) (oas.ListContactSubscriptionsResponseObject, error) {
	p := principalFrom(ctx)
	if err := s.contactForConsent(ctx, p, req.ContactId); err != nil {
		return nil, err
	}
	subs, err := s.st.ListContactSubscriptions(ctx, store.ListContactSubscriptionsParams{WorkspaceID: p.workspaceID, ContactID: req.ContactId})
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(subs))
	for _, sub := range subs {
		ids = append(ids, sub.ListID)
	}
	lists, err := s.st.ListMailingListsByIDs(ctx, store.ListMailingListsByIDsParams{WorkspaceID: p.workspaceID, Ids: ids})
	if err != nil {
		return nil, err
	}
	inboxes, err := agentInboxes(ctx, s.st.Queries, p)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]store.MailingList{}
	owned := map[uuid.UUID]bool{}
	for _, l := range lists {
		if inboxes != nil && !inboxes[l.InboxID] {
			continue
		}
		ok, seen := owned[l.InboxID]
		if !seen {
			if ok, err = inboxObjectsVisible(ctx, s.st.Queries, p, l.InboxID); err != nil {
				return nil, err
			}
			owned[l.InboxID] = ok
		}
		if ok {
			byID[l.ID] = l
		}
	}
	var kept []store.ListSubscription
	for _, sub := range subs {
		if _, ok := byID[sub.ListID]; ok {
			kept = append(kept, sub)
		}
	}
	bodies, err := subscriptionBodies(ctx, s.st.Queries, p.workspaceID, kept)
	if err != nil {
		return nil, err
	}
	out := oas.ListContactSubscriptions200JSONResponse{Items: make([]oas.ContactSubscription, len(bodies))}
	for i, b := range bodies {
		out.Items[i] = oas.ContactSubscription{
			Id: b.Id, ListId: b.ListId, List: listRef(byID[b.ListId]), Contact: b.Contact, Email: b.Email, State: b.State,
			Source: b.Source, ConsentAt: b.ConsentAt, ConfirmedAt: b.ConfirmedAt, ConfirmationSentAt: b.ConfirmationSentAt,
			UnsubscribedAt: b.UnsubscribedAt, UnsubscribeReason: b.UnsubscribeReason, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
		}
	}
	return out, nil
}

func (s *Server) contactForConsent(ctx context.Context, p principal, id uuid.UUID) error {
	found, err := s.st.ContactExists(ctx, store.ContactExistsParams{WorkspaceID: p.workspaceID, ID: id})
	if err != nil {
		return err
	}
	if !found {
		return errContactGone
	}
	return visibleContact(ctx, s.st.Queries, p, id)
}

func (s *Server) ExportListConsent(ctx context.Context, req oas.ExportListConsentRequestObject) (oas.ExportListConsentResponseObject, error) {
	p := principalFrom(ctx)
	list, err := visibleList(ctx, s.st.Queries, p, req.ListId, false)
	if err != nil {
		return nil, err
	}
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	if req.Params.Format != nil && !req.Params.Format.Valid() {
		return nil, errValidation("format must be csv, json or iys")
	}
	iys := req.Params.Format != nil && *req.Params.Format == oas.ExportListConsentParamsFormatIys
	if iys && list.Kind == listKindStatus {
		return nil, errListKind
	}
	format := "iys"
	if !iys {
		if format, err = consentFormat((*string)(req.Params.Format)); err != nil {
			return nil, err
		}
	}
	if err := s.audit(ctx, s.st.Queries, p, oas.AuditListConsentExported, oas.AuditTargetList, list.ID, auditDetails{"name": list.Name, "format": format, "started": true}); err != nil {
		return nil, err
	}
	if iys {
		return iysStream{
			s: s, ctx: ctx, name: "iys-" + list.ID.String(),
			arg: store.ListIYSRecordsParams{WorkspaceID: p.workspaceID, ListID: list.ID, Since: req.Params.Since},
		}, nil
	}
	return consentStream{
		s: s, ctx: ctx, format: format, name: "consent-" + list.ID.String(),
		arg: store.ListConsentRecordsParams{WorkspaceID: p.workspaceID, ListID: &list.ID, Since: req.Params.Since, AllInboxes: true},
	}, nil
}

func (s *Server) ExportContactConsent(ctx context.Context, req oas.ExportContactConsentRequestObject) (oas.ExportContactConsentResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	if err := s.contactForConsent(ctx, p, req.ContactId); err != nil {
		return nil, err
	}
	format, err := consentFormat((*string)(req.Params.Format))
	if err != nil {
		return nil, err
	}
	if err := s.audit(ctx, s.st.Queries, p, oas.AuditContactConsentExported, oas.AuditTargetContact, req.ContactId, auditDetails{"format": format, "started": true}); err != nil {
		return nil, err
	}
	return consentStream{
		s: s, ctx: ctx, format: format, name: "consent-contact-" + req.ContactId.String(),
		arg: store.ListConsentRecordsParams{
			WorkspaceID: p.workspaceID, ContactID: &req.ContactId, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
		},
	}, nil
}

func consentFormat(v *string) (string, error) {
	if v == nil || *v == "csv" {
		return "csv", nil
	}
	if *v == "json" {
		return "json", nil
	}
	return "", errValidation("format must be csv or json")
}

// complainedAbout ends every subscription of an address that reported mail from the workspace as
// spam, and remembers the objection.
func (s *Server) complainedAbout(ctx context.Context, q *store.Queries, events *eventBatch, ws uuid.UUID, email string) error {
	subs, err := q.LockEmailSubscriptions(ctx, store.LockEmailSubscriptionsParams{WorkspaceID: ws, Email: email})
	if err != nil {
		return err
	}
	lists := map[uuid.UUID]store.MailingList{}
	reason := string(oas.UnsubscribeReasonComplaint)
	for _, sub := range subs {
		if sub.State == subUnsubscribed {
			continue
		}
		l, ok := lists[sub.ListID]
		if !ok {
			if l, err = q.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: ws, ID: sub.ListID}); err != nil {
				return err
			}
			lists[l.ID] = l
		}
		if _, err := s.unsubscribe(ctx, q, events, l, sub, reason, consentComplained, consentProof{source: reason}); err != nil {
			return err
		}
	}
	return s.rememberOptOuts(ctx, q, ws, []string{email}, optOutComplaint)
}

// ExpireSubscriptions deletes pending subscriptions nobody confirmed within 30 days and
// unsubscribed ones (with their consent events) after 3 years, keeping the objection as an opt-out.
func (s *Server) ExpireSubscriptions(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range ids {
		for {
			rows, err := s.st.ExpirePendingSubscriptions(ctx, store.ExpirePendingSubscriptionsParams{
				WorkspaceID: ws, Before: s.now().Add(-pendingKept), Now: s.now(), Lim: retentionBatch,
			})
			if err != nil {
				return err
			}
			var unconfirmed []uuid.UUID
			for _, r := range rows {
				if r.UnconfirmedStatus {
					unconfirmed = append(unconfirmed, r.ContactID)
				}
			}
			if len(unconfirmed) > 0 {
				if _, err := s.st.DeleteUnconfirmedContacts(ctx, store.DeleteUnconfirmedContactsParams{WorkspaceID: ws, Ids: unconfirmed}); err != nil {
					return err
				}
			}
			if len(rows) < retentionBatch {
				break
			}
		}
		for {
			var n int
			err := s.st.InTx(ctx, func(q *store.Queries) error {
				rows, err := q.ListExpiredUnsubscribed(ctx, store.ListExpiredUnsubscribedParams{WorkspaceID: ws, Before: s.now().Add(-unsubscribedKept), Lim: retentionBatch})
				if err != nil || len(rows) == 0 {
					return err
				}
				n = len(rows)
				ids := make([]uuid.UUID, n)
				var emails []string
				for i, r := range rows {
					ids[i] = r.ID
					if r.Kind != listKindStatus {
						emails = append(emails, r.Email)
					}
				}
				if err := s.rememberOptOuts(ctx, q, ws, emails, optOutErasure); err != nil {
					return err
				}
				_, err = q.DeleteSubscriptions(ctx, store.DeleteSubscriptionsParams{WorkspaceID: ws, Ids: ids})
				return err
			})
			if errors.Is(err, errOptOutKey) {
				s.log.ErrorContext(ctx, "unsubscribed subscriptions kept: their opt-outs cannot be recorded", slog.String("workspace_id", ws.String()), slog.Any("error", err))
				break
			}
			if err != nil {
				return err
			}
			if n < retentionBatch {
				break
			}
		}
	}
	return nil
}

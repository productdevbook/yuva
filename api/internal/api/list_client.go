package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	formTokenMinAge = 2 * time.Second
	formTokenMaxAge = 2 * time.Hour
	maxFormLists    = 10
)

var (
	errFormToken        = problem(http.StatusBadRequest, "form_token", "the form is too old or was sent too fast; load it again")
	errListNotFound     = problem(http.StatusNotFound, "list_not_found", "not a public list of this channel's inbox")
	errIdentityRequired = problem(http.StatusForbidden, "identity_required", "only signed-in users subscribe here; visitors use the subscribe form")
	errNoEmail          = problem(http.StatusConflict, "no_email", "the contact has no e-mail address")

	limitSubscribePerIP      = limit{10, time.Hour}
	limitSubscribePerChannel = limit{600, time.Hour}
)

func clientList(l store.MailingList) oas.ClientMailingList {
	return oas.ClientMailingList{Id: l.ID, Name: l.Name, Description: l.Description, DoubleOptIn: l.DoubleOptIn}
}

func (s *Server) ListClientMailingLists(ctx context.Context, req oas.ListClientMailingListsRequestObject) (oas.ListClientMailingListsResponseObject, error) {
	if err := s.rateLimit(ctx, rateCheck{"subscribe-lists:ip:" + rateIP(s.clientIP(requestFrom(ctx))), limitChannelPerIP}); err != nil {
		return nil, err
	}
	ch, err := s.docsChannel(ctx, req.ChannelKey, nil)
	if err != nil {
		return nil, err
	}
	lists, err := s.st.ListPublicMailingLists(ctx, store.ListPublicMailingListsParams{WorkspaceID: ch.WorkspaceID, InboxID: ch.InboxID})
	if err != nil {
		return nil, err
	}
	out := oas.ListClientMailingLists200JSONResponse{
		Body: oas.ClientMailingLists{
			Lists:     make([]oas.ClientMailingList, len(lists)),
			FormToken: s.sealToken(formTokenContext, []uuid.UUID{ch.ChannelID}, new(s.now())),
		},
		Headers: oas.ListClientMailingLists200ResponseHeaders{CacheControl: new("no-store")},
	}
	for i, l := range lists {
		out.Body.Lists[i] = clientList(l)
	}
	return out, nil
}

// formListsFor resolves the lists a form names: public, unarchived lists of the inbox.
func formListsFor(public []store.MailingList, ids []uuid.UUID) ([]store.MailingList, error) {
	if len(ids) == 0 || len(ids) > maxFormLists {
		return nil, errValidation("list_ids must name 1 to 10 lists")
	}
	var out []store.MailingList
	for _, id := range ids {
		i := slices.IndexFunc(public, func(l store.MailingList) bool { return l.ID == id })
		if i < 0 {
			return nil, errListNotFound
		}
		if !slices.ContainsFunc(out, func(l store.MailingList) bool { return l.ID == id }) {
			out = append(out, public[i])
		}
	}
	return out, nil
}

// CreateClientSubscription answers every address the same way; what happens behind the answer
// depends on what the workspace knows about it.
func (s *Server) CreateClientSubscription(ctx context.Context, req oas.CreateClientSubscriptionRequestObject) (oas.CreateClientSubscriptionResponseObject, error) {
	r := requestFrom(ctx)
	if err := s.rateLimit(ctx, rateCheck{"subscribe:ip:" + rateIP(s.clientIP(r)), limitSubscribePerIP}); err != nil {
		return nil, err
	}
	ch, err := s.docsChannel(ctx, req.ChannelKey, nil)
	if err != nil {
		return nil, err
	}
	if err := s.rateLimit(ctx, rateCheck{"subscribe:channel:" + ch.ChannelID.String(), limitSubscribePerChannel}); err != nil {
		return nil, err
	}
	b := req.Body
	page, err := channelPage(b.PageUrl, ch.Kind, ch.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	ids, issued, ok := s.openToken(b.FormToken, formTokenContext, 1, true)
	if age := s.now().Sub(issued); !ok || ids[0] != ch.ChannelID || age < formTokenMinAge || age > formTokenMaxAge {
		return nil, errFormToken
	}
	public, err := s.st.ListPublicMailingLists(ctx, store.ListPublicMailingListsParams{WorkspaceID: ch.WorkspaceID, InboxID: ch.InboxID})
	if err != nil {
		return nil, err
	}
	lists, err := formListsFor(public, b.ListIds)
	if err != nil {
		return nil, err
	}
	addr, err := subscriptionAddress(b.Email)
	if err != nil {
		return nil, err
	}
	text, err := trimmed(b.ConsentText, 1, 2000, "consent_text")
	if err != nil {
		return nil, err
	}
	name := ""
	if b.Name != nil {
		if name, err = trimmed(*b.Name, 0, 200, "name"); err != nil {
			return nil, err
		}
	}
	var locale *string
	if b.Locale != nil {
		if !languagePattern.MatchString(string(*b.Locale)) {
			return nil, errValidation("locale must be a BCP 47 language tag")
		}
		locale = new(string(*b.Locale))
	}
	answer := oas.CreateClientSubscription202JSONResponse{}
	for _, l := range lists {
		answer.Confirmation = answer.Confirmation || l.DoubleOptIn
	}
	if b.Website != nil && strings.TrimSpace(*b.Website) != "" {
		return answer, nil
	}
	pr := s.requestProof(r, string(oas.SubscriptionSourceForm))
	pr.pageURL, pr.text = &page, &text
	err = s.inTx(ctx, ch.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		return s.subscribeFromForm(ctx, q, events, ch.ChannelID, lists, addr, name, locale, pr)
	})
	if err != nil {
		return nil, err
	}
	return answer, nil
}

func (s *Server) subscribeFromForm(ctx context.Context, q *store.Queries, events *eventBatch, channelID uuid.UUID, lists []store.MailingList, addr, name string, locale *string, pr consentProof) error {
	ws := lists[0].WorkspaceID
	if suppressed, err := q.IsEmailSuppressedAnyScope(ctx, store.IsEmailSuppressedAnyScopeParams{WorkspaceID: ws, Email: addr}); err != nil || suppressed {
		return err
	}
	opted, err := s.optedOut(ctx, q, ws, addr)
	if err != nil {
		return err
	}
	ready, err := s.readyChannels(ctx, q, ws, lists)
	if err != nil {
		return err
	}
	var contactID *uuid.UUID
	contact := func() (uuid.UUID, error) {
		if contactID != nil {
			return *contactID, nil
		}
		id, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: addr})
		if store.IsNotFound(err) {
			id, err = q.SubscriptionContactByEmail(ctx, store.SubscriptionContactByEmailParams{WorkspaceID: ws, Email: addr})
		}
		if store.IsNotFound(err) {
			id, err = s.formContact(ctx, q, ws, addr, name, locale)
		}
		if err != nil {
			return uuid.Nil(), err
		}
		contactID = &id
		return id, nil
	}
	confirm := func(l store.MailingList, sub store.ListSubscription) error {
		if l.ChannelID == nil || !ready[*l.ChannelID] {
			return nil
		}
		if ok, err := s.confirmationAllowed(ctx, ws, l.ID, addr, &channelID); err != nil || !ok {
			return err
		}
		return s.queueConfirmation(ctx, q, events, sub)
	}
	for _, l := range lists {
		cur, err := q.LockSubscriptionByEmail(ctx, store.LockSubscriptionByEmailParams{WorkspaceID: ws, ListID: l.ID, Email: addr})
		if err != nil && !store.IsNotFound(err) {
			return err
		}
		if err == nil {
			if cur.State == subSubscribed {
				continue
			}
			if cur, err = s.requestAgain(ctx, q, events, l, cur); err != nil {
				return err
			}
			if err := s.consentEvent(ctx, q, cur, consentRequested, pr); err != nil {
				return err
			}
			if err := confirm(l, cur); err != nil {
				return err
			}
			continue
		}
		id, err := contact()
		if err != nil {
			return err
		}
		if !l.DoubleOptIn && !opted {
			sub, err := s.newSubscription(ctx, q, events, l, id, addr, subSubscribed, pr.source, new(s.now()))
			if err != nil {
				return err
			}
			if err := s.consentEvent(ctx, q, sub, consentSubscribed, pr); err != nil {
				return err
			}
			s.queueWelcome(events, l, sub)
			continue
		}
		sub, err := s.newSubscription(ctx, q, events, l, id, addr, subPending, pr.source, nil)
		if err != nil {
			return err
		}
		if err := s.consentEvent(ctx, q, sub, consentRequested, pr); err != nil {
			return err
		}
		if err := confirm(l, sub); err != nil {
			return err
		}
	}
	return nil
}

// formContact creates the contact of an address nobody has: the address stays unconfirmed
// (typed) until the person confirms a subscription.
func (s *Server) formContact(ctx context.Context, q *store.Queries, ws uuid.UUID, addr, name string, locale *string) (uuid.UUID, error) {
	now := s.now()
	c, err := q.CreateContact(ctx, store.CreateContactParams{ID: newID(), WorkspaceID: ws, Name: name, Attributes: []byte("{}"), Now: now})
	if err != nil {
		return uuid.Nil(), err
	}
	if locale != nil {
		if _, err := q.SetContactIdentity(ctx, store.SetContactIdentityParams{WorkspaceID: ws, ID: c.ID, Name: name, Locale: locale, Attributes: c.Attributes, Now: now}); err != nil {
			return uuid.Nil(), err
		}
	}
	if _, err := q.SetContactTypedEmail(ctx, store.SetContactTypedEmailParams{WorkspaceID: ws, ID: c.ID, TypedEmail: &addr, Now: now}); err != nil {
		return uuid.Nil(), err
	}
	return c.ID, q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: ws, ID: c.ID})
}

// clientState is the contact's state on a list: subscribed before pending before unsubscribed
// when the contact has several addresses on it.
func clientState(subs []store.ListSubscription) *oas.SubscriptionState {
	rank := map[string]int{subSubscribed: 3, subPending: 2, subUnsubscribed: 1}
	best := ""
	for _, sub := range subs {
		if rank[sub.State] > rank[best] {
			best = sub.State
		}
	}
	if best == "" {
		return nil
	}
	st := oas.SubscriptionState(best)
	return &st
}

func (s *Server) ListClientSubscriptions(ctx context.Context, _ oas.ListClientSubscriptionsRequestObject) (oas.ListClientSubscriptionsResponseObject, error) {
	cp := contactFrom(ctx)
	lists, err := s.st.ListPublicMailingLists(ctx, store.ListPublicMailingListsParams{WorkspaceID: cp.workspaceID, InboxID: cp.inboxID})
	if err != nil {
		return nil, err
	}
	subs, err := s.st.ListContactSubscriptions(ctx, store.ListContactSubscriptionsParams{WorkspaceID: cp.workspaceID, ContactID: cp.contactID})
	if err != nil {
		return nil, err
	}
	byList := map[uuid.UUID][]store.ListSubscription{}
	for _, sub := range subs {
		byList[sub.ListID] = append(byList[sub.ListID], sub)
	}
	out := oas.ListClientSubscriptions200JSONResponse{Items: make([]oas.ClientSubscription, len(lists))}
	for i, l := range lists {
		out.Items[i] = oas.ClientSubscription{List: clientList(l), State: clientState(byList[l.ID])}
	}
	return out, nil
}

func (s *Server) sessionList(ctx context.Context, q *store.Queries, cp contactPrincipal, id uuid.UUID) (store.MailingList, error) {
	l, err := q.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: cp.workspaceID, ID: id})
	if store.IsNotFound(err) || (err == nil && (l.InboxID != cp.inboxID || !l.Public || l.ArchivedAt != nil)) {
		return l, errListGone
	}
	return l, err
}

// SubscribeClient subscribes a signed-in user of the host app: at once with an address the
// workspace knows for them, else through a confirmation mail.
func (s *Server) SubscribeClient(ctx context.Context, req oas.SubscribeClientRequestObject) (oas.SubscribeClientResponseObject, error) {
	cp := contactFrom(ctx)
	if !cp.identified {
		return nil, errIdentityRequired
	}
	text, err := trimmed(req.Body.ConsentText, 1, 2000, "consent_text")
	if err != nil {
		return nil, err
	}
	pr := s.requestProof(requestFrom(ctx), string(oas.SubscriptionSourceIdentity))
	pr.text = &text
	if req.Body.PageUrl != nil {
		pr.pageURL = optionalText(*req.Body.PageUrl, 2000)
	}
	var state string
	err = s.inTx(ctx, cp.workspaceID, func(q *store.Queries, events *eventBatch) error {
		l, err := s.sessionList(ctx, q, cp, req.ListId)
		if err != nil {
			return err
		}
		c, err := q.LockContact(ctx, store.LockContactParams{WorkspaceID: cp.workspaceID, ID: cp.contactID})
		if err != nil {
			return err
		}
		emails, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: cp.workspaceID, ContactIds: []uuid.UUID{c.ID}})
		if err != nil {
			return err
		}
		var addr string
		known := len(emails) > 0
		switch {
		case known:
			addr = emails[0].Email
		case c.TypedEmail != nil:
			addr = *c.TypedEmail
		default:
			return errNoEmail
		}
		ws := cp.workspaceID
		suppressed, err := q.IsEmailSuppressedAnyScope(ctx, store.IsEmailSuppressedAnyScopeParams{WorkspaceID: ws, Email: addr})
		if err != nil {
			return err
		}
		opted, err := s.optedOut(ctx, q, ws, addr)
		if err != nil {
			return err
		}
		confirm := func(sub store.ListSubscription) error {
			if suppressed || l.ChannelID == nil {
				return nil
			}
			if ready, err := s.readyChannels(ctx, q, ws, []store.MailingList{l}); err != nil || !ready[*l.ChannelID] {
				return err
			}
			if ok, err := s.confirmationAllowed(ctx, ws, l.ID, addr, nil); err != nil || !ok {
				return err
			}
			return s.queueConfirmation(ctx, q, events, sub)
		}
		cur, err := q.LockSubscriptionByEmail(ctx, store.LockSubscriptionByEmailParams{WorkspaceID: ws, ListID: l.ID, Email: addr})
		if err != nil && !store.IsNotFound(err) {
			return err
		}
		if err == nil {
			if cur.State == subUnsubscribed {
				if cur, err = s.requestAgain(ctx, q, events, l, cur); err != nil {
					return err
				}
				if err := s.consentEvent(ctx, q, cur, consentRequested, pr); err != nil {
					return err
				}
				if err := confirm(cur); err != nil {
					return err
				}
			}
			state = cur.State
			return nil
		}
		if known && !suppressed && !opted {
			sub, err := s.newSubscription(ctx, q, events, l, c.ID, addr, subSubscribed, pr.source, new(s.now()))
			if err != nil {
				return err
			}
			if err := s.consentEvent(ctx, q, sub, consentSubscribed, pr); err != nil {
				return err
			}
			s.queueWelcome(events, l, sub)
			state = sub.State
			return nil
		}
		sub, err := s.newSubscription(ctx, q, events, l, c.ID, addr, subPending, pr.source, nil)
		if err != nil {
			return err
		}
		if err := s.consentEvent(ctx, q, sub, consentRequested, pr); err != nil {
			return err
		}
		state = sub.State
		return confirm(sub)
	})
	if err != nil {
		return nil, err
	}
	return oas.SubscribeClient200JSONResponse{State: oas.SubscriptionState(state)}, nil
}

func (s *Server) UnsubscribeClient(ctx context.Context, req oas.UnsubscribeClientRequestObject) (oas.UnsubscribeClientResponseObject, error) {
	cp := contactFrom(ctx)
	reason := string(oas.UnsubscribeReasonClient)
	pr := s.requestProof(requestFrom(ctx), reason)
	err := s.inTx(ctx, cp.workspaceID, func(q *store.Queries, events *eventBatch) error {
		l, err := s.sessionList(ctx, q, cp, req.ListId)
		if err != nil {
			return err
		}
		subs, err := q.LockContactListSubscriptions(ctx, store.LockContactListSubscriptionsParams{WorkspaceID: cp.workspaceID, ListID: l.ID, ContactID: cp.contactID})
		if err != nil {
			return err
		}
		for _, sub := range subs {
			if _, err := s.unsubscribe(ctx, q, events, l, sub, reason, consentUnsubscribed, pr); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.UnsubscribeClient204Response{}, nil
}

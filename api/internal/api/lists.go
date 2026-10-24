package api

import (
	"context"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/oapi-codegen/nullable"
	"github.com/oapi-codegen/runtime/types"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

var (
	errListGone             = problem(http.StatusNotFound, "not_found", "no such mailing list")
	errListNameTaken        = problem(http.StatusConflict, "name_taken", "another list of the inbox has this name")
	errEmailChannelRequired = problem(http.StatusConflict, "email_channel_required", "the list needs an e-mail channel of its inbox with an SMTP account")
	errListKind             = problem(http.StatusConflict, "list_kind", "this is a status page's subscriber list; it is managed on its status page")
)

const (
	listKindList   = string(oas.MailingListKindList)
	listKindStatus = string(oas.MailingListKindStatus)
)

// visibleList loads a list the caller may see: lists of inboxes it cannot access answer 404.
func visibleList(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.MailingList, error) {
	get := q.GetMailingList
	if lock {
		get = func(ctx context.Context, arg store.GetMailingListParams) (store.MailingList, error) {
			return q.LockMailingList(ctx, store.LockMailingListParams(arg))
		}
	}
	l, err := get(ctx, store.GetMailingListParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return l, errListGone
	}
	if err != nil {
		return l, err
	}
	ok, err := inboxObjectsVisible(ctx, q, p, l.InboxID)
	if err != nil {
		return l, err
	}
	if !ok {
		return l, errListGone
	}
	return l, nil
}

// listSender is the e-mail channel and From address a list's mails go out from; ok is false when
// the list has no channel, the channel has no SMTP account or no address to send from.
func listSender(ch store.EmailChannel) (string, bool) {
	if ch.SmtpHost == "" {
		return "", false
	}
	if ch.FromAddress != nil {
		return *ch.FromAddress, true
	}
	if isCatchAll(ch.Address) {
		return "", false
	}
	return ch.Address, true
}

func (s *Server) readyChannels(ctx context.Context, q *store.Queries, ws uuid.UUID, lists []store.MailingList) (map[uuid.UUID]bool, error) {
	var ids []uuid.UUID
	for _, l := range lists {
		if l.ChannelID != nil {
			ids = append(ids, *l.ChannelID)
		}
	}
	out := map[uuid.UUID]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListEmailChannels(ctx, store.ListEmailChannelsParams{WorkspaceID: ws, ChannelIds: ids})
	if err != nil {
		return nil, err
	}
	for _, ch := range rows {
		_, ok := listSender(ch)
		out[ch.ChannelID] = ok
	}
	return out, nil
}

func (s *Server) mailingListBodies(ctx context.Context, q *store.Queries, ws uuid.UUID, lists []store.MailingList) ([]oas.MailingList, error) {
	out := make([]oas.MailingList, len(lists))
	if len(lists) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, len(lists))
	for i, l := range lists {
		ids[i] = l.ID
	}
	counts, err := q.CountSubscriptions(ctx, store.CountSubscriptionsParams{WorkspaceID: ws, ListIds: ids})
	if err != nil {
		return nil, err
	}
	byList := map[uuid.UUID]oas.MailingListCounts{}
	for _, c := range counts {
		byList[c.ListID] = oas.MailingListCounts{Subscribed: c.Subscribed, Pending: c.Pending, Unsubscribed: c.Unsubscribed}
	}
	ready, err := s.readyChannels(ctx, q, ws, lists)
	if err != nil {
		return nil, err
	}
	pages := map[uuid.UUID]uuid.UUID{}
	var statusIDs []uuid.UUID
	for _, l := range lists {
		if l.Kind == listKindStatus {
			statusIDs = append(statusIDs, l.ID)
		}
	}
	if len(statusIDs) > 0 {
		rows, err := q.ListStatusPagesByLists(ctx, store.ListStatusPagesByListsParams{WorkspaceID: ws, ListIds: statusIDs})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if r.MailingListID != nil {
				pages[*r.MailingListID] = r.ID
			}
		}
	}
	for i, l := range lists {
		out[i] = oas.MailingList{
			Id: l.ID, InboxId: l.InboxID, Kind: oas.MailingListKind(l.Kind), ChannelId: l.ChannelID, Name: l.Name, Description: l.Description,
			DoubleOptIn: l.DoubleOptIn, Public: l.Public, Commercial: l.Commercial, Welcome: l.Welcome,
			ArchivedAt: l.ArchivedAt, Counts: byList[l.ID], CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
			SenderReady: l.ChannelID != nil && ready[*l.ChannelID],
		}
		if id, ok := pages[l.ID]; ok {
			out[i].StatusPageId = &id
		}
	}
	return out, nil
}

func (s *Server) mailingListBody(ctx context.Context, q *store.Queries, l store.MailingList) (oas.MailingList, error) {
	out, err := s.mailingListBodies(ctx, q, l.WorkspaceID, []store.MailingList{l})
	if err != nil {
		return oas.MailingList{}, err
	}
	return out[0], nil
}

func listRef(l store.MailingList) oas.MailingListRef {
	return oas.MailingListRef{Id: l.ID, InboxId: l.InboxID, Name: l.Name, Kind: oas.MailingListKind(l.Kind)}
}

// listChannel checks that a list may send through the channel: an e-mail channel of its inbox that
// has an SMTP account and an address to send from.
func listChannel(ctx context.Context, q *store.Queries, ws, inboxID, channelID uuid.UUID) error {
	ch, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: ws, ID: channelID})
	if store.IsNotFound(err) || (err == nil && (ch.InboxID != inboxID || ch.Kind != string(oas.ChannelKindEmail))) {
		return errEmailChannelRequired
	}
	if err != nil {
		return err
	}
	ec, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws, ChannelID: channelID})
	if store.IsNotFound(err) {
		return errEmailChannelRequired
	}
	if err != nil {
		return err
	}
	if _, ok := listSender(ec); !ok {
		return errEmailChannelRequired
	}
	return nil
}

type listFields struct {
	name, description                        string
	doubleOptIn, public, commercial, welcome bool
}

func (f *listFields) validate() error {
	var err error
	if f.name, err = trimmed(f.name, 1, 100, "name"); err != nil {
		return err
	}
	f.description, err = trimmed(f.description, 0, 500, "description")
	return err
}

func boolOr(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

func (s *Server) ListMailingLists(ctx context.Context, req oas.ListMailingListsRequestObject) (oas.ListMailingListsResponseObject, error) {
	p := principalFrom(ctx)
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	if req.Params.InboxId != nil {
		if _, err := ownedInbox(ctx, s.st.Queries, p, *req.Params.InboxId); err != nil {
			return nil, err
		}
	}
	kind := listKindList
	if req.Params.Kind != nil {
		if !req.Params.Kind.Valid() {
			return nil, errValidation("kind must be list or status")
		}
		kind = string(*req.Params.Kind)
	}
	rows, err := s.st.ListMailingLists(ctx, store.ListMailingListsParams{
		WorkspaceID: p.workspaceID, InboxID: req.Params.InboxId, Archived: boolOr(req.Params.Archived, false), Kind: kind,
		AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), CursorAt: at, CursorID: id, Lim: lim + 1,
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
	items, err := s.mailingListBodies(ctx, s.st.Queries, p.workspaceID, rows)
	if err != nil {
		return nil, err
	}
	return oas.ListMailingLists200JSONResponse{Items: items, NextCursor: next}, nil
}

func (s *Server) CreateMailingList(ctx context.Context, req oas.CreateMailingListRequestObject) (oas.CreateMailingListResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	in, err := ownedInbox(ctx, s.st.Queries, p, b.InboxId)
	if err != nil {
		return nil, err
	}
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	f := listFields{
		name: b.Name, doubleOptIn: boolOr(b.DoubleOptIn, true), public: boolOr(b.Public, true),
		commercial: boolOr(b.Commercial, true), welcome: boolOr(b.Welcome, true),
	}
	if b.Description != nil {
		f.description = *b.Description
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	if b.ChannelId != nil {
		if err := listChannel(ctx, s.st.Queries, p.workspaceID, in.ID, *b.ChannelId); err != nil {
			return nil, err
		}
	}
	l, err := s.st.CreateMailingList(ctx, store.CreateMailingListParams{
		ID: newID(), WorkspaceID: p.workspaceID, InboxID: in.ID, ChannelID: b.ChannelId, Kind: listKindList, Name: f.name, Description: f.description,
		DoubleOptIn: f.doubleOptIn, Public: f.public, Commercial: f.commercial, Welcome: f.welcome, Now: s.now(),
	})
	if store.IsUniqueViolation(err) {
		return nil, errListNameTaken
	}
	if err != nil {
		return nil, err
	}
	out, err := s.mailingListBody(ctx, s.st.Queries, l)
	if err != nil {
		return nil, err
	}
	return oas.CreateMailingList201JSONResponse(out), nil
}

func (s *Server) GetMailingList(ctx context.Context, req oas.GetMailingListRequestObject) (oas.GetMailingListResponseObject, error) {
	p := principalFrom(ctx)
	l, err := visibleList(ctx, s.st.Queries, p, req.ListId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.mailingListBody(ctx, s.st.Queries, l)
	if err != nil {
		return nil, err
	}
	return oas.GetMailingList200JSONResponse(out), nil
}

func (s *Server) UpdateMailingList(ctx context.Context, req oas.UpdateMailingListRequestObject) (oas.UpdateMailingListResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	var out store.MailingList
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := visibleList(ctx, q, p, req.ListId, true)
		if err != nil {
			return err
		}
		if err := requireManagerOrKey(p); err != nil {
			return err
		}
		if cur.Kind == listKindStatus && (b.DoubleOptIn != nil || b.Public != nil || b.Commercial != nil || b.Welcome != nil ||
			b.ChannelId.IsSpecified() || b.Archived != nil) {
			return errListKind
		}
		f := listFields{
			name: cur.Name, description: cur.Description, doubleOptIn: boolOr(b.DoubleOptIn, cur.DoubleOptIn),
			public: boolOr(b.Public, cur.Public), commercial: boolOr(b.Commercial, cur.Commercial), welcome: boolOr(b.Welcome, cur.Welcome),
		}
		if b.Name != nil {
			f.name = *b.Name
		}
		if b.Description != nil {
			f.description = *b.Description
		}
		if err := f.validate(); err != nil {
			return err
		}
		channel := cur.ChannelID
		if b.ChannelId.IsSpecified() {
			channel = nil
			if !b.ChannelId.IsNull() {
				id := b.ChannelId.MustGet()
				if err := listChannel(ctx, q, p.workspaceID, cur.InboxID, id); err != nil {
					return err
				}
				channel = &id
			}
		}
		archived := cur.ArchivedAt
		if b.Archived != nil {
			switch {
			case !*b.Archived:
				archived = nil
			case archived == nil:
				archived = new(s.now())
			}
		}
		out, err = q.UpdateMailingList(ctx, store.UpdateMailingListParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Name: f.name, Description: f.description, DoubleOptIn: f.doubleOptIn,
			Public: f.public, Commercial: f.commercial, Welcome: f.welcome, ChannelID: channel, ArchivedAt: archived, Now: s.now(),
		})
		if store.IsUniqueViolation(err) {
			return errListNameTaken
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	body, err := s.mailingListBody(ctx, s.st.Queries, out)
	if err != nil {
		return nil, err
	}
	return oas.UpdateMailingList200JSONResponse(body), nil
}

func (s *Server) DeleteMailingList(ctx context.Context, req oas.DeleteMailingListRequestObject) (oas.DeleteMailingListResponseObject, error) {
	p := principalFrom(ctx)
	var uploads []string
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		l, err := visibleList(ctx, q, p, req.ListId, true)
		if err != nil {
			return err
		}
		if err := requireManagerOrKey(p); err != nil {
			return err
		}
		if l.Kind == listKindStatus {
			return errListKind
		}
		if uploads, err = q.ListImportStorageKeys(ctx, store.ListImportStorageKeysParams{WorkspaceID: p.workspaceID, ListID: &l.ID}); err != nil {
			return err
		}
		used, err := q.BroadcastUsesList(ctx, store.BroadcastUsesListParams{WorkspaceID: p.workspaceID, ListID: l.ID.String()})
		if err != nil {
			return err
		}
		if used {
			return errListInUse
		}
		emails, err := q.ListUnsubscribedEmails(ctx, store.ListUnsubscribedEmailsParams{WorkspaceID: p.workspaceID, ListID: l.ID})
		if err != nil {
			return err
		}
		if err := s.rememberOptOuts(ctx, q, p.workspaceID, emails, optOutErasure); err != nil {
			return err
		}
		_, err = q.DeleteMailingList(ctx, store.DeleteMailingListParams{WorkspaceID: p.workspaceID, ID: l.ID})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.deleteObjects(ctx, uploads)
	return oas.DeleteMailingList204Response{}, nil
}

func (s *Server) GetMailingListGrowth(ctx context.Context, req oas.GetMailingListGrowthRequestObject) (oas.GetMailingListGrowthResponseObject, error) {
	p := principalFrom(ctx)
	days := int32(30)
	if req.Params.Days != nil {
		days = int32(*req.Params.Days)
		if days != 30 && days != 90 {
			return nil, errValidation("days must be 30 or 90")
		}
	}
	l, err := visibleList(ctx, s.st.Queries, p, req.ListId, false)
	if err != nil {
		return nil, err
	}
	today := utcDay(s.now())
	first := today.AddDate(0, 0, -int(days-1))
	rows, err := s.st.MailingListGrowth(ctx, store.MailingListGrowthParams{WorkspaceID: p.workspaceID, ListID: l.ID, Since: first})
	if err != nil {
		return nil, err
	}
	byDay := map[string]store.MailingListGrowthRow{}
	for _, r := range rows {
		byDay[r.Day.Format(time.DateOnly)] = r
	}
	out := oas.GetMailingListGrowth200JSONResponse{Days: make([]oas.MailingListGrowthDay, 0, days)}
	for d := first; !d.After(today); d = d.AddDate(0, 0, 1) {
		r := byDay[d.Format(time.DateOnly)]
		out.Days = append(out.Days, oas.MailingListGrowthDay{Date: types.Date{Time: d}, New: r.New, Confirmed: r.Confirmed, Unsubscribed: r.Unsubscribed})
	}
	c, err := s.st.MailingListConfirmations(ctx, store.MailingListConfirmationsParams{WorkspaceID: p.workspaceID, ListID: l.ID, Since: first})
	if err != nil {
		return nil, err
	}
	if c.Asked > 0 {
		out.ConfirmationRate = new(float64(c.Confirmed) / float64(c.Asked))
	}
	return out, nil
}

func inboxSenderBody(in store.Inbox) oas.InboxSender {
	return oas.InboxSender{LegalName: in.SenderLegalName, PostalAddress: in.SenderPostalAddress, Registration: in.SenderRegistration, Contact: in.SenderContact}
}

// senderLine is the sender identity a list mail's footer names, empty when the inbox has none.
func senderLine(in store.Inbox) string {
	var parts []string
	for _, v := range []*string{in.SenderLegalName, in.SenderPostalAddress, in.SenderRegistration, in.SenderContact} {
		if v != nil && *v != "" {
			parts = append(parts, *v)
		}
	}
	return strings.Join(parts, " · ")
}

func (s *Server) setInboxSender(ctx context.Context, q *store.Queries, in store.Inbox, b oas.InboxSenderUpdate) (store.Inbox, error) {
	arg := store.SetInboxSenderParams{
		WorkspaceID: in.WorkspaceID, ID: in.ID, LegalName: in.SenderLegalName, PostalAddress: in.SenderPostalAddress,
		Registration: in.SenderRegistration, Contact: in.SenderContact, Now: s.now(),
	}
	for _, f := range []struct {
		in    nullable.Nullable[string]
		out   **string
		max   int
		field string
	}{
		{b.LegalName, &arg.LegalName, 200, "sender.legal_name"},
		{b.PostalAddress, &arg.PostalAddress, 500, "sender.postal_address"},
		{b.Registration, &arg.Registration, 200, "sender.registration"},
		{b.Contact, &arg.Contact, 320, "sender.contact"},
	} {
		if !f.in.IsSpecified() {
			continue
		}
		*f.out = nil
		if f.in.IsNull() {
			continue
		}
		v, err := trimmed(f.in.MustGet(), 0, f.max, f.field)
		if err != nil {
			return in, err
		}
		if v != "" {
			*f.out = &v
		}
	}
	return q.SetInboxSender(ctx, arg)
}

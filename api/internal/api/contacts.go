package api

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxAttributesBytes = 16 << 10

var (
	errEmailTaken      = problem(http.StatusConflict, "email_taken", "another contact has this e-mail address")
	errExternalIDTaken = problem(http.StatusConflict, "external_id_taken", "another contact has this external id in the inbox")
)

type contactRow = store.GetContactRow

type contactInput struct {
	name        string
	emails      []string
	externalIDs []oas.ExternalId
	attributes  []byte
	blocked     bool
}

func (s *Server) contactBodies(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, rows []contactRow) ([]oas.Contact, error) {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	emails, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: workspaceID, ContactIds: ids})
	if err != nil {
		return nil, err
	}
	externals, err := q.ListContactExternalIDs(ctx, store.ListContactExternalIDsParams{WorkspaceID: workspaceID, ContactIds: ids})
	if err != nil {
		return nil, err
	}
	byEmail := map[uuid.UUID][]oas.Email{}
	for _, e := range emails {
		byEmail[e.ContactID] = append(byEmail[e.ContactID], oas.Email(e.Email))
	}
	suppressions, err := q.ListContactSuppressions(ctx, store.ListContactSuppressionsParams{WorkspaceID: workspaceID, ContactIds: ids})
	if err != nil {
		return nil, err
	}
	byUndeliverable := map[uuid.UUID][]oas.UndeliverableEmail{}
	for _, x := range suppressions {
		u := oas.UndeliverableEmail{Email: oas.Email(x.Email), Reason: oas.UndeliverableEmailReason(x.Reason), CreatedAt: x.CreatedAt}
		if x.Detail != "" {
			d := x.Detail
			u.Detail = &d
		}
		byUndeliverable[x.ContactID] = append(byUndeliverable[x.ContactID], u)
	}
	byExternal := map[uuid.UUID][]oas.ExternalId{}
	for _, x := range externals {
		byExternal[x.ContactID] = append(byExternal[x.ContactID], oas.ExternalId{InboxId: x.InboxID, ExternalId: x.ExternalID})
	}
	out := make([]oas.Contact, len(rows))
	for i, r := range rows {
		c := oas.Contact{
			Id: r.ID, Name: r.Name, Blocked: r.Blocked, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			Emails: byEmail[r.ID], ExternalIds: byExternal[r.ID], Attributes: oas.Attributes{},
			Undeliverable: byUndeliverable[r.ID], Locale: r.Locale,
		}
		if c.Undeliverable == nil {
			c.Undeliverable = []oas.UndeliverableEmail{}
		}
		if c.Emails == nil {
			c.Emails = []oas.Email{}
		}
		if c.ExternalIds == nil {
			c.ExternalIds = []oas.ExternalId{}
		}
		_ = json.Unmarshal(r.Attributes, &c.Attributes)
		out[i] = c
	}
	return out, nil
}

func (s *Server) contactBody(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, r contactRow) (oas.Contact, error) {
	out, err := s.contactBodies(ctx, q, workspaceID, []contactRow{r})
	if err != nil {
		return oas.Contact{}, err
	}
	return out[0], nil
}

func contactEmails(in []oas.Email) ([]string, error) {
	if len(in) > 20 {
		return nil, errValidation("at most 20 emails")
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range in {
		n, err := normalizeEmail(e)
		if err != nil {
			return nil, err
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out, nil
}

func contactExternalIDs(in []oas.ExternalId) ([]oas.ExternalId, error) {
	if len(in) > 20 {
		return nil, errValidation("at most 20 external_ids")
	}
	type key struct {
		inbox uuid.UUID
		id    string
	}
	seen := map[key]bool{}
	var out []oas.ExternalId
	for _, x := range in {
		id, err := trimmed(x.ExternalId, 1, 200, "external_id")
		if err != nil {
			return nil, err
		}
		k := key{x.InboxId, id}
		if !seen[k] {
			seen[k] = true
			out = append(out, oas.ExternalId{InboxId: x.InboxId, ExternalId: id})
		}
	}
	return out, nil
}

func contactAttributes(a *oas.Attributes) ([]byte, error) {
	if a == nil || *a == nil {
		return []byte("{}"), nil
	}
	b := mustJSON(*a)
	if len(b) > maxAttributesBytes {
		return nil, errValidation("attributes must be at most 16 KiB")
	}
	return b, nil
}

// visibleContact refuses an agent a contact that has conversations or external ids only in inboxes
// the agent cannot access.
func visibleContact(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) error {
	if p.seesAllInboxes() {
		return nil
	}
	ok, err := q.ContactVisibleToViewer(ctx, store.ContactVisibleToViewerParams{WorkspaceID: p.workspaceID, ViewerID: p.viewerID(), ContactID: id})
	if err != nil {
		return err
	}
	if !ok {
		return errContactGone
	}
	return nil
}

// agentInboxes is the set of inboxes an agent can access, nil for principals that see all.
func agentInboxes(ctx context.Context, q *store.Queries, p principal) (map[uuid.UUID]bool, error) {
	if p.seesAllInboxes() {
		return nil, nil
	}
	ids, err := q.ListViewerInboxIDs(ctx, store.ListViewerInboxIDsParams{WorkspaceID: p.workspaceID, ViewerID: p.viewerID()})
	out := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, err
}

var errExternalIDInbox = errValidation("external_ids names an inbox that does not exist")

// limitAgentKeys checks an agent's contact write: external ids only in inboxes the agent can
// access, keeping the contact's ids in other inboxes, and e-mails only on a contact that appears
// in no other inbox.
func limitAgentKeys(ctx context.Context, q *store.Queries, p principal, contactID *uuid.UUID, in *contactInput, setEmails, setExternal bool) error {
	allowed, err := agentInboxes(ctx, q, p)
	if err != nil || allowed == nil {
		return err
	}
	for _, x := range in.externalIDs {
		if !allowed[x.InboxId] {
			return errExternalIDInbox
		}
	}
	if contactID == nil {
		return nil
	}
	if setEmails {
		inboxes, err := q.ContactInboxIDs(ctx, store.ContactInboxIDsParams{WorkspaceID: p.workspaceID, ContactID: *contactID})
		if err != nil {
			return err
		}
		for _, id := range inboxes {
			if !allowed[id] {
				return problem(http.StatusForbidden, "forbidden", "the contact also writes to an inbox you cannot access; ask an admin to change its e-mail addresses")
			}
		}
	}
	if setExternal {
		cur, err := q.ListContactExternalIDs(ctx, store.ListContactExternalIDsParams{WorkspaceID: p.workspaceID, ContactIds: []uuid.UUID{*contactID}})
		if err != nil {
			return err
		}
		for _, x := range cur {
			if !allowed[x.InboxID] {
				in.externalIDs = append(in.externalIDs, oas.ExternalId{InboxId: x.InboxID, ExternalId: x.ExternalID})
			}
		}
	}
	return nil
}

func writeContactKeys(ctx context.Context, q *store.Queries, workspaceID, contactID uuid.UUID, in contactInput, setEmails, setExternal bool) error {
	if setEmails {
		if err := q.DeleteContactEmails(ctx, store.DeleteContactEmailsParams{WorkspaceID: workspaceID, ContactID: contactID}); err != nil {
			return err
		}
		for i, e := range in.emails {
			err := q.AddContactEmail(ctx, store.AddContactEmailParams{WorkspaceID: workspaceID, ContactID: contactID, Email: e, Position: int32(i)})
			if store.IsUniqueViolation(err) {
				return errEmailTaken
			}
			if err != nil {
				return err
			}
		}
	}
	if setExternal {
		if err := q.DeleteContactExternalIDs(ctx, store.DeleteContactExternalIDsParams{WorkspaceID: workspaceID, ContactID: contactID}); err != nil {
			return err
		}
		for _, x := range in.externalIDs {
			err := q.AddContactExternalID(ctx, store.AddContactExternalIDParams{WorkspaceID: workspaceID, InboxID: x.InboxId, ExternalID: x.ExternalId, ContactID: contactID})
			if store.IsUniqueViolation(err) {
				return errExternalIDTaken
			}
			if store.IsForeignKeyViolation(err) {
				return errExternalIDInbox
			}
			if err != nil {
				return err
			}
		}
	}
	return q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: workspaceID, ID: contactID})
}

func (s *Server) ListContacts(ctx context.Context, req oas.ListContactsRequestObject) (oas.ListContactsResponseObject, error) {
	p := principalFrom(ctx)
	prm := req.Params
	lim, err := pageSize(prm.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(prm.Cursor)
	if err != nil {
		return nil, err
	}
	q, err := searchQuery(prm.Q)
	if err != nil {
		return nil, err
	}
	if prm.Kind != nil && !prm.Kind.Valid() {
		return nil, errValidation("kind must be known or visitor")
	}
	if prm.Sort != nil && !prm.Sort.Valid() {
		return nil, errValidation("sort must be created or last_seen")
	}
	var viewer *uuid.UUID
	if !p.seesAllInboxes() {
		id := p.viewerID()
		viewer = &id
	}
	arg := store.ListContactsParams{
		WorkspaceID: p.workspaceID, ViewerID: viewer, Q: q, Kind: (*string)(prm.Kind), HasOpen: prm.HasOpen,
		CursorAt: at, CursorID: id, Lim: lim + 1,
	}
	var (
		rows []contactRow
		pos  []time.Time
	)
	if prm.Sort != nil && *prm.Sort == oas.LastSeen {
		found, err := s.st.ListContactsByActivity(ctx, store.ListContactsByActivityParams(arg))
		if err != nil {
			return nil, err
		}
		for _, r := range found {
			rows = append(rows, contactRow{ID: r.ID, WorkspaceID: r.WorkspaceID, Name: r.Name, Attributes: r.Attributes, Blocked: r.Blocked,
				CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Locale: r.Locale, TypedEmail: r.TypedEmail})
			pos = append(pos, r.ActiveAt)
		}
	} else {
		found, err := s.st.ListContacts(ctx, arg)
		if err != nil {
			return nil, err
		}
		for _, r := range found {
			rows = append(rows, contactRow(r))
			pos = append(pos, r.CreatedAt)
		}
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(pos[lim-1], rows[lim-1].ID)
		next = &c
	}
	items, err := s.contactBodies(ctx, s.st.Queries, p.workspaceID, rows)
	if err != nil {
		return nil, err
	}
	if err := s.withActivity(ctx, p, items); err != nil {
		return nil, err
	}
	return oas.ListContacts200JSONResponse{Items: items, NextCursor: next}, nil
}

// withActivity adds what the caller may know of each contact's conversations and when the contact
// was last active.
func (s *Server) withActivity(ctx context.Context, p principal, items []oas.Contact) error {
	ids := make([]uuid.UUID, len(items))
	for i, c := range items {
		ids[i] = c.Id
	}
	rows, err := s.st.ListContactActivity(ctx, store.ListContactActivityParams{
		WorkspaceID: p.workspaceID, Ids: ids, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return err
	}
	byID := make(map[uuid.UUID]store.ListContactActivityRow, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	for i := range items {
		r := byID[items[i].Id]
		a := &oas.ContactActivity{LastSeenAt: r.LastActiveAt, Conversations: r.Conversations, OpenConversations: r.OpenConversations}
		if r.Conversations > 0 {
			a.LastConversationAt = &r.LastConversationAt
		}
		items[i].Activity = a
	}
	return nil
}

func (s *Server) GetContactSummary(ctx context.Context, req oas.GetContactSummaryRequestObject) (oas.GetContactSummaryResponseObject, error) {
	p := principalFrom(ctx)
	if _, err := s.st.GetContact(ctx, store.GetContactParams{WorkspaceID: p.workspaceID, ID: req.ContactId}); store.IsNotFound(err) {
		return nil, errContactGone
	} else if err != nil {
		return nil, err
	}
	if err := visibleContact(ctx, s.st.Queries, p, req.ContactId); err != nil {
		return nil, err
	}
	firsts, err := s.st.ContactFirstReplies(ctx, store.ContactFirstRepliesParams{
		WorkspaceID: p.workspaceID, ContactID: req.ContactId, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return nil, err
	}
	ratings, err := s.st.ContactRatings(ctx, store.ContactRatingsParams{
		WorkspaceID: p.workspaceID, ContactID: req.ContactId, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return nil, err
	}
	out := oas.GetContactSummary200JSONResponse{FirstReplies: int64(len(firsts))}
	if len(firsts) > 0 {
		rows := make([]store.StatsFirstRepliesRow, len(firsts))
		for i, f := range firsts {
			rows[i] = store.StatsFirstRepliesRow(f)
		}
		out.MedianFirstReplySeconds = new(medianSeconds(rows))
	}
	for _, r := range ratings {
		if r.Rating == string(oas.Good) {
			out.Ratings.Good = r.N
		} else {
			out.Ratings.Bad = r.N
		}
	}
	return out, nil
}

func (s *Server) CreateContact(ctx context.Context, req oas.CreateContactRequestObject) (oas.CreateContactResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	var in contactInput
	var err error
	if b.Name != nil {
		if in.name, err = trimmed(*b.Name, 0, 200, "name"); err != nil {
			return nil, err
		}
	}
	if b.Emails != nil {
		if in.emails, err = contactEmails(*b.Emails); err != nil {
			return nil, err
		}
	}
	if b.ExternalIds != nil {
		if in.externalIDs, err = contactExternalIDs(*b.ExternalIds); err != nil {
			return nil, err
		}
	}
	if in.attributes, err = contactAttributes(b.Attributes); err != nil {
		return nil, err
	}
	in.blocked = b.Blocked != nil && *b.Blocked
	var out oas.Contact
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		if err := limitAgentKeys(ctx, q, p, nil, &in, true, true); err != nil {
			return err
		}
		r, err := q.CreateContact(ctx, store.CreateContactParams{
			ID: newID(), WorkspaceID: p.workspaceID, Name: in.name, Attributes: in.attributes, Blocked: in.blocked, Now: s.now(),
		})
		if err != nil {
			return err
		}
		if err := writeContactKeys(ctx, q, p.workspaceID, r.ID, in, true, true); err != nil {
			return err
		}
		out, err = s.contactBody(ctx, q, p.workspaceID, contactRow(r))
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateContact201JSONResponse(out), nil
}

func (s *Server) GetContact(ctx context.Context, req oas.GetContactRequestObject) (oas.GetContactResponseObject, error) {
	p := principalFrom(ctx)
	r, err := s.st.GetContact(ctx, store.GetContactParams{WorkspaceID: p.workspaceID, ID: req.ContactId})
	if store.IsNotFound(err) {
		return nil, errContactGone
	}
	if err != nil {
		return nil, err
	}
	if err := visibleContact(ctx, s.st.Queries, p, r.ID); err != nil {
		return nil, err
	}
	out, err := s.contactBody(ctx, s.st.Queries, p.workspaceID, r)
	if err != nil {
		return nil, err
	}
	items := []oas.Contact{out}
	if err := s.withActivity(ctx, p, items); err != nil {
		return nil, err
	}
	return oas.GetContact200JSONResponse(items[0]), nil
}

func (s *Server) LookupContact(ctx context.Context, req oas.LookupContactRequestObject) (oas.LookupContactResponseObject, error) {
	p := principalFrom(ctx)
	if _, err := visibleInbox(ctx, s.st.Queries, p, req.Params.InboxId); err != nil {
		return nil, err
	}
	id, err := s.st.GetContactIDByExternalID(ctx, store.GetContactIDByExternalIDParams{
		WorkspaceID: p.workspaceID, InboxID: req.Params.InboxId, ExternalID: req.Params.ExternalId,
	})
	if store.IsNotFound(err) {
		return nil, errContactGone
	}
	if err != nil {
		return nil, err
	}
	r, err := s.st.GetContact(ctx, store.GetContactParams{WorkspaceID: p.workspaceID, ID: id})
	if err != nil {
		return nil, err
	}
	out, err := s.contactBody(ctx, s.st.Queries, p.workspaceID, r)
	if err != nil {
		return nil, err
	}
	return oas.LookupContact200JSONResponse(out), nil
}

func (s *Server) UpdateContact(ctx context.Context, req oas.UpdateContactRequestObject) (oas.UpdateContactResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	var out oas.Contact
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := q.LockContact(ctx, store.LockContactParams{WorkspaceID: p.workspaceID, ID: req.ContactId})
		if store.IsNotFound(err) {
			return errContactGone
		}
		if err != nil {
			return err
		}
		if err := visibleContact(ctx, q, p, cur.ID); err != nil {
			return err
		}
		in := contactInput{name: cur.Name, attributes: cur.Attributes, blocked: cur.Blocked}
		if b.Name != nil {
			if in.name, err = trimmed(*b.Name, 0, 200, "name"); err != nil {
				return err
			}
		}
		if b.Emails != nil {
			if in.emails, err = contactEmails(*b.Emails); err != nil {
				return err
			}
		}
		if b.ExternalIds != nil {
			if in.externalIDs, err = contactExternalIDs(*b.ExternalIds); err != nil {
				return err
			}
		}
		if b.Attributes != nil {
			if in.attributes, err = contactAttributes(b.Attributes); err != nil {
				return err
			}
		}
		if b.Blocked != nil {
			in.blocked = *b.Blocked
		}
		if err := limitAgentKeys(ctx, q, p, &cur.ID, &in, b.Emails != nil, b.ExternalIds != nil); err != nil {
			return err
		}
		if b.ClearUndeliverable != nil {
			addrs, err := contactEmails(*b.ClearUndeliverable)
			if err != nil {
				return err
			}
			own, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: p.workspaceID, ContactIds: []uuid.UUID{cur.ID}})
			if err != nil {
				return err
			}
			for _, e := range addrs {
				if !slices.ContainsFunc(own, func(o store.ListContactEmailsRow) bool { return o.Email == e }) {
					return errValidation("clear_undeliverable names an address the contact does not have")
				}
				if err := q.ClearSuppression(ctx, store.ClearSuppressionParams{WorkspaceID: p.workspaceID, Email: e}); err != nil {
					return err
				}
			}
		}
		r, err := q.UpdateContact(ctx, store.UpdateContactParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Name: in.name, Attributes: in.attributes, Blocked: in.blocked, Now: s.now(),
		})
		if err != nil {
			return err
		}
		if err := writeContactKeys(ctx, q, p.workspaceID, cur.ID, in, b.Emails != nil, b.ExternalIds != nil); err != nil {
			return err
		}
		if out, err = s.contactBody(ctx, q, p.workspaceID, contactRow(r)); err != nil {
			return err
		}
		events.add(realtime.ContactUpdated, nil, nil, out)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateContact200JSONResponse(out), nil
}

func (s *Server) DeleteContact(ctx context.Context, req oas.DeleteContactRequestObject) (oas.DeleteContactResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrFullKey(p); err != nil {
		return nil, err
	}
	if err := s.deleteContact(ctx, p, req.ContactId); err != nil {
		return nil, err
	}
	return oas.DeleteContact204Response{}, nil
}

// deleteContact deletes the contact with everything that cascades from it, and keeps their
// external ids for the contact.deleted webhook.
func (s *Server) deleteContact(ctx context.Context, p principal, id uuid.UUID) error {
	var keys []string
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if _, err := q.LockContact(ctx, store.LockContactParams{WorkspaceID: p.workspaceID, ID: id}); store.IsNotFound(err) {
			return errContactGone
		} else if err != nil {
			return err
		}
		var err error
		if keys, err = q.ListContactStorageKeys(ctx, store.ListContactStorageKeysParams{WorkspaceID: p.workspaceID, ContactID: id}); err != nil {
			return err
		}
		raw, err := q.ListContactRawKeys(ctx, store.ListContactRawKeysParams{WorkspaceID: p.workspaceID, ContactID: id})
		if err != nil {
			return err
		}
		keys = append(keys, raw...)
		externals, err := q.ListContactExternalIDs(ctx, store.ListContactExternalIDsParams{WorkspaceID: p.workspaceID, ContactIds: []uuid.UUID{id}})
		if err != nil {
			return err
		}
		inboxes, err := q.ContactInboxIDs(ctx, store.ContactInboxIDsParams{WorkspaceID: p.workspaceID, ContactID: id})
		if err != nil {
			return err
		}
		snap := &deletedContact{Contact: oas.WebhookDeletedContact{Id: id, ExternalIds: []oas.ExternalId{}}, Inboxes: inboxes}
		for _, x := range externals {
			snap.Contact.ExternalIds = append(snap.Contact.ExternalIds, oas.ExternalId{InboxId: x.InboxID, ExternalId: x.ExternalID})
		}
		if _, err = q.DeleteContact(ctx, store.DeleteContactParams{WorkspaceID: p.workspaceID, ID: id}); err != nil {
			return err
		}
		events.add(realtime.ContactDeleted, nil, nil, oas.ContactRef{Id: id})
		events.items[len(events.items)-1].deleted = snap
		return nil
	})
	if err != nil {
		return err
	}
	s.deleteObjects(ctx, keys)
	return nil
}

func (s *Server) MergeContact(ctx context.Context, req oas.MergeContactRequestObject) (oas.MergeContactResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrFullKey(p); err != nil {
		return nil, err
	}
	to, from := req.ContactId, req.Body.SourceId
	if to == from {
		return nil, errValidation("source_id must be another contact")
	}
	var out oas.Contact
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		locked := map[uuid.UUID]store.LockContactRow{}
		ids := []uuid.UUID{to, from}
		slices.SortFunc(ids, func(a, b uuid.UUID) int { return a.Compare(b) })
		for _, id := range ids {
			r, err := q.LockContact(ctx, store.LockContactParams{WorkspaceID: p.workspaceID, ID: id})
			if store.IsNotFound(err) {
				return errContactGone
			}
			if err != nil {
				return err
			}
			locked[id] = r
		}
		target, source := locked[to], locked[from]
		attrs := map[string]any{}
		_ = json.Unmarshal(source.Attributes, &attrs)
		var own map[string]any
		_ = json.Unmarshal(target.Attributes, &own)
		for k, v := range own {
			attrs[k] = v
		}
		merged := mustJSON(attrs)
		if len(merged) > maxAttributesBytes {
			return errValidation("the merged attributes would be larger than 16 KiB")
		}
		externals, err := q.ListContactExternalIDs(ctx, store.ListContactExternalIDsParams{WorkspaceID: p.workspaceID, ContactIds: []uuid.UUID{from}})
		if err != nil {
			return err
		}
		inboxes, err := q.ContactInboxIDs(ctx, store.ContactInboxIDsParams{WorkspaceID: p.workspaceID, ContactID: from})
		if err != nil {
			return err
		}
		now := s.now()
		moved, err := q.MoveContactConversations(ctx, store.MoveContactConversationsParams{WorkspaceID: p.workspaceID, FromContact: from, ToContact: to, Now: now})
		if err != nil {
			return err
		}
		if err := q.MoveContactMessages(ctx, store.MoveContactMessagesParams{WorkspaceID: p.workspaceID, FromContact: &from, ToContact: &to}); err != nil {
			return err
		}
		if err := q.MoveContactEmails(ctx, store.MoveContactEmailsParams{WorkspaceID: p.workspaceID, FromContact: from, ToContact: to}); err != nil {
			return err
		}
		if err := q.MoveContactExternalIDs(ctx, store.MoveContactExternalIDsParams{WorkspaceID: p.workspaceID, FromContact: from, ToContact: to}); err != nil {
			return err
		}
		name, locale := target.Name, target.Locale
		if name == "" {
			name = source.Name
		}
		if locale == nil {
			locale = source.Locale
		}
		r, err := q.SetContactIdentity(ctx, store.SetContactIdentityParams{WorkspaceID: p.workspaceID, ID: to, Name: name, Locale: locale, Attributes: merged, Now: now})
		if err != nil {
			return err
		}
		if r.TypedEmail == nil && source.TypedEmail != nil {
			typed, err := q.SetContactTypedEmail(ctx, store.SetContactTypedEmailParams{WorkspaceID: p.workspaceID, ID: to, TypedEmail: source.TypedEmail, Now: now})
			if err != nil {
				return err
			}
			r = store.SetContactIdentityRow(typed)
		}
		if err := q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: p.workspaceID, ID: to}); err != nil {
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
		if _, err := q.DeleteContact(ctx, store.DeleteContactParams{WorkspaceID: p.workspaceID, ID: from}); err != nil {
			return err
		}
		if out, err = s.contactBody(ctx, q, p.workspaceID, contactRow(r)); err != nil {
			return err
		}
		events.add(realtime.ContactUpdated, nil, nil, out)
		snap := &deletedContact{Contact: oas.WebhookDeletedContact{Id: from, ExternalIds: []oas.ExternalId{}, MergedIntoId: &to}, Inboxes: inboxes}
		for _, x := range externals {
			snap.Contact.ExternalIds = append(snap.Contact.ExternalIds, oas.ExternalId{InboxId: x.InboxID, ExternalId: x.ExternalID})
		}
		events.add(realtime.ContactDeleted, nil, nil, oas.ContactRef{Id: from, MergedIntoId: &to})
		events.items[len(events.items)-1].deleted = snap
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.MergeContact200JSONResponse(out), nil
}

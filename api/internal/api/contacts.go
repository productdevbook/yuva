package api

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
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
	ok, err := q.ContactVisibleToMember(ctx, store.ContactVisibleToMemberParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ContactID: id})
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
	ids, err := q.ListMemberInboxIDs(ctx, store.ListMemberInboxIDsParams{WorkspaceID: p.workspaceID, MemberID: p.memberID})
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
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	q, err := searchQuery(req.Params.Q)
	if err != nil {
		return nil, err
	}
	var member *uuid.UUID
	if !p.seesAllInboxes() {
		member = &p.memberID
	}
	rows, err := s.st.ListContacts(ctx, store.ListContactsParams{WorkspaceID: p.workspaceID, MemberID: member, Q: q, CursorAt: at, CursorID: id, Lim: lim + 1})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		next = &c
	}
	conv := make([]contactRow, len(rows))
	for i, r := range rows {
		conv[i] = contactRow(r)
	}
	items, err := s.contactBodies(ctx, s.st.Queries, p.workspaceID, conv)
	if err != nil {
		return nil, err
	}
	return oas.ListContacts200JSONResponse{Items: items, NextCursor: next}, nil
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
	return oas.GetContact200JSONResponse(out), nil
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
	if err := requireManagerOrKey(p); err != nil {
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

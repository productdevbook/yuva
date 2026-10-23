package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxParticipants      = 20
	maxCopies            = 20
	maxParticipantChecks = 50

	participantActive    = string(oas.Active)
	participantSuggested = string(oas.Suggested)
	participantCc        = string(oas.Cc)
	participantForwarded = string(oas.Forwarded)
)

var (
	errParticipantsFull = problem(http.StatusConflict, "participants_full", "a conversation has at most 20 participants")
	errParticipantGone  = problem(http.StatusNotFound, "not_found", "no such participant")
)

// serverAddress reports an address the workspace receives mail for (any of its channels'
// addresses, tags or catch-all domains, proven or not, and any forwarding address) or sends from
// (a channel's from_address, the server's own sender). Other workspaces' addresses never count.
func (s *Server) serverAddress(ctx context.Context, ws uuid.UUID, addr string) (bool, error) {
	local, domain, _ := strings.Cut(addr, "@")
	if s.ingress.ForwardingDomain != "" && domain == s.ingress.ForwardingDomain {
		return true, nil
	}
	if slices.Contains(s.ingress.OwnAddresses, addr) {
		return true, nil
	}
	candidates := []string{addr}
	if base, _, tagged := strings.Cut(local, "+"); tagged {
		candidates = append(candidates, base+"@"+domain)
	}
	if domain != "" && !s.sharedMailDomain(domain) {
		candidates = append(candidates, catchAllLocal+"@"+domain)
	}
	return s.st.IsWorkspaceEmailAddress(ctx, store.IsWorkspaceEmailAddressParams{WorkspaceID: ws, Addresses: candidates, Address: addr})
}

// addParticipants keeps the To and Cc addresses of an inbound mail in c as participants with role,
// leaving out the sender, the contact's own addresses and every address of this server. They are active
// only when the mail passed DMARC: otherwise anyone could forge the contact's From and have
// replies copied to an address of their choosing.
func (s *Server) addParticipants(ctx context.Context, q *store.Queries, events *eventBatch, c store.Conversation, m *email.Message, sender string, verified bool, role string, now time.Time) error {
	own, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: c.WorkspaceID, ContactIds: []uuid.UUID{c.ContactID}})
	if err != nil {
		return err
	}
	skip := []string{strings.ToLower(sender)}
	for _, e := range own {
		skip = append(skip, e.Email)
	}
	checked := 0
	var changed int64
	for _, addr := range normalizedAddressList(slices.Concat(m.To, m.Cc)) {
		if slices.Contains(skip, addr) || len(addr) > 320 {
			continue
		}
		skip = append(skip, addr)
		if checked == maxParticipantChecks {
			break
		}
		checked++
		ours, err := s.serverAddress(ctx, c.WorkspaceID, addr)
		if err != nil {
			return err
		}
		if ours {
			continue
		}
		status := participantSuggested
		if verified {
			status = participantActive
		}
		n, err := putParticipant(ctx, q, c, addr, status, role, now)
		if err != nil {
			return err
		}
		changed += n
	}
	if changed == 0 {
		return nil
	}
	return emitParticipants(ctx, q, events, c)
}

// putParticipant adds addr to c's participants, or makes a suggested one active (and a forwarded
// one cc when role is cc), and reports whether anything changed; a full conversation adds nothing.
func putParticipant(ctx context.Context, q *store.Queries, c store.Conversation, addr, status, role string, now time.Time) (int64, error) {
	if status == participantActive {
		n, err := q.ActivateConversationParticipant(ctx, store.ActivateConversationParticipantParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID, Email: addr, Role: role})
		if err != nil || n > 0 {
			return n, err
		}
	}
	return q.AddConversationParticipant(ctx, store.AddConversationParticipantParams{
		WorkspaceID: c.WorkspaceID, ConversationID: c.ID, Email: addr, Role: role, Status: status, Now: now, MaxParticipants: maxParticipants,
	})
}

func participantItems(rows []store.ListConversationParticipantsRow) []oas.ConversationParticipant {
	out := make([]oas.ConversationParticipant, len(rows))
	for i, r := range rows {
		out[i] = oas.ConversationParticipant{
			Email: oas.Email(r.Email), Role: oas.ConversationParticipantRole(r.Role), Status: oas.ConversationParticipantStatus(r.Status),
			CreatedAt: r.CreatedAt, ContactId: r.ContactID, Name: r.Name,
		}
	}
	return out
}

func emitParticipants(ctx context.Context, q *store.Queries, events *eventBatch, c store.Conversation) error {
	rows, err := q.ListConversationParticipants(ctx, store.ListConversationParticipantsParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID})
	if err != nil {
		return err
	}
	events.conversation(realtime.ConversationParticipants, c, oas.ConversationParticipantsChange{ConversationId: c.ID, Items: participantItems(rows)})
	return nil
}

// copyList is the cc or bcc a reply was sent with; set is false when the caller left it out.
type copyList struct {
	set   bool
	addrs []string
}

func copiesOf(v *[]oas.Email) copyList {
	if v == nil {
		return copyList{}
	}
	out := copyList{set: true, addrs: []string{}}
	for _, e := range *v {
		out.addrs = append(out.addrs, string(e))
	}
	return out
}

func (in *messageInput) hasCopies() bool { return in.cc.set || in.bcc.set }

// setCopies fills the plan's Cc and Bcc: the given lists, checked, or by default the
// conversation's participants that can still receive mail.
func (s *Server) setCopies(ctx context.Context, q *store.Queries, c store.Conversation, plan *emailPlan, cc, bcc copyList) error {
	taken := []string{plan.to}
	if cc.set {
		list, err := s.checkCopies(ctx, q, c.WorkspaceID, "cc", cc.addrs, &taken)
		if err != nil {
			return err
		}
		plan.cc = list
	} else {
		rows, err := q.ListConversationParticipants(ctx, store.ListConversationParticipantsParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID})
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.Status != participantActive || r.Role == participantForwarded || slices.Contains(taken, r.Email) {
				continue
			}
			suppressed, err := q.IsEmailSuppressed(ctx, store.IsEmailSuppressedParams{WorkspaceID: c.WorkspaceID, Email: r.Email})
			if err != nil {
				return err
			}
			ours, err := s.serverAddress(ctx, c.WorkspaceID, r.Email)
			if err != nil {
				return err
			}
			if suppressed || ours {
				continue
			}
			taken = append(taken, r.Email)
			plan.cc = append(plan.cc, r.Email)
		}
	}
	list, err := s.checkCopies(ctx, q, c.WorkspaceID, "bcc", bcc.addrs, &taken)
	if err != nil {
		return err
	}
	plan.bcc = list
	return nil
}

func (s *Server) checkCopies(ctx context.Context, q *store.Queries, ws uuid.UUID, field string, in []string, taken *[]string) ([]string, error) {
	if len(in) > maxCopies {
		return nil, errValidation(field + " can name at most 20 addresses")
	}
	var out []string
	for _, v := range in {
		addr, err := s.copyAddress(ctx, ws, field, v)
		if err != nil {
			return nil, err
		}
		a := email.Address{Email: addr}
		if slices.Contains(*taken, a.Email) {
			continue
		}
		suppressed, err := q.IsEmailSuppressed(ctx, store.IsEmailSuppressedParams{WorkspaceID: ws, Email: a.Email})
		if err != nil {
			return nil, err
		}
		if suppressed {
			return nil, problem(http.StatusConflict, "email_undeliverable", field+": "+a.Email+" bounced or complained; clear it on the contact to send again")
		}
		*taken = append(*taken, a.Email)
		out = append(out, a.Email)
	}
	return out, nil
}

// copyAddress checks one address a caller names: valid, and not one the workspace receives or
// sends mail for. Both answer alike.
func (s *Server) copyAddress(ctx context.Context, ws uuid.UUID, field, v string) (string, error) {
	invalid := errValidation(field + ": " + v + " is not an e-mail address that replies can go to")
	a, ok := email.ParseAddress(v)
	if !ok || a.Name != "" || len(a.Email) > 320 || !strings.Contains(email.Domain(a.Email), ".") {
		return "", invalid
	}
	ours, err := s.serverAddress(ctx, ws, a.Email)
	if err != nil {
		return "", err
	}
	if ours {
		return "", invalid
	}
	return a.Email, nil
}

// keepCopiesAsParticipants makes the addresses a member's reply copied active participants of c.
func keepCopiesAsParticipants(ctx context.Context, q *store.Queries, events *eventBatch, c store.Conversation, plan *emailPlan, now time.Time) error {
	if plan == nil {
		return nil
	}
	var changed int64
	for _, addr := range plan.cc {
		n, err := putParticipant(ctx, q, c, addr, participantActive, participantCc, now)
		if err != nil {
			return err
		}
		changed += n
	}
	if changed == 0 {
		return nil
	}
	return emitParticipants(ctx, q, events, c)
}

func (s *Server) AddConversationParticipant(ctx context.Context, req oas.AddConversationParticipantRequestObject) (oas.AddConversationParticipantResponseObject, error) {
	p := principalFrom(ctx)
	addr, err := s.copyAddress(ctx, p.workspaceID, "email", string(req.Body.Email))
	if err != nil {
		return nil, err
	}
	var out []oas.ConversationParticipant
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := visibleConversation(ctx, q, p, req.ConversationId, true)
		if err != nil {
			return err
		}
		n, err := putParticipant(ctx, q, c, addr, participantActive, participantCc, s.now())
		if err != nil {
			return err
		}
		rows, err := q.ListConversationParticipants(ctx, store.ListConversationParticipantsParams{WorkspaceID: p.workspaceID, ConversationID: c.ID})
		if err != nil {
			return err
		}
		if n == 0 && !slices.ContainsFunc(rows, func(r store.ListConversationParticipantsRow) bool { return r.Email == addr }) {
			return errParticipantsFull
		}
		out = participantItems(rows)
		if n > 0 {
			events.conversation(realtime.ConversationParticipants, c, oas.ConversationParticipantsChange{ConversationId: c.ID, Items: out})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.AddConversationParticipant200JSONResponse{Items: out}, nil
}

func (s *Server) RemoveConversationParticipant(ctx context.Context, req oas.RemoveConversationParticipantRequestObject) (oas.RemoveConversationParticipantResponseObject, error) {
	p := principalFrom(ctx)
	var out []oas.ConversationParticipant
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := visibleConversation(ctx, q, p, req.ConversationId, true)
		if err != nil {
			return err
		}
		n, err := q.DeleteConversationParticipant(ctx, store.DeleteConversationParticipantParams{
			WorkspaceID: p.workspaceID, ConversationID: c.ID, Email: strings.ToLower(string(req.Email)),
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return errParticipantGone
		}
		rows, err := q.ListConversationParticipants(ctx, store.ListConversationParticipantsParams{WorkspaceID: p.workspaceID, ConversationID: c.ID})
		if err != nil {
			return err
		}
		out = participantItems(rows)
		events.conversation(realtime.ConversationParticipants, c, oas.ConversationParticipantsChange{ConversationId: c.ID, Items: out})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.RemoveConversationParticipant200JSONResponse{Items: out}, nil
}

var errCopiesNotEmailed = errValidation("cc and bcc go only with a reply that is e-mailed")

func (s *Server) ListConversationParticipants(ctx context.Context, req oas.ListConversationParticipantsRequestObject) (oas.ListConversationParticipantsResponseObject, error) {
	p := principalFrom(ctx)
	c, err := readableConversation(ctx, s.st.Queries, p, req.ConversationId)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListConversationParticipants(ctx, store.ListConversationParticipantsParams{WorkspaceID: p.workspaceID, ConversationID: c.ID})
	if err != nil {
		return nil, err
	}
	return oas.ListConversationParticipants200JSONResponse{Items: participantItems(rows)}, nil
}

func (s *Server) ListContactParticipations(ctx context.Context, req oas.ListContactParticipationsRequestObject) (oas.ListContactParticipationsResponseObject, error) {
	p := principalFrom(ctx)
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	if _, err := s.st.GetContact(ctx, store.GetContactParams{WorkspaceID: p.workspaceID, ID: req.ContactId}); store.IsNotFound(err) {
		return nil, errContactGone
	} else if err != nil {
		return nil, err
	}
	if err := visibleContact(ctx, s.st.Queries, p, req.ContactId); err != nil {
		return nil, err
	}
	rows, err := s.st.ListContactParticipations(ctx, store.ListContactParticipationsParams{
		WorkspaceID: p.workspaceID, ContactID: req.ContactId, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
		CursorAt: at, CursorID: id, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		cur := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		next = &cur
	}
	out := oas.ListContactParticipations200JSONResponse{Items: make([]oas.ContactParticipation, len(rows)), NextCursor: next}
	for i, r := range rows {
		out.Items[i] = oas.ContactParticipation{
			ConversationId: r.ID, InboxId: r.InboxID, Subject: r.Subject, Emails: emailList(r.Emails),
			Participant: r.Participant, CreatedAt: r.CreatedAt,
		}
	}
	return out, nil
}

// forgetContactAddresses removes a contact's addresses from other conversations' participants
// and stored e-mail recipients, before the contact is deleted.
func forgetContactAddresses(ctx context.Context, q *store.Queries, ws, contactID uuid.UUID) error {
	rows, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: ws, ContactIds: []uuid.UUID{contactID}})
	if err != nil || len(rows) == 0 {
		return err
	}
	emails := make([]string, len(rows))
	for i, r := range rows {
		emails[i] = r.Email
	}
	if err := q.DeleteParticipantsByEmails(ctx, store.DeleteParticipantsByEmailsParams{WorkspaceID: ws, Emails: emails}); err != nil {
		return err
	}
	if err := q.ScrubForwardRecipients(ctx, store.ScrubForwardRecipientsParams{WorkspaceID: ws, Emails: emails}); err != nil {
		return err
	}
	if err := q.ScrubMessageEmailAddresses(ctx, store.ScrubMessageEmailAddressesParams{WorkspaceID: ws, Emails: emails}); err != nil {
		return err
	}
	return q.ForgetMigrationAddresses(ctx, store.ForgetMigrationAddressesParams{WorkspaceID: ws, Emails: emails})
}

// clearMigrationPersonalData drops what a migration's work rows hold about people: the scanned
// headers or tickets, the thread plans, the names found and the subjects of the errors. The rows
// stay, so the counts do.
func clearMigrationPersonalData(ctx context.Context, q *store.Queries, ws, id uuid.UUID) error {
	if err := q.ClearMigrationItemMeta(ctx, store.ClearMigrationItemMetaParams{WorkspaceID: ws, MigrationID: id}); err != nil {
		return err
	}
	if err := q.ClearMigrationThreadPlans(ctx, store.ClearMigrationThreadPlansParams{WorkspaceID: ws, MigrationID: id}); err != nil {
		return err
	}
	if err := q.ClearMigrationErrorSubjects(ctx, store.ClearMigrationErrorSubjectsParams{WorkspaceID: ws, MigrationID: id}); err != nil {
		return err
	}
	if err := q.DeleteMigrationPlanNames(ctx, store.DeleteMigrationPlanNamesParams{WorkspaceID: ws, MigrationID: id}); err != nil {
		return err
	}
	return q.DeleteMigrationPlanNodes(ctx, store.DeleteMigrationPlanNodesParams{WorkspaceID: ws, MigrationID: id})
}

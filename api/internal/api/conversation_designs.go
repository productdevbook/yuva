package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/mail/design"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

var (
	limitDesignedOutreach   = limit{20, time.Hour}
	limitDesignedOutreachWS = limit{200, 24 * time.Hour}
	outreachTokenContext    = []byte("yuva outreach manage v1")

	errDesignNotEmailed      = problem(http.StatusBadRequest, "design_not_emailed", "only an outgoing message e-mailed to the contact can use a design")
	errDesignWithHTML        = errValidation("a message with design_id takes no html; the design is its HTML")
	errDesignOneRecipient    = problem(http.StatusBadRequest, "design_one_recipient", "new mail with a design goes to one to address and no cc, since its merge fields show that person's details")
	errDesignUnavailable     = problem(http.StatusConflict, "design_unavailable", "the design is archived, not offered in conversations, or another inbox's")
	errDesignLocaleUnknown   = problem(http.StatusBadRequest, "design_locale_unknown", "design_locale is not one of the design's versions")
	errEmailOptedOut         = problem(http.StatusConflict, "email_opted_out", "the address asked for no more mail like this; a designed message to someone who has not written is refused, a plain one still goes")
	errSenderIdentityMissing = problem(http.StatusConflict, "sender_identity_missing", "designed mail to someone who has not written carries the inbox's legal name and postal address; set them in the inbox's sender settings")
	errDesignHTMLTooLarge    = designProblem(http.StatusUnprocessableEntity, "html_too_large", "the rendered mail is over 500 KB", nil)
)

type designedMail struct {
	id       uuid.UUID
	version  int32
	locale   string
	name     string
	outreach bool
	html     string
	text     string
}

type designCall struct {
	id      uuid.UUID
	locale  *string
	to      string
	slot    *design.MessageSlot
	dark    bool
	preview bool
}

type designRender struct {
	row      store.EmailDesign
	locale   string
	outreach bool
	out      design.Output
}

// Seeing the conversation is seeing its inbox, so the design's inbox needs no visibility check.
func usableDesign(ctx context.Context, q *store.Queries, c store.Conversation, id uuid.UUID) (store.EmailDesign, error) {
	d, err := q.GetEmailDesign(ctx, store.GetEmailDesignParams{WorkspaceID: c.WorkspaceID, ID: id})
	if store.IsNotFound(err) {
		return d, errDesignGone
	}
	if err != nil {
		return d, err
	}
	if d.InboxID != nil && *d.InboxID != c.InboxID {
		return d, errDesignUnavailable
	}
	if d.ArchivedAt != nil || !d.ForConversations {
		return d, errDesignUnavailable
	}
	return d, nil
}

func messageBlockIssue(d design.Document) []design.Issue {
	if n, nested := d.HasMessageBlock(); n != 1 || nested {
		return []design.Issue{{Field: "blocks", Code: "message_block_missing"}}
	}
	return nil
}

func designVersion(d design.Document, want *string, contactLocale string) (string, error) {
	if want == nil {
		v, _ := d.Pick(contactLocale)
		return v, nil
	}
	i := slices.IndexFunc(d.Versions(), func(v string) bool { return strings.EqualFold(v, *want) })
	if i < 0 {
		return "", errDesignLocaleUnknown
	}
	return d.Versions()[i], nil
}

func (s *Server) contactWrote(ctx context.Context, q *store.Queries, c store.Conversation) (bool, error) {
	contact := c.ContactID
	return q.ContactWroteInConversation(ctx, store.ContactWroteInConversationParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID, ContactID: &contact})
}

func (s *Server) renderForConversation(ctx context.Context, q *store.Queries, c store.Conversation, dc designCall) (designRender, error) {
	row, err := usableDesign(ctx, q, c, dc.id)
	if err != nil {
		return designRender{}, err
	}
	d, issues, err := design.Parse(row.Document)
	if err != nil {
		return designRender{}, err
	}
	if len(issues) > 0 {
		return designRender{}, errors.New("stored design does not parse")
	}
	if is := messageBlockIssue(d); is != nil {
		return designRender{}, issuesProblem(is)
	}
	wrote, err := s.contactWrote(ctx, q, c)
	if err != nil {
		return designRender{}, err
	}
	values, contactLocale, err := contactValuesOf(ctx, q, c.WorkspaceID, c.ContactID)
	if err != nil {
		return designRender{}, err
	}
	if dc.to != "" {
		values.ContactEmail = dc.to
	}
	inbox, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: c.WorkspaceID, ID: c.InboxID})
	if err != nil {
		return designRender{}, err
	}
	values.InboxName = inbox.Name
	version, err := designVersion(d, dc.locale, contactLocale)
	if err != nil {
		return designRender{}, err
	}
	doc := d
	if version != "" {
		doc = d.Localized(version)
	}
	if is := doc.FieldIssues(values, design.ConversationFields); len(is) > 0 {
		return designRender{}, issuesProblem(is)
	}
	sc := renderScope{inbox: &inbox, values: &values, version: version, dark: dc.dark, message: dc.slot, footer: design.FooterReply}
	if !wrote {
		sc.footer = design.FooterOutreach
		sc.unsubscribe = s.linkBase() + managePathPrefix + "example"
		if !dc.preview {
			sc.unsubscribe = s.outreachURL(c)
		}
	}
	out, err := s.renderDesign(ctx, q, c.WorkspaceID, d, sc)
	if errors.Is(err, design.ErrHTMLTooLarge) {
		return designRender{}, errDesignHTMLTooLarge
	}
	if err != nil {
		return designRender{}, err
	}
	return designRender{row: row, locale: version, outreach: !wrote, out: out}, nil
}

func (s *Server) outreachToken(c store.Conversation) string {
	return s.sealToken(outreachTokenContext, []uuid.UUID{c.WorkspaceID, c.InboxID, c.ContactID, c.ID}, nil)
}

func (s *Server) outreachURL(c store.Conversation) string {
	return s.linkBase() + managePathPrefix + s.outreachToken(c)
}

func (s *Server) checkOutreach(ctx context.Context, q *store.Queries, c store.Conversation, to string) error {
	suppressed, err := q.IsEmailSuppressedAnyScope(ctx, store.IsEmailSuppressedAnyScopeParams{WorkspaceID: c.WorkspaceID, Email: to})
	if err != nil {
		return err
	}
	if suppressed {
		return errEmailOptedOut
	}
	if out, err := s.optedOut(ctx, q, c.WorkspaceID, to); err != nil || out {
		if err != nil {
			return err
		}
		return errEmailOptedOut
	}
	stopped, err := q.IsInboxMailOptedOut(ctx, store.IsInboxMailOptedOutParams{WorkspaceID: c.WorkspaceID, InboxID: c.InboxID, ContactID: c.ContactID})
	if err != nil {
		return err
	}
	if stopped {
		return errEmailOptedOut
	}
	if s.broadcastMode() != broadcastsApproval {
		return nil
	}
	w, err := q.GetWorkspace(ctx, c.WorkspaceID)
	if err != nil {
		return err
	}
	switch w.BroadcastsStatus {
	case wsApprovalRequired, wsApprovalRequested:
		return errBroadcastsApproval
	case wsSuspended:
		return errBroadcastsSuspended
	}
	return nil
}

func (s *Server) takeDesignedOutreach(ctx context.Context, q *store.Queries, p principal, ws uuid.UUID) error {
	day := s.now().UTC().Format(time.DateOnly)
	err := s.rateLimitTx(ctx, q,
		rateCheck{"designed_outreach:" + p.viewerID().String(), limitDesignedOutreach},
		rateCheck{"designed_outreach_ws:" + ws.String() + ":" + day, limitDesignedOutreachWS})
	var e *apiError
	if errors.As(err, &e) {
		e.Detail = "at most 20 designed messages to people who have not written go out per member or key an hour, 200 per workspace a day; " + e.Detail
	}
	return err
}

func senderIdentityMissing(r designRender) bool {
	return r.outreach && slices.ContainsFunc(r.out.Warnings, func(i design.Issue) bool { return i.Code == "sender_identity_missing" })
}

func messageSlot(m messageRow) *design.MessageSlot {
	cards := helpCardsOf(m.HelpCards)
	slot := &design.MessageSlot{Text: withoutHelpCards(m.Body, cards), CardsHTML: helpCardsHTML(cards), CardsText: helpCardLines(cards)}
	if m.SignatureHtml != nil && *m.SignatureHtml != "" {
		slot.SignatureHTML = *m.SignatureHtml
		if m.SignatureText != nil {
			slot.SignatureText = *m.SignatureText
		}
	}
	return slot
}

func (s *Server) checkDraftDesign(ctx context.Context, q *store.Queries, c store.Conversation, id uuid.UUID, locale *string, to string, m messageRow) error {
	emailed, err := isEmailConversation(ctx, q, c)
	if err != nil {
		return err
	}
	if !emailed {
		return errDesignNotEmailed
	}
	if to == "" {
		if to, err = contactAddress(ctx, q, c); err != nil {
			return err
		}
	}
	r, err := s.renderForConversation(ctx, q, c, designCall{id: id, locale: locale, to: to, slot: messageSlot(m), preview: true})
	if err != nil {
		return err
	}
	if senderIdentityMissing(r) {
		return errSenderIdentityMissing
	}
	if r.outreach && to != "" {
		return s.checkOutreach(ctx, q, c, to)
	}
	return nil
}

func contactAddress(ctx context.Context, q *store.Queries, c store.Conversation) (string, error) {
	to, err := q.ContactReplyAddress(ctx, store.ContactReplyAddressParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID})
	if err == nil {
		return to, nil
	}
	if !store.IsNotFound(err) {
		return "", err
	}
	emails, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: c.WorkspaceID, ContactIds: []uuid.UUID{c.ContactID}})
	if err != nil || len(emails) == 0 {
		return "", err
	}
	return emails[0].Email, nil
}

func (s *Server) designMessage(ctx context.Context, q *store.Queries, p principal, c store.Conversation, plan *emailPlan, msg messageRow) error {
	if plan == nil {
		return errDesignNotEmailed
	}
	if len(plan.toAll) > 1 || (len(plan.toAll) > 0 && len(plan.cc) > 0) {
		return errDesignOneRecipient
	}
	r, err := s.renderForConversation(ctx, q, c, designCall{id: *msg.EmailDesignID, locale: msg.EmailDesignLocale, to: plan.to, slot: messageSlot(msg)})
	if err != nil {
		return err
	}
	if senderIdentityMissing(r) {
		return errSenderIdentityMissing
	}
	if r.outreach {
		if err := s.checkOutreach(ctx, q, c, plan.to); err != nil {
			return err
		}
		if err := s.takeDesignedOutreach(ctx, q, p, c.WorkspaceID); err != nil {
			return err
		}
	}
	if err := markDesignImagesSent(ctx, q, c.WorkspaceID, r.row, s.now()); err != nil {
		return err
	}
	plan.design = &designedMail{
		id: r.row.ID, version: r.row.Version, locale: r.locale, name: r.row.Name, outreach: r.outreach,
		html: r.out.HTML, text: r.out.Text,
	}
	return nil
}

func markDesignImagesSent(ctx context.Context, q *store.Queries, ws uuid.UUID, row store.EmailDesign, now time.Time) error {
	d, _, err := design.Parse(row.Document)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for _, id := range d.ImageIDs() {
		if u, err := uuid.Parse(id); err == nil {
			ids = append(ids, u)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return q.MarkEmailImagesSent(ctx, store.MarkEmailImagesSentParams{Now: now, WorkspaceID: ws, Ids: ids})
}

func messageDesign(m messageRow) *oas.MessageDesign {
	if m.EmailDesignID == nil {
		return nil
	}
	out := &oas.MessageDesign{Id: m.EmailDesignID, Name: m.EmailDesignName}
	if m.EmailDesignLocale != nil {
		out.Locale = new(oas.LanguageTag(*m.EmailDesignLocale))
	}
	return out
}

func emailDesign(e *store.ListMessageEmailsRow) *oas.MessageDesign {
	if e == nil || e.DesignVersion == nil {
		return nil
	}
	out := &oas.MessageDesign{Id: e.DesignID, Name: e.DesignName, Version: e.DesignVersion}
	if e.DesignLocale != nil && *e.DesignLocale != "" {
		out.Locale = new(oas.LanguageTag(*e.DesignLocale))
	}
	return out
}

func (s *Server) PreviewConversationDesign(ctx context.Context, req oas.PreviewConversationDesignRequestObject) (oas.PreviewConversationDesignResponseObject, error) {
	p := principalFrom(ctx)
	if req.Body == nil {
		return nil, errValidation("send a JSON body")
	}
	b := req.Body
	q := s.st.Queries
	c, err := visibleConversation(ctx, q, p, req.ConversationId, false)
	if err != nil {
		return nil, err
	}
	if err := s.rateLimit(ctx, rateCheck{"render:" + p.viewerID().String(), limitRender}); err != nil {
		return nil, err
	}
	var locale *string
	if b.DesignLocale != nil {
		tag, err := languageTag(string(*b.DesignLocale), "design_locale")
		if err != nil {
			return nil, err
		}
		locale = &tag
	}
	emailed, err := isEmailConversation(ctx, q, c)
	if err != nil {
		return nil, err
	}
	if !emailed {
		return nil, errDesignNotEmailed
	}
	var slot *design.MessageSlot
	if b.Body != nil && strings.TrimSpace(*b.Body) != "" {
		if len([]rune(*b.Body)) > maxMessageBodyRunes {
			return nil, errValidation("body must be at most 65536 characters")
		}
		slot = &design.MessageSlot{Text: *b.Body}
	}
	if !p.isKey() && !p.deliversAsBot() {
		sig, err := s.messageSignature(ctx, q, p, c.InboxID)
		if err != nil {
			return nil, err
		}
		if sig.html != "" {
			if slot == nil {
				slot = &design.MessageSlot{}
			}
			slot.SignatureHTML, slot.SignatureText = sig.html, sig.text
		}
	}
	to, err := contactAddress(ctx, q, c)
	if err != nil {
		return nil, err
	}
	r, err := s.renderForConversation(ctx, q, c, designCall{id: b.DesignId, locale: locale, to: to, slot: slot, dark: boolOr(b.Dark, false), preview: true})
	if err != nil {
		return nil, err
	}
	subject := email.ReplySubject(c.Subject)
	if composed, err := q.ComposedDraftSubject(ctx, store.ComposedDraftSubjectParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID}); err == nil {
		subject = composed
	} else if !store.IsNotFound(err) {
		return nil, err
	}
	res := oas.PreviewConversationDesign200JSONResponse{
		Html: r.out.HTML, Text: r.out.Text, SizeBytes: int64(r.out.Size), Warnings: issueBodies(r.out.Warnings),
		Subject: subject, Footer: oas.DesignFooterReply,
	}
	if r.outreach {
		res.Footer = oas.DesignFooterOutreach
	}
	if r.locale != "" {
		res.Locale = new(oas.LanguageTag(r.locale))
	}
	return res, nil
}

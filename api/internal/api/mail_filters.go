package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxMailFilterValue = 200

var errRuleNoAccess = problem(http.StatusConflict, "no_access", "a member the filter stars for, assigns or notifies does not read the inbox")

var mailFilterFields = map[oas.AutomationConditionField]bool{
	oas.AutomationFieldFrom: true, oas.AutomationFieldTo: true, oas.AutomationFieldCc: true,
	oas.AutomationFieldHasAttachment: true, oas.AutomationFieldListId: true,
}

var mailOnlyActions = map[oas.AutomationActionType]bool{
	oas.AutomationActionStar: true, oas.AutomationActionArchive: true, oas.AutomationActionTrash: true, oas.AutomationActionForwardToMember: true,
}

var personalFilterActions = map[oas.AutomationActionType]bool{
	oas.AutomationActionAddLabels: true, oas.AutomationActionRemoveLabels: true, oas.AutomationActionStar: true,
	oas.AutomationActionArchive: true, oas.AutomationActionTrash: true, oas.AutomationActionSnooze: true,
	oas.AutomationActionForwardToMember: true, oas.AutomationActionSetStatus: true, oas.AutomationActionSetPriority: true,
	oas.AutomationActionAssignMember: true, oas.AutomationActionUnassign: true,
}

// isPersonal is the one place that tells a personal mailbox; one whose owner is gone stays personal.
func isPersonal(in store.Inbox) bool { return in.Personal }

// managesMailbox: a personal mailbox's filters are its owner's alone, by session or their own grant;
// delegates read the mailbox but do not manage them, and an orphaned mailbox's are nobody's.
func managesMailbox(p principal, in store.Inbox) bool {
	return isPersonal(in) && in.OwnerMemberID != nil && !p.isKey() && p.rule == nil && !p.auto &&
		p.memberID != uuid.Nil() && p.memberID == *in.OwnerMemberID
}

func inboundMail(e pendingEvent) bool {
	var m struct {
		Email *struct{} `json:"email"`
	}
	return json.Unmarshal(e.data, &m) == nil && m.Email != nil
}

func checkMailCondition(c *oas.AutomationCondition, at string) error {
	if c.Field == oas.AutomationFieldHasAttachment {
		if *c.Value != "true" && *c.Value != "false" {
			return errValidation(at + ".value must be true or false")
		}
		return nil
	}
	v, err := trimmed(*c.Value, minAddressValue, maxMailFilterValue, at+".value")
	if err != nil {
		return err
	}
	address := c.Field == oas.AutomationFieldFrom || c.Field == oas.AutomationFieldTo || c.Field == oas.AutomationFieldCc
	if address && !validAddressValue(v) {
		return errValidation(at + ".value of " + string(c.Field) + " has no spaces or commas")
	}
	c.Value = &v
	return nil
}

func mailMatches(m store.MailFactsOfRow, cond oas.AutomationCondition) bool {
	var got string
	switch cond.Field {
	case oas.AutomationFieldHasAttachment:
		return m.HasFile == (*cond.Value == "true")
	case oas.AutomationFieldFrom:
		got = m.FromAddress
	case oas.AutomationFieldTo:
		got = strings.Join(m.ToAddresses, ", ")
	case oas.AutomationFieldCc:
		got = strings.Join(m.CcAddresses, ", ")
	case oas.AutomationFieldListId:
		got = m.ListID
	}
	found := strings.Contains(strings.ToLower(got), strings.ToLower(*cond.Value))
	return found == (cond.Operator == oas.AutomationOpContains)
}

type MailForwardedArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	MemberID    uuid.UUID `json:"member_id"`
	MessageID   uuid.UUID `json:"message_id"`
	Rule        string    `json:"rule"`
}

func (MailForwardedArgs) Kind() string { return "mail_forwarded_notify" }

type mailForwardedWorker struct {
	river.WorkerDefaults[MailForwardedArgs]
	s *Server
}

func (w *mailForwardedWorker) Work(ctx context.Context, job *river.Job[MailForwardedArgs]) error {
	return w.s.NotifyMailForwarded(ctx, job.Args)
}

type mailForwardedData struct {
	Inbox, Contact, Subject, Rule, Preview, URL, SettingsURL string
}

func (s *Server) NotifyMailForwarded(ctx context.Context, a MailForwardedArgs) error {
	ws := a.WorkspaceID
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return err
	}
	msg, err := s.st.GetMessage(ctx, store.GetMessageParams{WorkspaceID: ws, ID: a.MessageID})
	if store.IsNotFound(err) || (err == nil && msg.RedactedAt != nil) {
		return nil
	}
	if err != nil {
		return err
	}
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: msg.ConversationID})
	if store.IsNotFound(err) || (err == nil && (c.Spam || c.TrashedAt != nil)) {
		return nil
	}
	if err != nil {
		return err
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: c.InboxID})
	if err != nil {
		return err
	}
	if ok, err := memberHasInbox(ctx, s.st.Queries, ws, in.ID, a.MemberID); err != nil || !ok {
		return err
	}
	r, err := s.st.GetNotificationMember(ctx, store.GetNotificationMemberParams{WorkspaceID: ws, ID: a.MemberID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	overrides, err := s.st.ListInboxNotificationsOfInbox(ctx, store.ListInboxNotificationsOfInboxParams{WorkspaceID: ws, InboxID: in.ID})
	if err != nil {
		return err
	}
	events := applyNotificationEvents(roleNotificationDefaults(r.Role), parseNotificationUpdate(r.NotificationEvents))
	for _, o := range overrides {
		if o.MemberID == r.ID {
			events = applyNotificationEvents(events, parseNotificationUpdate(o.Events))
		}
	}
	ch := notificationChannelsFor(events, oas.MailForwarded)
	if !ch.Push && !ch.Email {
		return nil
	}
	contact, err := s.contactDisplayName(ctx, ws, c.ContactID)
	if err != nil {
		return err
	}
	data := mailForwardedData{
		Inbox: in.Name, Contact: contact, Subject: c.Subject, Rule: a.Rule,
		URL:         s.auth.PublicURL + conversationPath(ws, c.ID),
		SettingsURL: s.auth.PublicURL + "/settings/notifications?workspace_id=" + ws.String(),
	}
	if ch.Push && s.push != nil {
		subs, err := s.st.ListLivePushSubscriptionIDs(ctx, store.ListLivePushSubscriptionIDsParams{PersonID: r.PersonID, Now: s.now(), WorkspaceID: &ws})
		if err != nil {
			return err
		}
		if len(subs) > 0 {
			pd := data
			pd.Preview = excerpt(msg.Body, pushPreviewRunes)
			m, err := mail.Render("push_mail_forwarded", r.Locale, pd)
			if err != nil {
				return err
			}
			conv, inbox := c.ID, in.ID
			payload := oas.PushNotification{
				Event: oas.MailForwarded, Title: m.Subject, Body: strings.TrimSpace(m.Text), Url: conversationPath(ws, c.ID),
				Tag: "conversation-" + c.ID.String(), ConversationId: &conv, WorkspaceId: &ws, InboxId: &inbox,
			}
			jobs := make([]river.InsertManyParams, 0, len(subs))
			for _, id := range subs {
				args := s.pushArgs(&ws, id, payload, in, c)
				args.MessageID = &msg.ID
				jobs = append(jobs, river.InsertManyParams{Args: args, InsertOpts: pushOpts()})
			}
			if _, err := s.jobs.InsertMany(ctx, jobs); err != nil {
				return err
			}
		}
	}
	if ch.Email {
		data.Preview = excerpt(msg.Body, emailPreviewRunes)
		m, err := mail.Render("mail_forwarded", r.Locale, data)
		if err != nil {
			return err
		}
		m.To = r.Email
		if err := s.mailer.Send(ctx, m); err != nil {
			s.log.WarnContext(ctx, "mail forwarded e-mail", slog.Any("error", err))
		}
	}
	return nil
}

func (s *Server) GetMessageFilterSuggestion(ctx context.Context, req oas.GetMessageFilterSuggestionRequestObject) (oas.GetMessageFilterSuggestionResponseObject, error) {
	p := principalFrom(ctx)
	me, err := s.visibleMessageEmail(ctx, p, req.MessageId)
	if err != nil {
		return nil, err
	}
	facts, err := s.st.MailFactsOf(ctx, store.MailFactsOfParams{WorkspaceID: p.workspaceID, ConversationID: me.ConversationID, ID: me.MessageID})
	if store.IsNotFound(err) {
		return nil, errMessageGone
	}
	if err != nil {
		return nil, err
	}
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: p.workspaceID, ID: me.ConversationID})
	if err != nil {
		return nil, err
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: p.workspaceID, ID: c.InboxID})
	if err != nil {
		return nil, err
	}
	if in.Purpose != string(oas.InboxPurposeMail) {
		return nil, errMessageGone
	}
	out := oas.MailFilterSuggestion{InboxId: in.ID, Trigger: oas.AutomationTriggerMailReceived, Conditions: []oas.AutomationCondition{}, Actions: []oas.AutomationAction{}}
	add := func(field oas.AutomationConditionField, value string) bool {
		value = strings.TrimSpace(value)
		if n := utf8.RuneCountInString(value); n < minAddressValue || n > maxMailFilterValue {
			return false
		}
		out.Conditions = append(out.Conditions, oas.AutomationCondition{Field: field, Operator: oas.AutomationOpContains, Value: &value})
		return true
	}
	out.Name = me.FromAddress
	add(oas.AutomationFieldFrom, me.FromAddress)
	if add(oas.AutomationFieldListId, facts.ListID) {
		out.Name = facts.ListID
	} else {
		add(oas.AutomationFieldSubject, me.Subject)
	}
	out.Name = truncateRunes(out.Name, 100)
	return oas.GetMessageFilterSuggestion200JSONResponse(out), nil
}

// ruleMailbox: a personal mailbox's filter acts as the system for that mailbox and nothing else.
func ruleMailbox(p principal, in store.Inbox) principal {
	if isPersonal(in) {
		p.mailbox = in.ID
	}
	return p
}

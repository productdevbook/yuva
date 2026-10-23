package api

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

type SLANotifyArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	Target         string    `json:"target"`
	DueAt          time.Time `json:"due_at"`
	Breached       bool      `json:"breached,omitempty"`
}

func (SLANotifyArgs) Kind() string { return "sla_notify" }

type slaNotifyWorker struct {
	river.WorkerDefaults[SLANotifyArgs]
	s *Server
}

func (w *slaNotifyWorker) Work(ctx context.Context, job *river.Job[SLANotifyArgs]) error {
	return w.s.NotifySLA(ctx, job.Args)
}

type slaMailData struct {
	Inbox, Contact, Subject, Target, Due, URL, SettingsURL string
}

// slaRecipients are the conversation's assignee or, without one, the members of its team; the
// second result tells whether the conversation is theirs as an assignee.
func slaRecipients(ctx context.Context, q *store.Queries, c store.Conversation) ([]uuid.UUID, bool, error) {
	if c.AssigneeID != nil {
		return []uuid.UUID{*c.AssigneeID}, true, nil
	}
	if c.TeamID == nil {
		return nil, false, nil
	}
	rows, err := q.ListTeamMemberIDs(ctx, store.ListTeamMemberIDsParams{WorkspaceID: c.WorkspaceID, TeamIds: []uuid.UUID{*c.TeamID}})
	if err != nil {
		return nil, false, err
	}
	out := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		out[i] = r.MemberID
	}
	return out, false, nil
}

// NotifySLA sends the pushes and the e-mail of an SLA warning or breach to the members it
// concerns who still have access to the inbox and want it.
func (s *Server) NotifySLA(ctx context.Context, a SLANotifyArgs) error {
	ws := a.WorkspaceID
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return err
	}
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: a.ConversationID})
	if store.IsNotFound(err) || (err == nil && (c.Spam || c.TrashedAt != nil)) {
		return nil
	}
	if err != nil {
		return err
	}
	who, mine, err := slaRecipients(ctx, s.st.Queries, c)
	if err != nil || len(who) == 0 {
		return err
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: c.InboxID})
	if err != nil {
		return err
	}
	candidates, err := s.st.ListNotificationCandidates(ctx, store.ListNotificationCandidatesParams{WorkspaceID: ws, InboxID: in.ID})
	if err != nil {
		return err
	}
	overrides, err := s.st.ListInboxNotificationsOfInbox(ctx, store.ListInboxNotificationsOfInboxParams{WorkspaceID: ws, InboxID: in.ID})
	if err != nil {
		return err
	}
	perMember := make(map[uuid.UUID]oas.NotificationEventsUpdate, len(overrides))
	for _, o := range overrides {
		perMember[o.MemberID] = parseNotificationUpdate(o.Events)
	}
	now := s.now()
	viewing, err := s.st.ListViewingMembers(ctx, store.ListViewingMembersParams{WorkspaceID: ws, ConversationID: &c.ID, FreshAfter: now.Add(-presenceFresh)})
	if err != nil {
		return err
	}
	contact, err := s.contactDisplayName(ctx, ws, c.ContactID)
	if err != nil {
		return err
	}
	ev, tmpl := oas.SlaWarning, "push_sla_warning"
	if a.Breached {
		ev, tmpl = oas.SlaBreached, "push_sla_breached"
	}
	loc, err := time.LoadLocation(in.Timezone)
	if err != nil {
		loc = time.UTC
	}
	data := slaMailData{
		Inbox: in.Name, Contact: contact, Subject: c.Subject, Target: a.Target, Due: a.DueAt.In(loc).Format("2006-01-02 15:04 MST"),
		URL:         s.auth.PublicURL + conversationPath(ws, c.ID),
		SettingsURL: s.auth.PublicURL + "/settings/notifications?workspace_id=" + ws.String(),
	}
	var jobs []river.InsertManyParams
	var mails []mail.Message
	for _, r := range candidates {
		if !containsID(who, r.ID) || containsID(viewing, r.ID) || (r.Availability == "away" && !mine) {
			continue
		}
		events := applyNotificationEvents(roleNotificationDefaults(r.Role), parseNotificationUpdate(r.NotificationEvents))
		ch := notificationChannelsFor(applyNotificationEvents(events, perMember[r.ID]), ev)
		if ch.Push && s.push != nil {
			subs, err := s.st.ListLivePushSubscriptionIDs(ctx, store.ListLivePushSubscriptionIDsParams{PersonID: r.PersonID, Now: now})
			if err != nil {
				return err
			}
			if len(subs) > 0 {
				msg, err := mail.Render(tmpl, r.Locale, data)
				if err != nil {
					return err
				}
				conv, inbox := c.ID, in.ID
				payload := oas.PushNotification{
					Event: ev, Title: msg.Subject, Body: strings.TrimSpace(msg.Text), Url: conversationPath(ws, c.ID),
					Tag: "conversation-" + c.ID.String(), ConversationId: &conv, WorkspaceId: &ws, InboxId: &inbox,
				}
				for _, id := range subs {
					jobs = append(jobs, river.InsertManyParams{Args: s.pushArgs(&ws, id, payload, in, c), InsertOpts: pushOpts()})
				}
			}
		}
		if a.Breached && ch.Email {
			msg, err := mail.Render("sla_breached", r.Locale, data)
			if err != nil {
				return err
			}
			msg.To = r.Email
			mails = append(mails, msg)
		}
	}
	if len(jobs) > 0 {
		if _, err := s.jobs.InsertMany(ctx, jobs); err != nil {
			return err
		}
	}
	for _, m := range mails {
		if err := s.mailer.Send(ctx, m); err != nil {
			s.log.WarnContext(ctx, "sla e-mail", slog.Any("error", err))
		}
	}
	return nil
}

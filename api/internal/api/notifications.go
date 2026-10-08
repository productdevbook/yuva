package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	defaultNotificationEmailDelay = 15
	minNotificationEmailDelay     = 5
	maxNotificationEmailDelay     = 1440
	notificationEmailPeriod       = time.Hour
	notificationEmailMessages     = 5
	pushPreviewRunes              = 140
	emailPreviewRunes             = 300
	liveNotificationTTL           = 4 * time.Hour
	asyncNotificationTTL          = 24 * time.Hour
)

type notificationEvent = oas.PushNotificationEvent

func roleNotificationDefaults(role string) oas.NotificationEvents {
	manager := role == roleOwner || role == roleAdmin
	return oas.NotificationEvents{
		NewLiveConversation:             oas.NotificationChannels{Push: true},
		NewAsyncConversation:            oas.NotificationChannels{Push: manager},
		MessageInMyConversation:         oas.NotificationChannels{Push: true, Email: true},
		MessageInUnassignedConversation: oas.NotificationChannels{Push: manager},
		AssignedToMe:                    oas.NotificationChannels{Push: true, Email: true},
		Mentioned:                       oas.NotificationChannels{Push: true, Email: true},
	}
}

func applyNotificationEvents(base oas.NotificationEvents, u oas.NotificationEventsUpdate) oas.NotificationEvents {
	set := func(dst *oas.NotificationChannels, v *oas.NotificationChannels) {
		if v != nil {
			*dst = *v
		}
	}
	set(&base.NewLiveConversation, u.NewLiveConversation)
	set(&base.NewAsyncConversation, u.NewAsyncConversation)
	set(&base.MessageInMyConversation, u.MessageInMyConversation)
	set(&base.MessageInUnassignedConversation, u.MessageInUnassignedConversation)
	set(&base.AssignedToMe, u.AssignedToMe)
	set(&base.Mentioned, u.Mentioned)
	return base
}

func mergeNotificationUpdates(base, u oas.NotificationEventsUpdate) oas.NotificationEventsUpdate {
	pick := func(a, b *oas.NotificationChannels) *oas.NotificationChannels {
		if b != nil {
			return b
		}
		return a
	}
	return oas.NotificationEventsUpdate{
		NewLiveConversation:             pick(base.NewLiveConversation, u.NewLiveConversation),
		NewAsyncConversation:            pick(base.NewAsyncConversation, u.NewAsyncConversation),
		MessageInMyConversation:         pick(base.MessageInMyConversation, u.MessageInMyConversation),
		MessageInUnassignedConversation: pick(base.MessageInUnassignedConversation, u.MessageInUnassignedConversation),
		AssignedToMe:                    pick(base.AssignedToMe, u.AssignedToMe),
		Mentioned:                       pick(base.Mentioned, u.Mentioned),
	}
}

func emptyNotificationUpdate(u oas.NotificationEventsUpdate) bool {
	return u.NewLiveConversation == nil && u.NewAsyncConversation == nil && u.MessageInMyConversation == nil &&
		u.MessageInUnassignedConversation == nil && u.AssignedToMe == nil && u.Mentioned == nil
}

func notificationChannelsFor(e oas.NotificationEvents, ev notificationEvent) oas.NotificationChannels {
	switch ev {
	case oas.NewLiveConversation:
		return e.NewLiveConversation
	case oas.NewAsyncConversation:
		return e.NewAsyncConversation
	case oas.MessageInMyConversation:
		return e.MessageInMyConversation
	case oas.MessageInUnassignedConversation:
		return e.MessageInUnassignedConversation
	case oas.AssignedToMe:
		return e.AssignedToMe
	case oas.Mentioned:
		return e.Mentioned
	}
	return oas.NotificationChannels{}
}

func parseNotificationUpdate(raw []byte) oas.NotificationEventsUpdate {
	var u oas.NotificationEventsUpdate
	_ = json.Unmarshal(raw, &u)
	return u
}

func emailDelayMinutes(v *int32) int {
	if v == nil {
		return defaultNotificationEmailDelay
	}
	return int(*v)
}

func (s *Server) notificationSettings(ctx context.Context, p principal) (oas.NotificationSettings, error) {
	m, err := s.st.GetMemberByPerson(ctx, store.GetMemberByPersonParams{WorkspaceID: p.workspaceID, PersonID: p.personID})
	if err != nil {
		return oas.NotificationSettings{}, err
	}
	defaults := roleNotificationDefaults(m.Role)
	rows, err := s.st.ListInboxNotifications(ctx, store.ListInboxNotificationsParams{
		WorkspaceID: p.workspaceID, MemberID: m.ID, AllInboxes: p.seesAllInboxes(),
	})
	if err != nil {
		return oas.NotificationSettings{}, err
	}
	out := oas.NotificationSettings{
		Events:            applyNotificationEvents(defaults, parseNotificationUpdate(m.NotificationEvents)),
		Defaults:          defaults,
		EmailDelayMinutes: emailDelayMinutes(m.NotificationEmailDelay),
		Inboxes:           make([]oas.InboxNotifications, len(rows)),
	}
	for i, r := range rows {
		out.Inboxes[i] = inboxNotificationsBody(r)
	}
	return out, nil
}

func inboxNotificationsBody(r store.InboxNotification) oas.InboxNotifications {
	return oas.InboxNotifications{InboxId: r.InboxID, Events: parseNotificationUpdate(r.Events), UpdatedAt: r.UpdatedAt}
}

func (s *Server) GetNotificationSettings(ctx context.Context, _ oas.GetNotificationSettingsRequestObject) (oas.GetNotificationSettingsResponseObject, error) {
	out, err := s.notificationSettings(ctx, principalFrom(ctx))
	if err != nil {
		return nil, err
	}
	return oas.GetNotificationSettings200JSONResponse(out), nil
}

func (s *Server) UpdateNotificationSettings(ctx context.Context, req oas.UpdateNotificationSettingsRequestObject) (oas.UpdateNotificationSettingsResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	if b.Events == nil && b.EmailDelayMinutes == nil {
		return nil, errValidation("send events or email_delay_minutes")
	}
	if d := b.EmailDelayMinutes; d != nil && (*d < minNotificationEmailDelay || *d > maxNotificationEmailDelay) {
		return nil, errValidation("email_delay_minutes must be 5 to 1440")
	}
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		m, err := q.GetMemberByPerson(ctx, store.GetMemberByPersonParams{WorkspaceID: p.workspaceID, PersonID: p.personID})
		if err != nil {
			return err
		}
		events := parseNotificationUpdate(m.NotificationEvents)
		if b.Events != nil {
			events = mergeNotificationUpdates(events, *b.Events)
		}
		delay := m.NotificationEmailDelay
		if b.EmailDelayMinutes != nil {
			delay = new(int32(*b.EmailDelayMinutes))
		}
		return q.SetMemberNotifications(ctx, store.SetMemberNotificationsParams{
			WorkspaceID: p.workspaceID, ID: m.ID, Events: mustJSON(events), EmailDelay: delay,
		})
	})
	if err != nil {
		return nil, err
	}
	out, err := s.notificationSettings(ctx, p)
	if err != nil {
		return nil, err
	}
	return oas.UpdateNotificationSettings200JSONResponse(out), nil
}

func (s *Server) SetInboxNotifications(ctx context.Context, req oas.SetInboxNotificationsRequestObject) (oas.SetInboxNotificationsResponseObject, error) {
	p := principalFrom(ctx)
	if emptyNotificationUpdate(req.Body.Events) {
		return nil, errValidation("events must name at least one event; DELETE removes the override")
	}
	if _, err := visibleInbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	}
	r, err := s.st.UpsertInboxNotifications(ctx, store.UpsertInboxNotificationsParams{
		WorkspaceID: p.workspaceID, MemberID: p.memberID, InboxID: req.InboxId, Events: mustJSON(req.Body.Events), Now: s.now(),
	})
	if err != nil {
		return nil, err
	}
	return oas.SetInboxNotifications200JSONResponse(inboxNotificationsBody(r)), nil
}

func (s *Server) DeleteInboxNotifications(ctx context.Context, req oas.DeleteInboxNotificationsRequestObject) (oas.DeleteInboxNotificationsResponseObject, error) {
	p := principalFrom(ctx)
	if _, err := visibleInbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	}
	n, err := s.st.DeleteInboxNotifications(ctx, store.DeleteInboxNotificationsParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, InboxID: req.InboxId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errNotFound
	}
	return oas.DeleteInboxNotifications204Response{}, nil
}

type NotifyArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	MessageID   uuid.UUID `json:"message_id"`
}

func (NotifyArgs) Kind() string { return "notify" }

type notifyWorker struct {
	river.WorkerDefaults[NotifyArgs]
	s *Server
}

func (w *notifyWorker) Work(ctx context.Context, job *river.Job[NotifyArgs]) error {
	return w.s.Notify(ctx, job.Args)
}

// notifiable tells whether a new message may notify members: a contact's message, an
// assignment, or a note that mentions members.
func notifiable(e pendingEvent) bool {
	if e.typ != realtime.MessageCreated {
		return false
	}
	var m struct {
		Kind   oas.MessageKind `json:"kind"`
		Author struct {
			Type oas.AuthorType `json:"type"`
		} `json:"author"`
		Event *struct {
			Type oas.EventType `json:"type"`
		} `json:"event"`
		Mentions []uuid.UUID `json:"mentions"`
	}
	if json.Unmarshal(e.data, &m) != nil {
		return false
	}
	switch m.Kind {
	case oas.MessageKindMessage:
		return m.Author.Type == oas.AuthorTypeContact
	case oas.MessageKindEvent:
		return m.Event != nil && m.Event.Type == oas.Assigned
	case oas.MessageKindNote:
		return len(m.Mentions) > 0
	}
	return false
}

func (s *Server) queueNotifications(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, items []pendingEvent) error {
	var jobs []river.InsertManyParams
	for _, e := range items {
		if !notifiable(e) {
			continue
		}
		var m struct {
			ID uuid.UUID `json:"id"`
		}
		if err := json.Unmarshal(e.data, &m); err != nil {
			return err
		}
		jobs = append(jobs, river.InsertManyParams{Args: NotifyArgs{WorkspaceID: workspaceID, MessageID: m.ID}})
	}
	if len(jobs) == 0 {
		return nil
	}
	_, err := s.jobs.InsertManyTx(ctx, tx, jobs)
	return err
}

// classify decides which notification a message causes and, for events about particular members,
// who gets it.
func (s *Server) classify(ctx context.Context, ws uuid.UUID, m store.GetMessageRow, c store.Conversation, in store.Inbox) (notificationEvent, []uuid.UUID, error) {
	switch {
	case m.Kind == string(oas.MessageKindMessage) && m.AuthorType == string(oas.AuthorTypeContact):
		first, err := s.st.GetFirstPublicMessage(ctx, store.GetFirstPublicMessageParams{WorkspaceID: ws, ConversationID: c.ID})
		if err != nil {
			return "", nil, err
		}
		switch {
		case first == m.ID && in.Mode == string(oas.Live):
			return oas.NewLiveConversation, nil, nil
		case first == m.ID:
			return oas.NewAsyncConversation, nil, nil
		case c.AssigneeID == nil:
			return oas.MessageInUnassignedConversation, nil, nil
		default:
			return oas.MessageInMyConversation, []uuid.UUID{*c.AssigneeID}, nil
		}
	case m.Kind == string(oas.MessageKindNote) && len(m.Mentions) > 0:
		return oas.Mentioned, m.Mentions, nil
	case m.Kind == string(oas.MessageKindEvent):
		var ev oas.MessageEvent
		if json.Unmarshal(m.Event, &ev) != nil || ev.Type != oas.Assigned || ev.AssigneeId == nil {
			return "", nil, nil
		}
		if m.AuthorMemberID != nil && *m.AuthorMemberID == *ev.AssigneeId {
			return "", nil, nil
		}
		if c.AssigneeID == nil || *c.AssigneeID != *ev.AssigneeId {
			return "", nil, nil
		}
		return oas.AssignedToMe, []uuid.UUID{*ev.AssigneeId}, nil
	}
	return "", nil, nil
}

func (s *Server) contactDisplayName(ctx context.Context, ws, id uuid.UUID) (string, error) {
	ct, err := s.st.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: id})
	if store.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if ct.Name != "" {
		return ct.Name, nil
	}
	emails, err := s.st.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: ws, ContactIds: []uuid.UUID{id}})
	if err != nil || len(emails) == 0 {
		return "", err
	}
	return emails[0].Email, nil
}

func (s *Server) memberDisplayName(ctx context.Context, ws uuid.UUID, id *uuid.UUID) (string, error) {
	if id == nil {
		return "", nil
	}
	m, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: ws, ID: *id})
	if store.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if m.Name != "" {
		return m.Name, nil
	}
	return m.Email, nil
}

func conversationPath(ws, conv uuid.UUID) string {
	return "/conversations/" + conv.String() + "?workspace_id=" + ws.String()
}

// Notify sends the pushes and schedules the e-mail fallbacks a new message causes.
func (s *Server) Notify(ctx context.Context, a NotifyArgs) error {
	ws := a.WorkspaceID
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return err
	}
	m, err := s.st.GetMessage(ctx, store.GetMessageParams{WorkspaceID: ws, ID: a.MessageID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: m.ConversationID})
	if store.IsNotFound(err) || (err == nil && c.Spam) {
		return nil
	}
	if err != nil {
		return err
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: c.InboxID})
	if err != nil {
		return err
	}
	ev, only, err := s.classify(ctx, ws, m, c, in)
	if err != nil || ev == "" {
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
	viewing, err := s.st.ListViewingMembers(ctx, store.ListViewingMembersParams{WorkspaceID: ws, ConversationID: &c.ID, FreshAfter: s.now().Add(-presenceFresh)})
	if err != nil {
		return err
	}
	contact, err := s.contactDisplayName(ctx, ws, c.ContactID)
	if err != nil {
		return err
	}
	actorName := ""
	if ev == oas.AssignedToMe || ev == oas.Mentioned {
		if actorName, err = s.memberDisplayName(ctx, ws, m.AuthorMemberID); err != nil {
			return err
		}
	}
	mine := ev == oas.MessageInMyConversation || ev == oas.AssignedToMe || ev == oas.Mentioned
	now := s.now()
	var jobs []river.InsertManyParams
	for _, r := range candidates {
		if only != nil && !containsID(only, r.ID) {
			continue
		}
		if m.AuthorMemberID != nil && *m.AuthorMemberID == r.ID {
			continue
		}
		if containsID(viewing, r.ID) {
			continue
		}
		if r.Availability == "away" && !mine {
			continue
		}
		events := applyNotificationEvents(roleNotificationDefaults(r.Role), parseNotificationUpdate(r.NotificationEvents))
		events = applyNotificationEvents(events, perMember[r.ID])
		ch := notificationChannelsFor(events, ev)
		if ch.Push && s.push != nil {
			subs, err := s.st.ListLivePushSubscriptionIDs(ctx, store.ListLivePushSubscriptionIDsParams{PersonID: r.PersonID, Now: now})
			if err != nil {
				return err
			}
			if len(subs) > 0 {
				payload, err := s.pushPayload(ev, r.Locale, ws, c, in, contact, actorName, m.Body)
				if err != nil {
					return err
				}
				for _, id := range subs {
					jobs = append(jobs, river.InsertManyParams{Args: s.pushArgs(&ws, id, payload, in, c), InsertOpts: pushOpts()})
				}
			}
		}
		if ch.Email {
			at := now.Add(time.Duration(emailDelayMinutes(r.NotificationEmailDelay)) * time.Minute)
			jobs = append(jobs, river.InsertManyParams{
				Args:       NotificationEmailArgs{WorkspaceID: ws, MemberID: r.ID, ConversationID: c.ID},
				InsertOpts: notificationEmailOpts(at),
			})
		}
	}
	if len(jobs) == 0 {
		return nil
	}
	_, err = s.jobs.InsertMany(ctx, jobs)
	return err
}

func containsID(ids []uuid.UUID, id uuid.UUID) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func (s *Server) pushPayload(ev notificationEvent, locale string, ws uuid.UUID, c store.Conversation, in store.Inbox, contact, actor, body string) (oas.PushNotification, error) {
	tmpl := "push_message"
	switch ev {
	case oas.AssignedToMe:
		tmpl = "push_assigned"
	case oas.Mentioned:
		tmpl = "push_mention"
	}
	msg, err := mail.Render(tmpl, locale, map[string]string{
		"Inbox": in.Name, "Contact": contact, "Actor": actor, "Preview": excerpt(body, pushPreviewRunes),
	})
	if err != nil {
		return oas.PushNotification{}, err
	}
	conv, inbox := c.ID, in.ID
	return oas.PushNotification{
		Event: ev, Title: msg.Subject, Body: strings.TrimSpace(msg.Text), Url: conversationPath(ws, c.ID),
		Tag: "conversation-" + c.ID.String(), ConversationId: &conv, WorkspaceId: &ws, InboxId: &inbox,
	}, nil
}

func (s *Server) pushArgs(ws *uuid.UUID, sub uuid.UUID, p oas.PushNotification, in store.Inbox, c store.Conversation) PushArgs {
	ttl, urgency := asyncNotificationTTL, "normal"
	if in.Mode == string(oas.Live) {
		ttl, urgency = liveNotificationTTL, "high"
	}
	return PushArgs{
		WorkspaceID: ws, SubscriptionID: sub, Payload: mustJSON(p), TTL: int(ttl / time.Second), Urgency: urgency,
		Topic: strings.ReplaceAll(c.ID.String(), "-", ""),
	}
}

type NotificationEmailArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	MemberID       uuid.UUID `json:"member_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
}

func (NotificationEmailArgs) Kind() string { return "notification_email" }

type notificationEmailWorker struct {
	river.WorkerDefaults[NotificationEmailArgs]
	s *Server
}

func (w *notificationEmailWorker) Work(ctx context.Context, job *river.Job[NotificationEmailArgs]) error {
	next, err := w.s.SendNotificationEmail(ctx, job.Args)
	if err != nil || next == nil {
		return err
	}
	_, err = w.s.jobs.Insert(ctx, job.Args, &river.InsertOpts{ScheduledAt: *next})
	return err
}

// notificationEmailOpts keeps one pending e-mail check per member and conversation; finished
// checks do not block the next one.
func notificationEmailOpts(at time.Time) *river.InsertOpts {
	return &river.InsertOpts{ScheduledAt: at, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
		rivertype.JobStateRetryable, rivertype.JobStateScheduled,
	}}}
}

type notificationMention struct {
	By   string
	Text string
}

type notificationEmailData struct {
	Inbox       string
	Contact     string
	Subject     string
	Messages    []string
	More        int
	Mentions    []notificationMention
	Assigned    bool
	AssignedBy  string
	URL         string
	SettingsURL string
	Minutes     int
}

// SendNotificationEmail e-mails a member the conversation's contact messages (and an assignment)
// they have not read once the oldest is older than their e-mail delay, at most once per
// conversation per hour. It returns when to check again, or nil.
func (s *Server) SendNotificationEmail(ctx context.Context, a NotificationEmailArgs) (*time.Time, error) {
	ws, now := a.WorkspaceID, s.now()
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return nil, err
	}
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: a.ConversationID})
	if store.IsNotFound(err) || (err == nil && c.Spam) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	candidates, err := s.st.ListNotificationCandidates(ctx, store.ListNotificationCandidatesParams{WorkspaceID: ws, InboxID: c.InboxID})
	if err != nil {
		return nil, err
	}
	var r *store.ListNotificationCandidatesRow
	for i := range candidates {
		if candidates[i].ID == a.MemberID {
			r = &candidates[i]
		}
	}
	if r == nil {
		return nil, nil
	}
	assigned := c.AssigneeID != nil && *c.AssigneeID == r.ID
	mentionsOnly := r.Availability == "away" && !assigned
	viewing, err := s.st.ListViewingMembers(ctx, store.ListViewingMembersParams{WorkspaceID: ws, ConversationID: &c.ID, FreshAfter: now.Add(-presenceFresh)})
	if err != nil || containsID(viewing, r.ID) {
		return nil, err
	}
	var after time.Time
	read, err := s.st.GetConversationRead(ctx, store.GetConversationReadParams{WorkspaceID: ws, MemberID: r.ID, ConversationID: c.ID})
	if err == nil {
		after = read.LastReadAt
	} else if !store.IsNotFound(err) {
		return nil, err
	}
	last, err := s.st.GetNotificationEmail(ctx, store.GetNotificationEmailParams{WorkspaceID: ws, MemberID: r.ID, ConversationID: c.ID})
	if err == nil {
		after = later(after, &last.Through)
	} else if !store.IsNotFound(err) {
		return nil, err
	}
	items, err := s.st.ListNotifiableMessages(ctx, store.ListNotifiableMessagesParams{WorkspaceID: ws, ConversationID: c.ID, After: after, MemberID: r.ID})
	if err != nil {
		return nil, err
	}
	if mentionsOnly {
		items = slices.DeleteFunc(items, func(it store.ListNotifiableMessagesRow) bool { return it.Kind != string(oas.MessageKindNote) })
	}
	if len(items) == 0 {
		return nil, nil
	}
	delay := time.Duration(emailDelayMinutes(r.NotificationEmailDelay)) * time.Minute
	due := items[0].CreatedAt.Add(delay)
	if last.SentAt.Add(notificationEmailPeriod).After(due) {
		due = last.SentAt.Add(notificationEmailPeriod)
	}
	if due.After(now) {
		return &due, nil
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: c.InboxID})
	if err != nil {
		return nil, err
	}
	contact, err := s.contactDisplayName(ctx, ws, c.ContactID)
	if err != nil {
		return nil, err
	}
	data := notificationEmailData{
		Inbox: in.Name, Contact: contact, Subject: c.Subject, Minutes: int(delay / time.Minute),
		URL:         s.auth.PublicURL + conversationPath(ws, c.ID),
		SettingsURL: s.auth.PublicURL + "/settings/notifications?workspace_id=" + ws.String(),
	}
	for _, it := range items {
		switch {
		case it.Kind == string(oas.MessageKindEvent) && assigned:
			data.Assigned = true
			if data.AssignedBy, err = s.memberDisplayName(ctx, ws, it.AuthorMemberID); err != nil {
				return nil, err
			}
		case it.Kind == string(oas.MessageKindNote) && len(data.Mentions) < notificationEmailMessages:
			by, err := s.memberDisplayName(ctx, ws, it.AuthorMemberID)
			if err != nil {
				return nil, err
			}
			data.Mentions = append(data.Mentions, notificationMention{By: by, Text: excerpt(it.Body, emailPreviewRunes)})
		case it.Kind == string(oas.MessageKindMessage) && len(data.Messages) < notificationEmailMessages:
			data.Messages = append(data.Messages, excerpt(it.Body, emailPreviewRunes))
		case it.Kind == string(oas.MessageKindMessage):
			data.More++
		}
	}
	if len(data.Messages) == 0 && !data.Assigned && len(data.Mentions) == 0 {
		return nil, nil
	}
	msg, err := mail.Render("notification", r.Locale, data)
	if err != nil {
		return nil, err
	}
	msg.To = r.Email
	if err := s.st.RecordNotificationEmail(ctx, store.RecordNotificationEmailParams{
		WorkspaceID: ws, MemberID: r.ID, ConversationID: c.ID, Now: now, Through: items[len(items)-1].CreatedAt,
	}); err != nil {
		return nil, err
	}
	if err := s.mailer.Send(ctx, msg); err != nil {
		s.log.WarnContext(ctx, "notification e-mail", slog.Any("error", err))
	}
	return nil, nil
}

var errPushDisabled = problem(http.StatusNotFound, "push_disabled", "this server has no Web Push keys configured")

var errPushRetry = errors.New("push attempt failed")

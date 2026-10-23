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

const (
	monitorNotifyEmail    = "email"
	monitorNotifyPush     = "push"
	monitorNotifyAttempts = 8
)

// StatusMonitorNotifyArgs tells the members of a monitor's inbox that it went down or came back. It
// carries the transition itself: the state it went down in, the message and the time.
type StatusMonitorNotifyArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	MonitorID   uuid.UUID `json:"monitor_id"`
	Down        bool      `json:"down"`
	State       string    `json:"state,omitempty"`
	Message     string    `json:"message,omitempty"`
	At          time.Time `json:"at"`
}

func (StatusMonitorNotifyArgs) Kind() string { return "status_monitor_notify" }

func (StatusMonitorNotifyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// StatusMonitorNotifyMemberArgs is one member's push or e-mail for a transition; being unique by its
// arguments, a retried fan-out queues nothing twice and a retried job repeats only its own channel.
type StatusMonitorNotifyMemberArgs struct {
	StatusMonitorNotifyArgs
	MemberID uuid.UUID `json:"member_id"`
	Channel  string    `json:"channel"`
}

func (StatusMonitorNotifyMemberArgs) Kind() string { return "status_monitor_notify_member" }

func (StatusMonitorNotifyMemberArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: monitorNotifyAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type statusMonitorNotifyWorker struct {
	river.WorkerDefaults[StatusMonitorNotifyArgs]
	s *Server
}

func (w *statusMonitorNotifyWorker) Work(ctx context.Context, job *river.Job[StatusMonitorNotifyArgs]) error {
	return w.s.NotifyStatusMonitor(ctx, job.Args)
}

type statusMonitorNotifyMemberWorker struct {
	river.WorkerDefaults[StatusMonitorNotifyMemberArgs]
	s *Server
}

func (w *statusMonitorNotifyMemberWorker) Work(ctx context.Context, job *river.Job[StatusMonitorNotifyMemberArgs]) error {
	return w.s.NotifyStatusMonitorMember(ctx, job.Args)
}

type statusMonitorMail struct {
	Inbox, Page, Monitor, Component, State, Message, URL, SettingsURL string
	Down                                                              bool
}

func statusMonitorPath(ws, page uuid.UUID) string {
	return "/status-pages/" + page.String() + "/monitors?workspace_id=" + ws.String()
}

func monitorNotifyEvent(in store.Inbox) oas.PushNotificationEvent {
	if in.Mode == string(oas.Live) {
		return oas.NewLiveConversation
	}
	return oas.NewAsyncConversation
}

// NotifyStatusMonitor queues, for each member of the inbox who gets new conversations by push or
// e-mail, one job per channel, members who are away left out.
func (s *Server) NotifyStatusMonitor(ctx context.Context, a StatusMonitorNotifyArgs) error {
	ws := a.WorkspaceID
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return err
	}
	m, err := s.st.GetStatusMonitorByID(ctx, store.GetStatusMonitorByIDParams{WorkspaceID: ws, ID: a.MonitorID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	pg, err := s.st.GetStatusPage(ctx, store.GetStatusPageParams{WorkspaceID: ws, ID: m.PageID})
	if err != nil {
		return err
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: pg.InboxID})
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
	ev := monitorNotifyEvent(in)
	var queued []river.InsertManyParams
	for _, r := range candidates {
		if r.Availability == "away" {
			continue
		}
		events := applyNotificationEvents(roleNotificationDefaults(r.Role), parseNotificationUpdate(r.NotificationEvents))
		ch := notificationChannelsFor(applyNotificationEvents(events, perMember[r.ID]), ev)
		channels := []string{}
		if ch.Push && s.push != nil {
			channels = append(channels, monitorNotifyPush)
		}
		if ch.Email {
			channels = append(channels, monitorNotifyEmail)
		}
		for _, channel := range channels {
			queued = append(queued, river.InsertManyParams{Args: StatusMonitorNotifyMemberArgs{StatusMonitorNotifyArgs: a, MemberID: r.ID, Channel: channel}})
		}
	}
	if len(queued) == 0 {
		return nil
	}
	_, err = s.jobs.InsertMany(ctx, queued)
	return err
}

// NotifyStatusMonitorMember sends one member's push or e-mail about a monitor's transition.
func (s *Server) NotifyStatusMonitorMember(ctx context.Context, a StatusMonitorNotifyMemberArgs) error {
	ws := a.WorkspaceID
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return err
	}
	m, err := s.st.GetStatusMonitorByID(ctx, store.GetStatusMonitorByIDParams{WorkspaceID: ws, ID: a.MonitorID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	pg, err := s.st.GetStatusPage(ctx, store.GetStatusPageParams{WorkspaceID: ws, ID: m.PageID})
	if err != nil {
		return err
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: pg.InboxID})
	if err != nil {
		return err
	}
	comp, err := s.st.GetStatusComponent(ctx, store.GetStatusComponentParams{WorkspaceID: ws, PageID: pg.ID, ID: m.ComponentID})
	if err != nil {
		return err
	}
	candidates, err := s.st.ListNotificationCandidates(ctx, store.ListNotificationCandidatesParams{WorkspaceID: ws, InboxID: in.ID})
	if err != nil {
		return err
	}
	for _, r := range candidates {
		if r.ID != a.MemberID {
			continue
		}
		data := statusMonitorMail{
			Inbox: in.Name, Page: pg.Name, Monitor: m.Name, Component: comp.Name, Down: a.Down, Message: a.Message,
			URL:         s.auth.PublicURL + statusMonitorPath(ws, pg.ID),
			SettingsURL: s.auth.PublicURL + "/settings/notifications?workspace_id=" + ws.String(),
		}
		if a.Down {
			data.State = statusWordsFor(r.Locale).States[a.State]
		}
		if a.Channel == monitorNotifyPush {
			return s.pushStatusMonitor(ctx, r.PersonID, r.Locale, in, pg, m, data)
		}
		msg, err := mail.Render("status_monitor", r.Locale, data)
		if err != nil {
			return err
		}
		msg.To = r.Email
		if err := s.mailer.Send(ctx, msg); err != nil {
			s.log.WarnContext(ctx, "status monitor e-mail", slog.Any("error", err))
			return err
		}
	}
	return nil
}

func (s *Server) pushStatusMonitor(ctx context.Context, person uuid.UUID, locale string, in store.Inbox, pg store.StatusPage,
	m store.StatusMonitor, data statusMonitorMail) error {
	if s.push == nil {
		return nil
	}
	ws, now := m.WorkspaceID, s.now()
	subs, err := s.st.ListLivePushSubscriptionIDs(ctx, store.ListLivePushSubscriptionIDsParams{PersonID: person, Now: now, WorkspaceID: &ws})
	if err != nil || len(subs) == 0 {
		return err
	}
	msg, err := mail.Render("push_status_monitor", locale, data)
	if err != nil {
		return err
	}
	inbox := in.ID
	payload := oas.PushNotification{
		Event: monitorNotifyEvent(in), Title: msg.Subject, Body: strings.TrimSpace(msg.Text), Url: statusMonitorPath(ws, pg.ID),
		Tag: "status-monitor-" + m.ID.String(), WorkspaceId: &ws, InboxId: &inbox,
	}
	queued := make([]river.InsertManyParams, len(subs))
	for i, id := range subs {
		queued[i] = river.InsertManyParams{Args: PushArgs{
			WorkspaceID: &ws, SubscriptionID: id, Payload: mustJSON(payload), TTL: int(liveNotificationTTL / time.Second), Urgency: "high",
			Topic: strings.ReplaceAll(m.ID.String(), "-", ""),
		}, InsertOpts: pushOpts()}
	}
	_, err = s.jobs.InsertMany(ctx, queued)
	return err
}

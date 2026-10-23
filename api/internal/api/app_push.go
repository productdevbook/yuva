package api

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/apppush"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxPushDevices    = 20
	appPushAttempts   = 5
	appPushCoalesce   = 30 * time.Second
	appPushTimeout    = 30 * time.Second
	appPushHTTPTimout = 20 * time.Second
)

// AppPushSettings points the APNs and FCM clients elsewhere, for tests; empty uses Apple and Google.
type AppPushSettings struct {
	APNsProduction string
	APNsSandbox    string
	FCM            string
	TokenURL       string
	HTTPClient     *http.Client
}

func newAppPushClient(d AppPushSettings) *apppush.Client {
	hc := d.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: appPushHTTPTimout}
	}
	return &apppush.Client{HTTP: hc, APNsProduction: d.APNsProduction, APNsSandbox: d.APNsSandbox, FCM: d.FCM, TokenURL: d.TokenURL}
}

var (
	errPushDeviceNotApp  = problem(http.StatusForbidden, "forbidden", "push devices are only for app channels")
	errPushNoCredentials = errValidation("app.push.enabled needs APNs or FCM credentials")
	appleIDPattern       = regexp.MustCompile(`^[A-Z0-9]{10}$`)
)

func apnsSecretContext(workspaceID, channelID uuid.UUID) []byte {
	return append(append([]byte("apns:"), workspaceID[:]...), channelID[:]...)
}

func fcmSecretContext(workspaceID, channelID uuid.UUID) []byte {
	return append(append([]byte("fcm:"), workspaceID[:]...), channelID[:]...)
}

func appPushBody(r *store.AppPushSetting) oas.AppPush {
	var out oas.AppPush
	if r == nil {
		return out
	}
	out.Enabled, out.ShowContent, out.LastError, out.LastErrorAt = r.Enabled, r.ShowContent, r.LastError, r.LastErrorAt
	if r.ApnsKey != nil {
		env := oas.Sandbox
		if r.ApnsProduction {
			env = oas.Production
		}
		out.Apns = oas.AppPushApns{Set: true, KeyId: r.ApnsKeyID, TeamId: r.ApnsTeamID, BundleId: r.ApnsBundleID, Environment: &env}
	}
	if r.FcmCredentials != nil {
		out.Fcm = oas.AppPushFcm{Set: true, ProjectId: r.FcmProjectID, ClientEmail: r.FcmClientEmail}
	}
	return out
}

func appPushFor(ctx context.Context, q *store.Queries, workspaceID, channelID uuid.UUID) (*store.AppPushSetting, error) {
	r, err := q.GetAppPushSettings(ctx, store.GetAppPushSettingsParams{WorkspaceID: workspaceID, ChannelID: channelID})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// fillAppPush puts an app channel's push settings into its answer.
func fillAppPush(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, out *oas.Channel) error {
	if out.App == nil {
		return nil
	}
	r, err := appPushFor(ctx, q, workspaceID, out.Id)
	if err != nil {
		return err
	}
	out.App.Push = appPushBody(r)
	return nil
}

func (s *Server) saveAppPush(ctx context.Context, q *store.Queries, p principal, channelID uuid.UUID, in *oas.AppPushInput) error {
	if in == nil {
		return nil
	}
	cur, err := appPushFor(ctx, q, p.workspaceID, channelID)
	if err != nil {
		return err
	}
	arg := store.SaveAppPushSettingsParams{WorkspaceID: p.workspaceID, ChannelID: channelID, ApnsProduction: true, Now: s.now()}
	if cur != nil {
		arg.Enabled, arg.ShowContent = cur.Enabled, cur.ShowContent
		arg.ApnsKeyID, arg.ApnsTeamID, arg.ApnsBundleID, arg.ApnsProduction, arg.ApnsKey = cur.ApnsKeyID, cur.ApnsTeamID, cur.ApnsBundleID, cur.ApnsProduction, cur.ApnsKey
		arg.FcmProjectID, arg.FcmClientEmail, arg.FcmCredentials = cur.FcmProjectID, cur.FcmClientEmail, cur.FcmCredentials
		arg.ApnsVerified = cur.ApnsVerified
	}
	if in.Enabled != nil {
		arg.Enabled = *in.Enabled
	}
	if in.ShowContent != nil {
		arg.ShowContent = *in.ShowContent
	}
	var fields []string
	if a := in.Apns; a != nil {
		changed, err := s.applyAPNsInput(&arg, p.workspaceID, channelID, a)
		if err != nil {
			return err
		}
		if changed {
			fields = append(fields, "apns")
			arg.ApnsVerified = false
		}
	}
	if f := in.Fcm; f != nil {
		changed, err := s.applyFCMInput(&arg, p.workspaceID, channelID, f)
		if err != nil {
			return err
		}
		if changed {
			fields = append(fields, "fcm")
		}
	}
	if arg.Enabled && arg.ApnsKey == nil && arg.FcmCredentials == nil {
		return errPushNoCredentials
	}
	if _, err := q.SaveAppPushSettings(ctx, arg); err != nil {
		return err
	}
	if len(fields) == 0 {
		return nil
	}
	return s.audit(ctx, q, p, oas.AuditChannelCredentialsChanged, oas.AuditTargetChannel, channelID, auditDetails{"fields": fields})
}

func (s *Server) applyAPNsInput(arg *store.SaveAppPushSettingsParams, workspaceID, channelID uuid.UUID, a *oas.AppPushApnsInput) (bool, error) {
	if a.PrivateKey != nil && strings.TrimSpace(*a.PrivateKey) == "" {
		had := arg.ApnsKey != nil
		arg.ApnsKeyID, arg.ApnsTeamID, arg.ApnsBundleID, arg.ApnsKey, arg.ApnsProduction = nil, nil, nil, nil, true
		return had, nil
	}
	keyID, teamID := strings.TrimSpace(deref(a.KeyId)), strings.TrimSpace(deref(a.TeamId))
	if !appleIDPattern.MatchString(keyID) {
		return false, errValidation("app.push.apns.key_id must be the 10-character key id")
	}
	if !appleIDPattern.MatchString(teamID) {
		return false, errValidation("app.push.apns.team_id must be the 10-character team id")
	}
	bundle, err := trimmed(deref(a.BundleId), 1, 155, "app.push.apns.bundle_id")
	if err != nil {
		return false, err
	}
	production := true
	if a.Environment != nil {
		if !a.Environment.Valid() {
			return false, errValidation("app.push.apns.environment must be sandbox or production")
		}
		production = *a.Environment == oas.Production
	}
	changed := deref(arg.ApnsKeyID) != keyID || deref(arg.ApnsTeamID) != teamID || deref(arg.ApnsBundleID) != bundle || arg.ApnsProduction != production
	if a.PrivateKey != nil {
		text := strings.TrimSpace(*a.PrivateKey)
		if _, err := apppush.ParseAPNsKey(text); err != nil {
			return false, errValidation("app.push.apns.private_key must be the .p8 file's text: " + err.Error())
		}
		arg.ApnsKey, changed = s.secrets.Seal([]byte(text), apnsSecretContext(workspaceID, channelID)), true
	} else if arg.ApnsKey == nil {
		return false, errValidation("app.push.apns.private_key is required")
	}
	arg.ApnsKeyID, arg.ApnsTeamID, arg.ApnsBundleID, arg.ApnsProduction = &keyID, &teamID, &bundle, production
	return changed, nil
}

func (s *Server) applyFCMInput(arg *store.SaveAppPushSettingsParams, workspaceID, channelID uuid.UUID, f *oas.AppPushFcmInput) (bool, error) {
	if f.ServiceAccount == nil {
		if arg.FcmCredentials == nil {
			return false, errValidation("app.push.fcm.service_account is required")
		}
		if f.ProjectId == nil {
			return false, nil
		}
		project := strings.TrimSpace(*f.ProjectId)
		if project == "" || len(project) > 100 {
			return false, errValidation("app.push.fcm.project_id must be 1 to 100 characters")
		}
		changed := deref(arg.FcmProjectID) != project
		arg.FcmProjectID = &project
		return changed, nil
	}
	raw := strings.TrimSpace(*f.ServiceAccount)
	if raw == "" {
		had := arg.FcmCredentials != nil
		arg.FcmProjectID, arg.FcmClientEmail, arg.FcmCredentials = nil, nil, nil
		return had, nil
	}
	if len(raw) > 16384 {
		return false, errValidation("app.push.fcm.service_account must be at most 16 KiB")
	}
	sa, err := apppush.ParseServiceAccount([]byte(raw))
	if err != nil {
		return false, errValidation("app.push.fcm.service_account must be a service account's JSON key file: " + err.Error())
	}
	project := sa.ProjectID
	if f.ProjectId != nil {
		project = strings.TrimSpace(*f.ProjectId)
	}
	if project == "" || len(project) > 100 {
		return false, errValidation("app.push.fcm.project_id is required")
	}
	if len(sa.ClientEmail) > 320 {
		return false, errValidation("app.push.fcm.service_account has an invalid client_email")
	}
	email := sa.ClientEmail
	arg.FcmProjectID, arg.FcmClientEmail = &project, &email
	arg.FcmCredentials = s.secrets.Seal([]byte(raw), fcmSecretContext(workspaceID, channelID))
	return true, nil
}

func clientPushDeviceBody(d store.PushDevice) oas.ClientPushDevice {
	return oas.ClientPushDevice{Id: d.ID, Platform: oas.AppPlatform(d.Platform), AppId: d.AppID, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
}

func (s *Server) RegisterClientPushDevice(ctx context.Context, req oas.RegisterClientPushDeviceRequestObject) (oas.RegisterClientPushDeviceResponseObject, error) {
	cp := contactFrom(ctx)
	if cp.kind != string(oas.ChannelKindApp) {
		return nil, errPushDeviceNotApp
	}
	if err := s.rateLimit(ctx, s.writeChecks(ctx, cp)...); err != nil {
		return nil, err
	}
	b := req.Body
	if !b.Platform.Valid() {
		return nil, errValidation("platform must be ios or android")
	}
	token := strings.TrimSpace(b.Token)
	if token == "" || len(token) > 1024 || strings.ContainsFunc(token, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return nil, errValidation("token must be the device's push token, at most 1024 characters")
	}
	appID, err := trimmed(b.AppId, 1, 155, "app_id")
	if err != nil {
		return nil, err
	}
	var out store.PushDevice
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		var err error
		out, err = q.UpsertPushDevice(ctx, store.UpsertPushDeviceParams{
			ID: newID(), WorkspaceID: cp.workspaceID, ChannelID: cp.channelID, ContactID: cp.contactID,
			Platform: string(b.Platform), Token: token, AppID: appID, Now: s.now(),
		})
		if err != nil {
			return err
		}
		return q.TrimPushDevices(ctx, store.TrimPushDevicesParams{WorkspaceID: cp.workspaceID, ContactID: cp.contactID, Keep: maxPushDevices})
	})
	if err != nil {
		return nil, err
	}
	return oas.RegisterClientPushDevice200JSONResponse(clientPushDeviceBody(out)), nil
}

func (s *Server) DeleteClientPushDevice(ctx context.Context, req oas.DeleteClientPushDeviceRequestObject) (oas.DeleteClientPushDeviceResponseObject, error) {
	cp := contactFrom(ctx)
	n, err := s.st.DeletePushDevice(ctx, store.DeletePushDeviceParams{WorkspaceID: cp.workspaceID, ContactID: cp.contactID, ID: req.PushDeviceId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errNotFound
	}
	return oas.DeleteClientPushDevice204Response{}, nil
}

// AppPushArgs checks a conversation for replies to push to its contact's devices.
type AppPushArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
}

func (AppPushArgs) Kind() string { return "app_push_check" }

type appPushCheckWorker struct {
	river.WorkerDefaults[AppPushArgs]
	s *Server
}

func (w *appPushCheckWorker) Work(ctx context.Context, job *river.Job[AppPushArgs]) error {
	next, err := w.s.CheckAppPush(ctx, job.Args.WorkspaceID, job.Args.ConversationID)
	if err != nil || next == nil {
		return err
	}
	_, err = w.s.jobs.Insert(ctx, job.Args, &river.InsertOpts{ScheduledAt: *next, UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: appPushCoalesce}})
	return err
}

// AppPushSendArgs is one push to one device.
type AppPushSendArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	DeviceID       uuid.UUID `json:"device_id"`
	ContactID      uuid.UUID `json:"contact_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	MessageID      uuid.UUID `json:"message_id"`
	Badge          int       `json:"badge"`
}

func (AppPushSendArgs) Kind() string { return "app_push_send" }

func (AppPushSendArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.OutboundQueue}
}

type appPushSendWorker struct {
	river.WorkerDefaults[AppPushSendArgs]
	s *Server
}

func (w *appPushSendWorker) Work(ctx context.Context, job *river.Job[AppPushSendArgs]) error {
	retry, err := w.s.SendAppPush(ctx, job.Args)
	if err != nil {
		return err
	}
	if retry {
		return errPushRetry
	}
	return nil
}

func (w *appPushSendWorker) Timeout(*river.Job[AppPushSendArgs]) time.Duration { return appPushTimeout }

// scheduleAppPush queues a push check for a reply when the contact has a device on an app
// channel of the inbox with push on.
func (s *Server) scheduleAppPush(ctx context.Context, q *store.Queries, events *eventBatch, c store.Conversation) error {
	targets, err := q.ListPushTargets(ctx, store.ListPushTargetsParams{WorkspaceID: c.WorkspaceID, ContactID: c.ContactID, InboxID: c.InboxID})
	if err != nil || len(targets) == 0 {
		return err
	}
	events.job(AppPushArgs{WorkspaceID: c.WorkspaceID, ConversationID: c.ID}, &river.InsertOpts{})
	return nil
}

// scheduleContactPush runs when a contact's last connection goes, for replies that arrived while
// they were connected and they have not read.
func (s *Server) scheduleContactPush(ctx context.Context, workspaceID, contactID uuid.UUID) {
	ids, err := s.st.ListConversationsWithPushPending(ctx, store.ListConversationsWithPushPendingParams{WorkspaceID: workspaceID, ContactID: contactID})
	if err != nil {
		s.log.WarnContext(ctx, "app push", slog.Any("error", err))
		return
	}
	for _, id := range ids {
		if _, err := s.jobs.Insert(ctx, AppPushArgs{WorkspaceID: workspaceID, ConversationID: id}, &river.InsertOpts{}); err != nil {
			s.log.WarnContext(ctx, "app push", slog.Any("error", err))
		}
	}
}

// CheckAppPush pushes the newest reply the contact has neither read nor been pushed to every
// device of theirs on the inbox's app channels with push on, unless they are connected; pushes
// for one conversation are at least appPushCoalesce apart. It returns when to check again, or nil.
func (s *Server) CheckAppPush(ctx context.Context, workspaceID, conversationID uuid.UUID) (*time.Time, error) {
	if live, err := s.workspaceLive(ctx, workspaceID); err != nil || !live {
		return nil, err
	}
	now := s.now()
	var next *time.Time
	err := s.inTx(ctx, workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := q.LockConversation(ctx, store.LockConversationParams{WorkspaceID: workspaceID, ID: conversationID})
		if store.IsNotFound(err) || (err == nil && (c.TrashedAt != nil || c.Spam)) {
			return nil
		}
		if err != nil {
			return err
		}
		latest, err := q.LatestPushPendingReply(ctx, store.LatestPushPendingReplyParams{WorkspaceID: workspaceID, ConversationID: c.ID})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		connected, err := q.ContactConnected(ctx, store.ContactConnectedParams{WorkspaceID: workspaceID, ContactID: &c.ContactID, FreshAfter: now.Add(-presenceFresh)})
		if err != nil || connected {
			return err
		}
		if c.PushSentAt != nil {
			if due := c.PushSentAt.Add(appPushCoalesce); due.After(now) {
				next = &due
				return nil
			}
		}
		targets, err := q.ListPushTargets(ctx, store.ListPushTargetsParams{WorkspaceID: workspaceID, ContactID: c.ContactID, InboxID: c.InboxID})
		if err != nil || len(targets) == 0 {
			return err
		}
		badge, err := q.CountUnreadForContact(ctx, store.CountUnreadForContactParams{WorkspaceID: workspaceID, InboxID: c.InboxID, ContactID: c.ContactID})
		if err != nil {
			return err
		}
		for _, t := range targets {
			events.job(AppPushSendArgs{
				WorkspaceID: workspaceID, DeviceID: t.ID, ContactID: c.ContactID, ConversationID: c.ID, MessageID: latest.ID, Badge: int(badge),
			}, &river.InsertOpts{MaxAttempts: appPushAttempts})
		}
		return q.ClaimAppPush(ctx, store.ClaimAppPushParams{WorkspaceID: workspaceID, ID: c.ID, Through: latest.CreatedAt, Now: now})
	})
	return next, err
}

// appPushText renders the push titled with the inbox name, in the contact's language (else the
// inbox's), with the start of the reply when the channel shows content.
func appPushText(ctx context.Context, q *store.Queries, workspaceID, inboxID uuid.UUID, contactLocale *string, body string, showContent bool) (mail.Message, error) {
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: workspaceID, ID: inboxID})
	if err != nil {
		return mail.Message{}, err
	}
	locale := in.DefaultLocale
	if contactLocale != nil && *contactLocale != "" {
		locale = *contactLocale
	}
	data := map[string]any{"Inbox": in.Name}
	if showContent {
		data["Preview"] = excerpt(body, pushPreviewRunes)
	}
	return mail.Render("push_app_reply", locale, data)
}

// SendAppPush makes one attempt to deliver a push to a device, rendering the preview now: nothing
// is sent when the reply was redacted, trashed or deleted, or its conversation or contact is gone.
// Jobs queued before the args dropped the text carry a title and body, which are ignored. A token APNs or FCM reports
// invalid is deleted; a refused key is recorded on the channel. It reports whether to try again.
func (s *Server) SendAppPush(ctx context.Context, a AppPushSendArgs) (bool, error) {
	if live, err := s.workspaceLive(ctx, a.WorkspaceID); err != nil || !live {
		return false, err
	}
	t, err := s.st.GetPushDeviceTarget(ctx, store.GetPushDeviceTargetParams{WorkspaceID: a.WorkspaceID, ID: a.DeviceID, ContactID: a.ContactID})
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var mid *uuid.UUID
	if a.MessageID != uuid.Nil() {
		mid = &a.MessageID
	}
	pv, err := s.st.GetPushPreviewMessage(ctx, store.GetPushPreviewMessageParams{WorkspaceID: a.WorkspaceID, ConversationID: a.ConversationID, ContactID: a.ContactID, MessageID: mid})
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	msg, err := appPushText(ctx, s.st.Queries, a.WorkspaceID, pv.InboxID, pv.ContactLocale, pv.Body, t.ShowContent)
	if err != nil {
		return false, err
	}
	n := apppush.Notification{Title: msg.Subject, Body: strings.TrimSpace(msg.Text), Badge: a.Badge, ConversationID: a.ConversationID.String()}
	var r apppush.Result
	switch t.Platform {
	case string(oas.Ios):
		if t.ApnsKey == nil || deref(t.ApnsBundleID) != t.AppID {
			return false, nil
		}
		plain, err := s.secrets.Open(t.ApnsKey, apnsSecretContext(a.WorkspaceID, t.ChannelID))
		if err != nil {
			return false, s.appPushError(ctx, a.WorkspaceID, t.ChannelID, "the APNs key cannot be decrypted")
		}
		key, err := apppush.ParseAPNsKey(string(plain))
		if err != nil {
			return false, s.appPushError(ctx, a.WorkspaceID, t.ChannelID, "the APNs key cannot be read")
		}
		r = s.appPush.SendAPNs(ctx, apppush.APNs{
			KeyID: deref(t.ApnsKeyID), TeamID: deref(t.ApnsTeamID), BundleID: deref(t.ApnsBundleID), Production: t.ApnsProduction, Key: key,
		}, t.Token, n)
	case string(oas.Android):
		if t.FcmCredentials == nil {
			return false, nil
		}
		plain, err := s.secrets.Open(t.FcmCredentials, fcmSecretContext(a.WorkspaceID, t.ChannelID))
		if err != nil {
			return false, s.appPushError(ctx, a.WorkspaceID, t.ChannelID, "the FCM service account cannot be decrypted")
		}
		sa, err := apppush.ParseServiceAccount(plain)
		if err != nil {
			return false, s.appPushError(ctx, a.WorkspaceID, t.ChannelID, "the FCM service account cannot be read")
		}
		r = s.appPush.SendFCM(ctx, sa, deref(t.FcmProjectID), t.Token, n)
	default:
		return false, nil
	}
	if r.BadToken && !t.ApnsVerified {
		return false, s.appPushError(ctx, a.WorkspaceID, t.ChannelID, r.Err.Error()+" before any APNs push succeeded; check the environment (sandbox or production)")
	}
	switch {
	case r.OK():
		if t.Platform == string(oas.Ios) {
			if err := s.st.MarkAPNsVerified(ctx, store.MarkAPNsVerifiedParams{WorkspaceID: a.WorkspaceID, ChannelID: t.ChannelID}); err != nil {
				return false, err
			}
		}
		return false, s.st.SetAppPushError(ctx, store.SetAppPushErrorParams{WorkspaceID: a.WorkspaceID, ChannelID: t.ChannelID, Now: s.now()})
	case r.Gone, r.BadToken:
		s.log.InfoContext(ctx, "push device removed", slog.String("device_id", t.ID.String()), slog.Any("reason", r.Err))
		_, err := s.st.DropPushDevice(ctx, store.DropPushDeviceParams{WorkspaceID: a.WorkspaceID, ID: t.ID, Token: t.Token})
		return false, err
	case r.Retry:
		s.log.InfoContext(ctx, "app push retry", slog.String("device_id", t.ID.String()), slog.Any("reason", r.Err))
		return true, nil
	}
	return false, s.appPushError(ctx, a.WorkspaceID, t.ChannelID, r.Err.Error())
}

func (s *Server) appPushError(ctx context.Context, workspaceID, channelID uuid.UUID, reason string) error {
	reason = truncateRunes(reason, 1000)
	s.log.WarnContext(ctx, "app push failed", slog.String("channel_id", channelID.String()), slog.String("reason", reason))
	return s.st.SetAppPushError(ctx, store.SetAppPushErrorParams{WorkspaceID: workspaceID, ChannelID: channelID, Error: &reason, Now: s.now()})
}

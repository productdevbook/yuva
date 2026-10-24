package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/mail/design"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	bcDraft     = string(oas.BroadcastStateDraft)
	bcScheduled = string(oas.BroadcastStateScheduled)
	bcPreparing = string(oas.BroadcastStatePreparing)
	bcSending   = string(oas.BroadcastStateSending)
	bcPaused    = string(oas.BroadcastStatePaused)
	bcSent      = string(oas.BroadcastStateSent)
	bcCancelled = string(oas.BroadcastStateCancelled)
	bcFailed    = string(oas.BroadcastStateFailed)

	eventBroadcastUpdated  = string(oas.BroadcastEventTypeBroadcastUpdated)
	eventBroadcastProgress = string(oas.BroadcastProgressEventTypeBroadcastProgress)

	broadcastsOn       = string(oas.BroadcastsModeOn)
	broadcastsApproval = string(oas.BroadcastsModeApproval)
	broadcastsOff      = string(oas.BroadcastsModeOff)

	wsAllowed           = string(oas.Allowed)
	wsApprovalRequired  = string(oas.ApprovalRequired)
	wsApprovalRequested = string(oas.ApprovalRequested)
	wsSuspended         = string(oas.Suspended)

	maxBroadcastLists      = 10
	maxBroadcastConditions = 10
	maxScheduleAhead       = 90 * 24 * time.Hour
	hostedBroadcastLimit   = 1000
	testsPerHour           = 20
	subjectWarnRunes       = 80
	maxPaceDuration        = 365 * 24 * time.Hour
	maxPace                = 10_000_000
	paceMonth              = 30 * 24 * time.Hour
	paceShownFrom          = time.Minute
	paceSnoozeAtMost       = 5 * time.Minute
)

var (
	errBroadcastGone        = problem(http.StatusNotFound, "not_found", "no such broadcast")
	errBroadcastsDisabled   = problem(http.StatusForbidden, "broadcasts_disabled", "this server does not send broadcasts")
	errBroadcastsApproval   = problem(http.StatusForbidden, "broadcasts_approval_required", "the server's operator has not approved broadcasts for this workspace yet; ask in Settings")
	errBroadcastsSuspended  = problem(http.StatusForbidden, "broadcasts_suspended", "the workspace is suspended from sending broadcasts")
	errBroadcastsByKeys     = problem(http.StatusForbidden, "broadcasts_by_keys_disabled", "API keys and assistants may not schedule broadcasts in this workspace; an owner or admin can allow it")
	errBroadcastNotDraft    = problem(http.StatusConflict, "not_draft", "only a draft can be changed or scheduled")
	errBroadcastActive      = problem(http.StatusConflict, "broadcast_active", "the broadcast is scheduled or sending; cancel it first")
	errBroadcastState       = problem(http.StatusConflict, "invalid_state", "the broadcast is not in a state that allows this")
	errAlreadyStarted       = problem(http.StatusConflict, "already_started", "sending has started; pause or cancel it instead")
	errSenderNotReady       = problem(http.StatusConflict, "sender_not_ready", "the broadcast's e-mail channel cannot send: choose one with an SMTP account and an address to send from")
	errDesignRequired       = problem(http.StatusConflict, "design_required", "choose a design first")
	errTestLimit            = problem(http.StatusTooManyRequests, "test_limit", "at most 20 test mails an hour per workspace")
	errBroadcastLimit       = problem(http.StatusConflict, "limit_reached", "a workspace on this server has at most 1,000 broadcasts; delete old ones")
	errListInUse            = problem(http.StatusConflict, "list_in_use", "a scheduled or sending broadcast sends to this list")
	errInvalidApprovalState = problem(http.StatusConflict, "invalid_state", "the workspace does not need an approval now")
	attributeName           = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
)

func errPaceTooSlow(days int64) *apiError {
	e := problem(http.StatusConflict, "pace_too_slow", "at this pace the mail still to go out would take more than 365 days")
	e.Extra = map[string]any{"days": days}
	return e
}

type bcPace struct{ hour, day, month *int32 }

func paceOf(b store.Broadcast) bcPace {
	return bcPace{hour: b.PacePerHour, day: b.PacePerDay, month: b.PacePerMonth}
}

func (p bcPace) set() bool { return p.hour != nil || p.day != nil || p.month != nil }

// gap is the least time between two mails of the broadcast; ClaimBroadcastPaceSlot computes the same.
func (p bcPace) gap() time.Duration {
	var g time.Duration
	for _, w := range []struct {
		n   *int32
		per time.Duration
	}{{p.hour, time.Hour}, {p.day, 24 * time.Hour}, {p.month, paceMonth}} {
		if w.n != nil && *w.n > 0 {
			g = max(g, w.per/time.Duration(*w.n))
		}
	}
	return g
}

func (p bcPace) body() *oas.BroadcastPace {
	if !p.set() {
		return nil
	}
	return &oas.BroadcastPace{PerHour: p.hour, PerDay: p.day, PerMonth: p.month}
}

func paceGap(b store.Broadcast) time.Duration { return paceOf(b).gap() }

func paceNextShown(b store.Broadcast) *time.Time {
	if b.State != bcSending || b.PaceNextAt == nil || paceGap(b) < paceShownFrom {
		return nil
	}
	return b.PaceNextAt
}

func checkPace(in *oas.BroadcastPace) (bcPace, error) {
	if in == nil {
		return bcPace{}, nil
	}
	for _, f := range []struct {
		name string
		v    *int32
	}{{"per_hour", in.PerHour}, {"per_day", in.PerDay}, {"per_month", in.PerMonth}} {
		if f.v != nil && (*f.v < 1 || *f.v > maxPace) {
			return bcPace{}, errValidation("pace." + f.name + " must be a whole number from 1 to 10,000,000")
		}
	}
	return bcPace{hour: in.PerHour, day: in.PerDay, month: in.PerMonth}, nil
}

func paceSeconds(n int64, gap time.Duration, rate int32) float64 {
	if n <= 1 {
		return 0
	}
	return float64(n-1) * max(gap, time.Second/time.Duration(rate)).Seconds()
}

func (s *Server) channelRate(ctx context.Context, q *store.Queries, ws uuid.UUID, channelID *uuid.UUID) (int32, error) {
	if channelID == nil {
		return defaultBroadcastRate, nil
	}
	ch, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws, ChannelID: *channelID})
	switch {
	case store.IsNotFound(err), err == nil && ch.BroadcastRate <= 0:
		return defaultBroadcastRate, nil
	case err != nil:
		return 0, err
	}
	return ch.BroadcastRate, nil
}

func daysOf(secs float64) int64 { return int64(math.Ceil(secs / 86400)) }

func aboutDuration(secs float64) string {
	unit := func(n int64, one string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %ss", n, one)
	}
	switch {
	case secs < 90:
		return unit(int64(math.Ceil(secs)), "second")
	case secs < 90*60:
		return unit(int64(math.Round(secs/60)), "minute")
	case secs < 48*3600:
		return unit(int64(math.Round(secs/3600)), "hour")
	}
	return unit(daysOf(secs), "day")
}

// BroadcastSettings are the server's broadcast rules (YUVA_BROADCASTS,
// YUVA_BROADCAST_DAILY_RECIPIENTS, YUVA_MAIL_LINK_URL).
type BroadcastSettings struct {
	Mode            string
	DailyRecipients int64
	LinkURL         string
	// Resolver replaces the system resolver for deliverability checks and DKIM records, for tests.
	Resolver DNSResolver
}

type bcCondition struct {
	Attribute string   `json:"attribute"`
	Op        string   `json:"op"`
	Value     *string  `json:"value,omitempty"`
	Values    []string `json:"values,omitempty"`
}

type bcAudience struct {
	ListIDs    []uuid.UUID   `json:"list_ids"`
	Conditions []bcCondition `json:"conditions"`
	Locales    []string      `json:"locales"`
}

func parseAudience(raw []byte) bcAudience {
	var a bcAudience
	_ = json.Unmarshal(raw, &a)
	if a.ListIDs == nil {
		a.ListIDs = []uuid.UUID{}
	}
	if a.Conditions == nil {
		a.Conditions = []bcCondition{}
	}
	if a.Locales == nil {
		a.Locales = []string{}
	}
	return a
}

type bcCounts struct {
	Recipients          int64 `json:"recipients"`
	Prepared            int64 `json:"prepared"`
	Queued              int64 `json:"queued"`
	Sent                int64 `json:"sent"`
	Failed              int64 `json:"failed"`
	Bounced             int64 `json:"bounced"`
	Complained          int64 `json:"complained"`
	Unsubscribed        int64 `json:"unsubscribed"`
	Replied             int64 `json:"replied"`
	AutoReplied         int64 `json:"auto_replied"`
	Clicked             int64 `json:"clicked"`
	SkippedSuppressed   int64 `json:"skipped_suppressed"`
	SkippedOptedOut     int64 `json:"skipped_opted_out"`
	SkippedUnsubscribed int64 `json:"skipped_unsubscribed"`
	SkippedCancelled    int64 `json:"skipped_cancelled"`
	ExcludedSuppressed  int64 `json:"excluded_suppressed"`
	ExcludedOptedOut    int64 `json:"excluded_opted_out"`
}

func parseCounts(raw []byte) bcCounts {
	var c bcCounts
	_ = json.Unmarshal(raw, &c)
	return c
}

func (c bcCounts) body(state string) oas.BroadcastCounts {
	out := oas.BroadcastCounts{
		Recipients: c.Recipients, Queued: c.Queued, Sent: c.Sent, Failed: c.Failed, Bounced: c.Bounced, Complained: c.Complained,
		Unsubscribed: c.Unsubscribed, Replied: c.Replied, AutoReplied: c.AutoReplied, Clicked: c.Clicked,
		Skipped: oas.BroadcastSkipped{Suppressed: c.SkippedSuppressed + c.ExcludedSuppressed, OptedOut: c.SkippedOptedOut + c.ExcludedOptedOut, Unsubscribed: c.SkippedUnsubscribed, Cancelled: c.SkippedCancelled},
	}
	if state == bcPreparing {
		out.Prepared = new(c.Prepared)
	}
	return out
}

func broadcastBody(b store.Broadcast) oas.Broadcast {
	a := parseAudience(b.Audience)
	aud := oas.BroadcastAudience{ListIds: a.ListIDs, Conditions: make([]oas.BroadcastCondition, len(a.Conditions)), Locales: make([]oas.LanguageTag, len(a.Locales))}
	for i, c := range a.Conditions {
		aud.Conditions[i] = oas.BroadcastCondition{Attribute: c.Attribute, Op: oas.BroadcastConditionOp(c.Op), Value: c.Value}
		if c.Values != nil {
			aud.Conditions[i].Values = new(slices.Clone(c.Values))
		}
	}
	for i, l := range a.Locales {
		aud.Locales[i] = oas.LanguageTag(l)
	}
	out := oas.Broadcast{
		Id: b.ID, InboxId: b.InboxID, ChannelId: b.ChannelID, Name: b.Name, Subject: b.Subject, Preheader: b.Preheader,
		DesignId: b.DesignID, DesignVersion: b.DesignVersion, Audience: aud, ClickTracking: b.ClickTracking, Utm: b.Utm, UtmCampaign: b.UtmCampaign,
		State: oas.BroadcastState(b.State), StateDetail: b.StateDetail, ScheduledAt: b.ScheduledAt, WaitingUntil: b.WaitingUntil,
		StartedAt: b.StartedAt, FinishedAt: b.FinishedAt, LastTestAt: b.LastTestAt, Counts: parseCounts(b.Counts).body(b.State),
		CreatedBy: b.CreatedBy, ScheduledBy: b.ScheduledBy, ScheduledByApiKey: b.ScheduledByKey, ChangelogEntryId: b.ChangelogEntryID, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
	}
	if b.PauseReason != nil && b.State == bcPaused {
		r := oas.BroadcastPauseReason(*b.PauseReason)
		out.PauseReason = &r
	}
	if b.State == bcDraft {
		out.DesignVersion = nil
	}
	out.Pace = paceOf(b).body()
	out.PaceNextAt = paceNextShown(b)
	return out
}

func emitBroadcast(events *eventBatch, b store.Broadcast, previous string) {
	data := oas.BroadcastEventData{Broadcast: broadcastBody(b)}
	if previous != "" && previous != b.State {
		st := oas.BroadcastState(previous)
		data.PreviousState = &st
	}
	inbox := b.InboxID
	events.add(eventBroadcastUpdated, &inbox, nil, data)
}

func activeBroadcast(state string) bool {
	switch state {
	case bcScheduled, bcPreparing, bcSending, bcPaused:
		return true
	}
	return false
}

// visibleBroadcast loads a broadcast of an inbox the caller can see; others answer 404.
func visibleBroadcast(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.Broadcast, error) {
	get := q.GetBroadcast
	if lock {
		get = func(ctx context.Context, arg store.GetBroadcastParams) (store.Broadcast, error) {
			return q.LockBroadcast(ctx, store.LockBroadcastParams(arg))
		}
	}
	b, err := get(ctx, store.GetBroadcastParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return b, errBroadcastGone
	}
	if err != nil {
		return b, err
	}
	ok, err := inboxObjectsVisible(ctx, q, p, b.InboxID)
	if err != nil {
		return b, err
	}
	if !ok {
		return b, errBroadcastGone
	}
	return b, nil
}

func (s *Server) broadcastMode() string {
	if s.broadcasts.Mode == "" {
		return broadcastsOn
	}
	return s.broadcasts.Mode
}

// requireBroadcastWriter: owners, admins and keys change broadcasts, never on a server without them.
func (s *Server) requireBroadcastWriter(p principal) error {
	if err := requireManagerOrKey(p); err != nil {
		return err
	}
	if s.broadcastMode() == broadcastsOff {
		return errBroadcastsDisabled
	}
	return nil
}

// sendGate refuses scheduling and resuming while the workspace may not send, or a key or token
// acts while the workspace keeps that to members.
func (s *Server) sendGate(p principal, w store.Workspace) error {
	if s.broadcastMode() == broadcastsOff {
		return errBroadcastsDisabled
	}
	switch w.BroadcastsStatus {
	case wsApprovalRequired, wsApprovalRequested:
		return errBroadcastsApproval
	case wsSuspended:
		return errBroadcastsSuspended
	}
	if p.scoped() && !w.BroadcastsByKeys {
		return errBroadcastsByKeys
	}
	return nil
}

// broadcastAccount is the SMTP account and From address broadcasts of a channel use: its broadcast
// account when set, else its support account. ok is false when it cannot send.
func (s *Server) broadcastAccount(ch store.EmailChannel) (email.SMTPConfig, string, bool, error) {
	from := ch.Address
	if ch.FromAddress != nil {
		from = *ch.FromAddress
	}
	if isCatchAll(from) {
		return email.SMTPConfig{}, "", false, nil
	}
	if ch.BroadcastSmtpHost != nil && *ch.BroadcastSmtpHost != "" {
		cfg := email.SMTPConfig{Host: *ch.BroadcastSmtpHost, Port: 587, TLS: smtpTLSStartTLS}
		if ch.BroadcastSmtpPort != nil {
			cfg.Port = int(*ch.BroadcastSmtpPort)
		}
		if ch.BroadcastSmtpTls != nil {
			cfg.TLS = *ch.BroadcastSmtpTls
		}
		if ch.BroadcastSmtpUsername != nil {
			cfg.Username = *ch.BroadcastSmtpUsername
		}
		if len(ch.BroadcastSmtpPassword) > 0 {
			plain, err := s.secrets.Open(ch.BroadcastSmtpPassword, broadcastSMTPSecretContext(ch.WorkspaceID, ch.ChannelID))
			if err != nil {
				return cfg, from, false, errors.New("the broadcast SMTP password cannot be decrypted with this master key")
			}
			cfg.Password = string(plain)
		}
		return cfg, from, true, nil
	}
	if ch.SmtpHost == "" {
		return email.SMTPConfig{}, from, false, nil
	}
	cfg := email.SMTPConfig{Host: ch.SmtpHost, Port: int(ch.SmtpPort), Username: ch.SmtpUsername, TLS: ch.SmtpTls}
	if len(ch.SmtpPassword) > 0 {
		plain, err := s.secrets.Open(ch.SmtpPassword, smtpSecretContext(ch.WorkspaceID, ch.ChannelID))
		if err != nil {
			return cfg, from, false, errors.New("the SMTP password cannot be decrypted with this master key")
		}
		cfg.Password = string(plain)
	}
	return cfg, from, true, nil
}

func broadcastSMTPSecretContext(workspaceID, channelID uuid.UUID) []byte {
	return append(append([]byte("broadcast-smtp:"), workspaceID[:]...), channelID[:]...)
}

// broadcastChannel checks that a broadcast may go out through the channel: an e-mail channel of its
// inbox that can send.
func (s *Server) broadcastChannel(ctx context.Context, q *store.Queries, ws, inboxID, channelID uuid.UUID) error {
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
	if _, _, ok, _ := s.broadcastAccount(ec); !ok {
		return errEmailChannelRequired
	}
	return nil
}

func broadcastDesign(ctx context.Context, q *store.Queries, p principal, inboxID, id uuid.UUID) error {
	d, err := visibleDesign(ctx, q, p, id, false)
	if err != nil {
		return err
	}
	if d.ArchivedAt != nil {
		return errValidation("design_id names an archived design")
	}
	if d.InboxID != nil && *d.InboxID != inboxID {
		return errValidation("design_id names a design of another inbox")
	}
	return nil
}

func checkAudience(ctx context.Context, q *store.Queries, ws, inboxID uuid.UUID, in *oas.BroadcastAudienceInput) (bcAudience, error) {
	out := bcAudience{ListIDs: []uuid.UUID{}, Conditions: []bcCondition{}, Locales: []string{}}
	if in == nil {
		return out, nil
	}
	if len(in.ListIds) > maxBroadcastLists {
		return out, errValidation("audience.list_ids names at most 10 lists")
	}
	for _, id := range in.ListIds {
		if !slices.Contains(out.ListIDs, id) {
			out.ListIDs = append(out.ListIDs, id)
		}
	}
	if len(out.ListIDs) > 0 {
		lists, err := q.ListMailingListsByIDs(ctx, store.ListMailingListsByIDsParams{WorkspaceID: ws, Ids: out.ListIDs})
		if err != nil {
			return out, err
		}
		if len(lists) != len(out.ListIDs) {
			return out, errValidation("audience.list_ids names a list that does not exist")
		}
		for _, l := range lists {
			if l.InboxID != inboxID {
				return out, errValidation("audience.list_ids names a list of another inbox")
			}
			if l.Kind == listKindStatus {
				return out, errValidation("audience.list_ids names a status page's subscriber list, which takes only incident notices")
			}
		}
	}
	if in.Conditions != nil {
		if len(*in.Conditions) > maxBroadcastConditions {
			return out, errValidation("audience.conditions holds at most 10 conditions")
		}
		for _, c := range *in.Conditions {
			if !attributeName.MatchString(c.Attribute) {
				return out, errValidation("audience.conditions: attribute must be 1 to 64 letters, digits, '_', '.' or '-'")
			}
			if !c.Op.Valid() {
				return out, errValidation("audience.conditions: op must be eq, neq, exists, not_exists, contains, gt, lt or in")
			}
			bc := bcCondition{Attribute: c.Attribute, Op: string(c.Op)}
			switch c.Op {
			case oas.ConditionExists, oas.ConditionNotExists:
			case oas.ConditionIn:
				if c.Values == nil || len(*c.Values) == 0 || len(*c.Values) > 50 {
					return out, errValidation("audience.conditions: in needs 1 to 50 values")
				}
				bc.Values = slices.Clone(*c.Values)
			default:
				if c.Value == nil || len([]rune(*c.Value)) > 500 {
					return out, errValidation("audience.conditions: " + string(c.Op) + " needs a value of at most 500 characters")
				}
				bc.Value = new(*c.Value)
			}
			out.Conditions = append(out.Conditions, bc)
		}
	}
	if in.Locales != nil {
		if len(*in.Locales) > 20 {
			return out, errValidation("audience.locales names at most 20 languages")
		}
		for _, l := range *in.Locales {
			tag := strings.TrimSpace(string(l))
			if tag == "" || len(tag) > 35 {
				return out, errValidation("audience.locales holds language tags such as en or tr-TR")
			}
			if !slices.Contains(out.Locales, tag) {
				out.Locales = append(out.Locales, tag)
			}
		}
	}
	return out, nil
}

func (s *Server) ListBroadcasts(ctx context.Context, req oas.ListBroadcastsRequestObject) (oas.ListBroadcastsResponseObject, error) {
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
	var states []string
	if req.Params.State != nil {
		for _, st := range *req.Params.State {
			if !st.Valid() {
				return nil, errValidation("state must be draft, scheduled, preparing, sending, paused, sent, cancelled or failed")
			}
			states = append(states, string(st))
		}
	}
	rows, err := s.st.ListBroadcasts(ctx, store.ListBroadcastsParams{
		WorkspaceID: p.workspaceID, InboxID: req.Params.InboxId, States: states, AllInboxes: p.seesAllInboxes(),
		ViewerID: p.viewerID(), CursorAt: at, CursorID: id, Lim: lim + 1,
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
	out := oas.ListBroadcasts200JSONResponse{Items: make([]oas.Broadcast, len(rows)), NextCursor: next}
	for i, r := range rows {
		out.Items[i] = broadcastBody(r)
	}
	return out, nil
}

func (s *Server) CreateBroadcast(ctx context.Context, req oas.CreateBroadcastRequestObject) (oas.CreateBroadcastResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	in, err := ownedInbox(ctx, s.st.Queries, p, b.InboxId)
	if err != nil {
		return nil, err
	}
	if err := s.requireBroadcastWriter(p); err != nil {
		return nil, err
	}
	d, err := checkBroadcastDraft(*b)
	if err != nil {
		return nil, err
	}
	var out store.Broadcast
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		out, err = s.insertBroadcast(ctx, q, p, in, *b, d, nil, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateBroadcast201JSONResponse(broadcastBody(out)), nil
}

type broadcastDraft struct {
	id                       uuid.UUID
	name, subject, preheader string
	campaign                 string
	pace                     bcPace
}

func checkBroadcastDraft(b oas.BroadcastCreate) (broadcastDraft, error) {
	var d broadcastDraft
	var err error
	if d.name, err = trimmed(b.Name, 1, 200, "name"); err != nil {
		return d, err
	}
	if b.Subject != nil {
		if d.subject, err = trimmed(*b.Subject, 0, 300, "subject"); err != nil {
			return d, err
		}
	}
	if b.Preheader != nil {
		if d.preheader, err = trimmed(*b.Preheader, 0, 300, "preheader"); err != nil {
			return d, err
		}
	}
	d.id = newID()
	d.campaign = campaignOf(d.name, d.id)
	if b.UtmCampaign != nil {
		if d.campaign, err = checkCampaign(*b.UtmCampaign); err != nil {
			return d, err
		}
	}
	d.pace, err = checkPace(b.Pace)
	return d, err
}

// insertBroadcast stores a draft broadcast. A draft made from an entry records it, and its design is
// made by design in the same transaction, so nothing stays behind when the draft is refused.
func (s *Server) insertBroadcast(ctx context.Context, q *store.Queries, p principal, in store.Inbox, b oas.BroadcastCreate, d broadcastDraft, entry *uuid.UUID, design func(q *store.Queries) (uuid.UUID, error)) (store.Broadcast, error) {
	if s.broadcastMode() == broadcastsApproval {
		n, err := q.CountBroadcasts(ctx, p.workspaceID)
		if err != nil {
			return store.Broadcast{}, err
		}
		if n >= hostedBroadcastLimit {
			return store.Broadcast{}, errBroadcastLimit
		}
	}
	if b.ChannelId != nil {
		if err := s.broadcastChannel(ctx, q, p.workspaceID, in.ID, *b.ChannelId); err != nil {
			return store.Broadcast{}, err
		}
	}
	if design != nil {
		designID, err := design(q)
		if err != nil {
			return store.Broadcast{}, err
		}
		b.DesignId = &designID
	}
	if b.DesignId != nil {
		if err := broadcastDesign(ctx, q, p, in.ID, *b.DesignId); err != nil {
			return store.Broadcast{}, err
		}
	}
	aud, err := checkAudience(ctx, q, p.workspaceID, in.ID, b.Audience)
	if err != nil {
		return store.Broadcast{}, err
	}
	return q.CreateBroadcast(ctx, store.CreateBroadcastParams{
		ID: d.id, WorkspaceID: p.workspaceID, InboxID: in.ID, ChannelID: b.ChannelId, Name: d.name, Subject: d.subject,
		Preheader: d.preheader, DesignID: b.DesignId, Audience: mustJSON(aud), ClickTracking: boolOr(b.ClickTracking, false),
		Utm: boolOr(b.Utm, true), UtmCampaign: d.campaign, CreatedBy: p.memberRef(), PacePerHour: d.pace.hour,
		PacePerDay: d.pace.day, PacePerMonth: d.pace.month, ChangelogEntryID: entry, Now: s.now(),
	})
}

func (s *Server) GetBroadcast(ctx context.Context, req oas.GetBroadcastRequestObject) (oas.GetBroadcastResponseObject, error) {
	b, err := visibleBroadcast(ctx, s.st.Queries, principalFrom(ctx), req.BroadcastId, false)
	if err != nil {
		return nil, err
	}
	return oas.GetBroadcast200JSONResponse(broadcastBody(b)), nil
}

// saveBroadcast writes every column of a broadcast that Go changes; counts change only through
// AddBroadcastCounts and SetBroadcastCounts.
func (s *Server) saveBroadcast(ctx context.Context, q *store.Queries, b store.Broadcast) (store.Broadcast, error) {
	return q.SaveBroadcast(ctx, store.SaveBroadcastParams{
		WorkspaceID: b.WorkspaceID, ID: b.ID, ChannelID: b.ChannelID, Name: b.Name, Subject: b.Subject, Preheader: b.Preheader,
		DesignID: b.DesignID, DesignVersion: b.DesignVersion, Document: b.Document, Audience: b.Audience, ClickTracking: b.ClickTracking,
		Utm: b.Utm, UtmCampaign: b.UtmCampaign, State: b.State, PauseReason: b.PauseReason, StateDetail: b.StateDetail, ScheduledAt: b.ScheduledAt, WaitingUntil: b.WaitingUntil,
		StartedAt: b.StartedAt, ResumedAt: b.ResumedAt, FinishedAt: b.FinishedAt, LastTestAt: b.LastTestAt,
		SnapshotAfter: b.SnapshotAfter, SnapshotDone: b.SnapshotDone, SenderFailingSince: b.SenderFailingSince,
		ScheduledBy: b.ScheduledBy, ScheduledByKey: b.ScheduledByKey, Now: s.now(),
	})
}

func (s *Server) UpdateBroadcast(ctx context.Context, req oas.UpdateBroadcastRequestObject) (oas.UpdateBroadcastResponseObject, error) {
	p := principalFrom(ctx)
	body := req.Body
	var out store.Broadcast
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		b, err := visibleBroadcast(ctx, q, p, req.BroadcastId, true)
		if err != nil {
			return err
		}
		if err := s.requireBroadcastWriter(p); err != nil {
			return err
		}
		if b.State != bcDraft {
			if body.Pace == nil || !reflect.DeepEqual(*body, oas.BroadcastUpdate{Pace: body.Pace}) {
				return errBroadcastNotDraft
			}
			if !activeBroadcast(b.State) {
				return errBroadcastState
			}
			pace, err := checkPace(body.Pace)
			if err != nil {
				return err
			}
			if out, err = s.changePace(ctx, q, b, pace); err != nil {
				return err
			}
			emitBroadcast(events, out, "")
			return nil
		}
		pace, err := checkPace(body.Pace)
		if err != nil {
			return err
		}
		if body.Name != nil {
			if b.Name, err = trimmed(*body.Name, 1, 200, "name"); err != nil {
				return err
			}
		}
		if body.Subject != nil {
			if b.Subject, err = trimmed(*body.Subject, 0, 300, "subject"); err != nil {
				return err
			}
		}
		if body.Preheader != nil {
			if b.Preheader, err = trimmed(*body.Preheader, 0, 300, "preheader"); err != nil {
				return err
			}
		}
		if body.ChannelId.IsSpecified() {
			b.ChannelID = nil
			if !body.ChannelId.IsNull() {
				id := body.ChannelId.MustGet()
				if err := s.broadcastChannel(ctx, q, p.workspaceID, b.InboxID, id); err != nil {
					return err
				}
				b.ChannelID = &id
			}
		}
		if body.DesignId.IsSpecified() {
			b.DesignID = nil
			if !body.DesignId.IsNull() {
				id := body.DesignId.MustGet()
				if err := broadcastDesign(ctx, q, p, b.InboxID, id); err != nil {
					return err
				}
				b.DesignID = &id
			}
		}
		if body.Audience != nil {
			aud, err := checkAudience(ctx, q, p.workspaceID, b.InboxID, body.Audience)
			if err != nil {
				return err
			}
			b.Audience = mustJSON(aud)
		}
		if body.ClickTracking != nil {
			b.ClickTracking = *body.ClickTracking
		}
		if body.Utm != nil {
			b.Utm = *body.Utm
		}
		if body.UtmCampaign != nil {
			if b.UtmCampaign, err = checkCampaign(*body.UtmCampaign); err != nil {
				return err
			}
		}
		if out, err = s.saveBroadcast(ctx, q, b); err != nil || body.Pace == nil {
			return err
		}
		out, err = q.SetBroadcastPace(ctx, store.SetBroadcastPaceParams{
			WorkspaceID: b.WorkspaceID, ID: b.ID, PerHour: pace.hour, PerDay: pace.day, PerMonth: pace.month, Now: s.now(),
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateBroadcast200JSONResponse(broadcastBody(out)), nil
}

func (s *Server) changePace(ctx context.Context, q *store.Queries, b store.Broadcast, pace bcPace) (store.Broadcast, error) {
	oldGap, newGap := paceGap(b), pace.gap()
	if newGap > 0 {
		remaining := parseCounts(b.Counts).Queued
		if !b.SnapshotDone {
			count, err := s.countAudience(ctx, q, b, nil)
			if err != nil {
				return b, err
			}
			remaining = count.Recipients
		}
		rate, err := s.channelRate(ctx, q, b.WorkspaceID, b.ChannelID)
		if err != nil {
			return b, err
		}
		if secs := paceSeconds(remaining, newGap, rate); secs > maxPaceDuration.Seconds() {
			return b, errPaceTooSlow(daysOf(secs))
		}
	}
	var next *time.Time
	if newGap > 0 && b.PaceNextAt != nil {
		next = new(b.PaceNextAt.Add(newGap - oldGap))
	}
	return q.SetBroadcastPace(ctx, store.SetBroadcastPaceParams{
		WorkspaceID: b.WorkspaceID, ID: b.ID, PerHour: pace.hour, PerDay: pace.day, PerMonth: pace.month, NextAt: next, Now: s.now(),
	})
}

func (s *Server) DeleteBroadcast(ctx context.Context, req oas.DeleteBroadcastRequestObject) (oas.DeleteBroadcastResponseObject, error) {
	p := principalFrom(ctx)
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		b, err := visibleBroadcast(ctx, q, p, req.BroadcastId, true)
		if err != nil {
			return err
		}
		if err := requireManagerOrKey(p); err != nil {
			return err
		}
		if activeBroadcast(b.State) {
			return errBroadcastActive
		}
		_, err = q.DeleteBroadcast(ctx, store.DeleteBroadcastParams{WorkspaceID: p.workspaceID, ID: b.ID})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteBroadcast204Response{}, nil
}

// audienceRow is one recipient as the audience query returns it, with whether the address
// objected to all mail from the workspace.
type audienceRow struct {
	store.BroadcastAudiencePageRow
	optedOut bool
}

func (r audienceRow) sendable() bool {
	return r.Matches && !r.Blocked && !r.Suppressed && !r.optedOut
}

const audiencePage = 1000

// audiencePageAfter reads the next page of the broadcast's audience after the contact id `after`.
func (s *Server) audiencePageAfter(ctx context.Context, q *store.Queries, ws uuid.UUID, a bcAudience, after uuid.UUID) ([]audienceRow, error) {
	if len(a.ListIDs) == 0 {
		return nil, nil
	}
	rows, err := q.BroadcastAudiencePage(ctx, store.BroadcastAudiencePageParams{
		WorkspaceID: ws, ListIds: a.ListIDs, Conditions: mustJSON(a.Conditions), Locales: a.Locales, After: after, Lim: audiencePage,
	})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	emails := make([]string, len(rows))
	for i, r := range rows {
		emails[i] = r.Email
	}
	hashes, err := s.optOutHashes(ctx, q, ws, emails...)
	if err != nil {
		return nil, err
	}
	found, err := q.ListOptOutHashes(ctx, store.ListOptOutHashesParams{WorkspaceID: ws, Hashes: hashes})
	if err != nil {
		return nil, err
	}
	opted := map[string]bool{}
	for _, h := range found {
		opted[string(h)] = true
	}
	out := make([]audienceRow, len(rows))
	for i, r := range rows {
		out[i] = audienceRow{BroadcastAudiencePageRow: r, optedOut: opted[string(hashes[i])]}
	}
	return out, nil
}

// contactValues are the merge field values of an audience row.
func contactValues(name, addr string, attributes []byte) design.Values {
	v := design.Values{ContactName: name, ContactEmail: addr, Attributes: map[string]string{}}
	var attrs map[string]any
	if json.Unmarshal(attributes, &attrs) == nil {
		for k, x := range attrs {
			switch t := x.(type) {
			case string:
				v.Attributes[k] = t
			case nil:
			default:
				b, _ := json.Marshal(t)
				v.Attributes[k] = string(b)
			}
		}
	}
	return v
}

// countAudience walks the whole audience as sending would take it now.
func (s *Server) countAudience(ctx context.Context, q *store.Queries, b store.Broadcast, fields []design.Field) (oas.BroadcastAudienceCount, error) {
	a := parseAudience(b.Audience)
	out := oas.BroadcastAudienceCount{MissingMergeFields: map[string]int64{}, ByList: []oas.BroadcastListCount{}}
	var required []design.Field
	for _, f := range fields {
		if strings.HasPrefix(f.Name, "contact.") && !f.HasFallback && !slices.ContainsFunc(required, func(x design.Field) bool { return x.Name == f.Name }) {
			required = append(required, f)
		}
	}
	if len(a.ListIDs) == 0 {
		return out, nil
	}
	byList := map[uuid.UUID]int64{}
	after := uuid.Nil()
	for {
		rows, err := s.audiencePageAfter(ctx, q, b.WorkspaceID, a, after)
		if err != nil {
			return out, err
		}
		for _, r := range rows {
			out.Excluded.Duplicate += r.Subscriptions - 1
			switch {
			case !r.Matches:
				out.Excluded.Filtered++
			case r.Blocked:
				out.Excluded.Blocked++
			case r.Suppressed:
				out.Excluded.Suppressed++
			case r.optedOut:
				out.Excluded.OptedOut++
			default:
				out.Recipients++
				byList[r.ListID]++
				if len(required) > 0 {
					v := contactValues(r.Name, r.Email, r.Attributes)
					for _, f := range required {
						if strings.TrimSpace(v.Text("{{ "+f.Name+" }}")) == "" {
							out.MissingMergeFields[f.Name]++
						}
					}
				}
			}
		}
		if len(rows) < audiencePage {
			break
		}
		after = rows[len(rows)-1].ContactID
	}
	left, err := q.BroadcastAudienceLeftOut(ctx, store.BroadcastAudienceLeftOutParams{WorkspaceID: b.WorkspaceID, ListIds: a.ListIDs})
	if err != nil {
		return out, err
	}
	out.Excluded.Unsubscribed, out.Excluded.Pending = left.Unsubscribed, left.Pending
	for _, id := range a.ListIDs {
		if n := byList[id]; n > 0 {
			out.ByList = append(out.ByList, oas.BroadcastListCount{ListId: id, Recipients: n})
		}
	}
	return out, nil
}

// broadcastDocument is the document a broadcast sends: the frozen copy once scheduled, else its
// design as saved now.
func (s *Server) broadcastDocument(ctx context.Context, q *store.Queries, b store.Broadcast) (*design.Document, *store.EmailDesign, error) {
	raw := b.Document
	var row *store.EmailDesign
	if raw == nil {
		if b.DesignID == nil {
			return nil, nil, nil
		}
		d, err := q.GetEmailDesign(ctx, store.GetEmailDesignParams{WorkspaceID: b.WorkspaceID, ID: *b.DesignID})
		if store.IsNotFound(err) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, err
		}
		raw, row = d.Document, &d
	}
	doc, issues, err := design.Parse(raw)
	if err != nil || len(issues) > 0 {
		return nil, row, nil
	}
	return &doc, row, nil
}

func (s *Server) CountBroadcastAudience(ctx context.Context, req oas.CountBroadcastAudienceRequestObject) (oas.CountBroadcastAudienceResponseObject, error) {
	p := principalFrom(ctx)
	b, err := visibleBroadcast(ctx, s.st.Queries, p, req.BroadcastId, false)
	if err != nil {
		return nil, err
	}
	var fields []design.Field
	doc, _, err := s.broadcastDocument(ctx, s.st.Queries, b)
	if err != nil {
		return nil, err
	}
	if doc != nil {
		fields = doc.MergeFields()
	}
	fields = append(fields, design.Fields(b.Subject)...)
	out, err := s.countAudience(ctx, s.st.Queries, b, fields)
	if err != nil {
		return nil, err
	}
	return oas.CountBroadcastAudience200JSONResponse(out), nil
}

func (s *Server) RequestBroadcastApproval(ctx context.Context, req oas.RequestBroadcastApprovalRequestObject) (oas.RequestBroadcastApprovalResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireOwner(p); err != nil {
		return nil, err
	}
	b := req.Body
	website, err := trimmed(b.Website, 1, 2000, "website")
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(website, "https://") && !strings.HasPrefix(website, "http://") {
		return nil, errValidation("website must be an http(s) URL")
	}
	sender, err := trimmed(b.SenderDescription, 1, 2000, "sender_description")
	if err != nil {
		return nil, err
	}
	consent, err := trimmed(b.ConsentDescription, 1, 2000, "consent_description")
	if err != nil {
		return nil, err
	}
	var w store.Workspace
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.LockWorkspace(ctx, p.workspaceID); err != nil {
			return err
		}
		cur, err := q.GetWorkspace(ctx, p.workspaceID)
		if err != nil {
			return err
		}
		if cur.BroadcastsStatus != wsApprovalRequired {
			return errInvalidApprovalState
		}
		if w, err = q.SetWorkspaceBroadcastsStatus(ctx, store.SetWorkspaceBroadcastsStatusParams{ID: p.workspaceID, Status: wsApprovalRequested, Now: s.now()}); err != nil {
			return err
		}
		if err := s.audit(ctx, q, p, oas.AuditWorkspaceBroadcastApprovalRequested, oas.AuditTargetWorkspace, p.workspaceID, nil); err != nil {
			return err
		}
		owner, err := q.GetMember(ctx, store.GetMemberParams{WorkspaceID: p.workspaceID, ID: p.memberID})
		if err != nil {
			return err
		}
		return s.noteBroadcastOperator(ctx, q, noticeBroadcastApproval, owner.Email, w.Name,
			"website "+oneLine(website)+"; sends: "+oneLine(sender)+"; consent: "+oneLine(consent))
	})
	if err != nil {
		return nil, err
	}
	body, err := s.workspaceBody(ctx, w)
	if err != nil {
		return nil, err
	}
	return oas.RequestBroadcastApproval202JSONResponse(body), nil
}

// workspaceBody is the workspace with its broadcast status, limit and today's count.
func (s *Server) workspaceBody(ctx context.Context, w store.Workspace) (oas.Workspace, error) {
	return s.workspaceBodyQ(ctx, s.st.Queries, w)
}

func (s *Server) workspaceBodyQ(ctx context.Context, q *store.Queries, w store.Workspace) (oas.Workspace, error) {
	out := oas.Workspace{Id: w.ID, Name: w.Name, RetentionDays: w.RetentionDays, BotsMaySend: w.BotsMaySend, CreatedAt: w.CreatedAt}
	if w.Focus != nil {
		out.Focus = new(oas.WorkspaceFocus(*w.Focus))
	}
	out.Broadcasts = oas.WorkspaceBroadcasts{
		Mode: oas.BroadcastsMode(s.broadcastMode()), Status: oas.BroadcastsStatus(w.BroadcastsStatus), Reason: w.BroadcastsStatusReason,
		StatusAt: w.BroadcastsStatusAt, ByKeys: w.BroadcastsByKeys,
	}
	limit, sent, err := s.dailyAllowance(ctx, q, w)
	if err != nil {
		return out, err
	}
	out.Broadcasts.SentToday = sent
	if limit >= 0 {
		out.Broadcasts.DailyLimit = new(limit)
	}
	return out, nil
}

func (s *Server) queueBroadcastStart(events *eventBatch, b store.Broadcast, at time.Time) {
	events.job(BroadcastStartArgs{WorkspaceID: b.WorkspaceID, BroadcastID: b.ID}, broadcastJobOpts(at, ""))
}

func (s *Server) queueBroadcastSend(events *eventBatch, ws, channelID uuid.UUID) {
	events.jobOnce(BroadcastSendArgs{WorkspaceID: ws, ChannelID: channelID}, broadcastJobOpts(time.Time{}, broadcastQueue))
}

func (s *Server) broadcastProgress(ctx context.Context, b store.Broadcast) {
	inbox := b.InboxID
	s.signal(ctx, realtime.Event{Type: eventBroadcastProgress, WorkspaceID: b.WorkspaceID, InboxID: &inbox, Data: mustJSON(oas.BroadcastProgress{
		Id: b.ID, State: oas.BroadcastState(b.State), Counts: parseCounts(b.Counts).body(b.State), WaitingUntil: b.WaitingUntil,
		PaceNextAt: paceNextShown(b),
	})})
}

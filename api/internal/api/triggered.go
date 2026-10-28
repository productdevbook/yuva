package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/mail/design"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	bcOnce            = string(oas.KindOnce)
	bcTriggered       = string(oas.KindTriggered)
	basisService      = string(oas.BasisService)
	basisMarketing    = string(oas.BasisMarketing)
	triggerIdentified = string(oas.TriggerContactIdentified)
	triggerSubscribed = string(oas.TriggerSubscribed)
	triggerAttribute  = string(oas.TriggerAttributeChanged)

	enrollWaiting  = string(oas.TriggeredEnrollmentStateWaiting)
	enrollBackfill = string(oas.TriggeredEnrollmentSourceBackfill)

	maxLiveTriggered = 20
	maxDelayMinutes  = 525600
	maxBackfillDays  = 30
	enrollmentBatch  = 500
	backfillPage     = 1000
	lateAfter        = 7 * 24 * time.Hour
	triggeredBudget  = 30 * time.Second
)

var (
	triggeredTokenContext = []byte("yuva triggered manage v1")

	errTriggerInvalid     = problem(http.StatusBadRequest, "trigger_invalid", "a triggered e-mail needs trigger.event, trigger.delay_minutes and trigger.basis; a one-off broadcast takes no trigger")
	errBasisLists         = problem(http.StatusBadRequest, "basis_lists", "marketing mail goes to 1 to 10 lists of the inbox; service mail goes to no list")
	errTooManyTriggered   = problem(http.StatusConflict, "too_many_triggered", "an inbox has at most 20 triggered e-mails scheduled, active or paused")
	errServiceUnconfirmed = problem(http.StatusBadRequest, "service_basis_unconfirmed", "confirm that this mail is about using the product, not advertising (confirm_service: true)")
	errTriggerFields      = problem(http.StatusBadRequest, "trigger_invalid", "subscribed takes trigger.list_id, attribute_changed takes trigger.condition, contact_identified takes neither")
	errTriggerBackfill    = problem(http.StatusBadRequest, "trigger_invalid", "attribute_changed takes no back-fill: backfill_days must be 0")
	errEnrollmentGone     = problem(http.StatusNotFound, "not_found", "the triggered e-mail never enrolled this contact")
	errNotTriggered       = errValidation("only a triggered e-mail has enrollments")
)

type bcTrigger struct {
	Event     string       `json:"event"`
	ListID    *uuid.UUID   `json:"list_id,omitempty"`
	Condition *bcCondition `json:"condition,omitempty"`
}

func triggerOf(b store.Broadcast) bcTrigger {
	var t bcTrigger
	_ = json.Unmarshal(b.Trigger, &t)
	return t
}

func serviceBasis(b store.Broadcast) bool {
	return b.Kind == bcTriggered && deref(b.Basis) == basisService
}

func triggerEvent(b store.Broadcast) string {
	return triggerOf(b).Event
}

func triggerBody(b store.Broadcast) *oas.BroadcastTrigger {
	if b.Kind != bcTriggered || b.DelayMinutes == nil {
		return nil
	}
	t := triggerOf(b)
	out := &oas.BroadcastTrigger{
		Event: oas.BroadcastTriggerEvent(t.Event), ListId: t.ListID, DelayMinutes: *b.DelayMinutes,
		Basis: oas.BroadcastBasis(deref(b.Basis)), BackfillDays: b.BackfillDays,
	}
	if t.Condition != nil {
		out.Condition = new(conditionBody(*t.Condition))
	}
	return out
}

func delayOf(b store.Broadcast) time.Duration {
	if b.DelayMinutes == nil {
		return 0
	}
	return time.Duration(*b.DelayMinutes) * time.Minute
}

type triggerDraft struct {
	kind, event, basis string
	listID             *uuid.UUID
	condition          *bcCondition
	delay, backfill    *int32
}

func draftOf(b store.Broadcast) triggerDraft {
	t := triggerOf(b)
	return triggerDraft{
		kind: bcTriggered, event: t.Event, basis: deref(b.Basis), listID: t.ListID, condition: t.Condition,
		delay: b.DelayMinutes, backfill: &b.BackfillDays,
	}
}

func (d triggerDraft) stored() bcTrigger {
	return bcTrigger{Event: d.event, ListID: d.listID, Condition: d.condition}
}

func (d triggerDraft) check() error {
	switch d.event {
	case triggerIdentified:
		if d.listID != nil || d.condition != nil {
			return errTriggerFields
		}
	case triggerSubscribed:
		if d.listID == nil || d.condition != nil {
			return errTriggerFields
		}
	case triggerAttribute:
		if d.condition == nil || d.listID != nil {
			return errTriggerFields
		}
		if d.backfill != nil && *d.backfill > 0 {
			return errTriggerBackfill
		}
	default:
		return errTriggerInvalid
	}
	return nil
}

func checkTrigger(kind *oas.BroadcastKind, in *oas.BroadcastTriggerInput) (triggerDraft, error) {
	d := triggerDraft{kind: bcOnce}
	if kind != nil {
		if !kind.Valid() {
			return d, errValidation("kind must be once or triggered")
		}
		d.kind = string(*kind)
	}
	if d.kind == bcOnce {
		if in != nil {
			return d, errTriggerInvalid
		}
		return d, nil
	}
	if in == nil || in.Event == nil || in.DelayMinutes == nil || in.Basis == nil {
		return d, errTriggerInvalid
	}
	d.backfill = new(int32(0))
	if err := applyTrigger(&d, in); err != nil {
		return d, err
	}
	return d, d.check()
}

func applyTrigger(d *triggerDraft, in *oas.BroadcastTriggerInput) error {
	if in.Event != nil {
		if !in.Event.Valid() {
			return errTriggerInvalid
		}
		if string(*in.Event) != d.event {
			d.listID, d.condition = nil, nil
		}
		d.event = string(*in.Event)
	}
	if in.ListId != nil {
		d.listID = new(*in.ListId)
	}
	if in.Condition != nil {
		c, err := checkBroadcastCondition(*in.Condition, "trigger.condition")
		if err != nil {
			return err
		}
		d.condition = &c
	}
	if in.Basis != nil {
		if !in.Basis.Valid() {
			return errTriggerInvalid
		}
		d.basis = string(*in.Basis)
	}
	if in.DelayMinutes != nil {
		if *in.DelayMinutes < 0 || *in.DelayMinutes > maxDelayMinutes {
			return errValidation("trigger.delay_minutes must be 0 to 525600")
		}
		d.delay = in.DelayMinutes
	}
	if in.BackfillDays != nil {
		if *in.BackfillDays < 0 || *in.BackfillDays > maxBackfillDays {
			return errValidation("trigger.backfill_days must be 0 to 30")
		}
		d.backfill = in.BackfillDays
	}
	return nil
}

func checkTriggerList(ctx context.Context, q *store.Queries, ws, inboxID uuid.UUID, listID *uuid.UUID) error {
	if listID == nil {
		return nil
	}
	l, err := q.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: ws, ID: *listID})
	switch {
	case store.IsNotFound(err):
		return errValidation("trigger.list_id names a list that does not exist")
	case err != nil:
		return err
	case l.InboxID != inboxID:
		return errValidation("trigger.list_id names a list of another inbox")
	case l.Kind == listKindStatus:
		return errValidation("trigger.list_id names a status page's subscriber list, which takes only incident notices")
	}
	return nil
}

func checkBasisLists(basis string, lists int) error {
	if (basis == basisService && lists > 0) || (basis == basisMarketing && (lists == 0 || lists > maxBroadcastLists)) {
		return errBasisLists
	}
	return nil
}

func (s *Server) checkLiveTriggered(ctx context.Context, q *store.Queries, ws, inboxID, except uuid.UUID) error {
	n, err := q.CountLiveTriggered(ctx, store.CountLiveTriggeredParams{WorkspaceID: ws, InboxID: inboxID, ExceptID: except})
	if err != nil {
		return err
	}
	if n >= maxLiveTriggered {
		return errTooManyTriggered
	}
	return nil
}

func updateTriggeredDraft(ctx context.Context, q *store.Queries, b *store.Broadcast, in *oas.BroadcastTriggerInput) error {
	if b.Kind != bcTriggered {
		if in != nil {
			return errTriggerInvalid
		}
		return nil
	}
	d := draftOf(*b)
	if in != nil {
		if err := applyTrigger(&d, in); err != nil {
			return err
		}
	}
	if err := d.check(); err != nil {
		return err
	}
	if in != nil && in.ListId != nil {
		if err := checkTriggerList(ctx, q, b.WorkspaceID, b.InboxID, d.listID); err != nil {
			return err
		}
	}
	b.Trigger, b.Basis, b.DelayMinutes, b.BackfillDays = mustJSON(d.stored()), &d.basis, d.delay, *d.backfill
	return checkBasisLists(d.basis, len(parseAudience(b.Audience).ListIDs))
}

func (s *Server) updateStartedTriggered(ctx context.Context, q *store.Queries, p principal, b store.Broadcast, body *oas.BroadcastUpdate) (store.Broadcast, error) {
	switch b.State {
	case bcScheduled, bcActive, bcPaused:
	default:
		return b, errBroadcastState
	}
	paused := b.State == bcPaused
	refuse := body.Name != nil || body.ChannelId.IsSpecified() || body.Utm != nil || body.UtmCampaign != nil
	if !paused {
		refuse = refuse || body.DesignId.IsSpecified() || body.Subject != nil || body.Preheader != nil || body.Translations != nil ||
			body.Audience != nil || (body.ClickTracking != nil && *body.ClickTracking)
	}
	if t := body.Trigger; t != nil {
		refuse = refuse || t.Event != nil || t.Basis != nil || t.ListId != nil || t.Condition != nil || (t.BackfillDays != nil && b.State != bcScheduled)
	}
	if refuse {
		return b, errBroadcastState
	}
	var err error
	if t := body.Trigger; t != nil {
		d := draftOf(b)
		if err := applyTrigger(&d, t); err != nil {
			return b, err
		}
		if err := d.check(); err != nil {
			return b, err
		}
		b.DelayMinutes, b.BackfillDays = d.delay, *d.backfill
	}
	if body.Subject != nil {
		if b.Subject, err = trimmed(*body.Subject, 0, 300, "subject"); err != nil {
			return b, err
		}
	}
	if body.Preheader != nil {
		if b.Preheader, err = trimmed(*body.Preheader, 0, 300, "preheader"); err != nil {
			return b, err
		}
	}
	if body.Translations != nil {
		trans, err := checkTranslations(*body.Translations)
		if err != nil {
			return b, err
		}
		b.Translations = translationsJSON(trans)
	}
	if body.DesignId.IsSpecified() {
		if body.DesignId.IsNull() {
			return b, errDesignRequired
		}
		id := body.DesignId.MustGet()
		if err := broadcastDesign(ctx, q, p, b.InboxID, id); err != nil {
			return b, err
		}
		b.DesignID = &id
	}
	if body.Audience != nil {
		aud, err := checkAudience(ctx, q, p.workspaceID, b.InboxID, body.Audience)
		if err != nil {
			return b, err
		}
		if !slices.Equal(aud.ListIDs, parseAudience(b.Audience).ListIDs) {
			return b, errBroadcastState
		}
		b.Audience = mustJSON(aud)
	}
	if body.ClickTracking != nil {
		b.ClickTracking = *body.ClickTracking
	}
	out, err := s.saveBroadcast(ctx, q, b)
	if err != nil {
		return b, err
	}
	if body.Pace == nil && body.Timezone == nil && !body.SendWindow.IsSpecified() {
		return out, nil
	}
	next, err := sendTimeUpdate(out, body)
	if err != nil {
		return b, err
	}
	pace := paceOf(out)
	if body.Pace != nil {
		if pace, err = checkPace(body.Pace); err != nil {
			return b, err
		}
	}
	return s.changePace(ctx, q, out, next, pace)
}

func (s *Server) triggerContact(ctx context.Context, q *store.Queries, events *eventBatch, ws, inboxID, contactID uuid.UUID, event string, at time.Time) error {
	if events.noTriggers {
		return nil
	}
	live, err := q.ListLiveTriggered(ctx, store.ListLiveTriggeredParams{WorkspaceID: ws, InboxID: inboxID, Event: event})
	if err != nil {
		return err
	}
	return s.enroll(ctx, q, events, ws, contactID, at, live)
}

func (s *Server) enroll(ctx context.Context, q *store.Queries, events *eventBatch, ws, contactID uuid.UUID, at time.Time, live []store.ListLiveTriggeredRow) error {
	now := s.now()
	for _, b := range live {
		n, err := q.EnrollContact(ctx, store.EnrollContactParams{WorkspaceID: ws, BroadcastID: b.ID, ContactID: contactID, TriggeredAt: at, Now: now})
		if err != nil {
			return err
		}
		if n > 0 {
			due := at.Add(time.Duration(b.DelayMinutes) * time.Minute)
			events.jobOnce(TriggeredDueArgs{WorkspaceID: ws, BroadcastID: b.ID}, broadcastJobOpts(paceWake(due, now), broadcastQueue))
		}
	}
	return nil
}

func (s *Server) subscriptionConfirmed(ctx context.Context, q *store.Queries, events *eventBatch, list store.MailingList, sub store.ListSubscription, previous string) error {
	if events.noTriggers || list.Kind == listKindStatus || sub.State != subSubscribed || previous == subSubscribed || sub.Source == string(oas.SubscriptionSourceImport) {
		return nil
	}
	live, err := q.ListLiveTriggered(ctx, store.ListLiveTriggeredParams{
		WorkspaceID: sub.WorkspaceID, InboxID: list.InboxID, Event: triggerSubscribed, ListKey: list.ID.String(),
	})
	if err != nil {
		return err
	}
	return s.enroll(ctx, q, events, sub.WorkspaceID, sub.ContactID, s.now(), live)
}

// attributesWritten enrolls a contact whose attributes newly match an attribute_changed trigger of
// an inbox it belongs to or was written through; previous is nil when the write created it.
func (s *Server) attributesWritten(ctx context.Context, q *store.Queries, events *eventBatch, ws, contactID uuid.UUID, via []uuid.UUID, previous, attrs []byte) error {
	if events.noTriggers || (previous != nil && bytes.Equal(previous, attrs)) {
		return nil
	}
	if live, err := q.HasAttributeTriggered(ctx, ws); err != nil || !live {
		return err
	}
	if via == nil {
		via = []uuid.UUID{}
	}
	rows, err := q.ListAttributeTriggered(ctx, store.ListAttributeTriggeredParams{
		WorkspaceID: ws, Attributes: attrs, Previous: previous, ViaInboxes: via, ContactID: contactID,
	})
	if err != nil || len(rows) == 0 {
		return err
	}
	live := make([]store.ListLiveTriggeredRow, len(rows))
	for i, r := range rows {
		live[i] = store.ListLiveTriggeredRow(r)
	}
	return s.enroll(ctx, q, events, ws, contactID, s.now(), live)
}

// first says the caller knows the contact has no external id in the inbox yet.
func (s *Server) addExternalID(ctx context.Context, q *store.Queries, events *eventBatch, ws, inboxID, contactID uuid.UUID, externalID string, first bool) error {
	if !first && !events.noTriggers {
		inInbox, _, err := externalIDPlaces(ctx, q, ws, inboxID, contactID)
		if err != nil {
			return err
		}
		first = !inInbox
	}
	now := s.now()
	if err := q.AddContactExternalID(ctx, store.AddContactExternalIDParams{WorkspaceID: ws, InboxID: inboxID, ExternalID: externalID, ContactID: contactID, CreatedAt: &now}); err != nil {
		return err
	}
	if !first {
		return nil
	}
	return s.triggerContact(ctx, q, events, ws, inboxID, contactID, triggerIdentified, now)
}

func (s *Server) triggeredStopURL(b store.Broadcast, contactID uuid.UUID) string {
	return s.linkBase() + managePathPrefix + s.sealToken(triggeredTokenContext, []uuid.UUID{b.WorkspaceID, b.InboxID, contactID, b.ID}, nil)
}

func enrollmentCountsSet(r store.EnrollmentCountsRow) map[string]int64 {
	return map[string]int64{
		"enrolled": r.Enrolled, "waiting": r.Waiting, "skipped_no_address": r.NoAddress, "skipped_blocked": r.Blocked,
		"enrollment_suppressed": r.Suppressed, "enrollment_opted_out": r.OptedOut, "enrollment_stopped": r.Stopped,
		"skipped_not_subscribed": r.NotSubscribed, "skipped_not_matching": r.NotMatching, "skipped_late": r.Late, "skipped_removed": r.Removed,
	}
}

func enrollmentCounts(ctx context.Context, q *store.Queries, b store.Broadcast, set map[string]int64) error {
	r, err := q.EnrollmentCounts(ctx, store.EnrollmentCountsParams{WorkspaceID: b.WorkspaceID, BroadcastID: b.ID})
	if err != nil {
		return err
	}
	for k, v := range enrollmentCountsSet(r) {
		set[k] = v
	}
	return nil
}

func (s *Server) syncEnrollmentCounts(ctx context.Context, q *store.Queries, b store.Broadcast, extra map[string]int64) error {
	locked, err := q.LockBroadcast(ctx, store.LockBroadcastParams{WorkspaceID: b.WorkspaceID, ID: b.ID})
	if err != nil {
		return err
	}
	r, err := q.EnrollmentCounts(ctx, store.EnrollmentCountsParams{WorkspaceID: b.WorkspaceID, BroadcastID: b.ID})
	if err != nil {
		return err
	}
	var cur map[string]int64
	_ = json.Unmarshal(locked.Counts, &cur)
	delta := map[string]int64{}
	for k, v := range extra {
		if v != 0 {
			delta[k] = v
		}
	}
	for k, v := range enrollmentCountsSet(r) {
		if d := v - cur[k]; d != 0 {
			delta[k] += d
		}
	}
	if len(delta) == 0 {
		return nil
	}
	return q.AddBroadcastCounts(ctx, store.AddBroadcastCountsParams{WorkspaceID: b.WorkspaceID, ID: b.ID, Delta: mustJSON(delta)})
}

type TriggeredDueArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	BroadcastID uuid.UUID `json:"broadcast_id"`
}

func (TriggeredDueArgs) Kind() string { return "triggered_due" }

type triggeredDueWorker struct {
	river.WorkerDefaults[TriggeredDueArgs]
	s *Server
}

func (w *triggeredDueWorker) Timeout(*river.Job[TriggeredDueArgs]) time.Duration {
	return 2 * time.Minute
}

func (w *triggeredDueWorker) Work(ctx context.Context, job *river.Job[TriggeredDueArgs]) error {
	wait, err := w.s.ProcessTriggered(ctx, job.Args.WorkspaceID, job.Args.BroadcastID, triggeredBudget)
	if err != nil {
		return err
	}
	if wait >= 0 {
		return river.JobSnooze(wait)
	}
	return nil
}

// ProcessTriggered answers how long to wait before calling it again: the next due time, at most
// 5 minutes ahead, or -1 when nobody waits or it is not active.
func (s *Server) ProcessTriggered(ctx context.Context, ws, id uuid.UUID, budget time.Duration) (time.Duration, error) {
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return -1, err
	}
	deadline := time.Now().Add(budget)
	b, err := s.st.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: id})
	if store.IsNotFound(err) {
		return -1, nil
	}
	if err != nil {
		return -1, err
	}
	if b.Kind != bcTriggered || b.State != bcActive {
		return -1, nil
	}
	_, from, err := s.broadcastFrom(ctx, s.st.Queries, ws, b.ChannelID)
	if err != nil {
		return -1, s.senderFailure(ctx, ws, id, err)
	}
	domain := email.Domain(from)
	var doc design.Document
	if b.Document != nil {
		if d, issues, err := design.Parse(b.Document); err == nil && len(issues) == 0 {
			doc = d
		}
	}
	for {
		done, queued := false, false
		err := s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
			cur, err := q.LockBroadcast(ctx, store.LockBroadcastParams{WorkspaceID: ws, ID: id})
			if err != nil {
				return err
			}
			if cur.State != bcActive {
				done = true
				return nil
			}
			now := s.now()
			rows, err := q.LockDueEnrollments(ctx, store.LockDueEnrollmentsParams{WorkspaceID: ws, BroadcastID: id, DueBefore: now.Add(-delayOf(cur)), Lim: enrollmentBatch})
			if err != nil {
				return err
			}
			if len(rows) < enrollmentBatch {
				done = true
			}
			if len(rows) == 0 {
				return s.syncEnrollmentCounts(ctx, q, cur, nil)
			}
			n, err := s.decideEnrollments(ctx, q, cur, rows, doc, domain, now)
			if err != nil {
				return err
			}
			if n > 0 {
				queued = true
				if cur.ChannelID != nil {
					s.queueBroadcastSend(events, ws, *cur.ChannelID)
				}
			}
			return s.syncEnrollmentCounts(ctx, q, cur, map[string]int64{"recipients": n, "queued": n})
		})
		if err != nil {
			return -1, err
		}
		if queued {
			if cur, err := s.st.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: id}); err == nil {
				s.broadcastProgress(ctx, cur)
			}
		}
		if done {
			break
		}
		if time.Now().After(deadline) {
			return 0, nil
		}
	}
	cur, err := s.st.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: id})
	if err != nil || cur.State != bcActive {
		return -1, err
	}
	next, err := s.st.NextWaitingEnrollment(ctx, store.NextWaitingEnrollmentParams{WorkspaceID: ws, BroadcastID: id})
	if err != nil || next.Waiting == 0 {
		return -1, err
	}
	now := s.now()
	return max(paceWake(next.TriggeredAt.Add(delayOf(cur)), now).Sub(now), time.Second), nil
}

// The skip reasons are checked in the order docs/architecture.md lists them.
func (s *Server) decideEnrollments(ctx context.Context, q *store.Queries, b store.Broadcast, rows []store.LockDueEnrollmentsRow, doc design.Document, domain string, now time.Time) (int64, error) {
	ws := b.WorkspaceID
	a := parseAudience(b.Audience)
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ContactID
	}
	cands, err := q.TriggeredCandidates(ctx, store.TriggeredCandidatesParams{
		WorkspaceID: ws, InboxID: b.InboxID, ListIds: a.ListIDs, Conditions: mustJSON(a.Conditions), Locales: a.Locales, ContactIds: ids,
	})
	if err != nil {
		return 0, err
	}
	byID := make(map[uuid.UUID]store.TriggeredCandidatesRow, len(cands))
	addrOf := func(c store.TriggeredCandidatesRow) string {
		if c.ListEmail != "" {
			return c.ListEmail
		}
		return c.Email
	}
	var emails []string
	for _, c := range cands {
		byID[c.ContactID] = c
		if addr := addrOf(c); addr != "" {
			emails = append(emails, addr)
		}
	}
	suppressed := map[string]bool{}
	opted := map[string]bool{}
	if len(emails) > 0 {
		found, err := q.SuppressedEmailsIn(ctx, store.SuppressedEmailsInParams{WorkspaceID: ws, Emails: emails})
		if err != nil {
			return 0, err
		}
		for _, e := range found {
			suppressed[e] = true
		}
		hashes, err := s.optOutHashes(ctx, q, ws, emails...)
		if err != nil {
			return 0, err
		}
		hit, err := q.ListOptOutHashes(ctx, store.ListOptOutHashesParams{WorkspaceID: ws, Hashes: hashes})
		if err != nil {
			return 0, err
		}
		set := map[string]bool{}
		for _, h := range hit {
			set[string(h)] = true
		}
		for i, e := range emails {
			if set[string(hashes[i])] {
				opted[e] = true
			}
		}
	}
	service := serviceBasis(b)
	var skip store.SkipEnrollmentsParams
	var ins store.InsertBroadcastDeliveriesParams
	for _, r := range rows {
		c, ok := byID[r.ContactID]
		addr := addrOf(c)
		reason := ""
		switch {
		case !ok || c.Blocked:
			reason = string(oas.TriggeredSkipReasonBlocked)
		case addr == "":
			reason = string(oas.TriggeredSkipReasonNoAddress)
		case enrollmentDue(b, r.TriggeredAt, r.Source).Before(now.Add(-lateAfter)):
			reason = string(oas.TriggeredSkipReasonLate)
		case suppressed[addr]:
			reason = string(oas.TriggeredSkipReasonSuppressed)
		case opted[addr]:
			reason = string(oas.TriggeredSkipReasonOptedOut)
		case service && c.Stopped:
			reason = string(oas.TriggeredSkipReasonStopped)
		case !service && c.ListID == uuid.Nil():
			reason = string(oas.TriggeredSkipReasonNotSubscribed)
		case !c.Matches:
			reason = string(oas.TriggeredSkipReasonNotMatching)
		}
		if reason != "" {
			skip.ContactIds = append(skip.ContactIds, r.ContactID)
			skip.Reasons = append(skip.Reasons, reason)
			continue
		}
		token := email.NewConversationToken()
		ins.Ids = append(ins.Ids, newID())
		ins.ContactIds = append(ins.ContactIds, r.ContactID)
		ins.ListIds = append(ins.ListIds, c.ListID)
		ins.Emails = append(ins.Emails, addr)
		ins.Tokens = append(ins.Tokens, token)
		ins.Headers = append(ins.Headers, email.NewMessageID(token, domain))
		tag, _ := doc.Pick(deref(c.Locale))
		ins.Locales = append(ins.Locales, tag)
	}
	if len(skip.ContactIds) > 0 {
		skip.WorkspaceID, skip.BroadcastID, skip.Now = ws, b.ID, now
		if err := q.SkipEnrollments(ctx, skip); err != nil {
			return 0, err
		}
	}
	if len(ins.Ids) == 0 {
		return 0, nil
	}
	ins.WorkspaceID, ins.BroadcastID, ins.Now = ws, b.ID, now
	n, err := q.InsertBroadcastDeliveries(ctx, ins)
	if err != nil {
		return 0, err
	}
	if err := q.QueueEnrollments(ctx, store.QueueEnrollmentsParams{WorkspaceID: ws, BroadcastID: b.ID, ContactIds: ins.ContactIds, Now: now}); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Server) backfillTriggered(ctx context.Context, ws, id uuid.UUID, deadline time.Time) (time.Duration, error) {
	for {
		done := false
		err := s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
			cur, err := q.LockBroadcast(ctx, store.LockBroadcastParams{WorkspaceID: ws, ID: id})
			if err != nil {
				return err
			}
			if cur.Kind != bcTriggered || (cur.State != bcActive && cur.State != bcPaused) || cur.SnapshotDone || cur.StartedAt == nil {
				done = true
				return nil
			}
			after := uuid.Nil()
			if cur.SnapshotAfter != nil {
				after = *cur.SnapshotAfter
			}
			rows, err := backfillPageOf(ctx, q, cur, after)
			if err != nil {
				return err
			}
			if len(rows) > 0 {
				p := store.InsertBackfillEnrollmentsParams{WorkspaceID: ws, BroadcastID: id, Now: s.now()}
				for _, r := range rows {
					p.ContactIds = append(p.ContactIds, r.ContactID)
					p.TriggeredAts = append(p.TriggeredAts, r.TriggeredAt)
				}
				if _, err := q.InsertBackfillEnrollments(ctx, p); err != nil {
					return err
				}
			}
			if len(rows) == backfillPage {
				cur.SnapshotAfter = &rows[len(rows)-1].ContactID
				_, err = s.saveBroadcast(ctx, q, cur)
				return err
			}
			done = true
			cur.SnapshotAfter, cur.SnapshotDone = nil, true
			if cur, err = s.saveBroadcast(ctx, q, cur); err != nil {
				return err
			}
			if err := s.syncEnrollmentCounts(ctx, q, cur, nil); err != nil {
				return err
			}
			events.jobOnce(TriggeredDueArgs{WorkspaceID: ws, BroadcastID: id}, broadcastJobOpts(time.Time{}, broadcastQueue))
			return nil
		})
		if err != nil {
			return -1, err
		}
		if done {
			return -1, nil
		}
		if time.Now().After(deadline) {
			return 0, nil
		}
	}
}

func backfillPageOf(ctx context.Context, q *store.Queries, b store.Broadcast, after uuid.UUID) ([]store.BackfillIdentifiedPageRow, error) {
	start := *b.StartedAt
	from := start.AddDate(0, 0, -int(b.BackfillDays))
	t := triggerOf(b)
	switch {
	case t.Event == triggerSubscribed && t.ListID != nil:
		rows, err := q.BackfillSubscribedPage(ctx, store.BackfillSubscribedPageParams{
			WorkspaceID: b.WorkspaceID, ListID: *t.ListID, After: after, WindowFrom: from, WindowUntil: start, Lim: backfillPage,
		})
		out := make([]store.BackfillIdentifiedPageRow, len(rows))
		for i, r := range rows {
			out[i] = store.BackfillIdentifiedPageRow(r)
		}
		return out, err
	case t.Event == triggerIdentified:
		return q.BackfillIdentifiedPage(ctx, store.BackfillIdentifiedPageParams{
			WorkspaceID: b.WorkspaceID, InboxID: b.InboxID, After: after, WindowFrom: from, WindowUntil: start, Lim: backfillPage,
		})
	}
	return nil, nil
}

func backfillCount(ctx context.Context, q *store.Queries, b store.Broadcast, now time.Time) (int64, error) {
	if b.BackfillDays == 0 {
		return 0, nil
	}
	from := now.AddDate(0, 0, -int(b.BackfillDays))
	t := triggerOf(b)
	switch {
	case t.Event == triggerSubscribed && t.ListID != nil:
		return q.CountBackfillSubscribed(ctx, store.CountBackfillSubscribedParams{WorkspaceID: b.WorkspaceID, ListID: *t.ListID, WindowFrom: from, WindowUntil: now})
	case t.Event == triggerIdentified:
		return q.CountBackfillIdentified(ctx, store.CountBackfillIdentifiedParams{WorkspaceID: b.WorkspaceID, InboxID: b.InboxID, WindowFrom: from, WindowUntil: now})
	}
	return 0, nil
}

// A back-filled contact is due no earlier than the start, so its trigger's age never makes it late.
func enrollmentDue(b store.Broadcast, triggered time.Time, source string) time.Time {
	at := triggered.Add(delayOf(b))
	if source == enrollBackfill && b.StartedAt != nil && b.StartedAt.After(at) {
		return *b.StartedAt
	}
	return at
}

func sendsAt(b store.Broadcast, triggered time.Time, source string, now time.Time) time.Time {
	at := enrollmentDue(b, triggered, source)
	if at.Before(now) && b.State == bcActive {
		at = now
	}
	if w, ok := windowOf(b); ok {
		if open, opens, _ := w.at(at); !open {
			at = opens
		}
	}
	return at
}

func (s *Server) ListBroadcastEnrollments(ctx context.Context, req oas.ListBroadcastEnrollmentsRequestObject) (oas.ListBroadcastEnrollmentsResponseObject, error) {
	p := principalFrom(ctx)
	b, err := visibleBroadcast(ctx, s.st.Queries, p, req.BroadcastId, false)
	if err != nil {
		return nil, err
	}
	if b.Kind != bcTriggered {
		return nil, errNotTriggered
	}
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, cid, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	var state *string
	if req.Params.State != nil {
		if !req.Params.State.Valid() {
			return nil, errValidation("state must be waiting, queued, skipped or cancelled")
		}
		state = new(string(*req.Params.State))
	}
	oldest := state != nil && *state == enrollWaiting
	rows, err := s.st.ListEnrollments(ctx, store.ListEnrollmentsParams{
		WorkspaceID: p.workspaceID, BroadcastID: b.ID, State: state, CursorAt: at, CursorID: cid, OldestFirst: oldest, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].TriggeredAt, rows[lim-1].ContactID)
		next = &c
	}
	now := s.now()
	out := oas.ListBroadcastEnrollments200JSONResponse{Items: make([]oas.TriggeredEnrollment, len(rows)), NextCursor: next}
	for i, r := range rows {
		e := oas.TriggeredEnrollment{
			Contact: oas.TriggeredEnrollmentContact{Id: r.ContactID, Name: r.ContactName}, State: oas.TriggeredEnrollmentState(r.State),
			TriggeredAt: r.TriggeredAt, DeliveryId: r.DeliveryID, Source: oas.TriggeredEnrollmentSource(r.Source), CreatedAt: r.CreatedAt,
		}
		if r.ContactEmail != "" {
			e.Contact.Email = new(oas.Email(r.ContactEmail))
		}
		if r.SkipReason != nil {
			e.SkipReason = new(oas.TriggeredSkipReason(*r.SkipReason))
		}
		if r.State == enrollWaiting {
			e.SendsAt = new(sendsAt(b, r.TriggeredAt, r.Source, now))
		}
		out.Items[i] = e
	}
	return out, nil
}

func (s *Server) RemoveBroadcastEnrollment(ctx context.Context, req oas.RemoveBroadcastEnrollmentRequestObject) (oas.RemoveBroadcastEnrollmentResponseObject, error) {
	p := principalFrom(ctx)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		b, err := visibleBroadcast(ctx, q, p, req.BroadcastId, true)
		if err != nil {
			return err
		}
		if err := s.requireBroadcastWriter(p); err != nil {
			return err
		}
		if b.Kind != bcTriggered {
			return errEnrollmentGone
		}
		n, err := q.RemoveEnrollment(ctx, store.RemoveEnrollmentParams{WorkspaceID: p.workspaceID, BroadcastID: b.ID, ContactID: req.ContactId, Now: s.now()})
		if err != nil {
			return err
		}
		if n == 0 {
			found, err := q.EnrollmentExists(ctx, store.EnrollmentExistsParams{WorkspaceID: p.workspaceID, BroadcastID: b.ID, ContactID: req.ContactId})
			if err != nil {
				return err
			}
			if found {
				return errBroadcastState
			}
			return errEnrollmentGone
		}
		if err := s.syncEnrollmentCounts(ctx, q, b, nil); err != nil {
			return err
		}
		out, err := q.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: p.workspaceID, ID: b.ID})
		if err != nil {
			return err
		}
		inbox := out.InboxID
		events.add(eventBroadcastUpdated, &inbox, nil, oas.BroadcastEventData{Broadcast: s.broadcastBody(out), EnrollmentRemoved: &req.ContactId})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.RemoveBroadcastEnrollment204Response{}, nil
}

func triggerSentence(b store.Broadcast, inbox, list string) string {
	when := "right after"
	if d := delayOf(b); d > 0 {
		when = aboutDuration(d.Seconds()) + " after"
	}
	t := triggerOf(b)
	what := " a contact first signs in to " + inbox
	switch {
	case t.Event == triggerSubscribed:
		what = " a contact subscribes to " + list
	case t.Event == triggerAttribute && t.Condition != nil:
		c := t.Condition
		what = " a contact newly matches " + c.Attribute + " " + c.Op
		if c.Value != nil {
			what += " " + strconv.Quote(*c.Value)
		} else if len(c.Values) > 0 {
			what += " " + strings.Join(c.Values, ", ")
		}
	}
	return when + what + ", once each"
}

// triggerList is the name of a subscribed trigger's list; ok is false when the list is gone.
func triggerList(ctx context.Context, q *store.Queries, b store.Broadcast) (string, bool, error) {
	t := triggerOf(b)
	if t.Event != triggerSubscribed || t.ListID == nil {
		return "", true, nil
	}
	l, err := q.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: b.WorkspaceID, ID: *t.ListID})
	if store.IsNotFound(err) {
		return "", false, nil
	}
	return l.Name, err == nil, err
}

func (l *reviewList) triggered(b store.Broadcast, inbox store.Inbox, list string, listOK bool, doc *design.Document, backfill int64) {
	if listOK {
		l.add(oas.BroadcastReviewKeyTrigger, oas.ReviewOk, triggerSentence(b, inbox.Name, list))
	} else {
		l.add(oas.BroadcastReviewKeyTrigger, oas.ReviewBlocking, "the list the trigger names was deleted: stop this triggered e-mail and create another")
	}
	var fields []design.Field
	if doc != nil {
		fields = doc.MergeFields()
		for _, tag := range doc.Versions() {
			fields = append(fields, doc.Localized(tag).MergeFields()...)
			fields = append(fields, design.Fields(broadcastSubject(b, tag))...)
		}
	}
	fields = append(fields, design.Fields(strings.TrimSpace(b.Subject))...)
	service := serviceBasis(b)
	switch {
	case service && slices.ContainsFunc(fields, func(f design.Field) bool { return f.Name == "list.name" }):
		l.add(oas.BroadcastReviewKeyBasis, oas.ReviewBlocking, "service mail goes to no list: remove the list.name merge field")
	case service:
		l.add(oas.BroadcastReviewKeyBasis, oas.ReviewWarning, "service mail may only be about using "+inbox.Name+
			" (getting started, the account, a request for feedback), never advertising or offers; each mail lets the person stop it")
	default:
		l.add(oas.BroadcastReviewKeyBasis, oas.ReviewOk, "only subscribers of the lists get it")
	}
	lists := len(parseAudience(b.Audience).ListIDs)
	switch {
	case !service && lists == 0:
		l.add(oas.BroadcastReviewKeyAudienceNonempty, oas.ReviewBlocking, "choose at least one list")
	case b.BackfillDays > 0:
		l.add(oas.BroadcastReviewKeyAudienceNonempty, oas.ReviewOk, fmt.Sprintf("%d from the last %d days, then everyone the trigger enrolls", backfill, b.BackfillDays))
	default:
		l.add(oas.BroadcastReviewKeyAudienceNonempty, oas.ReviewOk, "everyone the trigger enrolls from the start on")
	}
	var bare []string
	for _, f := range requiredFields(fields) {
		bare = append(bare, f.Name)
	}
	if len(bare) > 0 {
		slices.Sort(bare)
		l.add(oas.BroadcastReviewKeyMergeFields, oas.ReviewWarning, strings.Join(bare, ", ")+" has no fallback: contacts without it get an empty value")
	} else {
		l.add(oas.BroadcastReviewKeyMergeFields, oas.ReviewOk, "")
	}
}

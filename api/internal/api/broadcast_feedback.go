package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/metrics"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	warmupStart        = 500
	warmupMinSent      = 100
	warmupMaxDoublings = 20
	rateWindow         = 30 * 24 * time.Hour
	sendingDaysKept    = 90 * 24 * time.Hour
	deliveriesKept     = 400 * 24 * time.Hour
	replyNotifyPeriod  = 10 * time.Minute
	scannerWindow      = 2 * time.Second

	noticeBroadcastApproval  = "broadcast_approval"
	noticeBroadcastSuspended = "broadcast_suspended"
	noticeBroadcastPaused    = "broadcast_paused"
)

// warmupCap is the daily limit of an approved workspace on an approval server: 500 recipients,
// doubled for every clean day (at least 100 sent, under 3% bounces and 0.1% complaints); a bad day
// takes one doubling back.
func warmupCap(days []store.BroadcastSendingDay, today time.Time) int64 {
	d := 0
	for _, day := range days {
		if !day.Day.Before(today) || day.Sent < warmupMinSent {
			continue
		}
		if day.Bounced*100 < 3*day.Sent && day.Complained*1000 < day.Sent {
			d = min(d+1, warmupMaxDoublings)
		} else {
			d = max(d-1, 0)
		}
	}
	return int64(warmupStart) << d
}

// dailyAllowance is the workspace's limit of broadcast recipients today (UTC) and how many it sent;
// limit is -1 without one.
func (s *Server) dailyAllowance(ctx context.Context, q *store.Queries, w store.Workspace) (limit, sent int64, err error) {
	now := s.now()
	today := utcDay(now)
	days, err := q.ListSendingDays(ctx, store.ListSendingDaysParams{WorkspaceID: w.ID, Since: today.Add(-sendingDaysKept)})
	if err != nil {
		return -1, 0, err
	}
	for _, d := range days {
		if d.Day.Equal(today) {
			sent = int64(d.Sent)
		}
	}
	limit = -1
	tighter := func(n int64) {
		if limit < 0 || n < limit {
			limit = n
		}
	}
	if s.broadcasts.DailyRecipients > 0 {
		tighter(s.broadcasts.DailyRecipients)
	}
	if w.BroadcastDailyLimit != nil {
		tighter(int64(*w.BroadcastDailyLimit))
	}
	if s.broadcastMode() == broadcastsApproval {
		tighter(warmupCap(days, today))
	}
	return limit, sent, nil
}

// suppressBounced stops all mail to an address that bounced for good.
func (s *Server) suppressBounced(ctx context.Context, q *store.Queries, events *eventBatch, ws uuid.UUID, addr, detail string) error {
	if err := q.SuppressEmail(ctx, store.SuppressEmailParams{WorkspaceID: ws, Email: addr, Reason: suppressBounce, Detail: truncateRunes(detail, 2000), Now: s.now()}); err != nil {
		return err
	}
	return s.contactChanged(ctx, q, events, ws, addr)
}

func (s *Server) contactChanged(ctx context.Context, q *store.Queries, events *eventBatch, ws uuid.UUID, addr string) error {
	id, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: ws, Email: addr})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	row, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: id})
	if err != nil {
		return err
	}
	body, err := s.contactBody(ctx, q, ws, allInboxScope, row)
	if err != nil {
		return err
	}
	events.add(realtime.ContactUpdated, nil, nil, body)
	return nil
}

func countDelta(key string) []byte { return mustJSON(map[string]int64{key: 1}) }

// broadcastBounced records a hard bounce reported after sending (SES, a delivery report).
func (s *Server) broadcastBounced(ctx context.Context, q *store.Queries, events *eventBatch, d store.BroadcastDelivery, detail string) error {
	n, err := q.MarkDeliveryBounced(ctx, store.MarkDeliveryBouncedParams{WorkspaceID: d.WorkspaceID, ID: d.ID, Now: s.now(), Error: optionalText(detail, maxDeliveryError)})
	if err != nil {
		return err
	}
	if n == 1 {
		delta := map[string]int64{"bounced": 1}
		if d.State == deliveryQueued {
			delta["queued"] = -1
		}
		if err := q.AddBroadcastCounts(ctx, store.AddBroadcastCountsParams{WorkspaceID: d.WorkspaceID, ID: d.BroadcastID, Delta: mustJSON(delta)}); err != nil {
			return err
		}
		if err := q.AddSendingDay(ctx, store.AddSendingDayParams{WorkspaceID: d.WorkspaceID, Day: utcDay(s.now()), Bounced: 1}); err != nil {
			return err
		}
	}
	return s.suppressBounced(ctx, q, events, d.WorkspaceID, d.Email, detail)
}

// broadcastComplained records a recipient who called a broadcast spam: broadcasts stop for the
// address, it leaves every list of the workspace and its objection is remembered. Support replies
// can still reach it.
func (s *Server) broadcastComplained(ctx context.Context, q *store.Queries, events *eventBatch, d store.BroadcastDelivery, detail string) error {
	ws := d.WorkspaceID
	n, err := q.MarkDeliveryComplained(ctx, store.MarkDeliveryComplainedParams{WorkspaceID: ws, ID: d.ID, Now: s.now()})
	if err != nil {
		return err
	}
	if n == 1 {
		metrics.BroadcastComplaints.Inc()
		if err := q.AddBroadcastCounts(ctx, store.AddBroadcastCountsParams{WorkspaceID: ws, ID: d.BroadcastID, Delta: countDelta("complained")}); err != nil {
			return err
		}
		if err := q.AddSendingDay(ctx, store.AddSendingDayParams{WorkspaceID: ws, Day: utcDay(s.now()), Complained: 1}); err != nil {
			return err
		}
	}
	if err := q.SuppressEmailScoped(ctx, store.SuppressEmailScopedParams{
		WorkspaceID: ws, Email: d.Email, Reason: suppressComplaint, Detail: truncateRunes(detail, 2000), Scope: "broadcasts", Now: s.now(),
	}); err != nil {
		return err
	}
	if err := s.complainedAbout(ctx, q, events, ws, d.Email); err != nil {
		return err
	}
	return s.contactChanged(ctx, q, events, ws, d.Email)
}

// deliveryFeedback applies a bounce or complaint to a delivery, then the automatic pauses.
func (s *Server) deliveryFeedback(ctx context.Context, d store.BroadcastDelivery, complaint bool, detail string) error {
	err := s.inTx(ctx, d.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		if complaint {
			return s.broadcastComplained(ctx, q, events, d, detail)
		}
		return s.broadcastBounced(ctx, q, events, d, detail)
	})
	if err != nil {
		return err
	}
	return s.checkBroadcastRates(ctx, d.WorkspaceID, d.BroadcastID)
}

// checkBroadcastRates pauses a broadcast whose bounces or complaints since it started or was
// resumed reach the limits, and suspends a workspace whose last 30 days reach them.
func (s *Server) checkBroadcastRates(ctx context.Context, ws, id uuid.UUID) error {
	b, err := s.st.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: id})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if b.State == bcSending || b.State == bcPreparing {
		since := b.ResumedAt
		if since == nil {
			since = b.StartedAt
		}
		if since == nil {
			since = &b.CreatedAt
		}
		r, err := s.st.BroadcastRates(ctx, store.BroadcastRatesParams{WorkspaceID: ws, BroadcastID: id, Since: since})
		if err != nil {
			return err
		}
		reason, detail := oas.BroadcastPauseReason(""), ""
		switch {
		case r.Tried >= 200 && r.Bounced*100 >= 5*r.Tried:
			reason = oas.BroadcastPauseReasonBounceRate
			detail = fmt.Sprintf("%d of %d mails bounced (%.1f%%)", r.Bounced, r.Tried, 100*rate(r.Bounced, r.Tried))
		case r.Sent >= 1000 && r.Complained*1000 >= 3*r.Sent:
			reason = oas.BroadcastPauseReasonComplaintRate
			detail = fmt.Sprintf("%d of %d recipients reported it as spam (%.2f%%)", r.Complained, r.Sent, 100*rate(r.Complained, r.Sent))
		}
		if reason != "" {
			paused := false
			err := s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
				var err error
				paused, err = s.pauseBroadcast(ctx, q, events, id, ws, reason, detail)
				return err
			})
			if err != nil {
				return err
			}
			if paused {
				s.noteBroadcastPaused(ctx, b, string(reason)+": "+detail)
			}
		}
	}
	return s.checkWorkspaceRates(ctx, ws)
}

// checkWorkspaceRates suspends the workspace from sending when, over the last 30 days, 0.3% of at
// least 1,000 broadcast mails were reported as spam or 8% of at least 2,000 bounced.
func (s *Server) checkWorkspaceRates(ctx context.Context, ws uuid.UUID) error {
	days, err := s.st.ListSendingDays(ctx, store.ListSendingDaysParams{WorkspaceID: ws, Since: utcDay(s.now()).Add(-rateWindow)})
	if err != nil {
		return err
	}
	var sent, bounced, complained int64
	for _, d := range days {
		sent, bounced, complained = sent+int64(d.Sent), bounced+int64(d.Bounced), complained+int64(d.Complained)
	}
	why := ""
	switch {
	case sent >= 1000 && complained*1000 >= 3*sent:
		why = fmt.Sprintf("%.2f%% of the %d broadcast mails of the last 30 days were reported as spam", 100*rate(complained, sent), sent)
	case sent >= 2000 && bounced*100 >= 8*sent:
		why = fmt.Sprintf("%.1f%% of the %d broadcast mails of the last 30 days bounced", 100*rate(bounced, sent), sent)
	default:
		return nil
	}
	return s.suspendBroadcasts(ctx, ws, why, true)
}

// suspendBroadcasts stops a workspace from sending and pauses what is running. byRates tells the
// operator digest whether the server decided it.
func (s *Server) suspendBroadcasts(ctx context.Context, ws uuid.UUID, why string, byRates bool) error {
	changed := false
	err := s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
		if _, err := q.LockWorkspace(ctx, ws); err != nil {
			return err
		}
		cur, err := q.GetWorkspace(ctx, ws)
		if err != nil {
			return err
		}
		if cur.BroadcastsStatus == wsSuspended && byRates {
			return nil
		}
		if _, err = q.SetWorkspaceBroadcastsStatus(ctx, store.SetWorkspaceBroadcastsStatusParams{ID: ws, Status: wsSuspended, Reason: optionalText(why, 500), Now: s.now()}); err != nil {
			return err
		}
		changed = true
		running, err := q.ListRunningBroadcasts(ctx, ws)
		if err != nil {
			return err
		}
		for _, b := range running {
			if _, err := s.pauseBroadcast(ctx, q, events, b.ID, ws, oas.BroadcastPauseReasonWorkspaceSuspended, why); err != nil {
				return err
			}
		}
		if byRates {
			return s.noteBroadcastOperator(ctx, q, noticeBroadcastSuspended, "", cur.Name, why)
		}
		return s.audit(ctx, q, principal{workspaceID: ws}, oas.AuditWorkspaceBroadcastsSuspended, oas.AuditTargetWorkspace, ws,
			auditDetails{"reason": why, "paused_broadcasts": len(running)})
	})
	if err != nil || !changed {
		return err
	}
	s.log.WarnContext(ctx, "workspace suspended from broadcasts", slog.String("workspace_id", ws.String()), slog.String("reason", why))
	return nil
}

// noteBroadcastOperator records a digest line for the operator: approval requests, suspensions and
// automatic pauses, with rates and names but no mail content or addresses of recipients.
func (s *Server) noteBroadcastOperator(ctx context.Context, q *store.Queries, kind, from, workspace, detail string) error {
	if len(s.operator) == 0 {
		return nil
	}
	return q.CreateOperatorNotice(ctx, store.CreateOperatorNoticeParams{
		ID: newID(), Kind: kind, Email: from, WorkspaceName: &workspace, Detail: optionalText(detail, 2000), CreatedAt: s.now(),
	})
}

func (s *Server) noteBroadcastPaused(ctx context.Context, b store.Broadcast, detail string) {
	w, err := s.st.GetWorkspace(ctx, b.WorkspaceID)
	if err == nil {
		err = s.noteBroadcastOperator(ctx, s.st.Queries, noticeBroadcastPaused, "", w.Name, "broadcast "+b.ID.String()+" paused: "+detail)
	}
	if err != nil {
		s.log.ErrorContext(ctx, "operator notice", slog.Any("error", err))
	}
}

// deliveryOfTokens finds the delivery whose Message-ID a reply names, by the token in ours or by the
// one its relay put in place of ours, for a broadcast of the receiving channel's inbox.
func deliveryOfTokens(ctx context.Context, q *store.Queries, ch store.FindEmailChannelByAddressRow, m *email.Message, contactID uuid.UUID) (*store.BroadcastDelivery, error) {
	ids := slices.Concat(m.InReplyTo, m.References)
	if len(ids) == 0 {
		return nil, nil
	}
	var tokens []string
	for _, id := range ids {
		if t, _, ok := email.TokenFromID(id); ok {
			tokens = append(tokens, t)
		}
	}
	d, err := q.FindBroadcastDeliveryByTokens(ctx, store.FindBroadcastDeliveryByTokensParams{WorkspaceID: ch.WorkspaceID, InboxID: ch.InboxID, Tokens: tokens, Ids: ids, ContactID: contactID})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// broadcastAutoReply counts an automatic answer (out of office) to a broadcast instead of storing
// it, when it names no conversation: thousands of them would otherwise fill the inbox after a send.
func (s *Server) broadcastAutoReply(ctx context.Context, ch store.FindEmailChannelByAddressRow, m *email.Message) (bool, error) {
	thread, err := threadOf(ctx, s.st.Queries, ch, m, uuid.Nil())
	if err != nil || thread != nil {
		return false, err
	}
	d, err := deliveryOfTokens(ctx, s.st.Queries, ch, m, uuid.Nil())
	if err != nil {
		return false, err
	}
	if d == nil {
		return s.noticeAutoReply(ctx, ch, m)
	}
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		n, err := q.MarkDeliveryAutoReplied(ctx, store.MarkDeliveryAutoRepliedParams{WorkspaceID: d.WorkspaceID, ID: d.ID, Now: s.now()})
		if err != nil || n == 0 {
			return err
		}
		return q.AddBroadcastCounts(ctx, store.AddBroadcastCountsParams{WorkspaceID: d.WorkspaceID, ID: d.BroadcastID, Delta: countDelta("auto_replied")})
	})
	return err == nil, err
}

// broadcastReplyStart links a conversation opened by a reply to its broadcast; when the delivery's
// own recipient wrote, the timeline starts with a broadcast_reply event and the delivery counts as
// replied.
func (s *Server) broadcastReplyStart(ctx context.Context, q *store.Queries, events *eventBatch, c store.Conversation, d store.BroadcastDelivery, contactID uuid.UUID, now time.Time) (store.Conversation, error) {
	if d.ContactID != contactID {
		return c, nil
	}
	b, err := q.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: c.WorkspaceID, ID: d.BroadcastID})
	if err != nil {
		return c, err
	}
	if err := s.systemEvent(ctx, q, events, c, oas.MessageEvent{Type: oas.BroadcastReply, Broadcast: &oas.BroadcastRef{Id: b.ID, Name: b.Name}}, now); err != nil {
		return c, err
	}
	n, err := q.MarkDeliveryReplied(ctx, store.MarkDeliveryRepliedParams{WorkspaceID: c.WorkspaceID, ID: d.ID, Now: now, ConversationID: &c.ID})
	if err != nil || n == 0 {
		return c, err
	}
	return c, q.AddBroadcastCounts(ctx, store.AddBroadcastCountsParams{WorkspaceID: c.WorkspaceID, ID: d.BroadcastID, Delta: countDelta("replied")})
}

// unsubscribedThrough counts an unsubscribe made with a broadcast's link on its delivery.
func (s *Server) unsubscribedThrough(ctx context.Context, q *store.Queries, ws, broadcast, contact uuid.UUID) error {
	n, err := q.MarkDeliveryUnsubscribed(ctx, store.MarkDeliveryUnsubscribedParams{WorkspaceID: ws, BroadcastID: broadcast, ContactID: contact, Now: s.now()})
	if err != nil || n == 0 {
		return err
	}
	return q.AddBroadcastCounts(ctx, store.AddBroadcastCountsParams{WorkspaceID: ws, ID: broadcast, Delta: countDelta("unsubscribed")})
}

// applyFeedbackReport handles an ARF abuse report (RFC 5965) that reached a channel: only from a
// sender whose DMARC result the receiving server vouches for, and only for mail we sent from this
// workspace to one address that alone was given its Message-ID.
func (s *Server) applyFeedbackReport(ctx context.Context, ch store.FindEmailChannelByAddressRow, m *email.Message, dmarc string) (IngestResult, bool, error) {
	f := m.Feedback
	if f == nil || !f.Complaint() || f.OriginalMessageID == "" || dmarc != email.DMARCPass {
		return IngestResult{}, false, nil
	}
	d, err := s.st.FindBroadcastDeliveryByHeader(ctx, store.FindBroadcastDeliveryByHeaderParams{WorkspaceID: ch.WorkspaceID, HeaderMessageID: f.OriginalMessageID})
	if err == nil {
		if f.OriginalRcptTo != "" && f.OriginalRcptTo != d.Email {
			return IngestResult{}, false, nil
		}
		if ok, err := s.reportConcerns(ctx, ch.WorkspaceID, d.Email, d.HeaderMessageID, d.RelayMessageID); err != nil || !ok {
			return IngestResult{}, false, err
		}
		if err := s.deliveryFeedback(ctx, d, true, "feedback report: "+f.Type); err != nil {
			return IngestResult{}, false, err
		}
		return IngestResult{Status: IngestBounce}, true, nil
	}
	if !store.IsNotFound(err) {
		return IngestResult{}, false, err
	}
	if nd, err := noticeDeliveryOf(ctx, s.st.Queries, ch.WorkspaceID, []string{f.OriginalMessageID}); err != nil {
		return IngestResult{}, false, err
	} else if nd != nil {
		if f.OriginalRcptTo != "" && f.OriginalRcptTo != nd.Email {
			return IngestResult{}, false, nil
		}
		if ok, err := s.reportConcerns(ctx, ch.WorkspaceID, nd.Email, nd.HeaderMessageID, nd.RelayMessageID); err != nil || !ok {
			return IngestResult{}, false, err
		}
		if err := s.noticeFeedback(ctx, *nd, true, "feedback report: "+f.Type); err != nil {
			return IngestResult{}, false, err
		}
		return IngestResult{Status: IngestBounce}, true, nil
	}
	sent, err := s.st.FindOutboundEmailByHeader(ctx, store.FindOutboundEmailByHeaderParams{WorkspaceID: ch.WorkspaceID, HeaderMessageID: f.OriginalMessageID})
	if store.IsNotFound(err) {
		return s.applyConfirmationFeedback(ctx, f)
	}
	if err != nil {
		return IngestResult{}, false, err
	}
	to := soleRecipient(sent)
	if to == "" || (f.OriginalRcptTo != "" && f.OriginalRcptTo != to) {
		return IngestResult{}, false, nil
	}
	if ok, err := s.reportConcerns(ctx, ch.WorkspaceID, to, sent.HeaderMessageID, sent.RelayMessageID); err != nil || !ok {
		return IngestResult{}, false, err
	}
	recipients := []bounced{{email: to, reason: suppressComplaint, permanent: true, detail: "feedback report: " + f.Type}}
	if err := s.recordBounce(ctx, ch.WorkspaceID, &sent.MessageID, recipients, false); err != nil {
		return IngestResult{}, false, err
	}
	return IngestResult{Status: IngestBounce, ConversationID: &sent.ConversationID, MessageID: &sent.MessageID}, true, nil
}

// serveClick sends a click on a tracked link on to its address, counting the recipient once per
// link; HEAD requests and bursts of clicks by link scanners are not counted.
func (s *Server) serveClick(w http.ResponseWriter, r *http.Request) {
	ws, delivery, pos, ok := s.openClickToken(r.PathValue("token"))
	ctx := r.Context()
	if !ok {
		goneListPage(w)
		return
	}
	d, err := s.st.GetBroadcastDelivery(ctx, store.GetBroadcastDeliveryParams{WorkspaceID: ws, ID: delivery})
	if err != nil {
		goneListPage(w)
		return
	}
	link, err := s.st.GetBroadcastLink(ctx, store.GetBroadcastLinkParams{WorkspaceID: ws, BroadcastID: d.BroadcastID, Position: int32(pos)})
	if err != nil {
		goneListPage(w)
		return
	}
	if r.Method == http.MethodGet {
		if err := s.countClick(ctx, d, link); err != nil {
			s.log.ErrorContext(ctx, "count click", slog.Any("error", err))
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, link.Url, http.StatusFound)
}

func (s *Server) countClick(ctx context.Context, d store.BroadcastDelivery, link store.BroadcastLink) error {
	ws := d.WorkspaceID
	return s.st.InTx(ctx, func(q *store.Queries) error {
		recent, err := q.CountRecentBroadcastClicks(ctx, store.CountRecentBroadcastClicksParams{WorkspaceID: ws, DeliveryID: d.ID, Since: s.now().Add(-scannerWindow)})
		if err != nil || recent >= 2 {
			return err
		}
		n, err := q.RecordBroadcastClick(ctx, store.RecordBroadcastClickParams{WorkspaceID: ws, DeliveryID: d.ID, Position: link.Position, Now: s.now()})
		if err != nil || n == 0 {
			return err
		}
		if err := q.CountBroadcastLinkClick(ctx, store.CountBroadcastLinkClickParams{WorkspaceID: ws, BroadcastID: d.BroadcastID, Position: link.Position}); err != nil {
			return err
		}
		if n, err = q.MarkDeliveryClicked(ctx, store.MarkDeliveryClickedParams{WorkspaceID: ws, ID: d.ID, Now: s.now()}); err != nil || n == 0 {
			return err
		}
		return q.AddBroadcastCounts(ctx, store.AddBroadcastCountsParams{WorkspaceID: ws, ID: d.BroadcastID, Delta: countDelta("clicked")})
	})
}

// BroadcastRepliesArgs groups the notifications about replies to one broadcast per member and ten
// minutes.
type BroadcastRepliesArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	BroadcastID uuid.UUID `json:"broadcast_id"`
	MemberID    uuid.UUID `json:"member_id"`
	Until       time.Time `json:"until"`
}

func (BroadcastRepliesArgs) Kind() string { return "broadcast_replies_notify" }

type broadcastRepliesWorker struct {
	river.WorkerDefaults[BroadcastRepliesArgs]
	s *Server
}

func (w *broadcastRepliesWorker) Work(ctx context.Context, job *river.Job[BroadcastRepliesArgs]) error {
	return w.s.NotifyBroadcastReplies(ctx, job.Args)
}

// replyNotifyArgs is the grouped notification a reply to a broadcast causes for a member: one job
// per member and ten-minute period, sent at its end.
func replyNotifyArgs(ws, broadcast, member uuid.UUID, now time.Time) river.InsertManyParams {
	until := now.Truncate(replyNotifyPeriod).Add(replyNotifyPeriod)
	return river.InsertManyParams{
		Args:       BroadcastRepliesArgs{WorkspaceID: ws, BroadcastID: broadcast, MemberID: member, Until: until},
		InsertOpts: &river.InsertOpts{ScheduledAt: until, UniqueOpts: river.UniqueOpts{ByArgs: true}},
	}
}

// NotifyBroadcastReplies pushes "N new replies" to a member for the replies to a broadcast that
// opened conversations in the period ending at a.Until.
func (s *Server) NotifyBroadcastReplies(ctx context.Context, a BroadcastRepliesArgs) error {
	ws := a.WorkspaceID
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live || s.push == nil {
		return err
	}
	b, err := s.st.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: a.BroadcastID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	n, err := s.st.CountBroadcastReplyConversations(ctx, store.CountBroadcastReplyConversationsParams{
		WorkspaceID: ws, BroadcastID: &b.ID, Since: a.Until.Add(-replyNotifyPeriod), Until: a.Until,
	})
	if err != nil || n == 0 {
		return err
	}
	if ok, err := memberHasInbox(ctx, s.st.Queries, ws, b.InboxID, a.MemberID); err != nil || !ok {
		return err
	}
	m, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: ws, ID: a.MemberID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: b.InboxID})
	if err != nil {
		return err
	}
	person, err := s.st.GetPerson(ctx, m.PersonID)
	if err != nil {
		return err
	}
	msg, err := mail.Render("push_broadcast_replies", person.Locale, map[string]any{"Inbox": in.Name, "Broadcast": b.Name, "Count": int(n)})
	if err != nil {
		return err
	}
	subs, err := s.st.ListLivePushSubscriptionIDs(ctx, store.ListLivePushSubscriptionIDsParams{PersonID: m.PersonID, Now: s.now(), WorkspaceID: &ws})
	if err != nil || len(subs) == 0 {
		return err
	}
	inbox := in.ID
	payload := oas.PushNotification{
		Event: oas.NewAsyncConversation, Title: msg.Subject, Body: strings.TrimSpace(msg.Text),
		Url: "/conversations?broadcast_id=" + b.ID.String() + "&workspace_id=" + ws.String(), Tag: "broadcast-" + b.ID.String(),
		WorkspaceId: &ws, InboxId: &inbox,
	}
	var batch []river.InsertManyParams
	for _, id := range subs {
		batch = append(batch, river.InsertManyParams{Args: PushArgs{
			WorkspaceID: &ws, SubscriptionID: id, Payload: mustJSON(payload), TTL: int(asyncNotificationTTL / time.Second), Urgency: "normal",
			Topic: strings.ReplaceAll(b.ID.String(), "-", ""),
		}, InsertOpts: pushOpts()})
	}
	_, err = s.jobs.InsertMany(ctx, batch)
	return err
}

// CleanBroadcasts removes deliveries 400 days after their broadcast finished (the counters stay on
// the broadcast) and daily sending totals after 90 days.
func (s *Server) CleanBroadcasts(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	for _, ws := range ids {
		if _, err := s.st.DeleteOldBroadcastDeliveries(ctx, store.DeleteOldBroadcastDeliveriesParams{WorkspaceID: ws, Before: new(now.Add(-deliveriesKept)), Lim: 5000}); err != nil {
			return err
		}
		if err := s.st.DeleteOldSendingDays(ctx, store.DeleteOldSendingDaysParams{WorkspaceID: ws, Before: utcDay(now).Add(-sendingDaysKept)}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) applyConfirmationFeedback(ctx context.Context, f *email.Feedback) (IngestResult, bool, error) {
	c, err := s.st.FindEmailConfirmationByHeaderAnyWorkspace(ctx, f.OriginalMessageID)
	if store.IsNotFound(err) {
		return IngestResult{}, false, nil
	}
	if err != nil {
		return IngestResult{}, false, err
	}
	if f.OriginalRcptTo != "" && !strings.EqualFold(f.OriginalRcptTo, c.Email) {
		return IngestResult{}, false, nil
	}
	if err := s.confirmationComplained(ctx, c, "feedback report: "+f.Type); err != nil {
		return IngestResult{}, false, err
	}
	return IngestResult{Status: IngestBounce}, true, nil
}

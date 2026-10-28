package api

import (
	"context"
	"errors"
	"fmt"
	"html"
	"maps"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/mail/design"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

type reviewList struct{ items []oas.BroadcastReviewItem }

func (l *reviewList) add(key oas.BroadcastReviewKey, status oas.BroadcastReviewStatus, detail string) {
	it := oas.BroadcastReviewItem{Key: key, Status: status}
	if detail != "" {
		it.Detail = new(detail)
	}
	l.items = append(l.items, it)
}

func (l *reviewList) blocking() []oas.BroadcastReviewItem {
	var out []oas.BroadcastReviewItem
	for _, it := range l.items {
		if it.Status == oas.ReviewBlocking {
			out = append(out, it)
		}
	}
	return out
}

// reviewBroadcast runs the checklist that scheduling needs: every item, its status and, for the
// panel, the audience count and the HTML size.
func (s *Server) reviewBroadcast(ctx context.Context, q *store.Queries, b store.Broadcast) (oas.BroadcastReview, error) {
	ws := b.WorkspaceID
	var l reviewList
	out := oas.BroadcastReview{}
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: b.InboxID})
	if err != nil {
		return out, err
	}
	if deref(in.SenderLegalName) == "" || deref(in.SenderPostalAddress) == "" {
		l.add(oas.BroadcastReviewKeySenderIdentity, oas.ReviewBlocking, "set the inbox's legal name and postal address (Settings, inbox, Sender identity); the footer shows them")
	} else {
		l.add(oas.BroadcastReviewKeySenderIdentity, oas.ReviewOk, "")
	}
	var ch *store.EmailChannel
	if b.ChannelID == nil {
		l.add(oas.BroadcastReviewKeySenderReady, oas.ReviewBlocking, "choose the e-mail channel it goes out through")
	} else if err := s.broadcastChannel(ctx, q, ws, b.InboxID, *b.ChannelID); err != nil {
		var apiErr *apiError
		if !errors.As(err, &apiErr) {
			return out, err
		}
		l.add(oas.BroadcastReviewKeySenderReady, oas.ReviewBlocking, "the channel has no SMTP account or address to send from")
	} else {
		e, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws, ChannelID: *b.ChannelID})
		if err != nil {
			return out, err
		}
		ch = &e
		l.add(oas.BroadcastReviewKeySenderReady, oas.ReviewOk, "")
	}
	w, err := q.GetWorkspace(ctx, ws)
	if err != nil {
		return out, err
	}
	switch {
	case s.broadcastMode() == broadcastsOff:
		l.add(oas.BroadcastReviewKeyApproval, oas.ReviewBlocking, "this server does not send broadcasts")
	case w.BroadcastsStatus == wsApprovalRequired:
		l.add(oas.BroadcastReviewKeyApproval, oas.ReviewBlocking, "ask the server's operator to allow broadcasts (Settings, Workspace, Broadcasts)")
	case w.BroadcastsStatus == wsApprovalRequested:
		l.add(oas.BroadcastReviewKeyApproval, oas.ReviewBlocking, "the server's operator has not answered the approval request yet")
	case w.BroadcastsStatus == wsSuspended:
		l.add(oas.BroadcastReviewKeyApproval, oas.ReviewBlocking, "the workspace is suspended from sending broadcasts")
	default:
		l.add(oas.BroadcastReviewKeyApproval, oas.ReviewOk, "")
	}
	doc, row, err := s.broadcastDocument(ctx, q, b)
	if err != nil {
		return out, err
	}
	var rendered []versionRender
	if doc == nil {
		l.add(oas.BroadcastReviewKeyDesign, oas.ReviewBlocking, "choose a design")
	} else {
		r, err := s.newBroadcastRenderer(ctx, q, b, *doc, nil)
		if err != nil {
			return out, err
		}
		tags := doc.Versions()
		if len(tags) == 0 {
			tags = []string{""}
		}
		tooLarge := ""
		for _, tag := range tags {
			o, err := r.render(r.sample(false), r.firstList(), tag)
			switch {
			case errors.Is(err, design.ErrHTMLTooLarge):
				tooLarge = versionNote(tag)
			case err != nil:
				return out, err
			default:
				rendered = append(rendered, versionRender{tag, o})
			}
		}
		if tooLarge != "" {
			rendered = nil
			l.add(oas.BroadcastReviewKeyDesign, oas.ReviewBlocking, "the design renders to more than 500 KB of HTML"+tooLarge)
		} else {
			l.add(oas.BroadcastReviewKeyDesign, oas.ReviewOk, "")
		}
		if n, _ := doc.HasMessageBlock(); n > 0 {
			l.add(oas.BroadcastReviewKeyMessageBlock, oas.ReviewBlocking, messageBlockReview)
		}
	}
	subject := strings.TrimSpace(b.Subject)
	var count oas.BroadcastAudienceCount
	if b.Kind == bcTriggered {
		if count.Recipients, err = backfillCount(ctx, q, b, s.now()); err != nil {
			return out, err
		}
		l.triggered(b, in, doc, count.Recipients)
	} else {
		if count, err = s.countAudience(ctx, q, b, doc, true); err != nil {
			return out, err
		}
		switch {
		case len(parseAudience(b.Audience).ListIDs) == 0:
			l.add(oas.BroadcastReviewKeyAudienceNonempty, oas.ReviewBlocking, "choose at least one list")
		case count.Recipients == 0:
			l.add(oas.BroadcastReviewKeyAudienceNonempty, oas.ReviewBlocking, "nobody in the lists would get it")
		default:
			l.add(oas.BroadcastReviewKeyAudienceNonempty, oas.ReviewOk, strconv.FormatInt(count.Recipients, 10)+" recipients")
		}
	}
	out.Recipients = count.Recipients
	switch {
	case b.Kind == bcTriggered:
	case len(count.MissingMergeFields) > 0:
		var parts []string
		for f, n := range count.MissingMergeFields {
			parts = append(parts, fmt.Sprintf("%s is empty for %d", f, n))
		}
		slices.Sort(parts)
		l.add(oas.BroadcastReviewKeyMergeFields, oas.ReviewBlocking, strings.Join(parts, "; ")+": give the fields a fallback")
	default:
		l.add(oas.BroadcastReviewKeyMergeFields, oas.ReviewOk, "")
	}
	if len(rendered) > 0 {
		largest := rendered[0]
		var altIn, httpIn []string
		for _, v := range rendered {
			if len(v.out.HTML) > len(largest.out.HTML) {
				largest = v
			}
			if slices.ContainsFunc(v.out.Warnings, func(w design.Issue) bool { return w.Code == "alt_missing" || w.Code == "image_missing" }) {
				altIn = append(altIn, v.tag)
			}
			for _, m := range hrefAttr.FindAllStringSubmatch(v.out.HTML, -1) {
				if u := html.UnescapeString(m[1]); strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, s.linkBase()+"/") {
					httpIn = append(httpIn, v.tag)
					break
				}
			}
		}
		size := int32(len(largest.out.HTML))
		out.HtmlBytes = &size
		switch {
		case size > design.ClipBytes:
			l.add(oas.BroadcastReviewKeyHtmlSize, oas.ReviewBlocking, "over 102 KB of HTML: Gmail cuts the mail off, footer and unsubscribe link included"+versionNote(largest.tag))
		case size > design.NearClipBytes:
			l.add(oas.BroadcastReviewKeyHtmlSize, oas.ReviewWarning, "over 90 KB of HTML, close to where Gmail cuts mail off"+versionNote(largest.tag))
		default:
			l.add(oas.BroadcastReviewKeyHtmlSize, oas.ReviewOk, "")
		}
		if len(altIn) > 0 {
			l.add(oas.BroadcastReviewKeyImagesAlt, oas.ReviewWarning, "an image has no alt text or no image"+versionNote(altIn...))
		} else {
			l.add(oas.BroadcastReviewKeyImagesAlt, oas.ReviewOk, "")
		}
		if len(httpIn) > 0 {
			l.add(oas.BroadcastReviewKeyLinks, oas.ReviewWarning, "a link uses http:, not https:"+versionNote(httpIn...))
		} else {
			l.add(oas.BroadcastReviewKeyLinks, oas.ReviewOk, "")
		}
	}
	switch n := utf8.RuneCountInString(subject); {
	case n == 0:
		l.add(oas.BroadcastReviewKeySubject, oas.ReviewBlocking, "write a subject")
	case n > 300:
		l.add(oas.BroadcastReviewKeySubject, oas.ReviewBlocking, "the subject is longer than 300 characters")
	default:
		var long []string
		if n > subjectWarnRunes {
			long = append(long, "")
			if doc != nil && doc.Versioned() {
				long[0] = doc.Locale
			}
		}
		trans := parseTranslations(b.Translations)
		for _, tag := range slices.Sorted(maps.Keys(trans)) {
			if utf8.RuneCountInString(trans[tag].Subject) > subjectWarnRunes {
				long = append(long, tag)
			}
		}
		if len(long) > 0 {
			l.add(oas.BroadcastReviewKeySubject, oas.ReviewWarning, "inboxes show about the first 80 characters of a subject"+versionNote(long...))
		} else {
			l.add(oas.BroadcastReviewKeySubject, oas.ReviewOk, "")
		}
	}
	if ch != nil {
		status, detail, err := s.deliverabilityReview(ctx, q, *ch)
		if err != nil {
			return out, err
		}
		l.add(oas.BroadcastReviewKeyDeliverability, status, detail)
		sc, err := s.sesBroadcastCheck(ctx, q, *ch, b.InboxID)
		if err != nil {
			return out, err
		}
		if sc.applies {
			l.add(oas.BroadcastReviewKeySesTenant, sc.status, sc.detail)
		}
	}
	limit, sent, err := s.dailyAllowance(ctx, q, w)
	if err != nil {
		return out, err
	}
	n := count.Recipients
	var dailySecs float64
	if limit > 0 && n > limit-sent {
		left := n - max(limit-sent, 0)
		days := int32(1 + (left+limit-1)/limit)
		out.Days = &days
		now := s.now()
		dailySecs = nextUTCMidnight(now).Sub(now).Seconds() + float64(days-2)*86400
		l.add(oas.BroadcastReviewKeyDailyCap, oas.ReviewWarning, fmt.Sprintf("the workspace sends to at most %d recipients a day; sending takes %d days", limit, days))
	} else {
		if limit > 0 {
			out.Days = new(int32(1))
		}
		l.add(oas.BroadcastReviewKeyDailyCap, oas.ReviewOk, "")
	}
	rate, err := s.channelRate(ctx, q, ws, b.ChannelID)
	if err != nil {
		return out, err
	}
	now := s.now()
	gap := paceGap(b, now)
	secs, windowDays := sendingSeconds(b, n, rate, now)
	sending := secs
	if _, ok := windowOf(b); ok {
		sending = secs - windowWait(b, now)
	}
	if n > 0 {
		out.DurationSeconds = new(int64(math.Ceil(max(secs, dailySecs))))
	}
	if windowDays > 0 && (out.Days == nil || int64(*out.Days) < windowDays) {
		out.Days = new(int32(windowDays))
	}
	switch {
	case paceLimited(b, now) && secs > maxPaceDuration.Seconds():
		l.add(oas.BroadcastReviewKeyPace, oas.ReviewBlocking, fmt.Sprintf("at this pace sending would take about %d days; a broadcast may take at most 365", daysOf(secs)))
	case gap == 0 || sending == 0:
		l.add(oas.BroadcastReviewKeyPace, oas.ReviewOk, "")
	default:
		l.add(oas.BroadcastReviewKeyPace, oas.ReviewOk, "about "+aboutDuration(sending))
	}
	l.sendWindow(b, now)
	changed := b.UpdatedAt
	if row != nil && row.UpdatedAt.After(changed) {
		changed = row.UpdatedAt
	}
	if b.LastTestAt == nil || b.LastTestAt.Before(changed) {
		l.add(oas.BroadcastReviewKeyTestSent, oas.ReviewWarning, "send a test to yourself since the last change")
	} else {
		l.add(oas.BroadcastReviewKeyTestSent, oas.ReviewOk, "")
	}
	if b.ClickTracking {
		l.add(oas.BroadcastReviewKeyClickTracking, oas.ReviewWarning, "links point at "+s.linkBase()+" and count who opens them")
	} else {
		l.add(oas.BroadcastReviewKeyClickTracking, oas.ReviewOk, "")
	}
	l.translations(b, doc)
	out.Items = l.items
	out.Ready = len(l.blocking()) == 0
	return out, nil
}

type versionRender struct {
	tag string
	out design.Output
}

func versionNote(tags ...string) string {
	tags = slices.DeleteFunc(slices.Clone(tags), func(t string) bool { return t == "" })
	if len(tags) == 0 {
		return ""
	}
	return " (" + strings.Join(tags, ", ") + ")"
}

// sendWindow warns when the window is closed at the time sending starts: the scheduled time when
// there is one, else now.
func (l *reviewList) sendWindow(b store.Broadcast, now time.Time) {
	w, ok := windowOf(b)
	if !ok {
		l.add(oas.BroadcastReviewKeySendWindow, oas.ReviewOk, "")
		return
	}
	start, when := now, "now"
	if b.ScheduledAt != nil && b.ScheduledAt.After(now) {
		start, when = *b.ScheduledAt, "at the scheduled time"
	}
	open, opens, _ := w.at(start)
	if open {
		l.add(oas.BroadcastReviewKeySendWindow, oas.ReviewOk, "")
		return
	}
	l.add(oas.BroadcastReviewKeySendWindow, oas.ReviewWarning, fmt.Sprintf("the window is closed %s; sending starts at %s %s (%s UTC)",
		when, opens.Format("15:04"), b.Timezone, opens.UTC().Format("2 Jan 2006 15:04")))
}

// windowWait is how long sending waits for the window from its start, now or scheduled_at while
// that is later.
func windowWait(b store.Broadcast, now time.Time) float64 {
	w, ok := windowOf(b)
	if !ok {
		return 0
	}
	start := now
	if b.ScheduledAt != nil && b.ScheduledAt.After(now) {
		start = *b.ScheduledAt
	}
	_, opens, _ := w.at(start)
	return max(opens.Sub(start), 0).Seconds()
}

func (s *Server) ReviewBroadcast(ctx context.Context, req oas.ReviewBroadcastRequestObject) (oas.ReviewBroadcastResponseObject, error) {
	b, err := visibleBroadcast(ctx, s.st.Queries, principalFrom(ctx), req.BroadcastId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.reviewBroadcast(ctx, s.st.Queries, b)
	if err != nil {
		return nil, err
	}
	return oas.ReviewBroadcast200JSONResponse(out), nil
}

func (s *Server) SendBroadcastTest(ctx context.Context, req oas.SendBroadcastTestRequestObject) (oas.SendBroadcastTestResponseObject, error) {
	p := principalFrom(ctx)
	b, err := visibleBroadcast(ctx, s.st.Queries, p, req.BroadcastId, false)
	if err != nil {
		return nil, err
	}
	if err := s.requireBroadcastWriter(p); err != nil {
		return nil, err
	}
	var memberIDs []uuid.UUID
	var contactID *uuid.UUID
	if req.Body != nil {
		if req.Body.MemberIds != nil {
			for _, id := range *req.Body.MemberIds {
				if !slices.Contains(memberIDs, id) {
					memberIDs = append(memberIDs, id)
				}
			}
			if len(memberIDs) == 0 || len(memberIDs) > 5 {
				return nil, errValidation("member_ids names 1 to 5 members")
			}
		}
		contactID = req.Body.ContactId
	}
	want := ""
	if req.Body != nil && req.Body.Locale != nil {
		if want, err = languageTag(string(*req.Body.Locale), "locale"); err != nil {
			return nil, err
		}
	}
	if len(memberIDs) == 0 {
		if p.isKey() {
			return nil, errValidation("an API key names the members to send the test to in member_ids")
		}
		memberIDs = []uuid.UUID{p.memberID}
	}
	members, err := s.st.CountWorkspaceMembersByIDs(ctx, store.CountWorkspaceMembersByIDsParams{WorkspaceID: p.workspaceID, Ids: memberIDs})
	if err != nil {
		return nil, err
	}
	if len(members) != len(memberIDs) {
		return nil, errValidation("member_ids names someone who is not a member of the workspace; tests go to members only")
	}
	if contactID != nil {
		c, err := s.st.GetContact(ctx, store.GetContactParams{WorkspaceID: p.workspaceID, ID: *contactID})
		if store.IsNotFound(err) {
			return nil, errContactGone
		} else if err != nil {
			return nil, err
		}
		if err := visibleContact(ctx, s.st.Queries, p, *contactID); err != nil {
			return nil, err
		}
		if req.Body.Locale == nil {
			want = deref(c.Locale)
		}
	}
	if b.ChannelID == nil || s.broadcastChannel(ctx, s.st.Queries, p.workspaceID, b.InboxID, *b.ChannelID) != nil {
		return nil, errSenderNotReady
	}
	doc, row, err := s.broadcastDocument(ctx, s.st.Queries, b)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, errDesignRequired
	}
	if n, _ := doc.HasMessageBlock(); n > 0 {
		return nil, reviewFailed([]oas.BroadcastReviewItem{{Key: oas.BroadcastReviewKeyMessageBlock, Status: oas.ReviewBlocking, Detail: new(messageBlockReview)}})
	}
	for range members {
		if s.rateLimit(ctx, rateCheck{"broadcast-test:" + p.workspaceID.String(), limit{testsPerHour, time.Hour}}) != nil {
			return nil, errTestLimit
		}
	}
	out := oas.SendBroadcastTest202JSONResponse{SentTo: make([]oas.Email, len(members))}
	tag, _ := doc.Pick(want)
	if tag != "" {
		out.Locale = new(oas.LanguageTag(tag))
	}
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.MarkBroadcastTested(ctx, store.MarkBroadcastTestedParams{WorkspaceID: p.workspaceID, ID: b.ID, Now: s.now()}); err != nil {
			return err
		}
		if row != nil {
			if err := s.snapshotDesign(ctx, q, p, *row, "test", true); err != nil {
				return err
			}
		}
		for i, m := range members {
			out.SentTo[i] = oas.Email(m.Email)
			events.job(BroadcastTestArgs{WorkspaceID: p.workspaceID, BroadcastID: b.ID, MemberID: m.ID, ContactID: contactID, Locale: tag}, listMailOpts())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

const messageBlockReview = "the design has a message block for conversation mail, which a broadcast would send empty; choose a design without one"

func reviewFailed(items []oas.BroadcastReviewItem) *apiError {
	e := problem(http.StatusConflict, "review_failed", "the review found what must change first; see items")
	e.Extra = map[string]any{"items": items}
	return e
}

func (s *Server) ScheduleBroadcast(ctx context.Context, req oas.ScheduleBroadcastRequestObject) (oas.ScheduleBroadcastResponseObject, error) {
	p := principalFrom(ctx)
	b, err := visibleBroadcast(ctx, s.st.Queries, p, req.BroadcastId, false)
	if err != nil {
		return nil, err
	}
	if err := s.requireBroadcastWriter(p); err != nil {
		return nil, err
	}
	now := s.now()
	sendAt := now
	var confirm *int64
	if req.Body != nil {
		at := req.Body.SendAt
		if req.Body.SendAtLocal != nil {
			if at != nil {
				return nil, errValidation("give send_at or send_at_local, not both")
			}
			local, err := parseLocalDateTime(*req.Body.SendAtLocal, zoneOf(b))
			if err != nil {
				return nil, err
			}
			at = &local
		}
		if at != nil {
			if at.After(now.Add(maxScheduleAhead)) {
				return nil, errValidation("send_at must be at most 90 days ahead")
			}
			if at.After(now) {
				sendAt = at.UTC()
			}
		}
		confirm = req.Body.ConfirmRecipients
		if b.Kind != bcTriggered && (req.Body.BackfillDays != nil || req.Body.ConfirmService != nil) {
			return nil, errValidation("backfill_days and confirm_service are for triggered e-mails")
		}
		if req.Body.BackfillDays != nil {
			if *req.Body.BackfillDays < 0 || *req.Body.BackfillDays > maxBackfillDays {
				return nil, errValidation("backfill_days must be 0 to 30")
			}
			b.BackfillDays = *req.Body.BackfillDays
		}
	}
	if b.State != bcDraft {
		return nil, errBroadcastNotDraft
	}
	confirmService := req.Body != nil && boolOr(req.Body.ConfirmService, false)
	if serviceBasis(b) && !confirmService {
		return nil, errServiceUnconfirmed
	}
	w, err := s.st.GetWorkspace(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	if err := s.sendGate(p, w); err != nil {
		return nil, err
	}
	review, err := s.reviewBroadcast(ctx, s.st.Queries, b)
	if err != nil {
		return nil, err
	}
	var blocking []oas.BroadcastReviewItem
	for _, it := range review.Items {
		if it.Status == oas.ReviewBlocking {
			blocking = append(blocking, it)
		}
	}
	if len(blocking) > 0 {
		return nil, reviewFailed(blocking)
	}
	if confirm != nil && *confirm != review.Recipients {
		e := problem(http.StatusConflict, "audience_changed", "the audience is not the size that was confirmed; review it again")
		e.Extra = map[string]any{"recipients": review.Recipients}
		return nil, e
	}
	var out store.Broadcast
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := visibleBroadcast(ctx, q, p, b.ID, true)
		if err != nil {
			return err
		}
		if cur.State != bcDraft {
			return errBroadcastNotDraft
		}
		if !cur.UpdatedAt.Equal(b.UpdatedAt) {
			return problem(http.StatusConflict, "broadcast_changed", "the draft changed while it was reviewed; review it again")
		}
		if err := s.freezeDesign(ctx, q, p, &cur); err != nil {
			return err
		}
		cur.State, cur.ScheduledAt, cur.PauseReason, cur.StateDetail = bcScheduled, &sendAt, nil, nil
		cur.ScheduledBy, cur.ScheduledByKey = p.memberRef(), nil
		if p.isKey() {
			cur.ScheduledByKey = new(p.keyID)
		}
		if cur.Kind == bcTriggered {
			if err := s.checkLiveTriggered(ctx, q, p.workspaceID, cur.InboxID, cur.ID); err != nil {
				return err
			}
			now := s.now()
			cur.BackfillDays = b.BackfillDays
			cur.ServiceConfirmedBy, cur.ServiceConfirmedByKey, cur.ServiceConfirmedAt = nil, nil, nil
			if serviceBasis(cur) {
				cur.ServiceConfirmedBy, cur.ServiceConfirmedByKey, cur.ServiceConfirmedAt = cur.ScheduledBy, cur.ScheduledByKey, &now
			}
			if err := s.audit(ctx, q, p, oas.AuditBroadcastScheduled, oas.AuditTargetBroadcast, cur.ID, auditDetails{
				"name": cur.Name, "kind": cur.Kind, "basis": deref(cur.Basis), "backfill_days": cur.BackfillDays, "service_confirmed": serviceBasis(cur),
			}); err != nil {
				return err
			}
		}
		if out, err = s.saveBroadcast(ctx, q, cur); err != nil {
			return err
		}
		if err := q.SetBroadcastCounts(ctx, store.SetBroadcastCountsParams{WorkspaceID: p.workspaceID, ID: out.ID, Counts: []byte(`{"recipients": 0}`)}); err != nil {
			return err
		}
		if err := s.freezeLinks(ctx, q, out); err != nil {
			return err
		}
		s.emitBroadcast(events, out, bcDraft)
		if err := q.DeleteBroadcastStartJobs(ctx, out.WorkspaceID, out.ID); err != nil {
			return err
		}
		s.queueBroadcastStart(events, out, sendAt)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.ScheduleBroadcast200JSONResponse(s.broadcastBody(out)), nil
}

func (s *Server) freezeDesign(ctx context.Context, q *store.Queries, p principal, b *store.Broadcast) error {
	if b.DesignID == nil {
		return errDesignRequired
	}
	d, err := q.LockEmailDesign(ctx, store.LockEmailDesignParams{WorkspaceID: b.WorkspaceID, ID: *b.DesignID})
	if err != nil {
		return err
	}
	if err := s.snapshotDesign(ctx, q, p, d, "scheduled", true); err != nil {
		return err
	}
	b.Document, b.DesignVersion = d.Document, new(d.Version)
	return nil
}

func (s *Server) freezeLinks(ctx context.Context, q *store.Queries, b store.Broadcast) error {
	if err := q.DeleteBroadcastLinks(ctx, store.DeleteBroadcastLinksParams{WorkspaceID: b.WorkspaceID, BroadcastID: b.ID}); err != nil {
		return err
	}
	if !b.ClickTracking {
		return nil
	}
	doc, issues, err := design.Parse(b.Document)
	if err != nil || len(issues) > 0 {
		return errDesignRequired
	}
	r, err := s.newBroadcastRenderer(ctx, q, b, doc, nil)
	if err != nil {
		return err
	}
	tags := doc.Versions()
	if len(tags) == 0 {
		tags = []string{""}
	}
	for _, tag := range tags {
		links, err := r.trackableLinks(tag)
		if err != nil {
			return err
		}
		for pos, u := range links {
			if err := q.InsertBroadcastLink(ctx, store.InsertBroadcastLinkParams{
				WorkspaceID: b.WorkspaceID, BroadcastID: b.ID, Locale: tag, Position: int32(pos), Url: u,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// changeBroadcast locks a broadcast the caller may send and applies a state change.
func (s *Server) changeBroadcast(ctx context.Context, id uuid.UUID, change func(q *store.Queries, events *eventBatch, b *store.Broadcast, w store.Workspace) error) (store.Broadcast, error) {
	p := principalFrom(ctx)
	var out store.Broadcast
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		b, err := visibleBroadcast(ctx, q, p, id, true)
		if err != nil {
			return err
		}
		if err := requireManagerOrKey(p); err != nil {
			return err
		}
		w, err := q.GetWorkspace(ctx, p.workspaceID)
		if err != nil {
			return err
		}
		prev := b.State
		if err := change(q, events, &b, w); err != nil {
			return err
		}
		if out, err = s.saveBroadcast(ctx, q, b); err != nil {
			return err
		}
		if out.State == bcCancelled {
			if _, err := q.SkipQueuedDeliveries(ctx, store.SkipQueuedDeliveriesParams{WorkspaceID: p.workspaceID, BroadcastID: out.ID}); err != nil {
				return err
			}
			if err := s.recountBroadcast(ctx, q, &out); err != nil {
				return err
			}
		}
		s.emitBroadcast(events, out, prev)
		return nil
	})
	return out, err
}

func (s *Server) UnscheduleBroadcast(ctx context.Context, req oas.UnscheduleBroadcastRequestObject) (oas.UnscheduleBroadcastResponseObject, error) {
	out, err := s.changeBroadcast(ctx, req.BroadcastId, func(q *store.Queries, _ *eventBatch, b *store.Broadcast, _ store.Workspace) error {
		if b.State != bcScheduled {
			if b.State == bcDraft {
				return errBroadcastState
			}
			return errAlreadyStarted
		}
		if err := q.DeleteBroadcastStartJobs(ctx, b.WorkspaceID, b.ID); err != nil {
			return err
		}
		b.State, b.ScheduledAt, b.Document, b.DesignVersion, b.ScheduledBy, b.ScheduledByKey = bcDraft, nil, nil, nil, nil, nil
		b.ServiceConfirmedBy, b.ServiceConfirmedByKey, b.ServiceConfirmedAt = nil, nil, nil
		return q.DeleteBroadcastLinks(ctx, store.DeleteBroadcastLinksParams{WorkspaceID: b.WorkspaceID, BroadcastID: b.ID})
	})
	if err != nil {
		return nil, err
	}
	return oas.UnscheduleBroadcast200JSONResponse(s.broadcastBody(out)), nil
}

func (s *Server) PauseBroadcast(ctx context.Context, req oas.PauseBroadcastRequestObject) (oas.PauseBroadcastResponseObject, error) {
	out, err := s.changeBroadcast(ctx, req.BroadcastId, func(_ *store.Queries, _ *eventBatch, b *store.Broadcast, _ store.Workspace) error {
		if b.State != bcPreparing && b.State != bcSending && b.State != bcActive {
			return errBroadcastState
		}
		b.State, b.PauseReason, b.StateDetail, b.WaitingUntil = bcPaused, new(string(oas.BroadcastPauseReasonMember)), nil, nil
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.PauseBroadcast200JSONResponse(s.broadcastBody(out)), nil
}

func (s *Server) ResumeBroadcast(ctx context.Context, req oas.ResumeBroadcastRequestObject) (oas.ResumeBroadcastResponseObject, error) {
	p := principalFrom(ctx)
	reviewed, err := visibleBroadcast(ctx, s.st.Queries, p, req.BroadcastId, false)
	if err != nil {
		return nil, err
	}
	if reviewed.Kind == bcTriggered && reviewed.State == bcPaused {
		if err := requireManagerOrKey(p); err != nil {
			return nil, err
		}
		draft := reviewed
		draft.Document = nil
		review, err := s.reviewBroadcast(ctx, s.st.Queries, draft)
		if err != nil {
			return nil, err
		}
		var blocking []oas.BroadcastReviewItem
		for _, it := range review.Items {
			if it.Status == oas.ReviewBlocking && it.Key != oas.BroadcastReviewKeyApproval {
				blocking = append(blocking, it)
			}
		}
		if len(blocking) > 0 {
			return nil, reviewFailed(blocking)
		}
	}
	out, err := s.changeBroadcast(ctx, req.BroadcastId, func(q *store.Queries, events *eventBatch, b *store.Broadcast, w store.Workspace) error {
		if b.State != bcPaused {
			return errBroadcastState
		}
		if err := s.sendGate(p, w); err != nil {
			return err
		}
		if b.Kind == bcTriggered {
			if !b.UpdatedAt.Equal(reviewed.UpdatedAt) {
				return problem(http.StatusConflict, "broadcast_changed", "the triggered e-mail changed while it was reviewed; resume it again")
			}
			if err := s.freezeDesign(ctx, q, p, b); err != nil {
				return err
			}
			if err := s.freezeLinks(ctx, q, *b); err != nil {
				return err
			}
		}
		if b.ChannelID != nil {
			ch, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: b.WorkspaceID, ChannelID: *b.ChannelID})
			if err != nil && !store.IsNotFound(err) {
				return err
			}
			if err == nil {
				sc, err := s.sesBroadcastCheck(ctx, q, ch, b.InboxID)
				if err != nil {
					return err
				}
				if sc.status == oas.ReviewBlocking {
					return errSESTenantBlocked
				}
			}
		}
		now := s.now()
		b.PauseReason, b.StateDetail, b.SenderFailingSince, b.WaitingUntil, b.ResumedAt = nil, nil, nil, nil, &now
		if b.Kind == bcTriggered {
			switch {
			case b.StartedAt != nil:
				b.State = bcActive
			case b.ScheduledAt != nil && b.ScheduledAt.After(now):
				b.State = bcScheduled
				s.queueBroadcastStart(events, *b, *b.ScheduledAt)
				return nil
			default:
				if err := s.startTriggered(ctx, q, b, w); err != nil {
					return err
				}
			}
			if !b.SnapshotDone {
				s.queueBroadcastStart(events, *b, time.Time{})
			}
			events.jobOnce(TriggeredDueArgs{WorkspaceID: b.WorkspaceID, BroadcastID: b.ID}, broadcastJobOpts(time.Time{}, broadcastQueue))
			if b.ChannelID != nil {
				s.queueBroadcastSend(events, b.WorkspaceID, *b.ChannelID)
			}
			return nil
		}
		if !b.SnapshotDone {
			b.State = bcPreparing
			s.queueBroadcastStart(events, *b, time.Time{})
			return nil
		}
		b.State = bcSending
		if b.ChannelID != nil {
			s.queueBroadcastSend(events, b.WorkspaceID, *b.ChannelID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.ResumeBroadcast200JSONResponse(s.broadcastBody(out)), nil
}

func (s *Server) CancelBroadcast(ctx context.Context, req oas.CancelBroadcastRequestObject) (oas.CancelBroadcastResponseObject, error) {
	out, err := s.changeBroadcast(ctx, req.BroadcastId, func(q *store.Queries, _ *eventBatch, b *store.Broadcast, _ store.Workspace) error {
		if !activeBroadcast(b.State) {
			return errBroadcastState
		}
		now := s.now()
		b.State, b.FinishedAt, b.PauseReason, b.WaitingUntil = bcCancelled, &now, nil, nil
		if err := q.CancelWaitingEnrollments(ctx, store.CancelWaitingEnrollmentsParams{WorkspaceID: b.WorkspaceID, BroadcastID: b.ID, Now: now}); err != nil {
			return err
		}
		return q.DeleteBroadcastStartJobs(ctx, b.WorkspaceID, b.ID)
	})
	if err != nil {
		return nil, err
	}
	return oas.CancelBroadcast200JSONResponse(s.broadcastBody(out)), nil
}

func broadcastDeliveryBody(d store.ListBroadcastDeliveriesRow) oas.BroadcastDelivery {
	out := oas.BroadcastDelivery{
		Id: d.ID, Contact: oas.SubscriptionContact{Id: d.ContactID, Name: d.ContactName}, Email: oas.Email(d.Email), ListId: d.ListID,
		State: oas.BroadcastDeliveryState(d.State), Error: d.Error, Attempts: d.Attempts, SentAt: d.SentAt, BouncedAt: d.BouncedAt,
		ComplainedAt: d.ComplainedAt, UnsubscribedAt: d.UnsubscribedAt, RepliedAt: d.RepliedAt, AutoRepliedAt: d.AutoRepliedAt,
		ClickedAt: d.ClickedAt, ConversationId: d.ConversationID, CreatedAt: d.CreatedAt,
	}
	if d.Locale != nil {
		out.Locale = new(oas.LanguageTag(*d.Locale))
	}
	if d.SkipReason != nil {
		r := oas.BroadcastSkipReason(*d.SkipReason)
		out.SkipReason = &r
	}
	return out
}

func (s *Server) ListBroadcastDeliveries(ctx context.Context, req oas.ListBroadcastDeliveriesRequestObject) (oas.ListBroadcastDeliveriesResponseObject, error) {
	p := principalFrom(ctx)
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	var after *uuid.UUID
	if c := req.Params.Cursor; c != nil && *c != "" {
		id, err := uuid.Parse(*c)
		if err != nil {
			return nil, errInvalidCursor
		}
		after = &id
	}
	var state, q *string
	if req.Params.State != nil {
		if !req.Params.State.Valid() {
			return nil, errValidation("state must be queued, sent, failed, bounced, complained or skipped")
		}
		state = new(string(*req.Params.State))
	}
	if req.Params.Q != nil {
		if v := strings.TrimSpace(*req.Params.Q); v != "" {
			q = &v
		}
	}
	var locale *string
	if req.Params.Locale != nil {
		tag, err := languageTag(string(*req.Params.Locale), "locale")
		if err != nil {
			return nil, err
		}
		locale = &tag
	}
	b, err := visibleBroadcast(ctx, s.st.Queries, p, req.BroadcastId, false)
	if err != nil {
		return nil, err
	}
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	rows, err := s.st.ListBroadcastDeliveries(ctx, store.ListBroadcastDeliveriesParams{
		WorkspaceID: p.workspaceID, BroadcastID: b.ID, State: state, Q: q, Replied: req.Params.Replied, Locale: locale, After: after, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := rows[lim-1].ID.String()
		next = &c
	}
	out := oas.ListBroadcastDeliveries200JSONResponse{Items: make([]oas.BroadcastDelivery, len(rows)), NextCursor: next}
	for i, r := range rows {
		out.Items[i] = broadcastDeliveryBody(r)
	}
	return out, nil
}

func rate(n, of int64) float64 {
	if of <= 0 {
		return 0
	}
	return float64(n) / float64(of)
}

func (s *Server) GetBroadcastReport(ctx context.Context, req oas.GetBroadcastReportRequestObject) (oas.GetBroadcastReportResponseObject, error) {
	p := principalFrom(ctx)
	var locale *string
	if req.Params.Locale != nil {
		tag, err := languageTag(string(*req.Params.Locale), "locale")
		if err != nil {
			return nil, err
		}
		locale = &tag
	}
	b, err := visibleBroadcast(ctx, s.st.Queries, p, req.BroadcastId, false)
	if err != nil {
		return nil, err
	}
	versions := frozenVersions(b)
	if locale != nil && !slices.Contains(versions, *locale) {
		return nil, errValidation("locale must be a language version of the broadcast's design: " + strings.Join(versions, ", "))
	}
	var byLocale []store.RecountBroadcastByLocaleRow
	if len(versions) > 0 {
		if byLocale, err = s.st.RecountBroadcastByLocale(ctx, store.RecountBroadcastByLocaleParams{WorkspaceID: p.workspaceID, BroadcastID: b.ID, Locale: locale}); err != nil {
			return nil, err
		}
	}
	c := parseCounts(b.Counts)
	if locale != nil {
		c = bcCounts{}
		if len(byLocale) > 0 {
			c = localeCounts(byLocale[0])
		}
	}
	out := oas.GetBroadcastReport200JSONResponse{
		BroadcastId: b.ID, Counts: c.body(b.State, locale == nil && b.Kind == bcTriggered), Rates: c.rates(), Timeline: []oas.BroadcastReportPoint{}, Replies: []oas.BroadcastReportReply{},
	}
	if locale != nil {
		out.Locale = new(oas.LanguageTag(*locale))
	} else if len(versions) > 0 {
		list := make([]oas.BroadcastLocaleReport, len(byLocale))
		for i, r := range byLocale {
			lc := localeCounts(r)
			list[i] = oas.BroadcastLocaleReport{Locale: oas.LanguageTag(r.Locale), Counts: lc.body(b.State, false), Rates: lc.rates()}
		}
		out.Locales = &list
	}
	start := b.CreatedAt
	if b.StartedAt != nil {
		start = *b.StartedAt
	}
	points, err := s.st.BroadcastTimeline(ctx, store.BroadcastTimelineParams{WorkspaceID: p.workspaceID, BroadcastID: b.ID, HourlyUntil: start.Add(72 * time.Hour), Locale: locale})
	if err != nil {
		return nil, err
	}
	for _, pt := range points {
		out.Timeline = append(out.Timeline, oas.BroadcastReportPoint{
			At: pt.At.UTC(), Sent: pt.Sent, Bounced: pt.Bounced, Complained: pt.Complained, Unsubscribed: pt.Unsubscribed,
			Replied: pt.Replied, Clicked: pt.Clicked,
		})
	}
	if b.ClickTracking {
		list := []oas.BroadcastReportLink{}
		if len(versions) > 0 && locale == nil {
			links, err := s.st.ListBroadcastLinksByURL(ctx, store.ListBroadcastLinksByURLParams{WorkspaceID: p.workspaceID, BroadcastID: b.ID})
			if err != nil {
				return nil, err
			}
			for _, l := range links {
				list = append(list, oas.BroadcastReportLink{Url: l.Url, Clicks: l.Clicks})
			}
		} else {
			links, err := s.st.ListBroadcastLinks(ctx, store.ListBroadcastLinksParams{WorkspaceID: p.workspaceID, BroadcastID: b.ID, Locale: locale})
			if err != nil {
				return nil, err
			}
			for _, l := range links {
				list = append(list, oas.BroadcastReportLink{Url: l.Url, Clicks: l.Clicks})
			}
		}
		out.Links = &list
	}
	replies, err := s.st.ListBroadcastReplies(ctx, store.ListBroadcastRepliesParams{WorkspaceID: p.workspaceID, BroadcastID: b.ID, Locale: locale})
	if err != nil {
		return nil, err
	}
	for _, r := range replies {
		out.Replies = append(out.Replies, oas.BroadcastReportReply{
			ConversationId: r.ConversationID, Contact: oas.SubscriptionContact{Id: r.ContactID, Name: r.ContactName}, At: r.RepliedAt,
		})
	}
	if locale != nil {
		return out, nil
	}
	visits, err := s.campaignVisits(ctx, p, b)
	if err != nil {
		return nil, err
	}
	if visits != nil {
		out.CampaignVisits = &visits
	}
	return out, nil
}

func (c bcCounts) rates() oas.BroadcastRates {
	return oas.BroadcastRates{
		Bounced: rate(c.Bounced, c.Sent), Complained: rate(c.Complained, c.Sent), Unsubscribed: rate(c.Unsubscribed, c.Sent),
		Replied: rate(c.Replied, c.Sent), Clicked: rate(c.Clicked, c.Sent),
	}
}

func localeCounts(r store.RecountBroadcastByLocaleRow) bcCounts {
	return bcCounts{
		Recipients: r.Recipients, Queued: r.Queued, Sent: r.Sent, Failed: r.Failed, Bounced: r.Bounced, Complained: r.Complained,
		Unsubscribed: r.Unsubscribed, Replied: r.Replied, AutoReplied: r.AutoReplied, Clicked: r.Clicked,
		SkippedSuppressed: r.SkippedSuppressed, SkippedOptedOut: r.SkippedOptedOut, SkippedUnsubscribed: r.SkippedUnsubscribed,
		SkippedCancelled: r.SkippedCancelled,
	}
}

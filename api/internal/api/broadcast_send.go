package api

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"mime"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/mail/design"
	"github.com/productdevbook/yuva/api/internal/metrics"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	broadcastQueue     = jobs.BroadcastQueue
	broadcastBatch     = 50
	batchTime          = 5 * time.Second
	sessionMessages    = 100
	sessionAge         = 5 * time.Minute
	senderDownPause    = 30 * time.Minute
	rejectsInARow      = 20
	maxDeliveryTries   = 5
	progressEvery      = 5 * time.Second
	startBudget        = 30 * time.Second
	sendBudget         = 50 * time.Second
	clickPathPrefix    = "/c/"
	maxTrackedLinks    = 500
	feedbackSenderID   = "yuvabroadcast"
	sampleOtherName    = "Grace Hopper"
	sampleOtherAddress = "grace@example.org"
)

var (
	retryAfter        = []time.Duration{5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour}
	clickTokenContext = []byte("yuva broadcast click v1")
	hrefAttr          = regexp.MustCompile(`href="([^"]*)"`)
)

type BroadcastStartArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	BroadcastID uuid.UUID `json:"broadcast_id"`
}

func (BroadcastStartArgs) Kind() string { return "broadcast_start" }

type BroadcastSendArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ChannelID   uuid.UUID `json:"channel_id"`
}

func (BroadcastSendArgs) Kind() string { return "broadcast_send" }

type BroadcastTestArgs struct {
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	BroadcastID uuid.UUID  `json:"broadcast_id"`
	MemberID    uuid.UUID  `json:"member_id"`
	ContactID   *uuid.UUID `json:"contact_id,omitempty"`
}

func (BroadcastTestArgs) Kind() string { return "broadcast_test" }

// broadcastJobOpts keeps one waiting job per broadcast or channel; finished jobs do not block the
// next one.
func broadcastJobOpts(at time.Time, queue string) *river.InsertOpts {
	o := &river.InsertOpts{Queue: queue, MaxAttempts: 25, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
		rivertype.JobStateRetryable, rivertype.JobStateScheduled,
	}}}
	if !at.IsZero() {
		o.ScheduledAt = at
	}
	return o
}

type broadcastStartWorker struct {
	river.WorkerDefaults[BroadcastStartArgs]
	s *Server
}

func (w *broadcastStartWorker) Timeout(*river.Job[BroadcastStartArgs]) time.Duration {
	return 2 * time.Minute
}

func (w *broadcastStartWorker) Work(ctx context.Context, job *river.Job[BroadcastStartArgs]) error {
	wait, err := w.s.StartBroadcast(ctx, job.Args.WorkspaceID, job.Args.BroadcastID, startBudget)
	if err != nil {
		return err
	}
	if wait >= 0 {
		return river.JobSnooze(wait)
	}
	return nil
}

type broadcastSendWorker struct {
	river.WorkerDefaults[BroadcastSendArgs]
	s *Server
}

func (w *broadcastSendWorker) Timeout(*river.Job[BroadcastSendArgs]) time.Duration {
	return 3 * time.Minute
}

func (w *broadcastSendWorker) Work(ctx context.Context, job *river.Job[BroadcastSendArgs]) error {
	wait, err := w.s.SendBroadcasts(ctx, job.Args.WorkspaceID, job.Args.ChannelID, sendBudget)
	if err != nil {
		return err
	}
	if wait >= 0 {
		return river.JobSnooze(wait)
	}
	return nil
}

type broadcastTestWorker struct {
	river.WorkerDefaults[BroadcastTestArgs]
	s *Server
}

func (w *broadcastTestWorker) Timeout(*river.Job[BroadcastTestArgs]) time.Duration {
	return 2 * time.Minute
}

func (w *broadcastTestWorker) Work(ctx context.Context, job *river.Job[BroadcastTestArgs]) error {
	return w.s.SendBroadcastTestMail(ctx, job.Args, job.Attempt >= job.MaxAttempts)
}

type broadcastSweepWorker struct {
	river.WorkerDefaults[jobs.BroadcastSweepArgs]
	s *Server
}

func (w *broadcastSweepWorker) Work(ctx context.Context, _ *river.Job[jobs.BroadcastSweepArgs]) error {
	return w.s.SweepBroadcasts(ctx)
}

// SweepBroadcasts makes sure every broadcast that should be moving has its job: a job lost to a
// race with a finishing one, or a crash, is queued again.
func (s *Server) SweepBroadcasts(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	for _, ws := range ids {
		rows, err := s.st.ListRunningBroadcasts(ctx, ws)
		if err != nil {
			return err
		}
		var batch []river.InsertManyParams
		seen := map[uuid.UUID]bool{}
		for _, b := range rows {
			switch {
			case b.State == bcPreparing || (b.State == bcScheduled && b.ScheduledAt != nil && !b.ScheduledAt.After(now)):
				batch = append(batch, river.InsertManyParams{Args: BroadcastStartArgs{WorkspaceID: ws, BroadcastID: b.ID}, InsertOpts: broadcastJobOpts(time.Time{}, "")})
			case b.State == bcSending && b.ChannelID != nil && !seen[*b.ChannelID]:
				seen[*b.ChannelID] = true
				batch = append(batch, river.InsertManyParams{Args: BroadcastSendArgs{WorkspaceID: ws, ChannelID: *b.ChannelID}, InsertOpts: broadcastJobOpts(time.Time{}, broadcastQueue)})
			}
		}
		if len(batch) > 0 {
			if _, err := s.jobs.InsertMany(ctx, batch); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) linkBase() string {
	if s.broadcasts.LinkURL != "" {
		return s.broadcasts.LinkURL
	}
	return s.auth.PublicURL
}

func (s *Server) clickURL(ws, delivery uuid.UUID, position int) string {
	plain := make([]byte, 0, 34)
	plain = append(append(plain, ws[:]...), delivery[:]...)
	plain = binary.BigEndian.AppendUint16(plain, uint16(position))
	return s.linkBase() + clickPathPrefix + base64.RawURLEncoding.EncodeToString(s.secrets.Seal(plain, clickTokenContext))
}

func (s *Server) openClickToken(token string) (ws, delivery uuid.UUID, position int, ok bool) {
	if len(token) > 200 {
		return ws, delivery, 0, false
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return ws, delivery, 0, false
	}
	plain, err := s.secrets.Open(sealed, clickTokenContext)
	if err != nil || len(plain) != 34 {
		return ws, delivery, 0, false
	}
	copy(ws[:], plain[:16])
	copy(delivery[:], plain[16:32])
	return ws, delivery, int(binary.BigEndian.Uint16(plain[32:])), true
}

// bcRenderer renders a broadcast for one recipient after another; it is built once per batch.
type bcRenderer struct {
	s       *Server
	b       store.Broadcast
	doc     design.Document
	inbox   store.Inbox
	lists   map[uuid.UUID]store.MailingList
	image   func(design.Block) (design.Image, bool)
	tracked map[int]bool
	hrefs   int

	fromName, from, replyTo, domain string
	cfg                             email.SMTPConfig
	dkim                            *email.DKIMKey
	ses                             []email.Header
}

func (s *Server) newBroadcastRenderer(ctx context.Context, q *store.Queries, b store.Broadcast, doc design.Document, ch *store.EmailChannel) (*bcRenderer, error) {
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: b.WorkspaceID, ID: b.InboxID})
	if err != nil {
		return nil, err
	}
	r := &bcRenderer{s: s, b: b, doc: doc, inbox: in, lists: map[uuid.UUID]store.MailingList{}}
	a := parseAudience(b.Audience)
	if len(a.ListIDs) > 0 {
		lists, err := q.ListMailingListsByIDs(ctx, store.ListMailingListsByIDsParams{WorkspaceID: b.WorkspaceID, Ids: a.ListIDs})
		if err != nil {
			return nil, err
		}
		for _, l := range lists {
			r.lists[l.ID] = l
		}
	}
	if r.image, err = s.designImages(ctx, q, b.WorkspaceID, doc); err != nil {
		return nil, err
	}
	if b.ClickTracking && b.State != bcDraft {
		links, err := q.ListBroadcastLinks(ctx, store.ListBroadcastLinksParams{WorkspaceID: b.WorkspaceID, BroadcastID: b.ID})
		if err != nil {
			return nil, err
		}
		r.tracked = map[int]bool{}
		for _, l := range links {
			r.tracked[int(l.Position)] = true
		}
		if len(r.tracked) > 0 {
			sample, err := r.render(r.sample(false), r.firstList())
			if err != nil {
				return nil, err
			}
			r.hrefs = len(hrefAttr.FindAllStringIndex(sample.HTML, -1))
		}
	}
	if ch != nil {
		cfg, from, ok, err := s.broadcastAccount(*ch)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errSenderNotReady
		}
		r.cfg, r.from, r.domain = cfg, from, email.Domain(from)
		if r.dkim, err = s.dkimKey(*ch, from, cfg.Host); err != nil {
			return nil, err
		}
		if r.ses, err = s.sesHeaders(ctx, *ch, cfg); err != nil {
			return nil, err
		}
		r.fromName = ch.DisplayName
		if r.fromName == "" {
			if c, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: b.WorkspaceID, ID: ch.ChannelID}); err == nil {
				r.fromName = c.Name
			}
		}
		if r.fromName == "" {
			r.fromName = in.Name
		}
		r.replyTo = ch.Address
		if isCatchAll(r.replyTo) {
			r.replyTo = from
		}
	}
	return r, nil
}

func (r *bcRenderer) firstList() uuid.UUID {
	for _, id := range parseAudience(r.b.Audience).ListIDs {
		if _, ok := r.lists[id]; ok {
			return id
		}
	}
	return uuid.Nil()
}

func (r *bcRenderer) sample(other bool) design.Values {
	v := design.Values{ContactName: sampleContactName, ContactEmail: sampleContactEmail, Attributes: map[string]string{}}
	v.UnsubscribeURL = r.s.linkBase() + managePathPrefix + "sample-a"
	if other {
		v.ContactName, v.ContactEmail = sampleOtherName, sampleOtherAddress
		v.Attributes["sample"] = "b"
		v.UnsubscribeURL = r.s.linkBase() + managePathPrefix + "sample-b"
	}
	v.PreferencesURL = v.UnsubscribeURL
	return v
}

func (r *bcRenderer) render(v design.Values, listID uuid.UUID) (design.Output, error) {
	v.InboxName = r.inbox.Name
	if l, ok := r.lists[listID]; ok {
		v.ListName = l.Name
	}
	in := design.Input{
		Doc: r.doc, Subject: v.Text(r.b.Subject), Preheader: r.doc.Preheader, Lang: mailLang(r.inbox.DefaultLocale), Values: v,
		Image: r.image, Icon: r.s.iconURL,
		Sender: design.Sender{LegalName: deref(r.inbox.SenderLegalName), PostalAddress: deref(r.inbox.SenderPostalAddress),
			Registration: deref(r.inbox.SenderRegistration), Contact: deref(r.inbox.SenderContact)},
	}
	if r.b.Preheader != "" {
		in.Preheader = r.b.Preheader
	}
	out, err := design.Render(in)
	if err != nil {
		return out, err
	}
	return r.addUTM(out), nil
}

// trackableLinks lists the links that are the same for every recipient (no merge field in them)
// and lead to the web, by their position among the HTML's links.
func (r *bcRenderer) trackableLinks() (map[int]string, error) {
	a, err := r.render(r.sample(false), r.firstList())
	if err != nil {
		return nil, err
	}
	b, err := r.render(r.sample(true), r.firstList())
	if err != nil {
		return nil, err
	}
	ha, hb := hrefAttr.FindAllStringSubmatch(a.HTML, -1), hrefAttr.FindAllStringSubmatch(b.HTML, -1)
	out := map[int]string{}
	if len(ha) != len(hb) {
		return out, nil
	}
	own := r.s.linkBase() + managePathPrefix
	for i := range ha {
		u := html.UnescapeString(ha[i][1])
		if i >= maxTrackedLinks || ha[i][1] != hb[i][1] || strings.HasPrefix(u, own) || len(u) > 2048 {
			continue
		}
		if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
			out[i] = u
		}
	}
	return out, nil
}

func (r *bcRenderer) track(htmlBody string, ws, delivery uuid.UUID) string {
	if len(r.tracked) == 0 {
		return htmlBody
	}
	matches := hrefAttr.FindAllStringSubmatchIndex(htmlBody, -1)
	if len(matches) != r.hrefs {
		return htmlBody
	}
	var b strings.Builder
	last := 0
	for i, m := range matches {
		if !r.tracked[i] {
			continue
		}
		b.WriteString(htmlBody[last:m[2]])
		b.WriteString(html.EscapeString(r.s.clickURL(ws, delivery, i)))
		last = m[3]
	}
	b.WriteString(htmlBody[last:])
	return b.String()
}

func headerPhrase(name string) string {
	name = strings.NewReplacer(`"`, "", `\`, "", "\r", " ", "\n", " ").Replace(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	for _, c := range name {
		if c >= utf8.RuneSelf {
			return mime.QEncoding.Encode("utf-8", name)
		}
	}
	return `"` + name + `"`
}

func hex8(id uuid.UUID) string { return strings.ReplaceAll(id.String(), "-", "")[24:] }

// listHeaders are the headers every broadcast mail carries: one-click unsubscribe (RFC 8058),
// the list (RFC 2919), Gmail's feedback loop id and fewer out-of-office answers from Exchange.
func (r *bcRenderer) listHeaders(manage string, list store.MailingList, hasList bool) []email.Header {
	h := []email.Header{
		{Name: "List-Unsubscribe", Value: "<" + manage + ">"},
		{Name: "List-Unsubscribe-Post", Value: "List-Unsubscribe=One-Click"},
		{Name: "X-Auto-Response-Suppress", Value: "OOF, AutoReply"},
	}
	listPart := "00000000"
	if hasList {
		id := strings.ReplaceAll(list.ID.String(), "-", "")[20:]
		value := "<l" + id + "." + r.domain + ">"
		if p := headerPhrase(list.Name); p != "" {
			value = p + " " + value
		}
		h = append(h, email.Header{Name: "List-Id", Value: value})
		listPart = hex8(list.ID)
	}
	h = append(h, email.Header{Name: "Feedback-ID", Value: hex8(r.b.ID) + ":" + listPart + ":" + hex8(r.b.WorkspaceID) + ":" + feedbackSenderID})
	return append(h, r.ses...)
}

// message builds one recipient's mail.
func (r *bcRenderer) message(d store.BroadcastDelivery, c contactRow) ([]byte, error) {
	ws := r.b.WorkspaceID
	listID := uuid.Nil()
	if d.ListID != nil {
		listID = *d.ListID
	}
	manage := r.s.manageURL(ws, listID, d.ContactID, r.b.ID)
	v := contactValues(c.Name, d.Email, c.Attributes)
	v.UnsubscribeURL, v.PreferencesURL = manage, manage
	out, err := r.render(v, listID)
	if err != nil {
		return nil, err
	}
	l, hasList := r.lists[listID]
	raw, err := email.Build(email.Outgoing{
		From: email.Address{Name: r.fromName, Email: r.from}, ReplyTo: email.Address{Name: r.fromName, Email: r.replyTo},
		To: email.Address{Name: c.Name, Email: d.Email}, Subject: v.Text(r.b.Subject), MessageID: d.HeaderMessageID,
		Text: out.Text, HTML: r.track(out.HTML, ws, d.ID), Date: r.s.now(), Headers: r.listHeaders(manage, l, hasList),
	})
	if err != nil {
		return nil, err
	}
	return signWith(r.dkim, raw)
}

// StartBroadcast takes the audience of a broadcast whose time has come, page by page, and starts
// sending. It answers how long to wait before calling it again: 0 when the budget ran out, until
// the scheduled time but at most 5 minutes when that has not come, -1 when there is nothing left to do.
func (s *Server) StartBroadcast(ctx context.Context, ws, id uuid.UUID, budget time.Duration) (time.Duration, error) {
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return -1, err
	}
	deadline := time.Now().Add(budget)
	var b store.Broadcast
	goOn := false
	notYet := time.Duration(-1)
	err := s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
		var err error
		b, err = q.LockBroadcast(ctx, store.LockBroadcastParams{WorkspaceID: ws, ID: id})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		switch b.State {
		case bcPreparing:
			goOn = true
			return nil
		case bcScheduled:
		default:
			return nil
		}
		if b.ScheduledAt != nil && b.ScheduledAt.After(s.now()) {
			notYet = min(b.ScheduledAt.Sub(s.now()), paceSnoozeAtMost)
			return nil
		}
		w, err := q.GetWorkspace(ctx, ws)
		if err != nil {
			return err
		}
		prev := b.State
		if s.broadcastMode() == broadcastsOff || w.BroadcastsStatus != wsAllowed {
			b.State, b.PauseReason = bcPaused, new(string(oas.BroadcastPauseReasonWorkspaceSuspended))
			b.StateDetail = new("the workspace may not send broadcasts now")
		} else {
			b.State = bcPreparing
			goOn = true
		}
		if b, err = s.saveBroadcast(ctx, q, b); err != nil {
			return err
		}
		s.emitBroadcast(events, b, prev)
		return nil
	})
	if err != nil || !goOn {
		return notYet, err
	}
	a := parseAudience(b.Audience)
	from := ""
	if b.ChannelID != nil {
		if ch, err := s.st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws, ChannelID: *b.ChannelID}); err == nil {
			_, from, _, _ = s.broadcastAccount(ch)
		}
	}
	if from == "" {
		return -1, s.failBroadcast(ctx, ws, id, "the broadcast's e-mail channel cannot send")
	}
	domain := email.Domain(from)
	for {
		done := false
		err := s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
			cur, err := q.LockBroadcast(ctx, store.LockBroadcastParams{WorkspaceID: ws, ID: id})
			if err != nil {
				return err
			}
			if cur.State != bcPreparing {
				done = true
				return nil
			}
			after := uuid.Nil()
			if cur.SnapshotAfter != nil {
				after = *cur.SnapshotAfter
			}
			rows, err := s.audiencePageAfter(ctx, q, ws, a, after)
			if err != nil {
				return err
			}
			var ins store.InsertBroadcastDeliveriesParams
			delta := map[string]int64{}
			for _, r := range rows {
				switch {
				case !r.Matches || r.Blocked:
				case r.Suppressed:
					delta["excluded_suppressed"]++
				case r.optedOut:
					delta["excluded_opted_out"]++
				default:
					token := email.NewConversationToken()
					ins.Ids = append(ins.Ids, newID())
					ins.ContactIds = append(ins.ContactIds, r.ContactID)
					ins.ListIds = append(ins.ListIds, r.ListID)
					ins.Emails = append(ins.Emails, r.Email)
					ins.Tokens = append(ins.Tokens, token)
					ins.Headers = append(ins.Headers, email.NewMessageID(token, domain))
				}
			}
			if len(ins.Ids) > 0 {
				ins.WorkspaceID, ins.BroadcastID, ins.Now = ws, id, s.now()
				n, err := q.InsertBroadcastDeliveries(ctx, ins)
				if err != nil {
					return err
				}
				delta["prepared"] = n
			}
			if len(delta) > 0 {
				if err := q.AddBroadcastCounts(ctx, store.AddBroadcastCountsParams{WorkspaceID: ws, ID: id, Delta: mustJSON(delta)}); err != nil {
					return err
				}
			}
			if len(rows) == audiencePage {
				cur.SnapshotAfter = &rows[len(rows)-1].ContactID
				_, err = s.saveBroadcast(ctx, q, cur)
				return err
			}
			done = true
			return s.finishSnapshot(ctx, q, events, cur)
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

// finishSnapshot moves a prepared broadcast to sending, or straight to sent when nobody is left.
func (s *Server) finishSnapshot(ctx context.Context, q *store.Queries, events *eventBatch, b store.Broadcast) error {
	n, err := q.QueuedBroadcastDeliveries(ctx, store.QueuedBroadcastDeliveriesParams{WorkspaceID: b.WorkspaceID, BroadcastID: b.ID})
	if err != nil {
		return err
	}
	prev, now := b.State, s.now()
	b.SnapshotDone, b.SnapshotAfter = true, nil
	if b.StartedAt == nil {
		b.StartedAt = &now
	}
	b.ResumedAt = &now
	b.State = bcSending
	if n.Queued == 0 {
		b.State, b.FinishedAt = bcSent, &now
	}
	if b, err = s.saveBroadcast(ctx, q, b); err != nil {
		return err
	}
	if err := s.recountBroadcast(ctx, q, &b); err != nil {
		return err
	}
	s.emitBroadcast(events, b, prev)
	if b.State == bcSending && b.ChannelID != nil {
		s.queueBroadcastSend(events, b.WorkspaceID, *b.ChannelID)
	}
	return nil
}

// recountBroadcast counts a broadcast's deliveries again, keeping the counters deliveries cannot
// tell (nothing is lost when old deliveries are cleaned up later).
func (s *Server) recountBroadcast(ctx context.Context, q *store.Queries, b *store.Broadcast) error {
	r, err := q.RecountBroadcast(ctx, store.RecountBroadcastParams{WorkspaceID: b.WorkspaceID, BroadcastID: b.ID})
	if err != nil {
		return err
	}
	c := parseCounts(b.Counts)
	set := map[string]int64{
		"recipients": r.Recipients, "queued": r.Queued, "sent": r.Sent, "failed": r.Failed, "bounced": r.Bounced,
		"complained": r.Complained, "unsubscribed": r.Unsubscribed, "replied": r.Replied, "auto_replied": max(r.AutoReplied, c.AutoReplied),
		"clicked": r.Clicked, "skipped_suppressed": r.SkippedSuppressed, "skipped_opted_out": r.SkippedOptedOut,
		"skipped_unsubscribed": r.SkippedUnsubscribed, "skipped_cancelled": r.SkippedCancelled,
	}
	if err := q.SetBroadcastCounts(ctx, store.SetBroadcastCountsParams{WorkspaceID: b.WorkspaceID, ID: b.ID, Counts: mustJSON(set)}); err != nil {
		return err
	}
	fresh, err := q.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: b.WorkspaceID, ID: b.ID})
	if err != nil {
		return err
	}
	*b = fresh
	return nil
}

func (s *Server) failBroadcast(ctx context.Context, ws, id uuid.UUID, why string) error {
	return s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
		b, err := q.LockBroadcast(ctx, store.LockBroadcastParams{WorkspaceID: ws, ID: id})
		if err != nil {
			return err
		}
		if b.State != bcPreparing && b.State != bcSending {
			return nil
		}
		prev, now := b.State, s.now()
		b.State, b.StateDetail, b.FinishedAt = bcFailed, &why, &now
		if b, err = s.saveBroadcast(ctx, q, b); err != nil {
			return err
		}
		if _, err := q.SkipQueuedDeliveries(ctx, store.SkipQueuedDeliveriesParams{WorkspaceID: ws, BroadcastID: id}); err != nil {
			return err
		}
		if err := s.recountBroadcast(ctx, q, &b); err != nil {
			return err
		}
		s.emitBroadcast(events, b, prev)
		return nil
	})
}

// pauseBroadcast pauses a preparing or sending broadcast for a reason other than a member.
func (s *Server) pauseBroadcast(ctx context.Context, q *store.Queries, events *eventBatch, id uuid.UUID, ws uuid.UUID, reason oas.BroadcastPauseReason, detail string) (bool, error) {
	b, err := q.LockBroadcast(ctx, store.LockBroadcastParams{WorkspaceID: ws, ID: id})
	if err != nil {
		return false, err
	}
	if b.State != bcPreparing && b.State != bcSending && b.State != bcScheduled {
		return false, nil
	}
	prev := b.State
	b.State, b.PauseReason, b.WaitingUntil = bcPaused, new(string(reason)), nil
	b.StateDetail = optionalText(detail, 1000)
	if b, err = s.saveBroadcast(ctx, q, b); err != nil {
		return false, err
	}
	s.emitBroadcast(events, b, prev)
	return true, nil
}

type deliveryResult struct {
	d        store.BroadcastDelivery
	state    string
	skip     *string
	errText  *string
	sentAt   *time.Time
	bounced  *time.Time
	next     *time.Time
	relayID  *string
	attempts int32
}

var (
	progressMu   sync.Mutex
	progressSent = map[uuid.UUID]time.Time{}
)

func (s *Server) progressDue(id uuid.UUID, every time.Duration) bool {
	progressMu.Lock()
	defer progressMu.Unlock()
	now := time.Now()
	if now.Sub(progressSent[id]) < every {
		return false
	}
	if len(progressSent) > 10000 {
		clear(progressSent)
	}
	progressSent[id] = now
	return true
}

func nextUTCMidnight(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, time.UTC)
}

// SendBroadcasts sends the queued mail of the channel's sending broadcasts, oldest broadcast first,
// in batches over one SMTP connection at the channel's pace, for about budget. It answers how long
// to wait before calling again, or a negative duration when nothing is left to send.
func (s *Server) SendBroadcasts(ctx context.Context, ws, channelID uuid.UUID, budget time.Duration) (time.Duration, error) {
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return -1, err
	}
	deadline := time.Now().Add(budget)
	var (
		sess      email.Session
		sessCfg   email.SMTPConfig
		sessSent  int
		sessStart time.Time
		deferred  = map[uuid.UUID]time.Time{}
	)
	closeSession := func() {
		if sess != nil {
			_ = sess.Close()
			sess = nil
		}
	}
	defer closeSession()
	paceFactor := 1.0
	for {
		if time.Now().After(deadline) {
			return 0, nil
		}
		now := s.now()
		list, err := s.st.ListSendingBroadcastsOfChannel(ctx, store.ListSendingBroadcastsOfChannelParams{WorkspaceID: ws, ChannelID: &channelID})
		if err != nil {
			return 0, err
		}
		var b *store.Broadcast
		wait := time.Duration(-1)
		for i := range list {
			at, ok := deferred[list[i].ID]
			if list[i].WaitingUntil != nil && list[i].WaitingUntil.After(now) {
				at, ok = *list[i].WaitingUntil, true
			}
			if ok {
				if d := at.Sub(now); wait < 0 || d < wait {
					wait = max(d, time.Second)
				}
				continue
			}
			b = &list[i]
			break
		}
		if b == nil {
			return wait, nil
		}
		if sw, ok := windowOf(*b); ok {
			if open, opens, _ := sw.at(now); !open {
				s.broadcastProgress(ctx, *b)
				deferred[b.ID] = paceWake(opens, now)
				continue
			}
		}
		w, err := s.st.GetWorkspace(ctx, ws)
		if err != nil {
			return 0, err
		}
		limit, sentToday, err := s.dailyAllowance(ctx, s.st.Queries, w)
		if err != nil {
			return 0, err
		}
		size := int32(broadcastBatch)
		if limit >= 0 {
			left := limit - sentToday
			if left <= 0 {
				until := nextUTCMidnight(now)
				if err := s.st.SetBroadcastsWaiting(ctx, store.SetBroadcastsWaitingParams{WorkspaceID: ws, Until: &until}); err != nil {
					return 0, err
				}
				if cur, err := s.st.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: b.ID}); err == nil {
					s.broadcastProgress(ctx, cur)
				}
				deferred[b.ID] = until
				continue
			}
			size = int32(min(left, int64(broadcastBatch)))
		}
		if b.WaitingUntil != nil {
			if err := s.st.SetBroadcastsWaiting(ctx, store.SetBroadcastsWaitingParams{WorkspaceID: ws}); err != nil {
				return 0, err
			}
		}
		gap := paceGap(*b, now)
		switch {
		case gap > batchTime:
			size = 1
		case gap > 0:
			size = min(size, int32(batchTime/gap)+1)
		}
		ch, err := s.st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws, ChannelID: channelID})
		if store.IsNotFound(err) {
			if err := s.failBroadcast(ctx, ws, b.ID, "the broadcast's e-mail channel was removed"); err != nil {
				return 0, err
			}
			continue
		}
		if err != nil {
			return 0, err
		}
		due, err := s.st.DueBroadcastDeliveries(ctx, store.DueBroadcastDeliveriesParams{WorkspaceID: ws, BroadcastID: b.ID, Now: now, Lim: size})
		if err != nil {
			return 0, err
		}
		if len(due) == 0 {
			qd, err := s.st.QueuedBroadcastDeliveries(ctx, store.QueuedBroadcastDeliveriesParams{WorkspaceID: ws, BroadcastID: b.ID})
			if err != nil {
				return 0, err
			}
			if qd.Queued == 0 {
				if err := s.FinishBroadcast(ctx, ws, b.ID); err != nil {
					return 0, err
				}
				continue
			}
			deferred[b.ID] = qd.NextAttempt
			continue
		}
		if gap > 0 && b.PaceNextAt != nil && b.PaceNextAt.After(now) {
			deferred[b.ID] = paceWake(*b.PaceNextAt, now)
			continue
		}
		doc, _, err := s.broadcastDocument(ctx, s.st.Queries, *b)
		if err != nil {
			return 0, err
		}
		if doc == nil {
			if err := s.failBroadcast(ctx, ws, b.ID, "the broadcast's design cannot be read"); err != nil {
				return 0, err
			}
			continue
		}
		r, err := s.newBroadcastRenderer(ctx, s.st.Queries, *b, *doc, &ch)
		if errors.Is(err, errSenderNotReady) {
			if err := s.pauseForSender(ctx, ws, b.ID, "the channel has no SMTP account or address to send from"); err != nil {
				return 0, err
			}
			continue
		}
		if err != nil {
			return 0, err
		}
		if sc, err := s.sesBroadcastCheck(ctx, s.st.Queries, ch, b.InboxID); err != nil {
			return 0, err
		} else if sc.wait {
			deferred[b.ID] = now.Add(sesSetupWait)
			continue
		} else if sc.status == oas.ReviewBlocking {
			if err := s.pauseFor(ctx, ws, b.ID, oas.BroadcastPauseReasonSesTenant, sc.detail); err != nil {
				return 0, err
			}
			continue
		}
		if sess != nil && (sessCfg != r.cfg || sessSent >= sessionMessages || time.Since(sessStart) > sessionAge) {
			closeSession()
		}
		if sess == nil {
			openCtx, cancel := context.WithTimeout(ctx, time.Minute)
			sess, err = email.OpenSession(openCtx, s.sender, r.cfg)
			cancel()
			if err != nil {
				sess = nil
				var setup *email.SetupError
				if errors.As(err, &setup) && setup.Permanent {
					if err := s.pauseForSender(ctx, ws, b.ID, "the SMTP server refused the account: "+err.Error()); err != nil {
						return 0, err
					}
					continue
				}
				if err := s.senderDown(ctx, ws, b.ID, err); err != nil {
					return 0, err
				}
				return time.Minute, nil
			}
			sessCfg, sessSent, sessStart = r.cfg, 0, time.Now()
		}
		pace := time.Duration(float64(time.Second) / (float64(ch.BroadcastRate) * paceFactor))
		results, stop, rejects := s.sendBatch(ctx, r, sess, due, pace, gap, &paceFactor, &sessSent)
		if stop == stopBroken {
			closeSession()
		}
		if err := s.commitBatch(ctx, *b, results, rejects); err != nil {
			return 0, err
		}
		if stop == stopPace || stop == stopWindow {
			cur, err := s.st.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: b.ID})
			if err != nil {
				return 0, err
			}
			now := s.now()
			deferred[b.ID] = now.Add(time.Second)
			if cur.PaceNextAt != nil && cur.PaceNextAt.After(now) {
				deferred[b.ID] = paceWake(*cur.PaceNextAt, now)
			}
			if sw, ok := windowOf(cur); ok {
				if open, opens, _ := sw.at(now); !open {
					s.broadcastProgress(ctx, cur)
					deferred[b.ID] = paceWake(opens, now)
				}
			}
		}
	}
}

// paceWake is when to look at a broadcast waiting for its next slot again: at the slot, but within
// 5 minutes, so a changed pace applies soon and the channel's job never sleeps through it.
func paceWake(next, now time.Time) time.Time {
	if latest := now.Add(paceSnoozeAtMost); next.After(latest) {
		return latest
	}
	return next
}

const (
	stopNone = iota
	stopState
	stopBroken
	stopPace
	stopWindow
)

func windowClosed(b store.Broadcast, now time.Time) bool {
	w, ok := windowOf(b)
	if !ok {
		return false
	}
	open, _, _ := w.at(now)
	return !open
}

// sendBatch sends due mail one by one, re-reading the broadcast's state and sending window before
// each, so pausing, cancelling or the window's close takes effect within the batch. A batch ends
// after 50 mails or 5 seconds, so counters move even at a slow pace.
func (s *Server) sendBatch(ctx context.Context, r *bcRenderer, sess email.Session, due []store.BroadcastDelivery, pace, gap time.Duration, paceFactor *float64, sessSent *int) ([]deliveryResult, int, int) {
	ws := r.b.WorkspaceID
	lists := parseAudience(r.b.Audience).ListIDs
	var out []deliveryResult
	rejects := 0
	until := time.Now().Add(batchTime)
	attempted := false
	for i, d := range due {
		if i > 0 && time.Now().After(until) {
			return out, stopNone, rejects
		}
		cur, err := s.st.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: r.b.ID})
		if err != nil || cur.State != bcSending {
			return out, stopState, rejects
		}
		if windowClosed(cur, s.now()) {
			return out, stopWindow, rejects
		}
		res := deliveryResult{d: d, state: deliveryQueued, attempts: d.Attempts}
		skip := func(reason string) {
			res.state, res.skip = string(oas.BroadcastDeliveryStateSkipped), new(reason)
		}
		subscribed, err := s.st.IsContactSubscribed(ctx, store.IsContactSubscribedParams{WorkspaceID: ws, ContactID: d.ContactID, Email: d.Email, ListIds: lists})
		if err != nil {
			return out, stopState, rejects
		}
		suppressed, err := s.st.IsEmailSuppressedAnyScope(ctx, store.IsEmailSuppressedAnyScopeParams{WorkspaceID: ws, Email: d.Email})
		if err != nil {
			return out, stopState, rejects
		}
		opted, err := s.optedOut(ctx, s.st.Queries, ws, d.Email)
		if err != nil {
			return out, stopState, rejects
		}
		switch {
		case !subscribed:
			skip(string(oas.BroadcastSkipReasonUnsubscribed))
		case suppressed:
			skip(string(oas.BroadcastSkipReasonSuppressed))
		case opted:
			skip(string(oas.BroadcastSkipReasonOptedOut))
		}
		if res.skip != nil {
			out = append(out, res)
			continue
		}
		c, err := s.st.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: d.ContactID})
		if store.IsNotFound(err) {
			skip(string(oas.BroadcastSkipReasonUnsubscribed))
			out = append(out, res)
			continue
		}
		if err != nil {
			return out, stopState, rejects
		}
		raw, err := r.message(d, c)
		if err != nil {
			res.state, res.errText = deliveryFailed, new(truncateRunes("the mail could not be built: "+err.Error(), maxDeliveryError))
			out = append(out, res)
			continue
		}
		if wait := max(pace, gap); attempted && wait > 0 {
			if s.sleep(ctx, wait) != nil {
				return out, stopState, rejects
			}
			cur, err = s.st.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: r.b.ID})
			if err != nil || cur.State != bcSending {
				return out, stopState, rejects
			}
			if windowClosed(cur, s.now()) {
				return out, stopWindow, rejects
			}
		}
		if gap > 0 {
			now := s.now()
			n, err := s.st.ClaimBroadcastPaceSlot(ctx, store.ClaimBroadcastPaceSlotParams{WorkspaceID: ws, ID: r.b.ID, Now: now, DaySecs: sendingDay(cur, now).Seconds()})
			if err != nil {
				return out, stopState, rejects
			}
			if n == 0 {
				return out, stopPace, rejects
			}
		}
		attempted = true
		sendCtx, cancel := context.WithTimeout(ctx, time.Minute)
		relayID, err := sess.Send(sendCtx, r.from, []string{d.Email}, raw)
		cancel()
		*sessSent++
		now := s.now()
		res.attempts = d.Attempts + 1
		switch {
		case err == nil:
			res.state, res.sentAt = deliverySent, &now
			if relayID != "" {
				res.relayID = &relayID
			}
			rejects = 0
			metrics.BroadcastMessages.WithLabelValues("sent").Inc()
		case email.BadMailbox(err):
			res.state, res.bounced, res.errText = string(oas.BroadcastDeliveryStateBounced), &now, new(truncateRunes(err.Error(), maxDeliveryError))
			metrics.BroadcastMessages.WithLabelValues("bounced").Inc()
		case email.IsPermanent(err):
			res.state, res.errText = deliveryFailed, new(truncateRunes(err.Error(), maxDeliveryError))
			rejects++
			metrics.BroadcastMessages.WithLabelValues("rejected").Inc()
		default:
			if code, _, ok := email.Reply(err); ok && code == 454 && strings.Contains(strings.ToLower(err.Error()), "throttl") {
				*paceFactor = max(*paceFactor/2, 0.05)
			}
			res.errText = new(truncateRunes(err.Error(), maxDeliveryError))
			if int(res.attempts) >= maxDeliveryTries {
				res.state = deliveryFailed
			} else {
				res.next = new(now.Add(retryAfter[res.attempts-1]))
			}
			metrics.BroadcastMessages.WithLabelValues("deferred").Inc()
		}
		out = append(out, res)
		var broken *email.BrokenError
		if errors.As(err, &broken) {
			return out, stopBroken, rejects
		}
		if rejects >= rejectsInARow {
			return out, stopState, rejects
		}
	}
	return out, stopNone, rejects
}

// commitBatch stores a batch's results with its counters, daily and monthly totals in one
// transaction, then applies the automatic pauses.
func (s *Server) commitBatch(ctx context.Context, b store.Broadcast, results []deliveryResult, rejects int) error {
	if len(results) == 0 && rejects < rejectsInARow {
		return nil
	}
	ws := b.WorkspaceID
	var pausedForRejects bool
	err := s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
		delta := map[string]int64{}
		var sent, bounced int32
		for _, r := range results {
			if err := q.SaveBroadcastDelivery(ctx, store.SaveBroadcastDeliveryParams{
				WorkspaceID: ws, ID: r.d.ID, State: r.state, SkipReason: r.skip, Attempts: r.attempts, NextAttemptAt: r.next,
				Error: r.errText, SentAt: r.sentAt, BouncedAt: r.bounced, RelayMessageID: r.relayID,
			}); err != nil {
				return err
			}
			if r.state != deliveryQueued {
				delta["queued"]--
			}
			switch r.state {
			case deliverySent:
				delta["sent"]++
				sent++
			case deliveryFailed:
				delta["failed"]++
			case string(oas.BroadcastDeliveryStateBounced):
				delta["bounced"]++
				bounced++
				if err := s.suppressBounced(ctx, q, events, ws, r.d.Email, deref(r.errText)); err != nil {
					return err
				}
			case string(oas.BroadcastDeliveryStateSkipped):
				delta["skipped_"+*r.skip]++
			}
		}
		if len(delta) > 0 {
			if err := q.AddBroadcastCounts(ctx, store.AddBroadcastCountsParams{WorkspaceID: ws, ID: b.ID, Delta: mustJSON(delta)}); err != nil {
				return err
			}
		}
		if sent > 0 || bounced > 0 {
			if err := q.AddSendingDay(ctx, store.AddSendingDayParams{WorkspaceID: ws, Day: utcDay(s.now()), Sent: sent, Bounced: bounced}); err != nil {
				return err
			}
		}
		if sent > 0 {
			if err := q.AddBroadcastUsage(ctx, store.AddBroadcastUsageParams{WorkspaceID: ws, Month: monthOf(s.now()), Recipients: int64(sent)}); err != nil {
				return err
			}
		}
		if rejects >= rejectsInARow {
			ok, err := s.pauseBroadcast(ctx, q, events, b.ID, ws, oas.BroadcastPauseReasonSenderError, fmt.Sprintf("the SMTP server refused %d mails in a row; the last answer: %s", rejects, lastError(results)))
			if err != nil {
				return err
			}
			pausedForRejects = ok
		}
		cur, err := q.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: b.ID})
		if err != nil {
			return err
		}
		if cur.State != bcSending && cur.State != bcPaused {
			return s.recountBroadcast(ctx, q, &cur)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if pausedForRejects {
		s.noteBroadcastPaused(ctx, b, string(oas.BroadcastPauseReasonSenderError))
	}
	if err := s.checkBroadcastRates(ctx, ws, b.ID); err != nil {
		return err
	}
	if cur, err := s.st.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: b.ID}); err == nil && s.progressDue(b.ID, progressEvery) {
		s.broadcastProgress(ctx, cur)
	}
	return nil
}

func lastError(results []deliveryResult) string {
	for i := len(results) - 1; i >= 0; i-- {
		if results[i].errText != nil {
			return *results[i].errText
		}
	}
	return ""
}

func (s *Server) pauseForSender(ctx context.Context, ws, id uuid.UUID, detail string) error {
	return s.pauseFor(ctx, ws, id, oas.BroadcastPauseReasonSenderError, detail)
}

func (s *Server) pauseFor(ctx context.Context, ws, id uuid.UUID, reason oas.BroadcastPauseReason, detail string) error {
	var b store.Broadcast
	paused := false
	err := s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
		var err error
		paused, err = s.pauseBroadcast(ctx, q, events, id, ws, reason, detail)
		if err != nil {
			return err
		}
		b, err = q.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: id})
		return err
	})
	if err == nil && paused {
		s.noteBroadcastPaused(ctx, b, string(reason))
	}
	return err
}

// senderDown records that the SMTP server cannot be reached; after 30 minutes the broadcast pauses.
func (s *Server) senderDown(ctx context.Context, ws, id uuid.UUID, cause error) error {
	since := time.Time{}
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		b, err := q.LockBroadcast(ctx, store.LockBroadcastParams{WorkspaceID: ws, ID: id})
		if err != nil {
			return err
		}
		if b.SenderFailingSince == nil {
			b.SenderFailingSince = new(s.now())
			_, err = s.saveBroadcast(ctx, q, b)
		}
		since = *b.SenderFailingSince
		return err
	})
	if err != nil {
		return err
	}
	s.log.WarnContext(ctx, "broadcast SMTP server unreachable", slog.String("broadcast_id", id.String()), slog.String("error", mail.RedactAddresses(cause.Error())))
	if s.now().Sub(since) >= senderDownPause {
		return s.pauseForSender(ctx, ws, id, "the SMTP server could not be reached for 30 minutes: "+cause.Error())
	}
	return nil
}

// FinishBroadcast marks a sending broadcast sent once no mail is queued.
func (s *Server) FinishBroadcast(ctx context.Context, ws, id uuid.UUID) error {
	return s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
		b, err := q.LockBroadcast(ctx, store.LockBroadcastParams{WorkspaceID: ws, ID: id})
		if err != nil {
			return err
		}
		if b.State != bcSending {
			return nil
		}
		n, err := q.QueuedBroadcastDeliveries(ctx, store.QueuedBroadcastDeliveriesParams{WorkspaceID: ws, BroadcastID: id})
		if err != nil || n.Queued > 0 {
			return err
		}
		now := s.now()
		b.State, b.FinishedAt, b.WaitingUntil = bcSent, &now, nil
		if b, err = s.saveBroadcast(ctx, q, b); err != nil {
			return err
		}
		if err := s.recountBroadcast(ctx, q, &b); err != nil {
			return err
		}
		s.emitBroadcast(events, b, bcSending)
		return nil
	})
}

// SendBroadcastTestMail sends one test of a broadcast to a member, through the broadcast's channel.
func (s *Server) SendBroadcastTestMail(ctx context.Context, a BroadcastTestArgs, lastAttempt bool) error {
	ws := a.WorkspaceID
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return err
	}
	skip := func(reason string) error {
		s.log.InfoContext(ctx, "broadcast test not sent", slog.String("broadcast_id", a.BroadcastID.String()), slog.String("reason", mail.RedactAddresses(reason)))
		return nil
	}
	b, err := s.st.GetBroadcast(ctx, store.GetBroadcastParams{WorkspaceID: ws, ID: a.BroadcastID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	members, err := s.st.CountWorkspaceMembersByIDs(ctx, store.CountWorkspaceMembersByIDsParams{WorkspaceID: ws, Ids: []uuid.UUID{a.MemberID}})
	if err != nil {
		return err
	}
	if len(members) == 0 {
		return skip("the member left")
	}
	if b.ChannelID == nil {
		return skip("no channel")
	}
	ch, err := s.st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: ws, ChannelID: *b.ChannelID})
	if store.IsNotFound(err) {
		return skip("the channel is gone")
	}
	if err != nil {
		return err
	}
	doc, _, err := s.broadcastDocument(ctx, s.st.Queries, b)
	if err != nil {
		return err
	}
	if doc == nil {
		return skip("no design")
	}
	r, err := s.newBroadcastRenderer(ctx, s.st.Queries, b, *doc, &ch)
	if errors.Is(err, errSenderNotReady) {
		return skip("the channel cannot send")
	}
	if err != nil {
		return err
	}
	v := r.sample(false)
	if a.ContactID != nil {
		if c, err := s.st.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: *a.ContactID}); err == nil {
			addr := ""
			if emails, err := s.st.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: ws, ContactIds: []uuid.UUID{c.ID}}); err == nil && len(emails) > 0 {
				addr = emails[0].Email
			}
			v = contactValues(c.Name, addr, c.Attributes)
		}
	}
	manage := s.manageURL(ws, uuid.Nil(), uuid.Nil(), b.ID)
	v.UnsubscribeURL, v.PreferencesURL = manage, manage
	listID := r.firstList()
	out, err := r.render(v, listID)
	if err != nil {
		return skip(err.Error())
	}
	l, hasList := r.lists[listID]
	to := members[0].Email
	raw, err := email.Build(email.Outgoing{
		From: email.Address{Name: r.fromName, Email: r.from}, ReplyTo: email.Address{Name: r.fromName, Email: r.replyTo},
		To: email.Address{Email: to}, Subject: "[Test] " + v.Text(b.Subject), Text: out.Text, HTML: out.HTML, Date: s.now(),
		MessageID: email.NewMessageID(email.NewConversationToken(), r.domain), Headers: r.listHeaders(manage, l, hasList),
	})
	if err != nil {
		return err
	}
	if raw, err = signWith(r.dkim, raw); err != nil {
		return err
	}
	sendCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := s.sender.Send(sendCtx, r.cfg, r.from, []string{to}, raw); err != nil {
		if email.IsPermanent(err) || lastAttempt {
			return skip(err.Error())
		}
		return mail.RedactError(err)
	}
	return nil
}

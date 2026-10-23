package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	operatorDigestDelay = 5 * time.Minute
	// Sent notices are kept only to know when the last digest went out.
	operatorNoticeKeep = time.Hour
	noticeSignup       = "signup"
	noticeWorkspace    = "workspace"
	// noticeWorkspaceDeleted is the only record of a workspace's deletion that outlives it.
	noticeWorkspaceDeleted = "workspace_deleted"
	// noticeEmailUnproven names an e-mail channel that receives by address only once proven; the
	// upgrade that introduced proofs writes one per channel.
	noticeEmailUnproven = "email_unproven"
)

var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)

type operatorDigestWorker struct {
	river.WorkerDefaults[jobs.OperatorDigestArgs]
	s *Server
}

func (w *operatorDigestWorker) Work(ctx context.Context, _ *river.Job[jobs.OperatorDigestArgs]) error {
	return w.s.SendOperatorDigest(ctx)
}

// requestCountry is the visitor's country when Cloudflare is the trusted proxy: it sets
// CF-IPCountry itself, replacing whatever the client sent.
func (s *Server) requestCountry(r *http.Request) string {
	if r == nil || !strings.EqualFold(s.auth.ClientIPHeader, "CF-Connecting-IP") {
		return ""
	}
	c := r.Header.Get("CF-IPCountry")
	if !countryCode.MatchString(c) || c == "XX" || c == "T1" {
		return ""
	}
	return c
}

// noteOperator records a sign-up or a new workspace for the operator digest, in the transaction
// that made it.
func (s *Server) noteOperator(ctx context.Context, q *store.Queries, kind, email string, reader bool, workspace *string) error {
	if len(s.operator) == 0 {
		return nil
	}
	r := requestFrom(ctx)
	var ip string
	var country *string
	if r != nil {
		ip = s.clientIP(r)
		if c := s.requestCountry(r); c != "" {
			country = &c
		}
	}
	return q.CreateOperatorNotice(ctx, store.CreateOperatorNoticeParams{
		ID: newID(), Kind: kind, Email: email, Reader: reader, WorkspaceName: workspace, Ip: ip, Country: country, CreatedAt: s.now(),
	})
}

type operatorItem struct {
	Time, Email, Workspace, From, Title, Detail string
	Reader                                      bool
}

var broadcastNoticeTitles = map[string]string{
	noticeBroadcastApproval:  "Broadcast approval requested",
	noticeBroadcastSuspended: "Workspace suspended from broadcasts",
	noticeBroadcastPaused:    "Broadcast paused",
	noticeAnalyticsLimit:     "Analytics monthly limit reached",
	noticeAnalyticsDropped:   "Analytics events mostly dropped",
	noticeSESTenantPaused:    "SES tenant paused",
	noticeWorkspaceDeleted:   "Workspace deleted",
	noticeEmailUnproven:      "E-mail address to prove",
}

func oneLine(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r < ' ' || r == 0x7f || r == 0x2028 || r == 0x2029 }), " ")
}

// SendOperatorDigest e-mails the operator addresses the sign-ups and workspaces since the last
// digest, at most once per operatorDigestDelay.
func (s *Server) SendOperatorDigest(ctx context.Context) error {
	if len(s.operator) == 0 {
		return s.logEmailNotices(ctx)
	}
	now := s.now()
	var notices []store.OperatorNotice
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		if err := q.LockOperatorNotices(ctx); err != nil {
			return err
		}
		last, err := q.LastOperatorDigestAt(ctx)
		if err != nil && !store.IsNotFound(err) {
			return err
		}
		if last != nil && last.Add(operatorDigestDelay).After(now) {
			return nil
		}
		if notices, err = q.ListUnsentOperatorNotices(ctx); err != nil || len(notices) == 0 {
			return err
		}
		ids := make([]uuid.UUID, len(notices))
		for i, n := range notices {
			ids[i] = n.ID
		}
		if err := q.MarkOperatorNoticesSent(ctx, store.MarkOperatorNoticesSentParams{SentAt: now, Ids: ids}); err != nil {
			return err
		}
		return q.DeleteSentOperatorNotices(ctx, now.Add(-operatorNoticeKeep))
	})
	if err != nil || len(notices) == 0 {
		return err
	}
	var items []operatorItem
	signups, workspaces, broadcasts, analyticsNotices, deletions, addresses := 0, 0, 0, 0, 0, 0
	for _, n := range notices {
		it := operatorItem{Time: n.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"), Email: n.Email, Reader: n.Reader, From: n.Ip}
		if n.Country != nil {
			it.From = *n.Country
		}
		switch title, ok := broadcastNoticeTitles[n.Kind]; {
		case ok:
			it.Title, it.From = title, ""
			if n.WorkspaceName != nil {
				it.Workspace = oneLine(*n.WorkspaceName)
			}
			if n.Detail != nil {
				it.Detail = oneLine(*n.Detail)
			}
			switch n.Kind {
			case noticeAnalyticsLimit, noticeAnalyticsDropped:
				analyticsNotices++
			case noticeWorkspaceDeleted:
				deletions++
			case noticeEmailUnproven:
				addresses++
			default:
				broadcasts++
			}
		case n.Kind == noticeWorkspace && n.WorkspaceName != nil:
			it.Workspace = oneLine(*n.WorkspaceName)
			workspaces++
		default:
			signups++
		}
		items = append(items, it)
	}
	server := s.auth.PublicURL
	if u, err := url.Parse(server); err == nil && u.Host != "" {
		server = u.Host
	}
	msg, err := mail.Render("operator_digest", "en", map[string]any{
		"Items": items, "Signups": signups, "Workspaces": workspaces, "Broadcasts": broadcasts, "Analytics": analyticsNotices, "Deletions": deletions, "Addresses": addresses, "Server": server, "URL": s.auth.PublicURL,
	})
	if err != nil {
		return err
	}
	for _, to := range s.operator {
		m := msg
		m.To = to
		s.sendMail(ctx, m)
	}
	return nil
}

// logEmailNotices writes the e-mail address notices to the log once when the server has no
// operator address to mail them to.
func (s *Server) logEmailNotices(ctx context.Context) error {
	return s.st.InTx(ctx, func(q *store.Queries) error {
		if err := q.LockOperatorNotices(ctx); err != nil {
			return err
		}
		notices, err := q.ListUnsentOperatorNotices(ctx)
		if err != nil {
			return err
		}
		var ids []uuid.UUID
		for _, n := range notices {
			if n.Kind != noticeEmailUnproven {
				continue
			}
			s.log.WarnContext(ctx, "e-mail channel address must be proven; an operator who trusts it runs yuva channel prove",
				slog.String("workspace", deref(n.WorkspaceName)), slog.String("detail", deref(n.Detail)))
			ids = append(ids, n.ID)
		}
		if len(ids) == 0 {
			return nil
		}
		now := s.now()
		if err := q.MarkOperatorNoticesSent(ctx, store.MarkOperatorNoticesSentParams{SentAt: now, Ids: ids}); err != nil {
			return err
		}
		return q.DeleteSentOperatorNotices(ctx, now.Add(-operatorNoticeKeep))
	})
}

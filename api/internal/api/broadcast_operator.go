package api

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

// OperatorApproveBroadcasts lets a workspace send broadcasts (an approval, or lifting a suspension).
// Broadcasts it paused stay paused until a member resumes them.
func (s *Server) OperatorApproveBroadcasts(ctx context.Context, ws uuid.UUID, reason string) error {
	return s.answerBroadcastApproval(ctx, ws, wsAllowed, oas.AuditWorkspaceBroadcastsApproved, reason)
}

// OperatorDenyBroadcasts answers an approval request with no; the workspace may ask again.
func (s *Server) OperatorDenyBroadcasts(ctx context.Context, ws uuid.UUID, reason string) error {
	return s.answerBroadcastApproval(ctx, ws, wsApprovalRequired, oas.AuditWorkspaceBroadcastsDenied, reason)
}

func (s *Server) answerBroadcastApproval(ctx context.Context, ws uuid.UUID, status string, action oas.AuditAction, reason string) error {
	return s.st.InTx(ctx, func(q *store.Queries) error {
		w, err := q.SetWorkspaceBroadcastsStatus(ctx, store.SetWorkspaceBroadcastsStatusParams{ID: ws, Status: status, Reason: optionalText(reason, 500), Now: s.now()})
		if err != nil {
			return err
		}
		details := auditDetails{}
		if w.BroadcastsStatusReason != nil {
			details["reason"] = *w.BroadcastsStatusReason
		}
		return s.audit(ctx, q, principal{workspaceID: ws}, action, oas.AuditTargetWorkspace, ws, details)
	})
}

// OperatorSuspendBroadcasts stops a workspace from sending and pauses its running broadcasts.
func (s *Server) OperatorSuspendBroadcasts(ctx context.Context, ws uuid.UUID, reason string) error {
	if reason == "" {
		reason = "suspended by the server's operator"
	}
	return s.suspendBroadcasts(ctx, ws, reason, false)
}

// OperatorLimitBroadcasts sets the workspace's daily recipients limit; 0 removes it.
func (s *Server) OperatorLimitBroadcasts(ctx context.Context, ws uuid.UUID, daily int) error {
	if daily < 0 {
		return errors.New("--daily must be 0 or more")
	}
	var limit *int32
	if daily > 0 {
		limit = new(int32(daily))
	}
	return s.st.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.LockWorkspace(ctx, ws); err != nil {
			return err
		}
		cur, err := q.GetWorkspace(ctx, ws)
		if err != nil {
			return err
		}
		w, err := q.SetWorkspaceBroadcastDailyLimit(ctx, store.SetWorkspaceBroadcastDailyLimitParams{ID: ws, DailyLimit: limit})
		if err != nil || equalPtr(cur.BroadcastDailyLimit, w.BroadcastDailyLimit) {
			return err
		}
		return s.audit(ctx, q, principal{workspaceID: ws}, oas.AuditWorkspaceBroadcastLimitChanged, oas.AuditTargetWorkspace, ws,
			auditDetails{"from": cur.BroadcastDailyLimit, "to": w.BroadcastDailyLimit})
	})
}

// BroadcastSummary is what the operator sees of a workspace's broadcasts.
type BroadcastSummary struct {
	Status, Reason                  string
	DailyLimit, SentToday           int64
	Sent30, Bounced30, Complained30 int64
	SenderDomains                   []string
	Latest                          []BroadcastLine
	Triggered                       []TriggeredLine
}

type TriggeredLine struct {
	Name, State, Basis              string
	Sent30, Bounced30, Complained30 int64
}

type BroadcastLine struct {
	Name, Subject, State string
	Recipients, Sent     int64
	Bounced, Complained  int64
	CreatedAt            time.Time
}

func (s *Server) OperatorBroadcastSummary(ctx context.Context, ws uuid.UUID) (BroadcastSummary, error) {
	var out BroadcastSummary
	w, err := s.st.GetWorkspace(ctx, ws)
	if err != nil {
		return out, err
	}
	out.Status, out.Reason = w.BroadcastsStatus, deref(w.BroadcastsStatusReason)
	if out.DailyLimit, out.SentToday, err = s.dailyAllowance(ctx, s.st.Queries, w); err != nil {
		return out, err
	}
	days, err := s.st.ListSendingDays(ctx, store.ListSendingDaysParams{WorkspaceID: ws, Since: utcDay(s.now()).Add(-rateWindow)})
	if err != nil {
		return out, err
	}
	for _, d := range days {
		out.Sent30, out.Bounced30, out.Complained30 = out.Sent30+int64(d.Sent), out.Bounced30+int64(d.Bounced), out.Complained30+int64(d.Complained)
	}
	if out.SenderDomains, err = s.st.ListBroadcastSenderDomains(ctx, store.ListBroadcastSenderDomainsParams{WorkspaceID: ws, Since: new(s.now().Add(-rateWindow))}); err != nil {
		return out, err
	}
	rows, err := s.st.ListLatestBroadcasts(ctx, store.ListLatestBroadcastsParams{WorkspaceID: ws, Lim: 10})
	if err != nil {
		return out, err
	}
	for _, b := range rows {
		c := parseCounts(b.Counts)
		out.Latest = append(out.Latest, BroadcastLine{Name: b.Name, Subject: b.Subject, State: b.State, Recipients: c.Recipients,
			Sent: c.Sent, Bounced: c.Bounced, Complained: c.Complained, CreatedAt: b.CreatedAt})
	}
	live, err := s.st.ListLiveTriggeredSummary(ctx, store.ListLiveTriggeredSummaryParams{WorkspaceID: ws, Since: new(s.now().Add(-rateWindow))})
	if err != nil {
		return out, err
	}
	for _, t := range live {
		out.Triggered = append(out.Triggered, TriggeredLine{Name: t.Name, State: t.State, Basis: t.Basis, Sent30: t.Sent, Bounced30: t.Bounced, Complained30: t.Complained})
	}
	return out, nil
}

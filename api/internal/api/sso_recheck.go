package api

import (
	"context"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/productdevbook/yuva/api/internal/jobs"
	yuvamail "github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	ssoRecheckEvery = 24 * time.Hour
	ssoLapseAfter   = 7 * 24 * time.Hour
	ssoRecheckBatch = 500
)

// SSODomainCheckArgs looks a verified domain's TXT record up again. The lookup is a DNS call, so it
// runs on the outbound queue.
type SSODomainCheckArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	DomainID    uuid.UUID `json:"domain_id"`
}

func (SSODomainCheckArgs) Kind() string { return "sso_domain_check" }

func (SSODomainCheckArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.OutboundQueue, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable,
	}}}
}

// QueueSSODomainChecks queues a check of every verified domain not looked up for a day.
func (s *Server) QueueSSODomainChecks(ctx context.Context) error {
	due, err := s.st.ListSsoDomainsToRecheck(ctx, store.ListSsoDomainsToRecheckParams{Due: s.now().Add(-ssoRecheckEvery), Lim: ssoRecheckBatch})
	if err != nil || len(due) == 0 {
		return err
	}
	params := make([]river.InsertManyParams, len(due))
	for i, d := range due {
		params[i] = river.InsertManyParams{Args: SSODomainCheckArgs{WorkspaceID: d.WorkspaceID, DomainID: d.ID}}
	}
	_, err = s.jobs.InsertMany(ctx, params)
	return err
}

// RecheckSSODomain looks a verified domain's TXT record up again. A record missing for seven days
// in a row ends the claim: the domain goes back to pending, sso_required turns off with the last
// verified domain, and the owners are told.
func (s *Server) RecheckSSODomain(ctx context.Context, ws, id uuid.UUID) (store.SsoDomain, error) {
	d, err := s.st.GetSsoDomain(ctx, store.GetSsoDomainParams{WorkspaceID: ws, ID: id})
	if err != nil || d.VerifiedAt == nil {
		return d, err
	}
	seen, problem, failed := s.lookupSSODomain(ctx, d.Domain, d.Token)
	now := s.now()
	var prob *string
	if !seen {
		prob = &problem
	}
	d, err = s.st.MarkSsoDomainRechecked(ctx, store.MarkSsoDomainRecheckedParams{
		WorkspaceID: ws, ID: id, Now: now, Problem: prob, Found: seen, Missing: !seen && !failed,
	})
	if store.IsNotFound(err) {
		return s.st.GetSsoDomain(ctx, store.GetSsoDomainParams{WorkspaceID: ws, ID: id})
	}
	if err != nil || d.MissingSince == nil || now.Sub(*d.MissingSince) < ssoLapseAfter {
		return d, err
	}
	since := *d.MissingSince
	relaxed := false
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		d, err = q.LapseSsoDomain(ctx, store.LapseSsoDomainParams{WorkspaceID: ws, ID: id, Before: now.Add(-ssoLapseAfter)})
		if err != nil {
			return err
		}
		n, err := q.RelaxSsoWithoutDomains(ctx, store.RelaxSsoWithoutDomainsParams{WorkspaceID: ws, Now: now})
		if err != nil {
			return err
		}
		relaxed = n > 0
		return writeSystemAudit(ctx, q, ws, now, oas.AuditSsoDomainLapsed, oas.AuditTargetSsoDomain, d.ID,
			auditDetails{"domain": d.Domain, "missing_since": since, "sso_required_off": relaxed})
	})
	if store.IsNotFound(err) {
		return s.st.GetSsoDomain(ctx, store.GetSsoDomainParams{WorkspaceID: ws, ID: id})
	}
	if err != nil {
		return d, err
	}
	s.mailSSODomainLapsed(ctx, ws, d.Domain, since, relaxed)
	return d, nil
}

func (s *Server) mailSSODomainLapsed(ctx context.Context, ws uuid.UUID, domain string, since time.Time, relaxed bool) {
	w, err := s.st.GetWorkspace(ctx, ws)
	if err != nil {
		s.log.WarnContext(ctx, "sso domain mail", "workspace", ws, "error", err)
		return
	}
	owners, err := s.st.ListOwnerAddresses(ctx, ws)
	if err != nil {
		s.log.WarnContext(ctx, "sso domain mail", "workspace", ws, "error", err)
		return
	}
	for _, o := range owners {
		msg, err := yuvamail.Render("sso_domain_lapsed", o.Locale, map[string]any{
			"Workspace": w.Name, "Domain": domain, "Record": ssoTXT + domain, "Since": since.UTC().Format("2006-01-02"),
			"Days": int(ssoLapseAfter / (24 * time.Hour)), "RequiredOff": relaxed, "URL": s.auth.PublicURL + "/settings/sso",
		})
		if err != nil {
			s.log.WarnContext(ctx, "sso domain mail", "workspace", ws, "error", err)
			return
		}
		msg.To = o.Email
		s.sendMail(ctx, msg)
	}
}

type ssoDomainCheckWorker struct {
	river.WorkerDefaults[SSODomainCheckArgs]
	s *Server
}

func (w *ssoDomainCheckWorker) Work(ctx context.Context, job *river.Job[SSODomainCheckArgs]) error {
	_, err := w.s.RecheckSSODomain(ctx, job.Args.WorkspaceID, job.Args.DomainID)
	if store.IsNotFound(err) {
		return nil
	}
	return err
}

type ssoDomainSweepWorker struct {
	river.WorkerDefaults[jobs.SSODomainSweepArgs]
	s *Server
}

func (w *ssoDomainSweepWorker) Work(ctx context.Context, _ *river.Job[jobs.SSODomainSweepArgs]) error {
	return w.s.QueueSSODomainChecks(ctx)
}

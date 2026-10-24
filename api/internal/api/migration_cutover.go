package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/migrate"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	eventMigrationForwardingConfirmation = string(oas.MigrationForwardingConfirmationEventTypeMigrationForwardingConfirmation)

	verifiedForwarding = string(oas.VerifiedForwarding)
	verifiedDirect     = string(oas.VerifiedDirect)

	defaultSyncMaxDays = 30
	defaultAuditDays   = 365
	defaultSyncEvery   = 2 * time.Minute
	probeEvery         = time.Minute
	removeBatch        = 200

	migrationProbeSubject = "Yuva forwarding test"
	migrationProbeText    = "This message checks that mail to this mailbox reaches Yuva. Yuva recognises it and stores nothing; you can delete it.\r\n"

	reasonQuiet     = "Delivery to Yuva is verified and sync found nothing new for the quiet period."
	reasonSyncLimit = "Sync stopped after the server's limit of days; mail that still reaches only the old mailbox is no longer copied."
)

var (
	errConfirmCount = problem(http.StatusConflict, "confirm_count_mismatch", "the number of conversations to remove changed; show the new removable_conversations and ask again")
	errNoForwarding = errValidation("this server has no forwarding domain (YUVA_FORWARDING_DOMAIN)")
)

func (s *Server) forwardingAddress(e store.EmailChannel) string {
	if s.ingress.ForwardingDomain == "" || e.ForwardToken == "" {
		return ""
	}
	return e.ForwardToken + "@" + s.ingress.ForwardingDomain
}

// quietUntil is when a verified migration's quiet period ends: the period after verification, or
// after sync last stored mail that had not also reached Yuva directly.
func quietUntil(m store.Migration) *time.Time {
	if m.VerifiedAt == nil {
		return nil
	}
	base := *m.VerifiedAt
	if m.QuietSince != nil && m.QuietSince.After(base) {
		base = *m.QuietSince
	}
	return new(base.Add(time.Duration(migrationOptions(m.Options).QuietPeriodHours) * time.Hour))
}

type forwardingConfirmationDoc struct {
	Provider   string    `json:"provider"`
	Code       string    `json:"code"`
	Link       string    `json:"link,omitempty"`
	From       string    `json:"from,omitempty"`
	ReceivedAt time.Time `json:"received_at"`
}

func readForwardingConfirmation(raw []byte) *oas.MigrationForwardingConfirmation {
	if len(raw) == 0 {
		return nil
	}
	var d forwardingConfirmationDoc
	if json.Unmarshal(raw, &d) != nil || d.Code == "" {
		return nil
	}
	out := &oas.MigrationForwardingConfirmation{Provider: oas.MigrationForwardingConfirmationProvider(d.Provider), Code: d.Code, ReceivedAt: d.ReceivedAt}
	if d.Link != "" {
		out.Link = &d.Link
	}
	if d.From != "" {
		out.From = &d.From
	}
	return out
}

func probeInfo(workspaceID, migrationID uuid.UUID) string {
	return "migration-probe:" + workspaceID.String() + migrationID.String()
}

func probeMAC(key []byte, migrationID uuid.UUID) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(migrationID.String()))
	return mac.Sum(nil)
}

// probeValue is the Yuva-Probe header of a migration's test message: its id and an HMAC of it
// under a key only this server derives.
func (s *Server) probeValue(workspaceID, migrationID uuid.UUID) string {
	return migrationID.String() + "." + hex.EncodeToString(probeMAC(s.secrets.Derive(probeInfo(workspaceID, migrationID)), migrationID))
}

// probeMigration is the migration a Yuva-Probe header names when its HMAC is this server's.
func (s *Server) probeMigration(workspaceID uuid.UUID, header string) (uuid.UUID, bool) {
	id, mac, ok := strings.Cut(strings.TrimSpace(header), ".")
	if !ok {
		return uuid.Nil(), false
	}
	mid, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil(), false
	}
	got, err := hex.DecodeString(mac)
	if err != nil || !s.anyDerived(probeInfo(workspaceID, mid), func(k []byte) bool { return hmac.Equal(got, probeMAC(k, mid)) }) {
		return uuid.Nil(), false
	}
	return mid, true
}

func probeCode(value string) string {
	sum := sha256.Sum256([]byte(value))
	return strconv.FormatUint(uint64(sum[0])<<16|uint64(sum[1])<<8|uint64(sum[2]), 10)
}

// takeMigrationProbe recognises a migration's test message, before the own-sender rule would drop
// it, and records that delivery to Yuva works by route; nothing of it is stored.
func (s *Server) takeMigrationProbe(ctx context.Context, ch store.FindEmailChannelByAddressRow, m *email.Message, route string) (bool, error) {
	if m.MigrationProbe == "" {
		return false, nil
	}
	mid, ok := s.probeMigration(ch.WorkspaceID, m.MigrationProbe)
	if !ok {
		return false, nil
	}
	err := s.inTx(ctx, ch.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := q.GetChannelMigration(ctx, store.GetChannelMigrationParams{WorkspaceID: ch.WorkspaceID, ID: mid, ChannelID: &ch.ChannelID})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		v, err := q.VerifyMigrationDelivery(ctx, store.VerifyMigrationDeliveryParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID, Route: &route, Now: new(s.now())})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := q.AddMigrationAudit(ctx, store.AddMigrationAuditParams{
			WorkspaceID: cur.WorkspaceID, MigrationID: cur.ID, Action: "delivery_verified", Detail: mustJSON(map[string]any{"route": route}), Now: s.now(),
		}); err != nil {
			return err
		}
		return s.migrationEvent(ctx, q, events, v, &cur.State)
	})
	if err != nil {
		return false, err
	}
	s.log.InfoContext(ctx, "migration probe received", slog.String("migration_id", mid.String()), slog.String("route", route))
	return true, nil
}

// takeForwardingConfirmation keeps Google's forwarding confirmation to a channel's forwarding
// address on the migration waiting for it instead of opening a conversation; it needs a passing
// DMARC result from the trusted receiver.
func (s *Server) takeForwardingConfirmation(ctx context.Context, ch store.FindEmailChannelByAddressRow, m *email.Message) (bool, error) {
	c, ok := email.GoogleForwardingConfirmation(m)
	if !ok || m.DMARC(s.ingress.AuthservID) != email.DMARCPass {
		return false, nil
	}
	taken := false
	var body oas.MigrationForwardingConfirmation
	var migrationID uuid.UUID
	var personal *uuid.UUID
	err := s.inTx(ctx, ch.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := q.FindForwardingMigration(ctx, store.FindForwardingMigrationParams{WorkspaceID: ch.WorkspaceID, ChannelID: &ch.ChannelID})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		doc := forwardingConfirmationDoc{Provider: string(oas.ForwardingGoogle), Code: c.Code, Link: c.Link, From: c.From, ReceivedAt: s.now()}
		m, err := q.SetMigrationForwardingConfirmation(ctx, store.SetMigrationForwardingConfirmationParams{
			WorkspaceID: cur.WorkspaceID, ID: cur.ID, Confirmation: mustJSON(doc), Now: s.now(),
		})
		if err != nil {
			return err
		}
		taken, migrationID, body = true, m.ID, *readForwardingConfirmation(m.ForwardingConfirmation)
		if personal, err = migrationEventInbox(ctx, q, m); err != nil {
			return err
		}
		return s.migrationEvent(ctx, q, events, m, &cur.State)
	})
	if err != nil || !taken {
		return false, err
	}
	s.signal(ctx, realtime.Event{Type: eventMigrationForwardingConfirmation, WorkspaceID: ch.WorkspaceID, InboxID: personal,
		Data: mustJSON(oas.MigrationForwardingConfirmationData{MigrationId: migrationID, Confirmation: body})})
	return true, nil
}

// MigrationProbeArgs sends a migration's test message.
type MigrationProbeArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	MigrationID uuid.UUID `json:"migration_id"`
}

func (MigrationProbeArgs) Kind() string { return "migration_probe" }

func (MigrationProbeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.EmailQueue}
}

type migrationProbeWorker struct {
	river.WorkerDefaults[MigrationProbeArgs]
	s *Server
}

func (w *migrationProbeWorker) Work(ctx context.Context, job *river.Job[MigrationProbeArgs]) error {
	return w.s.SendMigrationProbeMail(ctx, job.Args, job.Attempt >= job.MaxAttempts)
}

func (s *Server) SendMigrationProbe(ctx context.Context, req oas.SendMigrationProbeRequestObject) (oas.SendMigrationProbeResponseObject, error) {
	out, err := s.migrationStep(ctx, req.MigrationId, func(q *store.Queries, events *eventBatch, p principal, cur store.Migration) (store.Migration, error) {
		if cur.State != migrationSyncing {
			return cur, errMigrationState("a test message is sent while the migration syncs, after the copy")
		}
		if cur.ChannelID == nil {
			return cur, errMigrationState("the migration's e-mail channel was deleted")
		}
		now := s.now()
		if cur.ProbeSentAt != nil && now.Before(cur.ProbeSentAt.Add(probeEvery)) {
			return cur, rateLimited(cur.ProbeSentAt.Add(probeEvery).Sub(now))
		}
		m, err := q.SetMigrationProbeSent(ctx, store.SetMigrationProbeSentParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID, Now: &now})
		if err != nil {
			return m, err
		}
		if _, err := s.migrationAudit(ctx, q, p, m.ID, "probe_sent", nil); err != nil {
			return m, err
		}
		events.job(MigrationProbeArgs{WorkspaceID: m.WorkspaceID, MigrationID: m.ID}, &river.InsertOpts{MaxAttempts: 3})
		return m, nil
	})
	if err != nil {
		return nil, err
	}
	return oas.SendMigrationProbe202JSONResponse(out), nil
}

// SendMigrationProbeMail mails the test message to the old mailbox from the server's mailer, or
// without one through the channel's SMTP account.
func (s *Server) SendMigrationProbeMail(ctx context.Context, a MigrationProbeArgs, lastAttempt bool) error {
	if live, err := s.workspaceLive(ctx, a.WorkspaceID); err != nil || !live {
		return err
	}
	m, err := s.st.GetMigration(ctx, store.GetMigrationParams{WorkspaceID: a.WorkspaceID, ID: a.MigrationID})
	if store.IsNotFound(err) || (err == nil && (m.State != migrationSyncing || m.ChannelID == nil)) {
		return nil
	}
	if err != nil {
		return err
	}
	cfg, from := email.SMTPConfig{}, ""
	var sesH []email.Header
	if s.migrations.ProbeSMTP != nil && s.migrations.ProbeFrom != "" {
		cfg, from = *s.migrations.ProbeSMTP, s.migrations.ProbeFrom
	} else {
		e, err := s.st.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: m.WorkspaceID, ChannelID: *m.ChannelID})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if e.SmtpHost == "" {
			s.addMigrationError(ctx, m, "probe_failed", "neither the server nor the channel has an SMTP account to send the test message", "", nil, "")
			return nil
		}
		cfg = email.SMTPConfig{Host: e.SmtpHost, Port: int(e.SmtpPort), Username: e.SmtpUsername, TLS: e.SmtpTls}
		if len(e.SmtpPassword) > 0 {
			plain, err := s.secrets.Open(e.SmtpPassword, smtpSecretContext(e.WorkspaceID, e.ChannelID))
			if err != nil {
				s.addMigrationError(ctx, m, "probe_failed", "the channel's SMTP password cannot be decrypted with this master key", "", nil, "")
				return nil
			}
			cfg.Password = string(plain)
		}
		from = e.Address
		if e.FromAddress != nil {
			from = *e.FromAddress
		}
		if sesH, err = s.sesHeaders(ctx, e, cfg); err != nil {
			return err
		}
	}
	value := s.probeValue(m.WorkspaceID, m.ID)
	raw, err := email.Build(email.Outgoing{
		From: email.Address{Name: "Yuva", Email: from}, To: email.Address{Email: m.SourceAddress},
		Subject: migrationProbeSubject + " " + probeCode(value), Text: migrationProbeText, Date: s.now(),
		MessageID: email.NewMessageID(email.NewConversationToken(), email.Domain(from)), AutoSubmitted: true,
		Headers: append([]email.Header{{Name: email.MigrationProbeHeader, Value: value}, {Name: "X-Auto-Response-Suppress", Value: "All"}}, sesH...),
	})
	if err != nil {
		return err
	}
	sendCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := s.sender.Send(sendCtx, cfg, from, []string{m.SourceAddress}, raw); err != nil {
		if email.IsPermanent(err) || lastAttempt {
			s.addMigrationError(ctx, m, "probe_failed", migrate.Scrub(err.Error(), cfg.Password), "", nil, "")
			return nil
		}
		return mail.RedactError(err)
	}
	return nil
}

// completeMigration ends a migration as completed and deletes its credentials in the same
// transaction; p is nil when sync ends it on its own.
func (s *Server) completeMigration(ctx context.Context, q *store.Queries, events *eventBatch, p *principal, cur store.Migration, reason, why string) (store.Migration, error) {
	n, err := q.DeleteMigrationCredentials(ctx, store.DeleteMigrationCredentialsParams{WorkspaceID: cur.WorkspaceID, MigrationID: cur.ID})
	if err != nil {
		return cur, err
	}
	if err := q.DeleteMigrationSignIn(ctx, store.DeleteMigrationSignInParams{WorkspaceID: cur.WorkspaceID, MigrationID: cur.ID}); err != nil {
		return cur, err
	}
	m, err := q.FinishMigration(ctx, store.FinishMigrationParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID, State: migrationCompleted, StateReason: reason, Now: new(s.now())})
	if err != nil {
		return m, err
	}
	if err := clearMigrationPersonalData(ctx, q, m.WorkspaceID, m.ID); err != nil {
		return m, err
	}
	audit := func(action string, detail map[string]any) error {
		if p != nil {
			_, err := s.migrationAudit(ctx, q, *p, m.ID, action, detail)
			return err
		}
		_, err := q.AddMigrationAudit(ctx, store.AddMigrationAuditParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Action: action, Detail: mustJSON(detail), Now: s.now()})
		return err
	}
	if err := audit("finished", map[string]any{"state": cur.State, "by": why}); err != nil {
		return m, err
	}
	if n > 0 {
		if err := audit("credentials_deleted", map[string]any{}); err != nil {
			return m, err
		}
	}
	return m, s.migrationChanged(ctx, q, events, m, &cur.State)
}

func (s *Server) FinishMigration(ctx context.Context, req oas.FinishMigrationRequestObject) (oas.FinishMigrationResponseObject, error) {
	force := req.Body != nil && req.Body.Force != nil && *req.Body.Force
	p := principalFrom(ctx)
	var out oas.Migration
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := visibleMigration(ctx, q, p, req.MigrationId, true)
		if err != nil {
			return err
		}
		m := cur
		switch {
		case cur.State == migrationCompleted:
		case cur.State == migrationSyncing, cur.State == migrationNeedsAttention && cur.LiveFrom != nil:
			if cur.VerifiedAt == nil && !force {
				return errMigrationState("delivery to Yuva is not verified yet: send a test message, or finish with force to stop copying mail from the old mailbox anyway")
			}
			if err := q.ClearMigrationLease(ctx, store.ClearMigrationLeaseParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID}); err != nil {
				return err
			}
			by := "request"
			if force && cur.VerifiedAt == nil {
				by = "request_unverified"
			}
			if m, err = s.completeMigration(ctx, q, events, &p, cur, "", by); err != nil {
				return err
			}
		default:
			return errMigrationState("a migration is finished while it syncs, after its copy")
		}
		out, err = s.oneMigration(ctx, q, m)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.FinishMigration200JSONResponse(out), nil
}

// MigrationRemoveArgs removes a migration's imported history.
type MigrationRemoveArgs struct{ MigrationJobArgs }

func (MigrationRemoveArgs) Kind() string { return "migration_remove" }

type migrationRemoveWorker struct {
	river.WorkerDefaults[MigrationRemoveArgs]
	s *Server
}

func (w *migrationRemoveWorker) Timeout(*river.Job[MigrationRemoveArgs]) time.Duration {
	return migrationJobTimeout
}

func (w *migrationRemoveWorker) Work(ctx context.Context, job *river.Job[MigrationRemoveArgs]) error {
	return w.s.workMigration(ctx, job.Args.MigrationJobArgs, job.ID)
}

func (s *Server) RemoveMigrationHistory(ctx context.Context, req oas.RemoveMigrationHistoryRequestObject) (oas.RemoveMigrationHistoryResponseObject, error) {
	var confirm *int64
	if req.Body != nil {
		confirm = req.Body.ConfirmCount
	}
	if confirm != nil && *confirm < 0 {
		return nil, errValidation("confirm_count must be 0 or more")
	}
	out, err := s.migrationStep(ctx, req.MigrationId, func(q *store.Queries, events *eventBatch, p principal, cur store.Migration) (store.Migration, error) {
		switch cur.State {
		case migrationRemoving, migrationRemoved:
			return cur, nil
		case migrationCompleted, migrationCancelled:
		default:
			return cur, errMigrationState("imported history is removed once the migration is completed or cancelled")
		}
		counts, err := q.CountMigrationHistory(ctx, store.CountMigrationHistoryParams{WorkspaceID: cur.WorkspaceID, MigrationIds: []uuid.UUID{cur.ID}})
		if err != nil {
			return cur, err
		}
		var removable, total int64
		if len(counts) > 0 {
			removable, total = counts[0].Removable, counts[0].Conversations
		}
		if confirm != nil && *confirm != removable {
			return cur, errConfirmCount
		}
		m, err := s.setMigrationState(ctx, q, cur, migrationRemoving)
		if err != nil {
			return m, err
		}
		if _, err := s.migrationAudit(ctx, q, p, m.ID, "history_removed", map[string]any{"conversations": removable, "kept": total - removable}); err != nil {
			return m, err
		}
		events.job(migrationJob(m, 0), migrationJobOpts())
		return m, nil
	})
	if err != nil {
		return nil, err
	}
	return oas.RemoveMigrationHistory202JSONResponse(out), nil
}

// remove deletes the migration's conversations nobody wrote in, 200 at a time, then the contacts
// it created that are left without a conversation or message.
func (r *migrationRun) remove(ctx context.Context) (MigrationSlice, error) {
	s, m := r.s, r.m
	for r.timeLeft() {
		ok, err := r.renew(ctx)
		if err != nil || !ok {
			return MigrationSlice{Done: true}, err
		}
		n, _, err := s.deleteConversationBatch(ctx, m.WorkspaceID, true, func(q *store.Queries, _ *eventBatch) ([]uuid.UUID, error) {
			return q.ListRemovableMigrationConversations(ctx, store.ListRemovableMigrationConversationsParams{WorkspaceID: m.WorkspaceID, MigrationID: &m.ID, MaxRows: removeBatch})
		})
		if err != nil {
			return MigrationSlice{}, err
		}
		if n == removeBatch {
			continue
		}
		return MigrationSlice{Done: true}, s.inTx(ctx, m.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
			cur, err := q.LockMigration(ctx, store.LockMigrationParams{WorkspaceID: m.WorkspaceID, ID: m.ID})
			if err != nil || cur.State != migrationRemoving {
				return err
			}
			if _, err := q.DeleteLeftoverMigrationContacts(ctx, store.DeleteLeftoverMigrationContactsParams{WorkspaceID: m.WorkspaceID, MigrationID: &m.ID}); err != nil {
				return err
			}
			done, err := s.setMigrationState(ctx, q, cur, migrationRemoved)
			if err != nil {
				return err
			}
			r.m = done
			return s.migrationChanged(ctx, q, events, done, &cur.State)
		})
	}
	return MigrationSlice{}, nil
}

// CleanMigrationAudit deletes audit entries of migrations that ended, or were deleted, longer ago
// than the audit period.
func (s *Server) CleanMigrationAudit(ctx context.Context) error {
	before := s.now().AddDate(0, 0, -s.migrations.AuditDays)
	rows, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range rows {
		if _, err := s.st.DeleteOldMigrationAudit(ctx, store.DeleteOldMigrationAuditParams{WorkspaceID: ws, Before: before}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) RotateForwardingAddress(ctx context.Context, req oas.RotateForwardingAddressRequestObject) (oas.RotateForwardingAddressResponseObject, error) {
	p := principalFrom(ctx)
	if err := s.requireChannelManager(ctx, p, uuid.Nil(), req.ChannelId); err != nil {
		return nil, err
	}
	c, err := s.visibleChannel(ctx, p, req.ChannelId)
	if err != nil {
		return nil, err
	}
	if c.Kind != string(oas.ChannelKindEmail) {
		return nil, errValidation("only e-mail channels have a forwarding address")
	}
	if s.ingress.ForwardingDomain == "" {
		return nil, errNoForwarding
	}
	var e store.EmailChannel
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		var err error
		if e, err = q.RotateForwardToken(ctx, store.RotateForwardTokenParams{WorkspaceID: p.workspaceID, ChannelID: c.ID}); err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditChannelForwardingAddressRotated, oas.AuditTargetChannel, c.ID, nil)
	})
	if store.IsNotFound(err) {
		return nil, errValidation("only e-mail channels have a forwarding address")
	}
	if err != nil {
		return nil, err
	}
	t, err := s.channelTenant(ctx, s.st.Queries, e)
	if err != nil {
		return nil, err
	}
	return oas.RotateForwardingAddress200JSONResponse(s.channelBody(c, &e, nil, t)), nil
}

func errorsAsIngest(err error) (*IngestError, bool) {
	var ie *IngestError
	return ie, errors.As(err, &ie)
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/migrate"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/migrate/imapread"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	eventMigrationProgress = string(oas.MigrationProgressEventTypeMigrationProgress)

	migrationSliceBudget       = 25 * time.Second
	migrationLease             = 45 * time.Second
	migrationJobTimeout        = 75 * time.Second
	migrationConnectTimeout    = 30 * time.Second
	migrationItemAttempts      = 5
	migrationProgressEvery     = 2 * time.Second
	migrationRateWindow        = 5 * time.Minute
	migrationNetworkFailures   = 8
	defaultMigrationMaxMessage = 50 << 20
)

var migrationJobStates = []rivertype.JobState{
	rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable,
}

// MigrationJobArgs name a migration; Resume is the job a watchdog replaces, so its job is unique
// even while the dead one still looks running.
type MigrationJobArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	MigrationID uuid.UUID `json:"migration_id"`
	Resume      int64     `json:"resume,omitempty"`
}

type MigrationScanArgs struct{ MigrationJobArgs }

func (MigrationScanArgs) Kind() string { return "migration_scan" }

type MigrationCopyArgs struct{ MigrationJobArgs }

func (MigrationCopyArgs) Kind() string { return "migration_copy" }

func migrationJobOpts() *river.InsertOpts {
	return &river.InsertOpts{Queue: jobs.MigrationQueue, MaxAttempts: 10, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: migrationJobStates}}
}

func migrationJob(m store.Migration, resume int64) river.JobArgs {
	a := MigrationJobArgs{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Resume: resume}
	switch m.State {
	case migrationScanning:
		return MigrationScanArgs{a}
	case migrationSyncing:
		return MigrationSyncArgs{a}
	case migrationRemoving:
		return MigrationRemoveArgs{a}
	}
	return MigrationCopyArgs{a}
}

var migrationJobKinds = []string{
	MigrationScanArgs{}.Kind(), MigrationCopyArgs{}.Kind(), MigrationSyncArgs{}.Kind(), MigrationRemoveArgs{}.Kind(),
}

type migrationScanWorker struct {
	river.WorkerDefaults[MigrationScanArgs]
	s *Server
}

func (w *migrationScanWorker) Timeout(*river.Job[MigrationScanArgs]) time.Duration {
	return migrationJobTimeout
}

func (w *migrationScanWorker) Work(ctx context.Context, job *river.Job[MigrationScanArgs]) error {
	return w.s.workMigration(ctx, job.Args.MigrationJobArgs, job.ID)
}

type migrationCopyWorker struct {
	river.WorkerDefaults[MigrationCopyArgs]
	s *Server
}

func (w *migrationCopyWorker) Timeout(*river.Job[MigrationCopyArgs]) time.Duration {
	return migrationJobTimeout
}

func (w *migrationCopyWorker) Work(ctx context.Context, job *river.Job[MigrationCopyArgs]) error {
	return w.s.workMigration(ctx, job.Args.MigrationJobArgs, job.ID)
}

type migrationWatchdogWorker struct {
	river.WorkerDefaults[jobs.MigrationWatchdogArgs]
	s *Server
}

func (w *migrationWatchdogWorker) Work(ctx context.Context, _ *river.Job[jobs.MigrationWatchdogArgs]) error {
	_, err := w.s.RunMigrationWatchdog(ctx)
	return err
}

func (s *Server) workMigration(ctx context.Context, a MigrationJobArgs, jobID int64) error {
	res, err := s.RunMigrationSlice(ctx, a, jobID, migrationSliceBudget)
	if err != nil {
		return err
	}
	if res.Done {
		return nil
	}
	if d := res.Until.Sub(s.now()); d > 0 {
		return river.JobSnooze(d)
	}
	return river.JobSnooze(0)
}

// MigrationSlice is the outcome of one slice of a migration's work: Done when its job may end,
// else Until when it waits for the source.
type MigrationSlice struct {
	Done  bool
	Until time.Time
}

type migrationCheckpoint struct {
	Phase   string        `json:"phase,omitempty"`
	Plan    *planProgress `json:"plan,omitempty"`
	Samples [][2]int64    `json:"samples,omitempty"`
}

func readCheckpoint(raw []byte) migrationCheckpoint {
	var c migrationCheckpoint
	_ = json.Unmarshal(raw, &c)
	return c
}

func readThrottle(raw []byte) migrate.Throttle {
	var t migrate.Throttle
	_ = json.Unmarshal(raw, &t)
	return t
}

type migrationRun struct {
	s         *Server
	m         store.Migration
	job       int64
	deadline  time.Time
	opts      oas.MigrationOptions
	preset    migrate.Preset
	throttle  migrate.Throttle
	reader    *sourceReader
	file      *uploadFile
	deskSrc   helpdesk.Source
	host      string
	secrets   []string
	lastFlush time.Time
}

// RunMigrationSlice works on a scanning, copying, syncing or removing migration for about budget,
// under a lease on the migration row so only one job works on it at a time.
func (s *Server) RunMigrationSlice(ctx context.Context, a MigrationJobArgs, jobID int64, budget time.Duration) (MigrationSlice, error) {
	if live, err := s.workspaceLive(ctx, a.WorkspaceID); err != nil || !live {
		return MigrationSlice{Done: true}, err
	}
	now := s.now()
	m, err := s.st.TakeMigrationLease(ctx, store.TakeMigrationLeaseParams{
		WorkspaceID: a.WorkspaceID, ID: a.MigrationID, JobID: &jobID, States: []string{migrationScanning, migrationCopying, migrationSyncing, migrationRemoving},
		Now: &now, Until: new(now.Add(migrationLease)),
	})
	if store.IsNotFound(err) {
		return MigrationSlice{Done: true}, nil
	}
	if err != nil {
		return MigrationSlice{}, err
	}
	if m.WaitingUntil != nil && now.Before(*m.WaitingUntil) {
		return MigrationSlice{Until: *m.WaitingUntil}, nil
	}
	preset, _ := migrate.LookupPreset(m.Preset)
	r := &migrationRun{s: s, m: m, job: jobID, deadline: time.Now().Add(budget), opts: migrationOptions(m.Options), preset: preset,
		throttle: readThrottle(m.Throttle)}
	defer r.close()
	var res MigrationSlice
	switch m.State {
	case migrationScanning:
		res, err = r.scan(ctx)
	case migrationSyncing:
		res, err = r.sync(ctx)
	case migrationRemoving:
		res, err = r.remove(ctx)
	default:
		res, err = r.copy(ctx)
	}
	if err == nil && res.Done {
		_ = s.st.ReleaseMigrationLease(context.WithoutCancel(ctx), store.ReleaseMigrationLeaseParams{WorkspaceID: m.WorkspaceID, ID: m.ID, JobID: &jobID})
	}
	return res, err
}

func (r *migrationRun) close() {
	if r.file != nil {
		r.file.Close()
		r.file = nil
	}
	if r.reader != nil {
		_ = r.reader.r.Close()
		r.reader = nil
	}
	r.deskSrc = nil
}

func (r *migrationRun) timeLeft() bool { return time.Now().Before(r.deadline) }

// renew extends the lease while the migration is still in the state this run works on.
func (r *migrationRun) renew(ctx context.Context) (bool, error) {
	now := r.s.now()
	m, err := r.s.st.TakeMigrationLease(ctx, store.TakeMigrationLeaseParams{
		WorkspaceID: r.m.WorkspaceID, ID: r.m.ID, JobID: &r.job, States: []string{r.m.State}, Now: &now, Until: new(now.Add(migrationLease)),
	})
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	r.m = m
	return true, nil
}

// sourceReader bounds every read of the mailbox, so a server that stops answering fails the
// read well before the job's own time runs out. It remembers the folder it has open.
type sourceReader struct {
	r      *imapread.Reader
	open   string
	status imapread.Status
}

const migrationCallTimeout = 40 * time.Second

func (s *sourceReader) Folders(ctx context.Context) ([]imapread.Folder, error) {
	ctx, cancel := context.WithTimeout(ctx, migrationCallTimeout)
	defer cancel()
	return s.r.Folders(ctx)
}

func (s *sourceReader) Status(ctx context.Context, name string) (imapread.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, migrationCallTimeout)
	defer cancel()
	return s.r.Status(ctx, name)
}

func (s *sourceReader) Examine(ctx context.Context, name string) (imapread.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, migrationCallTimeout)
	defer cancel()
	s.open = ""
	st, err := s.r.Examine(ctx, name)
	if err == nil {
		s.open, s.status = name, st
	}
	return st, err
}

// Open examines a folder unless it is the one already open: opening a folder makes the server
// look through all of it again, which for a large maildir takes most of a second.
func (s *sourceReader) Open(ctx context.Context, name string) (imapread.Status, error) {
	if s.open == name {
		return s.status, nil
	}
	return s.Examine(ctx, name)
}

func (s *sourceReader) SearchUIDs(ctx context.Context, since time.Time) ([]uint32, error) {
	ctx, cancel := context.WithTimeout(ctx, migrationCallTimeout)
	defer cancel()
	return s.r.SearchUIDs(ctx, since)
}

func (s *sourceReader) FetchHeaders(ctx context.Context, uids []uint32, fields []string, each func(imapread.Header) error) error {
	ctx, cancel := context.WithTimeout(ctx, migrationCallTimeout)
	defer cancel()
	return s.r.FetchHeaders(ctx, uids, fields, each)
}

func (s *sourceReader) FetchRaw(ctx context.Context, uids []uint32, maxBytes int64, each func(imapread.Message) error) error {
	ctx, cancel := context.WithTimeout(ctx, migrationCallTimeout)
	defer cancel()
	return s.r.FetchRaw(ctx, uids, maxBytes, each)
}

// source signs in to the mailbox once per slice.
func (r *migrationRun) source(ctx context.Context) (*sourceReader, error) {
	if r.reader != nil {
		return r.reader, nil
	}
	s := r.s
	cred, err := s.st.GetMigrationCredentials(ctx, store.GetMigrationCredentialsParams{WorkspaceID: r.m.WorkspaceID, MigrationID: r.m.ID})
	if store.IsNotFound(err) {
		return nil, &imapread.ServerError{Type: "NO", Code: "AUTHENTICATIONFAILED", Text: "no credentials are stored"}
	}
	if err != nil {
		return nil, err
	}
	r.host = cred.Host
	var login func(ctx context.Context, reader *imapread.Reader) error
	if microsoftMigration(r.m) && cred.Kind == string(oas.CredentialsOauth) {
		sec, err := s.microsoftCredentials(ctx, r.m, cred)
		r.secrets = sec.secrets()
		if err != nil {
			return nil, err
		}
		login = func(ctx context.Context, reader *imapread.Reader) error {
			return reader.LoginXOAUTH2(ctx, sec.Account, sec.Tokens.Access)
		}
	} else {
		plain, err := s.secrets.Open(cred.Sealed, migrationSecretContext(r.m.WorkspaceID, r.m.ID))
		if err != nil {
			return nil, err
		}
		var sec migrationSecret
		if err := json.Unmarshal(plain, &sec); err != nil {
			return nil, err
		}
		r.secrets = []string{sec.Username, sec.Password}
		login = func(ctx context.Context, reader *imapread.Reader) error {
			return reader.Login(ctx, sec.Username, sec.Password)
		}
	}
	cctx, cancel := context.WithTimeout(ctx, migrationConnectTimeout)
	defer cancel()
	dial := s.sourceDial(cred.Host)
	if dial == nil {
		ips, err := migrate.Addresses(cctx, s.migrations.Resolver, cred.Host, s.migrations.AllowPrivate)
		if err != nil {
			return nil, err
		}
		dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return migrate.DialAddresses(ctx, ips, int(cred.Port), 10*time.Second)
		}
	}
	cfg := imapread.Config{Host: cred.Host, Port: int(cred.Port), TLS: imapread.TLSMode(cred.Tls), RootCAs: s.migrations.RootCAs, Dial: dial}
	if cred.CertificateSha256 != nil {
		cfg.CertificateSHA256 = *cred.CertificateSha256
	}
	reader, err := imapread.Open(cctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := login(cctx, reader); err != nil {
		_ = reader.Close()
		return nil, err
	}
	r.reader = &sourceReader{r: reader}
	return r.reader, nil
}

func (r *migrationRun) saveThrottle(ctx context.Context) error {
	return r.s.st.SetMigrationThrottle(ctx, store.SetMigrationThrottleParams{WorkspaceID: r.m.WorkspaceID, ID: r.m.ID, Throttle: mustJSON(r.throttle)})
}

// clearWaiting drops a wait reason once the source answers again.
func (r *migrationRun) clearWaiting(ctx context.Context) error {
	if r.throttle.Backoff == 0 && r.m.WaitingUntil == nil && r.m.StateReason == "" {
		return nil
	}
	r.throttle.Backoff = 0
	if err := r.saveThrottle(ctx); err != nil {
		return err
	}
	if r.m.WaitingUntil == nil && r.m.StateReason == "" {
		return nil
	}
	m, err := r.s.st.SetMigrationRunState(ctx, store.SetMigrationRunStateParams{WorkspaceID: r.m.WorkspaceID, ID: r.m.ID, State: r.m.State, Now: r.s.now()})
	if err == nil {
		r.m = m
	}
	return err
}

const (
	reasonReconnect   = "The mailbox no longer accepts the password; reconnect it to continue."
	reasonSignIn      = "Microsoft no longer accepts Yuva's sign-in to the mailbox (consent withdrawn, password changed or the sign-in expired); sign in with Microsoft again to continue."
	reasonIMAPOff     = "The mailbox refused Yuva's Microsoft sign-in over IMAP, usually because IMAP was turned off for it; ask an admin to turn IMAP on for the mailbox, then sign in again."
	reasonRefused     = "Yuva kept a command from the mailbox that could have changed it; nothing was changed. Tell the server's operator."
	reasonUnreachable = "The mail server has not answered for a while; check it, then resume."
	reasonDaily       = "Waiting for the provider's daily download limit."
	reasonLimit       = "The provider asked Yuva to slow down; continuing shortly."
	reasonNetwork     = "The mail server did not answer; trying again shortly."
	reasonMonthly     = "This workspace imported as much as the server allows this month; continuing next month."
)

// trouble handles a failure of the source: a needed reconnect stops the migration in
// needs_attention, a limit or a network problem waits.
func (r *migrationRun) trouble(ctx context.Context, err error) (MigrationSlice, error) {
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		return MigrationSlice{}, ctx.Err()
	}
	r.close()
	kind, code, detail := migrate.Classify(err, r.host, r.secrets...)
	now := r.s.now()
	var until time.Time
	reason := ""
	switch kind {
	case migrate.TroubleRefused:
		r.s.log.ErrorContext(ctx, "migration source write refused", slog.String("migration_id", r.m.ID.String()), slog.String("command", detail))
		return MigrationSlice{Done: true}, r.needsAttention(ctx, code, detail, reasonRefused)
	case migrate.TroubleReconnect:
		reason := reasonReconnect
		switch code {
		case migrate.CodeTokenRefreshFailed, migrate.CodeScopeTooBroad:
			reason = reasonSignIn
		case migrate.CodeXOAUTH2Failed:
			reason = reasonIMAPOff
		}
		return MigrationSlice{Done: true}, r.needsAttention(ctx, code, detail, reason)
	case migrate.TroubleDaily:
		until, reason = r.throttle.WindowEnd(now), reasonDaily
	case migrate.TroubleLimit:
		until, reason = r.throttle.Retry(now), reasonLimit
		var limit *helpdesk.LimitError
		if errors.As(err, &limit) {
			until = limit.Until
		}
	default:
		until, reason = r.throttle.Retry(now), reasonNetwork
		if r.throttle.Backoff >= migrationNetworkFailures {
			r.throttle.Backoff = 0
			if err := r.saveThrottle(ctx); err != nil {
				return MigrationSlice{}, err
			}
			return MigrationSlice{Done: true}, r.needsAttention(ctx, code, detail, reasonUnreachable)
		}
	}
	if err := r.saveThrottle(ctx); err != nil {
		return MigrationSlice{}, err
	}
	if r.throttle.Backoff <= 1 || kind == migrate.TroubleDaily {
		r.s.addMigrationError(ctx, r.m, code, detail, "", nil, "")
	}
	m, err := r.s.st.SetMigrationRunState(ctx, store.SetMigrationRunStateParams{
		WorkspaceID: r.m.WorkspaceID, ID: r.m.ID, State: r.m.State, StateReason: reason, WaitingUntil: &until, Now: now,
	})
	if err != nil {
		return MigrationSlice{}, err
	}
	r.m = m
	r.s.migrationProgress(ctx, r.m)
	return MigrationSlice{Until: until}, nil
}

func (r *migrationRun) needsAttention(ctx context.Context, code, detail, reason string) error {
	r.s.addMigrationError(ctx, r.m, code, detail, "", nil, "")
	return r.s.inTx(ctx, r.m.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := q.LockMigration(ctx, store.LockMigrationParams{WorkspaceID: r.m.WorkspaceID, ID: r.m.ID})
		if err != nil || cur.State != r.m.State {
			return err
		}
		m, err := q.SetMigrationRunState(ctx, store.SetMigrationRunStateParams{
			WorkspaceID: cur.WorkspaceID, ID: cur.ID, State: migrationNeedsAttention, StateReason: reason, Now: r.s.now(),
		})
		if err != nil {
			return err
		}
		if _, err := q.AddMigrationAudit(ctx, store.AddMigrationAuditParams{
			WorkspaceID: cur.WorkspaceID, MigrationID: cur.ID, Action: "needs_attention", Detail: mustJSON(map[string]any{"code": code}), Now: r.s.now(),
		}); err != nil {
			return err
		}
		r.m = m
		return r.s.migrationChanged(ctx, q, events, m, &cur.State)
	})
}

func (s *Server) addMigrationError(ctx context.Context, m store.Migration, code, detail, folder string, at *time.Time, subject string) {
	err := s.st.AddMigrationError(context.WithoutCancel(ctx), store.AddMigrationErrorParams{
		WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Code: code, Detail: truncateRunes(detail, 2000), Folder: truncateRunes(folder, 1000),
		ItemDate: at, ItemSubject: truncateRunes(subject, 500), Now: s.now(),
	})
	if err != nil {
		s.log.WarnContext(ctx, "migration error row", slog.Any("error", err))
	}
}

// migrationCounts recounts the copy from its items and stores the counts on the folders and the
// migration.
func (s *Server) refreshMigrationCounts(ctx context.Context, q *store.Queries, m store.Migration) (oas.MigrationCounts, []oas.MigrationFolderProgress, error) {
	rows, err := q.MigrationCopyCounts(ctx, store.MigrationCopyCountsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
	if err != nil {
		return oas.MigrationCounts{}, nil, err
	}
	var total oas.MigrationCounts
	byFolder := map[int32]*oas.MigrationFolderProgress{}
	var order []int32
	for _, r := range rows {
		id := int32(-1)
		if r.FolderID != nil {
			id = *r.FolderID
		}
		f := byFolder[id]
		if f == nil {
			f = &oas.MigrationFolderProgress{Id: id}
			byFolder[id] = f
			order = append(order, id)
		}
		total.Total += r.N
		switch r.State {
		case migrate.StateImported:
			total.Imported += r.N
			f.Imported += r.N
		case migrate.StateDuplicate:
			total.Duplicate += r.N
			f.Duplicate += r.N
		case migrate.StateSkipped:
			total.Skipped += r.N
			f.Skipped += r.N
		case migrate.StateFailed:
			total.Failed += r.N
			f.Failed += r.N
		}
		if r.State != migrate.StatePlanned {
			total.Bytes += r.Bytes
			f.Bytes += r.Bytes
		}
	}
	folders := make([]oas.MigrationFolderProgress, 0, len(order))
	var ids, imported, duplicate, skipped, failed []int32
	var bytes []int64
	for _, id := range order {
		f := byFolder[id]
		if id < 0 {
			continue
		}
		folders = append(folders, *f)
		ids, imported, duplicate, skipped, failed = append(ids, id), append(imported, int32(f.Imported)), append(duplicate, int32(f.Duplicate)),
			append(skipped, int32(f.Skipped)), append(failed, int32(f.Failed))
		bytes = append(bytes, f.Bytes)
	}
	if len(ids) > 0 {
		if err := q.SetMigrationFolderCounts(ctx, store.SetMigrationFolderCountsParams{
			WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Ids: ids, Imported: imported, Duplicate: duplicate, Skipped: skipped, Failed: failed, Bytes: bytes,
		}); err != nil {
			return total, folders, err
		}
	}
	return total, folders, q.SetMigrationCounts(ctx, store.SetMigrationCountsParams{WorkspaceID: m.WorkspaceID, ID: m.ID, Counts: mustJSON(total)})
}

func done(c oas.MigrationCounts) int64 { return c.Imported + c.Duplicate + c.Skipped + c.Failed }

// migrationRate is messages a minute over the last five minutes, from the samples each flush keeps.
func migrationRate(c migrationCheckpoint, counts oas.MigrationCounts) (*int64, *int64) {
	if len(c.Samples) < 2 {
		return nil, nil
	}
	first, last := c.Samples[0], c.Samples[len(c.Samples)-1]
	secs := last[0] - first[0]
	if secs <= 0 {
		return nil, nil
	}
	perMinute := (last[1] - first[1]) * 60 / secs
	if perMinute <= 0 {
		return new(int64(0)), nil
	}
	left := (counts.Total - done(counts)) * 60 / perMinute
	return &perMinute, &left
}

// flush stores the counts and, at most every two seconds unless forced, tells managers.
func (r *migrationRun) flush(ctx context.Context, force bool) error {
	if !force && time.Since(r.lastFlush) < migrationProgressEvery {
		return nil
	}
	r.lastFlush = time.Now()
	counts, _, err := r.s.refreshMigrationCounts(ctx, r.s.st.Queries, r.m)
	if err != nil {
		return err
	}
	cp := readCheckpoint(r.m.Checkpoint)
	now := r.s.now().Unix()
	cp.Samples = append(cp.Samples, [2]int64{now, done(counts)})
	for len(cp.Samples) > 2 && cp.Samples[1][0] <= now-int64(migrationRateWindow/time.Second) {
		cp.Samples = cp.Samples[1:]
	}
	r.m.Checkpoint = mustJSON(cp)
	r.m.Counts = mustJSON(counts)
	if err := r.s.st.SetMigrationCheckpoint(ctx, store.SetMigrationCheckpointParams{WorkspaceID: r.m.WorkspaceID, ID: r.m.ID, Checkpoint: r.m.Checkpoint}); err != nil {
		return err
	}
	r.s.migrationProgress(ctx, r.m)
	return nil
}

// migrationProgress sends the migration.progress signal from the counts stored on the migration.
func (s *Server) migrationProgress(ctx context.Context, m store.Migration) {
	folders, err := s.st.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
	if err != nil {
		s.log.WarnContext(ctx, "migration progress", slog.Any("error", err))
		return
	}
	counts := migrationCounts(m.Counts)
	out := oas.MigrationProgress{Id: m.ID, State: oas.MigrationState(m.State), Counts: counts, Folders: []oas.MigrationFolderProgress{}, WaitingUntil: m.WaitingUntil}
	out.RatePerMinute, out.SecondsLeft = migrationRate(readCheckpoint(m.Checkpoint), counts)
	for _, f := range folders {
		out.Folders = append(out.Folders, oas.MigrationFolderProgress{
			Id: f.ID, Imported: int64(f.Imported), Duplicate: int64(f.Duplicate), Skipped: int64(f.Skipped), Failed: int64(f.Failed), Bytes: f.Bytes,
		})
	}
	personal, err := migrationEventInbox(ctx, s.st.Queries, m)
	if err != nil {
		s.log.WarnContext(ctx, "migration progress", slog.Any("error", err))
		return
	}
	s.signal(ctx, realtime.Event{Type: eventMigrationProgress, WorkspaceID: m.WorkspaceID, InboxID: personal, Data: mustJSON(out)})
}

// RunMigrationWatchdog queues a job for every scanning, copying, syncing or removing migration
// whose lease expired and that has no job waiting to run: its process died, and River would retry
// it only after an hour.
func (s *Server) RunMigrationWatchdog(ctx context.Context) (int, error) {
	now := s.now()
	rows, err := s.st.ListStaleMigrations(ctx, &now)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, row := range rows {
		res, err := s.jobs.JobList(ctx, river.NewJobListParams().
			Kinds(migrationJobKinds...).
			States(rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateScheduled, rivertype.JobStateRetryable).
			Where("args->>'migration_id' = @id", river.NamedArgs{"id": row.ID.String()}).First(1))
		if err != nil {
			return queued, err
		}
		if len(res.Jobs) > 0 {
			continue
		}
		resume := int64(1)
		if row.LeaseJobID != nil {
			resume = *row.LeaseJobID
		}
		m := store.Migration{WorkspaceID: row.WorkspaceID, ID: row.ID, State: row.State}
		if _, err := s.jobs.Insert(ctx, migrationJob(m, resume), migrationJobOpts()); err != nil {
			return queued, err
		}
		s.log.InfoContext(ctx, "migration resumed by the watchdog", slog.String("migration_id", row.ID.String()), slog.String("state", row.State))
		queued++
	}
	return queued, nil
}

const migrationWorkKept = 30 * 24 * time.Hour

// CleanMigrations deletes the items, threads and errors of migrations that ended 30 days ago; their
// counts stay on the migration and its folders, and the channel's own duplicate lookup keeps a
// later migration of the same mailbox from importing anything twice.
func (s *Server) CleanMigrations(ctx context.Context) error {
	if err := s.dropEndedUploads(ctx); err != nil {
		return err
	}
	rows, err := s.st.ListFinishedMigrations(ctx, new(s.now().Add(-migrationWorkKept)))
	if err != nil {
		return err
	}
	for _, r := range rows {
		err := s.st.InTx(ctx, func(q *store.Queries) error {
			if err := q.DeleteMigrationItems(ctx, store.DeleteMigrationItemsParams{WorkspaceID: r.WorkspaceID, MigrationID: r.ID}); err != nil {
				return err
			}
			if err := q.DeleteMigrationThreads(ctx, store.DeleteMigrationThreadsParams{WorkspaceID: r.WorkspaceID, MigrationID: r.ID}); err != nil {
				return err
			}
			return q.DeleteMigrationErrors(ctx, store.DeleteMigrationErrorsParams{WorkspaceID: r.WorkspaceID, MigrationID: r.ID})
		})
		if err != nil {
			return err
		}
	}
	return nil
}

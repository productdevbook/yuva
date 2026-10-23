package api

import (
	"context"
	"math"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	reportBucket        = 15 * time.Minute
	reportDay           = 24 * time.Hour
	reportLateMargin    = 10 * time.Minute
	reportRecomputeDays = 31
	reportTailMax       = 24 * time.Hour
	reportRunBudget     = 20 * time.Second
	reportRollupTimeout = 5 * time.Minute
	reportBinGrowth     = 1.05
	reportMaxBin        = 400
)

var reportDurationMetrics = []string{"first_reply", "resolution"}

// reportBin is the histogram bin of a duration: 0 below a second, then bin b holds
// [1.05^(b-1), 1.05^b) seconds, so a bin's middle is within 2.5% of anything in it.
func reportBin(secs int64) int16 {
	if secs < 1 {
		return 0
	}
	b := int(math.Floor(math.Log(float64(secs))/math.Log(reportBinGrowth))) + 1
	return int16(min(b, reportMaxBin))
}

func reportBinSeconds(bin int16) float64 {
	if bin == 0 {
		return 0
	}
	return math.Pow(reportBinGrowth, float64(bin)-0.5)
}

type reportKey struct {
	bucket  time.Time
	inbox   uuid.UUID
	channel string
	team    uuid.UUID
	member  uuid.UUID
	label   uuid.UUID
	metric  string
	bin     int16
}

func optID(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.UUID{}
	}
	return *id
}

// reportCells adds facts up per quarter-hour bucket and dimension, once without a label and once
// per label of the conversation.
func reportCells(facts []store.ReportFactsRow, labels map[uuid.UUID][]uuid.UUID) map[reportKey]int32 {
	out := map[reportKey]int32{}
	for _, f := range facts {
		k := reportKey{
			bucket: f.At.UTC().Truncate(reportBucket), inbox: f.InboxID, team: optID(f.TeamID), member: optID(f.MemberID), metric: f.Metric,
		}
		if f.ChannelKind != nil {
			k.channel = *f.ChannelKind
		}
		for _, m := range reportDurationMetrics {
			if f.Metric == m {
				k.bin = reportBin(f.Secs)
			}
		}
		out[k]++
		for _, l := range labels[f.ConversationID] {
			lk := k
			lk.label = l
			out[lk]++
		}
	}
	return out
}

func (s *Server) reportFacts(ctx context.Context, q *store.Queries, ws uuid.UUID, since, until time.Time) (map[reportKey]int32, error) {
	facts, err := q.ReportFacts(ctx, store.ReportFactsParams{WorkspaceID: ws, Since: since, Until: until})
	if err != nil {
		return nil, err
	}
	seen := map[uuid.UUID]bool{}
	var ids []uuid.UUID
	for _, f := range facts {
		if !seen[f.ConversationID] {
			seen[f.ConversationID] = true
			ids = append(ids, f.ConversationID)
		}
	}
	labels := map[uuid.UUID][]uuid.UUID{}
	if len(ids) > 0 {
		rows, err := q.ListReportLabels(ctx, store.ListReportLabelsParams{WorkspaceID: ws, ConversationIds: ids})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			labels[r.ConversationID] = append(labels[r.ConversationID], r.LabelID)
		}
	}
	return reportCells(facts, labels), nil
}

func nilID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil() {
		return nil
	}
	return &id
}

func nilText(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// rebuildReports replaces the rollups of the buckets in [since, until); since is on a bucket.
func (s *Server) rebuildReports(ctx context.Context, tx pgx.Tx, q *store.Queries, ws uuid.UUID, since, until time.Time) error {
	cells, err := s.reportFacts(ctx, q, ws, since, until)
	if err != nil {
		return err
	}
	if err := q.DeleteReportRollups(ctx, store.DeleteReportRollupsParams{WorkspaceID: ws, Since: since, Until: until}); err != nil {
		return err
	}
	rows := make([][]any, 0, len(cells))
	for k, n := range cells {
		rows = append(rows, []any{ws, k.bucket, k.inbox, nilText(k.channel), nilID(k.team), nilID(k.member), nilID(k.label), k.metric, k.bin, n})
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"report_rollups"},
		[]string{"workspace_id", "bucket", "inbox_id", "channel_kind", "team_id", "member_id", "label_id", "metric", "bin", "n"}, pgx.CopyFromRows(rows))
	return err
}

// RollupReports brings a workspace's report rollups up to the last quarter-hour edge: on the first
// run from the first activity reports count, a UTC day per step; then today on every run, and once
// a UTC day the 31 days before (fewer when retention deletes conversations sooner) for late changes.
// built_until stays on a bucket edge, so a reader never sees a bucket that is half rolled up and
// half counted live. It says whether work is left after its time budget.
func (s *Server) RollupReports(ctx context.Context, ws uuid.UUID) (bool, error) {
	started := time.Now()
	w, err := s.st.GetWorkspace(ctx, ws)
	if store.IsNotFound(err) || (err == nil && w.DeletedAt != nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	if _, err := s.st.GetReportRollupState(ctx, ws); store.IsNotFound(err) {
		first, err := s.st.ReportEarliestActivity(ctx, store.ReportEarliestActivityParams{WorkspaceID: ws, Now: now})
		if err != nil {
			return false, err
		}
		if err := s.st.InitReportRollupState(ctx, store.InitReportRollupStateParams{
			WorkspaceID: ws, BuiltUntil: first.UTC().Truncate(reportDay), RecomputedOn: now.Truncate(reportDay), Now: now,
		}); err != nil {
			return false, err
		}
	} else if err != nil {
		return false, err
	}
	for {
		done, err := s.reportStep(ctx, ws, now, w.RetentionDays)
		if err != nil || done {
			return false, err
		}
		if time.Since(started) > reportRunBudget {
			return true, nil
		}
	}
}

// reportStep does one transaction of RollupReports: a past day still to build, a day of the
// nightly rebuild, or today so far, which ends the run.
func (s *Server) reportStep(ctx context.Context, ws uuid.UUID, now time.Time, retentionDays *int32) (bool, error) {
	done := false
	err := s.st.InTxRaw(ctx, func(tx pgx.Tx, q *store.Queries) error {
		st, err := q.LockReportRollupState(ctx, ws)
		if store.IsNotFound(err) {
			done = true
			return nil
		}
		if err != nil {
			return err
		}
		today := now.Truncate(reportDay)
		b := st.BuiltUntil
		if b.Before(today) {
			from, to := b.Add(-reportLateMargin).Truncate(reportBucket), b.Truncate(reportDay).Add(reportDay)
			if err := s.rebuildReports(ctx, tx, q, ws, from, to); err != nil {
				return err
			}
			return q.SetReportBuiltUntil(ctx, store.SetReportBuiltUntilParams{WorkspaceID: ws, BuiltUntil: to, Now: now})
		}
		if st.RecomputedOn == nil || st.RecomputedOn.Before(today) {
			at := today.Add(-reportRecomputeDays * reportDay)
			if retentionDays != nil {
				kept := now.Add(-time.Duration(*retentionDays) * reportDay).Truncate(reportDay).Add(reportDay)
				at = later(at, &kept)
			}
			at = later(at, st.RecomputeAt).UTC()
			if at.Before(today) {
				if err := s.rebuildReports(ctx, tx, q, ws, at, at.Add(reportDay)); err != nil {
					return err
				}
				next := at.Add(reportDay)
				return q.SetReportRecompute(ctx, store.SetReportRecomputeParams{WorkspaceID: ws, RecomputeAt: &next, RecomputedOn: st.RecomputedOn, Now: now})
			}
			if err := q.SetReportRecompute(ctx, store.SetReportRecomputeParams{WorkspaceID: ws, RecomputedOn: &today, Now: now}); err != nil {
				return err
			}
		}
		done = true
		from := b.Add(-reportLateMargin).Truncate(reportBucket)
		if from.After(today) {
			from = today
		}
		edge := now.Truncate(reportBucket)
		if err := s.rebuildReports(ctx, tx, q, ws, from, edge); err != nil {
			return err
		}
		return q.SetReportBuiltUntil(ctx, store.SetReportBuiltUntilParams{WorkspaceID: ws, BuiltUntil: edge, Now: now})
	})
	return done, err
}

// ReportRollupArgs brings one workspace's report rollups up to date.
type ReportRollupArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
}

func (ReportRollupArgs) Kind() string { return "report_rollup" }

func (ReportRollupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable,
	}}}
}

type reportRollupWorker struct {
	river.WorkerDefaults[ReportRollupArgs]
	s *Server
}

func (w *reportRollupWorker) Timeout(*river.Job[ReportRollupArgs]) time.Duration {
	return reportRollupTimeout
}

func (w *reportRollupWorker) Work(ctx context.Context, j *river.Job[ReportRollupArgs]) error {
	more, err := w.s.RollupReports(ctx, j.Args.WorkspaceID)
	if err != nil {
		return err
	}
	if more {
		return river.JobSnooze(0)
	}
	return nil
}

type reportRollupScanWorker struct {
	river.WorkerDefaults[jobs.ReportRollupScanArgs]
	s *Server
}

func (w *reportRollupScanWorker) Work(ctx context.Context, _ *river.Job[jobs.ReportRollupScanArgs]) error {
	return w.s.ScanReportRollups(ctx)
}

// ScanReportRollups queues a rollup for every workspace.
func (s *Server) ScanReportRollups(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	batch := make([]river.InsertManyParams, 0, len(ids))
	for _, ws := range ids {
		batch = append(batch, river.InsertManyParams{Args: ReportRollupArgs{WorkspaceID: ws}})
	}
	if len(batch) == 0 {
		return nil
	}
	_, err = s.jobs.InsertMany(ctx, batch)
	return err
}

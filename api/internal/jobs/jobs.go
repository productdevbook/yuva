package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
	"github.com/riverqueue/rivercontrib/otelriver"

	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/telemetry"
)

func Migrate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	m, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Logger: log})
	if err != nil {
		return err
	}
	res, err := m.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("migrate river: %w", err)
	}
	for _, v := range res.Versions {
		log.Info("river migration applied", slog.Int("version", v.Version))
	}
	return nil
}

const (
	EventRetention              = 7 * 24 * time.Hour
	WebhookRetention            = 7 * 24 * time.Hour
	IdempotencyRetention        = 24 * time.Hour
	EmailConfirmationRetention  = 30 * 24 * time.Hour
	UnusedOAuthClientAge        = 30 * 24 * time.Hour
	PresenceSweepInterval       = time.Minute
	OperatorDigestInterval      = time.Minute
	BroadcastSweepInterval      = 5 * time.Minute
	SESTenantStatusInterval     = 5 * time.Minute
	StatusMonitorSweepInterval  = 30 * time.Second
	StatusChecksCleanupInterval = time.Hour
	StatusNoticeSweepInterval   = time.Minute
	StatusDomainSweepInterval   = time.Minute
	ChangelogSweepInterval      = time.Minute
)

// StatusNoticeQueue holds the status notice jobs, so a notice about an outage never waits behind a
// newsletter on the broadcast queue.
const StatusNoticeQueue = "status_notice"

// StatusCheckQueue holds the http and tcp checks of status monitors and the judging of their rounds,
// at most StatusCheckWorkers at a time per process.
const (
	StatusCheckQueue   = "status_checks"
	StatusCheckWorkers = 20
)

// BroadcastQueue holds the broadcast sending jobs, so a large send never delays replies,
// notifications or webhooks on the default queue.
const BroadcastQueue = "broadcast"

// MigrationQueue holds the jobs that move mailboxes into Yuva, for the same reason.
const MigrationQueue = "migrations"

const MigrationWatchdogInterval = time.Minute

// AnalyticsQueue holds the analytics maintenance and rollup jobs, so they never delay e-mail,
// webhooks or migrations.
const AnalyticsQueue = "analytics"

const (
	AnalyticsPartitionsInterval = time.Hour
	AnalyticsRollupScanInterval = 5 * time.Minute
	AnalyticsSaltsInterval      = 15 * time.Minute
	AnalyticsCohortsInterval    = time.Hour
	AnalyticsGeoIPInterval      = 24 * time.Hour
	AnalyticsAlertsInterval     = time.Hour
	AnalyticsReportsInterval    = time.Hour
)

// New starts no migration workers when migrationWorkers is 0, and no analytics workers or analytics
// periodic jobs when analyticsWorkers is 0. sesTenants adds the periodic Amazon SES tenant status check.
func New(pool *pgxpool.Pool, q *store.Queries, log *slog.Logger, broadcastWorkers, migrationWorkers, analyticsWorkers int, sesTenants bool, register ...func(*river.Workers)) (*river.Client[pgx.Tx], error) {
	if broadcastWorkers < 1 {
		broadcastWorkers = 2
	}
	queues := map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 10}, BroadcastQueue: {MaxWorkers: broadcastWorkers}, StatusNoticeQueue: {MaxWorkers: 2},
		StatusCheckQueue: {MaxWorkers: StatusCheckWorkers}}
	if migrationWorkers > 0 {
		queues[MigrationQueue] = river.QueueConfig{MaxWorkers: migrationWorkers}
	}
	if analyticsWorkers > 0 {
		queues[AnalyticsQueue] = river.QueueConfig{MaxWorkers: analyticsWorkers}
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &EventCleanupWorker{Queries: q, Now: time.Now})
	for _, r := range register {
		r(workers)
	}
	periodic := []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(time.Hour), func() (river.JobArgs, *river.InsertOpts) {
			return EventCleanupArgs{}, nil
		}, &river.PeriodicJobOpts{ID: "event_cleanup", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(time.Hour), func() (river.JobArgs, *river.InsertOpts) {
			return RetentionArgs{}, nil
		}, &river.PeriodicJobOpts{ID: "retention", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(PresenceSweepInterval), func() (river.JobArgs, *river.InsertOpts) {
			return PresenceSweepArgs{}, nil
		}, &river.PeriodicJobOpts{ID: "presence_sweep"}),
		river.NewPeriodicJob(river.PeriodicInterval(OperatorDigestInterval), func() (river.JobArgs, *river.InsertOpts) {
			return OperatorDigestArgs{}, nil
		}, &river.PeriodicJobOpts{ID: "operator_digest", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(BroadcastSweepInterval), func() (river.JobArgs, *river.InsertOpts) {
			return BroadcastSweepArgs{}, nil
		}, &river.PeriodicJobOpts{ID: "broadcast_sweep", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(MigrationWatchdogInterval), func() (river.JobArgs, *river.InsertOpts) {
			return MigrationWatchdogArgs{}, nil
		}, &river.PeriodicJobOpts{ID: "migration_watchdog", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(StatusMonitorSweepInterval), func() (river.JobArgs, *river.InsertOpts) {
			return StatusMonitorSweepArgs{}, nil
		}, &river.PeriodicJobOpts{ID: "status_monitor_sweep", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(StatusChecksCleanupInterval), func() (river.JobArgs, *river.InsertOpts) {
			return StatusChecksCleanupArgs{}, nil
		}, &river.PeriodicJobOpts{ID: "status_checks_cleanup", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(StatusNoticeSweepInterval), func() (river.JobArgs, *river.InsertOpts) {
			return StatusNoticeSweepArgs{}, nil
		}, &river.PeriodicJobOpts{ID: "status_notice_sweep", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(StatusDomainSweepInterval), func() (river.JobArgs, *river.InsertOpts) {
			return StatusDomainSweepArgs{}, nil
		}, &river.PeriodicJobOpts{ID: "status_domain_sweep", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(ChangelogSweepInterval), func() (river.JobArgs, *river.InsertOpts) {
			return ChangelogSweepArgs{}, nil
		}, &river.PeriodicJobOpts{ID: "changelog_sweep", RunOnStart: true}),
	}
	if sesTenants {
		periodic = append(periodic, river.NewPeriodicJob(river.PeriodicInterval(SESTenantStatusInterval), func() (river.JobArgs, *river.InsertOpts) {
			return SESTenantStatusArgs{}, nil
		}, &river.PeriodicJobOpts{ID: "ses_tenant_status", RunOnStart: true}))
	}
	if analyticsWorkers > 0 {
		onAnalytics := &river.InsertOpts{Queue: AnalyticsQueue}
		periodic = append(periodic,
			river.NewPeriodicJob(river.PeriodicInterval(AnalyticsPartitionsInterval), func() (river.JobArgs, *river.InsertOpts) {
				return AnalyticsPartitionsArgs{}, onAnalytics
			}, &river.PeriodicJobOpts{ID: "analytics_partitions", RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(AnalyticsRollupScanInterval), func() (river.JobArgs, *river.InsertOpts) {
				return AnalyticsRollupScanArgs{}, onAnalytics
			}, &river.PeriodicJobOpts{ID: "analytics_rollup_scan", RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(AnalyticsSaltsInterval), func() (river.JobArgs, *river.InsertOpts) {
				return AnalyticsSaltsArgs{}, onAnalytics
			}, &river.PeriodicJobOpts{ID: "analytics_salts", RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(AnalyticsCohortsInterval), func() (river.JobArgs, *river.InsertOpts) {
				return AnalyticsCohortsArgs{}, onAnalytics
			}, &river.PeriodicJobOpts{ID: "analytics_cohorts", RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(AnalyticsGeoIPInterval), func() (river.JobArgs, *river.InsertOpts) {
				return AnalyticsGeoIPArgs{}, onAnalytics
			}, &river.PeriodicJobOpts{ID: "analytics_geoip"}),
			river.NewPeriodicJob(river.PeriodicInterval(AnalyticsAlertsInterval), func() (river.JobArgs, *river.InsertOpts) {
				return AnalyticsAlertsArgs{}, onAnalytics
			}, &river.PeriodicJobOpts{ID: "analytics_alerts"}),
			river.NewPeriodicJob(river.PeriodicInterval(AnalyticsReportsInterval), func() (river.JobArgs, *river.InsertOpts) {
				return AnalyticsReportsArgs{}, onAnalytics
			}, &river.PeriodicJobOpts{ID: "analytics_reports", RunOnStart: true}),
		)
	}
	var middleware []rivertype.Middleware
	if telemetry.Enabled() {
		middleware = append(middleware, otelriver.NewMiddleware(&otelriver.MiddlewareConfig{
			EnableTracePropagation:      true,
			EnableWorkSpanJobKindSuffix: true,
		}))
	}
	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger:       log,
		Middleware:   middleware,
		Queues:       queues,
		Workers:      workers,
		PeriodicJobs: periodic,
	})
}

// AnalyticsPartitionsArgs is worked by the api package: raw partitions ahead and past the raw
// window, rollups past their property's retention, old ingest counts.
type AnalyticsPartitionsArgs struct{}

func (AnalyticsPartitionsArgs) Kind() string { return "analytics_partitions" }

// AnalyticsRollupScanArgs is worked by the api package: it queues a rollup for every finished or
// dirty day.
type AnalyticsRollupScanArgs struct{}

func (AnalyticsRollupScanArgs) Kind() string { return "analytics_rollup_scan" }

// AnalyticsSaltsArgs is worked by the api package: it deletes the salts past their grace and the
// visit claims past their window.
type AnalyticsSaltsArgs struct{}

func (AnalyticsSaltsArgs) Kind() string { return "analytics_salts" }

// AnalyticsCohortsArgs is worked by the api package: it forgets persistent ids first seen more than
// 13 months ago and rebuilds the retention cohorts.
type AnalyticsCohortsArgs struct{}

func (AnalyticsCohortsArgs) Kind() string { return "analytics_cohorts" }

// AnalyticsGeoIPArgs is worked by the api package: it tells every process to fetch the month's
// GeoIP database if it has not got it yet.
type AnalyticsGeoIPArgs struct{}

func (AnalyticsGeoIPArgs) Kind() string { return "analytics_geoip" }

// RetentionArgs is worked by the api package, which owns object storage.
type RetentionArgs struct{}

func (RetentionArgs) Kind() string { return "retention" }

// PresenceSweepArgs is worked by the api package, which owns the realtime signals.
type PresenceSweepArgs struct{}

func (PresenceSweepArgs) Kind() string { return "presence_sweep" }

// OperatorDigestArgs is worked by the api package, which owns the mail templates.
type OperatorDigestArgs struct{}

func (OperatorDigestArgs) Kind() string { return "operator_digest" }

// BroadcastSweepArgs is worked by the api package, which owns sending.
type BroadcastSweepArgs struct{}

func (BroadcastSweepArgs) Kind() string { return "broadcast_sweep" }

// SESTenantStatusArgs is worked by the api package: it reads the sending status of the Amazon SES
// tenants it manages, of one inbox or, without ids, of all.
type SESTenantStatusArgs struct {
	WorkspaceID *uuid.UUID `json:"workspace_id,omitempty"`
	InboxID     *uuid.UUID `json:"inbox_id,omitempty"`
}

func (SESTenantStatusArgs) Kind() string { return "ses_tenant_status" }

func (SESTenantStatusArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable,
	}}}
}

type StatusNoticeSweepArgs struct{}

func (StatusNoticeSweepArgs) Kind() string { return "status_notice_sweep" }

// StatusMonitorSweepArgs is worked by the api package: it fails heartbeats whose beat is overdue and
// queues the http and tcp checks that are due.
type StatusMonitorSweepArgs struct{}

func (StatusMonitorSweepArgs) Kind() string { return "status_monitor_sweep" }

func (StatusMonitorSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable,
	}}}
}

// StatusDomainSweepArgs is worked by the api package: it queues the checks of status page domains
// that are due.
type StatusDomainSweepArgs struct{}

func (StatusDomainSweepArgs) Kind() string { return "status_domain_sweep" }

func (StatusDomainSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable,
	}}}
}

// ChangelogSweepArgs is worked by the api package: it publishes the What's new entries whose
// schedule is due and has no job left.
type ChangelogSweepArgs struct{}

func (ChangelogSweepArgs) Kind() string { return "changelog_sweep" }

func (ChangelogSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable,
	}}}
}

// StatusChecksCleanupArgs is worked by the api package: it deletes monitor checks after 30 days.
type StatusChecksCleanupArgs struct{}

func (StatusChecksCleanupArgs) Kind() string { return "status_checks_cleanup" }

// MigrationWatchdogArgs is worked by the api package, which owns migrations.
type MigrationWatchdogArgs struct{}

func (MigrationWatchdogArgs) Kind() string { return "migration_watchdog" }

type EventCleanupArgs struct{}

func (EventCleanupArgs) Kind() string { return "event_cleanup" }

type EventCleanupWorker struct {
	river.WorkerDefaults[EventCleanupArgs]
	Queries *store.Queries
	Now     func() time.Time
}

func (w *EventCleanupWorker) Work(ctx context.Context, _ *river.Job[EventCleanupArgs]) error {
	ids, err := w.Queries.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	before := w.Now().Add(-EventRetention)
	for _, id := range ids {
		if _, err := w.Queries.DeleteEventsBefore(ctx, store.DeleteEventsBeforeParams{WorkspaceID: id, Before: before}); err != nil {
			return fmt.Errorf("workspace %s: %w", id, err)
		}
		if _, err := w.Queries.DeleteStaleConnections(ctx, store.DeleteStaleConnectionsParams{WorkspaceID: id, Before: w.Now().Add(-time.Hour)}); err != nil {
			return fmt.Errorf("workspace %s: %w", id, err)
		}
		if _, err := w.Queries.DeleteExpiredContactSessions(ctx, store.DeleteExpiredContactSessionsParams{WorkspaceID: id, Before: w.Now()}); err != nil {
			return fmt.Errorf("workspace %s: %w", id, err)
		}
		if _, err := w.Queries.DeleteFinishedWebhookDeliveries(ctx, store.DeleteFinishedWebhookDeliveriesParams{WorkspaceID: id, Before: w.Now().Add(-WebhookRetention)}); err != nil {
			return fmt.Errorf("workspace %s: %w", id, err)
		}
		if _, err := w.Queries.DeleteIdempotencyKeysBefore(ctx, store.DeleteIdempotencyKeysBeforeParams{WorkspaceID: id, Before: w.Now().Add(-IdempotencyRetention)}); err != nil {
			return fmt.Errorf("workspace %s: %w", id, err)
		}
		if _, err := w.Queries.DeleteExpiredOAuthTokens(ctx, store.DeleteExpiredOAuthTokensParams{WorkspaceID: id, Before: w.Now()}); err != nil {
			return fmt.Errorf("workspace %s: %w", id, err)
		}
		if _, err := w.Queries.DeleteExpiredOAuthCodes(ctx, store.DeleteExpiredOAuthCodesParams{WorkspaceID: id, Before: w.Now()}); err != nil {
			return fmt.Errorf("workspace %s: %w", id, err)
		}
		if _, err := w.Queries.DeleteOldEmailConfirmations(ctx, store.DeleteOldEmailConfirmationsParams{WorkspaceID: id, Before: w.Now().Add(-EmailConfirmationRetention)}); err != nil {
			return fmt.Errorf("workspace %s: %w", id, err)
		}
		if _, err := w.Queries.DeleteExpiredConfirmationBounces(ctx, store.DeleteExpiredConfirmationBouncesParams{WorkspaceID: id, Before: w.Now().Add(-EmailConfirmationRetention)}); err != nil {
			return fmt.Errorf("workspace %s: %w", id, err)
		}
	}
	if _, err := w.Queries.DeleteExpiredOAuthRequests(ctx, w.Now()); err != nil {
		return err
	}
	_, err = w.Queries.DeleteUnusedOAuthClients(ctx, w.Now().Add(-UnusedOAuthClientAge))
	return err
}

// AnalyticsAlertsArgs is worked by the api package: it checks the alert rules of every property
// that has alert subscribers.
type AnalyticsAlertsArgs struct{}

func (AnalyticsAlertsArgs) Kind() string { return "analytics_alerts" }

// AnalyticsReportsArgs is worked by the api package: it mails the weekly and monthly reports that
// are due.
type AnalyticsReportsArgs struct{}

func (AnalyticsReportsArgs) Kind() string { return "analytics_reports" }

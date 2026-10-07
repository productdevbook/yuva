package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/productdevbook/yuva/api/internal/store"
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

const EventRetention = 24 * time.Hour

func New(pool *pgxpool.Pool, q *store.Queries, log *slog.Logger, register ...func(*river.Workers)) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &EventCleanupWorker{Queries: q, Now: time.Now})
	for _, r := range register {
		r(workers)
	}
	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger:  log,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 10}},
		Workers: workers,
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(time.Hour), func() (river.JobArgs, *river.InsertOpts) {
				return EventCleanupArgs{}, nil
			}, &river.PeriodicJobOpts{ID: "event_cleanup", RunOnStart: true}),
		},
	})
}

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
	}
	return nil
}

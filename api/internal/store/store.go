package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/productdevbook/yuva/api/internal/telemetry"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	Pool *pgxpool.Pool
	*Queries
}

func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConnLifetime = time.Hour
	if telemetry.Enabled() {
		cfg.ConnConfig.Tracer = otelpgx.NewTracer(
			otelpgx.WithDisableSQLStatementInAttributes(),
			otelpgx.WithDisableConnectionDetailsInAttributes(),
			otelpgx.WithSpanNameFunc(spanName),
		)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return &Store{Pool: pool, Queries: New(pool)}, nil
}

func (s *Store) Close() { s.Pool.Close() }

func (s *Store) WaitReady(ctx context.Context) error {
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := s.Pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("database not reachable: %w", err)
		case <-time.After(time.Second):
		}
	}
}

func (s *Store) InTx(ctx context.Context, fn func(q *Queries) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		return fn(s.WithTx(tx))
	})
}

// Savepoint runs fn in a savepoint when q runs in a transaction, so a statement that fails there
// leaves the transaction usable; outside one it runs fn on q.
func (q *Queries) Savepoint(ctx context.Context, fn func(q *Queries) error) error {
	tx, ok := q.db.(pgx.Tx)
	if !ok {
		return fn(q)
	}
	return pgx.BeginFunc(ctx, tx, func(sp pgx.Tx) error { return fn(q.WithTx(sp)) })
}

func (s *Store) InTxRaw(ctx context.Context, fn func(tx pgx.Tx, q *Queries) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		return fn(tx, s.WithTx(tx))
	})
}

func (s *Store) migrations(log *slog.Logger) (*goose.Provider, error) {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, stdlib.OpenDBFromPool(s.Pool), sub,
		goose.WithSessionLocker(locker),
		goose.WithSlog(log),
	)
}

func (s *Store) MigrateUp(ctx context.Context, log *slog.Logger) error {
	p, err := s.migrations(log)
	if err != nil {
		return err
	}
	defer p.Close()
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func (s *Store) MigrateDown(ctx context.Context, log *slog.Logger) error {
	p, err := s.migrations(log)
	if err != nil {
		return err
	}
	defer p.Close()
	if _, err := p.Down(ctx); err != nil {
		return fmt.Errorf("migrate down: %w", err)
	}
	return nil
}

func (s *Store) MigrationStatus(ctx context.Context, log *slog.Logger) ([]*goose.MigrationStatus, error) {
	p, err := s.migrations(log)
	if err != nil {
		return nil, err
	}
	defer p.Close()
	return p.Status(ctx)
}

var ErrNoRows = pgx.ErrNoRows

func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func UniqueViolationConstraint(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName
	}
	return ""
}

// IsQueryCanceled reports a statement stopped by statement_timeout or a cancel request.
func IsQueryCanceled(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "57014"
}

func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// spanName is the sqlc query name from the leading "-- name:" comment, else the statement's
// first word; the statement text and its arguments never reach a span.
func spanName(stmt string) string {
	if rest, ok := strings.CutPrefix(stmt, "-- name: "); ok {
		if name, _, _ := strings.Cut(rest, " "); name != "" {
			return name
		}
	}
	if f := strings.Fields(stmt); len(f) > 0 && !strings.HasPrefix(f[0], "--") {
		return strings.ToUpper(f[0])
	}
	return "QUERY"
}

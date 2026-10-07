package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
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

func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

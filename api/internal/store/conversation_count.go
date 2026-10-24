package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var errStatementCaptured = errors.New("statement captured")

type statementCapture struct {
	sql  string
	args []any
}

func (c *statementCapture) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errStatementCaptured
}

func (c *statementCapture) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	c.sql, c.args = sql, args
	return nil, errStatementCaptured
}

func (c *statementCapture) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

// CountListConversations counts the rows ListConversations returns for arg (so at most arg.Lim).
// It wraps that query's own statement and arguments in count(*), so the count can never filter
// differently from the list.
func (q *Queries) CountListConversations(ctx context.Context, arg ListConversationsParams) (int64, error) {
	c := &statementCapture{}
	if _, err := New(c).ListConversations(ctx, arg); !errors.Is(err, errStatementCaptured) {
		return 0, errors.New("ListConversations did not run its statement")
	}
	var n int64
	err := q.db.QueryRow(ctx, "SELECT count(*) FROM (\n"+c.sql+"\n) listed", c.args...).Scan(&n)
	return n, err
}

// CountListConversationsWithin is CountListConversations in a read-only transaction whose statements
// stop after timeout; timedOut reports a count the database gave up on. prepare, when set, completes
// arg inside that transaction, so its queries stop at the timeout too.
func (s *Store) CountListConversationsWithin(ctx context.Context, arg ListConversationsParams, timeout time.Duration, prepare func(context.Context, *Queries, *ListConversationsParams) error) (n int64, timedOut bool, err error) {
	err = pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('statement_timeout', $1, true)", strconv.FormatInt(timeout.Milliseconds(), 10)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('jit', 'off', true)"); err != nil {
			return err
		}
		if prepare != nil {
			if err := prepare(ctx, New(tx), &arg); err != nil {
				return err
			}
		}
		var err error
		n, err = New(tx).CountListConversations(ctx, arg)
		return err
	})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "57014" {
		return 0, true, nil
	}
	return n, false, err
}

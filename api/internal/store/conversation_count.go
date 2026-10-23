package store

import (
	"context"
	"errors"

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

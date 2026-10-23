package store

import (
	"context"
	"uuid"

	"github.com/jackc/pgx/v5"
)

// SealedColumn names a column that holds a value sealed under the master key, in a table with one
// such value per row id within a workspace. WorkspaceColumn and IDColumn may be the same column.
type SealedColumn struct {
	Table, Column, WorkspaceColumn, IDColumn string
}

type SealedValue struct {
	ID     uuid.UUID
	Sealed []byte
}

func (c SealedColumn) ident() (table, col, ws, id string) {
	return pgx.Identifier{c.Table}.Sanitize(), pgx.Identifier{c.Column}.Sanitize(),
		pgx.Identifier{c.WorkspaceColumn}.Sanitize(), pgx.Identifier{c.IDColumn}.Sanitize()
}

// SealedPage lists one workspace's values of the column after the row id after, in id order.
func (q *Queries) SealedPage(ctx context.Context, c SealedColumn, workspaceID, after uuid.UUID, limit int) ([]SealedValue, error) {
	table, col, ws, id := c.ident()
	rows, err := q.db.Query(ctx, "SELECT "+id+", "+col+" FROM "+table+" WHERE "+ws+" = $1 AND "+col+" IS NOT NULL AND "+id+" > $2 ORDER BY "+id+" LIMIT $3",
		workspaceID, after, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (SealedValue, error) {
		var v SealedValue
		err := r.Scan(&v.ID, &v.Sealed)
		return v, err
	})
}

// ReplaceSealed writes next in place of old, and reports false when the value is no longer old.
func (q *Queries) ReplaceSealed(ctx context.Context, c SealedColumn, workspaceID, rowID uuid.UUID, old, next []byte) (bool, error) {
	table, col, ws, id := c.ident()
	tag, err := q.db.Exec(ctx, "UPDATE "+table+" SET "+col+" = $4 WHERE "+ws+" = $1 AND "+id+" = $2 AND "+col+" = $3",
		workspaceID, rowID, old, next)
	return tag.RowsAffected() == 1, err
}

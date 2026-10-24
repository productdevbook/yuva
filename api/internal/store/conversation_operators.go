package store

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
)

func (p ListConversationsParams) HasOperators() bool {
	if p.OpImpossible || p.OpHasHits || p.OpAfter != nil || p.OpBefore != nil || p.OpNotAfter != nil || p.OpNotBefore != nil ||
		len(p.OpMissIds)+len(p.OpFromVals)+len(p.OpToVals)+len(p.OpCcVals)+len(p.OpSubjectVals) > 0 {
		return true
	}
	return slices.ContainsFunc([]*bool{p.OpFromMe, p.OpToMe, p.OpHasAttachment, p.OpIsStarred, p.OpInStarred, p.OpInInbox, p.OpInSnoozed, p.OpInArchive, p.OpInTrash, p.OpInSent, p.OpInDrafts, p.OpInScheduled},
		func(b *bool) bool { return b != nil })
}

// JIT is off because the planner prices the operator subqueries high enough to compile them, which costs more than the query.
func (s *Store) ListConversationsPlanned(ctx context.Context, arg ListConversationsParams) ([]ListConversationsRow, error) {
	var rows []ListConversationsRow
	err := pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('jit', 'off', true)"); err != nil {
			return err
		}
		var err error
		rows, err = New(tx).ListConversations(ctx, arg)
		return err
	})
	return rows, err
}

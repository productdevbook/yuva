package store

import (
	"context"
	"uuid"
)

// River's job table is not part of the sqlc schema.
const deleteBroadcastStartJobs = `DELETE FROM river_job
WHERE kind = 'broadcast_start' AND state = ANY('{available,pending,retryable,scheduled}'::river_job_state[])
  AND args->>'workspace_id' = $1 AND args->>'broadcast_id' = $2`

func (q *Queries) DeleteBroadcastStartJobs(ctx context.Context, workspaceID, broadcastID uuid.UUID) error {
	_, err := q.db.Exec(ctx, deleteBroadcastStartJobs, workspaceID.String(), broadcastID.String())
	return err
}

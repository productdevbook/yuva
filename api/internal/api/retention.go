package api

import (
	"context"
	"fmt"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const retentionBatch = 200

func (s *Server) UpdateWorkspace(ctx context.Context, req oas.UpdateWorkspaceRequestObject) (oas.UpdateWorkspaceResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireOwner(p); err != nil {
		return nil, err
	}
	b := req.Body
	if !b.RetentionDays.IsSpecified() {
		return nil, errValidation("retention_days is required; null keeps everything")
	}
	var days *int32
	if !b.RetentionDays.IsNull() {
		v := b.RetentionDays.MustGet()
		if v < 1 || v > 36500 {
			return nil, errValidation("retention_days must be 1 to 36500")
		}
		days = &v
	}
	w, err := s.st.SetWorkspaceRetention(ctx, store.SetWorkspaceRetentionParams{ID: p.workspaceID, RetentionDays: days})
	if err != nil {
		return nil, err
	}
	return oas.UpdateWorkspace200JSONResponse(workspaceBody(w)), nil
}

type retentionWorker struct {
	river.WorkerDefaults[jobs.RetentionArgs]
	s *Server
}

func (w *retentionWorker) Work(ctx context.Context, _ *river.Job[jobs.RetentionArgs]) error {
	return w.s.ApplyRetention(ctx)
}

// ApplyRetention deletes, in every workspace with a retention period, the closed conversations
// untouched for that long and the raw e-mails older than it.
func (s *Server) ApplyRetention(ctx context.Context) error {
	rows, err := s.st.ListWorkspaceRetention(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		before := s.now().AddDate(0, 0, -int(r.RetentionDays))
		if err := s.expireConversations(ctx, r.ID, before); err != nil {
			return fmt.Errorf("workspace %s: %w", r.ID, err)
		}
		if err := s.expireRawEmails(ctx, r.ID, before); err != nil {
			return fmt.Errorf("workspace %s: %w", r.ID, err)
		}
	}
	return nil
}

func (s *Server) expireConversations(ctx context.Context, workspaceID uuid.UUID, before time.Time) error {
	for {
		var keys []string
		var n int
		err := s.st.InTx(ctx, func(q *store.Queries) error {
			ids, err := q.ListExpiredConversationIDs(ctx, store.ListExpiredConversationIDsParams{
				WorkspaceID: workspaceID, Before: before, MaxRows: retentionBatch,
			})
			if err != nil || len(ids) == 0 {
				return err
			}
			n = len(ids)
			if keys, err = q.ListConversationsStorageKeys(ctx, store.ListConversationsStorageKeysParams{WorkspaceID: workspaceID, Ids: ids}); err != nil {
				return err
			}
			raw, err := q.ListConversationsRawKeys(ctx, store.ListConversationsRawKeysParams{WorkspaceID: workspaceID, Ids: ids})
			if err != nil {
				return err
			}
			keys = append(keys, raw...)
			_, err = q.DeleteConversations(ctx, store.DeleteConversationsParams{WorkspaceID: workspaceID, Ids: ids})
			return err
		})
		if err != nil {
			return err
		}
		s.deleteObjects(ctx, keys)
		if n < retentionBatch {
			return nil
		}
	}
}

func (s *Server) expireRawEmails(ctx context.Context, workspaceID uuid.UUID, before time.Time) error {
	for {
		keys, err := s.st.ListExpiredRawKeys(ctx, store.ListExpiredRawKeysParams{WorkspaceID: workspaceID, Before: before, MaxRows: retentionBatch})
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		if _, err := s.st.ClearRawKeys(ctx, store.ClearRawKeysParams{WorkspaceID: workspaceID, Keys: keys}); err != nil {
			return err
		}
		s.deleteObjects(ctx, keys)
		if len(keys) < retentionBatch {
			return nil
		}
	}
}

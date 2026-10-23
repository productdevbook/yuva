package api

import (
	"context"
	"fmt"
	"log/slog"
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
	b := req.Body
	if !b.RetentionDays.IsSpecified() && b.BotsMaySend == nil && b.BroadcastsByKeys == nil {
		return nil, errValidation("send retention_days, bots_may_send or broadcasts_by_keys")
	}
	if b.RetentionDays.IsSpecified() && !p.isKey() {
		if err := requireOwner(p); err != nil {
			return nil, err
		}
	}
	if b.BotsMaySend != nil || b.BroadcastsByKeys != nil {
		if p.isGrant() {
			return nil, errMemberSessionRequired
		}
		if err := requireManager(p); err != nil {
			return nil, err
		}
	}
	var days *int32
	if b.RetentionDays.IsSpecified() && !b.RetentionDays.IsNull() {
		v := b.RetentionDays.MustGet()
		if v < 1 || v > 36500 {
			return nil, errValidation("retention_days must be 1 to 36500")
		}
		days = &v
	}
	var w store.Workspace
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.LockWorkspace(ctx, p.workspaceID); err != nil {
			return err
		}
		var err error
		if w, err = q.GetWorkspace(ctx, p.workspaceID); err != nil {
			return err
		}
		if b.RetentionDays.IsSpecified() {
			from := w.RetentionDays
			if w, err = q.SetWorkspaceRetention(ctx, store.SetWorkspaceRetentionParams{ID: p.workspaceID, RetentionDays: days}); err != nil {
				return err
			}
			if !equalPtr(from, w.RetentionDays) {
				if err := s.audit(ctx, q, p, oas.AuditWorkspaceRetentionChanged, oas.AuditTargetWorkspace, p.workspaceID, auditDetails{"from": from, "to": w.RetentionDays}); err != nil {
					return err
				}
			}
		}
		if b.BotsMaySend != nil {
			if w, err = q.SetWorkspaceBotsMaySend(ctx, store.SetWorkspaceBotsMaySendParams{ID: p.workspaceID, BotsMaySend: *b.BotsMaySend}); err != nil {
				return err
			}
		}
		if b.BroadcastsByKeys != nil {
			w, err = q.SetWorkspaceBroadcastsByKeys(ctx, store.SetWorkspaceBroadcastsByKeysParams{ID: p.workspaceID, ByKeys: *b.BroadcastsByKeys})
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	body, err := s.workspaceBody(ctx, w)
	if err != nil {
		return nil, err
	}
	return oas.UpdateWorkspace200JSONResponse(body), nil
}

type retentionWorker struct {
	river.WorkerDefaults[jobs.RetentionArgs]
	s *Server
}

func (w *retentionWorker) Work(ctx context.Context, _ *river.Job[jobs.RetentionArgs]) error {
	if err := w.s.queueWorkspaceDeletions(ctx); err != nil {
		return err
	}
	now := w.s.now()
	if err := w.s.st.DeleteExpiredReaderTokens(ctx, now); err != nil {
		return err
	}
	if err := w.s.st.DeleteOldReaderNotifications(ctx, now.Add(-readerNotificationsKept)); err != nil {
		return err
	}
	if err := w.s.ExpireSubscriptions(ctx); err != nil {
		return err
	}
	if err := w.s.CleanUnusedImages(ctx); err != nil {
		return err
	}
	if err := w.s.CleanBroadcasts(ctx); err != nil {
		return err
	}
	if err := w.s.CleanHelp(ctx); err != nil {
		return err
	}
	if err := w.s.CleanRoadmap(ctx); err != nil {
		return err
	}
	if err := w.s.CleanChangelog(ctx); err != nil {
		return err
	}
	if err := w.s.CleanStatusNotices(ctx); err != nil {
		return err
	}
	if err := w.s.CleanListImports(ctx); err != nil {
		return err
	}
	if err := w.s.CleanMigrations(ctx); err != nil {
		return err
	}
	if err := w.s.CleanMigrationAudit(ctx); err != nil {
		return err
	}
	if err := w.s.CleanAudit(ctx); err != nil {
		w.s.log.ErrorContext(ctx, "audit log retention", slog.Any("error", err))
	}
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
		n, _, err := s.deleteConversationBatch(ctx, workspaceID, func(q *store.Queries) ([]uuid.UUID, error) {
			return q.ListExpiredConversationIDs(ctx, store.ListExpiredConversationIDsParams{
				WorkspaceID: workspaceID, Before: before, MaxRows: retentionBatch,
			})
		})
		if err != nil {
			return err
		}
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

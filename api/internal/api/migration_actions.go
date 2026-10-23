package api

import (
	"context"
	"strconv"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

var errMigrationConnectFirst = errMigrationState("connect the mailbox first: set its credentials and test the connection")

// migrationStep runs one lifecycle action on a locked migration and answers with the migration
// as it is afterwards.
func (s *Server) migrationStep(ctx context.Context, id uuid.UUID, fn func(q *store.Queries, events *eventBatch, p principal, cur store.Migration) (store.Migration, error)) (oas.Migration, error) {
	p := principalFrom(ctx)
	var out oas.Migration
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := visibleMigration(ctx, q, p, id, true)
		if err != nil {
			return err
		}
		m, err := fn(q, events, p, cur)
		if err != nil {
			return err
		}
		if err := s.migrationChanged(ctx, q, events, m, &cur.State); err != nil {
			return err
		}
		out, err = s.oneMigration(ctx, q, m)
		return err
	})
	return out, err
}

func (s *Server) checkRunningLimit(ctx context.Context, q *store.Queries, m store.Migration) error {
	if s.migrations.MaxActive <= 0 {
		return nil
	}
	n, err := q.CountRunningMigrations(ctx, store.CountRunningMigrationsParams{WorkspaceID: m.WorkspaceID, ID: m.ID})
	if err != nil {
		return err
	}
	if int(n) >= s.migrations.MaxActive {
		return errHostedLimit(oas.LimitMigrationMaxActive, "this server runs at most "+strconv.Itoa(s.migrations.MaxActive)+" migrations of a workspace at a time (YUVA_MIGRATION_MAX_ACTIVE)")
	}
	return nil
}

func hasMigrationCredentials(ctx context.Context, q *store.Queries, m store.Migration) error {
	_, err := q.GetMigrationCredentials(ctx, store.GetMigrationCredentialsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
	if store.IsNotFound(err) {
		return errMigrationConnectFirst
	}
	return err
}

func (s *Server) setMigrationState(ctx context.Context, q *store.Queries, m store.Migration, state string) (store.Migration, error) {
	if err := q.ClearMigrationLease(ctx, store.ClearMigrationLeaseParams{WorkspaceID: m.WorkspaceID, ID: m.ID}); err != nil {
		return m, err
	}
	return q.SetMigrationRunState(ctx, store.SetMigrationRunStateParams{WorkspaceID: m.WorkspaceID, ID: m.ID, State: state, Now: s.now()})
}

func (s *Server) ScanMigration(ctx context.Context, req oas.ScanMigrationRequestObject) (oas.ScanMigrationResponseObject, error) {
	out, err := s.migrationStep(ctx, req.MigrationId, func(q *store.Queries, events *eventBatch, p principal, cur store.Migration) (store.Migration, error) {
		switch cur.State {
		case migrationScanning:
			return cur, nil
		case migrationConnected, migrationReady:
		default:
			return cur, errMigrationState("a migration is scanned once it is connected, and again while it is ready")
		}
		if err := migrationConnectedSource(ctx, q, cur); err != nil {
			return cur, err
		}
		if err := s.checkRunningLimit(ctx, q, cur); err != nil {
			return cur, err
		}
		if err := q.SetMigrationCheckpoint(ctx, store.SetMigrationCheckpointParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID, Checkpoint: mustJSON(migrationCheckpoint{Phase: scanPhaseList})}); err != nil {
			return cur, err
		}
		m, err := s.setMigrationState(ctx, q, cur, migrationScanning)
		if err != nil {
			return m, err
		}
		if _, err := s.migrationAudit(ctx, q, p, m.ID, "scanned", nil); err != nil {
			return m, err
		}
		events.job(migrationJob(m, 0), migrationJobOpts())
		return m, nil
	})
	if err != nil {
		return nil, err
	}
	return oas.ScanMigration200JSONResponse(out), nil
}

func (s *Server) StartMigration(ctx context.Context, req oas.StartMigrationRequestObject) (oas.StartMigrationResponseObject, error) {
	out, err := s.migrationStep(ctx, req.MigrationId, func(q *store.Queries, events *eventBatch, p principal, cur store.Migration) (store.Migration, error) {
		switch cur.State {
		case migrationCopying:
			return cur, nil
		case migrationReady:
		default:
			return cur, errMigrationState("a migration starts copying once its dry run is ready")
		}
		if cur.InboxID == nil || cur.ChannelID == nil {
			return cur, errMigrationState("choose the inbox and the e-mail channel the history goes to first")
		}
		if err := migrationConnectedSource(ctx, q, cur); err != nil {
			return cur, err
		}
		if err := s.checkRunningLimit(ctx, q, cur); err != nil {
			return cur, err
		}
		if s.migrations.MaxBytes > 0 {
			used, err := q.ImportedBytesThisMonth(ctx, store.ImportedBytesThisMonthParams{WorkspaceID: cur.WorkspaceID, Month: monthOf(s.now())})
			if err != nil {
				return cur, err
			}
			if used >= s.migrations.MaxBytes {
				return cur, errHostedLimit(oas.LimitMigrationMaxBytes, "this workspace imported as much as the server allows this month (YUVA_MIGRATION_MAX_BYTES)")
			}
		}
		if _, err := q.MarkMigrationItemsInCopy(ctx, store.MarkMigrationItemsInCopyParams{WorkspaceID: cur.WorkspaceID, MigrationID: cur.ID}); err != nil {
			return cur, err
		}
		counts, _, err := s.refreshMigrationCounts(ctx, q, cur)
		if err != nil {
			return cur, err
		}
		if err := q.SetMigrationCheckpoint(ctx, store.SetMigrationCheckpointParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID, Checkpoint: []byte("{}")}); err != nil {
			return cur, err
		}
		if err := q.ClearMigrationLease(ctx, store.ClearMigrationLeaseParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID}); err != nil {
			return cur, err
		}
		m, err := q.StartMigrationCopy(ctx, store.StartMigrationCopyParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID, Now: s.now()})
		if err != nil {
			return m, err
		}
		if _, err := s.migrationAudit(ctx, q, p, m.ID, "started", map[string]any{"messages": counts.Total}); err != nil {
			return m, err
		}
		events.job(migrationJob(m, 0), migrationJobOpts())
		return m, nil
	})
	if err != nil {
		return nil, err
	}
	return oas.StartMigration200JSONResponse(out), nil
}

func (s *Server) PauseMigration(ctx context.Context, req oas.PauseMigrationRequestObject) (oas.PauseMigrationResponseObject, error) {
	out, err := s.migrationStep(ctx, req.MigrationId, func(q *store.Queries, events *eventBatch, p principal, cur store.Migration) (store.Migration, error) {
		switch cur.State {
		case migrationPaused:
			return cur, nil
		case migrationCopying:
		default:
			return cur, errMigrationState("only a copying migration can be paused")
		}
		m, err := s.setMigrationState(ctx, q, cur, migrationPaused)
		if err != nil {
			return m, err
		}
		_, err = s.migrationAudit(ctx, q, p, m.ID, "paused", nil)
		return m, err
	})
	if err != nil {
		return nil, err
	}
	return oas.PauseMigration200JSONResponse(out), nil
}

func (s *Server) ResumeMigration(ctx context.Context, req oas.ResumeMigrationRequestObject) (oas.ResumeMigrationResponseObject, error) {
	out, err := s.migrationStep(ctx, req.MigrationId, func(q *store.Queries, events *eventBatch, p principal, cur store.Migration) (store.Migration, error) {
		next := migrationCopying
		switch cur.State {
		case migrationCopying:
			return cur, nil
		case migrationPaused:
		case migrationNeedsAttention:
			if cur.StartedAt == nil {
				next = migrationConnected
			}
		default:
			return cur, errMigrationState("only a paused migration, or one that needs attention, can be resumed")
		}
		if err := migrationConnectedSource(ctx, q, cur); err != nil {
			return cur, err
		}
		if next == migrationCopying {
			if err := s.checkRunningLimit(ctx, q, cur); err != nil {
				return cur, err
			}
			if err := q.SetMigrationThrottle(ctx, store.SetMigrationThrottleParams{
				WorkspaceID: cur.WorkspaceID, ID: cur.ID, Throttle: mustJSON(func() any { t := readThrottle(cur.Throttle); t.Backoff = 0; return t }()),
			}); err != nil {
				return cur, err
			}
		}
		m, err := s.setMigrationState(ctx, q, cur, next)
		if err != nil {
			return m, err
		}
		if _, err := s.migrationAudit(ctx, q, p, m.ID, "resumed", map[string]any{"state": next}); err != nil {
			return m, err
		}
		if next == migrationCopying {
			events.job(migrationJob(m, 0), migrationJobOpts())
		}
		return m, nil
	})
	if err != nil {
		return nil, err
	}
	return oas.ResumeMigration200JSONResponse(out), nil
}

func (s *Server) CancelMigration(ctx context.Context, req oas.CancelMigrationRequestObject) (oas.CancelMigrationResponseObject, error) {
	out, err := s.migrationStep(ctx, req.MigrationId, func(q *store.Queries, events *eventBatch, p principal, cur store.Migration) (store.Migration, error) {
		if cur.State == string(oas.MigrationCancelled) {
			return cur, nil
		}
		if !cancellableMigration(cur.State) {
			return cur, errMigrationState("a draft is deleted rather than cancelled, and a finished migration cannot be cancelled")
		}
		n, err := q.DeleteMigrationCredentials(ctx, store.DeleteMigrationCredentialsParams{WorkspaceID: cur.WorkspaceID, MigrationID: cur.ID})
		if err != nil {
			return cur, err
		}
		if err := q.DeleteMigrationSignIn(ctx, store.DeleteMigrationSignInParams{WorkspaceID: cur.WorkspaceID, MigrationID: cur.ID}); err != nil {
			return cur, err
		}
		m, err := q.FinishMigration(ctx, store.FinishMigrationParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID, State: string(oas.MigrationCancelled), Now: new(s.now())})
		if err != nil {
			return m, err
		}
		if err := clearMigrationPersonalData(ctx, q, m.WorkspaceID, m.ID); err != nil {
			return m, err
		}
		if _, err := s.migrationAudit(ctx, q, p, m.ID, "cancelled", map[string]any{"state": cur.State}); err != nil {
			return m, err
		}
		if n > 0 {
			_, err = s.migrationAudit(ctx, q, p, m.ID, "credentials_deleted", nil)
		}
		return m, err
	})
	if err != nil {
		return nil, err
	}
	if out.Source == oas.MigrationSourceFile {
		ws := principalFrom(ctx).workspaceID
		s.dropUploadLogged(ctx, ws, out.Id)
		m, err := s.st.GetMigration(ctx, store.GetMigrationParams{WorkspaceID: ws, ID: out.Id})
		if err != nil {
			return nil, err
		}
		if out, err = s.oneMigration(ctx, s.st.Queries, m); err != nil {
			return nil, err
		}
	}
	return oas.CancelMigration200JSONResponse(out), nil
}

func (s *Server) RetryMigrationItems(ctx context.Context, req oas.RetryMigrationItemsRequestObject) (oas.RetryMigrationItemsResponseObject, error) {
	out, err := s.migrationStep(ctx, req.MigrationId, func(q *store.Queries, events *eventBatch, p principal, cur store.Migration) (store.Migration, error) {
		switch cur.State {
		case migrationCopying, migrationPaused, migrationSyncing:
		default:
			return cur, errMigrationState("failed messages are retried while the migration copies, is paused or syncs")
		}
		n, err := q.RetryFailedMigrationItems(ctx, store.RetryFailedMigrationItemsParams{WorkspaceID: cur.WorkspaceID, MigrationID: cur.ID})
		if err != nil || n == 0 {
			return cur, err
		}
		counts, _, err := s.refreshMigrationCounts(ctx, q, cur)
		if err != nil {
			return cur, err
		}
		cur.Counts = mustJSON(counts)
		m := cur
		if cur.State == migrationSyncing {
			if m, err = s.setMigrationState(ctx, q, cur, migrationCopying); err != nil {
				return m, err
			}
		}
		if _, err := s.migrationAudit(ctx, q, p, m.ID, "retried", map[string]any{"conversations": n}); err != nil {
			return m, err
		}
		if m.State == migrationCopying && cur.State != migrationSyncing {
			events.jobOnce(migrationJob(m, 0), migrationJobOpts())
		}
		return m, nil
	})
	if err != nil {
		return nil, err
	}
	return oas.RetryMigrationItems200JSONResponse(out), nil
}

func (s *Server) ListMigrationFolders(ctx context.Context, req oas.ListMigrationFoldersRequestObject) (oas.ListMigrationFoldersResponseObject, error) {
	p := principalFrom(ctx)
	m, err := visibleMigration(ctx, s.st.Queries, p, req.MigrationId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
	if err != nil {
		return nil, err
	}
	out := oas.ListMigrationFolders200JSONResponse{Items: make([]oas.MigrationFolder, len(rows))}
	var targets map[int32]uuid.UUID
	if helpdeskMigration(m) {
		targets = folderInboxes(m, rows)
	}
	for i, f := range rows {
		out.Items[i] = oas.MigrationFolder{
			Id: f.ID, Name: f.Name, Kind: oas.MigrationFolderKind(f.Kind), Selected: f.Selected, Use: folderUse(f, rows),
			LabelAction: oas.MigrationFolderLabelAction(f.LabelAction), LabelId: f.LabelID, Messages: int64(f.Total), SizeBytes: f.SizeBytes,
			OldestAt: f.OldestAt, NewestAt: f.NewestAt, Imported: int64(f.Imported), Duplicate: int64(f.Duplicate), Skipped: int64(f.Skipped),
			Failed: int64(f.Failed), Bytes: f.Bytes, Address: f.Address,
		}
		if id, ok := targets[f.ID]; ok {
			out.Items[i].InboxId = &id
		}
	}
	return out, nil
}

func (s *Server) GetMigrationPreview(ctx context.Context, req oas.GetMigrationPreviewRequestObject) (oas.GetMigrationPreviewResponseObject, error) {
	p := principalFrom(ctx)
	m, err := visibleMigration(ctx, s.st.Queries, p, req.MigrationId, false)
	if err != nil {
		return nil, err
	}
	doc := readEstimate(m.Estimate)
	if doc == nil || doc.Preview == nil {
		return nil, errMigrationState("the preview is ready once a scan finished")
	}
	return oas.GetMigrationPreview200JSONResponse(*doc.Preview), nil
}

func (s *Server) ListMigrationErrors(ctx context.Context, req oas.ListMigrationErrorsRequestObject) (oas.ListMigrationErrorsResponseObject, error) {
	p := principalFrom(ctx)
	m, err := visibleMigration(ctx, s.st.Queries, p, req.MigrationId, false)
	if err != nil {
		return nil, err
	}
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	var before *int64
	if c := req.Params.Cursor; c != nil && *c != "" {
		v, err := strconv.ParseInt(*c, 10, 64)
		if err != nil || v < 1 {
			return nil, errInvalidCursor
		}
		before = &v
	}
	rows, err := s.st.ListMigrationErrors(ctx, store.ListMigrationErrorsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, BeforeID: before, Lim: lim + 1})
	if err != nil {
		return nil, err
	}
	out := oas.ListMigrationErrors200JSONResponse{Items: []oas.MigrationError{}}
	if len(rows) > int(lim) {
		rows = rows[:lim]
		out.NextCursor = new(strconv.FormatInt(rows[lim-1].ID, 10))
	}
	for _, r := range rows {
		out.Items = append(out.Items, oas.MigrationError{
			Id: r.ID, Code: r.Code, Detail: r.Detail, Folder: r.Folder, ItemDate: r.ItemDate, Subject: r.ItemSubject, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

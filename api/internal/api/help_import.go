package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	helpImportQueued    = string(oas.HelpImportStateQueued)
	helpImportRunning   = string(oas.HelpImportStateRunning)
	helpImportDone      = string(oas.HelpImportStateDone)
	helpImportFailed    = string(oas.HelpImportStateFailed)
	helpImportCancelled = string(oas.HelpImportStateCancelled)

	helpImportSourceZendesk   = string(oas.Zendesk)
	helpImportSourceHelpScout = string(oas.Helpscout)

	eventHelpImportUpdated  = string(oas.HelpImportUpdated)
	eventHelpImportProgress = string(oas.HelpImportProgressType)

	helpImportProgressEvery = 2 * time.Second
	helpImportRedirectsPage = 500
)

var (
	limitHelpImportStart = limit{10, time.Hour}

	errHelpImportGone    = problem(http.StatusNotFound, "not_found", "no such import")
	errHelpImportRunning = problem(http.StatusConflict, "import_running", "this help center is being imported into; wait for it to end or cancel it")
	errHelpImportSource  = problem(http.StatusUnprocessableEntity, "source_not_connected", "the migration is not signed in to Zendesk any more; connect it again or start a new Zendesk migration")
	errHelpImportKind    = problem(http.StatusUnprocessableEntity, "source_mismatch", "migration_id must be a Zendesk migration")
)

func helpImportSecretContext(ws, id uuid.UUID) []byte {
	return append(append([]byte("help-import:"), ws[:]...), id[:]...)
}

type helpImportOptions struct {
	Locale          string `json:"locale,omitempty"`
	Site            string `json:"site,omitempty"`
	OverwriteEdited bool   `json:"overwrite_edited,omitempty"`
}

func readHelpImportOptions(raw []byte) helpImportOptions {
	var o helpImportOptions
	_ = json.Unmarshal(raw, &o)
	return o
}

func helpImportCounts(raw []byte) oas.HelpImportCounts {
	var c oas.HelpImportCounts
	_ = json.Unmarshal(raw, &c)
	return c
}

func helpImportBody(imp store.HelpImport, redirects int32) oas.HelpImport {
	skipped := imp.SkippedLocales
	if skipped == nil {
		skipped = []string{}
	}
	return oas.HelpImport{
		Id: imp.ID, CenterId: imp.CenterID, Source: oas.HelpImportSource(imp.Source), MigrationId: imp.MigrationID,
		State: oas.HelpImportState(imp.State), Phase: oas.HelpImportPhase(imp.Phase), Counts: helpImportCounts(imp.Counts),
		SkippedLocales: skipped, Redirects: redirects, OverwriteEdited: new(readHelpImportOptions(imp.Options).OverwriteEdited), Error: imp.Error,
		MemberId: imp.MemberID, ApiKeyId: imp.ApiKeyID, CreatedAt: imp.CreatedAt, UpdatedAt: imp.UpdatedAt, StartedAt: imp.StartedAt, FinishedAt: imp.FinishedAt,
	}
}

func (s *Server) helpImportOut(ctx context.Context, q *store.Queries, imp store.HelpImport) (oas.HelpImport, error) {
	n, err := q.CountHelpImportRedirects(ctx, store.CountHelpImportRedirectsParams{WorkspaceID: imp.WorkspaceID, ImportID: imp.ID})
	if err != nil {
		return oas.HelpImport{}, err
	}
	return helpImportBody(imp, n), nil
}

func (s *Server) helpImportSignal(ctx context.Context, c store.HelpCenter, imp store.HelpImport) {
	body, err := s.helpImportOut(ctx, s.st.Queries, imp)
	if err != nil {
		s.log.WarnContext(ctx, "help import signal", "error", err)
		return
	}
	inbox := c.InboxID
	s.signal(ctx, realtime.Event{Type: eventHelpImportUpdated, WorkspaceID: imp.WorkspaceID, InboxID: &inbox, Data: mustJSON(body)})
}

func (s *Server) helpImportProgress(ctx context.Context, c store.HelpCenter, imp store.HelpImport) {
	if !s.progressDue(imp.ID, helpImportProgressEvery) {
		return
	}
	inbox := c.InboxID
	s.signal(ctx, realtime.Event{Type: eventHelpImportProgress, WorkspaceID: imp.WorkspaceID, InboxID: &inbox, Data: mustJSON(oas.HelpImportProgress{
		Id: imp.ID, CenterId: imp.CenterID, State: oas.HelpImportState(imp.State), Phase: oas.HelpImportPhase(imp.Phase), Counts: helpImportCounts(imp.Counts),
	})})
}

func visibleHelpImport(ctx context.Context, q *store.Queries, p principal, centerID, id uuid.UUID, lock bool) (store.HelpCenter, store.HelpImport, error) {
	c, err := visibleHelpCenter(ctx, q, p, centerID, false)
	if err != nil {
		return c, store.HelpImport{}, err
	}
	var imp store.HelpImport
	if lock {
		imp, err = q.LockHelpImport(ctx, store.LockHelpImportParams{WorkspaceID: p.workspaceID, ID: id})
		if err == nil && imp.CenterID != c.ID {
			err = errHelpImportGone
		}
	} else {
		imp, err = q.GetHelpImport(ctx, store.GetHelpImportParams{WorkspaceID: p.workspaceID, CenterID: c.ID, ID: id})
	}
	if store.IsNotFound(err) {
		return c, imp, errHelpImportGone
	}
	return c, imp, err
}

func (s *Server) StartHelpImport(ctx context.Context, req oas.StartHelpImportRequestObject) (oas.StartHelpImportResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	opts := helpImportOptions{OverwriteEdited: b.OverwriteEdited != nil && *b.OverwriteEdited}
	var key string
	switch b.Source {
	case oas.Zendesk:
		if b.MigrationId == nil {
			return nil, errValidation("migration_id is required for zendesk")
		}
		if b.DocsApiKey != nil || b.SiteId != nil || b.Locale != nil {
			return nil, errValidation("docs_api_key, site_id and locale apply to helpscout only")
		}
	case oas.Helpscout:
		if b.MigrationId != nil {
			return nil, errValidation("migration_id applies to zendesk only")
		}
		if p.isGrant() {
			return nil, errMemberSessionRequired
		}
		var err error
		if key, err = trimmed(deref(b.DocsApiKey), 1, 200, "docs_api_key"); err != nil {
			return nil, err
		}
		if b.SiteId != nil {
			if opts.Site, err = trimmed(*b.SiteId, 1, 100, "site_id"); err != nil {
				return nil, err
			}
		}
		if b.Locale != nil {
			if err := validHelpLocale(*b.Locale, "locale"); err != nil {
				return nil, err
			}
			opts.Locale = *b.Locale
		}
	default:
		return nil, errValidation("source must be zendesk or helpscout")
	}
	if err := s.rateLimit(ctx, rateCheck{"help-import:" + p.workspaceID.String(), limitHelpImportStart}); err != nil {
		return nil, err
	}
	var (
		imp    store.HelpImport
		center store.HelpCenter
		out    oas.HelpImport
	)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		c, err := visibleHelpCenter(ctx, q, p, req.CenterId, true)
		if err != nil {
			return err
		}
		center = c
		id := newID()
		var migration *uuid.UUID
		var sealed []byte
		switch b.Source {
		case oas.Zendesk:
			if requireMigrationManager(p) != nil || requireScope(p, oas.MigrationsManage) != nil {
				return errMigrationGone
			}
			m, err := q.GetMigration(ctx, store.GetMigrationParams{WorkspaceID: p.workspaceID, ID: *b.MigrationId})
			if store.IsNotFound(err) {
				return errMigrationGone
			}
			if err != nil {
				return err
			}
			if err := mayManageMigration(ctx, q, p, m); err != nil {
				return errMigrationGone
			}
			if m.Source != helpImportSourceZendesk {
				return errHelpImportKind
			}
			if _, err := q.GetMigrationCredentials(ctx, store.GetMigrationCredentialsParams{WorkspaceID: p.workspaceID, MigrationID: m.ID}); store.IsNotFound(err) {
				return errHelpImportSource
			} else if err != nil {
				return err
			}
			migration = &m.ID
		case oas.Helpscout:
			if opts.Locale == "" {
				opts.Locale = c.DefaultLocale
			}
			if err := offersLocale(c, opts.Locale); err != nil {
				return err
			}
			sealed = s.secrets.Seal([]byte(key), helpImportSecretContext(p.workspaceID, id))
		}
		member, apiKey := helpCreatedBy(p)
		imp, err = q.CreateHelpImport(ctx, store.CreateHelpImportParams{
			ID: id, WorkspaceID: p.workspaceID, CenterID: c.ID, Source: string(b.Source), MigrationID: migration, Origin: "", Sealed: sealed,
			Options: mustJSON(opts), MemberID: member, ApiKeyID: apiKey, Now: s.now(),
		})
		if store.IsUniqueViolation(err) {
			return errHelpImportRunning
		}
		if err != nil {
			return err
		}
		events.job(HelpImportArgs{WorkspaceID: p.workspaceID, ImportID: imp.ID}, helpImportOpts())
		out, err = s.helpImportOut(ctx, q, imp)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.helpImportSignal(ctx, center, imp)
	return oas.StartHelpImport202JSONResponse(out), nil
}

func (s *Server) ListHelpImports(ctx context.Context, req oas.ListHelpImportsRequestObject) (oas.ListHelpImportsResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	c, err := visibleHelpCenter(ctx, s.st.Queries, p, req.CenterId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListHelpImports(ctx, store.ListHelpImportsParams{WorkspaceID: p.workspaceID, CenterID: c.ID, CursorAt: at, CursorID: id, Lim: lim + 1})
	if err != nil {
		return nil, err
	}
	out := oas.HelpImportPage{Items: []oas.HelpImport{}}
	if len(rows) > int(lim) {
		rows = rows[:lim]
		out.NextCursor = new(encodeCursor(rows[len(rows)-1].CreatedAt, rows[len(rows)-1].ID))
	}
	for _, r := range rows {
		body, err := s.helpImportOut(ctx, s.st.Queries, r)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, body)
	}
	return oas.ListHelpImports200JSONResponse(out), nil
}

func (s *Server) GetHelpImport(ctx context.Context, req oas.GetHelpImportRequestObject) (oas.GetHelpImportResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	_, imp, err := visibleHelpImport(ctx, s.st.Queries, principalFrom(ctx), req.CenterId, req.ImportId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.helpImportOut(ctx, s.st.Queries, imp)
	if err != nil {
		return nil, err
	}
	return oas.GetHelpImport200JSONResponse(out), nil
}

func (s *Server) CancelHelpImport(ctx context.Context, req oas.CancelHelpImportRequestObject) (oas.CancelHelpImportResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var (
		center    store.HelpCenter
		imp       store.HelpImport
		cancelled bool
	)
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		c, cur, err := visibleHelpImport(ctx, q, p, req.CenterId, req.ImportId, true)
		if err != nil {
			return err
		}
		center = c
		if cur.State != helpImportQueued && cur.State != helpImportRunning {
			return nil
		}
		imp, err = q.FinishHelpImport(ctx, store.FinishHelpImportParams{WorkspaceID: p.workspaceID, ID: cur.ID, State: helpImportCancelled, Now: new(s.now())})
		cancelled = err == nil
		return err
	})
	if err != nil {
		return nil, err
	}
	if cancelled {
		s.helpImportSignal(ctx, center, imp)
	}
	return oas.CancelHelpImport204Response{}, nil
}

type helpRedirectCursor struct{ kind, source, locale string }

func encodeHelpRedirectCursor(c helpRedirectCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.kind + "\x1f" + c.source + "\x1f" + c.locale))
}

func decodeHelpRedirectCursor(v *string) (helpRedirectCursor, error) {
	if v == nil || *v == "" {
		return helpRedirectCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*v)
	parts := strings.Split(string(raw), "\x1f")
	if err != nil || len(parts) != 3 {
		return helpRedirectCursor{}, errInvalidCursor
	}
	return helpRedirectCursor{parts[0], parts[1], parts[2]}, nil
}

func (s *Server) helpRedirectRows(ctx context.Context, c store.HelpCenter, importID uuid.UUID, after helpRedirectCursor, lim int32) ([]oas.HelpImportRedirect, []store.ListHelpImportRedirectsRow, error) {
	rows, err := s.st.ListHelpImportRedirects(ctx, store.ListHelpImportRedirectsParams{
		WorkspaceID: c.WorkspaceID, ImportID: importID, AfterKind: after.kind, AfterSource: after.source, AfterLocale: after.locale, Lim: lim,
	})
	if err != nil {
		return nil, nil, err
	}
	out := make([]oas.HelpImportRedirect, len(rows))
	for i, r := range rows {
		item := oas.HelpImportRedirect{Kind: oas.HelpImportRedirectKind(r.Kind), Locale: r.Locale, OldUrl: r.OldUrl, Published: r.Published}
		if r.Kind == string(oas.HelpImportRedirectKindArticle) {
			item.ArticleId = new(r.TargetID)
			item.NewUrl = s.helpArticleURL(c, r.Number, r.Locale, r.Slug)
		} else {
			item.CollectionId = new(r.TargetID)
			item.NewUrl = s.helpURL(c, "/"+r.Locale+"/collections/"+strconv.Itoa(int(r.Number))+"-"+r.Slug)
		}
		out[i] = item
	}
	return out, rows, nil
}

func (s *Server) ListHelpImportRedirects(ctx context.Context, req oas.ListHelpImportRedirectsRequestObject) (oas.ListHelpImportRedirectsResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	after, err := decodeHelpRedirectCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	c, imp, err := visibleHelpImport(ctx, s.st.Queries, p, req.CenterId, req.ImportId, false)
	if err != nil {
		return nil, err
	}
	items, rows, err := s.helpRedirectRows(ctx, c, imp.ID, after, lim+1)
	if err != nil {
		return nil, err
	}
	out := oas.HelpImportRedirectPage{Items: items}
	if len(items) > int(lim) {
		out.Items = items[:lim]
		last := rows[lim-1]
		out.NextCursor = new(encodeHelpRedirectCursor(helpRedirectCursor{last.Kind, last.SourceID, last.Locale}))
	}
	return oas.ListHelpImportRedirects200JSONResponse(out), nil
}

func (s *Server) DownloadHelpImportRedirects(ctx context.Context, req oas.DownloadHelpImportRedirectsRequestObject) (oas.DownloadHelpImportRedirectsResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	c, imp, err := visibleHelpImport(ctx, s.st.Queries, principalFrom(ctx), req.CenterId, req.ImportId, false)
	if err != nil {
		return nil, err
	}
	return helpRedirectsStream{s: s, ctx: ctx, center: c, id: imp.ID}, nil
}

type helpRedirectsStream struct {
	s      *Server
	ctx    context.Context
	center store.HelpCenter
	id     uuid.UUID
}

func (e helpRedirectsStream) VisitDownloadHelpImportRedirectsResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="help-import-redirects-`+e.id.String()+`.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	var buf strings.Builder
	buf.WriteString(csvCell("kind") + "," + csvCell("locale") + "," + csvCell("old_url") + "," + csvCell("new_url") + "," + csvCell("published") + "\r\n")
	var after helpRedirectCursor
	for {
		items, rows, err := e.s.helpRedirectRows(e.ctx, e.center, e.id, after, helpImportRedirectsPage)
		if err != nil {
			return err
		}
		for _, it := range items {
			buf.WriteString(csvCell(string(it.Kind)) + "," + csvCell(it.Locale) + "," + csvCell(it.OldUrl) + "," + csvCell(it.NewUrl) + "," + csvCell(strconv.FormatBool(it.Published)) + "\r\n")
		}
		if _, err := io.WriteString(w, buf.String()); err != nil {
			return err
		}
		buf.Reset()
		if len(rows) < helpImportRedirectsPage {
			return nil
		}
		last := rows[len(rows)-1]
		after = helpRedirectCursor{last.Kind, last.SourceID, last.Locale}
	}
}

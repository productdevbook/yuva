package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/productdevbook/yuva/api/internal/analytics"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/storage"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	analyticsExportTTL       = 7 * 24 * time.Hour
	maxAnalyticsExportBytes  = 1 << 30
	maxRawExportDays         = 31
	maxExportsPerWorkspace   = 3
	analyticsExportTimeout   = time.Hour
	analyticsExportAttempts  = 3
	analyticsExportKeyPrefix = "analytics-exports/"
)

var (
	errExportGone     = problem(http.StatusNotFound, "not_found", "no such export")
	errExportNotReady = problem(http.StatusConflict, "export_not_ready", "the export has not finished")
	errExportsBusy    = &apiError{Status: http.StatusTooManyRequests, Code: "rate_limited",
		Detail: "this workspace already runs three exports; wait for one to finish", RetryAfter: 30}
	errExportTooLarge = errors.New("too_large")
)

var dailyExportMetrics = []oas.AnalyticsMetric{
	oas.AnalyticsMetricVisitors, oas.AnalyticsMetricVisits, oas.AnalyticsMetricPageviews, oas.AnalyticsMetricViewsPerVisit,
	oas.AnalyticsMetricBounceRate, oas.AnalyticsMetricVisitDuration, oas.AnalyticsMetricTimeOnPage, oas.AnalyticsMetricScrollDepth,
}

// exportParams is what a queued export needs besides its row: a view's query with its range fixed
// when it was asked for, or a daily export's dimension.
type exportParams struct {
	Query     *oas.AnalyticsQuery `json:"query,omitempty"`
	Dimension string              `json:"dimension,omitempty"`
}

// AnalyticsExportArgs writes one export's file.
type AnalyticsExportArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ExportID    uuid.UUID `json:"export_id"`
}

func (AnalyticsExportArgs) Kind() string { return "analytics_export" }

func (AnalyticsExportArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.AnalyticsQueue, MaxAttempts: analyticsExportAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type analyticsExportWorker struct {
	river.WorkerDefaults[AnalyticsExportArgs]
	s *Server
}

func (w *analyticsExportWorker) Timeout(*river.Job[AnalyticsExportArgs]) time.Duration {
	return analyticsExportTimeout
}

func (w *analyticsExportWorker) Work(ctx context.Context, job *river.Job[AnalyticsExportArgs]) error {
	err := w.s.RunAnalyticsExport(ctx, job.Args.WorkspaceID, job.Args.ExportID)
	if err != nil && job.Attempt >= job.MaxAttempts {
		w.s.failExport(context.WithoutCancel(ctx), job.Args.WorkspaceID, job.Args.ExportID, "internal")
	}
	return err
}

// mayExportRaw: raw rows are pseudonymous personal data, so only those who manage analytics get them.
func mayExportRaw(p principal) error {
	if err := requireScope(p, oas.AnalyticsManage); err != nil {
		return err
	}
	return requireAnalyticsManager(p)
}

func exportFileName(ex store.AnalyticsExport) string {
	return "analytics-" + ex.Kind + "-" + ex.FromDay.Format(time.DateOnly) + "-" + ex.ToDay.Format(time.DateOnly) + "." + ex.Format
}

func exportBody(ex store.AnalyticsExport) oas.AnalyticsExport {
	out := oas.AnalyticsExport{
		Id: ex.ID, PropertyId: ex.PropertyID, Kind: oas.AnalyticsExportKind(ex.Kind), Format: oas.AnalyticsExportFormat(ex.Format),
		Status: oas.AnalyticsExportStatus(ex.Status), From: openapi_types.Date{Time: ex.FromDay}, To: openapi_types.Date{Time: ex.ToDay},
		Progress: int(ex.Progress), Rows: ex.Rows, SizeBytes: ex.SizeBytes, Error: ex.Error, CreatedBy: ex.CreatedBy,
		CreatedAt: ex.CreatedAt, FinishedAt: ex.FinishedAt, ExpiresAt: ex.ExpiresAt,
	}
	if ex.Status == string(oas.AnalyticsExportDone) {
		out.FileName = new(exportFileName(ex))
	}
	return out
}

func (s *Server) CreateAnalyticsExport(ctx context.Context, req oas.CreateAnalyticsExportRequestObject) (oas.CreateAnalyticsExportResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	if !b.Kind.Valid() {
		return nil, errValidation("kind must be view, daily or raw")
	}
	format := oas.AnalyticsExportCSV
	if b.Format != nil {
		format = *b.Format
	}
	if !format.Valid() {
		return nil, errValidation("format must be csv or ndjson")
	}
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	if b.Kind == oas.AnalyticsExportRaw {
		if err := mayExportRaw(p); err != nil {
			return nil, err
		}
		if b.Format != nil && format != oas.AnalyticsExportNDJSON {
			return nil, errValidation("raw exports are NDJSON")
		}
		format = oas.AnalyticsExportNDJSON
	}
	today := analytics.LocalDay(s.now(), propertyLocation(prop.Timezone))
	var params exportParams
	var from, to time.Time
	switch b.Kind {
	case oas.AnalyticsExportView:
		if b.Query == nil || b.Range != nil || b.Dimension != nil {
			return nil, errValidation("a view export takes query, with its own range")
		}
		q := *b.Query
		q.Compare, q.Limit, q.Cursor = nil, nil, nil
		ar, err := s.analyticsRequest(ctx, prop, q)
		if err != nil {
			return nil, err
		}
		from, to = ar.q.From, ar.q.To
		q.Range = oas.AnalyticsRange{From: &openapi_types.Date{Time: from}, To: &openapi_types.Date{Time: to}}
		params.Query = &q
	default:
		if b.Range == nil || b.Query != nil {
			return nil, errValidation("daily and raw exports take range, not query")
		}
		if from, to, err = analyticsRange(*b.Range, today); err != nil {
			return nil, err
		}
		if b.Dimension != nil {
			if b.Kind != oas.AnalyticsExportDaily {
				return nil, errValidation("only daily exports take a dimension")
			}
			if !b.Dimension.Valid() || *b.Dimension == oas.AnalyticsDimEvent || *b.Dimension == oas.AnalyticsDimProp {
				return nil, errValidation("dimension must be a visit or page dimension, not event or prop")
			}
			params.Dimension = string(*b.Dimension)
		}
		if b.Kind == oas.AnalyticsExportRaw {
			if to.Sub(from) >= maxRawExportDays*24*time.Hour {
				return nil, problem(http.StatusUnprocessableEntity, "range_too_large", "a raw export may span at most 31 days")
			}
			if raw := s.rawFrom(); from.Before(raw) {
				e := problem(http.StatusUnprocessableEntity, "needs_raw_window", "raw rows are kept from "+raw.Format(time.DateOnly)+"; narrow the range")
				e.Extra = map[string]any{"raw_window_from": raw.Format(time.DateOnly)}
				return nil, e
			}
		}
	}
	var out store.AnalyticsExport
	err = s.st.InTxRaw(ctx, func(tx pgx.Tx, q *store.Queries) error {
		now := s.now()
		n, err := q.CountUnfinishedAnalyticsExports(ctx, store.CountUnfinishedAnalyticsExportsParams{WorkspaceID: p.workspaceID, Now: now})
		if err != nil {
			return err
		}
		if n >= maxExportsPerWorkspace {
			return errExportsBusy
		}
		out, err = q.CreateAnalyticsExport(ctx, store.CreateAnalyticsExportParams{
			ID: newID(), WorkspaceID: p.workspaceID, PropertyID: prop.ID, Kind: string(b.Kind), Format: string(format),
			Params: mustJSON(params), FromDay: from, ToDay: to, CreatedBy: p.memberRef(), Now: now, ExpiresAt: now.Add(analyticsExportTTL),
		})
		if err != nil {
			return err
		}
		if _, err = s.jobs.InsertTx(ctx, tx, AnalyticsExportArgs{WorkspaceID: p.workspaceID, ExportID: out.ID}, nil); err != nil {
			return err
		}
		err = s.audit(ctx, q, p, oas.AuditAnalyticsExportCreated, oas.AuditTargetAnalyticsExport, out.ID,
			auditDetails{"property_id": prop.ID, "kind": out.Kind, "format": out.Format})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateAnalyticsExport202JSONResponse(exportBody(out)), nil
}

func (s *Server) ListAnalyticsExports(ctx context.Context, req oas.ListAnalyticsExportsRequestObject) (oas.ListAnalyticsExportsResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListAnalyticsExports(ctx, store.ListAnalyticsExportsParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID, Now: s.now(), WithRaw: mayExportRaw(p) == nil})
	if err != nil {
		return nil, err
	}
	out := oas.ListAnalyticsExports200JSONResponse{Items: make([]oas.AnalyticsExport, len(rows))}
	for i, ex := range rows {
		out.Items[i] = exportBody(ex)
	}
	return out, nil
}

// visibleExport is an export of a property the caller sees that has not expired; a raw one only
// for those who may make raw exports.
func (s *Server) visibleExport(ctx context.Context, p principal, propertyID, id uuid.UUID) (store.AnalyticsExport, error) {
	prop, err := visibleProperty(ctx, s.st.Queries, p, propertyID, false)
	if err != nil {
		return store.AnalyticsExport{}, err
	}
	ex, err := s.st.GetAnalyticsExport(ctx, store.GetAnalyticsExportParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID, ID: id, Now: s.now()})
	if store.IsNotFound(err) {
		return ex, errExportGone
	}
	if err != nil {
		return ex, err
	}
	if ex.Kind == string(oas.AnalyticsExportRaw) {
		if err := mayExportRaw(p); err != nil {
			return ex, err
		}
	}
	return ex, nil
}

func (s *Server) GetAnalyticsExport(ctx context.Context, req oas.GetAnalyticsExportRequestObject) (oas.GetAnalyticsExportResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	ex, err := s.visibleExport(ctx, principalFrom(ctx), req.PropertyId, req.ExportId)
	if err != nil {
		return nil, err
	}
	return oas.GetAnalyticsExport200JSONResponse(exportBody(ex)), nil
}

func (s *Server) DeleteAnalyticsExport(ctx context.Context, req oas.DeleteAnalyticsExportRequestObject) (oas.DeleteAnalyticsExportResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	ex, err := s.visibleExport(ctx, p, req.PropertyId, req.ExportId)
	if err != nil {
		return nil, err
	}
	key, err := s.st.DeleteAnalyticsExport(ctx, store.DeleteAnalyticsExportParams{WorkspaceID: p.workspaceID, PropertyID: ex.PropertyID, ID: ex.ID})
	if store.IsNotFound(err) {
		return nil, errExportGone
	}
	if err != nil {
		return nil, err
	}
	if key != nil {
		if err := s.objects.Delete(ctx, *key); err != nil {
			return nil, err
		}
	}
	return oas.DeleteAnalyticsExport204Response{}, nil
}

func (s *Server) DownloadAnalyticsExport(ctx context.Context, req oas.DownloadAnalyticsExportRequestObject) (oas.DownloadAnalyticsExportResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	ex, err := s.visibleExport(ctx, principalFrom(ctx), req.PropertyId, req.ExportId)
	if err != nil {
		return nil, err
	}
	if ex.Status != string(oas.AnalyticsExportDone) || ex.StorageKey == nil {
		return nil, errExportNotReady
	}
	body, err := s.objects.Open(ctx, *ex.StorageKey)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, errExportGone
	}
	if err != nil {
		return nil, err
	}
	disposition := `attachment; filename="` + exportFileName(ex) + `"`
	headers := oas.DownloadAnalyticsExport200ResponseHeaders{ContentDisposition: &disposition}
	if ex.Format == string(oas.AnalyticsExportCSV) {
		return oas.DownloadAnalyticsExport200TextcsvResponse{Body: body, ContentLength: ex.SizeBytes, Headers: headers}, nil
	}
	return oas.DownloadAnalyticsExport200ApplicationxNdjsonResponse{Body: body, ContentLength: ex.SizeBytes, Headers: headers}, nil
}

type cappedWriter struct {
	w     io.Writer
	n     int64
	limit int64
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.n+int64(len(p)) > c.limit {
		return 0, errExportTooLarge
	}
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func (s *Server) failExport(ctx context.Context, ws, id uuid.UUID, reason string) {
	if _, err := s.st.FailAnalyticsExport(ctx, store.FailAnalyticsExportParams{WorkspaceID: ws, ID: id, Error: reason, Now: s.now()}); err != nil {
		s.log.ErrorContext(ctx, "analytics export", slog.String("export_id", id.String()), slog.Any("error", err))
	}
}

// exportFailure names why an export cannot succeed however often it runs; "" means it may on
// another attempt.
func exportFailure(err error) string {
	if errors.Is(err, errExportTooLarge) {
		return "too_large"
	}
	var e *apiError
	if errors.As(err, &e) {
		switch e.Code {
		case "needs_raw_window", "range_too_large":
			return e.Code
		case "validation_failed", "not_found":
			return "invalid_query: " + e.Detail
		}
	}
	return ""
}

// RunAnalyticsExport writes an export's file to storage. A failure that another attempt cannot
// mend marks the export failed and returns nil; other errors are returned to be retried.
func (s *Server) RunAnalyticsExport(ctx context.Context, ws, id uuid.UUID) error {
	ex, err := s.st.GetAnalyticsExportForJob(ctx, store.GetAnalyticsExportForJobParams{WorkspaceID: ws, ID: id})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if n, err := s.st.StartAnalyticsExport(ctx, store.StartAnalyticsExportParams{WorkspaceID: ws, ID: id}); err != nil || n == 0 {
		return err
	}
	prop, err := s.st.GetAnalyticsProperty(ctx, store.GetAnalyticsPropertyParams{WorkspaceID: ws, ID: ex.PropertyID})
	if store.IsNotFound(err) {
		s.failExport(ctx, ws, id, "invalid_query: the property is gone")
		return nil
	}
	if err != nil {
		return err
	}
	f, err := os.CreateTemp("", "yuva-export-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	buf := bufio.NewWriterSize(f, 1<<16)
	w := &cappedWriter{w: buf, limit: maxAnalyticsExportBytes}
	rows, err := s.writeAnalyticsExport(ctx, prop, ex, w)
	if err == nil {
		err = buf.Flush()
	}
	if err != nil {
		if reason := exportFailure(err); reason != "" {
			s.failExport(ctx, ws, id, reason)
			return nil
		}
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	key := analyticsExportKeyPrefix + ws.String() + "/" + id.String() + "." + ex.Format
	contentType := "text/csv"
	if ex.Format == string(oas.AnalyticsExportNDJSON) {
		contentType = "application/x-ndjson"
	}
	if err := s.objects.Put(ctx, key, f, w.n, contentType); err != nil {
		return err
	}
	now := s.now()
	n, err := s.st.FinishAnalyticsExport(ctx, store.FinishAnalyticsExportParams{WorkspaceID: ws, ID: id, Rows: rows, SizeBytes: w.n,
		StorageKey: &key, Now: now, ExpiresAt: now.Add(analyticsExportTTL)})
	if err != nil {
		return err
	}
	if n == 0 {
		return s.objects.Delete(ctx, key)
	}
	return nil
}

func (s *Server) writeAnalyticsExport(ctx context.Context, prop store.AnalyticsProperty, ex store.AnalyticsExport, w io.Writer) (int64, error) {
	var params exportParams
	if err := json.Unmarshal(ex.Params, &params); err != nil {
		return 0, err
	}
	if ex.Kind == string(oas.AnalyticsExportRaw) {
		return s.writeRawExport(ctx, prop, ex, w)
	}
	q := oas.AnalyticsQuery{Range: oas.AnalyticsRange{From: &openapi_types.Date{Time: ex.FromDay}, To: &openapi_types.Date{Time: ex.ToDay}}}
	if ex.Kind == string(oas.AnalyticsExportView) {
		if params.Query == nil {
			return 0, errValidation("the export has no query")
		}
		q = *params.Query
	} else {
		dims := []oas.AnalyticsDimension{oas.AnalyticsByDay}
		if params.Dimension != "" {
			dims = append(dims, oas.AnalyticsDimension(params.Dimension))
		}
		q.Dimensions, q.Metrics = &dims, dailyExportMetrics
	}
	ar, err := s.analyticsRequest(ctx, prop, q)
	if err != nil {
		return 0, err
	}
	res, err := s.analyticsResult(ctx, prop, ar, true)
	if err != nil {
		return 0, err
	}
	if ex.Format == string(oas.AnalyticsExportCSV) {
		return int64(len(res.Rows)), writeAnalyticsCSV(w, ar.q.Dimensions, ar.q.Metrics, res.Rows)
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, r := range res.Rows {
		raw, _ := json.Marshal(r.Metrics)
		line := map[string]any{}
		_ = json.Unmarshal(raw, &line)
		for i, d := range ar.q.Dimensions {
			if i < len(r.Dimensions) {
				line[d] = r.Dimensions[i]
			}
		}
		if r.Country != nil {
			line["country"] = *r.Country
		}
		if r.Region != nil {
			line["region"] = *r.Region
		}
		if err := enc.Encode(line); err != nil {
			return 0, err
		}
	}
	return int64(len(res.Rows)), nil
}

type rawExportLine struct {
	Day   string `json:"day"`
	Visit int64  `json:"visit"`
	analytics.RawEvent
}

// writeRawExport writes a property's events day by day; a visit becomes a number counting up
// within the file, so nothing in it joins a visitor across files or to stored rows.
func (s *Server) writeRawExport(ctx context.Context, prop store.AnalyticsProperty, ex store.AnalyticsExport, w io.Writer) (int64, error) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	days := int(ex.ToDay.Sub(ex.FromDay).Hours()/24) + 1
	var rows, next int64
	for i := range days {
		day := ex.FromDay.AddDate(0, 0, i)
		visits := map[int64]int64{}
		err := s.analytics.store.RawEvents(ctx, prop.WorkspaceID, prop.ID, day, func(e analytics.RawEvent) error {
			n, ok := visits[e.Visit]
			if !ok {
				next++
				n = next
				visits[e.Visit] = n
			}
			rows++
			return enc.Encode(rawExportLine{Day: day.Format(time.DateOnly), Visit: n, RawEvent: e})
		})
		if err != nil {
			return rows, err
		}
		if _, err := s.st.ProgressAnalyticsExport(ctx, store.ProgressAnalyticsExportParams{WorkspaceID: prop.WorkspaceID, ID: ex.ID,
			Progress: int16((i + 1) * 99 / days), Rows: rows}); err != nil {
			return rows, err
		}
	}
	return rows, nil
}

// CleanAnalyticsExports deletes the exports past their expiry with their files.
func (s *Server) CleanAnalyticsExports(ctx context.Context) (int, error) {
	ids, err := s.st.ListAnalyticsWorkspaceIDs(ctx)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, ws := range ids {
		for {
			rows, err := s.st.ListExpiredAnalyticsExports(ctx, store.ListExpiredAnalyticsExportsParams{WorkspaceID: ws, Now: s.now()})
			if err != nil {
				return deleted, err
			}
			for _, r := range rows {
				if err := s.deleteExportFile(ctx, ws, r.ID, r.StorageKey); err != nil {
					return deleted, err
				}
				deleted++
			}
			if len(rows) < 500 {
				break
			}
		}
	}
	return deleted, nil
}

func (s *Server) deleteExportFile(ctx context.Context, ws, id uuid.UUID, key *string) error {
	if key != nil {
		if err := s.objects.Delete(ctx, *key); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
	}
	return s.st.DeleteAnalyticsExportRow(ctx, store.DeleteAnalyticsExportRowParams{WorkspaceID: ws, ID: id})
}

// deletePropertyExports removes a property's export files before its rows go with it.
func (s *Server) deletePropertyExports(ctx context.Context, ws, propertyID uuid.UUID) error {
	rows, err := s.st.ListPropertyAnalyticsExports(ctx, store.ListPropertyAnalyticsExportsParams{WorkspaceID: ws, PropertyID: propertyID})
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := s.deleteExportFile(ctx, ws, r.ID, r.StorageKey); err != nil {
			return err
		}
	}
	return nil
}

package api

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	monitorHeartbeat = string(oas.StatusMonitorHeartbeat)
	monitorPush      = string(oas.StatusMonitorPush)

	monitorPending = string(oas.MonitorPending)
	monitorUp      = string(oas.MonitorUp)
	monitorDown    = string(oas.MonitorDown)

	onFailureNone    = string(oas.StatusOnFailureNone)
	onFailureDraft   = string(oas.StatusOnFailureDraft)
	onFailurePublish = string(oas.StatusOnFailurePublish)

	statusEventMonitorDown = string(oas.StatusMonitorDownEvent)
	statusEventMonitorUp   = string(oas.StatusMonitorUpEvent)

	defaultMonitorInterval = 300
	defaultMonitorGrace    = 60
	statusChecksKept       = 30 * 24 * time.Hour
	monitorTokenPrefixLen  = 8
	maxMonitorMessageRunes = 1000

	heartbeatPathPrefix  = "/ingress/heartbeat/"
	statusPushPathPrefix = "/ingress/status/"
)

var (
	errStatusMonitorGone  = problem(http.StatusNotFound, "not_found", "no such monitor")
	errStatusMonitorLimit = problem(http.StatusForbidden, "status_monitor_limit", "this workspace has as many monitors as the server allows (YUVA_STATUS_MONITORS)")
	errMonitorKind        = problem(http.StatusConflict, "monitor_kind", "this token belongs to a monitor of another kind")

	limitMonitorPerToken   = limit{60, time.Minute}
	limitMonitorPerAddress = limit{600, time.Minute}
)

func (s *Server) monitorLimit() int { return s.status.Monitors }

func monitorBody(m store.StatusMonitor) oas.StatusMonitor {
	out := oas.StatusMonitor{
		Id: m.ID, PageId: m.PageID, ComponentId: m.ComponentID, Kind: oas.StatusMonitorKind(m.Kind), Name: m.Name,
		State: oas.StatusMonitorState(m.State), StateSince: m.ChangedAt, Paused: m.Paused, FailureState: oas.StatusMonitorFailureState(m.FailureState),
		OnFailure: oas.StatusMonitorOnFailure(m.OnFailure), FailureThreshold: m.FailureThreshold, RecoveryThreshold: m.RecoveryThreshold,
		Interval: m.IntervalSeconds, Grace: m.GraceSeconds, TokenPrefix: m.TokenPrefix, ConsecutiveFailures: m.ConsecutiveFailures,
		ConsecutiveSuccesses: m.ConsecutiveSuccesses, LastResultAt: m.LastResultAt, LastMessage: m.LastMessage, NextBeatDue: m.DueAt,
		IncidentId: m.IncidentID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	if m.DownState != nil {
		st := oas.StatusComponentStateValue(*m.DownState)
		out.DownState = &st
	}
	outsideBody(m, &out)
	return out
}

func (s *Server) monitorURL(kind, token string) string {
	if kind == monitorHeartbeat {
		return s.auth.PublicURL + heartbeatPathPrefix + token
	}
	return s.auth.PublicURL + statusPushPathPrefix + token
}

func newMonitorToken() (token, prefix string) {
	token = randomToken(24)
	return token, token[:monitorTokenPrefixLen]
}

func (s *Server) monitorWithToken(m store.StatusMonitor, token string) oas.StatusMonitorWithToken {
	b := monitorBody(m)
	out := oas.StatusMonitorWithToken{
		Id: b.Id, PageId: b.PageId, ComponentId: b.ComponentId, Kind: b.Kind, Name: b.Name, State: b.State, StateSince: b.StateSince,
		DownState: b.DownState, Paused: b.Paused, FailureState: b.FailureState, OnFailure: b.OnFailure, FailureThreshold: b.FailureThreshold,
		RecoveryThreshold: b.RecoveryThreshold, Interval: b.Interval, Grace: b.Grace, TokenPrefix: b.TokenPrefix,
		ConsecutiveFailures: b.ConsecutiveFailures, ConsecutiveSuccesses: b.ConsecutiveSuccesses, LastResultAt: b.LastResultAt,
		LastMessage: b.LastMessage, NextBeatDue: b.NextBeatDue, IncidentId: b.IncidentId, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
		Timeout: b.Timeout, Http: b.Http, Tcp: b.Tcp, NextCheckAt: b.NextCheckAt, CertExpiresAt: b.CertExpiresAt,
	}
	if token != "" {
		u := s.monitorURL(m.Kind, token)
		out.Token, out.Url = &token, &u
	}
	return out
}

func visibleStatusMonitor(ctx context.Context, q *store.Queries, pg store.StatusPage, id uuid.UUID) (store.StatusMonitor, error) {
	m, err := q.GetStatusMonitor(ctx, store.GetStatusMonitorParams{WorkspaceID: pg.WorkspaceID, PageID: pg.ID, ID: id})
	if store.IsNotFound(err) {
		return m, errStatusMonitorGone
	}
	return m, err
}

func lockedPageMonitor(ctx context.Context, q *store.Queries, pg store.StatusPage, id uuid.UUID) (store.StatusMonitor, error) {
	m, err := q.LockStatusMonitor(ctx, store.LockStatusMonitorParams{WorkspaceID: pg.WorkspaceID, ID: id})
	if store.IsNotFound(err) || (err == nil && m.PageID != pg.ID) {
		return m, errStatusMonitorGone
	}
	return m, err
}

func monitorComponent(ctx context.Context, q *store.Queries, pg store.StatusPage, id uuid.UUID) (store.StatusComponent, error) {
	c, err := q.GetStatusComponent(ctx, store.GetStatusComponentParams{WorkspaceID: pg.WorkspaceID, PageID: pg.ID, ID: id})
	if store.IsNotFound(err) {
		return c, errValidation("component_id names no component of this page")
	}
	return c, err
}

func monitorThreshold(v *int32, cur int32, field string) (int32, error) {
	if v == nil {
		return cur, nil
	}
	if *v < 1 || *v > 10 {
		return 0, errValidation(field + " must be 1 to 10")
	}
	return *v, nil
}

func validFailureState(v string) bool {
	return v == stDegraded || v == stPartialOutage || v == stMajorOutage
}

func validOnFailure(v string) bool {
	return v == onFailureNone || v == onFailureDraft || v == onFailurePublish
}

func heartbeatDue(now time.Time, interval, grace int32) *time.Time {
	t := now.Add(time.Duration(interval+grace) * time.Second)
	return &t
}

func (s *Server) ListStatusMonitors(ctx context.Context, req oas.ListStatusMonitorsRequestObject) (oas.ListStatusMonitorsResponseObject, error) {
	p := principalFrom(ctx)
	pg, err := visibleStatusPage(ctx, s.st.Queries, p, req.PageId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListStatusMonitors(ctx, store.ListStatusMonitorsParams{WorkspaceID: p.workspaceID, PageID: pg.ID})
	if err != nil {
		return nil, err
	}
	out := oas.ListStatusMonitors200JSONResponse{Items: make([]oas.StatusMonitor, len(rows))}
	for i, m := range rows {
		out.Items[i] = monitorBody(m)
	}
	return out, nil
}

func (s *Server) CreateStatusMonitor(ctx context.Context, req oas.CreateStatusMonitorRequestObject) (oas.CreateStatusMonitorResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	kind := string(b.Kind)
	if kind != monitorHeartbeat && kind != monitorPush && !outsideKind(kind) {
		return nil, errValidation("kind must be heartbeat, push, http or tcp")
	}
	name, err := trimmed(b.Name, 1, 100, "name")
	if err != nil {
		return nil, err
	}
	failState := stMajorOutage
	if b.FailureState != nil {
		if failState = string(*b.FailureState); !validFailureState(failState) {
			return nil, errValidation("failure_state must be degraded, partial_outage or major_outage")
		}
	}
	onFailure := onFailureDraft
	if b.OnFailure != nil {
		if onFailure = string(*b.OnFailure); !validOnFailure(onFailure) {
			return nil, errValidation("on_failure must be none, draft or publish")
		}
	}
	defFail, defRecover := int32(1), int32(1)
	if outsideKind(kind) {
		defFail, defRecover = defaultOutsideFailures, defaultOutsideRecovery
	}
	failN, err := monitorThreshold(b.FailureThreshold, defFail, "failure_threshold")
	if err != nil {
		return nil, err
	}
	recoverN, err := monitorThreshold(b.RecoveryThreshold, defRecover, "recovery_threshold")
	if err != nil {
		return nil, err
	}
	var (
		interval, grace *int32
		spec            outsideSpec
	)
	if outsideKind(kind) {
		if spec, err = s.newOutsideSpec(ctx, *b); err != nil {
			return nil, err
		}
		interval = &spec.interval
	} else if b.Http != nil || b.Tcp != nil || b.Timeout != nil {
		return nil, errValidation("only http and tcp monitors have http, tcp and timeout")
	} else if kind == monitorHeartbeat {
		iv, gr := int32(defaultMonitorInterval), int32(defaultMonitorGrace)
		if b.Interval != nil {
			iv = *b.Interval
		}
		if b.Grace != nil {
			gr = *b.Grace
		}
		if err := checkHeartbeatTiming(iv, gr); err != nil {
			return nil, err
		}
		interval, grace = &iv, &gr
	} else if b.Interval != nil || b.Grace != nil {
		return nil, errValidation("only a heartbeat monitor has interval and grace")
	}
	paused := b.Paused != nil && *b.Paused
	var (
		token, prefix string
		hash          []byte
		prefixPtr     *string
	)
	if !outsideKind(kind) {
		token, prefix = newMonitorToken()
		hash, prefixPtr = hashSecret(token), &prefix
	}
	var out oas.StatusMonitorWithToken
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		pg, err := visibleStatusPage(ctx, q, p, req.PageId, true)
		if err != nil {
			return err
		}
		if _, err := monitorComponent(ctx, q, pg, b.ComponentId); err != nil {
			return err
		}
		if lim := s.monitorLimit(); lim > 0 {
			n, err := q.CountStatusMonitors(ctx, p.workspaceID)
			if err != nil {
				return err
			}
			if n >= int64(lim) {
				return errStatusMonitorLimit
			}
		}
		now := s.now()
		var due *time.Time
		if kind == monitorHeartbeat && !paused {
			due = heartbeatDue(now, *interval, *grace)
		}
		id := newID()
		arg := store.CreateStatusMonitorParams{
			ID: id, WorkspaceID: p.workspaceID, PageID: pg.ID, ComponentID: b.ComponentId, Kind: kind, Name: name,
			TokenHash: hash, TokenPrefix: prefixPtr, IntervalSeconds: interval, GraceSeconds: grace, FailureState: failState,
			FailureThreshold: failN, RecoveryThreshold: recoverN, OnFailure: onFailure, Paused: paused, DueAt: due, Now: now,
			HttpHeaderNames: []string{},
		}
		if outsideKind(kind) {
			arg.Target, arg.TimeoutSeconds, arg.HttpMethod, arg.HttpBody = &spec.target, &spec.timeout, spec.method, spec.body
			arg.HttpHeaderNames, arg.HttpHeaders = spec.headerNames(), s.sealHeaders(p.workspaceID, id, spec.headers)
			arg.StatusMin, arg.StatusMax, arg.Keyword, arg.FollowRedirects, arg.CertWarnDays = spec.statusMin, spec.statusMax, spec.keyword, spec.follow, spec.certWarn
			if !paused {
				arg.CheckAt = &now
			}
		}
		m, err := q.CreateStatusMonitor(ctx, arg)
		if err != nil {
			return err
		}
		out = s.monitorWithToken(m, token)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateStatusMonitor201JSONResponse(out), nil
}

func checkHeartbeatTiming(interval, grace int32) error {
	if interval < 30 || interval > 2592000 {
		return errValidation("interval must be 30 seconds to 30 days")
	}
	if grace < 0 || grace > 86400 {
		return errValidation("grace must be 0 seconds to a day")
	}
	return nil
}

func (s *Server) GetStatusMonitor(ctx context.Context, req oas.GetStatusMonitorRequestObject) (oas.GetStatusMonitorResponseObject, error) {
	p := principalFrom(ctx)
	pg, err := visibleStatusPage(ctx, s.st.Queries, p, req.PageId, false)
	if err != nil {
		return nil, err
	}
	m, err := visibleStatusMonitor(ctx, s.st.Queries, pg, req.MonitorId)
	if err != nil {
		return nil, err
	}
	return oas.GetStatusMonitor200JSONResponse(monitorBody(m)), nil
}

func (s *Server) UpdateStatusMonitor(ctx context.Context, req oas.UpdateStatusMonitorRequestObject) (oas.UpdateStatusMonitorResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out oas.StatusMonitor
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		pg, err := visibleStatusPage(ctx, q, p, req.PageId, true)
		if err != nil {
			return err
		}
		m, err := lockedPageMonitor(ctx, q, pg, req.MonitorId)
		if err != nil {
			return err
		}
		now, oldComponent := s.now(), m.ComponentID
		arg := store.UpdateStatusMonitorSettingsParams{
			WorkspaceID: m.WorkspaceID, ID: m.ID, Name: m.Name, ComponentID: m.ComponentID, FailureState: m.FailureState,
			FailureThreshold: m.FailureThreshold, RecoveryThreshold: m.RecoveryThreshold, OnFailure: m.OnFailure,
			IntervalSeconds: m.IntervalSeconds, GraceSeconds: m.GraceSeconds, DueAt: m.DueAt, DueExtended: m.DueExtended, Now: now,
			Target: m.Target, TimeoutSeconds: m.TimeoutSeconds, HttpMethod: m.HttpMethod, HttpHeaderNames: m.HttpHeaderNames,
			HttpHeaders: m.HttpHeaders, HttpBody: m.HttpBody, StatusMin: m.StatusMin, StatusMax: m.StatusMax, Keyword: m.Keyword,
			FollowRedirects: m.FollowRedirects, CertWarnDays: m.CertWarnDays, CertExpiresAt: m.CertExpiresAt, CheckAt: m.CheckAt,
		}
		if arg.HttpHeaderNames == nil {
			arg.HttpHeaderNames = []string{}
		}
		if outsideKind(m.Kind) {
			spec, moved, err := s.updateOutsideSpec(ctx, m, *b)
			if err != nil {
				return err
			}
			arg.Target, arg.TimeoutSeconds, arg.IntervalSeconds, arg.HttpMethod, arg.HttpBody = &spec.target, &spec.timeout, &spec.interval, spec.method, spec.body
			arg.StatusMin, arg.StatusMax, arg.Keyword, arg.FollowRedirects, arg.CertWarnDays = spec.statusMin, spec.statusMax, spec.keyword, spec.follow, spec.certWarn
			if m.Kind == monitorHTTP {
				arg.HttpHeaderNames, arg.HttpHeaders = spec.headerNames(), s.sealHeaders(m.WorkspaceID, m.ID, spec.headers)
			}
			if moved {
				arg.CertExpiresAt = nil
				if !m.Paused {
					arg.CheckAt = &now
				}
			}
		} else if b.Http != nil || b.Tcp != nil || b.Timeout != nil {
			return errValidation("only http and tcp monitors have http, tcp and timeout")
		}
		if b.Name != nil {
			if arg.Name, err = trimmed(*b.Name, 1, 100, "name"); err != nil {
				return err
			}
		}
		if b.ComponentId != nil {
			if _, err := monitorComponent(ctx, q, pg, *b.ComponentId); err != nil {
				return err
			}
			arg.ComponentID = *b.ComponentId
		}
		if b.FailureState != nil {
			if arg.FailureState = string(*b.FailureState); !validFailureState(arg.FailureState) {
				return errValidation("failure_state must be degraded, partial_outage or major_outage")
			}
		}
		if b.OnFailure != nil {
			if arg.OnFailure = string(*b.OnFailure); !validOnFailure(arg.OnFailure) {
				return errValidation("on_failure must be none, draft or publish")
			}
		}
		if arg.FailureThreshold, err = monitorThreshold(b.FailureThreshold, m.FailureThreshold, "failure_threshold"); err != nil {
			return err
		}
		if arg.RecoveryThreshold, err = monitorThreshold(b.RecoveryThreshold, m.RecoveryThreshold, "recovery_threshold"); err != nil {
			return err
		}
		if (b.Interval != nil || b.Grace != nil) && !outsideKind(m.Kind) {
			if m.Kind != monitorHeartbeat {
				return errValidation("only a heartbeat monitor has interval and grace")
			}
			iv, gr := *m.IntervalSeconds, *m.GraceSeconds
			if b.Interval != nil {
				iv = *b.Interval
			}
			if b.Grace != nil {
				gr = *b.Grace
			}
			if err := checkHeartbeatTiming(iv, gr); err != nil {
				return err
			}
			arg.IntervalSeconds, arg.GraceSeconds = &iv, &gr
			if m.DueAt != nil {
				arg.DueAt, arg.DueExtended = heartbeatDue(now, iv, gr), false
			}
		}
		if m, err = q.UpdateStatusMonitorSettings(ctx, arg); err != nil {
			return err
		}
		touched := []uuid.UUID{oldComponent, m.ComponentID}
		if b.Paused != nil && *b.Paused != m.Paused {
			held := m
			pause := store.SetStatusMonitorPausedParams{WorkspaceID: m.WorkspaceID, ID: m.ID, Paused: *b.Paused, State: monitorPending, ChangedAt: now, Now: now}
			if !*b.Paused && m.Kind == monitorHeartbeat {
				pause.DueAt = heartbeatDue(now, *m.IntervalSeconds, *m.GraceSeconds)
			}
			if !*b.Paused && outsideKind(m.Kind) {
				pause.CheckAt = &now
			}
			if m, err = q.SetStatusMonitorPaused(ctx, pause); err != nil {
				return err
			}
			if *b.Paused {
				if err := q.DeletePendingStatusChecks(ctx, store.DeletePendingStatusChecksParams{WorkspaceID: m.WorkspaceID, MonitorID: m.ID}); err != nil {
					return err
				}
			}
			if *b.Paused && held.IncidentID != nil {
				if _, err := s.resolveMonitorIncident(ctx, q, ev, pg, held, *held.IncidentID, monitorEndPaused, now); err != nil {
					return err
				}
			}
		}
		if err := s.settleComponents(ctx, q, ev, pg, touched, nil, statusActor{statusSourceMonitor, &m.ID}, now); err != nil {
			return err
		}
		out = monitorBody(m)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateStatusMonitor200JSONResponse(out), nil
}

func (s *Server) DeleteStatusMonitor(ctx context.Context, req oas.DeleteStatusMonitorRequestObject) (oas.DeleteStatusMonitorResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		pg, err := visibleStatusPage(ctx, q, p, req.PageId, true)
		if err != nil {
			return err
		}
		m, err := lockedPageMonitor(ctx, q, pg, req.MonitorId)
		if err != nil {
			return err
		}
		if _, err := q.DeleteStatusMonitor(ctx, store.DeleteStatusMonitorParams{WorkspaceID: m.WorkspaceID, PageID: pg.ID, ID: m.ID}); err != nil {
			return err
		}
		now := s.now()
		if m.IncidentID != nil {
			if _, err := s.resolveMonitorIncident(ctx, q, ev, pg, m, *m.IncidentID, monitorEndRemoved, now); err != nil {
				return err
			}
		}
		return s.settleComponents(ctx, q, ev, pg, []uuid.UUID{m.ComponentID}, nil, statusActor{statusSourceMonitor, &m.ID}, now)
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteStatusMonitor204Response{}, nil
}

func (s *Server) RotateStatusMonitorToken(ctx context.Context, req oas.RotateStatusMonitorTokenRequestObject) (oas.RotateStatusMonitorTokenResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	token, prefix := newMonitorToken()
	var out oas.StatusMonitorWithToken
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		pg, err := visibleStatusPage(ctx, q, p, req.PageId, true)
		if err != nil {
			return err
		}
		m, err := lockedPageMonitor(ctx, q, pg, req.MonitorId)
		if err != nil {
			return err
		}
		if outsideKind(m.Kind) {
			return errNoMonitorToken
		}
		if m, err = q.SetStatusMonitorToken(ctx, store.SetStatusMonitorTokenParams{
			WorkspaceID: m.WorkspaceID, ID: m.ID, TokenHash: hashSecret(token), TokenPrefix: &prefix, Now: s.now(),
		}); err != nil {
			return err
		}
		out = s.monitorWithToken(m, token)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.RotateStatusMonitorToken200JSONResponse(out), nil
}

func (s *Server) ListStatusMonitorChecks(ctx context.Context, req oas.ListStatusMonitorChecksRequestObject) (oas.ListStatusMonitorChecksResponseObject, error) {
	p := principalFrom(ctx)
	pg, err := visibleStatusPage(ctx, s.st.Queries, p, req.PageId, false)
	if err != nil {
		return nil, err
	}
	m, err := visibleStatusMonitor(ctx, s.st.Queries, pg, req.MonitorId)
	if err != nil {
		return nil, err
	}
	limit, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListStatusChecks(ctx, store.ListStatusChecksParams{WorkspaceID: p.workspaceID, MonitorID: m.ID, CursorAt: at, CursorID: id, Lim: limit + 1})
	if err != nil {
		return nil, err
	}
	out := oas.ListStatusMonitorChecks200JSONResponse{Items: []oas.StatusCheck{}}
	if len(rows) > int(limit) {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		c := encodeCursor(last.At, last.ID)
		out.NextCursor = &c
	}
	for _, c := range rows {
		item := oas.StatusCheck{Id: c.ID, At: c.At, Ok: c.Ok, Source: oas.StatusCheckSource(c.Source), Message: c.Message, Error: c.Error,
			DurationMs: c.DurationMs, StatusCode: c.StatusCode}
		if c.State != nil {
			st := oas.StatusComponentStateValue(*c.State)
			item.State = &st
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// monitorResult is one beat, missed beat or pushed state. A failure sets state, or the monitor's
// failure state when state is empty.
type monitorResult struct {
	ok      bool
	source  string
	state   string
	message string
	errText string
	// check is the pending check an outside check recorded, which counts now.
	check *uuid.UUID
}

func messagePtr(v string) *string {
	if v == "" {
		return nil
	}
	v = truncateRunes(v, maxMonitorMessageRunes)
	return &v
}

// takeMonitorResult records a result for a monitor found by its token and moves the monitor, its
// component and its incident when a threshold is reached. A paused monitor counts nothing.
func (s *Server) takeMonitorResult(ctx context.Context, ws, id uuid.UUID, r monitorResult) (store.StatusMonitor, error) {
	var out store.StatusMonitor
	err := s.inTx(ctx, ws, func(q *store.Queries, ev *eventBatch) error {
		pg, m, err := lockMonitorWithPage(ctx, q, ws, id)
		if err != nil {
			return err
		}
		if m.Paused {
			out = m
			if r.check != nil {
				return q.DeleteStatusCheck(ctx, store.DeleteStatusCheckParams{WorkspaceID: ws, ID: *r.check})
			}
			return nil
		}
		out, err = s.applyMonitorResult(ctx, q, ev, pg, m, r, s.now())
		return err
	})
	return out, err
}

// lockMonitorWithPage locks the page before the monitor, in the order the member endpoints use.
func lockMonitorWithPage(ctx context.Context, q *store.Queries, ws, id uuid.UUID) (store.StatusPage, store.StatusMonitor, error) {
	cur, err := q.GetStatusMonitorByID(ctx, store.GetStatusMonitorByIDParams{WorkspaceID: ws, ID: id})
	if store.IsNotFound(err) {
		return store.StatusPage{}, cur, errStatusMonitorGone
	}
	if err != nil {
		return store.StatusPage{}, cur, err
	}
	pg, err := q.LockStatusPage(ctx, store.LockStatusPageParams{WorkspaceID: ws, ID: cur.PageID})
	if store.IsNotFound(err) {
		return pg, cur, errStatusMonitorGone
	}
	if err != nil {
		return pg, cur, err
	}
	m, err := lockedPageMonitor(ctx, q, pg, id)
	return pg, m, err
}

func (s *Server) applyMonitorResult(ctx context.Context, q *store.Queries, ev *eventBatch, pg store.StatusPage, m store.StatusMonitor,
	r monitorResult, now time.Time) (store.StatusMonitor, error) {
	claim := r.state
	if claim == "" {
		claim = m.FailureState
	}
	check := store.CreateStatusCheckParams{ID: newID(), WorkspaceID: m.WorkspaceID, MonitorID: m.ID, At: now, Ok: r.ok, Source: r.source,
		Message: messagePtr(r.message), Error: messagePtr(r.errText)}
	if !r.ok {
		check.State = &claim
	}
	if r.check != nil {
		n, err := q.FinishStatusCheck(ctx, store.FinishStatusCheckParams{WorkspaceID: m.WorkspaceID, ID: *r.check, State: check.State})
		if err != nil || n == 0 {
			return m, err
		}
	} else if err := q.CreateStatusCheck(ctx, check); err != nil {
		return m, err
	}
	fails, succ := m.ConsecutiveFailures, m.ConsecutiveSuccesses
	if r.ok {
		fails, succ = 0, min(succ+1, 1000)
	} else {
		fails, succ = min(fails+1, 1000), 0
	}
	msg := r.message
	if msg == "" && !r.ok {
		msg = r.errText
	}
	arg := store.SetStatusMonitorResultParams{
		WorkspaceID: m.WorkspaceID, ID: m.ID, State: m.State, DownState: m.DownState, ConsecutiveFailures: fails, ConsecutiveSuccesses: succ,
		At: now, LastMessage: messagePtr(msg), DueAt: m.DueAt, DueExtended: m.DueExtended, IncidentID: m.IncidentID, ChangedAt: m.ChangedAt,
	}
	if m.Kind == monitorHeartbeat {
		if r.source == string(oas.StatusCheckMissed) {
			next := m.DueAt.Add(time.Duration(*m.IntervalSeconds) * time.Second)
			arg.DueAt, arg.DueExtended = &next, false
		} else {
			arg.DueAt, arg.DueExtended = heartbeatDue(now, *m.IntervalSeconds, *m.GraceSeconds), false
		}
	}
	var change string
	switch {
	case !r.ok && m.State != monitorDown && fails >= m.FailureThreshold:
		change = monitorDown
		arg.State, arg.DownState, arg.ChangedAt = monitorDown, &claim, now
	case !r.ok && m.State == monitorDown && claim != *m.DownState:
		arg.DownState = &claim
		change = "worse"
	case r.ok && m.State == monitorDown && succ >= m.RecoveryThreshold:
		change = monitorUp
		arg.State, arg.DownState, arg.ChangedAt, arg.IncidentID = monitorUp, nil, now, nil
	case r.ok && m.State == monitorPending:
		arg.State = monitorUp
	}
	if arg.State == monitorDown {
		arg.DueAt, arg.DueExtended = nil, false
	}
	var incident *uuid.UUID
	if change == monitorDown && m.OnFailure != onFailureNone {
		id, err := s.monitorIncident(ctx, q, ev, pg, m, claim, now)
		if err != nil {
			return m, err
		}
		arg.IncidentID, incident = id, id
	}
	next, err := q.SetStatusMonitorResult(ctx, arg)
	if err != nil {
		return m, err
	}
	if change == monitorUp && m.IncidentID != nil {
		if incident, err = s.resolveMonitorIncident(ctx, q, ev, pg, m, *m.IncidentID, monitorEndRecovered, now); err != nil {
			return next, err
		}
	}
	if change == "" {
		return next, nil
	}
	if change == "worse" && m.IncidentID != nil {
		if err := s.matchMonitorIncident(ctx, q, ev, pg, m, claim, now); err != nil {
			return next, err
		}
	}
	if err := s.settleComponents(ctx, q, ev, pg, []uuid.UUID{m.ComponentID}, nil, statusActor{statusSourceMonitor, &m.ID}, now); err != nil {
		return next, err
	}
	if change == "worse" {
		return next, nil
	}
	typ := statusEventMonitorDown
	if change == monitorUp {
		typ = statusEventMonitorUp
	}
	inbox := pg.InboxID
	ev.add(typ, &inbox, nil, oas.StatusMonitorEventData{Monitor: monitorBody(next), Page: statusPageRef(pg), IncidentId: incident})
	notify := StatusMonitorNotifyArgs{WorkspaceID: m.WorkspaceID, MonitorID: m.ID, Down: change == monitorDown, Message: truncateRunes(msg, maxMonitorMessageRunes), At: now}
	if notify.Down {
		notify.State = claim
	}
	ev.job(notify, nil)
	return next, nil
}

func monitorImpact(state string) string {
	if state == stMajorOutage {
		return string(oas.StatusImpactMajor)
	}
	return string(oas.StatusImpactMinor)
}

// monitorIncident returns the open incident the monitor opened before, or opens one as a draft or
// published per on_failure, touching the monitor's component with the failing state.
func (s *Server) monitorIncident(ctx context.Context, q *store.Queries, ev *eventBatch, pg store.StatusPage, m store.StatusMonitor,
	state string, now time.Time) (*uuid.UUID, error) {
	if m.IncidentID != nil {
		inc, err := lockedStatusIncident(ctx, q, pg, *m.IncidentID)
		if err == nil && !incidentClosed(inc.State) {
			return &inc.ID, nil
		}
		if err != nil && err != errStatusIncidentGone {
			return nil, err
		}
	}
	c, err := q.GetStatusComponent(ctx, store.GetStatusComponentParams{WorkspaceID: pg.WorkspaceID, PageID: pg.ID, ID: m.ComponentID})
	if err != nil {
		return nil, err
	}
	words := statusWordsFor(pg.Locale)
	arg := store.CreateStatusIncidentParams{
		ID: newID(), WorkspaceID: pg.WorkspaceID, PageID: pg.ID, Kind: kindIncident, Title: truncateRunes(fmt.Sprintf(words.MonitorTitle, c.Name), 200),
		Impact: monitorImpact(state), State: incInvestigating, Notify: true, Now: now,
	}
	if m.OnFailure == onFailurePublish {
		arg.PublishedAt = &now
	}
	inc, err := q.CreateStatusIncident(ctx, arg)
	if err != nil {
		return nil, err
	}
	if err := writeIncidentComponents(ctx, q, inc, []store.StatusIncidentComponent{{WorkspaceID: pg.WorkspaceID, ComponentID: c.ID, State: state}}); err != nil {
		return nil, err
	}
	up, err := q.CreateStatusIncidentUpdate(ctx, store.CreateStatusIncidentUpdateParams{
		ID: newID(), WorkspaceID: pg.WorkspaceID, IncidentID: inc.ID, State: incInvestigating, Body: fmt.Sprintf(words.MonitorBody, c.Name), Notify: true, Now: now,
	})
	if err != nil {
		return nil, err
	}
	if err := s.queueNotice(ctx, q, ev, pg, inc, []uuid.UUID{up.ID}, false); err != nil {
		return nil, err
	}
	if _, err := s.incidentEvent(ctx, q, ev, pg, inc, oas.StatusChangeCreated, &up); err != nil {
		return nil, err
	}
	return &inc.ID, nil
}

const (
	monitorEndRecovered = iota
	monitorEndPaused
	monitorEndRemoved
)

// matchMonitorIncident gives the incident the monitor opened the impact and component state the
// monitor claims now, whether worse or better than before.
func (s *Server) matchMonitorIncident(ctx context.Context, q *store.Queries, ev *eventBatch, pg store.StatusPage, m store.StatusMonitor,
	state string, now time.Time) error {
	inc, err := lockedStatusIncident(ctx, q, pg, *m.IncidentID)
	if err == errStatusIncidentGone || (err == nil && incidentClosed(inc.State)) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := q.SetStatusIncidentComponentState(ctx, store.SetStatusIncidentComponentStateParams{
		WorkspaceID: pg.WorkspaceID, IncidentID: inc.ID, ComponentID: m.ComponentID, State: state,
	}); err != nil {
		return err
	}
	arg := incidentUpdateArg(inc, now)
	arg.Impact = monitorImpact(state)
	if inc, err = q.UpdateStatusIncident(ctx, arg); err != nil {
		return err
	}
	_, err = s.incidentEvent(ctx, q, ev, pg, inc, oas.StatusChangeChanged, nil)
	return err
}

// resolveMonitorIncident resolves the incident the monitor opened unless a member closed it first. It
// puts back only the components an incident or monitor set, so a member's own state stays.
func (s *Server) resolveMonitorIncident(ctx context.Context, q *store.Queries, ev *eventBatch, pg store.StatusPage, m store.StatusMonitor,
	id uuid.UUID, end int, now time.Time) (*uuid.UUID, error) {
	inc, err := lockedStatusIncident(ctx, q, pg, id)
	if err == errStatusIncidentGone {
		return nil, nil
	}
	if err != nil || incidentClosed(inc.State) {
		return nil, err
	}
	name := m.Name
	if c, err := q.GetStatusComponent(ctx, store.GetStatusComponentParams{WorkspaceID: pg.WorkspaceID, PageID: pg.ID, ID: m.ComponentID}); err == nil {
		name = c.Name
	}
	words := statusWordsFor(pg.Locale)
	format := words.MonitorResolved
	switch end {
	case monitorEndPaused:
		format = words.MonitorPaused
	case monitorEndRemoved:
		format = words.MonitorRemoved
	}
	body := fmt.Sprintf(format, name)
	if _, err := s.moveIncidentWith(ctx, q, ev, pg, inc, incResolved, body, nil, nil, oas.StatusChangeResolved, false, true, now); err != nil {
		return nil, err
	}
	return &inc.ID, nil
}

func (s *Server) serveHeartbeat(fail bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f := fail
		switch r.URL.Query().Get("fail") {
		case "1", "true":
			f = true
		}
		res := monitorResult{ok: !f, source: string(oas.StatusCheckBeat)}
		if f {
			res.source, res.errText = string(oas.StatusCheckFail), "the job reported a failure"
		}
		s.serveMonitorIngress(w, r, monitorHeartbeat, func() (monitorResult, bool, error) { return res, true, nil })
	}
}

func (s *Server) serveStatusPush(w http.ResponseWriter, r *http.Request) {
	s.serveMonitorIngress(w, r, monitorPush, func() (monitorResult, bool, error) {
		body, err := readLimited(r, maxStatusPushBytes)
		if err != nil {
			return monitorResult{}, false, err
		}
		return parseStatusPush(body)
	})
}

// serveMonitorIngress limits calls per client address, finds the monitor by its token, limits calls
// per monitor and takes the result read reports; take false answers with the monitor's state and
// changes nothing. Only an address, never a token the caller made up, is counted before the lookup.
func (s *Server) serveMonitorIngress(w http.ResponseWriter, r *http.Request, kind string, read func() (monitorResult, bool, error)) {
	token := r.PathValue("token")
	if token == "" || len(token) > 100 {
		writeProblem(w, errStatusMonitorGone)
		return
	}
	ctx := r.Context()
	if err := s.rateLimit(ctx, rateCheck{"monitor:ip:" + rateIP(s.clientIP(r)), limitMonitorPerAddress}); err != nil {
		s.writeError(w, r, err)
		return
	}
	hash := hashSecret(token)
	row, err := s.st.FindStatusMonitorByToken(ctx, hash)
	if store.IsNotFound(err) {
		writeProblem(w, errStatusMonitorGone)
		return
	}
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if err := s.rateLimit(ctx, rateCheck{"monitor:" + hex.EncodeToString(hash[:16]), limitMonitorPerToken}); err != nil {
		s.writeError(w, r, err)
		return
	}
	if live, err := s.workspaceLive(ctx, row.WorkspaceID); err != nil || !live {
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		writeProblem(w, errStatusMonitorGone)
		return
	}
	if row.Kind != kind {
		writeProblem(w, errMonitorKind)
		return
	}
	res, take, err := read()
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var m store.StatusMonitor
	if take {
		m, err = s.takeMonitorResult(ctx, row.WorkspaceID, row.ID, res)
	} else {
		m, err = s.st.GetStatusMonitorByID(ctx, store.GetStatusMonitorByIDParams{WorkspaceID: row.WorkspaceID, ID: row.ID})
		if store.IsNotFound(err) {
			err = errStatusMonitorGone
		}
	}
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, oas.StatusIngressResult{State: oas.StatusMonitorState(m.State), Paused: m.Paused})
}

type statusMonitorSweepWorker struct {
	river.WorkerDefaults[jobs.StatusMonitorSweepArgs]
	s *Server
}

func (w *statusMonitorSweepWorker) Work(ctx context.Context, _ *river.Job[jobs.StatusMonitorSweepArgs]) error {
	return errors.Join(w.s.SweepStatusMonitors(ctx), w.s.ScheduleStatusChecks(ctx))
}

type statusChecksCleanupWorker struct {
	river.WorkerDefaults[jobs.StatusChecksCleanupArgs]
	s *Server
}

func (w *statusChecksCleanupWorker) Work(ctx context.Context, _ *river.Job[jobs.StatusChecksCleanupArgs]) error {
	return w.s.CleanStatusChecks(ctx)
}

// SweepStatusMonitors counts a failure for every heartbeat whose beat is overdue. A monitor that
// fails to be checked is logged and the others still are; the errors come back joined.
func (s *Server) SweepStatusMonitors(ctx context.Context) error {
	due, err := s.st.ListDueStatusMonitors(ctx, s.now())
	if err != nil {
		return err
	}
	var errs []error
	for _, d := range due {
		if err := s.missedBeat(ctx, d.WorkspaceID, d.ID); err != nil && err != errStatusMonitorGone {
			s.log.ErrorContext(ctx, "status monitor sweep", slog.String("workspace_id", d.WorkspaceID.String()),
				slog.String("monitor_id", d.ID.String()), slog.Any("error", err))
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// missedBeat fails a heartbeat whose beat did not come in time. When this server started after the
// beat was expected, the beat may have been sent while it was not taking requests, so the heartbeat
// gets one more interval first.
func (s *Server) missedBeat(ctx context.Context, ws, id uuid.UUID) error {
	return s.inTx(ctx, ws, func(q *store.Queries, ev *eventBatch) error {
		pg, m, err := lockMonitorWithPage(ctx, q, ws, id)
		if err != nil {
			return err
		}
		now := s.now()
		if m.Kind != monitorHeartbeat || m.Paused || m.DueAt == nil || m.DueAt.After(now) {
			return nil
		}
		interval := time.Duration(*m.IntervalSeconds) * time.Second
		grace := time.Duration(*m.GraceSeconds) * time.Second
		if !m.DueExtended && s.started.After(m.DueAt.Add(-grace)) {
			return q.ExtendStatusMonitorDue(ctx, store.ExtendStatusMonitorDueParams{WorkspaceID: ws, ID: m.ID, DueAt: maxTime(*m.DueAt, s.started).Add(interval)})
		}
		_, err = s.applyMonitorResult(ctx, q, ev, pg, m, monitorResult{
			source: string(oas.StatusCheckMissed), errText: "no beat for " + strconv.Itoa(int((interval+grace)/time.Second)) + " seconds",
		}, now)
		return err
	})
}

// CleanStatusChecks deletes checks older than 30 days, and pending ones whose round was never judged.
func (s *Server) CleanStatusChecks(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	before, stale := now.Add(-statusChecksKept), now.Add(-staleCheckAge)
	for _, ws := range ids {
		for {
			n, err := s.st.DeleteOldStatusChecks(ctx, store.DeleteOldStatusChecksParams{WorkspaceID: ws, Before: before, StaleBefore: stale})
			if err != nil {
				return err
			}
			if n < 10000 {
				break
			}
		}
	}
	return nil
}

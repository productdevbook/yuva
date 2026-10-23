package api

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxViews         = 50
	maxViewFilters   = 30
	maxViewValue     = 2000
	viewCountCap     = 1000
	viewCountWorkers = 4
	viewCountTimeout = time.Second
	viewPersonal     = string(oas.SavedViewPersonal)
	viewTeam         = string(oas.SavedViewTeam)
	viewStatusOpen   = string(oas.ConversationStatusOpen)
	viewParamLabelID = "label_id"
	viewParamTeamID  = "team_id"
)

var (
	errViewGone        = problem(http.StatusNotFound, "not_found", "no such view")
	errViewNeedsMember = problem(http.StatusForbidden, "member_session_required", "a personal view belongs to a member; an API key manages workspace and team views only")
	errViewLimit       = problem(http.StatusForbidden, "view_limit", "at most 50 personal views per member, 50 workspace views and 50 views per team")
)

func errViewFilter(param, detail string) *apiError {
	e := problem(http.StatusBadRequest, "validation_failed", detail)
	e.Field = param
	return e
}

// listFilterNames are the query parameters of ListConversations, read from the generated binding
// so that a parameter added to the list is a filter of saved views too.
var listFilterNames = sync.OnceValue(func() map[string]bool {
	out := map[string]bool{}
	t := reflect.TypeFor[oas.ListConversationsParams]()
	for i := range t.NumField() {
		if tag, ok := t.Field(i).Tag.Lookup("form"); ok {
			out[strings.Split(tag, ",")[0]] = true
		}
	}
	return out
})

type listParamsCapture struct {
	oas.ServerInterface
	got *oas.ListConversationsParams
}

func (c listParamsCapture) ListConversations(_ http.ResponseWriter, _ *http.Request, prm oas.ListConversationsParams) {
	*c.got = prm
}

// bindListParams reads query parameters with the generated binding of ListConversations.
func bindListParams(vals url.Values) (oas.ListConversationsParams, error) {
	var got oas.ListConversationsParams
	var bindErr error
	w := oas.ServerInterfaceWrapper{
		Handler:          listParamsCapture{got: &got},
		ErrorHandlerFunc: func(_ http.ResponseWriter, _ *http.Request, err error) { bindErr = err },
	}
	w.ListConversations(nil, &http.Request{URL: &url.URL{RawQuery: vals.Encode()}, Header: http.Header{}})
	if bindErr != nil {
		var bad *oas.InvalidParamFormatError
		if errors.As(bindErr, &bad) {
			return got, errViewFilter(bad.ParamName, bad.ParamName+" is not valid: "+bad.Err.Error())
		}
		return got, errValidation(bindErr.Error())
	}
	return got, nil
}

// cleanViewFilters checks the names and sizes of filters and writes ids in their canonical form.
func cleanViewFilters(in map[string]string) (map[string]string, error) {
	if len(in) > maxViewFilters {
		return nil, errValidation("filters holds at most 30 parameters")
	}
	out := make(map[string]string, len(in))
	for _, k := range slices.Sorted(maps.Keys(in)) {
		v := in[k]
		if len([]rune(v)) > maxViewValue {
			return nil, errViewFilter(k, k+" must be at most 2000 characters")
		}
		if key, ok := strings.CutPrefix(k, fieldFilterPrefix); ok && key != "" {
			out[k] = v
			continue
		}
		switch {
		case k == "cursor" || k == "limit":
			return nil, errViewFilter(k, k+" pages the list; it is not a filter")
		case !listFilterNames()[k]:
			return nil, errViewFilter(k, k+" is not a parameter of the conversation list")
		}
		if strings.HasSuffix(k, "_id") {
			if id, err := uuid.Parse(strings.TrimSpace(v)); err == nil {
				v = id.String()
			}
		}
		out[k] = v
	}
	return out, nil
}

// viewFilter turns a view's filters into the list's query for p, with the list's own parser.
func (s *Server) viewFilter(ctx context.Context, q *store.Queries, p principal, filters map[string]string) (store.ListConversationsParams, error) {
	vals := url.Values{}
	fields := map[string][]string{}
	for k, v := range filters {
		if key, ok := strings.CutPrefix(k, fieldFilterPrefix); ok {
			fields[key] = []string{v}
			continue
		}
		vals.Set(k, v)
	}
	prm, err := bindListParams(vals)
	if err != nil {
		return store.ListConversationsParams{}, err
	}
	arg, err := s.conversationFilter(ctx, q, p, prm, fields)
	var bad *listFilterError
	if errors.As(err, &bad) {
		detail := bad.err.Error()
		var e *apiError
		if errors.As(bad.err, &e) {
			detail = e.Detail
		}
		return arg, errViewFilter(bad.param, detail)
	}
	return arg, err
}

// checkViewFilters cleans and parses filters for the writer, holding the labels, teams and fields
// they name until the transaction ends, so that a deletion strips them either before or after.
func (s *Server) checkViewFilters(ctx context.Context, q *store.Queries, p principal, in map[string]string) ([]byte, error) {
	filters, err := cleanViewFilters(in)
	if err != nil {
		return nil, err
	}
	if v, ok := filters[viewParamLabelID]; ok {
		if id, err := uuid.Parse(v); err == nil {
			found, err := q.LockLabels(ctx, store.LockLabelsParams{WorkspaceID: p.workspaceID, Ids: []uuid.UUID{id}})
			if err != nil {
				return nil, err
			}
			if len(found) == 0 {
				return nil, errViewFilter(viewParamLabelID, "no such label")
			}
		}
	}
	if v, ok := filters[viewParamTeamID]; ok {
		if id, err := uuid.Parse(v); err == nil {
			found, err := q.ShareLockTeams(ctx, store.ShareLockTeamsParams{WorkspaceID: p.workspaceID, Ids: []uuid.UUID{id}})
			if err != nil {
				return nil, err
			}
			if len(found) == 0 {
				return nil, errViewFilter(viewParamTeamID, "no such team")
			}
		}
	}
	if slices.ContainsFunc(slices.Collect(maps.Keys(filters)), func(k string) bool { return strings.HasPrefix(k, fieldFilterPrefix) }) {
		if err := q.ShareLockCustomFields(ctx, p.workspaceID); err != nil {
			return nil, err
		}
	}
	if _, err := s.viewFilter(ctx, q, p, filters); err != nil {
		return nil, err
	}
	return mustJSON(filters), nil
}

func viewFilters(v store.SavedView) map[string]string {
	out := map[string]string{}
	_ = json.Unmarshal(v.Filters, &out)
	return out
}

func savedViewBody(v store.SavedView) oas.SavedView {
	return oas.SavedView{
		Id: v.ID, Visibility: oas.SavedViewVisibility(v.Visibility), OwnerMemberId: v.OwnerMemberID, TeamId: v.TeamID,
		Name: v.Name, Filters: viewFilters(v), Position: v.Position, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

type fieldDefsKey struct{}

// filterFieldDefs are the conversation field definitions for field filters, loaded once for all the
// views of a request when it put them in ctx.
func filterFieldDefs(ctx context.Context, q *store.Queries, workspaceID uuid.UUID) ([]store.CustomField, error) {
	if defs, ok := ctx.Value(fieldDefsKey{}).([]store.CustomField); ok {
		return defs, nil
	}
	return conversationFieldDefs(ctx, q, workspaceID)
}

func hasFieldFilter(v oas.SavedView) bool {
	return slices.ContainsFunc(slices.Collect(maps.Keys(v.Filters)), func(k string) bool { return strings.HasPrefix(k, fieldFilterPrefix) })
}

// withViewCounts counts each view's open conversations under p's own access, a few at a time, each
// count stopped after viewCountTimeout (count_unavailable). Filters that do not apply to p (an inbox
// p cannot see, assignee=me for a key) leave the count out.
func (s *Server) withViewCounts(ctx context.Context, p principal, rows []store.SavedView) ([]oas.SavedView, error) {
	out := make([]oas.SavedView, len(rows))
	for i, r := range rows {
		out[i] = savedViewBody(r)
	}
	if slices.ContainsFunc(out, hasFieldFilter) {
		defs, err := conversationFieldDefs(ctx, s.st.Queries, p.workspaceID)
		if err != nil {
			return nil, err
		}
		ctx = context.WithValue(ctx, fieldDefsKey{}, defs)
	}
	errs := make([]error, len(out))
	sem := make(chan struct{}, viewCountWorkers)
	var wg sync.WaitGroup
	for i := range out {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			errs[i] = s.countView(ctx, p, &out[i])
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) countView(ctx context.Context, p principal, v *oas.SavedView) error {
	arg, err := s.viewFilter(ctx, s.st.Queries, p, v.Filters)
	if err != nil {
		var e *apiError
		if errors.As(err, &e) {
			return nil
		}
		return err
	}
	var n int64
	if arg.Statuses == nil || slices.Contains(arg.Statuses, viewStatusOpen) {
		arg.Statuses, arg.Lim = []string{viewStatusOpen}, viewCountCap+1
		var timedOut bool
		if n, timedOut, err = s.st.CountListConversationsWithin(ctx, arg, viewCountTimeout); err != nil {
			return err
		}
		if timedOut {
			v.CountUnavailable = new(true)
			return nil
		}
	}
	v.Count = new(int32(min(n, viewCountCap)))
	v.CountCapped = new(n > viewCountCap)
	return nil
}

func (s *Server) withViewCount(ctx context.Context, p principal, v store.SavedView) (oas.SavedView, error) {
	out, err := s.withViewCounts(ctx, p, []store.SavedView{v})
	if err != nil {
		return oas.SavedView{}, err
	}
	return out[0], nil
}

// seesAllTeamViews: owners, admins and keys not limited to inboxes see and manage the views of
// every team; others only those of their teams.
func seesAllTeamViews(p principal) bool {
	if p.isKey() {
		return !p.limited
	}
	return p.role == roleOwner || p.role == roleAdmin
}

func visibleViews(ctx context.Context, q *store.Queries, p principal, ids []uuid.UUID) ([]store.SavedView, error) {
	return q.VisibleSavedViews(ctx, store.VisibleSavedViewsParams{WorkspaceID: p.workspaceID, Ids: ids, MemberID: p.memberRef(), AllTeams: seesAllTeamViews(p)})
}

func visibleView(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (store.SavedView, error) {
	rows, err := visibleViews(ctx, q, p, []uuid.UUID{id})
	if err != nil {
		return store.SavedView{}, err
	}
	if len(rows) == 0 {
		return store.SavedView{}, errViewGone
	}
	return rows[0], nil
}

// mayWriteView also asks for conversations:read: a written view answers with its count, which
// searches the conversations.
func mayWriteView(p principal, visibility string) error {
	if visibility == viewPersonal {
		if p.isKey() {
			return errViewNeedsMember
		}
		if p.memberID == uuid.Nil() {
			return errForbidden
		}
	} else {
		if err := requireManagerOrKey(p); err != nil {
			return err
		}
		if visibility == viewTeam && !seesAllTeamViews(p) {
			return errForbidden
		}
	}
	if err := requireScope(p, oas.ViewsWrite); err != nil {
		return err
	}
	return requireScope(p, oas.ConversationsRead)
}

func validViewPosition(pos *int32) error {
	if pos != nil && (*pos < 0 || *pos > 10000) {
		return errValidation("position must be 0 to 10000")
	}
	return nil
}

func (s *Server) ListViews(ctx context.Context, req oas.ListViewsRequestObject) (oas.ListViewsResponseObject, error) {
	p := principalFrom(ctx)
	var rows []store.SavedView
	var err error
	if t := req.Params.TeamId; t != nil {
		if !seesAllTeamViews(p) {
			mine, err := s.st.ListMemberTeamIDs(ctx, store.ListMemberTeamIDsParams{WorkspaceID: p.workspaceID, MemberID: p.memberID})
			if err != nil {
				return nil, err
			}
			if p.isKey() || !slices.Contains(mine, *t) {
				return nil, errTeamGone
			}
		}
		rows, err = s.st.ListTeamSavedViews(ctx, store.ListTeamSavedViewsParams{WorkspaceID: p.workspaceID, TeamID: t})
	} else {
		rows, err = s.st.ListSavedViewsFor(ctx, store.ListSavedViewsForParams{WorkspaceID: p.workspaceID, MemberID: p.memberRef()})
	}
	if err != nil {
		return nil, err
	}
	items, err := s.withViewCounts(ctx, p, rows)
	if err != nil {
		return nil, err
	}
	return oas.ListViews200JSONResponse{Items: items}, nil
}

func (s *Server) GetView(ctx context.Context, req oas.GetViewRequestObject) (oas.GetViewResponseObject, error) {
	p := principalFrom(ctx)
	v, err := visibleView(ctx, s.st.Queries, p, req.ViewId)
	if err != nil {
		return nil, err
	}
	out, err := s.withViewCount(ctx, p, v)
	if err != nil {
		return nil, err
	}
	return oas.GetView200JSONResponse(out), nil
}

func (s *Server) CreateView(ctx context.Context, req oas.CreateViewRequestObject) (oas.CreateViewResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	if !b.Visibility.Valid() {
		return nil, errValidation("visibility must be personal, workspace or team")
	}
	visibility := string(b.Visibility)
	if err := mayWriteView(p, visibility); err != nil {
		return nil, err
	}
	name, err := trimmed(b.Name, 1, 100, "name")
	if err != nil {
		return nil, err
	}
	if err := validViewPosition(b.Position); err != nil {
		return nil, err
	}
	if (visibility == viewTeam) != (b.TeamId != nil) {
		return nil, errValidation("team_id is required for a team view and only for one")
	}
	arg := store.CreateSavedViewParams{ID: newID(), WorkspaceID: p.workspaceID, Visibility: visibility, TeamID: b.TeamId, Name: name, Now: s.now()}
	if b.Position != nil {
		arg.Position = *b.Position
	}
	if visibility == viewPersonal {
		arg.OwnerMemberID = new(p.memberID)
	}
	var out store.SavedView
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		if err := q.LockSavedViews(ctx, p.workspaceID); err != nil {
			return err
		}
		if b.TeamId != nil {
			found, err := q.ShareLockTeams(ctx, store.ShareLockTeamsParams{WorkspaceID: p.workspaceID, Ids: []uuid.UUID{*b.TeamId}})
			if err != nil {
				return err
			}
			if len(found) == 0 {
				return errValidation("team_id must be a team of the workspace")
			}
		}
		n, err := q.CountSavedViewsIn(ctx, store.CountSavedViewsInParams{WorkspaceID: p.workspaceID, Visibility: visibility, OwnerMemberID: arg.OwnerMemberID, TeamID: arg.TeamID})
		if err != nil {
			return err
		}
		if n >= maxViews {
			return errViewLimit
		}
		if arg.Filters, err = s.checkViewFilters(ctx, q, p, b.Filters); err != nil {
			return err
		}
		out, err = q.CreateSavedView(ctx, arg)
		return err
	})
	if err != nil {
		return nil, err
	}
	body, err := s.withViewCount(ctx, p, out)
	if err != nil {
		return nil, err
	}
	return oas.CreateView201JSONResponse(body), nil
}

func (s *Server) UpdateView(ctx context.Context, req oas.UpdateViewRequestObject) (oas.UpdateViewResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	if err := validViewPosition(b.Position); err != nil {
		return nil, err
	}
	arg := store.UpdateSavedViewParams{WorkspaceID: p.workspaceID, ID: req.ViewId, Position: b.Position, Now: s.now()}
	if b.Name != nil {
		name, err := trimmed(*b.Name, 1, 100, "name")
		if err != nil {
			return nil, err
		}
		arg.Name = &name
	}
	var out store.SavedView
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		v, err := visibleView(ctx, q, p, req.ViewId)
		if err != nil {
			return err
		}
		if err := mayWriteView(p, v.Visibility); err != nil {
			return err
		}
		if _, err := q.LockSavedView(ctx, store.LockSavedViewParams{WorkspaceID: p.workspaceID, ID: v.ID}); store.IsNotFound(err) {
			return errViewGone
		} else if err != nil {
			return err
		}
		if b.Filters != nil {
			if arg.Filters, err = s.checkViewFilters(ctx, q, p, *b.Filters); err != nil {
				return err
			}
		}
		out, err = q.UpdateSavedView(ctx, arg)
		if store.IsNotFound(err) {
			return errViewGone
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	body, err := s.withViewCount(ctx, p, out)
	if err != nil {
		return nil, err
	}
	return oas.UpdateView200JSONResponse(body), nil
}

func (s *Server) DeleteView(ctx context.Context, req oas.DeleteViewRequestObject) (oas.DeleteViewResponseObject, error) {
	p := principalFrom(ctx)
	v, err := visibleView(ctx, s.st.Queries, p, req.ViewId)
	if err != nil {
		return nil, err
	}
	if err := mayWriteView(p, v.Visibility); err != nil {
		return nil, err
	}
	n, err := s.st.DeleteSavedView(ctx, store.DeleteSavedViewParams{WorkspaceID: p.workspaceID, ID: v.ID})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errViewGone
	}
	return oas.DeleteView204Response{}, nil
}

func (s *Server) ReorderViews(ctx context.Context, req oas.ReorderViewsRequestObject) (oas.ReorderViewsResponseObject, error) {
	p := principalFrom(ctx)
	ids := req.Body.Ids
	if len(ids) == 0 || len(ids) > maxViews {
		return nil, errValidation("ids must name 1 to 50 views")
	}
	if len(slices.Compact(slices.SortedFunc(slices.Values(ids), func(a, b uuid.UUID) int { return a.Compare(b) }))) != len(ids) {
		return nil, errValidation("ids must not repeat a view")
	}
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		if err := q.LockSavedViews(ctx, p.workspaceID); err != nil {
			return err
		}
		seen, err := visibleViews(ctx, q, p, ids)
		if err != nil {
			return err
		}
		if len(seen) != len(ids) {
			return errViewGone
		}
		for _, v := range seen {
			if err := mayWriteView(p, v.Visibility); err != nil {
				return err
			}
		}
		_, err = q.SetSavedViewPositions(ctx, store.SetSavedViewPositionsParams{WorkspaceID: p.workspaceID, Ids: ids, Now: s.now()})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.ReorderViews204Response{}, nil
}

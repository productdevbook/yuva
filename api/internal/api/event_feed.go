package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	defaultFeedPage = 100
	maxFeedPage     = 500
	feedBatch       = 500
	feedScanLimit   = 5000
)

var errCursorExpired = problem(http.StatusGone, "cursor_expired", "events after this cursor are no longer kept; reload over the lists and continue from GET /v1/events/latest")

func (s *Server) ListEvents(ctx context.Context, req oas.ListEventsRequestObject) (oas.ListEventsResponseObject, error) {
	p := principalFrom(ctx)
	after := req.Params.After
	if after < 0 {
		return nil, errValidation("after must be a non-negative integer")
	}
	lim := int32(defaultFeedPage)
	if req.Params.Limit != nil {
		if *req.Params.Limit < 1 || *req.Params.Limit > maxFeedPage {
			return nil, errValidation("limit must be 1 to 500")
		}
		lim = *req.Params.Limit
	}
	f := &eventFilter{p: p}
	if err := f.reload(ctx, s.st.Queries); err != nil {
		return nil, err
	}
	page := oas.EventPage{Events: []oas.StoredEvent{}, Next: after}
	scanned := 0
	for first := true; ; first = false {
		rows, err := s.st.ListEventsAfter(ctx, store.ListEventsAfterParams{WorkspaceID: p.workspaceID, After: page.Next, Lim: feedBatch})
		if err != nil {
			return nil, err
		}
		if first && after > 0 {
			oldest, err := s.st.FirstEventID(ctx, p.workspaceID)
			if err != nil {
				return nil, err
			}
			if oldest == 0 || after < oldest {
				return nil, errCursorExpired
			}
		}
		for _, row := range rows {
			if int32(len(page.Events)) == lim {
				page.HasMore = true
				return oas.ListEvents200JSONResponse(page), nil
			}
			e := realtime.FromRow(row)
			ok, err := f.allows(ctx, s.st.Queries, e)
			if err != nil {
				return nil, err
			}
			page.Next = row.ID
			if !ok {
				continue
			}
			b, err := json.Marshal(e)
			if err != nil {
				return nil, err
			}
			var item oas.StoredEvent
			if err := item.UnmarshalJSON(b); err != nil {
				return nil, err
			}
			page.Events = append(page.Events, item)
		}
		scanned += len(rows)
		if len(rows) < feedBatch {
			return oas.ListEvents200JSONResponse(page), nil
		}
		if scanned >= feedScanLimit {
			page.HasMore = true
			return oas.ListEvents200JSONResponse(page), nil
		}
	}
}

func (s *Server) GetLatestEvent(ctx context.Context, _ oas.GetLatestEventRequestObject) (oas.GetLatestEventResponseObject, error) {
	p := principalFrom(ctx)
	id, err := s.st.LastEventID(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	return oas.GetLatestEvent200JSONResponse{Id: id}, nil
}

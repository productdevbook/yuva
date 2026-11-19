package api

import (
	"context"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	activityPageSize    = 20
	activityMaxPageSize = 50
)

func (s *Server) ListActivity(ctx context.Context, req oas.ListActivityRequestObject) (oas.ListActivityResponseObject, error) {
	p := principalFrom(ctx)
	lim := int32(activityPageSize)
	if l := req.Params.Limit; l != nil {
		if *l < 1 || *l > activityMaxPageSize {
			return nil, errValidation("limit must be 1 to 50")
		}
		lim = *l
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	if req.Params.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *req.Params.InboxId); err != nil {
			return nil, err
		}
	}
	rows, err := s.st.ListActivity(ctx, store.ListActivityParams{
		WorkspaceID: p.workspaceID, InboxID: req.Params.InboxId, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), ViewerMemberID: p.memberRef(),
		CursorAt: at, CursorID: id, WithHelp: !s.help.Off, WithBroadcasts: s.broadcastMode() != broadcastsOff, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	out := oas.ListActivity200JSONResponse{Items: make([]oas.ActivityItem, 0, len(rows))}
	if len(rows) > int(lim) {
		rows = rows[:lim]
		out.NextCursor = new(encodeCursor(rows[lim-1].At, rows[lim-1].ID))
	}
	for _, r := range rows {
		it := oas.ActivityItem{
			Id: r.ID, Kind: oas.ActivityKind(r.Kind), At: r.At, InboxId: r.InboxID,
			Target: oas.ActivityTarget{Type: oas.ActivityTargetType(r.TargetType), Id: r.TargetID, Title: r.Title, ParentId: r.ParentID},
		}
		if r.ActorMemberID != nil {
			it.Actor = &oas.ActivityActor{MemberId: *r.ActorMemberID, Name: derefString(r.ActorName)}
		}
		var d oas.ActivityDetail
		if r.Rating != "" {
			d.Rating = new(oas.Rating(r.Rating))
		}
		if r.State != "" {
			d.State = new(r.State)
		}
		if r.Locale != "" {
			d.Locale = new(r.Locale)
		}
		if d != (oas.ActivityDetail{}) {
			it.Detail = &d
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}

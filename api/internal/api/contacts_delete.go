package api

import (
	"context"
	"errors"
	"net/http"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

func errContactCountChanged(n int) *apiError {
	e := problem(http.StatusConflict, "count_changed", "confirm_count is not the number of contacts named; show the selection again and confirm its count")
	e.Extra = map[string]any{"count": n}
	return e
}

func (s *Server) DeleteContacts(ctx context.Context, req oas.DeleteContactsRequestObject) (oas.DeleteContactsResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrFullKey(p); err != nil {
		return nil, err
	}
	if len(req.Body.Ids) == 0 || len(req.Body.Ids) > maxBulkConversations {
		return nil, errValidation("ids must name 1 to 100 contacts")
	}
	seen := map[uuid.UUID]bool{}
	var ids []uuid.UUID
	for _, id := range req.Body.Ids {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if int(req.Body.ConfirmCount) != len(ids) {
		return nil, errContactCountChanged(len(ids))
	}
	out := oas.ContactDeleteResult{Failed: []oas.ContactBulkFailure{}}
	for _, id := range ids {
		err := s.deleteContact(ctx, p, id)
		var ae *apiError
		switch {
		case err == nil:
			out.Deleted++
		case errors.As(err, &ae) && ae.Status < http.StatusInternalServerError:
			f := oas.ContactBulkFailure{Id: id, Status: int32(ae.Status), Code: ae.Code}
			if ae.Detail != "" {
				f.Detail = new(ae.Detail)
			}
			out.Failed = append(out.Failed, f)
		default:
			return nil, err
		}
	}
	return oas.DeleteContacts200JSONResponse(out), nil
}

func (s *Server) GetContactDeletionPreview(ctx context.Context, req oas.GetContactDeletionPreviewRequestObject) (oas.GetContactDeletionPreviewResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrFullKey(p); err != nil {
		return nil, err
	}
	found, err := s.st.ContactExists(ctx, store.ContactExistsParams{WorkspaceID: p.workspaceID, ID: req.ContactId})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errContactGone
	}
	withTrash, _ := trashView(p)
	row, err := s.st.ContactDeletionPreview(ctx, store.ContactDeletionPreviewParams{
		WorkspaceID: p.workspaceID, ContactID: req.ContactId, WithTrash: withTrash, TrashOwner: p.memberRef(),
		ViewerMemberID: p.memberRef(), AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return nil, err
	}
	return oas.GetContactDeletionPreview200JSONResponse{ConversationCount: row.ConversationCount, HasHiddenConversations: row.HasHiddenConversations}, nil
}

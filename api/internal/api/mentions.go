package api

import (
	"context"
	"slices"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

func (s *Server) conversationSummaries(ctx context.Context, workspaceID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]oas.ConversationSummary, error) {
	ids = slices.Compact(slices.SortedFunc(slices.Values(ids), uuid.UUID.Compare))
	rows, err := s.st.ListConversationSummaries(ctx, store.ListConversationSummariesParams{WorkspaceID: workspaceID, Ids: ids})
	if err != nil {
		return nil, err
	}
	contactIDs := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		contactIDs = append(contactIDs, r.ContactID)
	}
	contactRows, err := s.st.ListContactSummaries(ctx, store.ListContactSummariesParams{WorkspaceID: workspaceID, Ids: contactIDs})
	if err != nil {
		return nil, err
	}
	contacts := make(map[uuid.UUID]oas.ConversationContact, len(contactRows))
	for _, c := range contactRows {
		cc := oas.ConversationContact{Id: c.ID, Name: c.Name}
		if c.Email != "" {
			e := oas.Email(c.Email)
			cc.Email = &e
		}
		contacts[c.ID] = cc
	}
	out := make(map[uuid.UUID]oas.ConversationSummary, len(rows))
	for _, r := range rows {
		out[r.ID] = oas.ConversationSummary{
			Id: r.ID, InboxId: r.InboxID, Subject: r.Subject, Status: oas.ConversationStatus(r.Status), Contact: contacts[r.ContactID],
		}
	}
	return out, nil
}

func (s *Server) ListMentions(ctx context.Context, req oas.ListMentionsRequestObject) (oas.ListMentionsResponseObject, error) {
	p := principalFrom(ctx)
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, cid, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListMentions(ctx, store.ListMentionsParams{
		MemberID: p.memberID, WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), CursorAt: at, CursorID: cid, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	unseen, err := s.st.CountUnseenMentions(ctx, store.CountUnseenMentionsParams{MemberID: p.memberID, WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes()})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		next = &c
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ConversationID
	}
	convs, err := s.conversationSummaries(ctx, p.workspaceID, ids)
	if err != nil {
		return nil, err
	}
	items := make([]oas.Mention, len(rows))
	for i, r := range rows {
		author := messageAuthor(messageRow{
			AuthorType: r.AuthorType, AuthorMemberID: r.AuthorMemberID, AuthorApiKeyID: r.AuthorApiKeyID, Via: r.Via,
			BotName: r.BotName, BotAvatarUrl: r.BotAvatarUrl,
		})
		items[i] = oas.Mention{
			Note:         oas.MentionNote{Id: r.ID, Text: previewText(r.Body), Author: author, CreatedAt: r.CreatedAt},
			Conversation: convs[r.ConversationID],
			Seen:         r.Seen,
		}
	}
	return oas.ListMentions200JSONResponse{Items: items, NextCursor: next, Unseen: unseen}, nil
}

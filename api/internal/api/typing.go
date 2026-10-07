package api

import (
	"context"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

func (s *Server) SetMemberTyping(ctx context.Context, req oas.SetMemberTypingRequestObject) (oas.SetMemberTypingResponseObject, error) {
	p := principalFrom(ctx)
	c, err := visibleConversation(ctx, s.st.Queries, p, req.ConversationId, false)
	if err != nil {
		return nil, err
	}
	m, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: p.workspaceID, ID: p.memberID})
	if err != nil {
		return nil, err
	}
	typing := req.Body == nil || req.Body.Typing == nil || *req.Body.Typing
	memberID, name := p.memberID, m.Name
	data := oas.Typing{ConversationId: c.ID, Typing: typing, Author: oas.TypingAuthor{Type: oas.TypingAuthorTypeMember, MemberId: &memberID, Name: &name}}
	inbox, conv := c.InboxID, c.ID
	s.signal(ctx, realtime.Event{Type: realtime.Typing, WorkspaceID: p.workspaceID, InboxID: &inbox, ConversationID: &conv, Data: mustJSON(data)})
	return oas.SetMemberTyping204Response{}, nil
}

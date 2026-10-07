package api

import (
	"context"
	"fmt"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
)

// operatorContext lets the command line act as an owner of one workspace through the same
// handlers, validation and events as the HTTP API.
func operatorContext(ctx context.Context, workspaceID uuid.UUID) context.Context {
	return context.WithValue(ctx, principalKey, principal{workspaceID: workspaceID, role: roleOwner})
}

// OperatorCreateInbox creates an inbox for the command line and returns its identity secret once.
func (s *Server) OperatorCreateInbox(ctx context.Context, workspaceID uuid.UUID, body oas.InboxCreate) (oas.Inbox, string, error) {
	res, err := s.CreateInbox(operatorContext(ctx, workspaceID), oas.CreateInboxRequestObject{Body: &body})
	if err != nil {
		return oas.Inbox{}, "", err
	}
	created, ok := res.(oas.CreateInbox201JSONResponse)
	if !ok {
		return oas.Inbox{}, "", fmt.Errorf("unexpected response %T", res)
	}
	return created.Inbox, created.IdentitySecret, nil
}

func (s *Server) OperatorListInboxes(ctx context.Context, workspaceID uuid.UUID) ([]oas.Inbox, error) {
	res, err := s.ListInboxes(operatorContext(ctx, workspaceID), oas.ListInboxesRequestObject{})
	if err != nil {
		return nil, err
	}
	list, ok := res.(oas.ListInboxes200JSONResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response %T", res)
	}
	return list.Items, nil
}

func (s *Server) OperatorCreateChannel(ctx context.Context, workspaceID, inboxID uuid.UUID, body oas.ChannelCreate) (oas.Channel, error) {
	res, err := s.CreateChannel(operatorContext(ctx, workspaceID), oas.CreateChannelRequestObject{InboxId: inboxID, Body: &body})
	if err != nil {
		return oas.Channel{}, err
	}
	created, ok := res.(oas.CreateChannel201JSONResponse)
	if !ok {
		return oas.Channel{}, fmt.Errorf("unexpected response %T", res)
	}
	return oas.Channel(created), nil
}

func (s *Server) OperatorListChannels(ctx context.Context, workspaceID, inboxID uuid.UUID) ([]oas.Channel, error) {
	res, err := s.ListChannels(operatorContext(ctx, workspaceID), oas.ListChannelsRequestObject{InboxId: inboxID})
	if err != nil {
		return nil, err
	}
	list, ok := res.(oas.ListChannels200JSONResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response %T", res)
	}
	return list.Items, nil
}

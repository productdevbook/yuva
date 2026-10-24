package api

import (
	"context"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

type setupWant struct {
	inboxes, channels, properties, lists bool
}

// setupParts is what the caller can see of the workspace's setup: the source of both the MCP
// get_setup_state tool and Home's checklist.
type setupParts struct {
	inboxes    []store.ListSetupPartsRow
	channels   map[uuid.UUID][]store.ListSetupPartsRow
	properties []store.ListSetupPartsRow
	lists      []store.ListSetupPartsRow
	analytics  bool
}

func (s *Server) setupParts(ctx context.Context, p principal, want setupWant) (setupParts, error) {
	out := setupParts{channels: map[uuid.UUID][]store.ListSetupPartsRow{}, analytics: s.analytics != nil}
	rows, err := s.st.ListSetupParts(ctx, store.ListSetupPartsParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
		WithInboxes: want.inboxes, WithChannels: want.channels, WithProperties: want.properties && out.analytics, WithLists: want.lists,
	})
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		switch r.Part {
		case "inbox":
			out.inboxes = append(out.inboxes, r)
		case "channel":
			out.channels[*r.InboxID] = append(out.channels[*r.InboxID], r)
		case "property":
			out.properties = append(out.properties, r)
		case "list":
			out.lists = append(out.lists, r)
		}
	}
	return out, nil
}

func (sp setupParts) checklist() []oas.HomeSetupItem {
	var chat, email, mail, app, docs int64
	for _, in := range sp.inboxes {
		if in.Purpose != string(oas.InboxPurposeMail) {
			continue
		}
		for _, ch := range sp.channels[in.ID] {
			if ch.Kind == string(oas.ChannelKindEmail) {
				mail++
				break
			}
		}
	}
	for _, chs := range sp.channels {
		for _, ch := range chs {
			switch ch.Kind {
			case string(oas.ChannelKindChat):
				chat++
				if ch.Docs {
					docs++
				}
			case string(oas.ChannelKindEmail):
				email++
			case string(oas.ChannelKindApp):
				app++
			}
		}
	}
	item := func(id oas.HomeSetupItemId, n int64) oas.HomeSetupItem {
		return oas.HomeSetupItem{Id: id, Count: n, Done: n > 0}
	}
	out := []oas.HomeSetupItem{
		item(oas.HomeSetupItemIdChat, chat), item(oas.HomeSetupItemIdEmail, email), item(oas.HomeSetupItemIdMail, mail), item(oas.HomeSetupItemIdApp, app), item(oas.HomeSetupItemIdDocs, docs),
	}
	if sp.analytics {
		out = append(out, item(oas.HomeSetupItemIdAnalytics, int64(len(sp.properties))))
	}
	return append(out, item(oas.HomeSetupItemIdNewsletter, int64(len(sp.lists))))
}

package api

import (
	"context"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
)

type mcpTeam struct {
	ID          uuid.UUID   `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	MemberIDs   []uuid.UUID `json:"member_ids" jsonschema:"member ids; names come from list_members"`
	InboxIDs    []uuid.UUID `json:"inbox_ids" jsonschema:"the inboxes the team is granted"`
}

var mcpTeamTools = []mcpTool{
	defineTool("list_teams", "List teams",
		"Lists the teams you can see with their members and inboxes, so assign and search_conversations have team ids. Owners and admins see every team, others the teams they belong to and those granted an inbox they see. Maps to GET /v1/teams.",
		readOnly, nil, mcpTool{ops: []string{"ListTeams"}},
		func(ctx context.Context, c *mcpCall, _ noInput) (mcpPage[mcpTeam], error) {
			res, err := c.s.ListTeams(ctx, oas.ListTeamsRequestObject{})
			if err != nil {
				return mcpPage[mcpTeam]{}, err
			}
			out := mcpPage[mcpTeam]{Items: []mcpTeam{}}
			for _, t := range res.(oas.ListTeams200JSONResponse).Items {
				out.Items = append(out.Items, mcpTeam{ID: t.Id, Name: t.Name, Description: t.Description, MemberIDs: t.MemberIds, InboxIDs: t.InboxIds})
			}
			return out, nil
		}),
}

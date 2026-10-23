package api

import (
	"context"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
)

type mcpIssueIntegration struct {
	ID            uuid.UUID `json:"id"`
	Provider      string    `json:"provider" jsonschema:"github or linear"`
	Name          string    `json:"name"`
	Account       string    `json:"account"`
	DefaultTarget string    `json:"default_target,omitempty" jsonschema:"where create_issue files issues without a target: owner/repo or a Linear team"`
}

type mcpIssueIntegrations struct {
	Items []mcpIssueIntegration `json:"items"`
}

type mcpIssueLink struct {
	ID             uuid.UUID `json:"id" jsonschema:"the link's id"`
	Provider       string    `json:"provider"`
	Key            string    `json:"key" jsonschema:"owner/repo#12 or ENG-123"`
	URL            string    `json:"url"`
	Title          string    `json:"title"`
	State          string    `json:"state" jsonschema:"the issue's state as the tracker names it"`
	Progress       string    `json:"progress" jsonschema:"the feedback progress the state sets: planned, in_progress, done or wont_do"`
	ConversationID uuid.UUID `json:"conversation_id"`
	Created        bool      `json:"created" jsonschema:"false when the feedback was already linked to this issue"`
}

type linkIssueIn struct {
	ConversationID uuid.UUID `json:"conversation_id" jsonschema:"a feedback conversation"`
	IntegrationID  uuid.UUID `json:"integration_id" jsonschema:"from list_issue_integrations"`
	URL            string    `json:"url" jsonschema:"the issue's address, owner/repo#12 or ENG-123"`
}

type createIssueIn struct {
	ConversationID uuid.UUID `json:"conversation_id" jsonschema:"a feedback conversation"`
	IntegrationID  uuid.UUID `json:"integration_id" jsonschema:"from list_issue_integrations"`
	Title          string    `json:"title" jsonschema:"at most 256 characters"`
	Body           string    `json:"body,omitempty" jsonschema:"Markdown; Yuva adds a link back to the conversation. Leave out the customer's name, address and other personal data unless the person you work for asks to include them"`
	Target         string    `json:"target,omitempty" jsonschema:"owner/repo or a Linear team key; defaults to the integration's default_target"`
}

func mcpIssueLinkOf(l oas.IssueLink, created bool) mcpIssueLink {
	out := mcpIssueLink{ID: l.Id, Provider: string(l.Provider), Key: l.Key, URL: l.Url, Title: l.Title, State: l.State, Progress: string(l.Progress), Created: created}
	if l.ConversationId != nil {
		out.ConversationID = *l.ConversationId
	}
	return out
}

func (c *mcpCall) linkIssue(ctx context.Context, conversation uuid.UUID, body oas.IssueLinkCreate) (mcpIssueLink, error) {
	res, err := c.s.LinkConversationIssue(ctx, oas.LinkConversationIssueRequestObject{ConversationId: conversation, Body: &body})
	if err != nil {
		return mcpIssueLink{}, err
	}
	switch r := res.(type) {
	case oas.LinkConversationIssue201JSONResponse:
		return mcpIssueLinkOf(oas.IssueLink(r), true), nil
	case oas.LinkConversationIssue200JSONResponse:
		return mcpIssueLinkOf(oas.IssueLink(r), false), nil
	}
	return mcpIssueLink{}, errInternal
}

var mcpIssueTools = []mcpTool{
	defineTool("list_issue_integrations", "List GitHub and Linear connections",
		"The workspace's GitHub and Linear integrations that link_issue and create_issue use, with the default place new issues go. Needs integrations:read. Maps to GET /v1/issue-integrations.",
		readOnly, nil, mcpTool{ops: []string{"ListIssueIntegrations"}},
		func(ctx context.Context, c *mcpCall, _ struct{}) (mcpIssueIntegrations, error) {
			res, err := c.s.ListIssueIntegrations(ctx, oas.ListIssueIntegrationsRequestObject{})
			if err != nil {
				return mcpIssueIntegrations{}, err
			}
			out := mcpIssueIntegrations{Items: []mcpIssueIntegration{}}
			for _, in := range res.(oas.ListIssueIntegrations200JSONResponse).Items {
				out.Items = append(out.Items, mcpIssueIntegration{ID: in.Id, Provider: string(in.Provider), Name: in.Name, Account: in.Account, DefaultTarget: orZero(in.DefaultTarget)})
			}
			return out, nil
		}),
	defineTool("link_issue", "Link a feedback to a GitHub or Linear issue",
		"Links an existing GitHub or Linear issue to a feedback conversation. The issue's state sets the feedback's progress at once and whenever it changes, which the customer sees. Linking again changes nothing. Needs issues:write and conversations:read. Maps to POST /v1/conversations/{id}/issues with url.",
		idempotentPublic, nil, mcpTool{ops: []string{"LinkConversationIssue"}},
		func(ctx context.Context, c *mcpCall, in linkIssueIn) (mcpIssueLink, error) {
			return c.linkIssue(ctx, in.ConversationID, oas.IssueLinkCreate{IntegrationId: in.IntegrationID, Url: &in.URL})
		}),
	defineTool("create_issue", "Create a GitHub or Linear issue from a feedback",
		"Creates an issue in GitHub or Linear with the title and body you write, adds a link back to the conversation, and links it to the feedback; its state then sets the feedback's progress, which the customer sees. Nothing of the conversation is copied: write the body yourself and leave out personal data unless asked. Needs issues:write and conversations:read. Maps to POST /v1/conversations/{id}/issues with title.",
		delivering, nil, mcpTool{ops: []string{"LinkConversationIssue"}},
		func(ctx context.Context, c *mcpCall, in createIssueIn) (mcpIssueLink, error) {
			b := oas.IssueLinkCreate{IntegrationId: in.IntegrationID, Title: &in.Title}
			if in.Body != "" {
				b.Body = &in.Body
			}
			if in.Target != "" {
				b.Target = &in.Target
			}
			return c.linkIssue(ctx, in.ConversationID, b)
		}),
}

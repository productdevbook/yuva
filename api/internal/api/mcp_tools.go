package api

import (
	"context"
	"net/http"
	"slices"
	"time"
	"uuid"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oapi-codegen/nullable"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

type mcpTool struct {
	name   string
	ops    []string
	scopes []oas.ApiKeyScope
	allow  func(p principal) bool
	add    func(srv *mcp.Server, c *mcpCall)
}

func (t mcpTool) listed(p principal) bool {
	for _, op := range t.ops {
		if keyMayCall(p, op) != nil {
			return false
		}
	}
	for _, sc := range t.scopes {
		if requireScope(p, sc) != nil {
			return false
		}
	}
	return t.allow == nil || t.allow(p)
}

func boolPtr(b bool) *bool { return &b }

var (
	readOnly    = mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(false)}
	additive    = mcp.ToolAnnotations{DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(false)}
	idempotent  = mcp.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(false)}
	delivering  = mcp.ToolAnnotations{DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true)}
	destructive = mcp.ToolAnnotations{DestructiveHint: boolPtr(true), OpenWorldHint: boolPtr(false)}
)

func defineTool[In, Out any](name, title, description string, ann mcp.ToolAnnotations, enums map[string][]any,
	t mcpTool, run func(ctx context.Context, c *mcpCall, in In) (Out, error)) mcpTool {
	ann.Title = title
	tool := &mcp.Tool{
		Name: name, Title: title, Description: description + customerContentRule, Annotations: &ann,
		InputSchema: mcpSchema[In](enums), OutputSchema: mcpSchema[Out](nil),
	}
	t.name = name
	t.add = func(srv *mcp.Server, c *mcpCall) {
		mcp.AddTool(srv, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
			out, err := run(c.as(ctx), c, in)
			if err != nil {
				var zero Out
				return nil, zero, c.toolError(ctx, err)
			}
			return nil, out, nil
		})
	}
	return t
}

var conversationStatuses = []any{"open", "pending", "snoozed", "closed"}

type mcpPage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty" jsonschema:"pass as cursor to get the next page"`
}

type searchConversationsIn struct {
	Query     string     `json:"query,omitempty" jsonschema:"full-text search over subject, messages, notes and contact; -word excludes"`
	InboxID   *uuid.UUID `json:"inbox_id,omitempty"`
	Status    string     `json:"status,omitempty"`
	Assignee  string     `json:"assignee,omitempty" jsonschema:"a member id, me or unassigned"`
	LabelID   *uuid.UUID `json:"label_id,omitempty"`
	ContactID *uuid.UUID `json:"contact_id,omitempty"`
	Spam      bool       `json:"spam,omitempty" jsonschema:"true lists only conversations flagged as spam"`
	Cursor    string     `json:"cursor,omitempty"`
	Limit     int32      `json:"limit,omitempty" jsonschema:"1 to 100, default 25"`
}

type listFeedbackIn struct {
	Query    string     `json:"query,omitempty" jsonschema:"full-text search"`
	InboxID  *uuid.UUID `json:"inbox_id,omitempty"`
	Category string     `json:"category,omitempty"`
	Status   string     `json:"status,omitempty"`
	Cursor   string     `json:"cursor,omitempty"`
	Limit    int32      `json:"limit,omitempty" jsonschema:"1 to 100, default 25"`
}

func optString[T ~string](v string) *T {
	if v == "" {
		return nil
	}
	t := T(v)
	return &t
}

func optLimit(v int32) *int32 {
	if v == 0 {
		return nil
	}
	return &v
}

func (c *mcpCall) listConversations(ctx context.Context, params oas.ListConversationsParams) (mcpPage[mcpConversation], error) {
	res, err := c.s.ListConversations(ctx, oas.ListConversationsRequestObject{Params: params})
	if err != nil {
		return mcpPage[mcpConversation]{}, err
	}
	page := res.(oas.ListConversations200JSONResponse)
	out := mcpPage[mcpConversation]{Items: []mcpConversation{}}
	for _, it := range page.Items {
		out.Items = append(out.Items, conversationItemOut(it))
	}
	if page.NextCursor != nil {
		out.NextCursor = *page.NextCursor
	}
	return out, nil
}

type conversationIn struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	Cursor         string    `json:"cursor,omitempty" jsonschema:"next_cursor of an earlier call, to load older messages"`
	Limit          int32     `json:"limit,omitempty" jsonschema:"messages per page, 1 to 100, default 50"`
}

type conversationOutWithMessages struct {
	Conversation mcpConversation `json:"conversation"`
	Messages     []mcpMessage    `json:"messages" jsonschema:"the latest messages, notes, drafts and timeline events, oldest first"`
	NextCursor   string          `json:"next_cursor,omitempty" jsonschema:"pass as cursor to load older messages"`
}

func (c *mcpCall) conversation(ctx context.Context, id uuid.UUID, cursor string, limit int32) (conversationOutWithMessages, error) {
	var out conversationOutWithMessages
	res, err := c.s.GetConversation(ctx, oas.GetConversationRequestObject{ConversationId: id})
	if err != nil {
		return out, err
	}
	out.Conversation = conversationOut(oas.Conversation(res.(oas.GetConversation200JSONResponse)))
	if limit == 0 {
		limit = 50
	}
	desc := oas.ListMessagesParamsOrder("desc")
	params := oas.ListMessagesParams{Order: &desc, Limit: &limit}
	if cursor != "" {
		params.Cursor = &cursor
	}
	mres, err := c.s.ListMessages(ctx, oas.ListMessagesRequestObject{ConversationId: id, Params: params})
	if err != nil {
		return out, err
	}
	page := mres.(oas.ListMessages200JSONResponse)
	out.Messages = []mcpMessage{}
	for _, m := range slices.Backward(page.Items) {
		out.Messages = append(out.Messages, messageOut(m))
	}
	if page.NextCursor != nil {
		out.NextCursor = *page.NextCursor
	}
	if requireScope(c.p, oas.ContactsRead) == nil {
		if cres, err := c.s.GetContact(ctx, oas.GetContactRequestObject{ContactId: out.Conversation.ContactID}); err == nil {
			ct := cres.(oas.GetContact200JSONResponse)
			out.Conversation.CustomerContent.ContactName = ct.Name
			if len(ct.Emails) > 0 {
				out.Conversation.CustomerContent.ContactEmail = string(ct.Emails[0])
			}
		}
	}
	return out, nil
}

type noInput struct{}

type contactIn struct {
	ContactID uuid.UUID `json:"contact_id"`
}

type lookupContactIn struct {
	InboxID    uuid.UUID `json:"inbox_id"`
	ExternalID string    `json:"external_id" jsonschema:"the contact's id in the host app of that inbox"`
}

type mcpLabel struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Color string    `json:"color"`
}

type mcpCannedReply struct {
	ID       uuid.UUID `json:"id"`
	Title    string    `json:"title"`
	Shortcut string    `json:"shortcut"`
	Body     string    `json:"body" jsonschema:"text written by the workspace's members"`
}

type mcpCount struct {
	ID    uuid.UUID `json:"id"`
	Count int64     `json:"count"`
}

type mcpCategoryCount struct {
	Category string `json:"category"`
	Count    int64  `json:"count"`
}

type mcpCounts struct {
	All                int64              `json:"all" jsonschema:"open conversations the caller can see"`
	Mine               int64              `json:"mine" jsonschema:"open conversations assigned to the calling member"`
	Unassigned         int64              `json:"unassigned"`
	Spam               int64              `json:"spam"`
	Feedback           int64              `json:"feedback"`
	Inboxes            []mcpCount         `json:"inboxes" jsonschema:"open conversations per inbox id"`
	Labels             []mcpCount         `json:"labels" jsonschema:"open conversations per label id"`
	FeedbackCategories []mcpCategoryCount `json:"feedback_categories"`
}

type replyIn struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	Body           string    `json:"body" jsonschema:"plain text"`
}

type sendDraftIn struct {
	MessageID uuid.UUID `json:"message_id" jsonschema:"the id draft_reply returned"`
}

type assignIn struct {
	ConversationID uuid.UUID  `json:"conversation_id"`
	AssigneeID     *uuid.UUID `json:"assignee_id,omitempty" jsonschema:"a member id; omit to unassign"`
}

type setStatusIn struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	Status         string    `json:"status"`
}

type snoozeIn struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	Until          time.Time `json:"until" jsonschema:"when the conversation opens again, in the future"`
}

type labelsIn struct {
	ConversationID uuid.UUID   `json:"conversation_id"`
	LabelIDs       []uuid.UUID `json:"label_ids" jsonschema:"label ids from list_labels"`
}

type moveIn struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	InboxID        uuid.UUID `json:"inbox_id" jsonschema:"the inbox to move it to"`
}

type bulkIn struct {
	ConversationIDs []uuid.UUID `json:"conversation_ids" jsonschema:"1 to 100 conversations"`
	Status          string      `json:"status,omitempty"`
	SnoozeUntil     *time.Time  `json:"snooze_until,omitempty" jsonschema:"required with status snoozed"`
	AssigneeID      *uuid.UUID  `json:"assignee_id,omitempty" jsonschema:"assign to this member"`
	Unassign        bool        `json:"unassign,omitempty" jsonschema:"true removes the assignee"`
	AddLabels       []uuid.UUID `json:"add_labels,omitempty"`
	RemoveLabels    []uuid.UUID `json:"remove_labels,omitempty"`
}

type mcpBulkFailure struct {
	ID     uuid.UUID `json:"id"`
	Status int32     `json:"status"`
	Code   string    `json:"code"`
	Detail string    `json:"detail,omitempty"`
}

type bulkOut struct {
	Updated []mcpConversation `json:"updated"`
	Failed  []mcpBulkFailure  `json:"failed" jsonschema:"conversations that were not changed, with the reason"`
}

type mergeIn struct {
	ContactID uuid.UUID `json:"contact_id" jsonschema:"the contact that stays"`
	SourceID  uuid.UUID `json:"source_id" jsonschema:"the contact merged into it and then deleted"`
}

func (c *mcpCall) createMessage(ctx context.Context, conv uuid.UUID, body oas.MessageCreate) (mcpMessage, error) {
	res, err := c.s.CreateMessage(ctx, oas.CreateMessageRequestObject{ConversationId: conv, JSONBody: &body})
	if err != nil {
		return mcpMessage{}, err
	}
	switch m := res.(type) {
	case oas.CreateMessage201JSONResponse:
		return messageOut(oas.Message(m)), nil
	case oas.CreateMessage200JSONResponse:
		return messageOut(oas.Message(m)), nil
	}
	return mcpMessage{}, errInternal
}

// noPendingDraft stops send_reply from delivering a copy of a draft the caller already wrote
// there; assistants asked to "send it" otherwise leave the draft behind.
func (c *mcpCall) noPendingDraft(ctx context.Context, conv uuid.UUID) error {
	arg := store.CallerDraftInConversationParams{WorkspaceID: c.p.workspaceID, ConversationID: conv}
	if c.p.isKey() {
		arg.ApiKeyID = &c.p.keyID
	} else {
		arg.MemberID = &c.p.memberID
		if c.p.via != "" {
			arg.Via = &c.p.via
		}
	}
	id, err := c.s.st.CallerDraftInConversation(ctx, arg)
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return problem(http.StatusConflict, "draft_pending", "you have a draft in this conversation ("+id.String()+"); send it with send_draft or discard it in Yuva instead of sending its text again")
}

func (c *mcpCall) updateConversation(ctx context.Context, id uuid.UUID, b oas.ConversationUpdate) (mcpConversation, error) {
	res, err := c.s.UpdateConversation(ctx, oas.UpdateConversationRequestObject{ConversationId: id, Body: &b})
	if err != nil {
		return mcpConversation{}, err
	}
	return conversationOut(oas.Conversation(res.(oas.UpdateConversation200JSONResponse))), nil
}

func (c *mcpCall) bulk(ctx context.Context, b oas.ConversationBulkUpdate) (bulkOut, error) {
	res, err := c.s.BulkUpdateConversations(ctx, oas.BulkUpdateConversationsRequestObject{Body: &b})
	if err != nil {
		return bulkOut{}, err
	}
	r := res.(oas.BulkUpdateConversations200JSONResponse)
	out := bulkOut{Updated: []mcpConversation{}, Failed: []mcpBulkFailure{}}
	for _, u := range r.Updated {
		out.Updated = append(out.Updated, conversationOut(u))
	}
	for _, f := range r.Failed {
		mf := mcpBulkFailure{ID: f.Id, Status: f.Status, Code: f.Code}
		if f.Detail != nil {
			mf.Detail = *f.Detail
		}
		out.Failed = append(out.Failed, mf)
	}
	return out, nil
}

func (c *mcpCall) changeLabels(ctx context.Context, in labelsIn, add bool) (mcpConversation, error) {
	if len(in.LabelIDs) == 0 {
		return mcpConversation{}, errValidation("label_ids must name at least one label")
	}
	b := oas.ConversationBulkUpdate{ConversationIds: []uuid.UUID{in.ConversationID}}
	ids := in.LabelIDs
	if add {
		b.AddLabels = &ids
	} else {
		b.RemoveLabels = &ids
	}
	out, err := c.bulk(ctx, b)
	if err != nil {
		return mcpConversation{}, err
	}
	if len(out.Failed) > 0 {
		f := out.Failed[0]
		return mcpConversation{}, problem(int(f.Status), f.Code, f.Detail)
	}
	return out.Updated[0], nil
}

func mayDeliver(p principal) bool { return !(p.deliversAsBot() && !p.botsMaySend) }

var mcpTools = []mcpTool{
	defineTool("search_conversations", "Search conversations",
		"Lists conversations in the inboxes you can see, most recent activity first, with filters and full-text search. Maps to GET /v1/conversations.",
		readOnly, map[string][]any{"status": conversationStatuses}, mcpTool{ops: []string{"ListConversations"}},
		func(ctx context.Context, c *mcpCall, in searchConversationsIn) (mcpPage[mcpConversation], error) {
			params := oas.ListConversationsParams{
				InboxId: in.InboxID, ContactId: in.ContactID, LabelId: in.LabelID, Status: optString[oas.ConversationStatus](in.Status),
				Assignee: optString[string](in.Assignee), Q: optString[string](in.Query), Cursor: optString[string](in.Cursor), Limit: optLimit(in.Limit),
			}
			if in.Spam {
				params.Spam = &in.Spam
			}
			return c.listConversations(ctx, params)
		}),
	defineTool("get_conversation", "Get a conversation",
		"Returns a conversation with its latest messages, notes, drafts and timeline events. Maps to GET /v1/conversations/{id} and its messages.",
		readOnly, nil, mcpTool{ops: []string{"GetConversation", "ListMessages"}},
		func(ctx context.Context, c *mcpCall, in conversationIn) (conversationOutWithMessages, error) {
			return c.conversation(ctx, in.ConversationID, in.Cursor, in.Limit)
		}),
	defineTool("list_inboxes", "List inboxes",
		"Lists the inboxes you can see with their language, timezone and mode. Maps to GET /v1/inboxes.",
		readOnly, nil, mcpTool{ops: []string{"ListInboxes"}},
		func(ctx context.Context, c *mcpCall, _ noInput) (mcpPage[mcpInbox], error) {
			res, err := c.s.ListInboxes(ctx, oas.ListInboxesRequestObject{})
			if err != nil {
				return mcpPage[mcpInbox]{}, err
			}
			out := mcpPage[mcpInbox]{Items: []mcpInbox{}}
			for _, in := range res.(oas.ListInboxes200JSONResponse).Items {
				out.Items = append(out.Items, inboxOut(in))
			}
			return out, nil
		}),
	defineTool("get_contact", "Get a contact",
		"Returns a contact with names, addresses, external ids and attributes. Maps to GET /v1/contacts/{id}.",
		readOnly, nil, mcpTool{ops: []string{"GetContact"}},
		func(ctx context.Context, c *mcpCall, in contactIn) (mcpContact, error) {
			res, err := c.s.GetContact(ctx, oas.GetContactRequestObject{ContactId: in.ContactID})
			if err != nil {
				return mcpContact{}, err
			}
			return contactOut(oas.Contact(res.(oas.GetContact200JSONResponse))), nil
		}),
	defineTool("lookup_contact", "Look up a contact",
		"Finds a contact by the host app's user id in an inbox. Maps to GET /v1/contacts/lookup.",
		readOnly, nil, mcpTool{ops: []string{"LookupContact"}},
		func(ctx context.Context, c *mcpCall, in lookupContactIn) (mcpContact, error) {
			res, err := c.s.LookupContact(ctx, oas.LookupContactRequestObject{Params: oas.LookupContactParams{InboxId: in.InboxID, ExternalId: in.ExternalID}})
			if err != nil {
				return mcpContact{}, err
			}
			return contactOut(oas.Contact(res.(oas.LookupContact200JSONResponse))), nil
		}),
	defineTool("list_labels", "List labels",
		"Lists the workspace's labels. Maps to GET /v1/labels.",
		readOnly, nil, mcpTool{ops: []string{"ListLabels"}},
		func(ctx context.Context, c *mcpCall, _ noInput) (mcpPage[mcpLabel], error) {
			res, err := c.s.ListLabels(ctx, oas.ListLabelsRequestObject{})
			if err != nil {
				return mcpPage[mcpLabel]{}, err
			}
			out := mcpPage[mcpLabel]{Items: []mcpLabel{}}
			for _, l := range res.(oas.ListLabels200JSONResponse).Items {
				out.Items = append(out.Items, mcpLabel{ID: l.Id, Name: l.Name, Color: l.Color})
			}
			return out, nil
		}),
	defineTool("list_canned_replies", "List canned replies",
		"Lists the workspace's saved replies, written by its members. Maps to GET /v1/canned-replies.",
		readOnly, nil, mcpTool{ops: []string{"ListCannedReplies"}},
		func(ctx context.Context, c *mcpCall, _ noInput) (mcpPage[mcpCannedReply], error) {
			res, err := c.s.ListCannedReplies(ctx, oas.ListCannedRepliesRequestObject{})
			if err != nil {
				return mcpPage[mcpCannedReply]{}, err
			}
			out := mcpPage[mcpCannedReply]{Items: []mcpCannedReply{}}
			for _, r := range res.(oas.ListCannedReplies200JSONResponse).Items {
				out.Items = append(out.Items, mcpCannedReply{ID: r.Id, Title: r.Title, Shortcut: r.Shortcut, Body: r.Body})
			}
			return out, nil
		}),
	defineTool("get_counts", "Count open conversations",
		"Counts open conversations you can see: all, assigned to you, unassigned, spam, feedback, per inbox and per label. Maps to GET /v1/conversations/counts.",
		readOnly, nil, mcpTool{ops: []string{"GetConversationCounts"}},
		func(ctx context.Context, c *mcpCall, _ noInput) (mcpCounts, error) {
			res, err := c.s.GetConversationCounts(ctx, oas.GetConversationCountsRequestObject{})
			if err != nil {
				return mcpCounts{}, err
			}
			r := res.(oas.GetConversationCounts200JSONResponse)
			out := mcpCounts{All: r.All, Mine: r.Mine, Unassigned: r.Unassigned, Spam: r.Spam, Feedback: r.Feedback,
				Inboxes: []mcpCount{}, Labels: []mcpCount{}, FeedbackCategories: []mcpCategoryCount{}}
			for _, x := range r.Inboxes {
				out.Inboxes = append(out.Inboxes, mcpCount{ID: x.Id, Count: x.Count})
			}
			for _, x := range r.Labels {
				out.Labels = append(out.Labels, mcpCount{ID: x.Id, Count: x.Count})
			}
			for _, x := range r.FeedbackCategories {
				out.FeedbackCategories = append(out.FeedbackCategories, mcpCategoryCount{Category: string(x.Category), Count: x.Count})
			}
			return out, nil
		}),
	defineTool("list_feedback", "List feedback",
		"Lists feedback that customers sent from apps (bug reports, ideas, praise), newest activity first. Maps to GET /v1/conversations?kind=feedback.",
		readOnly, map[string][]any{"category": {"bug", "idea", "praise", "other"}, "status": conversationStatuses},
		mcpTool{ops: []string{"ListConversations"}},
		func(ctx context.Context, c *mcpCall, in listFeedbackIn) (mcpPage[mcpConversation], error) {
			kind := oas.ConversationKindFeedback
			return c.listConversations(ctx, oas.ListConversationsParams{
				Kind: &kind, InboxId: in.InboxID, Category: optString[oas.FeedbackCategory](in.Category),
				Status: optString[oas.ConversationStatus](in.Status), Q: optString[string](in.Query),
				Cursor: optString[string](in.Cursor), Limit: optLimit(in.Limit),
			})
		}),

	defineTool("draft_reply", "Draft a reply",
		"Stores a reply to the contact as a draft. Nothing is delivered: a member reviews, edits and sends it in Yuva. Maps to POST /v1/conversations/{id}/messages with draft: true.",
		additive, nil, mcpTool{ops: []string{"CreateMessage"}, scopes: []oas.ApiKeyScope{oas.MessagesWrite}},
		func(ctx context.Context, c *mcpCall, in replyIn) (mcpMessage, error) {
			return c.createMessage(ctx, in.ConversationID, oas.MessageCreate{
				Kind: oas.MessageCreateKindMessage, Direction: new(oas.Out), Body: &in.Body, Draft: boolPtr(true),
			})
		}),
	defineTool("send_reply", "Send a reply",
		"Sends a new reply to the contact now, by e-mail or chat. Prefer draft_reply unless the person you work for asked to send; to send a draft that exists, use send_draft. Maps to POST /v1/conversations/{id}/messages.",
		delivering, nil, mcpTool{ops: []string{"CreateMessage"}, scopes: []oas.ApiKeyScope{oas.MessagesWrite}, allow: mayDeliver},
		func(ctx context.Context, c *mcpCall, in replyIn) (mcpMessage, error) {
			if err := c.noPendingDraft(ctx, in.ConversationID); err != nil {
				return mcpMessage{}, err
			}
			return c.createMessage(ctx, in.ConversationID, oas.MessageCreate{
				Kind: oas.MessageCreateKindMessage, Direction: new(oas.Out), Body: &in.Body,
			})
		}),
	defineTool("send_draft", "Send a draft",
		"Delivers a draft that already exists, by e-mail or chat, so the conversation keeps one message instead of the draft and a copy. Use it when the person you work for asks to send a draft from draft_reply; never send_reply with the same text. Maps to POST /v1/messages/{id}/send.",
		delivering, nil, mcpTool{ops: []string{"SendMessage"}, allow: mayDeliver},
		func(ctx context.Context, c *mcpCall, in sendDraftIn) (mcpMessage, error) {
			res, err := c.s.SendMessage(ctx, oas.SendMessageRequestObject{MessageId: in.MessageID})
			if err != nil {
				return mcpMessage{}, err
			}
			m, ok := res.(oas.SendMessage200JSONResponse)
			if !ok {
				return mcpMessage{}, errInternal
			}
			return messageOut(oas.Message(m)), nil
		}),
	defineTool("add_note", "Add a note",
		"Adds an internal note that only members see. Maps to POST /v1/conversations/{id}/messages with kind note.",
		additive, nil, mcpTool{ops: []string{"CreateMessage"}, scopes: []oas.ApiKeyScope{oas.NotesWrite}},
		func(ctx context.Context, c *mcpCall, in replyIn) (mcpMessage, error) {
			return c.createMessage(ctx, in.ConversationID, oas.MessageCreate{Kind: oas.MessageCreateKindNote, Body: &in.Body})
		}),
	defineTool("assign", "Assign a conversation",
		"Assigns a conversation to a member, or unassigns it. Maps to PATCH /v1/conversations/{id}.",
		idempotent, nil, mcpTool{ops: []string{"UpdateConversation"}},
		func(ctx context.Context, c *mcpCall, in assignIn) (mcpConversation, error) {
			b := oas.ConversationUpdate{AssigneeId: nullable.NewNullNullable[uuid.UUID]()}
			if in.AssigneeID != nil {
				b.AssigneeId = nullable.NewNullableWithValue(*in.AssigneeID)
			}
			return c.updateConversation(ctx, in.ConversationID, b)
		}),
	defineTool("set_status", "Set a conversation's status",
		"Opens, closes or marks a conversation pending (use snooze to snooze). Maps to PATCH /v1/conversations/{id}.",
		idempotent, map[string][]any{"status": {"open", "pending", "closed"}}, mcpTool{ops: []string{"UpdateConversation"}},
		func(ctx context.Context, c *mcpCall, in setStatusIn) (mcpConversation, error) {
			st := oas.ConversationStatus(in.Status)
			if st == oas.ConversationStatusSnoozed {
				return mcpConversation{}, errValidation("use snooze to snooze a conversation")
			}
			return c.updateConversation(ctx, in.ConversationID, oas.ConversationUpdate{Status: &st})
		}),
	defineTool("snooze", "Snooze a conversation",
		"Snoozes a conversation until a time; it opens again then. Maps to PATCH /v1/conversations/{id}.",
		idempotent, nil, mcpTool{ops: []string{"UpdateConversation"}},
		func(ctx context.Context, c *mcpCall, in snoozeIn) (mcpConversation, error) {
			st := oas.ConversationStatusSnoozed
			return c.updateConversation(ctx, in.ConversationID, oas.ConversationUpdate{Status: &st, SnoozeUntil: &in.Until})
		}),
	defineTool("add_labels", "Add labels",
		"Adds labels to a conversation and keeps the others. Maps to POST /v1/conversations/bulk with add_labels.",
		idempotent, nil, mcpTool{ops: []string{"BulkUpdateConversations"}},
		func(ctx context.Context, c *mcpCall, in labelsIn) (mcpConversation, error) {
			return c.changeLabels(ctx, in, true)
		}),
	defineTool("remove_labels", "Remove labels",
		"Removes labels from a conversation. Maps to POST /v1/conversations/bulk with remove_labels.",
		idempotent, nil, mcpTool{ops: []string{"BulkUpdateConversations"}},
		func(ctx context.Context, c *mcpCall, in labelsIn) (mcpConversation, error) {
			return c.changeLabels(ctx, in, false)
		}),
	defineTool("move_conversation", "Move a conversation",
		"Moves a conversation, with its messages, notes and labels, to another inbox you can see. Maps to POST /v1/conversations/{id}/move.",
		idempotent, nil, mcpTool{ops: []string{"MoveConversation"}},
		func(ctx context.Context, c *mcpCall, in moveIn) (mcpConversation, error) {
			res, err := c.s.MoveConversation(ctx, oas.MoveConversationRequestObject{ConversationId: in.ConversationID, Body: &oas.ConversationMove{InboxId: in.InboxID}})
			if err != nil {
				return mcpConversation{}, err
			}
			return conversationOut(oas.Conversation(res.(oas.MoveConversation200JSONResponse))), nil
		}),
	defineTool("bulk_update", "Change several conversations",
		"Applies one change to up to 100 conversations: status, assignee, labels to add and to remove. Each conversation is changed on its own; refused ones are listed in failed. Maps to POST /v1/conversations/bulk.",
		idempotent, map[string][]any{"status": conversationStatuses}, mcpTool{ops: []string{"BulkUpdateConversations"}},
		func(ctx context.Context, c *mcpCall, in bulkIn) (bulkOut, error) {
			b := oas.ConversationBulkUpdate{
				ConversationIds: in.ConversationIDs, Status: optString[oas.ConversationStatus](in.Status), SnoozeUntil: in.SnoozeUntil,
			}
			switch {
			case in.Unassign && in.AssigneeID != nil:
				return bulkOut{}, errValidation("send assignee_id or unassign, not both")
			case in.Unassign:
				b.AssigneeId = nullable.NewNullNullable[uuid.UUID]()
			case in.AssigneeID != nil:
				b.AssigneeId = nullable.NewNullableWithValue(*in.AssigneeID)
			}
			if in.AddLabels != nil {
				b.AddLabels = &in.AddLabels
			}
			if in.RemoveLabels != nil {
				b.RemoveLabels = &in.RemoveLabels
			}
			return c.bulk(ctx, b)
		}),
	defineTool("merge_contacts", "Merge two contacts",
		"Merges source_id into contact_id: conversations, addresses and external ids move over and the source contact is deleted. This cannot be undone. Owners and admins only. Maps to POST /v1/contacts/{id}/merge.",
		destructive, nil, mcpTool{ops: []string{"MergeContact"}, allow: func(p principal) bool { return requireManagerOrFullKey(p) == nil }},
		func(ctx context.Context, c *mcpCall, in mergeIn) (mcpContact, error) {
			res, err := c.s.MergeContact(ctx, oas.MergeContactRequestObject{ContactId: in.ContactID, Body: &oas.ContactMerge{SourceId: in.SourceID}})
			if err != nil {
				return mcpContact{}, err
			}
			return contactOut(oas.Contact(res.(oas.MergeContact200JSONResponse))), nil
		}),
}

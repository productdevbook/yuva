package api

import (
	"context"
	"net/http"
	"reflect"
	"time"
	"uuid"

	"github.com/oapi-codegen/runtime/types"

	"github.com/productdevbook/yuva/api/internal/oas"
)

var (
	errManagersOnly = problem(http.StatusForbidden, "forbidden", "only owners, admins and API keys may do this")
	errNoGrantSetup = problem(http.StatusForbidden, "forbidden", "only owners, admins and API keys set this up; an OAuth connection cannot, so use an API key or the panel")
	botSendingNote  = "sending is off for assistants in this workspace, so the message was stored as a draft for a member to send"
)

func managerTool(ops ...string) mcpTool {
	return mcpTool{ops: ops, allow: mayManage, denied: errManagersOnly}
}

// publicSetupTool is for operations whose handler refuses OAuth grants because they change what the public sees.
func publicSetupTool(ops ...string) mcpTool {
	return mcpTool{ops: ops, allow: func(p principal) bool { return !p.isGrant() && mayManage(p) }, denied: errNoGrantSetup}
}

type mcpMember struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Role         string    `json:"role" jsonschema:"owner, admin or agent"`
	Availability string    `json:"availability" jsonschema:"auto or away"`
	Online       bool      `json:"online"`
	Email        string    `json:"email,omitempty" jsonschema:"given to owners and admins only"`
}

type listMembersIn struct{}

type searchContactsIn struct {
	Query   string     `json:"query,omitempty" jsonschema:"full-text search over names, addresses and external ids"`
	Kind    string     `json:"kind,omitempty" jsonschema:"known has an address or external id; visitor has neither"`
	HasOpen *bool      `json:"has_open,omitempty" jsonschema:"true lists only contacts with an open conversation you can see, false only those without"`
	InboxID *uuid.UUID `json:"inbox_id,omitempty" jsonschema:"only contacts with a conversation or an external id in this inbox"`
	Sort    string     `json:"sort,omitempty" jsonschema:"created or last_seen"`
	Cursor  string     `json:"cursor,omitempty"`
	Limit   int32      `json:"limit,omitempty" jsonschema:"1 to 100, default 25"`
}

type createContactIn struct {
	Name        string          `json:"name,omitempty"`
	Emails      []string        `json:"emails,omitempty"`
	ExternalIDs []mcpExternalID `json:"external_ids,omitempty" jsonschema:"the contact's ids in the host apps, per inbox"`
	Attributes  map[string]any  `json:"attributes,omitempty" jsonschema:"plan, app version or anything else worth showing; at most 16 KiB"`
}

type updateContactIn struct {
	ContactID  uuid.UUID      `json:"contact_id"`
	Name       *string        `json:"name,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty" jsonschema:"replaces all the contact's attributes, so send the whole object; an empty object clears them"`
}

type contactNotesIn struct {
	ContactID uuid.UUID `json:"contact_id"`
	Cursor    string    `json:"cursor,omitempty"`
	Limit     int32     `json:"limit,omitempty" jsonschema:"1 to 100, default 25"`
}

type addContactNoteIn struct {
	ContactID uuid.UUID `json:"contact_id"`
	Body      string    `json:"body" jsonschema:"plain text, at most 10000 characters"`
}

type mcpContactNoteContent struct {
	Body string `json:"body"`
}

type mcpContactNote struct {
	ID              uuid.UUID             `json:"id"`
	ContactID       uuid.UUID             `json:"contact_id"`
	Author          mcpAuthor             `json:"author"`
	CreatedAt       time.Time             `json:"created_at"`
	CustomerContent mcpContactNoteContent `json:"customer_content"`
}

func contactNoteOut(n oas.ContactNote) mcpContactNote {
	author, _ := authorOut(n.Author)
	return mcpContactNote{ID: n.Id, ContactID: n.ContactId, Author: author, CreatedAt: n.CreatedAt, CustomerContent: mcpContactNoteContent{Body: n.Body}}
}

type mcpContactSubscriptionContent struct {
	Email string `json:"email"`
}

type mcpContactSubscription struct {
	ID              uuid.UUID                     `json:"id"`
	ListID          uuid.UUID                     `json:"list_id"`
	ListName        string                        `json:"list_name"`
	ListKind        string                        `json:"list_kind" jsonschema:"list for a newsletter or announcement list, status for the e-mail subscribers of a status page"`
	State           string                        `json:"state"`
	Source          string                        `json:"source"`
	ConsentAt       *time.Time                    `json:"consent_at,omitempty"`
	UnsubscribedAt  *time.Time                    `json:"unsubscribed_at,omitempty"`
	CreatedAt       time.Time                     `json:"created_at"`
	CustomerContent mcpContactSubscriptionContent `json:"customer_content"`
}

type startConversationIn struct {
	InboxID    uuid.UUID   `json:"inbox_id"`
	ContactID  uuid.UUID   `json:"contact_id"`
	Subject    string      `json:"subject,omitempty"`
	Body       string      `json:"body" jsonschema:"the first message to the contact, plain text"`
	Send       bool        `json:"send,omitempty" jsonschema:"true delivers the message now, only when the workspace lets assistants deliver and the person you work for asked to send; otherwise it is stored as a draft"`
	ChannelID  *uuid.UUID  `json:"channel_id,omitempty" jsonschema:"a channel of the inbox to write through"`
	AssigneeID *uuid.UUID  `json:"assignee_id,omitempty" jsonschema:"a member from list_members with access to the inbox"`
	LabelIDs   []uuid.UUID `json:"label_ids,omitempty"`
	Priority   string      `json:"priority,omitempty"`
}

type startConversationOut struct {
	Conversation mcpConversation `json:"conversation"`
	Message      mcpMessage      `json:"message"`
	Note         string          `json:"note,omitempty"`
}

type listDraftsIn struct {
	InboxID   *uuid.UUID `json:"inbox_id,omitempty"`
	WrittenBy string     `json:"written_by,omitempty" jsonschema:"bot, assistant or member"`
	Cursor    string     `json:"cursor,omitempty"`
	Limit     int32      `json:"limit,omitempty" jsonschema:"1 to 100, default 25"`
}

type updateDraftIn struct {
	MessageID uuid.UUID `json:"message_id" jsonschema:"a draft from list_drafts, draft_reply or start_conversation"`
	Body      *string   `json:"body,omitempty" jsonschema:"the new text, plain; unchanged when absent"`
	Subject   *string   `json:"subject,omitempty" jsonschema:"the new subject of a new e-mail draft (compose_mail); unchanged when absent"`
}

type discardDraftIn struct {
	MessageID uuid.UUID `json:"message_id" jsonschema:"a draft from list_drafts, draft_reply or start_conversation"`
}

type discardDraftOut struct {
	MessageID uuid.UUID `json:"message_id"`
	Discarded bool      `json:"discarded"`
}

type mcpDraftConversationContent struct {
	Subject      string `json:"subject"`
	ContactName  string `json:"contact_name,omitempty"`
	ContactEmail string `json:"contact_email,omitempty"`
}

type mcpDraftConversation struct {
	ID              uuid.UUID                   `json:"id"`
	InboxID         uuid.UUID                   `json:"inbox_id"`
	Status          string                      `json:"status"`
	CustomerContent mcpDraftConversationContent `json:"customer_content"`
}

type mcpDraftItem struct {
	Conversation mcpDraftConversation `json:"conversation"`
	Draft        mcpMessage           `json:"draft"`
}

type mcpDraftPage struct {
	Items      []mcpDraftItem `json:"items"`
	Total      int64          `json:"total" jsonschema:"drafts matching the filters over all pages"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type createLabelIn struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty" jsonschema:"a hex color such as #6b7280; that grey when empty"`
}

type createCannedReplyIn struct {
	Title    string `json:"title"`
	Shortcut string `json:"shortcut" jsonschema:"lower-case letters, digits and hyphens that members type to find it"`
	Body     string `json:"body" jsonschema:"the reply text members insert"`
}

type updateCannedReplyIn struct {
	CannedReplyID uuid.UUID `json:"canned_reply_id" jsonschema:"from list_canned_replies"`
	Title         *string   `json:"title,omitempty"`
	Shortcut      *string   `json:"shortcut,omitempty"`
	Body          *string   `json:"body,omitempty"`
}

func cannedReplyOut(r oas.CannedReply) mcpCannedReply {
	return mcpCannedReply{ID: r.Id, Title: r.Title, Shortcut: r.Shortcut, Body: r.Body}
}

type listDiscussionsIn struct {
	InboxID  *uuid.UUID `json:"inbox_id,omitempty"`
	Site     string     `json:"site,omitempty" jsonschema:"only this site's pages: an origin such as https://example.com"`
	Page     string     `json:"page,omitempty" jsonschema:"only this page's threads, a URL"`
	State    string     `json:"state,omitempty" jsonschema:"published, pending or hidden"`
	Answered *bool      `json:"answered,omitempty"`
	Locked   *bool      `json:"locked,omitempty"`
	Pending  *bool      `json:"pending,omitempty" jsonschema:"true lists only threads with posts waiting for a member"`
	Sort     string     `json:"sort,omitempty" jsonschema:"activity (default), new, top or votes"`
	Cursor   string     `json:"cursor,omitempty"`
	Limit    int32      `json:"limit,omitempty" jsonschema:"1 to 100, default 25"`
}

type discussionIn struct {
	DiscussionID uuid.UUID `json:"discussion_id" jsonschema:"a thread from list_discussions"`
}

type mcpDiscussionContent struct {
	Title      string `json:"title"`
	Body       string `json:"body,omitempty" jsonschema:"the opening post, Markdown as the reader wrote it; a preview in lists"`
	Page       string `json:"page,omitempty" jsonschema:"the documentation page the thread is on"`
	PageTitle  string `json:"page_title,omitempty"`
	AuthorName string `json:"author_name,omitempty"`
}

type mcpDiscussion struct {
	ID              uuid.UUID            `json:"id"`
	InboxID         *uuid.UUID           `json:"inbox_id,omitempty"`
	ConversationID  *uuid.UUID           `json:"conversation_id,omitempty" jsonschema:"the conversation it was started from"`
	Kind            string               `json:"kind" jsonschema:"page, or idea for a roadmap idea"`
	State           string               `json:"state" jsonschema:"published, pending (waits for a member) or hidden"`
	Locked          bool                 `json:"locked"`
	Answered        bool                 `json:"answered"`
	Votes           int32                `json:"votes"`
	Replies         int32                `json:"replies"`
	PendingPosts    int32                `json:"pending_posts,omitempty"`
	HiddenPosts     int32                `json:"hidden_posts,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
	LastActivityAt  time.Time            `json:"last_activity_at"`
	CustomerContent mcpDiscussionContent `json:"customer_content"`
}

func discussionOut(d oas.Discussion, body bool) mcpDiscussion {
	out := mcpDiscussion{
		ID: d.Id, InboxID: d.InboxId, ConversationID: d.ConversationId, Kind: string(d.Kind), State: string(d.State), Locked: d.Locked,
		Answered: d.AnsweredAt != nil, Votes: d.Votes, Replies: d.ReplyCount, CreatedAt: d.CreatedAt, LastActivityAt: d.LastActivityAt,
		CustomerContent: mcpDiscussionContent{Title: d.Title, Page: d.Page, PageTitle: deref(d.PageTitle), AuthorName: deref(d.Author.Name)},
	}
	if d.PendingPosts != nil {
		out.PendingPosts = *d.PendingPosts
	}
	if d.HiddenPosts != nil {
		out.HiddenPosts = *d.HiddenPosts
	}
	if body {
		out.CustomerContent.Body = d.Body
	} else {
		out.CustomerContent.Body = postExcerpt(d.Body)
	}
	return out
}

type mcpPostContent struct {
	Body       string `json:"body"`
	AuthorName string `json:"author_name,omitempty"`
}

type mcpPost struct {
	ID              uuid.UUID      `json:"id"`
	ParentPostID    *uuid.UUID     `json:"parent_post_id,omitempty"`
	AuthorKind      string         `json:"author_kind" jsonschema:"member, reader or anonymous"`
	MemberID        *uuid.UUID     `json:"member_id,omitempty"`
	ContactID       *uuid.UUID     `json:"contact_id,omitempty"`
	Via             string         `json:"via,omitempty" jsonschema:"the assistant or app a member wrote it with"`
	State           string         `json:"state"`
	Accepted        bool           `json:"accepted" jsonschema:"it is the thread's accepted answer"`
	Deleted         bool           `json:"deleted"`
	Votes           int32          `json:"votes"`
	CreatedAt       time.Time      `json:"created_at"`
	CustomerContent mcpPostContent `json:"customer_content"`
}

func postOut(p oas.DiscussionPost) mcpPost {
	return mcpPost{
		ID: p.Id, ParentPostID: p.ParentPostId, AuthorKind: string(p.Author.Kind), MemberID: p.Author.MemberId, ContactID: p.Author.ContactId,
		Via: deref(p.Author.Via), State: string(p.State), Accepted: p.Accepted, Deleted: p.Deleted, Votes: p.Votes, CreatedAt: p.CreatedAt,
		CustomerContent: mcpPostContent{Body: p.Body, AuthorName: deref(p.Author.Name)},
	}
}

type replyToDiscussionIn struct {
	DiscussionID uuid.UUID  `json:"discussion_id" jsonschema:"a thread from list_discussions"`
	Body         string     `json:"body" jsonschema:"Markdown, up to 20000 characters"`
	ParentPostID *uuid.UUID `json:"parent_post_id,omitempty" jsonschema:"a reply to the thread itself from get_discussion, to answer it; absent replies to the thread"`
}

type markDiscussionAnswerIn struct {
	DiscussionID uuid.UUID  `json:"discussion_id" jsonschema:"a thread from list_discussions"`
	PostID       *uuid.UUID `json:"post_id,omitempty" jsonschema:"the reply to mark, a published reply to the thread itself from get_discussion"`
	Clear        bool       `json:"clear,omitempty" jsonschema:"true clears the accepted answer instead; leave post_id out"`
}

type mcpDiscussionDetail struct {
	Discussion mcpDiscussion `json:"discussion"`
	Posts      []mcpPost     `json:"posts" jsonschema:"the replies, the accepted answer first, then oldest first; not the opening post"`
}

var mcpContactTools = []mcpTool{
	defineTool("list_members", "List team members",
		"Lists the workspace's members with their role and whether they are online, so assign has ids. Addresses are given to owners and admins only. Maps to GET /v1/members.",
		readOnly, nil, mcpTool{ops: []string{"ListMembers"}},
		func(ctx context.Context, c *mcpCall, _ listMembersIn) (mcpPage[mcpMember], error) {
			res, err := c.s.ListMembers(ctx, oas.ListMembersRequestObject{})
			if err != nil {
				return mcpPage[mcpMember]{}, err
			}
			out := mcpPage[mcpMember]{Items: []mcpMember{}}
			for _, m := range res.(oas.ListMembers200JSONResponse).Items {
				item := mcpMember{ID: m.Id, Name: m.Name, Role: string(m.Role), Availability: string(m.Availability), Online: m.Online}
				if mayManage(c.p) {
					item.Email = string(m.Email)
				}
				out.Items = append(out.Items, item)
			}
			return out, nil
		}),
	defineTool("search_contacts", "Search contacts",
		"Lists the contacts you can see, with full-text search and filters. Maps to GET /v1/contacts.",
		readOnly, map[string][]any{"kind": {"known", "visitor"}, "sort": {"created", "last_seen"}}, mcpTool{ops: []string{"ListContacts"}},
		func(ctx context.Context, c *mcpCall, in searchContactsIn) (mcpPage[mcpContact], error) {
			res, err := c.s.ListContacts(ctx, oas.ListContactsRequestObject{Params: oas.ListContactsParams{
				Q: optString[string](in.Query), Kind: optString[oas.ListContactsParamsKind](in.Kind), HasOpen: in.HasOpen, InboxId: in.InboxID,
				Sort: optString[oas.ListContactsParamsSort](in.Sort), Cursor: optString[string](in.Cursor), Limit: optLimit(in.Limit),
			}})
			if err != nil {
				return mcpPage[mcpContact]{}, err
			}
			page := oas.ContactPage(res.(oas.ListContacts200JSONResponse))
			out := mcpPage[mcpContact]{Items: []mcpContact{}}
			for _, ct := range page.Items {
				out.Items = append(out.Items, contactOut(ct))
			}
			if page.NextCursor != nil {
				out.NextCursor = *page.NextCursor
			}
			return out, nil
		}),
	defineTool("create_contact", "Create a contact",
		"Creates a contact with a name, addresses, host-app ids and attributes, for start_conversation or add_note. An address or external id that already belongs to a contact is refused; find that one with search_contacts. Maps to POST /v1/contacts.",
		additive, nil, mcpTool{ops: []string{"CreateContact"}},
		func(ctx context.Context, c *mcpCall, in createContactIn) (mcpContact, error) {
			body := oas.ContactCreate{Name: optString[string](in.Name)}
			if in.Emails != nil {
				emails := make([]oas.Email, len(in.Emails))
				for i, e := range in.Emails {
					emails[i] = types.Email(e)
				}
				body.Emails = &emails
			}
			if in.ExternalIDs != nil {
				ids := make([]oas.ExternalId, len(in.ExternalIDs))
				for i, e := range in.ExternalIDs {
					ids[i] = oas.ExternalId{InboxId: e.InboxID, ExternalId: e.ExternalID}
				}
				body.ExternalIds = &ids
			}
			if in.Attributes != nil {
				attrs := oas.Attributes(in.Attributes)
				body.Attributes = &attrs
			}
			res, err := c.s.CreateContact(ctx, oas.CreateContactRequestObject{Body: &body})
			if err != nil {
				return mcpContact{}, err
			}
			return contactOut(oas.Contact(res.(oas.CreateContact201JSONResponse))), nil
		}),
	defineTool("update_contact", "Update a contact",
		"Changes a contact's name and attributes. Attributes are replaced as a whole. Addresses, locale and blocking stay as they are. Maps to PATCH /v1/contacts/{id}.",
		idempotent, nil, mcpTool{ops: []string{"UpdateContact"}},
		func(ctx context.Context, c *mcpCall, in updateContactIn) (mcpContact, error) {
			body := oas.ContactUpdate{Name: in.Name}
			if in.Attributes != nil {
				attrs := oas.Attributes(in.Attributes)
				body.Attributes = &attrs
			}
			if reflect.DeepEqual(body, oas.ContactUpdate{}) {
				return mcpContact{}, errNothingToUpdate
			}
			res, err := c.s.UpdateContact(ctx, oas.UpdateContactRequestObject{ContactId: in.ContactID, Body: &body})
			if err != nil {
				return mcpContact{}, err
			}
			return contactOut(oas.Contact(res.(oas.UpdateContact200JSONResponse))), nil
		}),
	defineTool("list_contact_notes", "List a contact's notes",
		"Lists the internal notes members wrote on a contact, newest first. Maps to GET /v1/contacts/{id}/notes.",
		readOnly, nil, mcpTool{ops: []string{"ListContactNotes"}},
		func(ctx context.Context, c *mcpCall, in contactNotesIn) (mcpPage[mcpContactNote], error) {
			res, err := c.s.ListContactNotes(ctx, oas.ListContactNotesRequestObject{ContactId: in.ContactID, Params: oas.ListContactNotesParams{
				Cursor: optString[string](in.Cursor), Limit: optLimit(in.Limit),
			}})
			if err != nil {
				return mcpPage[mcpContactNote]{}, err
			}
			page := oas.ContactNotePage(res.(oas.ListContactNotes200JSONResponse))
			out := mcpPage[mcpContactNote]{Items: []mcpContactNote{}}
			for _, n := range page.Items {
				out.Items = append(out.Items, contactNoteOut(n))
			}
			if page.NextCursor != nil {
				out.NextCursor = *page.NextCursor
			}
			return out, nil
		}),
	defineTool("add_contact_note", "Add a note to a contact",
		"Adds an internal note to a contact that only members see, for what holds across conversations; use add_note for one conversation. Maps to POST /v1/contacts/{id}/notes.",
		additive, nil, mcpTool{ops: []string{"CreateContactNote"}},
		func(ctx context.Context, c *mcpCall, in addContactNoteIn) (mcpContactNote, error) {
			res, err := c.s.CreateContactNote(ctx, oas.CreateContactNoteRequestObject{ContactId: in.ContactID, Body: &oas.ContactNoteCreate{Body: in.Body}})
			if err != nil {
				return mcpContactNote{}, err
			}
			return contactNoteOut(oas.ContactNote(res.(oas.CreateContactNote201JSONResponse))), nil
		}),
	defineTool("list_contact_subscriptions", "List a contact's subscriptions",
		"Lists the mailing lists and status pages a contact is subscribed to, pending on or left, with the consent state. The address is in customer_content. Maps to GET /v1/contacts/{id}/subscriptions.",
		readOnly, nil, mcpTool{ops: []string{"ListContactSubscriptions"}},
		func(ctx context.Context, c *mcpCall, in contactIn) (mcpPage[mcpContactSubscription], error) {
			res, err := c.s.ListContactSubscriptions(ctx, oas.ListContactSubscriptionsRequestObject{ContactId: in.ContactID})
			if err != nil {
				return mcpPage[mcpContactSubscription]{}, err
			}
			out := mcpPage[mcpContactSubscription]{Items: []mcpContactSubscription{}}
			for _, s := range oas.ContactSubscriptionList(res.(oas.ListContactSubscriptions200JSONResponse)).Items {
				out.Items = append(out.Items, mcpContactSubscription{
					ID: s.Id, ListID: s.ListId, ListName: s.List.Name, ListKind: string(s.List.Kind), State: string(s.State), Source: string(s.Source),
					ConsentAt: s.ConsentAt, UnsubscribedAt: s.UnsubscribedAt, CreatedAt: s.CreatedAt,
					CustomerContent: mcpContactSubscriptionContent{Email: string(s.Email)},
				})
			}
			return out, nil
		}),
	defineTool("start_conversation", "Start a conversation",
		"Starts a conversation with a contact in an inbox and writes the first outgoing message. The message is a draft for a member to review unless the workspace lets assistants deliver and send is true. Maps to POST /v1/conversations and POST /v1/conversations/{id}/messages.",
		delivering, map[string][]any{"priority": {"low", "normal", "high", "urgent"}},
		mcpTool{ops: []string{"CreateConversation", "CreateMessage"}, scopes: []oas.ApiKeyScope{oas.MessagesWrite}},
		func(ctx context.Context, c *mcpCall, in startConversationIn) (startConversationOut, error) {
			body := oas.ConversationCreate{InboxId: in.InboxID, ContactId: in.ContactID, Subject: optString[string](in.Subject), ChannelId: in.ChannelID,
				AssigneeId: in.AssigneeID, Priority: optString[oas.Priority](in.Priority)}
			if in.LabelIDs != nil {
				body.Labels = &in.LabelIDs
			}
			if in.Body == "" {
				return startConversationOut{}, errValidation("body is the first message to the contact")
			}
			res, err := c.s.CreateConversation(ctx, oas.CreateConversationRequestObject{Body: &body})
			if err != nil {
				return startConversationOut{}, err
			}
			conv := oas.Conversation(res.(oas.CreateConversation201JSONResponse))
			out := startConversationOut{Conversation: conversationOut(conv)}
			draft := !in.Send || !mayDeliver(c.p)
			if in.Send && draft {
				out.Note = botSendingNote
			}
			msg := oas.MessageCreate{Kind: oas.MessageCreateKindMessage, Direction: new(oas.Out), Body: &in.Body}
			if draft {
				msg.Draft = boolPtr(true)
			}
			if out.Message, err = c.createMessage(ctx, conv.Id, msg); err != nil {
				return out, err
			}
			return out, nil
		}),
	defineTool("list_drafts", "List drafts waiting for review",
		"Lists the unsent drafts in the inboxes you can see, newest first, with the conversation each belongs to. Send one with send_draft. Maps to GET /v1/drafts.",
		readOnly, map[string][]any{"written_by": {"bot", "assistant", "member"}}, mcpTool{ops: []string{"ListDrafts"}},
		func(ctx context.Context, c *mcpCall, in listDraftsIn) (mcpDraftPage, error) {
			res, err := c.s.ListDrafts(ctx, oas.ListDraftsRequestObject{Params: oas.ListDraftsParams{
				InboxId: in.InboxID, Author: optString[oas.DraftAuthorKind](in.WrittenBy), Cursor: optString[string](in.Cursor), Limit: optLimit(in.Limit),
			}})
			if err != nil {
				return mcpDraftPage{}, err
			}
			page := oas.DraftPage(res.(oas.ListDrafts200JSONResponse))
			out := mcpDraftPage{Items: []mcpDraftItem{}, Total: page.Total}
			for _, it := range page.Items {
				cv := it.Conversation
				item := mcpDraftItem{
					Conversation: mcpDraftConversation{ID: cv.Id, InboxID: cv.InboxId, Status: string(cv.Status), CustomerContent: mcpDraftConversationContent{
						Subject: cv.Subject, ContactName: cv.Contact.Name,
					}},
					Draft: messageOut(it.Draft),
				}
				if cv.Contact.Email != nil {
					item.Conversation.CustomerContent.ContactEmail = string(*cv.Contact.Email)
				}
				out.Items = append(out.Items, item)
			}
			if page.NextCursor != nil {
				out.NextCursor = *page.NextCursor
			}
			return out, nil
		}),
	defineTool("update_draft", "Change a draft",
		"Rewrites an unsent draft: body replaces its text and subject, for a new e-mail draft, its subject. Only a draft can change, and a member's own draft only by that member; a sent message is refused. Maps to PATCH /v1/messages/{id}.",
		idempotent, nil, mcpTool{ops: []string{"UpdateMessage"}},
		func(ctx context.Context, c *mcpCall, in updateDraftIn) (mcpMessage, error) {
			if in.Body == nil && in.Subject == nil {
				return mcpMessage{}, errValidation("give a body or a subject")
			}
			res, err := c.s.UpdateMessage(ctx, oas.UpdateMessageRequestObject{MessageId: in.MessageID, Body: &oas.MessageUpdate{Body: in.Body, Subject: in.Subject}})
			if err != nil {
				return mcpMessage{}, err
			}
			m, ok := res.(oas.UpdateMessage200JSONResponse)
			if !ok {
				return mcpMessage{}, errInternal
			}
			return messageOut(oas.Message(m)), nil
		}),
	defineTool("discard_draft", "Discard a draft",
		"Deletes an unsent draft for good; a draft that started its conversation (compose_mail) takes that conversation with it. Only a draft can be discarded, and a member's own draft only by that member. Maps to DELETE /v1/messages/{id}.",
		destructive, nil, mcpTool{ops: []string{"DeleteMessage"}},
		func(ctx context.Context, c *mcpCall, in discardDraftIn) (discardDraftOut, error) {
			if _, err := c.s.DeleteMessage(ctx, oas.DeleteMessageRequestObject{MessageId: in.MessageID}); err != nil {
				return discardDraftOut{}, err
			}
			return discardDraftOut{MessageID: in.MessageID, Discarded: true}, nil
		}),
	defineTool("create_label", "Create a label",
		"Creates a label conversations can be tagged with. Owners, admins and keys only. Maps to POST /v1/labels.",
		additive, nil, managerTool("CreateLabel"),
		func(ctx context.Context, c *mcpCall, in createLabelIn) (mcpLabel, error) {
			res, err := c.s.CreateLabel(ctx, oas.CreateLabelRequestObject{Body: &oas.LabelCreate{Name: in.Name, Color: optString[string](in.Color)}})
			if err != nil {
				return mcpLabel{}, err
			}
			l := oas.Label(res.(oas.CreateLabel201JSONResponse))
			return mcpLabel{ID: l.Id, Name: l.Name, Color: l.Color}, nil
		}),
	defineTool("create_canned_reply", "Create a canned reply",
		"Saves a reply members insert while writing, found by its shortcut. Owners, admins and keys only. Maps to POST /v1/canned-replies.",
		additive, nil, managerTool("CreateCannedReply"),
		func(ctx context.Context, c *mcpCall, in createCannedReplyIn) (mcpCannedReply, error) {
			res, err := c.s.CreateCannedReply(ctx, oas.CreateCannedReplyRequestObject{Body: &oas.CannedReplyCreate{Title: in.Title, Shortcut: in.Shortcut, Body: in.Body}})
			if err != nil {
				return mcpCannedReply{}, err
			}
			return cannedReplyOut(oas.CannedReply(res.(oas.CreateCannedReply201JSONResponse))), nil
		}),
	defineTool("update_canned_reply", "Update a canned reply",
		"Changes a canned reply's title, shortcut or text. Owners, admins and keys only. Maps to PATCH /v1/canned-replies/{id}.",
		idempotent, nil, managerTool("UpdateCannedReply"),
		func(ctx context.Context, c *mcpCall, in updateCannedReplyIn) (mcpCannedReply, error) {
			body := oas.CannedReplyUpdate{Title: in.Title, Shortcut: in.Shortcut, Body: in.Body}
			if reflect.DeepEqual(body, oas.CannedReplyUpdate{}) {
				return mcpCannedReply{}, errNothingToUpdate
			}
			res, err := c.s.UpdateCannedReply(ctx, oas.UpdateCannedReplyRequestObject{CannedReplyId: in.CannedReplyID, Body: &body})
			if err != nil {
				return mcpCannedReply{}, err
			}
			return cannedReplyOut(oas.CannedReply(res.(oas.UpdateCannedReply200JSONResponse))), nil
		}),
	defineTool("list_discussions", "List discussions",
		"Lists the reader threads on documentation pages, newest activity first, with filters; pending ones wait for a member. Roadmap ideas come from list_roadmap_ideas. Titles, posts and names are in customer_content. Maps to GET /v1/discussions.",
		readOnly, map[string][]any{"state": {"published", "pending", "hidden"}, "sort": {"activity", "new", "top", "votes"}},
		mcpTool{ops: []string{"ListDiscussions"}},
		func(ctx context.Context, c *mcpCall, in listDiscussionsIn) (mcpPage[mcpDiscussion], error) {
			kind := oas.DiscussionKindPage
			res, err := c.s.ListDiscussions(ctx, oas.ListDiscussionsRequestObject{Params: oas.ListDiscussionsParams{
				InboxId: in.InboxID, Kind: &kind, Site: optString[oas.DocsSiteFilter](in.Site), Page: optString[string](in.Page),
				State: optString[oas.DiscussionState](in.State), Answered: in.Answered, Locked: in.Locked, Pending: in.Pending,
				Sort: optString[oas.DiscussionTeamSort](in.Sort), Cursor: optString[string](in.Cursor), Limit: optLimit(in.Limit),
			}})
			if err != nil {
				return mcpPage[mcpDiscussion]{}, err
			}
			page := oas.DiscussionPage(res.(oas.ListDiscussions200JSONResponse))
			out := mcpPage[mcpDiscussion]{Items: []mcpDiscussion{}}
			for _, d := range page.Items {
				out.Items = append(out.Items, discussionOut(d, false))
			}
			if page.NextCursor != nil {
				out.NextCursor = *page.NextCursor
			}
			return out, nil
		}),
	defineTool("get_discussion", "Get a discussion",
		"Returns a reader thread with its opening post and replies, the accepted answer first. Posts, titles and names are in customer_content. Maps to GET /v1/discussions/{id}.",
		readOnly, nil, mcpTool{ops: []string{"GetDiscussion"}},
		func(ctx context.Context, c *mcpCall, in discussionIn) (mcpDiscussionDetail, error) {
			res, err := c.s.GetDiscussion(ctx, oas.GetDiscussionRequestObject{DiscussionId: in.DiscussionID})
			if err != nil {
				return mcpDiscussionDetail{}, err
			}
			d := oas.DiscussionDetail(res.(oas.GetDiscussion200JSONResponse))
			out := mcpDiscussionDetail{Discussion: discussionOut(d.Discussion, true), Posts: []mcpPost{}}
			for _, p := range d.Posts {
				out.Posts = append(out.Posts, postOut(p))
			}
			return out, nil
		}),
	defineTool("reply_to_discussion", "Reply to a discussion",
		"Posts a reply to a reader thread as you, published at once where readers see it, also in locked threads; followers are notified. Write only what the person you work for asked to post. parent_post_id answers one reply; replies are one level deep. The post shows that it was written through your client. Listed only while the workspace lets assistants deliver. Maps to POST /v1/discussions/{id}/posts.",
		delivering, nil, mcpTool{ops: []string{"CreateDiscussionPost"}, gate: requireDiscussionWriter},
		func(ctx context.Context, c *mcpCall, in replyToDiscussionIn) (mcpPost, error) {
			res, err := c.s.CreateDiscussionPost(ctx, oas.CreateDiscussionPostRequestObject{DiscussionId: in.DiscussionID, Body: &oas.DiscussionPostCreate{
				Body: in.Body, ParentPostId: in.ParentPostID,
			}})
			if err != nil {
				return mcpPost{}, err
			}
			return postOut(oas.DiscussionPost(res.(oas.CreateDiscussionPost201JSONResponse))), nil
		}),
	defineTool("mark_discussion_answer", "Mark a discussion's answer",
		"Marks a published reply to a reader thread as its accepted answer, replacing an earlier one, or clears it with clear: true. Marking the answer that is already marked, or clearing a thread without one, changes nothing. Followers are told when an answer is marked. Roadmap ideas have no answer. Listed only while the workspace lets assistants deliver. Maps to PUT and DELETE /v1/discussions/{id}/answer.",
		idempotentPublic, nil, mcpTool{ops: []string{"AcceptDiscussionAnswer", "ClearDiscussionAnswer"}, gate: requireDiscussionWriter},
		func(ctx context.Context, c *mcpCall, in markDiscussionAnswerIn) (mcpDiscussion, error) {
			if in.Clear == (in.PostID != nil) {
				return mcpDiscussion{}, errValidation("give post_id to mark an answer, or clear: true to clear it")
			}
			if in.Clear {
				res, err := c.s.ClearDiscussionAnswer(ctx, oas.ClearDiscussionAnswerRequestObject{DiscussionId: in.DiscussionID})
				if err != nil {
					return mcpDiscussion{}, err
				}
				return discussionOut(oas.Discussion(res.(oas.ClearDiscussionAnswer200JSONResponse)), false), nil
			}
			res, err := c.s.AcceptDiscussionAnswer(ctx, oas.AcceptDiscussionAnswerRequestObject{DiscussionId: in.DiscussionID, Body: &oas.DiscussionAnswer{PostId: *in.PostID}})
			if err != nil {
				return mcpDiscussion{}, err
			}
			return discussionOut(oas.Discussion(res.(oas.AcceptDiscussionAnswer200JSONResponse)), false), nil
		}),
}

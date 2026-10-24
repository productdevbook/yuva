package api

import (
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
)

type mcpConversationContent struct {
	Subject      string            `json:"subject" jsonschema:"the conversation's subject"`
	ContactName  string            `json:"contact_name,omitempty" jsonschema:"the contact's name"`
	ContactEmail string            `json:"contact_email,omitempty" jsonschema:"the contact's first e-mail address"`
	LastMessage  string            `json:"last_message,omitempty" jsonschema:"a preview of the last message"`
	Feedback     map[string]string `json:"feedback,omitempty" jsonschema:"what the customer's app reported with feedback: app version, device, screen and so on"`
}

type mcpConversation struct {
	ID                 uuid.UUID              `json:"id"`
	InboxID            uuid.UUID              `json:"inbox_id"`
	ContactID          uuid.UUID              `json:"contact_id"`
	Kind               string                 `json:"kind" jsonschema:"conversation or feedback"`
	Status             string                 `json:"status" jsonschema:"open, pending, snoozed or closed"`
	Priority           string                 `json:"priority" jsonschema:"low, normal, high or urgent"`
	AssigneeID         *uuid.UUID             `json:"assignee_id,omitempty" jsonschema:"the assigned member, absent when unassigned"`
	TeamID             *uuid.UUID             `json:"team_id,omitempty" jsonschema:"the assigned team, absent when none; names come from list_teams"`
	AssignWaitingSince *time.Time             `json:"assign_waiting_since,omitempty" jsonschema:"automatic assignment found nobody eligible at this time and the conversation waits for an available member; absent otherwise"`
	Labels             []uuid.UUID            `json:"labels" jsonschema:"label ids; names come from list_labels"`
	Spam               bool                   `json:"spam"`
	Unread             *bool                  `json:"unread,omitempty" jsonschema:"whether the calling member has unread messages here"`
	SnoozeUntil        *time.Time             `json:"snooze_until,omitempty"`
	FeedbackCategory   string                 `json:"feedback_category,omitempty" jsonschema:"a key of the inbox feedback categories (bug, idea, praise and other by default), for feedback"`
	FeedbackScore      int32                  `json:"feedback_score,omitempty" jsonschema:"1 (worst) to 5 (best) when the contact scored the feedback"`
	FeedbackProgress   string                 `json:"feedback_progress,omitempty" jsonschema:"planned, in_progress, done or wont_do, for feedback whose progress a member set"`
	IdeaID             *uuid.UUID             `json:"idea_id,omitempty" jsonschema:"the roadmap idea this feedback is linked to"`
	Fields             map[string]any         `json:"fields,omitempty" jsonschema:"custom field values by key; names and types come from list_custom_fields"`
	CreatedAt          time.Time              `json:"created_at"`
	LastActivityAt     time.Time              `json:"last_activity_at"`
	SLA                *oas.ConversationSla   `json:"sla,omitempty" jsonschema:"the SLA policy that applies: state (on_track, due_soon, breached, met or paused), due_at and its targets; absent when none applies"`
	CustomerContent    mcpConversationContent `json:"customer_content"`
}

func feedbackContent(f *oas.Feedback) (string, map[string]string) {
	if f == nil {
		return "", nil
	}
	m := map[string]string{}
	for k, v := range map[string]*string{
		"app_version": f.AppVersion, "build": f.Build, "device_model": f.DeviceModel, "installation_id": f.InstallationId,
		"locale": f.Locale, "os": f.Os, "os_version": f.OsVersion, "screen": f.Screen,
	} {
		if v != nil {
			m[k] = *v
		}
	}
	return string(f.Category), m
}

func conversationOut(c oas.Conversation) mcpConversation {
	cat, fb := feedbackContent(c.Feedback)
	progress := ""
	if c.Feedback != nil && c.Feedback.Progress != nil {
		progress = string(*c.Feedback.Progress)
	}
	var score int32
	if c.Feedback != nil && c.Feedback.Score != nil {
		score = *c.Feedback.Score
	}
	labels := c.Labels
	if labels == nil {
		labels = []uuid.UUID{}
	}
	var idea *uuid.UUID
	if c.Idea != nil {
		idea = &c.Idea.Id
	}
	return mcpConversation{
		IdeaID: idea, ID: c.Id, InboxID: c.InboxId, ContactID: c.ContactId, Kind: string(c.Kind), Status: string(c.Status),
		Priority: string(c.Priority), AssigneeID: c.AssigneeId, TeamID: c.TeamId, AssignWaitingSince: c.AssignWaitingSince, Labels: labels, Spam: c.Spam, SnoozeUntil: c.SnoozeUntil,
		FeedbackCategory: cat, FeedbackScore: score, FeedbackProgress: progress, CreatedAt: c.CreatedAt, LastActivityAt: c.LastActivityAt,
		SLA: c.Sla, CustomerContent: mcpConversationContent{Subject: c.Subject, Feedback: fb}, Fields: c.Fields,
	}
}

func conversationItemOut(c oas.ConversationListItem) mcpConversation {
	out := conversationOut(oas.Conversation{
		Id: c.Id, InboxId: c.InboxId, ContactId: c.ContactId, Kind: c.Kind, Status: c.Status, Priority: c.Priority,
		AssigneeId: c.AssigneeId, TeamId: c.TeamId, AssignWaitingSince: c.AssignWaitingSince, Labels: c.Labels, Spam: c.Spam, SnoozeUntil: c.SnoozeUntil, Feedback: c.Feedback, Idea: c.Idea,
		CreatedAt: c.CreatedAt, LastActivityAt: c.LastActivityAt, Subject: c.Subject, Fields: c.Fields, Sla: c.Sla,
	})
	unread := c.Unread
	out.Unread = &unread
	out.CustomerContent.ContactName = c.Contact.Name
	if c.Contact.Email != nil {
		out.CustomerContent.ContactEmail = string(*c.Contact.Email)
	}
	if c.LastMessage != nil {
		out.CustomerContent.LastMessage = c.LastMessage.Text
	}
	return out
}

type mcpAuthor struct {
	Type      string     `json:"type" jsonschema:"member, bot, contact or system"`
	MemberID  *uuid.UUID `json:"member_id,omitempty"`
	APIKeyID  *uuid.UUID `json:"api_key_id,omitempty"`
	ContactID *uuid.UUID `json:"contact_id,omitempty"`
	Name      string     `json:"name,omitempty" jsonschema:"the member's or bot's name; a contact's name is in customer_content"`
	Via       string     `json:"via,omitempty" jsonschema:"the connected app a member wrote through"`
}

func authorOut(a oas.MessageAuthor) (mcpAuthor, string) {
	out := mcpAuthor{Type: string(a.Type), MemberID: a.MemberId, APIKeyID: a.ApiKeyId, ContactID: a.ContactId}
	if a.Via != nil {
		out.Via = *a.Via
	}
	name := ""
	if a.Name != nil {
		name = *a.Name
	}
	if a.Type == oas.AuthorTypeContact {
		return out, name
	}
	out.Name = name
	return out, ""
}

type mcpAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

type mcpMessageContent struct {
	Body        string          `json:"body" jsonschema:"the message text"`
	AuthorName  string          `json:"author_name,omitempty" jsonschema:"the contact's name when a contact wrote it"`
	From        string          `json:"from,omitempty" jsonschema:"the sender address of an e-mail"`
	Subject     string          `json:"subject,omitempty" jsonschema:"the subject of an e-mail"`
	Attachments []mcpAttachment `json:"attachments,omitempty"`
}

type mcpEvent struct {
	Type                     string      `json:"type"`
	Status                   string      `json:"status,omitempty"`
	PreviousStatus           string      `json:"previous_status,omitempty"`
	AssigneeID               *uuid.UUID  `json:"assignee_id,omitempty"`
	InboxID                  *uuid.UUID  `json:"inbox_id,omitempty"`
	AddedLabels              []uuid.UUID `json:"added_labels,omitempty"`
	RemovedLabels            []uuid.UUID `json:"removed_labels,omitempty"`
	FeedbackProgress         *string     `json:"feedback_progress,omitempty" jsonschema:"feedback_progress events: the new progress, empty when it was cleared"`
	PreviousFeedbackProgress string      `json:"previous_feedback_progress,omitempty"`
}

type mcpMessage struct {
	ID              uuid.UUID         `json:"id"`
	ConversationID  uuid.UUID         `json:"conversation_id"`
	Kind            string            `json:"kind" jsonschema:"message, note (internal), event (timeline change) or forward (internal: e-mailed to someone else, or their answer)"`
	Direction       string            `json:"direction,omitempty" jsonschema:"in from the contact, out to the contact"`
	Draft           bool              `json:"draft" jsonschema:"true for a draft that a member has not sent yet"`
	Author          mcpAuthor         `json:"author"`
	SentBy          *mcpAuthor        `json:"sent_by,omitempty" jsonschema:"who sent a draft"`
	DeliveryState   string            `json:"delivery_state,omitempty" jsonschema:"queued, sent or failed for e-mailed replies"`
	Event           *mcpEvent         `json:"event,omitempty"`
	Redacted        bool              `json:"redacted,omitempty" jsonschema:"true when a member removed its content"`
	CreatedAt       time.Time         `json:"created_at"`
	CustomerContent mcpMessageContent `json:"customer_content"`
}

func messageOut(m oas.Message) mcpMessage {
	author, contactName := authorOut(m.Author)
	out := mcpMessage{
		ID: m.Id, ConversationID: m.ConversationId, Kind: string(m.Kind), Draft: m.Draft, Author: author, CreatedAt: m.CreatedAt,
		Redacted:        m.RedactedAt != nil,
		CustomerContent: mcpMessageContent{Body: m.Body, AuthorName: contactName},
	}
	if m.Direction != nil {
		out.Direction = string(*m.Direction)
	}
	if m.SentBy != nil {
		sb, _ := authorOut(*m.SentBy)
		out.SentBy = &sb
	}
	if m.Delivery != nil {
		out.DeliveryState = string(m.Delivery.State)
	}
	if m.Event != nil {
		ev := &mcpEvent{Type: string(m.Event.Type), AssigneeID: m.Event.AssigneeId, InboxID: m.Event.InboxId}
		if m.Event.Status != nil {
			ev.Status = string(*m.Event.Status)
		}
		if m.Event.PreviousStatus != nil {
			ev.PreviousStatus = string(*m.Event.PreviousStatus)
		}
		if m.Event.AddedLabels != nil {
			ev.AddedLabels = *m.Event.AddedLabels
		}
		if m.Event.RemovedLabels != nil {
			ev.RemovedLabels = *m.Event.RemovedLabels
		}
		if m.Event.Type == oas.EventTypeFeedbackProgress {
			ev.FeedbackProgress = new("")
			if v, err := m.Event.FeedbackProgress.Get(); err == nil {
				ev.FeedbackProgress = new(string(v))
			}
		}
		if m.Event.PreviousFeedbackProgress != nil {
			ev.PreviousFeedbackProgress = string(*m.Event.PreviousFeedbackProgress)
		}
		out.Event = ev
	}
	if m.Email != nil {
		out.CustomerContent.From = string(m.Email.From)
		if m.Email.Subject != nil {
			out.CustomerContent.Subject = *m.Email.Subject
		}
	}
	for _, a := range m.Attachments {
		out.CustomerContent.Attachments = append(out.CustomerContent.Attachments, mcpAttachment{Filename: a.Filename, ContentType: a.ContentType, Size: a.Size})
	}
	return out
}

type mcpExternalID struct {
	InboxID    uuid.UUID `json:"inbox_id"`
	ExternalID string    `json:"external_id"`
}

type mcpContactContent struct {
	Name        string          `json:"name"`
	Emails      []string        `json:"emails"`
	ExternalIDs []mcpExternalID `json:"external_ids" jsonschema:"the contact's ids in the host apps, per inbox"`
	Attributes  map[string]any  `json:"attributes" jsonschema:"attributes the host app or the contact set"`
}

type mcpContact struct {
	ID                   uuid.UUID         `json:"id"`
	Blocked              bool              `json:"blocked"`
	Locale               string            `json:"locale,omitempty" jsonschema:"the contact's language, a BCP 47 tag"`
	UndeliverableAddress int               `json:"undeliverable_addresses" jsonschema:"how many of the contact's addresses bounced or complained"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	CustomerContent      mcpContactContent `json:"customer_content"`
}

func contactOut(c oas.Contact) mcpContact {
	out := mcpContact{
		ID: c.Id, Blocked: c.Blocked, UndeliverableAddress: len(c.Undeliverable), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		CustomerContent: mcpContactContent{Name: c.Name, Emails: []string{}, ExternalIDs: []mcpExternalID{}, Attributes: map[string]any{}},
	}
	if c.Locale != nil {
		out.Locale = *c.Locale
	}
	for _, e := range c.Emails {
		out.CustomerContent.Emails = append(out.CustomerContent.Emails, string(e))
	}
	for _, e := range c.ExternalIds {
		out.CustomerContent.ExternalIDs = append(out.CustomerContent.ExternalIDs, mcpExternalID{InboxID: e.InboxId, ExternalID: e.ExternalId})
	}
	for k, v := range c.Attributes {
		out.CustomerContent.Attributes[k] = v
	}
	return out
}

type mcpInbox struct {
	ID                   uuid.UUID `json:"id"`
	Name                 string    `json:"name"`
	Slug                 string    `json:"slug"`
	DefaultLocale        string    `json:"default_locale" jsonschema:"the inbox's language, a BCP 47 tag; write replies in it unless the contact wrote in another"`
	Timezone             string    `json:"timezone"`
	Mode                 string    `json:"mode" jsonschema:"live or async"`
	Purpose              string    `json:"purpose" jsonschema:"support or mail"`
	ExpectedReplyMinutes *int32    `json:"expected_reply_minutes,omitempty"`
	Sender               mcpSender `json:"sender" jsonschema:"who sends the inbox's list mail; every broadcast's footer shows it"`
	SenderReady          bool      `json:"sender_ready" jsonschema:"the legal name and postal address are set, so broadcasts are not blocked by the sender identity"`
}

type mcpSender struct {
	LegalName     string `json:"legal_name,omitempty"`
	PostalAddress string `json:"postal_address,omitempty"`
	Registration  string `json:"registration,omitempty" jsonschema:"such as a MERSİS number or VAT id"`
	Contact       string `json:"contact,omitempty" jsonschema:"an e-mail address, phone or URL"`
}

func inboxOut(in oas.Inbox) mcpInbox {
	return mcpInbox{
		ID: in.Id, Name: in.Name, Slug: in.Slug, DefaultLocale: in.DefaultLocale, Timezone: in.Timezone, Mode: string(in.Mode),
		Purpose: string(in.Purpose), ExpectedReplyMinutes: in.ExpectedReplyMinutes,
		Sender: mcpSender{
			LegalName: deref(in.Sender.LegalName), PostalAddress: deref(in.Sender.PostalAddress), Registration: deref(in.Sender.Registration),
			Contact: deref(in.Sender.Contact),
		},
		SenderReady: senderReady(in.Sender.LegalName, in.Sender.PostalAddress),
	}
}

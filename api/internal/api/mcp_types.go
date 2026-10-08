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
	ID               uuid.UUID              `json:"id"`
	InboxID          uuid.UUID              `json:"inbox_id"`
	ContactID        uuid.UUID              `json:"contact_id"`
	Kind             string                 `json:"kind" jsonschema:"conversation or feedback"`
	Status           string                 `json:"status" jsonschema:"open, pending, snoozed or closed"`
	Priority         string                 `json:"priority" jsonschema:"low, normal, high or urgent"`
	AssigneeID       *uuid.UUID             `json:"assignee_id,omitempty" jsonschema:"the assigned member, absent when unassigned"`
	Labels           []uuid.UUID            `json:"labels" jsonschema:"label ids; names come from list_labels"`
	Spam             bool                   `json:"spam"`
	Unread           *bool                  `json:"unread,omitempty" jsonschema:"whether the calling member has unread messages here"`
	SnoozeUntil      *time.Time             `json:"snooze_until,omitempty"`
	FeedbackCategory string                 `json:"feedback_category,omitempty" jsonschema:"bug, idea, praise or other, for feedback"`
	CreatedAt        time.Time              `json:"created_at"`
	LastActivityAt   time.Time              `json:"last_activity_at"`
	CustomerContent  mcpConversationContent `json:"customer_content"`
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
	labels := c.Labels
	if labels == nil {
		labels = []uuid.UUID{}
	}
	return mcpConversation{
		ID: c.Id, InboxID: c.InboxId, ContactID: c.ContactId, Kind: string(c.Kind), Status: string(c.Status),
		Priority: string(c.Priority), AssigneeID: c.AssigneeId, Labels: labels, Spam: c.Spam, SnoozeUntil: c.SnoozeUntil,
		FeedbackCategory: cat, CreatedAt: c.CreatedAt, LastActivityAt: c.LastActivityAt,
		CustomerContent: mcpConversationContent{Subject: c.Subject, Feedback: fb},
	}
}

func conversationItemOut(c oas.ConversationListItem) mcpConversation {
	out := conversationOut(oas.Conversation{
		Id: c.Id, InboxId: c.InboxId, ContactId: c.ContactId, Kind: c.Kind, Status: c.Status, Priority: c.Priority,
		AssigneeId: c.AssigneeId, Labels: c.Labels, Spam: c.Spam, SnoozeUntil: c.SnoozeUntil, Feedback: c.Feedback,
		CreatedAt: c.CreatedAt, LastActivityAt: c.LastActivityAt, Subject: c.Subject,
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
	Type           string      `json:"type"`
	Status         string      `json:"status,omitempty"`
	PreviousStatus string      `json:"previous_status,omitempty"`
	AssigneeID     *uuid.UUID  `json:"assignee_id,omitempty"`
	InboxID        *uuid.UUID  `json:"inbox_id,omitempty"`
	AddedLabels    []uuid.UUID `json:"added_labels,omitempty"`
	RemovedLabels  []uuid.UUID `json:"removed_labels,omitempty"`
}

type mcpMessage struct {
	ID              uuid.UUID         `json:"id"`
	ConversationID  uuid.UUID         `json:"conversation_id"`
	Kind            string            `json:"kind" jsonschema:"message, note (internal) or event (timeline change)"`
	Direction       string            `json:"direction,omitempty" jsonschema:"in from the contact, out to the contact"`
	Draft           bool              `json:"draft" jsonschema:"true for a draft that a member has not sent yet"`
	Author          mcpAuthor         `json:"author"`
	SentBy          *mcpAuthor        `json:"sent_by,omitempty" jsonschema:"who sent a draft"`
	DeliveryState   string            `json:"delivery_state,omitempty" jsonschema:"queued, sent or failed for e-mailed replies"`
	Event           *mcpEvent         `json:"event,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	CustomerContent mcpMessageContent `json:"customer_content"`
}

func messageOut(m oas.Message) mcpMessage {
	author, contactName := authorOut(m.Author)
	out := mcpMessage{
		ID: m.Id, ConversationID: m.ConversationId, Kind: string(m.Kind), Draft: m.Draft, Author: author, CreatedAt: m.CreatedAt,
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
	ExpectedReplyMinutes *int32    `json:"expected_reply_minutes,omitempty"`
}

func inboxOut(in oas.Inbox) mcpInbox {
	return mcpInbox{
		ID: in.Id, Name: in.Name, Slug: in.Slug, DefaultLocale: in.DefaultLocale, Timezone: in.Timezone, Mode: string(in.Mode),
		ExpectedReplyMinutes: in.ExpectedReplyMinutes,
	}
}

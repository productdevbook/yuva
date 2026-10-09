package api

import (
	"net/http"
	"slices"

	"github.com/productdevbook/yuva/api/internal/oas"
)

var allScopes = []oas.ApiKeyScope{
	oas.ConversationsRead, oas.ConversationsWrite, oas.MessagesWrite, oas.DraftsSend, oas.NotesWrite,
	oas.ContactsRead, oas.ContactsWrite, oas.InboxesRead, oas.InboxesManage, oas.LabelsWrite,
	oas.CannedRepliesWrite, oas.WebhooksManage, oas.WorkspaceManage, oas.FeedbackWrite,
}

var unlimitedOnlyScopes = []oas.ApiKeyScope{oas.InboxesManage, oas.WebhooksManage, oas.WorkspaceManage}

// operationScopes lists every operation an API key may call with the scopes it needs. An operation
// missing here needs a member session.
var operationScopes = map[string][]oas.ApiKeyScope{
	"GetWorkspace":     {oas.InboxesRead},
	"ListMembers":      {oas.InboxesRead},
	"GetMember":        {oas.InboxesRead},
	"ListInboxes":      {oas.InboxesRead},
	"GetInbox":         {oas.InboxesRead},
	"ListInboxMembers": {oas.InboxesRead},
	"ListChannels":     {oas.InboxesRead},
	"GetChannel":       {oas.InboxesRead},

	"CreateInbox":               {oas.InboxesManage},
	"UpdateInbox":               {oas.InboxesManage},
	"DeleteInbox":               {oas.InboxesManage},
	"RotateInboxIdentitySecret": {oas.InboxesManage},
	"GrantInboxAccess":          {oas.InboxesManage},
	"RevokeInboxAccess":         {oas.InboxesManage},
	"CreateChannel":             {oas.InboxesManage},
	"UpdateChannel":             {oas.InboxesManage},
	"DeleteChannel":             {oas.InboxesManage},
	"RotateChannelPublicKey":    {oas.InboxesManage},

	"UpdateWorkspace": {oas.WorkspaceManage},
	"GetUsage":        {oas.WorkspaceManage},

	"ListContacts":       {oas.ContactsRead},
	"LookupContact":      {oas.ContactsRead},
	"GetContact":         {oas.ContactsRead},
	"GetContactPresence": {oas.ContactsRead},
	"GetContactSummary":  {oas.ContactsRead, oas.ConversationsRead},

	"CreateContact":             {oas.ContactsWrite},
	"UpdateContact":             {oas.ContactsWrite},
	"DeleteContact":             {oas.ContactsWrite},
	"DeleteContactByExternalId": {oas.ContactsWrite},
	"MergeContact":              {oas.ContactsWrite},

	"ListConversations":     {oas.ConversationsRead},
	"GetConversationCounts": {oas.ConversationsRead},
	"GetStats":              {oas.ConversationsRead},
	"GetConversation":       {oas.ConversationsRead},
	"ListMessages":          {oas.ConversationsRead},
	"DownloadAttachment":    {oas.ConversationsRead},
	"GetMessageEmail":       {oas.ConversationsRead},
	"DownloadMessageRaw":    {oas.ConversationsRead},
	"ListLabels":            {oas.ConversationsRead},
	"ListCannedReplies":     {oas.ConversationsRead},
	"ListEvents":            {oas.ConversationsRead},
	"GetLatestEvent":        {oas.ConversationsRead},

	"CreateConversation":      {oas.ConversationsWrite},
	"UpdateConversation":      {oas.ConversationsWrite},
	"MoveConversation":        {oas.ConversationsWrite},
	"BulkUpdateConversations": {oas.ConversationsWrite},

	"CreateMessage": nil,
	"UpdateMessage": {oas.MessagesWrite},
	"DeleteMessage": {oas.MessagesWrite},
	"SendMessage":   {oas.DraftsSend, oas.MessagesWrite},

	"CreateLabel": {oas.LabelsWrite},
	"UpdateLabel": {oas.LabelsWrite},
	"DeleteLabel": {oas.LabelsWrite},

	"CreateCannedReply": {oas.CannedRepliesWrite},
	"UpdateCannedReply": {oas.CannedRepliesWrite},
	"DeleteCannedReply": {oas.CannedRepliesWrite},

	"ListWebhooks":          {oas.WebhooksManage},
	"CreateWebhook":         {oas.WebhooksManage},
	"GetWebhook":            {oas.WebhooksManage},
	"UpdateWebhook":         {oas.WebhooksManage},
	"DeleteWebhook":         {oas.WebhooksManage},
	"RotateWebhookSecret":   {oas.WebhooksManage},
	"ListWebhookDeliveries": {oas.WebhooksManage},
	"GetWebhookDelivery":    {oas.WebhooksManage},
	"RedeliverWebhook":      {oas.WebhooksManage},
	"ListWebhookAttempts":   {oas.WebhooksManage},

	"CreateFeedback": {oas.FeedbackWrite},
}

func errInsufficientScope(scope oas.ApiKeyScope) *apiError {
	e := problem(http.StatusForbidden, "insufficient_scope", "the API key or token lacks the scope "+string(scope))
	e.Scope = string(scope)
	return e
}

// keyMayCall refuses a key an operation that is not in operationScopes or whose scopes it lacks;
// CreateMessage (nil) needs messages:write or notes:write by its kind, checked once the body is read.
func keyMayCall(p principal, operationID string) error {
	scopes, ok := operationScopes[operationID]
	if !ok {
		return errMemberSessionRequired
	}
	for _, sc := range scopes {
		if err := requireScope(p, sc); err != nil {
			return err
		}
	}
	return nil
}

func requireScope(p principal, scope oas.ApiKeyScope) error {
	if p.scoped() && !slices.Contains(p.scopes, string(scope)) {
		return errInsufficientScope(scope)
	}
	return nil
}

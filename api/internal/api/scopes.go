package api

import (
	"net/http"
	"slices"

	"github.com/productdevbook/yuva/api/internal/oas"
)

var allScopes = []oas.ApiKeyScope{
	oas.ConversationsRead, oas.ConversationsWrite, oas.ConversationsDelete, oas.MessagesWrite, oas.DraftsSend, oas.NotesWrite,
	oas.ContactsRead, oas.ContactsWrite, oas.InboxesRead, oas.InboxesManage, oas.LabelsWrite,
	oas.CannedRepliesWrite, oas.WebhooksManage, oas.WorkspaceManage, oas.FeedbackWrite, oas.ListsRead, oas.ListsWrite,
	oas.BroadcastsRead, oas.BroadcastsWrite, oas.BroadcastsSend, oas.MigrationsManage,
	oas.AnalyticsRead, oas.AnalyticsWrite, oas.AnalyticsManage, oas.StatusRead, oas.StatusManage,
	oas.HelpRead, oas.HelpWrite, oas.HelpPublish, oas.RoadmapRead, oas.RoadmapWrite, oas.RoadmapStage,
	oas.ChangelogRead, oas.ChangelogWrite, oas.ChangelogPublish, oas.AuditRead,
}

// notImpliedScopes are left out of a key made without naming its scopes.
var notImpliedScopes = []oas.ApiKeyScope{oas.ConversationsDelete, oas.BroadcastsSend, oas.StatusManage, oas.HelpPublish, oas.RoadmapStage, oas.ChangelogPublish, oas.AuditRead}

var unlimitedOnlyScopes = []oas.ApiKeyScope{oas.InboxesManage, oas.WebhooksManage, oas.WorkspaceManage, oas.MigrationsManage, oas.AnalyticsManage, oas.AuditRead}

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

	"GetDeliverability": {oas.InboxesRead},

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
	"RotateForwardingAddress":   {oas.InboxesManage},
	"CheckDeliverability":       {oas.InboxesManage},
	"CreateDkimKey":             {oas.InboxesManage},
	"DeleteDkimKey":             {oas.InboxesManage},

	"UpdateWorkspace":          {oas.WorkspaceManage},
	"GetUsage":                 {oas.WorkspaceManage},
	"RequestBroadcastApproval": {oas.WorkspaceManage},

	"ListAuditLog": {oas.AuditRead},

	"ListContacts":       {oas.ContactsRead},
	"LookupContact":      {oas.ContactsRead},
	"GetContact":         {oas.ContactsRead},
	"GetContactPresence": {oas.ContactsRead},
	"GetContactSummary":  {oas.ContactsRead, oas.ConversationsRead},
	"ListContactNotes":   {oas.ContactsRead},
	"ListContactVisits":  {oas.ContactsRead, oas.AnalyticsRead},

	"CreateContact":             {oas.ContactsWrite},
	"UpdateContact":             {oas.ContactsWrite},
	"DeleteContact":             {oas.ContactsWrite},
	"DeleteContactByExternalId": {oas.ContactsWrite},
	"MergeContact":              {oas.ContactsWrite},
	"CreateContactNote":         {oas.ContactsWrite},
	"DeleteContactNote":         {oas.ContactsWrite},

	"ListConversations":     {oas.ConversationsRead},
	"GetConversationCounts": {oas.ConversationsRead},
	"GetStats":              {oas.ConversationsRead},
	"GetConversation":       {oas.ConversationsRead},
	"ListMessages":          {oas.ConversationsRead},
	"ListDrafts":            {oas.ConversationsRead},
	"DownloadAttachment":    {oas.ConversationsRead},
	"GetMessageEmail":       {oas.ConversationsRead},
	"DownloadMessageRaw":    {oas.ConversationsRead},
	"ListLabels":            {oas.ConversationsRead},
	"ListCannedReplies":     {oas.ConversationsRead},
	"ListEvents":            {oas.ConversationsRead},
	"GetLatestEvent":        {oas.ConversationsRead},
	"ListDiscussions":       {oas.ConversationsRead},
	"GetDiscussion":         {oas.ConversationsRead},
	"ListDiscussionPosts":   {oas.ConversationsRead},
	"ListDocsPages":         {oas.ConversationsRead},
	"GetDocsPage":           {oas.ConversationsRead},
	"GetDocsSummary":        {oas.ConversationsRead},
	"ListDocsSites":         {oas.ConversationsRead},

	"CreateDiscussionPost":   {oas.MessagesWrite},
	"AcceptDiscussionAnswer": {oas.MessagesWrite},
	"ClearDiscussionAnswer":  {oas.MessagesWrite},

	"CreateConversation":      {oas.ConversationsWrite},
	"UpdateConversation":      {oas.ConversationsWrite},
	"MoveConversation":        {oas.ConversationsWrite},
	"BulkUpdateConversations": nil,
	"TrashConversation":       {oas.ConversationsDelete},
	"RestoreConversation":     {oas.ConversationsDelete},

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

	"ListMailingLists":         {oas.ListsRead},
	"GetMailingList":           {oas.ListsRead},
	"GetMailingListGrowth":     {oas.ListsRead},
	"ListSubscriptions":        {oas.ListsRead},
	"GetSubscription":          {oas.ListsRead},
	"ExportListConsent":        {oas.ListsRead},
	"ListListImports":          {oas.ListsRead},
	"GetListImport":            {oas.ListsRead},
	"GetListImportErrors":      {oas.ListsRead},
	"ListContactSubscriptions": {oas.ContactsRead, oas.ListsRead},
	"ExportContactConsent":     {oas.ContactsRead, oas.ListsRead},

	"CreateMailingList":              {oas.ListsWrite},
	"UpdateMailingList":              {oas.ListsWrite},
	"DeleteMailingList":              {oas.ListsWrite},
	"CreateSubscription":             {oas.ListsWrite},
	"UpdateSubscription":             {oas.ListsWrite},
	"DeleteSubscription":             {oas.ListsWrite},
	"ResendSubscriptionConfirmation": {oas.ListsWrite},
	"CreateListImport":               {oas.ListsWrite},
	"UpdateListImport":               {oas.ListsWrite},
	"StartListImport":                {oas.ListsWrite},
	"CancelListImport":               {oas.ListsWrite},

	"ListEmailTemplates":      {oas.BroadcastsRead},
	"GetEmailTemplate":        {oas.BroadcastsRead},
	"ListEmailDesigns":        {oas.BroadcastsRead},
	"GetEmailDesign":          {oas.BroadcastsRead},
	"ListEmailDesignVersions": {oas.BroadcastsRead},
	"GetEmailDesignVersion":   {oas.BroadcastsRead},
	"RenderEmailDesign":       {oas.BroadcastsRead},
	"ListEmailImages":         {oas.BroadcastsRead},

	"CreateEmailDesign":         {oas.BroadcastsWrite},
	"UpdateEmailDesign":         {oas.BroadcastsWrite},
	"ArchiveEmailDesign":        {oas.BroadcastsWrite},
	"DuplicateEmailDesign":      {oas.BroadcastsWrite},
	"RestoreEmailDesignVersion": {oas.BroadcastsWrite},
	"UploadEmailImage":          {oas.BroadcastsWrite},
	"DeleteEmailImage":          {oas.BroadcastsWrite},

	"ListBroadcasts":          {oas.BroadcastsRead},
	"GetBroadcast":            {oas.BroadcastsRead},
	"CountBroadcastAudience":  {oas.BroadcastsRead},
	"ReviewBroadcast":         {oas.BroadcastsRead},
	"GetBroadcastReport":      {oas.BroadcastsRead},
	"ListBroadcastDeliveries": {oas.BroadcastsRead, oas.ContactsRead},

	"CreateBroadcast":   {oas.BroadcastsWrite},
	"UpdateBroadcast":   {oas.BroadcastsWrite},
	"DeleteBroadcast":   {oas.BroadcastsWrite},
	"SendBroadcastTest": {oas.BroadcastsWrite},

	"ScheduleBroadcast":   {oas.BroadcastsSend},
	"UnscheduleBroadcast": {oas.BroadcastsSend},
	"PauseBroadcast":      {oas.BroadcastsSend},
	"ResumeBroadcast":     {oas.BroadcastsSend},
	"CancelBroadcast":     {oas.BroadcastsSend},

	"ListMigrationPresets":    {oas.MigrationsManage},
	"DetectMigrationSource":   {oas.MigrationsManage},
	"ListMigrations":          {oas.MigrationsManage},
	"CreateMigration":         {oas.MigrationsManage},
	"GetMigration":            {oas.MigrationsManage},
	"UpdateMigration":         {oas.MigrationsManage},
	"DeleteMigration":         {oas.MigrationsManage},
	"SetMigrationCredentials": {oas.MigrationsManage},
	"TestMigrationConnection": {oas.MigrationsManage},
	"StartMigrationSignIn":    {oas.MigrationsManage},
	"GetMigrationUpload":      {oas.MigrationsManage},
	"UploadMigrationPart":     {oas.MigrationsManage},
	"CompleteMigrationUpload": {oas.MigrationsManage},
	"ListMigrationFolders":    {oas.MigrationsManage},
	"ScanMigration":           {oas.MigrationsManage},
	"GetMigrationPreview":     {oas.MigrationsManage},
	"StartMigration":          {oas.MigrationsManage},
	"PauseMigration":          {oas.MigrationsManage},
	"ResumeMigration":         {oas.MigrationsManage},
	"CancelMigration":         {oas.MigrationsManage},
	"RetryMigrationItems":     {oas.MigrationsManage},
	"ListMigrationErrors":     {oas.MigrationsManage},
	"SendMigrationProbe":      {oas.MigrationsManage},
	"FinishMigration":         {oas.MigrationsManage},
	"RemoveMigrationHistory":  {oas.MigrationsManage},

	"ListAnalyticsProperties": {oas.AnalyticsRead},
	"GetAnalyticsProperty":    {oas.AnalyticsRead},
	"CreateAnalyticsProperty": {oas.AnalyticsManage},
	"UpdateAnalyticsProperty": {oas.AnalyticsManage},
	"DeleteAnalyticsProperty": {oas.AnalyticsManage},
	"RotateAnalyticsKey":      {oas.AnalyticsManage},
	"DeleteAnalyticsData":     {oas.AnalyticsManage},
	"QueryAnalytics":          {oas.AnalyticsRead},
	"GetAnalyticsRealtime":    {oas.AnalyticsRead},
	"GetAnalyticsHealth":      {oas.AnalyticsRead},
	"GetAnalyticsRetention":   {oas.AnalyticsRead},
	"DeleteAnalyticsVisitor":  {oas.AnalyticsManage},
	"SendAnalyticsEvents":     {oas.AnalyticsWrite},

	"ListAnalyticsGoals":        {oas.AnalyticsRead},
	"CreateAnalyticsGoal":       {oas.AnalyticsManage},
	"UpdateAnalyticsGoal":       {oas.AnalyticsManage},
	"DeleteAnalyticsGoal":       {oas.AnalyticsManage},
	"ListAnalyticsFunnels":      {oas.AnalyticsRead},
	"CreateAnalyticsFunnel":     {oas.AnalyticsManage},
	"UpdateAnalyticsFunnel":     {oas.AnalyticsManage},
	"DeleteAnalyticsFunnel":     {oas.AnalyticsManage},
	"GetAnalyticsFunnelReport":  {oas.AnalyticsRead},
	"ListAnalyticsSegments":     {oas.AnalyticsRead},
	"CreateAnalyticsSegment":    {oas.AnalyticsRead},
	"UpdateAnalyticsSegment":    {oas.AnalyticsRead},
	"DeleteAnalyticsSegment":    {oas.AnalyticsRead},
	"ListAnalyticsAnnotations":  {oas.AnalyticsRead},
	"CreateAnalyticsAnnotation": {oas.AnalyticsRead},
	"UpdateAnalyticsAnnotation": {oas.AnalyticsRead},
	"DeleteAnalyticsAnnotation": {oas.AnalyticsRead},
	"ListAnalyticsShares":       {oas.AnalyticsRead},
	"CreateAnalyticsShare":      {oas.AnalyticsManage},
	"UpdateAnalyticsShare":      {oas.AnalyticsManage},
	"RevokeAnalyticsShare":      {oas.AnalyticsManage},
	"DeleteAnalyticsShare":      {oas.AnalyticsManage},
	"ListAnalyticsExports":      {oas.AnalyticsRead},
	"CreateAnalyticsExport":     {oas.AnalyticsRead},
	"GetAnalyticsExport":        {oas.AnalyticsRead},
	"DeleteAnalyticsExport":     {oas.AnalyticsRead},
	"DownloadAnalyticsExport":   {oas.AnalyticsRead},
	"ListAnalyticsAlerts":       {oas.AnalyticsRead},

	"ListStatusPages":              {oas.StatusRead},
	"GetStatusPage":                {oas.StatusRead},
	"PreviewStatusPage":            {oas.StatusRead},
	"ListStatusComponentGroups":    {oas.StatusRead},
	"ListStatusComponents":         {oas.StatusRead},
	"ListStatusIncidents":          {oas.StatusRead},
	"GetStatusIncident":            {oas.StatusRead},
	"CreateStatusPage":             {oas.StatusManage},
	"UpdateStatusPage":             {oas.StatusManage},
	"DeleteStatusPage":             {oas.StatusManage},
	"CreateStatusComponentGroup":   {oas.StatusManage},
	"UpdateStatusComponentGroup":   {oas.StatusManage},
	"DeleteStatusComponentGroup":   {oas.StatusManage},
	"CreateStatusComponent":        {oas.StatusManage},
	"UpdateStatusComponent":        {oas.StatusManage},
	"DeleteStatusComponent":        {oas.StatusManage},
	"SetStatusComponentState":      {oas.StatusManage},
	"CreateStatusIncident":         {oas.StatusManage},
	"UpdateStatusIncident":         {oas.StatusManage},
	"DeleteStatusIncident":         {oas.StatusManage},
	"PublishStatusIncident":        {oas.StatusManage},
	"CreateStatusIncidentUpdate":   {oas.StatusManage},
	"EditStatusIncidentUpdate":     {oas.StatusManage},
	"ListStatusMonitors":           {oas.StatusRead},
	"GetStatusMonitor":             {oas.StatusRead},
	"ListStatusMonitorChecks":      {oas.StatusRead},
	"GetStatusMonitorCheckSummary": {oas.StatusRead},
	"CreateStatusMonitor":          {oas.StatusManage},
	"UpdateStatusMonitor":          {oas.StatusManage},
	"DeleteStatusMonitor":          {oas.StatusManage},
	"RotateStatusMonitorToken":     {oas.StatusManage},
	"ListStatusNotices":            {oas.StatusRead},
	"GetStatusNotice":              {oas.StatusRead},
	"GetStatusNoticeReach":         {oas.StatusRead},
	"CancelStatusNotice":           {oas.StatusManage},

	"ListStatusIncidentConversations":    {oas.StatusRead, oas.ConversationsRead},
	"LinkStatusIncidentConversation":     {oas.StatusManage, oas.ConversationsRead},
	"UnlinkStatusIncidentConversation":   {oas.StatusManage, oas.ConversationsRead},
	"ReplyToStatusIncidentConversations": {oas.StatusManage, oas.MessagesWrite},
	"ListConversationIncidents":          {oas.ConversationsRead, oas.StatusRead},

	"ListHelpCenters":                 {oas.HelpRead},
	"GetHelpCenter":                   {oas.HelpRead},
	"PreviewHelpCenter":               {oas.HelpRead},
	"ListHelpCollections":             {oas.HelpRead},
	"ListHelpArticles":                {oas.HelpRead},
	"GetHelpArticle":                  {oas.HelpRead},
	"ListHelpArticleRevisions":        {oas.HelpRead},
	"SearchHelpArticles":              {oas.HelpRead},
	"GetHelpReport":                   {oas.HelpRead},
	"CreateHelpCollection":            {oas.HelpWrite},
	"UpdateHelpCollection":            {oas.HelpWrite},
	"DeleteHelpCollection":            {oas.HelpWrite},
	"CreateHelpArticle":               {oas.HelpWrite},
	"UpdateHelpArticle":               {oas.HelpWrite},
	"DeleteHelpArticle":               {oas.HelpWrite},
	"PutHelpArticleTranslation":       {oas.HelpWrite},
	"DeleteHelpArticleTranslation":    {oas.HelpWrite},
	"RestoreHelpArticleRevision":      {oas.HelpWrite},
	"RenderHelpMarkdown":              {oas.HelpWrite},
	"UploadHelpImage":                 {oas.HelpWrite},
	"CreateHelpCenter":                {oas.HelpPublish},
	"UpdateHelpCenter":                {oas.HelpPublish},
	"DeleteHelpCenter":                {oas.HelpPublish},
	"PublishHelpArticleTranslation":   {oas.HelpPublish},
	"UnpublishHelpArticleTranslation": {oas.HelpPublish},

	"ListRoadmapBoards":        {oas.RoadmapRead},
	"GetRoadmapBoard":          {oas.RoadmapRead},
	"PreviewRoadmapBoard":      {oas.RoadmapRead},
	"CreateRoadmapBoard":       {oas.RoadmapWrite},
	"UpdateRoadmapBoard":       {oas.RoadmapWrite},
	"DeleteRoadmapBoard":       {oas.RoadmapWrite},
	"ListIdeaStageChanges":     {oas.RoadmapRead},
	"MoveIdeaStage":            {oas.RoadmapStage},
	"LinkIdeaConversation":     {oas.RoadmapWrite, oas.ConversationsRead},
	"UnlinkIdeaConversation":   {oas.RoadmapWrite, oas.ConversationsRead},
	"ReplyToIdeaConversations": {oas.RoadmapWrite, oas.MessagesWrite},
	"GetIdeaLinkedReply":       {oas.RoadmapRead, oas.ConversationsRead},

	"ListChangelogs":                  {oas.ChangelogRead},
	"GetChangelog":                    {oas.ChangelogRead},
	"ListChangelogEntries":            {oas.ChangelogRead},
	"GetChangelogEntry":               {oas.ChangelogRead},
	"PreviewChangelog":                {oas.ChangelogRead},
	"CreateChangelogEntry":            {oas.ChangelogWrite},
	"UpdateChangelogEntry":            {oas.ChangelogWrite},
	"DeleteChangelogEntry":            {oas.ChangelogWrite},
	"PutChangelogEntryTranslation":    {oas.ChangelogWrite},
	"DeleteChangelogEntryTranslation": {oas.ChangelogWrite},
	"UploadChangelogImage":            {oas.ChangelogWrite},
	"CreateChangelog":                 {oas.ChangelogPublish},
	"UpdateChangelog":                 {oas.ChangelogPublish},
	"DeleteChangelog":                 {oas.ChangelogPublish},
	"PublishChangelogEntry":           {oas.ChangelogPublish},
	"ScheduleChangelogEntry":          {oas.ChangelogPublish},
	"UnscheduleChangelogEntry":        {oas.ChangelogPublish},
	"UnpublishChangelogEntry":         {oas.ChangelogPublish},
	"BroadcastChangelogEntry":         {oas.BroadcastsWrite, oas.ListsRead},
	"RenderMarkdown":                  nil,
}

func errInsufficientScope(scope oas.ApiKeyScope) *apiError {
	e := problem(http.StatusForbidden, "insufficient_scope", "the API key or token lacks the scope "+string(scope))
	e.Scope = string(scope)
	return e
}

// keyMayCall refuses a key an operation that is not in operationScopes or whose scopes it lacks;
// CreateMessage (nil) needs messages:write or notes:write by its kind, and BulkUpdateConversations
// (nil) conversations:delete for trashed and conversations:write for any other change, checked
// once the body is read.
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

import type {
  ChangelogLabel,
  BroadcastDeliveryState,
  ConversationQuery,
  DiscussionQuery,
  DiscussionState,
  IdeaStage,
  DocsPageSort,
  DraftAuthorKind,
  SubscriptionState,
} from "@/lib/api"

export type ConversationFilters = Omit<ConversationQuery, "cursor" | "limit"> & { fields?: Record<string, string> }

export type DraftFilters = { inbox_id?: string; author?: DraftAuthorKind }

export type DocsDays = 7 | 30 | 90

export type DiscussionFilters = Omit<DiscussionQuery, "cursor" | "limit">

export type GrowthDays = 30 | 90

export type QueueFilters = { inbox_id?: string; site?: string; page?: string }

export type HelpArticleFilters = {
  collection_id?: string
  locale?: string
  state?: "draft" | "published" | "changed" | "missing"
  q?: string
}

export type IdeaFilters = { board_id: string; stage?: IdeaStage; state?: DiscussionState; q?: string }

export type ChangelogEntryFilters = { state?: "draft" | "scheduled" | "published" | "changed"; label?: ChangelogLabel }

export const keys = {
  changelog: (ws: string) => ["ws", ws, "changelog"] as const,
  changelogs: (ws: string) => ["ws", ws, "changelog", "list"] as const,
  changelogEntries: (ws: string, changelog: string, f: ChangelogEntryFilters) =>
    ["ws", ws, "changelog", "entries", changelog, f] as const,
  changelogEntry: (ws: string, changelog: string, id: string) =>
    ["ws", ws, "changelog", "entry", changelog, id] as const,
  changelogBroadcasts: (ws: string, inbox: string) => ["ws", ws, "changelog", "broadcasts", inbox] as const,
  sso: (ws: string) => ["ws", ws, "sso"] as const,
  issueIntegrations: (ws: string) => ["ws", ws, "issue-integrations"] as const,
  slack: (ws: string) => ["ws", ws, "slack"] as const,
  slackChannels: (ws: string) => ["ws", ws, "slack", "channels"] as const,
  inboxSlack: (ws: string, inbox: string) => ["ws", ws, "slack", "inbox", inbox] as const,
  mySlack: (ws: string) => ["ws", ws, "slack", "me"] as const,
  roadmap: (ws: string) => ["ws", ws, "roadmap"] as const,
  roadmapBoards: (ws: string) => ["ws", ws, "roadmap", "boards"] as const,
  roadmapIdeas: (ws: string, f: IdeaFilters) => ["ws", ws, "roadmap", "ideas", f] as const,
  roadmapChanges: (ws: string, id: string) => ["ws", ws, "roadmap", "changes", id] as const,
  roadmapLinked: (ws: string) => ["ws", ws, "roadmap", "linked"] as const,
  roadmapLinkedList: (ws: string, id: string) => ["ws", ws, "roadmap", "linked", "list", id] as const,
  roadmapReply: (ws: string, id: string, reply: string, part: string) =>
    ["ws", ws, "roadmap", "linked", "reply", id, reply, part] as const,
  help: (ws: string) => ["ws", ws, "help"] as const,
  helpCenters: (ws: string) => ["ws", ws, "help", "centers"] as const,
  helpCenter: (ws: string, id: string) => ["ws", ws, "help", "center", id] as const,
  helpCollections: (ws: string, center: string) => ["ws", ws, "help", "collections", center] as const,
  helpArticles: (ws: string, center: string, f: HelpArticleFilters) =>
    ["ws", ws, "help", "articles", center, f] as const,
  helpArticle: (ws: string, center: string, id: string) => ["ws", ws, "help", "article", center, id] as const,
  helpRevisions: (ws: string, center: string, id: string, locale: string) =>
    ["ws", ws, "help", "revisions", center, id, locale] as const,
  helpImports: (ws: string, center: string) => ["ws", ws, "help", "imports", center] as const,
  helpImportRedirects: (ws: string, center: string, id: string) =>
    ["ws", ws, "help", "import-redirects", center, id] as const,
  helpReport: (ws: string, center: string, days: number) => ["ws", ws, "help", "report", center, days] as const,
  helpSearch: (ws: string, center: string, q: string, locale: string) =>
    ["ws", ws, "help", "search", center, q, locale] as const,
  analytics: (ws: string) => ["ws", ws, "analytics"] as const,
  analyticsProperties: (ws: string) => ["ws", ws, "analytics", "properties"] as const,
  analyticsProperty: (ws: string, id: string) => ["ws", ws, "analytics", "property", id] as const,
  analyticsData: (ws: string, id: string) => ["ws", ws, "analytics", "data", id] as const,
  workspace: (ws: string) => ["ws", ws, "workspace"] as const,
  members: (ws: string) => ["ws", ws, "members"] as const,
  invites: (ws: string) => ["ws", ws, "invites"] as const,
  auditLog: (ws: string, filters: string) => ["ws", ws, "audit-log", filters] as const,
  apiKeys: (ws: string) => ["ws", ws, "api-keys"] as const,
  oauthGrants: (ws: string) => ["ws", ws, "oauth-grants"] as const,
  inboxes: (ws: string) => ["ws", ws, "inboxes"] as const,
  inboxMembers: (ws: string, inbox: string) => ["ws", ws, "inbox-members", inbox] as const,
  channels: (ws: string, inbox: string) => ["ws", ws, "channels", inbox] as const,
  channel: (ws: string, id: string) => ["ws", ws, "channel", id] as const,
  deliverability: (ws: string, channel: string) => ["ws", ws, "deliverability", channel] as const,
  labels: (ws: string) => ["ws", ws, "labels"] as const,
  cannedReplies: (ws: string) => ["ws", ws, "canned-replies"] as const,
  views: (ws: string) => ["ws", ws, "views"] as const,
  teamViews: (ws: string, team: string) => ["ws", ws, "views", "team", team] as const,
  view: (ws: string, id: string) => ["ws", ws, "views", "one", id] as const,
  macros: (ws: string) => ["ws", ws, "macros"] as const,
  teams: (ws: string) => ["ws", ws, "teams"] as const,
  customFields: (ws: string) => ["ws", ws, "custom-fields"] as const,
  slaPolicies: (ws: string) => ["ws", ws, "sla-policies"] as const,
  automationRules: (ws: string) => ["ws", ws, "automation-rules"] as const,
  automationRuns: (ws: string, rule: string, outcome: string) =>
    ["ws", ws, "automation-rules", "runs", rule, outcome] as const,
  contact: (ws: string, id: string) => ["ws", ws, "contact", id] as const,
  contactSearch: (ws: string) => ["ws", ws, "contact-search"] as const,
  contactConversationCount: (ws: string, id: string) => ["ws", ws, "contact-conversation-count", id] as const,
  contactPresence: (ws: string, id: string) => ["ws", ws, "contact-presence", id] as const,
  contactSummary: (ws: string, id: string) => ["ws", ws, "contact-summary", id] as const,
  contactNotes: (ws: string, id: string) => ["ws", ws, "contact-notes", id] as const,
  contactVisits: (ws: string, id: string) => ["ws", ws, "contact-visits", id] as const,
  webhooks: (ws: string) => ["ws", ws, "webhooks"] as const,
  webhookList: (ws: string, inbox: string) => ["ws", ws, "webhooks", "list", inbox] as const,
  webhook: (ws: string, id: string) => ["ws", ws, "webhooks", "one", id] as const,
  webhookDeliveries: (ws: string, id: string, state: string) =>
    ["ws", ws, "webhooks", "deliveries", id, state] as const,
  webhookDelivery: (ws: string, id: string, delivery: string) =>
    ["ws", ws, "webhooks", "delivery", id, delivery] as const,
  webhookAttempts: (ws: string, id: string) => ["ws", ws, "webhooks", "attempts", id] as const,
  conversationLists: (ws: string) => ["ws", ws, "conversations"] as const,
  conversations: (ws: string, f: ConversationFilters) => ["ws", ws, "conversations", f] as const,
  counts: (ws: string) => ["ws", ws, "counts"] as const,
  stats: (ws: string) => ["ws", ws, "stats"] as const,
  home: (ws: string) => ["ws", ws, "home"] as const,
  conversation: (ws: string, id: string) => ["ws", ws, "conversation", id] as const,
  messages: (ws: string, id: string) => ["ws", ws, "messages", id] as const,
  participants: (ws: string, id: string) => ["ws", ws, "participants", id] as const,
  messageEmail: (ws: string, id: string) => ["ws", ws, "message-email", id] as const,
  passkeys: ["me", "passkeys"] as const,
  sessions: ["me", "sessions"] as const,
  notifications: (ws: string) => ["ws", ws, "notifications"] as const,
  mentions: (ws: string) => ["ws", ws, "mentions"] as const,
  draftLists: (ws: string) => ["ws", ws, "drafts"] as const,
  drafts: (ws: string, f: DraftFilters) => ["ws", ws, "drafts", f] as const,
  docs: (ws: string) => ["ws", ws, "docs"] as const,
  docsPages: (ws: string, inbox: string, site: string, days: DocsDays, sort: DocsPageSort) =>
    ["ws", ws, "docs", "pages", inbox, site, days, sort] as const,
  docsSummary: (ws: string, inbox: string, site: string, days: DocsDays) =>
    ["ws", ws, "docs", "summary", inbox, site, days] as const,
  docsPage: (ws: string, inbox: string, page: string, site: string, days: DocsDays) =>
    ["ws", ws, "docs", "page", inbox, page, site, days] as const,
  docsSites: (ws: string, inbox: string, days: DocsDays) => ["ws", ws, "docs", "sites", inbox, days] as const,
  discussions: (ws: string) => ["ws", ws, "discussions"] as const,
  discussionList: (ws: string, f: DiscussionFilters) => ["ws", ws, "discussions", "list", f] as const,
  discussion: (ws: string, id: string) => ["ws", ws, "discussions", "one", id] as const,
  discussionQueue: (ws: string, f: QueueFilters) => ["ws", ws, "discussions", "queue", f] as const,
  lists: (ws: string) => ["ws", ws, "lists"] as const,
  listPages: (ws: string, inbox: string, archived: boolean) => ["ws", ws, "lists", "pages", inbox, archived] as const,
  list: (ws: string, id: string) => ["ws", ws, "lists", "one", id] as const,
  listGrowth: (ws: string, id: string, days: GrowthDays) => ["ws", ws, "lists", "growth", id, days] as const,
  subscriptions: (ws: string, list: string, state: SubscriptionState | "", q: string) =>
    ["ws", ws, "lists", "subscriptions", list, state, q] as const,
  subscription: (ws: string, list: string, id: string) => ["ws", ws, "lists", "subscription", list, id] as const,
  listImports: (ws: string, list: string) => ["ws", ws, "lists", "imports", list] as const,
  listImport: (ws: string, list: string, id: string) => ["ws", ws, "lists", "import", list, id] as const,
  contactSubscriptions: (ws: string, contact: string) => ["ws", ws, "lists", "contact", contact] as const,
  broadcasts: (ws: string) => ["ws", ws, "broadcasts"] as const,
  broadcastPages: (ws: string, inbox: string, group: string) =>
    ["ws", ws, "broadcasts", "pages", inbox, group] as const,
  broadcast: (ws: string, id: string) => ["ws", ws, "broadcasts", "one", id] as const,
  broadcastReview: (ws: string, id: string) => ["ws", ws, "broadcasts", "review", id] as const,
  broadcastAudience: (ws: string, id: string, audience: string) =>
    ["ws", ws, "broadcasts", "audience", id, audience] as const,
  broadcastReport: (ws: string, id: string) => ["ws", ws, "broadcasts", "report", id] as const,
  status: (ws: string) => ["ws", ws, "status"] as const,
  statusPages: (ws: string, inbox: string) => ["ws", ws, "status", "pages", inbox] as const,
  statusPage: (ws: string, id: string) => ["ws", ws, "status", "page", id] as const,
  statusGroups: (ws: string, id: string) => ["ws", ws, "status", "groups", id] as const,
  statusComponents: (ws: string, id: string) => ["ws", ws, "status", "components", id] as const,
  statusIncidents: (ws: string, id: string, open: boolean) => ["ws", ws, "status", "incidents", id, open] as const,
  statusMonitors: (ws: string, id: string) => ["ws", ws, "status", "monitors", id] as const,
  statusChecks: (ws: string, page: string, id: string) => ["ws", ws, "status", "checks", page, id] as const,
  statusIncident: (ws: string, page: string, id: string) => ["ws", ws, "status", "incident", page, id] as const,
  statusNotices: (ws: string, page: string, incident: string) =>
    ["ws", ws, "status", "notices", page, incident] as const,
  statusReach: (ws: string, page: string, components: string[]) =>
    ["ws", ws, "status", "reach", page, components] as const,
  statusLinked: (ws: string) => ["ws", ws, "status", "linked"] as const,
  statusLinkedList: (ws: string, page: string, id: string) => ["ws", ws, "status", "linked", "list", page, id] as const,
  conversationIncidents: (ws: string, id: string) => ["ws", ws, "status", "linked", "of", id] as const,
  statusReply: (ws: string, id: string, reply: string, part: string) =>
    ["ws", ws, "status", "reply", id, reply, part] as const,
  migrations: (ws: string) => ["ws", ws, "migrations"] as const,
  migrationPages: (ws: string) => ["ws", ws, "migrations", "pages"] as const,
  migration: (ws: string, id: string) => ["ws", ws, "migrations", "one", id] as const,
  migrationFolders: (ws: string, id: string) => ["ws", ws, "migrations", "folders", id] as const,
  migrationPresets: (ws: string) => ["ws", ws, "migrations", "presets"] as const,
  migrationPreview: (ws: string, id: string, scannedAt: string) =>
    ["ws", ws, "migrations", "preview", id, scannedAt] as const,
  migrationErrors: (ws: string, id: string) => ["ws", ws, "migrations", "errors", id] as const,
  migrationUpload: (ws: string, id: string) => ["ws", ws, "migrations", "upload", id] as const,
  broadcastDeliveries: (ws: string, id: string, state: BroadcastDeliveryState | "", q: string, replied: boolean) =>
    ["ws", ws, "broadcasts", "deliveries", id, state, q, replied] as const,
}

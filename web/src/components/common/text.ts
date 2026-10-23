import { useLingui } from "@lingui/react/macro"

import {
  ApiError,
  type ApiKeyScope,
  type AppPlatform,
  type BroadcastDeliveryState,
  type BroadcastPauseReason,
  type BroadcastSkipReason,
  type BroadcastState,
  type ChannelKind,
  type ConsentEventKind,
  type ConversationStatus,
  type FeedbackProgress,
  type IdeaStage,
  type InboxMode,
  type MigrationFileRefusal,
  type Priority,
  type Role,
  type SubscriptionSource,
  type SubscriptionState,
  type UnsubscribeReason,
  type WebhookDeliveryState,
  type WebhookEventType,
  type MessagePreview,
  type Weekday,
} from "@/lib/api"

export const STATUSES: ConversationStatus[] = ["open", "pending", "snoozed", "closed"]
export const PRIORITIES: Priority[] = ["urgent", "high", "normal", "low"]
export const ROLES: Role[] = ["owner", "admin", "agent"]
export const CHANNEL_KINDS: ChannelKind[] = ["email", "chat", "app", "api"]
export const MODES: InboxMode[] = ["async", "live"]
export const WEEKDAYS: Weekday[] = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"]
export const IDEA_STAGES: IdeaStage[] = ["ideas", "planned", "in_progress", "shipped", "closed"]
export const PLATFORMS: AppPlatform[] = ["ios", "android"]
export const WEBHOOK_EVENTS: WebhookEventType[] = [
  "conversation.created",
  "conversation.updated",
  "conversation.rated",
  "conversation.trashed",
  "conversation.restored",
  "conversation.deleted",
  "message.created",
  "message.redacted",
  "feedback.created",
  "contact.updated",
  "contact.deleted",
  "draft.created",
  "draft.updated",
  "draft.deleted",
  "discussion.created",
  "discussion.post_created",
  "discussion.answered",
  "discussion.updated",
  "subscription.created",
  "subscription.updated",
  "broadcast.updated",
  "analytics.alert",
  "status.component.updated",
  "status.incident.created",
  "status.incident.updated",
  "status.incident.resolved",
  "status.monitor.down",
  "status.monitor.up",
  "help.article.published",
  "help.article.unpublished",
  "help.article.deleted",
  "discussion.stage_changed",
  "changelog.entry.published",
  "changelog.entry.unpublished",
  "changelog.entry.deleted",
  "issue_link.created",
  "issue_link.updated",
  "issue_link.deleted",
  "team.created",
  "team.updated",
  "team.deleted",
]
export const API_KEY_SCOPES: ApiKeyScope[] = [
  "conversations:read",
  "conversations:write",
  "conversations:delete",
  "messages:write",
  "messages:redact",
  "drafts:send",
  "notes:write",
  "contacts:read",
  "contacts:write",
  "inboxes:read",
  "inboxes:manage",
  "labels:write",
  "canned_replies:write",
  "webhooks:manage",
  "workspace:manage",
  "feedback:write",
  "lists:read",
  "lists:write",
  "broadcasts:read",
  "broadcasts:write",
  "broadcasts:send",
  "migrations:manage",
  "analytics:read",
  "analytics:write",
  "analytics:manage",
  "status:read",
  "status:manage",
  "help:read",
  "help:write",
  "help:publish",
  "roadmap:read",
  "roadmap:write",
  "roadmap:stage",
  "changelog:read",
  "changelog:write",
  "changelog:publish",
  "integrations:read",
  "integrations:manage",
  "issues:write",
  "audit:read",
]
export const OPT_IN_SCOPES: ApiKeyScope[] = ["conversations:delete", "messages:redact", "broadcasts:send", "audit:read"]
export const OWNER_ONLY_SCOPES: ApiKeyScope[] = ["audit:read"]
export type ApiKeyPreset = "read" | "support" | "everything" | "custom"
export const API_KEY_PRESET_SCOPES: Record<Exclude<ApiKeyPreset, "custom">, ApiKeyScope[]> = {
  read: [
    "conversations:read",
    "contacts:read",
    "inboxes:read",
    "lists:read",
    "broadcasts:read",
    "analytics:read",
    "status:read",
    "roadmap:read",
  ],
  support: [
    "conversations:read",
    "conversations:write",
    "messages:write",
    "notes:write",
    "contacts:read",
    "contacts:write",
    "inboxes:read",
    "labels:write",
    "feedback:write",
  ],
  everything: API_KEY_SCOPES.filter((s) => !OPT_IN_SCOPES.includes(s)),
}
export const WORKSPACE_WIDE_SCOPES: ApiKeyScope[] = [
  "inboxes:manage",
  "webhooks:manage",
  "workspace:manage",
  "migrations:manage",
  "analytics:manage",
  "integrations:manage",
  "audit:read",
]
export const DELIVERY_STATES: WebhookDeliveryState[] = ["pending", "succeeded", "failed"]
export const SUBSCRIPTION_STATES: SubscriptionState[] = ["subscribed", "pending", "unsubscribed"]
export const BROADCAST_DELIVERY_STATES: BroadcastDeliveryState[] = [
  "queued",
  "sent",
  "failed",
  "bounced",
  "complained",
  "skipped",
]

export function useEnumText() {
  const { t } = useLingui()
  const status: Record<ConversationStatus, string> = {
    open: t`Open`,
    pending: t`Pending`,
    snoozed: t`Snoozed`,
    closed: t`Closed`,
  }
  const resolved = t`Resolved`
  return {
    status,
    statusOf: (state: ConversationStatus, kind?: string) =>
      state === "closed" && kind === "feedback" ? resolved : status[state],
    priority: { urgent: t`Urgent`, high: t`High`, normal: t`Normal`, low: t`Low` } satisfies Record<Priority, string>,
    role: { owner: t`Owner`, admin: t`Admin`, agent: t`Agent` } satisfies Record<Role, string>,
    channel: { email: t`E-mail`, chat: t`Web chat`, app: t`Mobile app`, api: t`API` } satisfies Record<
      ChannelKind,
      string
    >,
    mode: { live: t`Live`, async: t`Async` } satisfies Record<InboxMode, string>,
    progress: {
      planned: t`Planned`,
      in_progress: t`In progress`,
      done: t({ message: "Done", context: "feedback progress" }),
      wont_do: t`Won't do`,
    } satisfies Record<FeedbackProgress, string>,
    stage: {
      ideas: t`Ideas`,
      planned: t`Planned`,
      in_progress: t`In progress`,
      shipped: t`Shipped`,
      closed: t`Closed`,
    } satisfies Record<IdeaStage, string>,
    platform: { ios: "iOS", android: "Android" } satisfies Record<AppPlatform, string>,
    event: {
      "conversation.created": t`A conversation started`,
      "conversation.updated": t`A conversation changed`,
      "conversation.rated": t`A contact rated a conversation`,
      "conversation.trashed": t`A conversation was moved to the Trash`,
      "conversation.restored": t`A conversation was restored from the Trash`,
      "conversation.deleted": t`A conversation was deleted for good`,
      "message.created": t`A message was added`,
      "message.redacted": t`A message was removed from a thread`,
      "feedback.created": t`Feedback arrived`,
      "contact.updated": t`A contact changed`,
      "contact.deleted": t`A contact was deleted`,
      "draft.created": t`A draft was written`,
      "draft.updated": t`A draft was edited`,
      "draft.deleted": t`A draft was discarded`,
      "discussion.created": t`A discussion started`,
      "discussion.post_created": t`A reply was written in a discussion`,
      "discussion.answered": t`A discussion got an accepted answer`,
      "discussion.updated": t`A discussion changed`,
      "subscription.created": t`Someone subscribed to a mailing list`,
      "subscription.updated": t`A subscription changed`,
      "broadcast.updated": t`A broadcast was scheduled, started, paused or finished`,
      "analytics.alert": t`An analytics property of the inbox raised an alert`,
      "status.component.updated": t`A status component changed or got a new state`,
      "status.incident.created": t`An incident or maintenance was published`,
      "status.incident.updated": t`An incident or maintenance changed or got an update`,
      "status.incident.resolved": t`An incident was resolved or maintenance completed`,
      "status.monitor.down": t`A status monitor went down`,
      "status.monitor.up": t`A status monitor recovered`,
      "help.article.published": t`A help center article was published`,
      "help.article.unpublished": t`A help center article was unpublished`,
      "help.article.deleted": t`A help center article was deleted`,
      "discussion.stage_changed": t`A roadmap idea moved to another stage`,
      "changelog.entry.published": t`A What's new entry was published`,
      "changelog.entry.unpublished": t`A What's new entry was unpublished`,
      "changelog.entry.deleted": t`A published What's new entry was deleted`,
      "issue_link.created": t`A GitHub or Linear issue was linked`,
      "issue_link.updated": t`A linked GitHub or Linear issue changed`,
      "issue_link.deleted": t`A GitHub or Linear issue was unlinked`,
      "team.created": t`A team was created`,
      "team.updated": t`A team was changed`,
      "team.deleted": t`A team was deleted`,
    } satisfies Record<WebhookEventType, string>,
    scope: {
      "conversations:read": t`Read conversations, messages, labels and canned replies`,
      "conversations:write": t`Change, move and bulk-update conversations`,
      "conversations:delete": t`Move conversations to the Trash, restore them and read the Trash`,
      "messages:write": t`Write messages and drafts`,
      "messages:redact": t`Remove the content of messages and notes`,
      "drafts:send": t`Send drafts`,
      "notes:write": t`Write notes`,
      "contacts:read": t`Read contacts`,
      "contacts:write": t`Change, delete and merge contacts`,
      "inboxes:read": t`Read the workspace, members, inboxes and channels`,
      "inboxes:manage": t`Manage inboxes, channels and inbox access`,
      "labels:write": t`Manage labels`,
      "canned_replies:write": t`Manage canned replies`,
      "webhooks:manage": t`Manage webhooks`,
      "workspace:manage": t`Change the workspace and read usage`,
      "feedback:write": t`Post feedback for your app's users`,
      "lists:read": t`Read mailing lists, subscribers and consent records`,
      "lists:write": t`Manage mailing lists and subscribers`,
      "broadcasts:read": t`Read broadcasts and e-mail designs`,
      "broadcasts:write": t`Write broadcasts and e-mail designs`,
      "broadcasts:send": t`Schedule and send broadcasts, when the workspace lets keys and assistants send them`,
      "migrations:manage": t`Move mailboxes to Yuva: find the provider, connect, scan, copy and switch`,
      "analytics:read": t`Read analytics properties and their numbers`,
      "analytics:write": t`Send analytics events from your servers`,
      "analytics:manage": t`Manage analytics properties, their keys and their data`,
      "status:read": t`Read status pages, components and incidents`,
      "status:manage": t`Manage status pages, set component states and run incidents`,
      "help:read": t`Read help centers, collections and articles`,
      "help:write": t`Edit help center collections and article drafts`,
      "help:publish": t`Publish and unpublish help center articles`,
      "roadmap:read": t`Read roadmap boards, ideas and stage history`,
      "roadmap:write": t`Link feedback to ideas, reply to linked customers and manage roadmap boards`,
      "roadmap:stage": t`Move ideas between roadmap stages`,
      "changelog:read": t`Read What's new changelogs and their entries`,
      "changelog:write": t`Write What's new entries and translations`,
      "changelog:publish": t`Publish, schedule and unpublish What's new entries`,
      "integrations:read": t`Read the connected GitHub and Linear trackers`,
      "integrations:manage": t`Connect, change and disconnect GitHub and Linear trackers`,
      "issues:write": t`Create and link GitHub and Linear issues for feedback and ideas`,
      "audit:read": t`Read the audit log (owners only)`,
    } satisfies Record<ApiKeyScope, string>,
    delivery: { pending: t`Pending`, succeeded: t`Succeeded`, failed: t`Failed` } satisfies Record<
      WebhookDeliveryState,
      string
    >,
    subscription: {
      pending: t`Waiting for confirmation`,
      subscribed: t`Subscribed`,
      unsubscribed: t`Unsubscribed`,
    } satisfies Record<SubscriptionState, string>,
    subscriptionSource: {
      form: t`Subscribe form`,
      identity: t`Signed-in user`,
      api: t`API`,
      member: t`Added by the team`,
      import: t`Import`,
      preferences: t`Preference page`,
    } satisfies Record<SubscriptionSource, string>,
    unsubscribeReason: {
      link: t`Unsubscribe link`,
      one_click: t`One-click unsubscribe in their mail app`,
      preferences: t`Preference page`,
      client: t`From your site or app`,
      member: t`By the team`,
      api: t`Through the API`,
      complaint: t`Reported a mail as spam`,
      erasure: t`The contact was deleted`,
    } satisfies Record<UnsubscribeReason, string>,
    consentEvent: {
      requested: t`Asked to subscribe`,
      confirmation_sent: t`Confirmation mail sent`,
      confirmed: t`Confirmed`,
      subscribed: t({ message: "Subscribed", context: "consent event" }),
      unsubscribed: t`Unsubscribed`,
      complained: t`Reported a mail as spam`,
    } satisfies Record<ConsentEventKind, string>,
    consentSource: {
      form: t`Subscribe form`,
      identity: t`Signed-in user on your site or app`,
      api: t`Through the API`,
      member: t`By the team`,
      import: t`Import`,
      preferences: t`Preference page`,
      link: t`Link in a mail`,
      one_click: t`One-click unsubscribe in their mail app`,
      client: t`From your site or app`,
      complaint: t`Spam report`,
      mail: t`Mail from Yuva`,
    } as Record<string, string | undefined>,
    broadcastState: {
      draft: t`Draft`,
      scheduled: t`Scheduled`,
      preparing: t`Preparing`,
      sending: t`Sending`,
      paused: t`Paused`,
      sent: t({ message: "Sent", context: "broadcast state" }),
      cancelled: t`Cancelled`,
      failed: t`Failed`,
    } satisfies Record<BroadcastState, string>,
    pauseReason: {
      member: t`Paused by your team.`,
      bounce_rate: t`Paused on its own: 5% or more of the mails sent bounced. Check the list for old or mistyped addresses before you resume.`,
      complaint_rate: t`Paused on its own: 0.3% or more of the recipients reported the mail as spam, the most mail providers accept. Make sure everyone on the lists asked for this mail before you resume.`,
      sender_error: t`Paused on its own: the SMTP server refused the account, its encryption or too many mails in a row. Check the channel's broadcast account, then resume.`,
      workspace_suspended: t`Paused because the workspace may not send broadcasts for now.`,
      operator: t`Paused by the server's operator.`,
      ses_tenant: t`Paused because Amazon SES paused this inbox's tenant, or the channel lost its tenant isolation. Resume it once the server's operator has fixed that.`,
    } satisfies Record<BroadcastPauseReason, string>,
    deliveryState: {
      queued: t`Queued`,
      sent: t({ message: "Sent", context: "delivery state" }),
      failed: t`Failed`,
      bounced: t`Bounced`,
      complained: t`Reported as spam`,
      skipped: t`Skipped`,
    } satisfies Record<BroadcastDeliveryState, string>,
    skipReason: {
      suppressed: t`The address bounced or complained`,
      opted_out: t`Objected to all mail`,
      unsubscribed: t`Unsubscribed before it went out`,
      cancelled: t`The broadcast was cancelled`,
    } satisfies Record<BroadcastSkipReason, string>,
    weekday: {
      mon: t`Monday`,
      tue: t`Tuesday`,
      wed: t`Wednesday`,
      thu: t`Thursday`,
      fri: t`Friday`,
      sat: t`Saturday`,
      sun: t`Sunday`,
    } satisfies Record<Weekday, string>,
  }
}

export function usePreviewText() {
  const { t } = useLingui()
  return (m: MessagePreview | undefined) => (m?.redacted ? t`Message removed` : (m?.text ?? ""))
}

export function useErrorText() {
  const { t } = useLingui()
  return (err: unknown): string => {
    if (!(err instanceof ApiError)) return t`Could not reach the server. Check your connection and try again.`
    if (err.code === "email_undeliverable")
      return t`This e-mail address bounced or complained, so the reply was not sent. Clear it on the contact to send again.`
    if (err.code === "email_not_configured")
      return t`This conversation's e-mail channel has no SMTP account, so the reply was not sent. Add one in the inbox settings.`
    if (err.code === "email_address_taken") return t`Another channel already receives mail at this address.`
    if (err.code === "tracker_auth_failed")
      return t`The tracker refused the token. Check that it is valid and has access to issues.`
    if (err.code === "issue_not_found") return t`The tracker has no such issue, or the token cannot see it.`
    if (err.code === "tracker_rejected")
      return t`The tracker refused the request. Check the target and your permissions.`
    if (err.code === "too_many_links") return t`This issue or this item has as many links as allowed.`
    if (err.code === "tracker_unavailable") return t`The tracker could not be reached. Try again in a moment.`
    if (err.code === "sso_discovery_failed")
      return err.detail
        ? t`Yuva could not read the provider's discovery document: ${err.detail}`
        : t`Yuva could not read the provider's discovery document. Check the issuer URL.`
    if (err.code === "sso_not_ready")
      return t`Single sign-on is required only once a provider is connected and a domain is verified.`
    if (err.code === "domain_exists") return t`This domain is already in the list.`
    if (err.code === "domain_taken") return t`Another workspace on this server has already verified this domain.`
    if (err.code === "sso_domain_limit") return t`This workspace has as many domains as allowed.`
    if (err.code === "member_session_required") return t`Sign in as yourself to change this. API keys cannot.`
    if (err.code === "sso_unavailable")
      return t`Your organisation's identity provider could not be reached. Try again in a moment.`
    if (err.code === "webhook_disabled") return t`The endpoint is disabled. Enable it to send deliveries again.`
    if (err.code === "no_email_channel")
      return t`The other inbox has no e-mail channel, so this e-mail conversation cannot move there. Add an e-mail channel to it first.`
    if (err.code === "email_no_sender")
      return t`The e-mail channel is a catch-all without a From address, so it has no address to send this conversation from. Set one in the inbox settings.`
    if (err.code === "not_redactable") return t`Drafts and timeline events cannot be removed.`
    if (err.code === "already_discussed") return t`This conversation is already a discussion.`
    if (err.code === "no_discussion_channel")
      return t`No chat channel of this inbox allows the page's site with discussions on. Turn discussions on in the inbox settings.`
    if (err.code === "discussions_off") return t`Discussions are off on this site's chat channel.`
    if (err.code === "page_origin") return t`The page is not on a site the chat channel allows.`
    if (err.code === "origin_not_allowed") return t`This site is not allowed to use this documentation channel.`
    if (err.code === "name_taken") return t`This inbox already has a list with this name.`
    if (err.code === "email_channel_required")
      return t`Pick an e-mail channel of this inbox that has an SMTP account. It sends the confirmation and welcome mails.`
    if (err.code === "unsubscribed")
      return t`This address unsubscribed from this list. Only the person can come back: choose “Mail them a confirmation” to ask them.`
    if (err.code === "opted_out")
      return t`This person unsubscribed from everything, reported a mail as spam or had their data erased, so the team cannot add them again.`
    if (err.code === "suppressed")
      return t`Mail to this address bounced or was reported as spam, so it cannot be added.`
    if (err.code === "invalid_email") return t`This is not a valid e-mail address.`
    if (err.code === "confirmation_limit")
      return t`This address got as many confirmation mails as allowed for now. Try again tomorrow.`
    if (err.code === "confirmation_required") return t`Only the person can subscribe again, by confirming a mail.`
    if (err.code === "not_pending") return t`This subscription is no longer waiting for confirmation.`
    if (err.code === "broadcast_active") return t`Cancel the broadcast before deleting it.`
    if (err.code === "not_draft") return t`This broadcast is no longer a draft, so it cannot be changed.`
    if (err.code === "pace_too_slow")
      return t`At that pace the mail still to go out would take more than 365 days. Choose a faster pace.`
    if (err.code === "ses_tenant_blocked")
      return t`Amazon SES paused this inbox's tenant, or its channel lost tenant isolation, so the broadcast cannot resume. The server's operator has to fix it first.`
    if (err.code === "review_failed") return t`The review found something that blocks sending. Fix the red items first.`
    if (err.code === "audience_changed")
      return t`The number of recipients changed since you looked. Check the new count and confirm again.`
    if (err.code === "broadcasts_disabled") return t`Broadcasts are turned off on this server.`
    if (err.code === "broadcasts_approval_required")
      return t`This workspace may send broadcasts once the server's operator approves it. Ask for approval in the workspace settings.`
    if (err.code === "broadcasts_suspended") return t`This workspace may not send broadcasts for now.`
    if (err.code === "broadcasts_by_keys_disabled")
      return t`Keys and assistants may not schedule broadcasts in this workspace.`
    if (err.code === "sender_not_ready")
      return t`The sending channel has no SMTP account or address to send from yet. Set it up in the inbox settings.`
    if (err.code === "design_required") return t`Pick a design first.`
    if (err.code === "test_limit") return t`This workspace sent as many tests as allowed this hour. Try again later.`
    if (err.code === "already_started") return t`Sending has already started. Pause or cancel it instead.`
    if (err.code === "invalid_state") return t`The broadcast changed in the meantime. Look at its state and try again.`
    if (err.code === "audience_too_large") return t`A broadcast can go to at most 1,000,000 recipients.`
    if (err.code === "limit_reached") return t`This workspace has as many broadcasts as allowed. Delete old ones first.`
    if (err.code === "attestation_required")
      return t`Confirm that everyone in the file agreed, and write where and when they did.`
    if (err.code === "not_csv") return t`This is not a CSV file. Save the spreadsheet as CSV and upload that.`
    if (err.code === "file_too_large") return t`The file is larger than 20 MB. Split it into smaller files.`
    if (err.code === "import_running")
      return t`Another import of this workspace is running. Imports run one at a time; try again when it ends.`
    if (err.code === "not_ready") return t`The import is no longer waiting to start. Look at its state.`
    if (err.code === "email_column_required") return t`Pick the column that holds the e-mail addresses.`
    if (err.code === "dkim_exists") return t`The channel already has a DKIM key. Delete it before creating another.`
    if (err.code === "dkim_dns_missing")
      return t`The DNS record is not visible yet. Publish it, wait a few minutes and try again.`
    if (err.code === "attachment_type_mismatch")
      return t`A file's content does not match its type. Check the file and try again.`
    if (err.code === "migration_state")
      return t`The migration moved on in the meantime. Look at its state and try again.`
    if (err.code === "hosted_limit") {
      const limit = (err.body as { limit?: string } | undefined)?.limit
      if (limit === "YUVA_MIGRATION_SOURCES") return t`This server does not offer imports from this kind of mailbox.`
      if (limit === "YUVA_MIGRATION_MAX_ACTIVE")
        return t`This server runs as many migrations at once as it allows. Wait until a running one ends, or ask the server's operator.`
      if (limit === "YUVA_MIGRATION_UPLOAD_MAX_BYTES")
        return t`The file is larger than this server takes for one export file. Split the export, or ask the server's operator.`
      if (limit === "YUVA_MIGRATION_MAX_BYTES")
        return t`The mailbox is larger than this server allows for one migration. Ask the server's operator.`
      if (err.detail) return err.detail
      return t`This server's limit for migrations is reached. Wait until a running one ends, or ask the server's operator.`
    }
    if (err.code === "file_refused") {
      const body = err.body as { reason?: MigrationFileRefusal; entry?: string } | undefined
      const reasons: Record<MigrationFileRefusal, string> = {
        unknown_format: t`This is neither an mbox file nor a zip. Upload the .mbox file itself, or a .zip of .eml files or of a Maildir.`,
        empty: t`The file holds no messages.`,
        corrupt: t`The zip cannot be read; it may be damaged or cut off. Make it again and upload it.`,
        path_traversal: t`The zip has an entry that points outside of it, so it was refused. Make the zip again from the folder.`,
        symlink: t`The zip holds a symbolic link, which Yuva does not follow. Make the zip again without links.`,
        too_many_entries: t`The zip has more than 1,000,000 entries. Split it into smaller zips, one migration each.`,
        zip_bomb: t`An entry in the zip unpacks to more than 100 times its size, so the file was refused as unsafe.`,
        entry_too_large: t`An entry in the zip is larger, unpacked, than this server takes for one file.`,
        encrypted: t`The zip is protected with a password. Make it again without one.`,
        unsupported_compression: t`The zip uses a compression method Yuva cannot read. Make it again as an ordinary zip.`,
        compressed_mbox: t`The mbox file is compressed inside the zip. Unzip the Takeout archive and upload the .mbox file.`,
      }
      const text = (body?.reason && reasons[body.reason]) || t`The file was refused.`
      const entry = body?.entry
      return entry ? `${text} ${t`Entry: ${entry}`}` : text
    }
    if (err.code === "upload_incomplete")
      return t`Some parts of the file did not arrive. Pick the same file again to send them.`
    if (err.code === "checksum_mismatch")
      return t`A part of the file arrived damaged several times. Check your connection, then pick the file again to continue.`
    if (err.code === "part_too_large")
      return t`A part of the file was larger than the server takes. Reload the page and pick the file again.`
    if (err.code === "provider_not_configured")
      return t`Outlook.com and Microsoft 365 connect with “Sign in with Microsoft”, which this server does not offer yet.`
    if (err.code === "confirm_count_mismatch")
      return t`The number of conversations changed since you looked. Check it and confirm again.`
    if (err.code === "goal_taken") return t`This property already has a goal with this name.`
    if (err.code === "goal_limit") return t`A property can have at most 100 goals. Delete one first.`
    if (err.code === "goal_in_use")
      return t`This goal is a step of a funnel. Take it out of the funnel, or delete the funnel, first.`
    if (err.code === "funnel_taken") return t`This property already has a funnel with this name.`
    if (err.code === "funnel_limit") return t`A property can have at most 20 funnels. Delete one first.`
    if (err.code === "segment_taken") return t`This property already has a segment with this name.`
    if (err.code === "segment_limit") return t`A property can have at most 100 segments. Delete one first.`
    if (err.code === "annotation_limit") return t`A property can have at most 1,000 notes. Delete old ones first.`
    if (err.code === "domain_taken")
      return t`Another status page already verified this domain. Only one page can hold a name.`
    if (err.code === "status_domain_limit")
      return t`This workspace has as many own domains as the server allows. Remove one first.`
    if (err.code === "slug_taken") return t`Another status page already uses this address.`
    if (err.code === "status_page_exists") return t`This inbox already has a status page.`
    if (err.code === "incident_closed")
      return t`The incident is already resolved or completed, so it takes no more changes.`
    if (err.code === "incident_published") return t`Only a draft can be deleted. Post an update instead.`
    if (err.code === "too_many_links")
      return t`An incident takes at most 5,000 linked conversations. Unlink some first.`
    if (err.code === "annotation_read_only") return t`Yuva's own notes cannot be changed.`
    switch (err.status) {
      case 400:
        return t`Some of the values are not valid.`
      case 401:
        return t`Your session has ended. Sign in again.`
      case 403:
        return t`You are not allowed to do this.`
      case 404:
        return t`It no longer exists or you cannot see it.`
      case 409:
        return t`This conflicts with something that already exists.`
      case 413:
        return t`The file is too large.`
      case 415:
        return t`This file type is not allowed.`
      case 429:
        return t`Too many attempts. Wait a little and try again.`
      default:
        return t`Something went wrong on the server. Try again.`
    }
  }
}

export function formatBytes(n: number, locale: string) {
  const units = ["B", "KB", "MB", "GB"]
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: i === 0 ? 0 : 1 }).format(v)} ${units[i]}`
}

export function formatDate(iso: string, locale: string) {
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(new Date(iso))
}

export function formatDateTime(iso: string, locale: string) {
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(new Date(iso))
}

export function formatRelative(iso: string, locale: string, now = Date.now()) {
  const sec = Math.round((new Date(iso).getTime() - now) / 1000)
  const abs = Math.abs(sec)
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" })
  if (abs < 5) return rtf.format(0, "second")
  if (abs < 60) return rtf.format(sec, "second")
  if (abs < 3600) return rtf.format(Math.round(sec / 60), "minute")
  if (abs < 86400) return rtf.format(Math.round(sec / 3600), "hour")
  if (abs < 7 * 86400) return rtf.format(Math.round(sec / 86400), "day")
  return formatDateTime(iso, locale)
}

export function formatShort(iso: string, locale: string, now = new Date()) {
  const d = new Date(iso)
  const sameDay = d.toDateString() === now.toDateString()
  if (sameDay) return new Intl.DateTimeFormat(locale, { timeStyle: "short" }).format(d)
  const sameYear = d.getFullYear() === now.getFullYear()
  return new Intl.DateTimeFormat(locale, sameYear ? { day: "numeric", month: "short" } : { dateStyle: "short" }).format(
    d
  )
}

export function initials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return "?"
  return (parts[0][0] + (parts.length > 1 ? parts[parts.length - 1][0] : "")).toLocaleUpperCase()
}

export function formatDuration(seconds: number, locale: string) {
  const unit = seconds < 60 ? "second" : seconds < 3600 ? "minute" : seconds < 86400 ? "hour" : "day"
  const size = { second: 1, minute: 60, hour: 3600, day: 86400 }[unit]
  return new Intl.NumberFormat(locale, { style: "unit", unit, unitDisplay: "short", maximumFractionDigits: 0 }).format(
    Math.round(seconds / size)
  )
}

export function formatMs(ms: number, locale: string) {
  if (ms >= 1000)
    return new Intl.NumberFormat(locale, { style: "unit", unit: "second", maximumFractionDigits: 1 }).format(ms / 1000)
  return new Intl.NumberFormat(locale, { style: "unit", unit: "millisecond", maximumFractionDigits: 0 }).format(ms)
}

import { useLingui } from "@lingui/react/macro"

import {
  ApiError,
  type ApiKeyScope,
  type AppPlatform,
  type ChannelKind,
  type ConversationStatus,
  type FeedbackCategory,
  type InboxMode,
  type Priority,
  type Role,
  type WebhookDeliveryState,
  type WebhookEventType,
  type Weekday,
} from "@/lib/api"

export const STATUSES: ConversationStatus[] = ["open", "pending", "snoozed", "closed"]
export const PRIORITIES: Priority[] = ["urgent", "high", "normal", "low"]
export const ROLES: Role[] = ["owner", "admin", "agent"]
export const CHANNEL_KINDS: ChannelKind[] = ["email", "chat", "app", "api"]
export const MODES: InboxMode[] = ["async", "live"]
export const WEEKDAYS: Weekday[] = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"]
export const FEEDBACK_CATEGORIES: FeedbackCategory[] = ["bug", "idea", "praise", "other"]
export const PLATFORMS: AppPlatform[] = ["ios", "android"]
export const WEBHOOK_EVENTS: WebhookEventType[] = [
  "conversation.created",
  "conversation.updated",
  "conversation.rated",
  "message.created",
  "feedback.created",
  "contact.updated",
  "contact.deleted",
  "draft.created",
  "draft.updated",
  "draft.deleted",
]
export const API_KEY_SCOPES: ApiKeyScope[] = [
  "conversations:read",
  "conversations:write",
  "messages:write",
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
]
export const WORKSPACE_WIDE_SCOPES: ApiKeyScope[] = ["inboxes:manage", "webhooks:manage", "workspace:manage"]
export const DELIVERY_STATES: WebhookDeliveryState[] = ["pending", "succeeded", "failed"]

export function useEnumText() {
  const { t } = useLingui()
  return {
    status: { open: t`Open`, pending: t`Pending`, snoozed: t`Snoozed`, closed: t`Closed` } satisfies Record<
      ConversationStatus,
      string
    >,
    priority: { urgent: t`Urgent`, high: t`High`, normal: t`Normal`, low: t`Low` } satisfies Record<Priority, string>,
    role: { owner: t`Owner`, admin: t`Admin`, agent: t`Agent` } satisfies Record<Role, string>,
    channel: { email: t`E-mail`, chat: t`Web chat`, app: t`Mobile app`, api: t`API` } satisfies Record<
      ChannelKind,
      string
    >,
    mode: { live: t`Live`, async: t`Async` } satisfies Record<InboxMode, string>,
    category: { bug: t`Bug`, idea: t`Idea`, praise: t`Praise`, other: t`Other` } satisfies Record<
      FeedbackCategory,
      string
    >,
    platform: { ios: "iOS", android: "Android" } satisfies Record<AppPlatform, string>,
    event: {
      "conversation.created": t`A conversation started`,
      "conversation.updated": t`A conversation changed`,
      "conversation.rated": t`A contact rated a conversation`,
      "message.created": t`A message was added`,
      "feedback.created": t`Feedback arrived`,
      "contact.updated": t`A contact changed`,
      "contact.deleted": t`A contact was deleted`,
      "draft.created": t`A draft was written`,
      "draft.updated": t`A draft was edited`,
      "draft.deleted": t`A draft was discarded`,
    } satisfies Record<WebhookEventType, string>,
    scope: {
      "conversations:read": t`Read conversations, messages, labels and canned replies`,
      "conversations:write": t`Change, move and bulk-update conversations`,
      "messages:write": t`Write messages and drafts`,
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
    } satisfies Record<ApiKeyScope, string>,
    delivery: { pending: t`Pending`, succeeded: t`Succeeded`, failed: t`Failed` } satisfies Record<
      WebhookDeliveryState,
      string
    >,
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

export function useErrorText() {
  const { t } = useLingui()
  return (err: unknown): string => {
    if (!(err instanceof ApiError)) return t`Could not reach the server. Check your connection and try again.`
    if (err.code === "email_undeliverable")
      return t`This e-mail address bounced or complained, so the reply was not sent. Clear it on the contact to send again.`
    if (err.code === "email_not_configured")
      return t`This conversation's e-mail channel has no SMTP account, so the reply was not sent. Add one in the inbox settings.`
    if (err.code === "email_address_taken") return t`Another channel already receives mail at this address.`
    if (err.code === "webhook_disabled") return t`The endpoint is disabled. Enable it to send deliveries again.`
    if (err.code === "no_email_channel")
      return t`The other inbox has no e-mail channel, so this e-mail conversation cannot move there. Add an e-mail channel to it first.`
    if (err.code === "email_no_sender")
      return t`The e-mail channel is a catch-all without a From address, so it has no address to send this conversation from. Set one in the inbox settings.`
    if (err.code === "attachment_type_mismatch") return t`A file's content does not match its type. Check the file and try again.`
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
  return new Intl.DateTimeFormat(locale, sameYear ? { day: "numeric", month: "short" } : { dateStyle: "short" }).format(d)
}

export function initials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return "?"
  return (parts[0][0] + (parts.length > 1 ? parts[parts.length - 1][0] : "")).toLocaleUpperCase()
}

export function formatDuration(seconds: number, locale: string) {
  const unit = seconds < 60 ? "second" : seconds < 3600 ? "minute" : seconds < 86400 ? "hour" : "day"
  const size = { second: 1, minute: 60, hour: 3600, day: 86400 }[unit]
  return new Intl.NumberFormat(locale, { style: "unit", unit, unitDisplay: "short", maximumFractionDigits: 0 }).format(Math.round(seconds / size))
}

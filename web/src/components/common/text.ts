import { useLingui } from "@lingui/react/macro"

import { ApiError, type ChannelKind, type ConversationStatus, type InboxMode, type Priority, type Role, type Weekday } from "@/lib/api"

export const STATUSES: ConversationStatus[] = ["open", "pending", "snoozed", "closed"]
export const PRIORITIES: Priority[] = ["urgent", "high", "normal", "low"]
export const ROLES: Role[] = ["owner", "admin", "agent"]
export const CHANNEL_KINDS: ChannelKind[] = ["email", "chat", "app", "api"]
export const MODES: InboxMode[] = ["async", "live"]
export const WEEKDAYS: Weekday[] = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"]

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

export function formatDateTime(iso: string, locale: string) {
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(new Date(iso))
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

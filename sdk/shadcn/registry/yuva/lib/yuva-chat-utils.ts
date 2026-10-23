import { ApiError } from "useyuva"

export interface YuvaChatLabels {
  title: string
  online: string
  away: string
  repliesWithin: (minutes: number) => string
  onlineReplies: string
  repliesHours: string
  repliesDays: string
  reconnecting: string
  loading: string
  loadError: string
  retry: string
  back: string
  conversations: string
  newConversation: string
  noConversations: string
  conversationRemoved: string
  noConversationsHint: string
  loadMore: string
  conversationFallback: string
  unread: string
  emptyTitle: string
  emptyDescription: string
  loadEarlier: string
  messages: string
  today: string
  yesterday: string
  team: string
  bot: string
  seen: string
  sending: string
  sendFailed: string
  remove: string
  download: string
  removed: string
  typing: (name: string) => string
  typingSomeone: string
  jumpToLatest: (unread: number) => string
  newCount: (unread: number) => string
  closed: string
  rateQuestion: string
  rateGood: string
  rateBad: string
  rated: string
  placeholder: string
  attach: string
  screenshot: string
  fileTooLarge: (name: string, limit: string) => string
  fileTypeNotAccepted: (name: string) => string
  send: string
  enterKey: string
  toSend: string
  generic: string
  offline: string
  rateLimited: string
  emailLabel: string
  emailPlaceholder: string
  emailSave: string
  emailAskTitle: string
  emailAskHint: string
  emailSentTitle: string
  emailSent: string
  emailChange: string
  emailResend: string
  emailResent: string
  emailUndeliverableTitle: string
  emailUndeliverableHint: string
  emailConfirmed: string
}

export const defaultYuvaChatLabels: YuvaChatLabels = {
  title: "Chat with us",
  online: "Online",
  away: "Away",
  repliesWithin: (minutes) => `Usually replies in ${minutes} min`,
  onlineReplies: "Online · replies in minutes",
  repliesHours: "Usually replies in a few hours",
  repliesDays: "Usually replies in a day or two",
  reconnecting: "Reconnecting…",
  loading: "Loading",
  loadError: "Could not load the chat.",
  retry: "Try again",
  back: "Back",
  conversations: "Conversations",
  newConversation: "New conversation",
  noConversations: "No conversations yet",
  conversationRemoved: "This conversation is no longer available.",
  noConversationsHint: "Start one and we will answer here.",
  loadMore: "Show more",
  conversationFallback: "Conversation",
  unread: "Unread",
  emptyTitle: "Say hello",
  emptyDescription: "Ask us anything. We reply here.",
  loadEarlier: "Show earlier messages",
  messages: "Messages",
  today: "Today",
  yesterday: "Yesterday",
  team: "Support",
  bot: "Assistant",
  seen: "Seen",
  sending: "Sending…",
  sendFailed: "Not sent",
  remove: "Remove",
  download: "Download",
  removed: "This message was removed",
  typing: (name) => `${name} is typing`,
  typingSomeone: "Typing",
  jumpToLatest: (unread) => (unread > 0 ? `${unread} new, jump to latest` : "Jump to latest"),
  newCount: (unread) => `${unread} new`,
  closed: "Conversation closed",
  rateQuestion: "How was this conversation?",
  rateGood: "Good",
  rateBad: "Bad",
  rated: "Thanks for your feedback.",
  placeholder: "Write a message",
  attach: "Attach a file",
  screenshot: "Take a screenshot",
  fileTooLarge: (name, limit) => `${name} is too large. The limit is ${limit}.`,
  fileTypeNotAccepted: (name) => `${name} is not an accepted file type.`,
  send: "Send",
  enterKey: "Enter",
  toSend: "to send",
  generic: "Something went wrong. Try again.",
  offline: "You seem to be offline.",
  rateLimited: "Too many messages. Wait a moment.",
  emailLabel: "E-mail address",
  emailPlaceholder: "you@example.com",
  emailSave: "Save",
  emailAskTitle: "Where can we reach you?",
  emailAskHint: "Leave your e-mail so replies reach you when you are away.",
  emailSentTitle: "Check your inbox",
  emailSent: "We sent a link to {address}. Open it so replies reach you by e-mail.",
  emailChange: "Change address",
  emailResend: "Send again",
  emailResent: "Sent again.",
  emailUndeliverableTitle: "We couldn't reach {address}",
  emailUndeliverableHint: "That address doesn't accept mail. Check it for a typo.",
  emailConfirmed: "Replies will also reach {address}",
}

export function chatErrorText(error: unknown, labels: YuvaChatLabels): string {
  const code = error instanceof ApiError ? error.code : ""
  if (code === "network") return labels.offline
  if (code === "rate_limited") return labels.rateLimited
  return labels.generic
}

export function initials(name: string | undefined): string {
  const words = (name ?? "").trim().split(/\s+/).filter(Boolean)
  return words
    .slice(0, 2)
    .map((word) => Array.from(word)[0]!.toUpperCase())
    .join("")
}

export function dayKey(iso: string): string {
  const date = new Date(iso)
  return `${date.getFullYear()}-${date.getMonth() + 1}-${date.getDate()}`
}

export function dayLabel(iso: string, labels: YuvaChatLabels, locale?: string): string {
  const key = dayKey(iso)
  const now = new Date()
  if (key === dayKey(now.toISOString())) return labels.today
  const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1)
  if (key === dayKey(yesterday.toISOString())) return labels.yesterday
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(new Date(iso))
}

export function timeLabel(iso: string, locale?: string): string {
  return new Intl.DateTimeFormat(locale, { timeStyle: "short" }).format(new Date(iso))
}

export function whenLabel(iso: string, locale?: string): string {
  const today = dayKey(iso) === dayKey(new Date().toISOString())
  return new Intl.DateTimeFormat(locale, today ? { timeStyle: "short" } : { dateStyle: "medium" }).format(new Date(iso))
}

export function sizeLabel(bytes: number, locale?: string): string {
  const units = ["B", "KB", "MB", "GB"]
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: value < 10 && unit > 0 ? 1 : 0 }).format(value)} ${units[unit]}`
}

export function soleLink(body: string): URL | null {
  const text = body.trim()
  if (!/^https?:\/\/\S+$/i.test(text)) return null
  try {
    return new URL(text)
  } catch {
    return null
  }
}

export function linkParts(body: string): { text: string; href?: string }[] {
  const parts: { text: string; href?: string }[] = []
  let last = 0
  for (const match of body.matchAll(/https?:\/\/[^\s<>"]+[^\s<>".,;:!?)\]]/gi)) {
    if (match.index > last) parts.push({ text: body.slice(last, match.index) })
    parts.push({ text: match[0], href: match[0] })
    last = match.index + match[0].length
  }
  if (last < body.length) parts.push({ text: body.slice(last) })
  return parts
}

export function statusLine(
  inbox: { presence?: { available: boolean }; expected_reply_minutes?: number } | null,
  labels: YuvaChatLabels
): string | null {
  if (!inbox) return null
  if (inbox.presence?.available) return labels.onlineReplies
  const minutes = inbox.expected_reply_minutes
  if (minutes) {
    if (minutes < 60) return labels.repliesWithin(minutes)
    return minutes < 24 * 60 ? labels.repliesHours : labels.repliesDays
  }
  return inbox.presence ? labels.away : null
}

export function withAddress(template: string, address: string): (string | { address: string })[] {
  const [before = "", ...rest] = template.split("{address}")
  const parts: (string | { address: string })[] = before ? [before] : []
  for (const part of rest) {
    parts.push({ address })
    if (part) parts.push(part)
  }
  return parts
}

export interface YuvaAttachmentLimits {
  max_bytes: number
  types: string[]
}

export function attachmentLimits(inbox: unknown): YuvaAttachmentLimits | null {
  const value = (inbox as { attachments?: Partial<YuvaAttachmentLimits> } | null)?.attachments
  if (!value || typeof value.max_bytes !== "number" || !Array.isArray(value.types)) return null
  return { max_bytes: value.max_bytes, types: value.types }
}

export function allowsType(limits: YuvaAttachmentLimits, type: string): boolean {
  const value = type.toLowerCase()
  return limits.types.some((entry) => entry === value || (entry.endsWith("/*") && value.startsWith(entry.slice(0, -1))))
}

export function fileProblem(
  limits: YuvaAttachmentLimits,
  file: { size: number; type: string }
): "size" | "type" | null {
  if (!allowsType(limits, file.type)) return "type"
  return file.size > limits.max_bytes ? "size" : null
}

export const canAttach = (limits: YuvaAttachmentLimits): boolean => limits.types.length > 0

export const acceptAttribute = (limits: YuvaAttachmentLimits): string => limits.types.join(",")

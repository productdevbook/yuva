import { useLingui } from "@lingui/react/macro"

import type { Contact, Inbox, Label, Member, Message, MessageAuthor } from "@/lib/api"

export type ThreadContext = {
  members: Map<string, Member>
  contact?: Contact
  labels: Label[]
  inboxes: Inbox[]
  subject: string
  expandQuoted: boolean
  inboxId?: string
  contactReadAt?: string
  find?: { query: string; current?: string }
  onQuote?: (m: Message) => void
  onForward?: (m: Message, to: Member) => void
}

function nameOf(a: MessageAuthor, ctx: ThreadContext, t: ReturnType<typeof useLingui>["t"]) {
  if (a.type === "contact") return ctx.contact?.name || ctx.contact?.emails[0] || t`Contact`
  if (a.type === "system") return t`System`
  if (a.type === "bot") return a.name || t`Bot`
  const member = a.member_id ? ctx.members.get(a.member_id) : undefined
  return member ? member.name || member.email : t`Deleted member`
}

export function useAuthorName(m: Message, ctx: ThreadContext) {
  const { t } = useLingui()
  const name = nameOf(m.author, ctx, t)
  const via = m.author.via
  return via ? t`${name} via ${via}` : name
}

export function useSenderName(m: Message, ctx: ThreadContext) {
  const { t } = useLingui()
  if (!m.sent_by) return null
  const name = nameOf(m.sent_by, ctx, t)
  const via = m.sent_by.via
  return via ? t`${name} via ${via}` : name
}

export function timeOf(iso: string, locale: string) {
  return new Intl.DateTimeFormat(locale, { timeStyle: "short" }).format(new Date(iso))
}

export function baseSubject(s: string) {
  return s.replace(/^(\s*(re|fwd?|aw|ynt|ilt)\s*:\s*)+/i, "").trim().toLowerCase()
}

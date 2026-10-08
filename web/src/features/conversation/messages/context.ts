import { useLingui } from "@lingui/react/macro"

import type { Contact, Inbox, Label, Member, Message } from "@/lib/api"

export type ThreadContext = {
  members: Map<string, Member>
  contact?: Contact
  labels: Label[]
  inboxes: Inbox[]
  subject: string
  expandQuoted: boolean
}

export function useAuthorName(m: Message, ctx: ThreadContext) {
  const { t } = useLingui()
  if (m.author.type === "contact") return ctx.contact?.name || ctx.contact?.emails[0] || t`Contact`
  if (m.author.type === "system") return t`System`
  const member = m.author.member_id ? ctx.members.get(m.author.member_id) : undefined
  return member ? member.name || member.email : t`Deleted member`
}

export function timeOf(iso: string, locale: string) {
  return new Intl.DateTimeFormat(locale, { timeStyle: "short" }).format(new Date(iso))
}

export function baseSubject(s: string) {
  return s.replace(/^(\s*(re|fwd?|aw|ynt|ilt)\s*:\s*)+/i, "").trim().toLowerCase()
}

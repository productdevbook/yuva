import { useLingui } from "@lingui/react/macro"

import type { Contact, Inbox, Label, Member, Message, MessageAuthor } from "@/lib/api"

export type ThreadContext = {
  members: Map<string, Member>
  contact?: Contact
  labels: Label[]
  inboxes: Inbox[]
  subject: string
  expandQuoted: boolean
  drafts: DraftControls
}

export type DraftControls = {
  editing: string | null
  setEditing: (id: string | null) => void
  latest?: string
  pending: boolean
  error: unknown
  send: (m: Message) => void
  save: (m: Message, body: string) => void
  discard: (m: Message) => void
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
  return nameOf(m.author, ctx, t)
}

export function useSenderName(m: Message, ctx: ThreadContext) {
  const { t } = useLingui()
  return m.sent_by ? nameOf(m.sent_by, ctx, t) : null
}

export function timeOf(iso: string, locale: string) {
  return new Intl.DateTimeFormat(locale, { timeStyle: "short" }).format(new Date(iso))
}

export function baseSubject(s: string) {
  return s.replace(/^(\s*(re|fwd?|aw|ynt|ilt)\s*:\s*)+/i, "").trim().toLowerCase()
}

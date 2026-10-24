import { useLingui } from "@lingui/react/macro"

import type { Contact, ConversationParticipant, Inbox, Label, Member, Message, MessageAuthor } from "@/lib/api"

export type ThreadContext = {
  members: Map<string, Member>
  contact?: Contact
  participants?: ConversationParticipant[]
  labels: Label[]
  inboxes: Inbox[]
  subject: string
  feedback?: boolean
  expandQuoted: boolean
  inboxId?: string
  contactReadAt?: string
  find?: { query: string; current?: string }
  onQuote?: (m: Message) => void
  onForward?: (m: Message, to: Member) => void
  onForwardMail?: (m: Message) => void
  onRemove?: (m: Message) => void
  canRemove?: (m: Message) => boolean
}

function nameOf(a: MessageAuthor, ctx: ThreadContext, t: ReturnType<typeof useLingui>["t"], from?: string) {
  if (a.type === "contact") {
    const address = from?.toLowerCase()
    if (address && ctx.contact && !ctx.contact.emails.some((e) => e.toLowerCase() === address)) {
      const person = ctx.participants?.find((p) => p.email.toLowerCase() === address)
      return person?.name || address
    }
    return ctx.contact?.name || ctx.contact?.emails[0] || t`Contact`
  }
  if (a.type === "system") return t`System`
  if (a.type === "bot") return a.name || t`Bot`
  const member = a.member_id ? ctx.members.get(a.member_id) : undefined
  return member ? member.name || member.email : t`Deleted member`
}

export function useAuthorName(m: Message, ctx: ThreadContext) {
  const { t } = useLingui()
  if (m.imported_author && !m.author.member_id) return m.imported_author.name || m.imported_author.address
  if (m.rule && m.author.type === "system") {
    const rule = m.rule.name
    return t`Rule ${rule}`
  }
  const name = nameOf(m.author, ctx, t, m.email?.from)
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
  return s
    .replace(/^(\s*(re|fwd?|aw|ynt|ilt)\s*:\s*)+/i, "")
    .trim()
    .toLowerCase()
}

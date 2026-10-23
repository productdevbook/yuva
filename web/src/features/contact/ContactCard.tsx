import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowUpRightIcon, ChevronDownIcon, XIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { ContactAvatar, ErrorLine } from "@/components/common"
import { formatRelative, formatShort, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { languageName, NewConversationDialog, useContactConversations } from "@/features/contact/ContactDetails"
import { ContactNotes } from "@/features/contact/ContactNotes"
import { ConversationMedia } from "@/features/contact/ConversationMedia"
import { ConversationFields, useConversationFieldList } from "@/features/fields/ConversationFields"
import { MergeContactDialog } from "@/features/contact/MergeContactDialog"
import { Participants } from "@/features/conversation/Participants"
import { useContact, useContactPresence } from "@/features/contact/queries"
import { useContactEntries } from "@/features/contact/entries"
import { contactName } from "@/features/contact/Section"
import { look, Overline } from "@/features/inbox/PageFrame"
import { ProductTag } from "@/features/inbox/ProductTag"
import type { Contact, ConversationListItem } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useChannelMap, useInboxes } from "@/lib/workspace"

export function CardSection({
  title,
  children,
  className,
  testId,
}: {
  title: React.ReactNode
  children: React.ReactNode
  className?: string
  testId?: string
}) {
  return (
    <section className={cn("border-b px-4 py-3.5", className)} data-testid={testId}>
      <Overline className="mb-2.5">{title}</Overline>
      {children}
    </section>
  )
}

export function Seen({ contactId }: { contactId: string }) {
  const { i18n } = useLingui()
  const presence = useContactPresence(contactId).data
  if (!presence || (!presence.online && !presence.last_seen_at)) return null
  const seen = presence.last_seen_at ? formatRelative(presence.last_seen_at, i18n.locale) : ""
  return (
    <p
      className="flex items-center gap-1.5 text-caption text-muted-foreground"
      data-testid="contact-presence"
      data-online={presence.online}
    >
      {presence.online && <span className="size-1.5 shrink-0 rounded-full bg-success" aria-hidden />}
      {presence.online ? <Trans>Online now</Trans> : <Trans>Seen {seen}</Trans>}
    </p>
  )
}

export function ProductRows({ contact }: { contact: Contact }) {
  const { t } = useLingui()
  return (
    <ul className="flex flex-col gap-2" data-testid="contact-products">
      {contact.inboxes.map((i) => {
        const ext = contact.external_ids.find((x) => x.inbox_id === i.id)?.external_id
        return (
          <li key={i.id} className="flex min-w-0 items-center gap-2">
            <ProductTag id={i.id} always />
            <span className="flex-1" />
            {ext && (
              <span className="min-w-0 truncate font-mono text-caption text-faint" title={t`User id: ${ext}`}>
                {ext}
              </span>
            )}
          </li>
        )
      })}
    </ul>
  )
}

export function DetailRows({ contact }: { contact: Contact }) {
  const { t, i18n } = useLingui()
  const entries = useContactEntries(contact).map((e): [string, string, boolean] => [e.label, e.value, e.raw])
  if (contact.locale) entries.push([t`Language`, languageName(contact.locale, i18n.locale), false])
  if (entries.length === 0) return null
  return (
    <dl className="flex flex-col gap-2" data-testid="attributes">
      {entries.map(([k, v, raw]) => (
        <div key={k} className="flex min-w-0 items-baseline gap-3">
          <dt className="shrink-0 text-muted-foreground" lang={raw ? "" : undefined}>
            {k}
          </dt>
          <dd className="min-w-0 flex-1 truncate text-end" title={v}>
            {v}
          </dd>
        </div>
      ))}
    </dl>
  )
}

export function statusDot(c: ConversationListItem) {
  return c.status === "closed" ? "bg-faint" : c.status === "snoozed" ? "bg-warning" : "bg-brand"
}

export function BeforeRows({
  items,
  currentId,
  dots,
  limit,
}: {
  items: ConversationListItem[]
  currentId?: string
  dots?: boolean
  limit?: number
}) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const inboxes = useInboxes().data ?? []
  const channels = useChannelMap()
  const rows = items.filter((c) => c.id !== currentId).slice(0, limit)
  if (rows.length === 0) {
    return (
      <p className="text-faint">
        {currentId ? <Trans>No other conversations.</Trans> : <Trans>No conversations yet.</Trans>}
      </p>
    )
  }
  return (
    <ul className="flex flex-col gap-3" data-testid="contact-conversations">
      {rows.map((c) => {
        const inbox = inboxes.find((i) => i.id === c.inbox_id)
        const channel = c.channel_id ? channels.get(c.channel_id) : undefined
        const title = c.subject || c.last_message?.text
        const kind = c.kind === "feedback" ? t`Feedback` : channel ? text.channel[channel.kind] : undefined
        return (
          <li key={c.id}>
            <Link
              to={c.kind === "feedback" ? `/feedback/${c.id}` : `/conversations/${c.id}`}
              className="group flex gap-2.5 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring"
              data-testid="contact-conversation"
            >
              {dots && <span className={cn("mt-[7px] size-1.5 shrink-0 rounded-full", statusDot(c))} aria-hidden />}
              <span className="flex min-w-0 flex-1 flex-col leading-[1.35]">
                <span
                  className={cn(
                    "truncate font-medium group-hover:underline",
                    !title && "font-normal text-muted-foreground italic"
                  )}
                >
                  {title || <Trans>No subject</Trans>}
                </span>
                <span className="truncate text-caption text-faint">
                  {[
                    inbox?.name,
                    kind,
                    formatShort(c.last_activity_at, i18n.locale),
                    dots ? text.statusOf(c.status, c.kind) : undefined,
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </span>
              </span>
            </Link>
          </li>
        )
      })}
    </ul>
  )
}

export function ContactAside({
  contactId,
  conversationId,
  onClose,
}: {
  contactId: string
  conversationId?: string
  onClose: () => void
}) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const contact = useContact(contactId)
  const conversations = useContactConversations(contactId)
  const [merging, setMerging] = useState(false)
  const [starting, setStarting] = useState(false)
  const [media, setMedia] = useState(false)
  const hasFields = useConversationFieldList().length > 0
  const c = contact.data

  const tools = (
    <span className="absolute end-2 top-2 flex gap-0.5">
      <Button
        variant="ghost"
        size="icon-xs"
        className="text-faint"
        render={<Link to={`/contacts/${contactId}`} />}
        aria-label={t`Open page`}
        title={t`Open page`}
        data-testid="open-contact-page"
      >
        <ArrowUpRightIcon />
      </Button>
      <Button variant="ghost" size="icon-xs" className="text-faint" onClick={onClose} aria-label={t`Close`}>
        <XIcon />
      </Button>
    </span>
  )

  if (contact.isPending) {
    return (
      <div className="relative flex flex-col gap-3 p-4">
        {tools}
        <Skeleton className="size-12 rounded-full" />
        <Skeleton className="h-5 w-32" />
      </div>
    )
  }
  if (!c) {
    return (
      <div className="relative p-4 pt-10">
        {tools}
        <ErrorLine error={contact.error} />
      </div>
    )
  }
  const name = contactName(c, t`Unnamed contact`)
  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <header className="relative flex flex-col items-start border-b px-4 pt-4 pb-3.5">
        {tools}
        <ContactAvatar id={c.id} name={name} className="size-12 text-[15px] font-semibold" />
        <h2 className="mt-3 max-w-full truncate text-[17px] leading-6 font-semibold" data-testid="contact-sheet-name">
          {name}
        </h2>
        {c.emails[0] && c.emails[0] !== name && (
          <span className="max-w-full truncate text-muted-foreground">{c.emails[0]}</span>
        )}
        <div className="mt-1">
          <Seen contactId={c.id} />
        </div>
        {c.blocked && (
          <span className="mt-1.5 rounded-md bg-destructive/10 px-2 py-0.5 text-caption text-destructive">
            <Trans>Blocked</Trans>
          </span>
        )}
      </header>
      {conversationId && hasFields && (
        <CardSection title={<Trans>Fields</Trans>} testId="fields-section">
          <ConversationFields conversationId={conversationId} />
        </CardSection>
      )}
      {conversationId && <Participants conversationId={conversationId} />}
      {c.inboxes.length > 0 && (
        <CardSection title={<Trans>Products</Trans>}>
          <ProductRows contact={c} />
        </CardSection>
      )}
      {(c.locale || Object.keys(c.attributes ?? {}).length > 0) && (
        <CardSection title={<Trans>Details</Trans>}>
          <DetailRows contact={c} />
        </CardSection>
      )}
      <CardSection title={<Trans>Before</Trans>}>
        {conversations.isPending ? (
          <Skeleton className="h-10 w-full" />
        ) : (
          <BeforeRows items={conversations.items} currentId={conversationId} limit={5} />
        )}
      </CardSection>
      <CardSection title={<Trans>Notes</Trans>}>
        <ContactNotes contactId={c.id} />
      </CardSection>
      {conversationId && (
        <section className="border-b px-4 py-3">
          <button
            type="button"
            onClick={() => setMedia((m) => !m)}
            aria-expanded={media}
            className={cn(look.over, "flex w-full items-center gap-1 outline-none hover:text-foreground")}
            data-testid="contact-tab-media"
          >
            <span className="flex-1 text-start">
              <Trans>Media, links and docs</Trans>
            </span>
            <ChevronDownIcon className={cn("size-3.5 transition-transform", media && "rotate-180")} />
          </button>
          {media && (
            <div className="mt-3">
              <ConversationMedia conversationId={conversationId} />
            </div>
          )}
        </section>
      )}
      <div className="flex flex-wrap gap-2 px-4 py-3.5">
        {canManage && (
          <Button variant="outline" className={look.btnSm} onClick={() => setMerging(true)} data-testid="merge-contact">
            <Trans>Merge…</Trans>
          </Button>
        )}
        <Button
          variant="outline"
          className={look.btnSm}
          onClick={() => setStarting(true)}
          data-testid="new-conversation-open"
        >
          <Trans>New conversation</Trans>
        </Button>
      </div>
      {canManage && <MergeContactDialog target={c} open={merging} onOpenChange={setMerging} />}
      <NewConversationDialog contact={c} open={starting} onOpenChange={setStarting} />
    </div>
  )
}

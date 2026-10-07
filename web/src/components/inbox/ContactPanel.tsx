import { Trans, useLingui } from "@lingui/react/macro"
import { BanIcon, LanguagesIcon, MailIcon, MailWarningIcon } from "lucide-react"
import { Link } from "react-router"

import { ErrorLine, PersonAvatar } from "@/components/common"
import { formatDateTime, formatRelative, formatShort, useEnumText } from "@/components/common/text"
import { statusIcons } from "@/components/inbox/ConversationControls"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { UndeliverableEmail } from "@/lib/api"
import { useClearUndeliverable, useContact, useContactPresence, useConversations, useInboxes } from "@/lib/queries"
import { cn } from "@/lib/utils"

function Section({ title, children }: { title: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-2 border-b px-4 py-3">
      <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{title}</h3>
      {children}
    </section>
  )
}

function languageName(tag: string, locale: string) {
  try {
    const name = new Intl.DisplayNames([locale], { type: "language" }).of(tag)
    return name && name !== tag ? `${name} (${tag})` : tag
  } catch {
    return tag
  }
}

function attrValue(v: unknown) {
  if (v === null || v === undefined) return "—"
  if (typeof v === "object") return JSON.stringify(v)
  return String(v)
}

function Undeliverable({ contactId, u }: { contactId: string; u: UndeliverableEmail }) {
  const { i18n } = useLingui()
  const clear = useClearUndeliverable(contactId)
  const since = formatDateTime(u.created_at, i18n.locale)
  return (
    <li
      className="flex flex-col gap-1 rounded-md border border-destructive/30 bg-destructive/5 px-2 py-1.5 text-sm"
      data-testid="undeliverable"
    >
      <span className="flex items-center gap-2">
        <MailWarningIcon className="size-3.5 shrink-0 text-destructive" />
        <span className="min-w-0 flex-1 truncate">{u.email}</span>
        <Button
          variant="outline"
          size="xs"
          onClick={() => clear.mutate(u.email)}
          disabled={clear.isPending}
          data-testid="clear-undeliverable"
        >
          <Trans>Clear</Trans>
        </Button>
      </span>
      <span className="text-xs text-destructive">
        {u.reason === "complaint" ? (
          <Trans>Undeliverable: marked as spam by the recipient, {since}</Trans>
        ) : (
          <Trans>Undeliverable: bounced, {since}</Trans>
        )}
      </span>
      {u.detail && <span className="text-xs break-words text-muted-foreground">{u.detail}</span>}
      <span className="text-xs text-muted-foreground">
        <Trans>Replies to this address are refused until you clear it.</Trans>
      </span>
      <ErrorLine error={clear.error} className="text-xs" />
    </li>
  )
}

function Presence({ contactId }: { contactId: string }) {
  const { i18n } = useLingui()
  const presence = useContactPresence(contactId).data
  if (!presence || (!presence.online && !presence.last_seen_at)) return null
  const seen = presence.last_seen_at ? formatRelative(presence.last_seen_at, i18n.locale) : ""
  return (
    <p
      className="flex items-center gap-1.5 text-xs text-muted-foreground"
      data-testid="contact-presence"
      data-online={presence.online}
      title={presence.last_seen_at ? formatDateTime(presence.last_seen_at, i18n.locale) : undefined}
    >
      <span className={cn("size-2 shrink-0 rounded-full", presence.online ? "bg-success" : "bg-muted-foreground/50")} />
      {presence.online ? <Trans>Online now</Trans> : <Trans>Last seen {seen}</Trans>}
    </p>
  )
}

export function ContactPanel({
  contactId,
  conversationId,
  hrefFor,
}: {
  contactId: string
  conversationId: string
  hrefFor: (id: string) => string
}) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const contact = useContact(contactId)
  const inboxes = useInboxes().data ?? []
  const others = useConversations({ contact_id: contactId })
  const c = contact.data

  if (contact.isPending) {
    return (
      <div className="flex flex-col gap-3 p-4">
        <Skeleton className="h-10 w-40" />
        <Skeleton className="h-24 w-full" />
      </div>
    )
  }
  if (!c) return <ErrorLine error={contact.error} className="p-4" />

  const name = c.name || c.emails[0] || t`Unnamed contact`
  const since = formatDateTime(c.created_at, i18n.locale)
  const attrs = Object.entries(c.attributes ?? {})
  const otherList = (others.data?.pages.flatMap((p) => p.items) ?? []).filter((x) => x.id !== conversationId)
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto" data-testid="contact-panel">
      <div className="flex items-center gap-3 border-b px-4 py-4">
        <PersonAvatar name={name} className="size-10 text-sm" />
        <div className="min-w-0">
          <p className="truncate font-medium">{name}</p>
          <Presence contactId={c.id} />
          <p className="text-xs text-muted-foreground">
            <Trans>Since {since}</Trans>
          </p>
          {c.locale && (
            <p className="flex items-center gap-1 text-xs text-muted-foreground" data-testid="contact-locale">
              <LanguagesIcon className="size-3.5 shrink-0" aria-label={t`Language`} />
              <span className="truncate">{languageName(c.locale, i18n.locale)}</span>
            </p>
          )}
        </div>
        {c.blocked && (
          <span className="ml-auto inline-flex items-center gap-1 rounded bg-destructive/10 px-1.5 py-0.5 text-xs text-destructive">
            <BanIcon className="size-3" />
            <Trans>Blocked</Trans>
          </span>
        )}
      </div>
      <Section title={<Trans>E-mail addresses</Trans>}>
        {c.emails.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            <Trans>None</Trans>
          </p>
        ) : (
          <ul className="flex flex-col gap-1">
            {c.emails.map((e) => {
              const u = c.undeliverable.find((x) => x.email === e)
              return u ? (
                <Undeliverable key={e} contactId={c.id} u={u} />
              ) : (
                <li key={e} className="flex items-center gap-2 text-sm">
                  <MailIcon className="size-3.5 shrink-0 text-muted-foreground" />
                  <span className="truncate">{e}</span>
                </li>
              )
            })}
          </ul>
        )}
      </Section>
      <Section title={<Trans>External ids</Trans>}>
        {c.external_ids.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            <Trans>None</Trans>
          </p>
        ) : (
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
            {c.external_ids.map((x) => (
              <div key={`${x.inbox_id}:${x.external_id}`} className="contents">
                <dt className="truncate text-muted-foreground">
                  {inboxes.find((i) => i.id === x.inbox_id)?.name ?? "?"}
                </dt>
                <dd className="truncate font-mono text-xs leading-5">{x.external_id}</dd>
              </div>
            ))}
          </dl>
        )}
      </Section>
      <Section title={<Trans>Attributes</Trans>}>
        {attrs.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            <Trans>None</Trans>
          </p>
        ) : (
          <dl className="grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-3 gap-y-1 text-sm">
            {attrs.map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="truncate text-muted-foreground">{k}</dt>
                <dd className="break-words">{attrValue(v)}</dd>
              </div>
            ))}
          </dl>
        )}
      </Section>
      <Section title={<Trans>Other conversations</Trans>}>
        {others.isPending ? (
          <Skeleton className="h-10 w-full" />
        ) : otherList.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            <Trans>None</Trans>
          </p>
        ) : (
          <ul className="-mx-2 flex flex-col">
            {otherList.map((o) => {
              const Icon = statusIcons[o.status]
              return (
                <li key={o.id}>
                  <Link
                    to={hrefFor(o.id)}
                    className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted"
                  >
                    <Icon className="size-3.5 shrink-0 text-muted-foreground" aria-label={text.status[o.status]} />
                    <span className={cn("min-w-0 flex-1 truncate", !o.subject && "text-muted-foreground italic")}>
                      {o.subject || <Trans>No subject</Trans>}
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {formatShort(o.last_activity_at, i18n.locale)}
                    </span>
                  </Link>
                </li>
              )
            })}
          </ul>
        )}
        {others.hasNextPage && (
          <Button
            variant="ghost"
            size="sm"
            className="self-start"
            onClick={() => void others.fetchNextPage()}
            disabled={others.isFetchingNextPage}
          >
            <Trans>Load more</Trans>
          </Button>
        )}
      </Section>
    </div>
  )
}

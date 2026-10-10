import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { useParams } from "react-router"

import { ContactAvatar, ErrorLine } from "@/components/common"
import { Pane, PaneBack } from "@/components/common/Column"
import { formatDateTime, formatDuration, formatRelative } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Attributes,
  ConversationFilters,
  ConversationRows,
  filterStatus,
  Identities,
  NewConversationDialog,
  SectionTitle,
  sinceText,
  useContactConversations,
  type ConversationFilter,
} from "@/features/contact/ContactDetails"
import { MergeContactDialog } from "@/features/contact/MergeContactDialog"
import { ContactNotes } from "@/features/contact/ContactNotes"
import { useContact, useContactSummary } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

function Card({ title, children, className }: { title: React.ReactNode; children: React.ReactNode; className?: string }) {
  return (
    <section className={cn("rounded-2xl border bg-card px-4 py-3.5", className)}>
      <SectionTitle className="mb-2.5">{title}</SectionTitle>
      {children}
    </section>
  )
}

function Stat({ value, label }: { value: string; label: React.ReactNode }) {
  return (
    <div className="flex flex-col items-center">
      <span className="text-title tabular-nums">{value}</span>
      <span className="text-caption text-faint">{label}</span>
    </div>
  )
}

export function ContactPage() {
  const { contactId = "" } = useParams()
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const contact = useContact(contactId)
  const [filter, setFilter] = useState<ConversationFilter>("all")
  const conversations = useContactConversations(contactId, filterStatus[filter])
  const summary = useContactSummary(contactId).data
  const [merging, setMerging] = useState(false)
  const [starting, setStarting] = useState(false)
  const c = contact.data
  const fmt = new Intl.NumberFormat(i18n.locale)

  if (contact.isPending) {
    return (
      <Pane wide className="gap-4">
        <Skeleton className="h-4 w-40" />
        <Skeleton className="size-16 rounded-full" />
      </Pane>
    )
  }
  if (!c) {
    return (
      <Pane wide>
        <PaneBack to="/contacts" label={t`Contacts`} />
        <ErrorLine error={contact.error} />
      </Pane>
    )
  }

  const name = contactName(c, t`Unnamed contact`)
  const since = sinceText(c, i18n.locale)
  const more = !!conversations.hasNextPage
  const good = summary?.ratings.good ?? 0
  const rated = good + (summary?.ratings.bad ?? 0)
  const seen = c.activity?.last_seen_at
  const seenText = seen ? formatRelative(seen, i18n.locale) : ""
  const attrs = !!c.locale || Object.keys(c.attributes ?? {}).length > 0

  return (
    <Pane wide className="block" testId="contact-page">
      <PaneBack to="/contacts" label={t`Contacts`} />

      <header className="flex flex-wrap items-center gap-4">
        <ContactAvatar id={c.id} name={name} className="size-16" />
        <div className="min-w-[240px] flex-1">
          <h1 className="text-page break-words" data-testid="contact-name">
            {name}
          </h1>
          <div className="mt-0.5 flex flex-wrap items-center gap-x-1.5 text-small text-muted-foreground">
            {c.emails[0] && (
              <>
                <span className="truncate">{c.emails[0]}</span>
                <span aria-hidden>·</span>
              </>
            )}
            <span>
              <Trans>Contact since {since}</Trans>
            </span>
            {seen && (
              <>
                <span aria-hidden>·</span>
                <span title={formatDateTime(seen, i18n.locale)} data-testid="contact-last-seen">
                  <Trans>last seen {seenText}</Trans>
                </span>
              </>
            )}
            {c.blocked && (
              <span className="rounded-md bg-destructive/10 px-2 py-0.5 text-caption text-destructive">
                <Trans>Blocked</Trans>
              </span>
            )}
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {canManage && (
            <Button variant="outline" onClick={() => setMerging(true)} data-testid="merge-contact">
              <Trans>Merge</Trans>
            </Button>
          )}
          <Button onClick={() => setStarting(true)} data-testid="new-conversation-open">
            <Trans>New conversation</Trans>
          </Button>
        </div>
      </header>

      <div className="mt-6 flex flex-wrap items-start gap-6">
        <section aria-label={t`Conversations`} className="flex min-w-0 flex-[999_1_520px] flex-col gap-3">
          <ConversationFilters value={filter} onChange={setFilter} contact={c} />
          <ConversationRows items={conversations.items} roomy pending={conversations.isPending} />
          {more && (
            <Button variant="ghost" size="sm" className="self-center" onClick={() => void conversations.fetchNextPage()} disabled={conversations.isFetchingNextPage}>
              <Trans>Load more</Trans>
            </Button>
          )}
        </section>

        <aside className="flex min-w-0 flex-[1_1_300px] flex-col gap-4">
          <Card title={<Trans>Contact and identities</Trans>}>
            <Identities contact={c} editable />
          </Card>
          {attrs && (
            <Card title={<Trans>Details</Trans>}>
              <Attributes contact={c} />
              {Object.keys(c.attributes ?? {}).length > 0 && (
                <p className="mt-2.5 text-caption text-faint">
                  <Trans>Your app or widget sends these when it identifies the contact.</Trans>
                </p>
              )}
            </Card>
          )}
          {c.activity && (
            <Card title={<Trans>Summary</Trans>}>
              <div className="grid grid-cols-3 gap-2" data-testid="contact-summary">
                <Stat value={fmt.format(c.activity.conversations)} label={<Trans>conversations</Trans>} />
                <Stat
                  value={summary?.median_first_reply_seconds !== undefined ? formatDuration(summary.median_first_reply_seconds, i18n.locale) : "—"}
                  label={<Trans>first reply</Trans>}
                />
                <Stat value={rated ? `${fmt.format(good)} / ${fmt.format(rated)}` : "—"} label={<Trans>rated good</Trans>} />
              </div>
            </Card>
          )}
          <Card title={<Trans>Team notes</Trans>} className="border-note-border">
            <ContactNotes contactId={c.id} />
          </Card>
        </aside>
      </div>

      {canManage && <MergeContactDialog target={c} open={merging} onOpenChange={setMerging} />}
      <NewConversationDialog contact={c} open={starting} onOpenChange={setStarting} />
    </Pane>
  )
}

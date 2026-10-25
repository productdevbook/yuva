import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, EllipsisIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { ContactAvatar, ErrorLine, toast } from "@/components/common"
import { formatDateTime, formatDuration, formatRelative } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { ContactNotes, useNoteCount } from "@/features/contact/ContactNotes"
import { ContactSubscriptions } from "@/features/contact/ContactSubscriptions"
import { ContactVisits } from "@/features/contact/ContactVisits"
import {
  Attributes,
  ConversationFilters,
  ConversationRows,
  filterStatus,
  Identities,
  NewConversationDialog,
  sinceText,
  useContactConversations,
  type ConversationFilter,
} from "@/features/contact/ContactDetails"
import { DeleteContactDialog } from "@/features/contact/DeleteContactDialog"
import { MergeContactDialog } from "@/features/contact/MergeContactDialog"
import { useContact, useContactSummary, useContactVisits, useUpdateContact } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import { Overline } from "@/features/inbox/PageFrame"
import { OverflowTabs } from "@/components/common/OverflowTabs"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const inkButton = "h-8 rounded-[9px] bg-foreground px-3.5 font-semibold text-background hover:bg-foreground/90"
const outlineButton = "h-8 rounded-[9px] border-input bg-card px-3 font-normal"

type DetailTab = "details" | "activity" | "conversations" | "notes"

function Block({
  page,
  title,
  className,
  children,
}: {
  page: boolean
  title: React.ReactNode
  className?: string
  children: React.ReactNode
}) {
  return (
    <section
      className={cn(page ? "rounded-2xl border bg-card px-4 py-3.5" : "border-b border-b-muted px-5 py-4", className)}
    >
      <Overline className="mb-2.5 font-semibold tracking-[0.06em]">{title}</Overline>
      {children}
    </section>
  )
}

function Stat({ value, label }: { value: string; label: React.ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-0.5 px-1.5 py-2.5">
      <span className="text-[17px] leading-6 font-semibold tabular-nums">{value}</span>
      <span className="text-caption text-faint">{label}</span>
    </div>
  )
}

function StatStrip({ children }: { children: React.ReactNode }) {
  return (
    <div
      className="grid grid-cols-3 overflow-hidden rounded-xl border [&>*]:border-s [&>*:first-child]:border-s-0"
      data-testid="contact-summary"
    >
      {children}
    </div>
  )
}

function Tag({ tone, children }: { tone: "danger" | "warn"; children: React.ReactNode }) {
  return (
    <span
      className={cn(
        "inline-flex h-[22px] items-center gap-1.5 rounded-full px-2 text-caption font-medium before:size-1.5 before:rounded-full before:bg-current",
        tone === "danger" ? "bg-destructive/10 text-destructive" : "bg-warning/10 text-warning"
      )}
    >
      {children}
    </span>
  )
}

function Banner({ tone, children, testId }: { tone: "danger" | "warn"; children: React.ReactNode; testId: string }) {
  return (
    <p
      role="status"
      className={cn(
        "rounded-[10px] px-3 py-2.5 text-small leading-[19px]",
        tone === "danger" ? "bg-destructive/10 text-destructive" : "bg-warning/10 text-warning"
      )}
      data-testid={testId}
    >
      {children}
    </p>
  )
}

export function ContactDetail({
  id,
  page,
  pageLink,
  back,
}: {
  id: string
  page: boolean
  pageLink?: string
  back?: { to: string; label: string }
}) {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const contact = useContact(id)
  const update = useUpdateContact(id)
  const [tab, setTab] = useState<DetailTab>("details")
  const [filter, setFilter] = useState<ConversationFilter>("all")
  const conversations = useContactConversations(id, filterStatus[filter])
  const summary = useContactSummary(id).data
  const visits = useContactVisits(id)
  const noteCount = useNoteCount(id)
  const [merging, setMerging] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [starting, setStarting] = useState(false)
  const c = contact.data
  const fmt = new Intl.NumberFormat(i18n.locale)

  if (contact.isPending) {
    return (
      <div className="flex flex-col gap-4 p-5" data-testid="contact-panel" aria-busy="true">
        <div className="flex items-center gap-3.5">
          <Skeleton className="size-14 shrink-0 rounded-full" />
          <div className="flex flex-1 flex-col gap-2">
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-3.5 w-28" />
          </div>
        </div>
        <Skeleton className="h-8 w-full rounded-[9px]" />
        <Skeleton className="h-16 w-full rounded-xl" />
      </div>
    )
  }
  if (!c) {
    return (
      <div className="flex flex-col gap-3 p-5" data-testid="contact-panel">
        {back && <BackLink {...back} />}
        <ErrorLine error={contact.error} />
      </div>
    )
  }

  const name = contactName(c, t`Unnamed contact`)
  const since = sinceText(c, i18n.locale)
  const seen = c.activity?.last_seen_at
  const seenText = seen ? formatRelative(seen, i18n.locale) : ""
  const good = summary?.ratings.good ?? 0
  const rated = good + (summary?.ratings.bad ?? 0)
  const visited = (visits.data?.pages[0]?.items.length ?? 0) > 0
  const attrCount = Object.keys(c.attributes ?? {}).length
  const blocked = c.blocked
  const undeliverable = c.undeliverable.length > 0

  const toggleBlock = () =>
    update.mutate(
      { blocked: !blocked },
      { onSuccess: () => toast(blocked ? t`${name} unblocked` : t`${name} blocked. Their mail is refused.`) }
    )

  const identities = (
    <Block page={page} title={<Trans>Contact and identities</Trans>}>
      <Identities contact={c} editable />
      {c.external_ids.length === 0 && (
        <p className="mt-2 text-caption text-faint">
          <Trans>No ids yet. This person has not been identified by an app or widget.</Trans>
        </p>
      )}
    </Block>
  )
  const attributes = (attrCount > 0 || !!c.locale) && (
    <Block page={page} title={<Trans>Attributes</Trans>}>
      <Attributes contact={c} />
      {attrCount > 0 && (
        <p className="mt-2.5 text-caption text-faint">
          <Trans>Your app or widget sends these when it identifies the contact.</Trans>
        </p>
      )}
    </Block>
  )
  const lists = (
    <Block page={page} title={<Trans>Mailing lists</Trans>}>
      <ContactSubscriptions contactId={c.id} />
    </Block>
  )
  const statCells = c.activity && (
    <>
      <Stat value={fmt.format(c.activity.conversations)} label={<Trans>conversations</Trans>} />
      <Stat
        value={
          summary?.median_first_reply_seconds !== undefined
            ? formatDuration(summary.median_first_reply_seconds, i18n.locale)
            : "—"
        }
        label={<Trans>first reply</Trans>}
      />
      <Stat value={rated ? `${fmt.format(good)} / ${fmt.format(rated)}` : "—"} label={<Trans>rated good</Trans>} />
    </>
  )
  const stats = statCells && (
    <Block page={page} title={<Trans>Summary</Trans>}>
      <div className="grid grid-cols-3 gap-2" data-testid="contact-summary">
        {statCells}
      </div>
    </Block>
  )
  const visitsBlock = !visits.error && (
    <Block page={page} title={<Trans>Recent visits</Trans>}>
      {visited ? (
        <ContactVisits visits={visits} />
      ) : visits.isPending ? (
        <Skeleton className="h-10 w-full" />
      ) : (
        <p className="text-faint">
          <Trans>No visits yet.</Trans>
        </p>
      )}
    </Block>
  )
  const convs = (
    <Block page={page} title={<Trans>Conversations</Trans>} className="flex flex-col gap-3">
      <ConversationFilters value={filter} onChange={setFilter} contact={c} />
      <ConversationRows items={conversations.items} roomy={page} pending={conversations.isPending} />
      {conversations.hasNextPage && (
        <Button
          variant="ghost"
          size="sm"
          className="self-center"
          onClick={() => void conversations.fetchNextPage()}
          disabled={conversations.isFetchingNextPage}
        >
          <Trans>Load more</Trans>
        </Button>
      )}
    </Block>
  )
  const notes = (
    <Block page={page} title={<Trans>Team notes</Trans>} className={page ? "border-note-border" : undefined}>
      <ContactNotes contactId={c.id} />
    </Block>
  )

  const tabs: { key: DetailTab; label: React.ReactNode; count?: string }[] = [
    { key: "details", label: <Trans>Details</Trans> },
    { key: "activity", label: <Trans>Activity</Trans> },
    {
      key: "conversations",
      label: <Trans>Conversations</Trans>,
      count: c.activity ? fmt.format(c.activity.conversations) : undefined,
    },
    { key: "notes", label: <Trans>Notes</Trans>, count: noteCount && noteCount !== "0" ? noteCount : undefined },
  ]

  return (
    <div className={cn("flex min-w-0 flex-col", page ? "gap-4" : "h-full min-h-0")} data-testid="contact-panel">
      {back && <BackLink {...back} />}
      <header className={cn("@container flex shrink-0 flex-col gap-3.5", !page && "border-b px-5 pt-4 pb-4")}>
        <div className="flex min-w-0 items-start gap-3.5">
          <ContactAvatar
            id={c.id}
            name={name}
            className={cn("shrink-0 font-semibold", page ? "size-16" : "size-14 text-lg")}
          />
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <h2
              className={cn(
                "min-w-0 [overflow-wrap:anywhere]",
                page ? "text-page" : "text-[18px] leading-6 font-semibold"
              )}
              data-testid="contact-name"
            >
              {name}
            </h2>
            {c.emails[0] && c.emails[0] !== name && (
              <span className="truncate text-small text-muted-foreground">{c.emails[0]}</span>
            )}
            <span className="text-small text-faint">
              <Trans>Contact since {since}</Trans>
              {seen && (
                <>
                  {" · "}
                  <span title={formatDateTime(seen, i18n.locale)} data-testid="contact-last-seen">
                    <Trans>last seen {seenText}</Trans>
                  </span>
                </>
              )}
            </span>
            {(blocked || undeliverable) && (
              <span className="mt-1 flex flex-wrap gap-1.5">
                {blocked && (
                  <Tag tone="danger">
                    <Trans>Blocked</Trans>
                  </Tag>
                )}
                {undeliverable && (
                  <Tag tone="warn">
                    <Trans>Undeliverable</Trans>
                  </Tag>
                )}
              </span>
            )}
          </div>
        </div>
        <div className="flex gap-2">
          <Button
            className={cn(inkButton, "min-w-0 flex-1")}
            onClick={() => setStarting(true)}
            data-testid="new-conversation-open"
          >
            <Trans>New conversation</Trans>
          </Button>
          {!page && pageLink && (
            <Button variant="outline" className={cn(outlineButton, "@max-sm:hidden")} render={<Link to={pageLink} />}>
              <Trans>Open full page</Trans>
            </Button>
          )}
          {canManage && (
            <Button
              variant="outline"
              className={outlineButton}
              onClick={() => setMerging(true)}
              data-testid="merge-contact"
            >
              <Trans>Merge…</Trans>
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button variant="outline" className={cn(outlineButton, "w-8 px-0")} />}
              aria-label={t`More`}
              data-testid="contact-more"
            >
              <EllipsisIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-48">
              {!page && pageLink && (
                <DropdownMenuItem render={<Link to={pageLink} />} data-testid="open-contact-page">
                  <Trans>Open full page</Trans>
                </DropdownMenuItem>
              )}
              <DropdownMenuItem
                onClick={() => {
                  void navigator.clipboard?.writeText(c.id)
                  toast(t`Contact id copied`)
                }}
              >
                <Trans>Copy contact id</Trans>
              </DropdownMenuItem>
              <DropdownMenuItem onClick={toggleBlock} data-testid="contact-block">
                {blocked ? <Trans>Unblock contact</Trans> : <Trans>Block contact</Trans>}
              </DropdownMenuItem>
              {canManage && (
                <DropdownMenuItem variant="destructive" onClick={() => setDeleting(true)} data-testid="contact-delete">
                  <Trans>Delete contact</Trans>
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <ErrorLine error={update.error} />
        {blocked && (
          <Banner tone="danger" testId="contact-blocked">
            <Trans>
              Blocked. Mail from this contact is refused and nothing new reaches the inbox. Unblock them from the menu.
            </Trans>
          </Banner>
        )}
        {undeliverable && (
          <Banner tone="warn" testId="contact-undeliverable">
            <Trans>
              Some addresses are undeliverable. Clear them under Contact and identities once they work again.
            </Trans>
          </Banner>
        )}
        {!page && statCells && <StatStrip>{statCells}</StatStrip>}
      </header>

      {page ? (
        <div className="grid grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)] items-start gap-4 phone:grid-cols-1">
          <div className="flex min-w-0 flex-col gap-4">
            {convs}
            {notes}
          </div>
          <div className="flex min-w-0 flex-col gap-4">
            {identities}
            {attributes}
            {lists}
            {stats}
            {visitsBlock}
          </div>
        </div>
      ) : (
        <>
          <OverflowTabs
            label={t`Contact`}
            current={tab}
            className="mx-0 shrink-0 border-b px-5"
            items={tabs.map((x) => ({
              key: x.key,
              label: x.label,
              count: x.count,
              testId: `contact-tab-${x.key}`,
              onSelect: () => setTab(x.key),
            }))}
          />
          <div className="min-h-0 flex-1 overflow-y-auto">
            {tab === "details" && (
              <>
                {identities}
                {attributes}
                {lists}
              </>
            )}
            {tab === "activity" && visitsBlock}
            {tab === "conversations" && convs}
            {tab === "notes" && notes}
          </div>
        </>
      )}

      {canManage && <MergeContactDialog target={c} open={merging} onOpenChange={setMerging} />}
      {canManage && <DeleteContactDialog contact={c} name={name} open={deleting} onOpenChange={setDeleting} />}
      <NewConversationDialog
        contact={c}
        open={starting}
        onOpenChange={setStarting}
        onCreated={() => {
          setFilter("all")
          setTab("conversations")
          toast(t`Conversation started`)
        }}
      />
    </div>
  )
}

function BackLink({ to, label }: { to: string; label: string }) {
  return (
    <Link
      to={to}
      className="inline-flex w-fit items-center gap-1.5 text-body text-faint transition-colors hover:text-foreground"
      data-testid="contact-back"
    >
      <ArrowLeftIcon className="size-3.5 rtl:rotate-180" />
      {label}
    </Link>
  )
}

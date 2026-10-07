import { Trans, useLingui } from "@lingui/react/macro"
import { FilterIcon, FlagIcon, InboxIcon, SearchIcon, XIcon } from "lucide-react"
import { forwardRef, useEffect, useRef, useState } from "react"
import { Link } from "react-router"

import { EmptyState, ErrorLine, LabelChip, PersonAvatar } from "@/components/common"
import { formatShort, STATUSES, useEnumText } from "@/components/common/text"
import { priorityClass, statusIcons } from "@/components/inbox/ConversationControls"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { Contact, Conversation, ConversationStatus } from "@/lib/api"
import { useInboxes, useLabels, useMemberMap, useMembers } from "@/lib/queries"
import { cn } from "@/lib/utils"

export type ListFilters = {
  status: ConversationStatus | "all"
  q: string
  inbox: string
  label: string
  assignee: string
}

export type ListBase = { kind: "view" | "inbox" | "label"; id: string }

type Props = {
  base: ListBase
  filters: ListFilters
  setFilters: (patch: Partial<ListFilters>) => void
  conversations: Conversation[]
  contacts: Map<string, Contact>
  selectedId?: string
  hrefFor: (id: string) => string
  isPending: boolean
  error: unknown
  hasNextPage: boolean
  isFetchingNextPage: boolean
  fetchNextPage: () => void
}

export const ConversationList = forwardRef<HTMLInputElement, Props>(function ConversationList(p, searchRef) {
  const { t } = useLingui()
  const text = useEnumText()
  const [q, setQ] = useState(p.filters.q)
  const sentinel = useRef<HTMLDivElement>(null)

  useEffect(() => setQ(p.filters.q), [p.filters.q])
  const { setFilters } = p
  const currentQ = p.filters.q
  useEffect(() => {
    if (q.trim() === currentQ) return
    const id = setTimeout(() => setFilters({ q: q.trim() }), 300)
    return () => clearTimeout(id)
  }, [q, currentQ, setFilters])

  const { hasNextPage, isFetchingNextPage, fetchNextPage } = p
  useEffect(() => {
    const el = sentinel.current
    if (!el || !hasNextPage) return
    const io = new IntersectionObserver((entries) => {
      if (entries[0]?.isIntersecting && !isFetchingNextPage) fetchNextPage()
    })
    io.observe(el)
    return () => io.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-col gap-2 border-b p-3">
        <div className="flex items-center gap-2">
          <div className="relative flex-1">
            <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              ref={searchRef}
              type="search"
              value={q}
              onChange={(e) => setQ(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Escape") e.currentTarget.blur()
              }}
              placeholder={t`Search conversations`}
              aria-label={t`Search conversations`}
              className="pl-8"
            />
          </div>
          <FilterMenu {...p} />
        </div>
        <Tabs value={p.filters.status} onValueChange={(v) => p.setFilters({ status: v as ListFilters["status"] })}>
          <TabsList className="w-full">
            {[...STATUSES, "all" as const].map((s) => (
              <TabsTrigger key={s} value={s} className="px-1.5 text-xs">
                {s === "all" ? <Trans>All</Trans> : text.status[s]}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto" data-testid="conversation-list">
        {p.isPending ? (
          <div className="flex flex-col gap-3 p-3">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-16 w-full" />
            ))}
          </div>
        ) : p.error ? (
          <ErrorLine error={p.error} className="p-4" />
        ) : p.conversations.length === 0 ? (
          <EmptyState icon={InboxIcon} title={<Trans>No conversations here</Trans>} className="py-16">
            {p.filters.q ? (
              <Trans>Nothing matches your search.</Trans>
            ) : (
              <Trans>New conversations show up here.</Trans>
            )}
          </EmptyState>
        ) : (
          <ul>
            {p.conversations.map((c) => (
              <Row
                key={c.id}
                c={c}
                contact={p.contacts.get(c.contact_id)}
                href={p.hrefFor(c.id)}
                selected={c.id === p.selectedId}
                showStatus={p.filters.status === "all"}
                showInbox={p.base.kind !== "inbox"}
              />
            ))}
          </ul>
        )}
        {p.hasNextPage && (
          <div ref={sentinel} className="flex justify-center p-3">
            <Button variant="ghost" size="sm" onClick={() => p.fetchNextPage()} disabled={p.isFetchingNextPage}>
              <Trans>Load more</Trans>
            </Button>
          </div>
        )}
      </div>
    </div>
  )
})

function Row({
  c,
  contact,
  href,
  selected,
  showStatus,
  showInbox,
}: {
  c: Conversation
  contact?: Contact
  href: string
  selected: boolean
  showStatus: boolean
  showInbox: boolean
}) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const members = useMemberMap()
  const inboxes = useInboxes().data ?? []
  const labels = useLabels().data ?? []
  const inbox = inboxes.find((i) => i.id === c.inbox_id)
  const assignee = c.assignee_id ? members.get(c.assignee_id) : undefined
  const name = contact ? contact.name || contact.emails[0] || t`Unnamed contact` : "…"
  const StatusIcon = statusIcons[c.status]
  return (
    <li>
      <Link
        to={href}
        data-testid="conversation-row"
        aria-current={selected ? "true" : undefined}
        className={cn(
          "flex gap-3 border-b px-3 py-3 transition-colors hover:bg-muted/60",
          selected && "bg-accent hover:bg-accent",
        )}
      >
        <PersonAvatar name={name} className="mt-0.5 size-8 text-xs" />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex items-baseline gap-2">
            <span className="min-w-0 flex-1 truncate text-sm font-medium">{name}</span>
            <time className="shrink-0 text-xs text-muted-foreground" dateTime={c.last_activity_at}>
              {formatShort(c.last_activity_at, i18n.locale)}
            </time>
          </div>
          <p className={cn("truncate text-sm", !c.subject && "text-muted-foreground italic")}>
            {c.subject || <Trans>No subject</Trans>}
          </p>
          <div className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
            {showStatus && (
              <span className="inline-flex items-center gap-1">
                <StatusIcon className="size-3.5" />
                {text.status[c.status]}
              </span>
            )}
            {(c.priority === "urgent" || c.priority === "high") && (
              <span className={cn("inline-flex items-center gap-0.5", priorityClass[c.priority])}>
                <FlagIcon className="size-3.5" />
                {text.priority[c.priority]}
              </span>
            )}
            {showInbox && inbox && <span className="truncate">{inbox.name}</span>}
            {labels
              .filter((l) => c.labels.includes(l.id))
              .slice(0, 2)
              .map((l) => (
                <LabelChip key={l.id} name={l.name} color={l.color} className="py-0 text-[11px]" />
              ))}
            {assignee && (
              <span className="ml-auto" title={assignee.name || assignee.email}>
                <PersonAvatar name={assignee.name || assignee.email} className="size-5 text-[9px]" />
              </span>
            )}
          </div>
        </div>
      </Link>
    </li>
  )
}

function FilterMenu(p: Props) {
  const { t } = useLingui()
  const inboxes = useInboxes().data ?? []
  const labels = useLabels().data ?? []
  const members = useMembers().data ?? []
  const active =
    (p.base.kind !== "inbox" && p.filters.inbox !== "") ||
    (p.base.kind !== "label" && p.filters.label !== "") ||
    (p.base.kind !== "view" && p.filters.assignee !== "")
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant={active ? "secondary" : "outline"} size="icon" />}
        aria-label={t`Filters`}
      >
        <FilterIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-h-[70svh] min-w-56 overflow-y-auto">
        {p.base.kind !== "view" && (
          <DropdownMenuGroup>
            <DropdownMenuLabel>
              <Trans>Assignee</Trans>
            </DropdownMenuLabel>
            <DropdownMenuRadioGroup value={p.filters.assignee} onValueChange={(v) => p.setFilters({ assignee: String(v) })}>
              <DropdownMenuRadioItem value="">
                <Trans>Anyone</Trans>
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="me">
                <Trans>Me</Trans>
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="unassigned">
                <Trans>Unassigned</Trans>
              </DropdownMenuRadioItem>
              {members.map((m) => (
                <DropdownMenuRadioItem key={m.id} value={m.id}>
                  {m.name || m.email}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuGroup>
        )}
        {p.base.kind !== "inbox" && (
          <>
            {p.base.kind !== "view" && <DropdownMenuSeparator />}
            <DropdownMenuGroup>
              <DropdownMenuLabel>
                <Trans>Inbox</Trans>
              </DropdownMenuLabel>
              <DropdownMenuRadioGroup value={p.filters.inbox} onValueChange={(v) => p.setFilters({ inbox: String(v) })}>
                <DropdownMenuRadioItem value="">
                  <Trans>All inboxes</Trans>
                </DropdownMenuRadioItem>
                {inboxes.map((i) => (
                  <DropdownMenuRadioItem key={i.id} value={i.id}>
                    {i.name}
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuGroup>
          </>
        )}
        {p.base.kind !== "label" && labels.length > 0 && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuLabel>
                <Trans>Label</Trans>
              </DropdownMenuLabel>
              <DropdownMenuRadioGroup value={p.filters.label} onValueChange={(v) => p.setFilters({ label: String(v) })}>
                <DropdownMenuRadioItem value="">
                  <Trans>Any label</Trans>
                </DropdownMenuRadioItem>
                {labels.map((l) => (
                  <DropdownMenuRadioItem key={l.id} value={l.id}>
                    {l.name}
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuGroup>
          </>
        )}
        {active && (
          <>
            <DropdownMenuSeparator />
            <Button
              variant="ghost"
              size="sm"
              className="w-full justify-start"
              onClick={() => p.setFilters({ inbox: "", label: "", assignee: "" })}
            >
              <XIcon />
              <Trans>Clear filters</Trans>
            </Button>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

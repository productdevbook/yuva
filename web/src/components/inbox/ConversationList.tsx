import { Trans, useLingui } from "@lingui/react/macro"
import { CircleAlertIcon, FilterIcon, FlagIcon, InboxIcon, MessageCircleIcon, ReplyIcon, SearchIcon, XIcon } from "lucide-react"
import { forwardRef, useEffect, useRef, useState } from "react"
import { Link } from "react-router"

import { EmptyState, ErrorLine, LabelChip, PersonAvatar, TypingDots } from "@/components/common"
import { formatShort, STATUSES, useEnumText, useErrorText } from "@/components/common/text"
import { BulkBar, type BulkChange } from "@/components/inbox/BulkBar"
import { priorityClass, statusIcons } from "@/components/inbox/ConversationControls"
import { CategoryChip } from "@/components/inbox/Feedback"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
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
import { ApiError, type ConversationBulkFailure, type ConversationListItem, type ConversationStatus } from "@/lib/api"
import {
  useBulkUpdateConversations,
  useChannel,
  useInboxes,
  useLabels,
  useMemberMap,
  useMembers,
} from "@/lib/queries"
import { useContactTyping } from "@/lib/typing"
import { cn } from "@/lib/utils"

export type ListFilters = {
  status: ConversationStatus | "all"
  q: string
  inbox: string
  label: string
  assignee: string
}

export type ListBase = { kind: "view" | "inbox" | "label" | "feedback"; id: string }

type Props = {
  base: ListBase
  filters: ListFilters
  setFilters: (patch: Partial<ListFilters>) => void
  conversations: ConversationListItem[]
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
  const errorText = useErrorText()
  const bulk = useBulkUpdateConversations()
  const [picked, setPicked] = useState<ReadonlySet<string>>(new Set())
  const [failed, setFailed] = useState<ReadonlyMap<string, ConversationBulkFailure>>(new Map())
  const anchor = useRef<string | null>(null)
  const ids = p.conversations.map((c) => c.id)
  const selection = ids.filter((id) => picked.has(id))

  const toggle = (id: string, on: boolean, range: boolean) => {
    const from = range && anchor.current ? ids.indexOf(anchor.current) : -1
    const to = ids.indexOf(id)
    const span = from === -1 ? [id] : ids.slice(Math.min(from, to), Math.max(from, to) + 1)
    setPicked((prev) => {
      const next = new Set(prev)
      for (const x of span) {
        if (on) next.add(x)
        else next.delete(x)
      }
      return next
    })
    anchor.current = id
  }
  const clear = () => {
    setPicked(new Set())
    setFailed(new Map())
    bulk.reset()
    anchor.current = null
  }
  const apply = (change: BulkChange) => {
    setFailed(new Map())
    bulk.mutate(
      { conversation_ids: selection, ...change },
      {
        onSuccess: (r) => {
          const refused = new Map(r.failed.map((f) => [f.id, f]))
          setFailed(refused)
          setPicked(new Set(selection.filter((id) => refused.has(id))))
        },
      },
    )
  }
  const failureText = (f: ConversationBulkFailure) => {
    const reason = errorText(new ApiError(f.status, { code: f.code }))
    return f.code === "validation_failed"
      ? t`Not changed: the assignee cannot see this conversation's inbox.`
      : f.code === "not_found"
        ? t`Not changed: it no longer exists or you cannot see it.`
        : t`Not changed: ${reason}`
  }

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
      {p.conversations.length > 0 && (
        <BulkBar
          total={ids.length}
          count={selection.length}
          pending={bulk.isPending}
          error={bulk.error}
          failed={[...failed.keys()].filter((id) => ids.includes(id)).length}
          onSelectAll={(on) => {
            setPicked(on ? new Set(ids) : new Set())
            anchor.current = null
          }}
          onClear={clear}
          onApply={apply}
        />
      )}
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
                href={p.hrefFor(c.id)}
                selected={c.id === p.selectedId}
                showStatus={p.filters.status === "all"}
                showInbox={p.base.kind !== "inbox"}
                picked={picked.has(c.id)}
                onPick={(on, range) => toggle(c.id, on, range)}
                failure={failed.has(c.id) ? failureText(failed.get(c.id)!) : undefined}
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
  href,
  selected,
  showStatus,
  showInbox,
  picked,
  onPick,
  failure,
}: {
  c: ConversationListItem
  href: string
  selected: boolean
  showStatus: boolean
  showInbox: boolean
  picked: boolean
  onPick: (on: boolean, range: boolean) => void
  failure?: string
}) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const members = useMemberMap()
  const inboxes = useInboxes().data ?? []
  const labels = useLabels().data ?? []
  const inbox = inboxes.find((i) => i.id === c.inbox_id)
  const assignee = c.assignee_id ? members.get(c.assignee_id) : undefined
  const name = c.contact.name || c.contact.email || t`Unnamed contact`
  const preview = c.last_message
  const typing = useContactTyping(c.id)
  const chat = useChannel(c.channel_id).data?.kind === "chat"
  const StatusIcon = statusIcons[c.status]
  return (
    <li
      className={cn(
        "flex border-b transition-colors hover:bg-muted/60",
        picked && "bg-primary/5",
        selected && "bg-accent hover:bg-accent",
      )}
      data-testid="conversation-item"
      data-picked={picked || undefined}
      data-failed={failure ? true : undefined}
    >
      <div className="flex shrink-0 pt-3.5 pl-3" onMouseDown={(e) => e.shiftKey && e.preventDefault()}>
        <Checkbox
          checked={picked}
          onCheckedChange={(on, details) => onPick(on, (details.event as MouseEvent).shiftKey === true)}
          aria-label={t`Select ${name}`}
          data-testid="row-select"
        />
      </div>
      <Link
        to={href}
        data-testid="conversation-row"
        data-unread={c.unread || undefined}
        aria-current={selected ? "true" : undefined}
        className="flex min-w-0 flex-1 gap-3 py-3 pr-3 pl-3"
      >
        <PersonAvatar name={name} className="mt-0.5 size-8 text-xs" />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex items-center gap-2">
            <span className={cn("min-w-0 flex-1 truncate text-sm", c.unread ? "font-semibold" : "font-medium")}>
              {name}
            </span>
            <time
              className={cn("shrink-0 text-xs", c.unread ? "font-medium text-foreground" : "text-muted-foreground")}
              dateTime={c.last_activity_at}
            >
              {formatShort(c.last_activity_at, i18n.locale)}
            </time>
            {c.unread && (
              <span className="size-2 shrink-0 rounded-full bg-primary" data-testid="unread-dot">
                <span className="sr-only">
                  <Trans>Unread</Trans>
                </span>
              </span>
            )}
          </div>
          <p
            className={cn(
              "truncate text-sm",
              !c.subject && "text-muted-foreground italic",
              c.unread && "font-semibold",
            )}
          >
            {c.subject || <Trans>No subject</Trans>}
          </p>
          {typing ? (
            <p className="flex min-w-0 items-center gap-1.5 text-xs text-primary" data-testid="row-typing">
              <TypingDots />
              <span className="truncate">
                <Trans>typing…</Trans>
              </span>
            </p>
          ) : preview && (
            <p
              className={cn("flex min-w-0 items-center gap-1 text-xs", c.unread ? "text-foreground" : "text-muted-foreground")}
              data-testid="conversation-preview"
            >
              {preview.author_type === "member" && <ReplyIcon className="size-3 shrink-0" aria-label={t`Reply`} />}
              <span className="truncate">{preview.text || <Trans>Attachment</Trans>}</span>
            </p>
          )}
          <div className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
            {c.kind === "feedback" && c.feedback && <CategoryChip category={c.feedback.category} />}
            {chat && (
              <span className="inline-flex shrink-0 items-center gap-1" data-testid="chat-badge">
                <MessageCircleIcon className="size-3.5" />
                {text.channel.chat}
              </span>
            )}
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
          {failure && (
            <p className="flex items-start gap-1 text-xs text-destructive" role="alert" data-testid="bulk-failure">
              <CircleAlertIcon className="mt-0.5 size-3.5 shrink-0" />
              <span>{failure}</span>
            </p>
          )}
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

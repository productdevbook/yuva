import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, MessageSquareHeartIcon, PaperclipIcon, SearchIcon, XIcon } from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"

import { ErrorLine, hostPath, Kbd, useContactName } from "@/components/common"
import { categoryTone, useFeedbackCategories } from "@/components/common/feedbackCategories"
import { useEnumText } from "@/components/common/text"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"
import { ScoreMark } from "@/features/conversation/Feedback"
import { IdeaMark } from "@/features/conversation/IdeaChip"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { InboxPicker } from "@/features/inbox/InboxPicker"
import { ListHead, UnderlineTabs } from "@/features/inbox/ListHead"
import { Ago, look } from "@/features/inbox/PageFrame"
import { ProductTag } from "@/features/inbox/ProductTag"
import { useListCursor } from "@/features/inbox/listCursor"
import { setListQuery, useListSearch } from "@/features/inbox/listSearch"
import { useConversations, useCounts } from "@/features/inbox/queries"
import type { ConversationListItem, Feedback, FeedbackCategory, FeedbackProgress } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useChannelMap } from "@/lib/workspace"

type Tab = "open" | "resolved"
type Category = FeedbackCategory | "all"

let rememberedTab: Tab = "open"
let rememberedCategory: Category = "all"
let focused = 0

function remember(tab: Tab, category: Category) {
  rememberedTab = tab
  rememberedCategory = category
}

function useDebounced(value: string, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

export const progressTone: Record<FeedbackProgress, string> = {
  planned: "border-blue-600/35 text-blue-600 dark:border-blue-400/35 dark:text-blue-400",
  in_progress: "border-warning/35 text-warning",
  done: "border-success/35 text-success",
  wont_do: "border-faint/35 text-faint",
}

export function CategoryPill({
  category,
  inboxId,
  className,
}: {
  category: FeedbackCategory
  inboxId?: string
  className?: string
}) {
  const { find } = useFeedbackCategories()
  return (
    <span
      className={cn(
        "inline-flex min-w-[52px] max-w-32 shrink-0 justify-center rounded-full border px-1 py-px text-caption font-medium",
        categoryTone(category).pill,
        className
      )}
      data-testid="category-chip"
      data-category={category}
    >
      <span className="truncate">{find(category, inboxId).name}</span>
    </span>
  )
}

export function ProgressPill({ progress }: { progress: FeedbackProgress }) {
  const text = useEnumText()
  return (
    <span className={cn(look.pill, progressTone[progress])} data-testid="progress-chip" data-progress={progress}>
      {text.progress[progress]}
    </span>
  )
}

export function feedbackPlace(f?: Feedback) {
  if (!f) return ""
  if (f.page_url) return [f.page_title || hostPath(f.page_url)].filter(Boolean).join(" · ")
  const app = [f.app_version && `${f.app_version}`, [f.os, f.os_version].filter(Boolean).join(" "), f.device_model]
  return app.filter(Boolean).join(" · ")
}

function Row({
  c,
  current,
  cursor,
  onOpen,
}: {
  c: ConversationListItem
  current: boolean
  cursor: boolean
  onOpen: () => void
}) {
  const { t } = useLingui()
  const channel = useChannelMap().get(c.channel_id ?? "")
  const contactName = useContactName()
  const name = contactName(c, channel, c.contact.name, c.contact.email)
  const f = c.feedback
  const where = feedbackPlace(f)
  const answered = c.last_message?.author_type === "member" || c.last_message?.author_type === "bot"
  return (
    <button
      type="button"
      onClick={onOpen}
      className={cn(
        "flex w-full items-center gap-3.5 border-b px-6 py-3 text-start outline-none transition-colors phone:px-4",
        current ? "bg-surface" : "hover:bg-surface",
        cursor && !current && "bg-surface",
        (current || cursor) && "shadow-[inset_2px_0_0_var(--brand)] rtl:shadow-[inset_-2px_0_0_var(--brand)]"
      )}
      aria-current={current || undefined}
      data-testid="feedback-row"
      data-conversation-id={c.id}
      data-unread={c.unread || undefined}
    >
      {f && <CategoryPill category={f.category} inboxId={c.inbox_id} />}
      <span className="flex min-w-0 flex-1 flex-col gap-[3px]">
        <span className={cn("truncate", c.unread ? "font-semibold" : "font-medium")}>{f?.text || c.subject}</span>
        <span className="flex min-w-0 items-center gap-2 text-caption text-faint">
          <ProductTag id={c.inbox_id} />
          <span className="max-w-32 shrink-0 truncate">{name}</span>
          {where && (
            <>
              <span aria-hidden>·</span>
              <span className="min-w-0 truncate">{where}</span>
            </>
          )}
          {!!f?.attachment_count && (
            <span
              className="inline-flex shrink-0 items-center gap-0.5"
              title={t`Screenshots attached`}
              data-testid="feedback-attachments"
            >
              <PaperclipIcon className="size-3" aria-label={t`Screenshots attached`} />
              {f.attachment_count}
            </span>
          )}
          {answered && (
            <span className="inline-flex shrink-0 items-center gap-0.5 text-success" data-testid="feedback-answered">
              <CheckIcon className="size-3" />
              <Trans>answered</Trans>
            </span>
          )}
          {f?.score ? <ScoreMark score={f.score} /> : null}
          {c.idea && <IdeaMark idea={c.idea} pill className="max-w-48" />}
        </span>
      </span>
      {f?.progress && <ProgressPill progress={f.progress} />}
      {c.unread && (
        <span className="size-2 shrink-0 rounded-full bg-brand" aria-label={t`Unread`} data-testid="row-unread" />
      )}
      <Ago iso={c.last_activity_at} className="w-14 shrink-0 text-end text-caption text-faint" />
    </button>
  )
}

export type FeedbackTab = Tab

export function rememberedFeedbackTab() {
  return rememberedTab
}

export function FeedbackList({
  currentId,
  onOpen,
  embedded = false,
  tab: controlledTab,
  onRows,
}: {
  currentId: string | null
  onOpen: (id: string) => void
  embedded?: boolean
  tab?: Tab
  onRows?: (ids: string[]) => void
}) {
  const { t, i18n } = useLingui()
  const { query, focus } = useListSearch()
  const q = useDebounced(query.trim(), 250)
  const [inboxId] = useInboxFilter()
  const [ownTab, setTabState] = useState<Tab>(rememberedTab)
  const tab = controlledTab ?? ownTab
  const [category, setCategoryState] = useState<Category>(rememberedCategory)
  const search = useRef<HTMLInputElement>(null)
  const scroller = useRef<HTMLDivElement>(null)
  const sentinel = useRef<HTMLDivElement>(null)
  const [nearEnd, setNearEnd] = useState(false)

  const counts = useCounts().data
  const list = useConversations({
    kind: ["feedback"],
    status: tab === "resolved" ? ["closed"] : ["open", "pending"],
    category: category === "all" ? undefined : category,
    inbox_id: inboxId || undefined,
    q: q || undefined,
  })
  const loaded = useMemo(() => list.data?.pages.flatMap((p) => p.items) ?? [], [list.data])
  const rows = loaded
  const { cursorId, setCursorId, moveCursor, openCursor } = useListCursor(rows, currentId, onOpen, true, scroller)
  const ids = rows.map((c) => c.id).join(" ")
  useEffect(() => {
    onRows?.(ids ? ids.split(" ") : [])
  }, [ids, onRows])

  useEffect(() => {
    if (focus <= focused) return
    focused = focus
    requestAnimationFrame(() => search.current?.focus())
  }, [focus])

  useEffect(() => {
    const el = sentinel.current
    const root = scroller.current
    if (!el || !root || typeof IntersectionObserver === "undefined") return
    const io = new IntersectionObserver((entries) => setNearEnd(entries.some((e) => e.isIntersecting)), {
      root,
      rootMargin: "400px",
    })
    io.observe(el)
    return () => io.disconnect()
  }, [])
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = list
  useEffect(() => {
    if (nearEnd && hasNextPage && !isFetchingNextPage) void fetchNextPage()
  }, [nearEnd, hasNextPage, isFetchingNextPage, fetchNextPage, loaded.length])

  const reset = () => {
    setCursorId(null)
    scroller.current?.scrollTo({ top: 0 })
  }
  const [shownTab, setShownTab] = useState(tab)
  if (shownTab !== tab) {
    setShownTab(tab)
    setCursorId(null)
  }
  const setTab = (next: Tab) => {
    remember(next, category)
    setTabState(next)
    reset()
  }
  const setCategory = (next: Category) => {
    remember(tab, next)
    setCategoryState(next)
    reset()
  }

  const fmt = new Intl.NumberFormat(i18n.locale)
  const known = useFeedbackCategories(inboxId || undefined).list
  const open = (k: Category) =>
    k === "all" ? (counts?.feedback ?? 0) : (counts?.feedback_categories.find((x) => x.category === k)?.count ?? 0)
  const showCount = !inboxId && tab === "open"
  const keys = [
    ...known.map((c) => c.key),
    ...(counts?.feedback_categories ?? []).map((x) => x.category).filter((k) => !known.some((c) => c.key === k)),
  ]
  const shownCategories: { key: Category; name: string; icon?: string }[] = [
    { key: "all", name: t`All` },
    ...keys
      .filter((k) => k !== "other" || category === "other" || open("other") > 0)
      .map((k) => known.find((c) => c.key === k) ?? { key: k, name: k, icon: "tag" }),
  ]

  const searchBox = (
    <InputGroup className="h-8 min-w-0 flex-1 rounded-lg border-transparent bg-muted shadow-none focus-within:border-border focus-within:bg-card dark:bg-muted">
      <InputGroupAddon>
        <SearchIcon className="size-[15px] text-faint" />
      </InputGroupAddon>
      <InputGroupInput
        ref={search}
        name="list-search"
        value={query}
        onChange={(e) => {
          setListQuery(e.target.value)
          setCursorId(null)
        }}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault()
            moveCursor(e.key === "ArrowDown" ? 1 : -1)
          } else if (e.key === "Enter") {
            e.preventDefault()
            openCursor()
          } else if (e.key === "Escape") {
            if (query) setListQuery("")
            else e.currentTarget.blur()
          }
        }}
        placeholder={t`Search`}
        aria-label={t`Search feedback`}
        data-testid="list-search"
      />
      <InputGroupAddon align="inline-end">
        {query ? (
          <InputGroupButton size="icon-xs" onClick={() => setListQuery("")} aria-label={t`Clear the search`}>
            <XIcon />
          </InputGroupButton>
        ) : (
          <Kbd className="phone:hidden">/</Kbd>
        )}
      </InputGroupAddon>
    </InputGroup>
  )
  const pills = (
    <div
      className="flex min-w-0 flex-1 flex-wrap gap-1.5"
      role="toolbar"
      aria-label={t`Category`}
      data-testid="feedback-categories"
    >
      {shownCategories.map(({ key: k, name }) => (
        <button
          key={k}
          type="button"
          aria-pressed={category === k}
          onClick={() => setCategory(k)}
          className={cn(
            "inline-flex h-[30px] shrink-0 items-center gap-1 rounded-full border px-3 text-[13px] outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
            category === k
              ? "border-foreground bg-foreground text-background"
              : "border-input text-muted-foreground hover:border-faint hover:text-foreground"
          )}
          data-testid={`feedback-category-${k}`}
        >
          {name}
          {showCount && <span className="tabular-nums opacity-60">{fmt.format(open(k))}</span>}
        </button>
      ))}
    </div>
  )

  return (
    <nav aria-label={t`Feedback`} className="flex h-full min-h-0 flex-col bg-background" data-testid="feedback-list">
      {embedded ? (
        <>
          <div className="flex shrink-0 border-b px-6 py-2.5">{searchBox}</div>
          <div className="flex shrink-0 border-b px-6 py-2.5">{pills}</div>
        </>
      ) : (
        <>
          <ListHead
            title={<Trans>Feedback</Trans>}
            sub={<Trans>Bugs, ideas and praise sent from your apps and pages</Trans>}
          >
            <div data-testid="feedback-tabs">
              <UnderlineTabs
                value={tab}
                onChange={setTab}
                label={t`Status`}
                items={[
                  {
                    value: "open",
                    label: <Trans>Open</Trans>,
                    count: inboxId ? undefined : counts?.feedback,
                    testId: "feedback-tab-open",
                  },
                  { value: "resolved", label: <Trans>Resolved</Trans>, testId: "feedback-tab-resolved" },
                ]}
              />
            </div>
          </ListHead>
          <div className="flex shrink-0 flex-col gap-2 border-b px-3 py-2">
            <div className="flex items-center gap-1.5">
              {searchBox}
              <InboxPicker className="max-w-36" />
            </div>
            {pills}
          </div>
        </>
      )}
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto" data-testid="list">
        {list.isPending ? (
          <div className="grid gap-1 p-3">
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="flex items-center gap-3 py-1.5">
                <Skeleton className="h-5 w-13 rounded-full" />
                <div className="flex flex-1 flex-col gap-2">
                  <Skeleton className="h-4 w-1/2" />
                  <Skeleton className="h-3 w-3/4" />
                </div>
              </div>
            ))}
          </div>
        ) : rows.length === 0 && !hasNextPage ? (
          <p className="px-6 py-10 text-center text-body text-faint" data-testid="list-empty">
            {q ? (
              <Trans>Nothing matches your search here.</Trans>
            ) : tab === "open" ? (
              <Trans>No open feedback.</Trans>
            ) : (
              <Trans>Nothing is resolved yet.</Trans>
            )}
          </p>
        ) : (
          rows.map((c) => (
            <Row key={c.id} c={c} current={c.id === currentId} cursor={c.id === cursorId} onOpen={() => onOpen(c.id)} />
          ))
        )}
        <div ref={sentinel} className="h-px" />
        {isFetchingNextPage && (
          <div className="flex items-center gap-3 px-3 py-2">
            <Skeleton className="h-5 w-13 rounded-full" />
            <Skeleton className="h-4 w-1/2" />
          </div>
        )}
        <ErrorLine error={list.error} className="px-4 py-2" />
      </div>
    </nav>
  )
}

export function EmptyFeedback() {
  const open = useCounts().data?.feedback
  const none = open === 0
  return (
    <div
      className="flex min-h-0 flex-1 flex-col items-center justify-center bg-surface px-8 text-center"
      data-testid="feedback-empty"
    >
      <div className="mb-5 grid size-16 place-items-center rounded-[20px] bg-brand-wash text-brand">
        {none ? <CheckIcon className="size-7" /> : <MessageSquareHeartIcon className="size-7" />}
      </div>
      <h1 className="text-title">
        {none ? <Trans>No open feedback.</Trans> : <Trans>Pick a piece of feedback</Trans>}
      </h1>
      <p className="mt-1.5 text-body text-muted-foreground">
        {none ? (
          <Trans>When someone sends feedback, it will be in the list.</Trans>
        ) : (
          <Trans>Choose one from the list, or move with ↑ ↓ and open it with Enter.</Trans>
        )}
      </p>
    </div>
  )
}

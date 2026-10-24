import { useLingui } from "@lingui/react/macro"
import { useEffect, useRef } from "react"
import { useNavigate } from "react-router"

import { ErrorLine } from "@/components/common"
import { DISCUSSION_KEYS } from "@/components/common/ShortcutSheet"
import { SelectToggle } from "@/components/common/Selection"
import { DiscussionBulkBar } from "@/features/docs/discussions/Bulk"
import { ListSkeleton, LoadMore, ThreadRow } from "@/features/docs/discussions/parts"
import { threadLink, useDiscussions, type ThreadView } from "@/features/docs/discussions/queries"
import { Card, EmptyRow } from "@/features/settings/ui"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useResetState } from "@/hooks/use-reset-state"
import { useSelection } from "@/hooks/use-selection"
import type { Discussion } from "@/lib/api"
import type { DiscussionFilters } from "@/lib/keys"
import { cn } from "@/lib/utils"

export function ThreadList({
  filters,
  showPage,
  from,
  keyboard,
  empty,
  link,
  currentId,
  bare,
  selectable,
}: {
  filters: DiscussionFilters
  showPage: boolean
  from?: string
  keyboard?: boolean
  empty: React.ReactNode
  link?: (d: Discussion) => string
  currentId?: string
  bare?: boolean
  selectable?: boolean
}) {
  const navigate = useNavigate()
  const list = useDiscussions(filters)
  const items = list.data?.pages.flatMap((p) => p.items) ?? []
  const [cursor, setCursor] = useResetState(-1, JSON.stringify(filters))
  const selection = useSelection(
    items.map((d) => d.id),
    JSON.stringify(filters)
  )
  const ref = useRef<HTMLUListElement>(null)
  const down = () => setCursor((i) => Math.min(items.length - 1, i + 1))
  const up = () => setCursor((i) => Math.max(0, i - 1))
  const hrefOf = (d: Discussion) => link?.(d) ?? threadLink(d.id)
  useHotkeys(
    {
      [DISCUSSION_KEYS.next]: down,
      [DISCUSSION_KEYS.previous]: up,
      ArrowDown: down,
      ArrowUp: up,
      Enter: () => {
        const d = items[cursor]
        if (d) navigate(hrefOf(d), { state: from ? { from } : undefined })
      },
    },
    !!keyboard
  )
  useEffect(() => {
    if (cursor >= 0)
      ref.current?.querySelectorAll("[data-testid=thread-row]")[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])
  const Frame = bare ? "div" : Card
  return (
    <Frame {...(bare ? {} : { flush: true })} data-testid="thread-list">
      {selectable && (
        <>
          <DiscussionBulkBar items={items} selection={selection} />
          {!selection.selecting && items.length > 0 && (
            <div className="flex justify-end px-3 pt-2">
              <SelectToggle selection={selection} />
            </div>
          )}
        </>
      )}
      {list.isPending ? (
        <ListSkeleton />
      ) : list.error ? (
        <ErrorLine error={list.error} className="p-4" />
      ) : items.length === 0 ? (
        <EmptyRow>{empty}</EmptyRow>
      ) : (
        <ul ref={ref} className="divide-y">
          {items.map((d, i) => (
            <ThreadRow
              key={d.id}
              d={d}
              cursor={i === cursor}
              showPage={showPage}
              from={from}
              to={hrefOf(d)}
              current={d.id === currentId}
              check={
                selectable && selection.selecting
                  ? { checked: selection.has(d.id), onToggle: (range) => selection.toggle(d.id, range) }
                  : undefined
              }
            />
          ))}
        </ul>
      )}
      <LoadMore list={list} />
    </Frame>
  )
}

export function Chips<T extends string>({
  value,
  onChange,
  items,
  label,
  testId,
}: {
  value: T
  onChange: (v: T) => void
  items: { value: T; label: React.ReactNode; title?: string }[]
  label: string
  testId: string
}) {
  return (
    <div className="flex flex-wrap gap-1.5" role="toolbar" aria-label={label}>
      {items.map((it) => (
        <button
          key={it.value}
          type="button"
          aria-pressed={value === it.value}
          title={it.title}
          onClick={() => onChange(it.value)}
          className={cn(
            "h-7 shrink-0 rounded-full px-3 text-small font-medium transition-colors",
            value === it.value ? "bg-brand-wash text-brand" : "bg-muted text-muted-foreground hover:text-foreground"
          )}
          data-testid={`${testId}-${it.value}`}
        >
          {it.label}
        </button>
      ))}
    </div>
  )
}

export function useViewText(): Record<ThreadView, string> {
  const { t } = useLingui()
  return {
    all: t`All`,
    unanswered: t`Unanswered`,
    pending: t`Waiting for approval`,
    locked: t`Locked`,
    hidden: t`Hidden`,
  }
}

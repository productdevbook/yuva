import { useLingui } from "@lingui/react/macro"
import { useEffect, useRef } from "react"
import { useNavigate } from "react-router"

import { ErrorLine } from "@/components/common"
import { DISCUSSION_KEYS, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { ListBox, ListBoxEmpty, ListBoxSkeleton } from "@/components/common/ListBox"
import { DiscussionBulkBar } from "@/features/docs/discussions/Bulk"
import { LoadMore, ThreadRow } from "@/features/docs/discussions/parts"
import { threadLink, useDiscussions, type ThreadView } from "@/features/docs/discussions/queries"
import { look } from "@/features/inbox/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useResetState } from "@/hooks/use-reset-state"
import { useSelection } from "@/hooks/use-selection"
import type { Discussion } from "@/lib/api"
import type { DiscussionFilters } from "@/lib/keys"

export function ThreadList({
  filters,
  showPage,
  from,
  keyboard,
  empty,
  link,
  currentId,
  selectable,
}: {
  filters: DiscussionFilters
  showPage: boolean
  from?: string
  keyboard?: boolean
  empty: React.ReactNode
  link?: (d: Discussion) => string
  currentId?: string
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
  const ref = useRef<HTMLDivElement>(null)
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
      ...(selectable
        ? {
            [SHORTCUTS.select]: () => items[cursor] && selection.toggle(items[cursor].id),
            ...(selection.selecting ? { Escape: selection.stop } : {}),
          }
        : {}),
    },
    !!keyboard
  )
  useEffect(() => {
    if (cursor >= 0) ref.current?.querySelectorAll("[data-row]")[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])
  return (
    <div ref={ref} data-testid="thread-list">
      <ListBox
        testId="list"
        bar={selectable && selection.selecting && <DiscussionBulkBar items={items} selection={selection} />}
      >
        {list.isPending ? (
          <ListBoxSkeleton rows={3} />
        ) : list.error && items.length === 0 ? (
          <ListBoxEmpty>
            <ErrorLine error={list.error} />
          </ListBoxEmpty>
        ) : items.length === 0 ? (
          <ListBoxEmpty testId="list-empty">{empty}</ListBoxEmpty>
        ) : (
          items.map((d, i) => (
            <ThreadRow
              key={d.id}
              d={d}
              cursor={i === cursor}
              showPage={showPage}
              from={from}
              to={hrefOf(d)}
              current={d.id === currentId}
              check={
                selectable
                  ? {
                      checked: selection.has(d.id),
                      selecting: selection.selecting,
                      onToggle: (range) => selection.toggle(d.id, range),
                    }
                  : undefined
              }
            />
          ))
        )}
      </ListBox>
      <LoadMore list={list} />
    </div>
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
          className={value === it.value ? look.listChipOn : look.listChip}
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

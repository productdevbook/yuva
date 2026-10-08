import { Trans, useLingui } from "@lingui/react/macro"
import { InboxIcon, SearchIcon } from "lucide-react"
import { forwardRef, useCallback, useEffect, useRef, useState } from "react"

import { PaneHeader } from "@/app/shell"
import { EmptyState, ErrorLine, Segmented } from "@/components/common"
import { STATUSES, useEnumText, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { BulkBar, type BulkChange } from "@/features/inbox/BulkBar"
import { ConversationRow } from "@/features/inbox/ConversationRow"
import { FilterMenu } from "@/features/inbox/FilterMenu"
import { useBulkUpdateConversations } from "@/features/inbox/queries"
import { useSelection } from "@/features/inbox/useSelection"
import type { ListBase, ListFilters } from "@/features/inbox/views"
import { ApiError, type ConversationBulkFailure, type ConversationListItem } from "@/lib/api"

type Props = {
  base: ListBase
  title: React.ReactNode
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

function useFailureText() {
  const { t } = useLingui()
  const errorText = useErrorText()
  return (f: ConversationBulkFailure) => {
    if (f.code === "validation_failed") return t`Not changed: the assignee cannot see this conversation's inbox.`
    if (f.code === "not_found") return t`Not changed: it no longer exists or you cannot see it.`
    const reason = errorText(new ApiError(f.status, { code: f.code }))
    return t`Not changed: ${reason}`
  }
}

function SearchField({ value, onChange, inputRef }: { value: string; onChange: (q: string) => void; inputRef: React.Ref<HTMLInputElement> }) {
  const { t } = useLingui()
  const [q, setQ] = useState(value)
  useEffect(() => setQ(value), [value])
  useEffect(() => {
    if (q.trim() === value) return
    const id = setTimeout(() => onChange(q.trim()), 300)
    return () => clearTimeout(id)
  }, [q, value, onChange])
  return (
    <div className="relative">
      <SearchIcon className="pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2 text-faint" />
      <Input
        ref={inputRef}
        type="search"
        value={q}
        onChange={(e) => setQ(e.target.value)}
        onKeyDown={(e) => e.key === "Escape" && e.currentTarget.blur()}
        placeholder={t`Search conversations`}
        aria-label={t`Search conversations`}
        className="rounded-full border-transparent bg-muted ps-9 hover:border-transparent focus-visible:bg-background"
      />
    </div>
  )
}

export const ConversationList = forwardRef<HTMLInputElement, Props>(function ConversationList(p, searchRef) {
  const { t } = useLingui()
  const text = useEnumText()
  const failureText = useFailureText()
  const sentinel = useRef<HTMLDivElement>(null)
  const bulk = useBulkUpdateConversations()
  const ids = p.conversations.map((c) => c.id)
  const sel = useSelection(ids)
  const [failed, setFailed] = useState<ReadonlyMap<string, ConversationBulkFailure>>(new Map())

  const clear = () => {
    sel.set([])
    setFailed(new Map())
    bulk.reset()
  }
  const apply = (change: BulkChange) => {
    setFailed(new Map())
    const chosen = sel.selection
    bulk.mutate(
      { conversation_ids: chosen, ...change },
      {
        onSuccess: (r) => {
          const refused = new Map(r.failed.map((f) => [f.id, f]))
          setFailed(refused)
          sel.set(chosen.filter((id) => refused.has(id)))
        },
      },
    )
  }

  const { setFilters, hasNextPage, isFetchingNextPage, fetchNextPage } = p
  const setQuery = useCallback((q: string) => setFilters({ q }), [setFilters])
  useEffect(() => {
    const el = sentinel.current
    if (!el || !hasNextPage) return
    const io = new IntersectionObserver((entries) => entries[0]?.isIntersecting && !isFetchingNextPage && fetchNextPage())
    io.observe(el)
    return () => io.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  const statuses = [...STATUSES.map((s) => ({ value: s, label: text.status[s] })), { value: "all" as const, label: t`All` }]
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PaneHeader title={p.title}>
        <FilterMenu base={p.base} filters={p.filters} setFilters={setFilters} />
      </PaneHeader>
      <div className="flex shrink-0 flex-col gap-2.5 border-b px-4 pb-3">
        <SearchField value={p.filters.q} onChange={setQuery} inputRef={searchRef} />
        <Segmented
          label={t`Status`}
          value={p.filters.status}
          onChange={(status) => setFilters({ status })}
          items={statuses}
          className="-mx-1"
        />
      </div>
      {sel.selection.length > 0 && (
        <BulkBar
          total={ids.length}
          count={sel.selection.length}
          pending={bulk.isPending}
          error={bulk.error}
          failed={[...failed.keys()].filter((id) => ids.includes(id)).length}
          onSelectAll={(on) => sel.set(on ? ids : [])}
          onClear={clear}
          onApply={apply}
        />
      )}
      <div className="min-h-0 flex-1 overflow-y-auto" data-testid="conversation-list">
        {p.isPending ? (
          <div className="flex flex-col gap-5 p-4">
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="flex gap-3">
                <Skeleton className="size-9 rounded-full" />
                <div className="flex flex-1 flex-col gap-2">
                  <Skeleton className="h-3.5 w-1/2" />
                  <Skeleton className="h-3.5 w-4/5" />
                </div>
              </div>
            ))}
          </div>
        ) : p.error ? (
          <ErrorLine error={p.error} className="p-4" />
        ) : p.conversations.length === 0 ? (
          <EmptyState icon={InboxIcon} title={<Trans>No conversations here</Trans>} className="py-16">
            {p.filters.q ? <Trans>Nothing matches your search.</Trans> : <Trans>New conversations show up here.</Trans>}
          </EmptyState>
        ) : (
          <ul className="divide-y">
            {p.conversations.map((c) => (
              <ConversationRow
                key={c.id}
                c={c}
                href={p.hrefFor(c.id)}
                selected={c.id === p.selectedId}
                showStatus={p.filters.status === "all"}
                showInbox={p.base.kind !== "inbox"}
                picked={sel.picked.has(c.id)}
                selecting={sel.selection.length > 0}
                onPick={(on, range) => sel.toggle(c.id, on, range)}
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

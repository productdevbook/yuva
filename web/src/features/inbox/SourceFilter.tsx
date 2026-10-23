import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ChevronDownIcon, ListFilterIcon, MegaphoneIcon } from "lucide-react"
import { useMemo, useSyncExternalStore } from "react"

import { hostOf, SOURCE_FILTERS, SourceIcon, useSourceText } from "@/components/common"
import { popupClass } from "@/components/ui/dropdown-menu"
import { Item } from "@/components/ui/item"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Separator } from "@/components/ui/separator"
import { look } from "@/features/inbox/PageFrame"
import { FieldFilterSection, fieldFilterText } from "@/features/fields/FieldFilter"
import { useConversationFieldList } from "@/features/fields/ConversationFields"
import { useBroadcasts } from "@/features/broadcasts/broadcastQueries"
import { useDocsSites } from "@/features/docs/queries"
import type { ChannelKind } from "@/lib/api"
import type { ConversationFilters } from "@/lib/keys"
import { cn } from "@/lib/utils"
import { useChannelMap } from "@/lib/workspace"

export type FieldFilterValue = { key: string; value: string }

export type SourceFilterValue = {
  source?: ChannelKind
  site?: string
  broadcast?: { id: string; name: string }
  field?: FieldFilterValue
}

let value: SourceFilterValue = {}
const listeners = new Set<() => void>()

export function useSourceFilter() {
  const v = useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => value
  )
  const set = (next: SourceFilterValue) => {
    value = next
    listeners.forEach((l) => l())
  }
  return [v, set] as const
}

export function sourceFilters(v: SourceFilterValue): ConversationFilters {
  const f: ConversationFilters = {}
  if (v.source) {
    f.kind = ["conversation"]
    f.channel_kind = [v.source]
  }
  if (v.site) f.site = v.site
  if (v.broadcast) f.broadcast_id = v.broadcast.id
  if (v.field) f.fields = { [v.field.key]: v.field.value }
  return f
}

export type Site = { origin: string; name?: string }

export function useSites(inboxId: string): Site[] {
  const docs = useDocsSites(inboxId, 90).data?.items
  const channels = useChannelMap()
  return useMemo(() => {
    const byOrigin = new Map<string, Site>()
    for (const s of docs ?? []) {
      const known = byOrigin.get(s.origin)
      if (!known?.name) byOrigin.set(s.origin, { origin: s.origin, name: s.channel_name })
    }
    for (const ch of channels.values()) {
      if (ch.kind !== "chat" || (inboxId && ch.inbox_id !== inboxId)) continue
      for (const o of ch.chat?.allowed_origins ?? [])
        if (!byOrigin.get(o)?.name) byOrigin.set(o, { origin: o, name: ch.name })
    }
    return [...byOrigin.values()].sort((a, b) => hostOf(a.origin).localeCompare(hostOf(b.origin)))
  }, [docs, channels, inboxId])
}

function Choice({
  active,
  onClick,
  children,
  testId,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
  testId?: string
}) {
  return (
    <Item
      render={<button type="button" />}
      size="xs"
      role="menuitemradio"
      aria-checked={active}
      className={cn("flex-nowrap", active && "bg-muted")}
      onClick={onClick}
      data-testid={testId}
    >
      {children}
      <CheckIcon className={cn("ms-auto size-4 shrink-0 text-brand", !active && "invisible")} />
    </Item>
  )
}

function BroadcastChoices({
  inboxId,
  v,
  update,
}: {
  inboxId: string
  v: SourceFilterValue
  update: (next: SourceFilterValue) => void
}) {
  const list = useBroadcasts(inboxId, ["sending", "paused", "sent", "cancelled"])
  const items = (list.data?.pages.flatMap((p) => p.items) ?? []).slice(0, 8).map((b) => ({ id: b.id, name: b.name }))
  if (v.broadcast && !items.some((b) => b.id === v.broadcast?.id)) items.unshift(v.broadcast)
  if (items.length === 0) return null
  return (
    <>
      <Separator className="mx-1.5 my-1 w-auto" />
      <p className="px-2.5 pt-1.5 pb-1 text-caption font-medium text-faint">
        <Trans>Replies to a broadcast</Trans>
      </p>
      <Choice active={!v.broadcast} onClick={() => update({ ...v, broadcast: undefined })} testId="broadcast-any">
        <span className="flex-1 text-start">
          <Trans>Any conversation</Trans>
        </span>
      </Choice>
      {items.map((b) => (
        <Choice
          key={b.id}
          active={v.broadcast?.id === b.id}
          onClick={() => update({ ...v, broadcast: b })}
          testId="broadcast-option"
        >
          <MegaphoneIcon className="size-4 text-muted-foreground" />
          <span className="min-w-0 flex-1 truncate text-start">{b.name}</span>
        </Choice>
      ))}
    </>
  )
}

export function SourceFilter({
  inboxId,
  onChange,
  placeholder,
}: {
  inboxId: string
  onChange: () => void
  placeholder?: React.ReactNode
}) {
  const { t } = useLingui()
  const text = useSourceText()
  const [v, set] = useSourceFilter()
  const sites = useSites(inboxId)
  const fields = useConversationFieldList()
  const update = (next: SourceFilterValue) => {
    set(next)
    onChange()
  }
  const active = !!v.source || !!v.site || !!v.broadcast || !!v.field
  const picked = v.field && fields.find((f) => f.key === v.field?.key)
  const summary = [
    v.source && text[v.source],
    v.site && hostOf(v.site),
    v.broadcast?.name,
    picked && v.field && fieldFilterText(picked, v.field.value, t`Yes`, t`No`),
  ]
    .filter(Boolean)
    .join(" · ")
  return (
    <Popover>
      <PopoverTrigger
        render={
          <button
            type="button"
            className={
              placeholder
                ? cn(active ? look.chipOn : look.chip, "min-w-0 max-w-44")
                : cn(
                    "inline-flex h-8 max-w-40 min-w-0 shrink items-center gap-1.5 rounded-lg border px-2.5 text-small transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring",
                    active
                      ? "border-brand/40 bg-brand-wash font-medium text-brand"
                      : "border-dashed border-input text-muted-foreground hover:border-faint hover:text-foreground"
                  )
            }
          />
        }
        aria-label={t`Filter by type, site and broadcast`}
        title={t`Filter by type, site and broadcast`}
        data-testid="source-filter"
      >
        <ListFilterIcon className="size-3.5 shrink-0" />
        <span className="truncate">{active ? summary : (placeholder ?? <Trans>Filter</Trans>)}</span>
        {placeholder && <ChevronDownIcon className="size-3.5 shrink-0 text-faint" />}
      </PopoverTrigger>
      <PopoverContent
        align="start"
        sideOffset={6}
        className={cn(
          popupClass,
          "max-h-[70svh] w-[280px] max-w-[calc(100vw-32px)] gap-0 overflow-y-auto rounded-xl p-1.5 ring-0"
        )}
        data-testid="source-filter-popup"
      >
        <p className="px-2.5 pt-1.5 pb-1 text-caption font-medium text-faint">
          <Trans>Type</Trans>
        </p>
        <Choice active={!v.source} onClick={() => update({ ...v, source: undefined })} testId="source-any">
          <span className="flex-1 text-start">
            <Trans>Every type</Trans>
          </span>
        </Choice>
        {SOURCE_FILTERS.map((s) => (
          <Choice key={s} active={v.source === s} onClick={() => update({ ...v, source: s })} testId={`source-${s}`}>
            <SourceIcon source={s} className="size-4 text-muted-foreground" />
            <span className="flex-1 text-start">{text[s]}</span>
          </Choice>
        ))}
        {sites.length > 0 && (
          <>
            <Separator className="mx-1.5 my-1 w-auto" />
            <p className="px-2.5 pt-1.5 pb-1 text-caption font-medium text-faint">
              <Trans>Site</Trans>
            </p>
            <Choice active={!v.site} onClick={() => update({ ...v, site: undefined })} testId="site-any">
              <span className="flex-1 text-start">
                <Trans>Every site</Trans>
              </span>
            </Choice>
            {sites.map((s) => (
              <Choice
                key={s.origin}
                active={v.site === s.origin}
                onClick={() => update({ ...v, site: s.origin })}
                testId="site-option"
              >
                <span className="min-w-0 flex-1 text-start">
                  <span className="block truncate">{s.name || hostOf(s.origin)}</span>
                  {s.name && <small className="block truncate text-caption text-faint">{hostOf(s.origin)}</small>}
                </span>
              </Choice>
            ))}
            <p className="px-2.5 pt-1 pb-1.5 text-caption text-faint">
              <Trans>A site shows the conversations sent from its pages.</Trans>
            </p>
          </>
        )}
        <BroadcastChoices inboxId={inboxId} v={v} update={update} />
        <FieldFilterSection fields={fields} value={v.field} onChange={(field) => update({ ...v, field })} />
      </PopoverContent>
    </Popover>
  )
}

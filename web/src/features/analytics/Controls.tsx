import { Trans, useLingui } from "@lingui/react/macro"
import {
  BookmarkIcon,
  CalendarRangeIcon,
  ChevronDownIcon,
  ChevronsUpDownIcon,
  GitCompareArrowsIcon,
  PencilIcon,
  SaveIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { formatDate } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useDeleteSegment, useSaveSegment, useSegments } from "@/features/analytics/queries"
import { OPS, PERIODS, sameFilters, todayIso, useView, type FilterOp, type Period } from "@/features/analytics/state"
import { flag, useAnalyticsText, valueText, type AnalyticsText } from "@/features/analytics/text"
import { Field } from "@/features/settings/ui"
import { ApiError, type AnalyticsFilter, type AnalyticsSegment } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const CUSTOM = "custom"

function RangeDialog({
  open,
  onOpenChange,
  title,
  initial,
  onPick,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: React.ReactNode
  initial: { from: string; to: string }
  onPick: (r: { from: string; to: string }) => void
}) {
  const [from, setFrom] = useState(initial.from)
  const [to, setTo] = useState(initial.to)
  const valid = !!from && !!to && from <= to
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="analytics-range-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (!valid) return
            onPick({ from, to })
            onOpenChange(false)
          }}
        >
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
          </DialogHeader>
          <div className="grid grid-cols-2 gap-3">
            <div className="flex flex-col gap-2">
              <Label htmlFor="range-from">
                <Trans>From</Trans>
              </Label>
              <Input id="range-from" type="date" value={from} max={to} onChange={(e) => setFrom(e.target.value)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="range-to">
                <Trans>To</Trans>
              </Label>
              <Input id="range-to" type="date" value={to} min={from} onChange={(e) => setTo(e.target.value)} />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!valid}>
              <Trans>Show</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function daysAgo(n: number) {
  const d = new Date(Date.parse(`${todayIso()}T00:00:00Z`) - n * 86_400_000)
  return d.toISOString().slice(0, 10)
}

export function RangePicker({ className, plain }: { className?: string; plain?: boolean } = {}) {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const { view, setRange } = useView()
  const [custom, setCustom] = useState(false)
  const r = view.range
  const customLabel = r.period
    ? t`Custom range`
    : `${formatDate(`${r.from}T12:00:00Z`, i18n.locale)} – ${formatDate(`${r.to}T12:00:00Z`, i18n.locale)}`
  const items: Record<string, string> = { ...text.period, [CUSTOM]: customLabel }
  return (
    <>
      <Select
        value={r.period ?? CUSTOM}
        items={items}
        onValueChange={(v) => {
          if (v === CUSTOM) setCustom(true)
          else if (v) setRange({ period: v as Period })
        }}
      >
        <SelectTrigger
          aria-label={t`Range`}
          className={cn("max-w-full min-w-0", className)}
          data-testid="analytics-range"
        >
          {!plain && <CalendarRangeIcon className="text-faint" />}
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="min-w-52">
          {PERIODS.map((p) => (
            <SelectItem key={p} value={p}>
              {text.period[p]}
            </SelectItem>
          ))}
          <SelectItem value={CUSTOM} onClick={() => setCustom(true)}>
            <Trans>Custom range…</Trans>
          </SelectItem>
        </SelectContent>
      </Select>
      {custom && (
        <RangeDialog
          open
          onOpenChange={setCustom}
          title={<Trans>Show the days</Trans>}
          initial={r.period ? { from: daysAgo(29), to: todayIso() } : { from: r.from!, to: r.to! }}
          onPick={setRange}
        />
      )}
    </>
  )
}

export function ComparePicker({ className }: { className?: string } = {}) {
  const { t, i18n } = useLingui()
  const { view, setCompare } = useView()
  const [custom, setCustom] = useState(false)
  const c = view.compare
  const value = c?.with ?? "none"
  const items: Record<string, string> = {
    none: t`No comparison`,
    previous: t`Compare with the period before`,
    year: t`Compare with a year earlier`,
    range: c?.from
      ? t`Compare with ${formatDate(`${c.from}T12:00:00Z`, i18n.locale)} – ${formatDate(`${c.to}T12:00:00Z`, i18n.locale)}`
      : t`Compare with other days`,
  }
  return (
    <>
      <Select
        value={value}
        items={items}
        onValueChange={(v) => {
          if (v === "range") setCustom(true)
          else setCompare(v === "previous" || v === "year" ? { with: v } : undefined)
        }}
      >
        <SelectTrigger
          aria-label={t`Comparison`}
          className={cn("max-w-full min-w-0", className)}
          data-testid="analytics-compare"
        >
          <GitCompareArrowsIcon className="text-faint" />
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="min-w-60">
          <SelectItem value="none">{items.none}</SelectItem>
          <SelectItem value="previous">{items.previous}</SelectItem>
          <SelectItem value="year">{items.year}</SelectItem>
          <SelectItem value="range" onClick={() => setCustom(true)}>
            <Trans>Compare with other days…</Trans>
          </SelectItem>
        </SelectContent>
      </Select>
      {custom && (
        <RangeDialog
          open
          onOpenChange={setCustom}
          title={<Trans>Compare with the days</Trans>}
          initial={c?.from && c.to ? { from: c.from, to: c.to } : { from: daysAgo(59), to: daysAgo(30) }}
          onPick={(r) => setCompare({ with: "range", ...r })}
        />
      )}
    </>
  )
}

function filterName(text: AnalyticsText, f: AnalyticsFilter) {
  return f.dimension === "prop" && f.prop ? f.prop : text.dimension[f.dimension]
}

export function filterLabel(text: AnalyticsText, f: AnalyticsFilter, locale: string) {
  return `${filterName(text, f)} ${text.op[f.op]} ${valueText(text, f.dimension, f.values[0] ?? "", locale)}`
}

function FilterChip({ f, i }: { f: AnalyticsFilter; i: number }) {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const { setFilter, removeFilter } = useView()
  const value = f.values[0] ?? ""
  const shown = valueText(text, f.dimension, value, i18n.locale)
  const label = filterLabel(text, f, i18n.locale)
  return (
    <li
      className="flex h-[30px] shrink-0 items-center rounded-lg border border-brand/30 bg-brand-wash ps-1 text-small"
      data-testid="analytics-filter"
    >
      <DropdownMenu>
        <DropdownMenuTrigger
          className="flex h-full max-w-72 items-center gap-1 rounded-lg ps-2 pe-1 outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
          aria-label={t`Change filter: ${label}`}
        >
          <span className="truncate text-muted-foreground" dir="auto">
            {filterName(text, f)}
          </span>
          <span className="text-muted-foreground">{text.op[f.op]}</span>
          <span className="truncate font-medium" dir="auto">
            {f.dimension === "country" && flag(value)} {shown}
          </span>
          <ChevronDownIcon className="size-3.5 shrink-0 text-faint" />
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuRadioGroup value={f.op} onValueChange={(op) => setFilter(i, { ...f, op: op as FilterOp })}>
            {OPS.map((op) => (
              <DropdownMenuRadioItem key={op} value={op}>
                {text.op[op]}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => removeFilter(i)}>
            <XIcon />
            <Trans>Remove filter</Trans>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <button
        type="button"
        onClick={() => removeFilter(i)}
        className="grid size-[22px] place-items-center rounded-md text-faint outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        aria-label={t`Remove filter: ${label}`}
      >
        <XIcon className="size-3.5" />
      </button>
    </li>
  )
}

export function FilterBar() {
  const { t } = useLingui()
  const { view, clearFilters } = useView()
  if (view.filters.length === 0) return null
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2" aria-label={t`Filters`} role="group">
      <ul className="flex flex-wrap items-center gap-1.5">
        {view.filters.map((f, i) => (
          <FilterChip key={`${f.dimension}${i}`} f={f} i={i} />
        ))}
      </ul>
      {view.filters.length > 1 && (
        <Button
          variant="ghost"
          size="sm"
          className="shrink-0"
          onClick={clearFilters}
          data-testid="analytics-clear-filters"
        >
          <Trans>Clear</Trans>
        </Button>
      )}
    </div>
  )
}

function SegmentDialog({
  propertyId,
  segment,
  filters,
  onClose,
  onSaved,
}: {
  propertyId: string
  segment?: AnalyticsSegment
  filters: AnalyticsFilter[]
  onClose: () => void
  onSaved: (s: AnalyticsSegment) => void
}) {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const save = useSaveSegment(propertyId)
  const [name, setName] = useState(segment?.name ?? "")
  const taken = save.error instanceof ApiError && save.error.code === "segment_taken"
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="segment-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (!name.trim()) return
            save.mutate(segment ? { segmentId: segment.id, name: name.trim() } : { name: name.trim(), filters }, {
              onSuccess: onSaved,
            })
          }}
        >
          <DialogHeader>
            <DialogTitle>
              {segment ? <Trans>Rename segment</Trans> : <Trans>Save these filters as a segment</Trans>}
            </DialogTitle>
          </DialogHeader>
          <Field
            label={<Trans>Name</Trans>}
            htmlFor="segment-name"
            error={taken ? <Trans>This property already has a segment with this name.</Trans> : undefined}
            hint={
              segment ? undefined : (
                <Trans>Everyone who sees this property can pick it. You, owners and admins can change it.</Trans>
              )
            }
          >
            <Input
              id="segment-name"
              required
              maxLength={100}
              value={name}
              placeholder={t`Paying customers from Germany`}
              onChange={(e) => setName(e.target.value)}
              data-testid="segment-name"
            />
          </Field>
          {!segment && (
            <ul className="flex flex-wrap gap-1.5" aria-label={t`Filters`}>
              {filters.map((f, i) => (
                <li key={i} className="rounded-full border bg-card px-2.5 py-0.5 text-small" dir="auto">
                  {filterLabel(text, f, i18n.locale)}
                </li>
              ))}
            </ul>
          )}
          {!taken && <ErrorLine error={save.error} />}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!name.trim() || save.isPending} data-testid="segment-save">
              <Trans>Save</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function SegmentPicker({ propertyId, className }: { propertyId: string; className?: string }) {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const { canManage, membership } = useSession()
  const segments = useSegments(propertyId)
  const { view, setSegment } = useView()
  const save = useSaveSegment(propertyId)
  const remove = useDeleteSegment(propertyId)
  const [ask, confirm] = useConfirm()
  const [dialog, setDialog] = useState<"new" | "rename" | null>(null)
  const items = segments.data ?? []
  const current = items.find((s) => s.id === view.segment)
  const changed = !!current && !sameFilters(current.filters, view.filters)
  const editable = !!current && (canManage || current.created_by === membership.member_id)
  const summary = (s: AnalyticsSegment) => s.filters.map((f) => filterLabel(text, f, i18n.locale)).join(", ")
  if (items.length === 0 && view.filters.length === 0) return null
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          className={cn(
            "flex h-[30px] max-w-full min-w-0 items-center gap-2 rounded-lg border border-input bg-background ps-3 pe-2.5 text-body whitespace-nowrap transition-colors outline-none hover:border-faint focus-visible:ring-2 focus-visible:ring-ring",
            className
          )}
          aria-label={t`Segment`}
          data-testid="analytics-segment"
        >
          <BookmarkIcon className="size-4 shrink-0 text-faint" />
          <span className="min-w-0 truncate">{current?.name ?? t`Segments`}</span>
          {changed && <span className="shrink-0 text-caption text-faint">{t`changed`}</span>}
          <ChevronsUpDownIcon className="size-3.5 shrink-0 text-faint" />
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-72 max-w-[calc(100vw-32px)]">
          {items.length > 0 && (
            <DropdownMenuGroup>
              <DropdownMenuLabel>
                <Trans>Saved segments</Trans>
              </DropdownMenuLabel>
              <DropdownMenuRadioGroup
                value={view.segment ?? ""}
                onValueChange={(v) => {
                  const s = items.find((x) => x.id === v)
                  setSegment(s?.id, s?.filters ?? [])
                }}
              >
                <DropdownMenuRadioItem value="">
                  <Trans>No segment</Trans>
                </DropdownMenuRadioItem>
                {items.map((s) => (
                  <DropdownMenuRadioItem key={s.id} value={s.id} data-testid="segment-item">
                    <span className="flex min-w-0 flex-col">
                      <span className="truncate">{s.name}</span>
                      <span className="truncate text-caption text-faint" dir="auto">
                        {summary(s)}
                      </span>
                    </span>
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuGroup>
          )}
          {items.length > 0 && <DropdownMenuSeparator />}
          {current && editable && changed && view.filters.length > 0 && (
            <DropdownMenuItem
              onClick={() => save.mutate({ segmentId: current.id, filters: view.filters })}
              data-testid="segment-update"
            >
              <SaveIcon />
              <Trans>Save the changes to “{current.name}”</Trans>
            </DropdownMenuItem>
          )}
          <DropdownMenuItem
            disabled={view.filters.length === 0}
            onClick={() => setDialog("new")}
            data-testid="segment-new"
          >
            <BookmarkIcon />
            <Trans>Save the filters as a new segment…</Trans>
          </DropdownMenuItem>
          {current && editable && (
            <>
              <DropdownMenuItem onClick={() => setDialog("rename")} data-testid="segment-rename">
                <PencilIcon />
                <Trans>Rename “{current.name}”…</Trans>
              </DropdownMenuItem>
              <DropdownMenuItem
                variant="destructive"
                onClick={() =>
                  ask({
                    title: <Trans>Delete the segment “{current.name}”?</Trans>,
                    description: <Trans>It is gone for everyone who uses it. The numbers stay.</Trans>,
                    confirm: <Trans>Delete segment</Trans>,
                    run: () => remove.mutate(current.id, { onSuccess: () => setSegment(undefined, view.filters) }),
                  })
                }
                data-testid="segment-delete"
              >
                <Trash2Icon />
                <Trans>Delete segment</Trans>
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      <ErrorLine error={save.error ?? remove.error} className="text-small" />
      {dialog && (
        <SegmentDialog
          propertyId={propertyId}
          segment={dialog === "rename" ? current : undefined}
          filters={view.filters}
          onClose={() => setDialog(null)}
          onSaved={(s) => {
            setDialog(null)
            if (dialog === "new") setSegment(s.id, s.filters)
          }}
        />
      )}
      {confirm}
    </>
  )
}

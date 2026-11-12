import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery } from "@tanstack/react-query"
import { DownloadIcon, HistoryIcon, UserCheckIcon } from "lucide-react"
import { useOutletContext } from "react-router"

import { chartDots, ErrorLine, Notice } from "@/components/common"
import { formatDate } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { useGeoAttribution, useShare } from "@/features/analytics/queries"
import { todayIso, useView } from "@/features/analytics/state"
import { api, ApiError, unwrap, type AnalyticsProperty } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useOptionalSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export function useCurrentProperty() {
  return useOutletContext<{ property: AnalyticsProperty }>().property
}

export type BarItem = {
  key: string
  label: React.ReactNode
  value: number
  display: string
  onClick?: () => void
  ltr?: boolean
}

export function Meter({ value, max }: { value: number; max: number }) {
  return (
    <span aria-hidden className="h-2 min-w-10 flex-1 overflow-hidden rounded-full bg-seg">
      <span
        className="block h-full rounded-full bg-brand"
        style={{ width: `${Math.max(1.5, (value / Math.max(1, max)) * 100)}%` }}
      />
    </span>
  )
}

export const meterRowClass =
  "flex h-10 w-full min-w-0 items-center gap-3 border-t border-line-soft px-4 text-body text-foreground outline-none first:border-t-0"

export function MeterRow({
  label,
  value,
  max,
  display,
  onClick,
  ltr,
  mono,
}: {
  label: React.ReactNode
  value: number
  max: number
  display: React.ReactNode
  onClick?: () => void
  ltr?: boolean
  mono?: boolean
}) {
  const inner = (
    <>
      <span
        className={cn("min-w-0 flex-[0_1_45%] truncate text-start", mono && "font-mono text-[13px]")}
        dir={ltr ? "ltr" : undefined}
      >
        {label}
      </span>
      <Meter value={value} max={max} />
      <span className="shrink-0 text-end font-mono text-xs tabular-nums">{display}</span>
    </>
  )
  return onClick ? (
    <button
      type="button"
      onClick={onClick}
      className={cn(meterRowClass, "transition-colors hover:bg-subtle focus-visible:bg-subtle")}
      data-testid="analytics-bar"
    >
      {inner}
    </button>
  ) : (
    <div className={meterRowClass} data-testid="analytics-bar">
      {inner}
    </div>
  )
}

export function BarList({
  items,
  head,
  testId,
}: {
  items: BarItem[]
  head?: [React.ReactNode, React.ReactNode]
  testId?: string
}) {
  const max = Math.max(1, ...items.map((i) => i.value))
  return (
    <div className="-mx-3 -my-3 flex flex-col" data-testid={testId}>
      {head && (
        <div className="flex justify-between border-b px-4 py-2 text-caption font-medium text-muted2">
          <span>{head[0]}</span>
          <span>{head[1]}</span>
        </div>
      )}
      <ul className="flex flex-col">
        {items.map((it) => (
          <li key={it.key} className="border-t border-line-soft first:border-t-0 [&>*]:border-t-0">
            <MeterRow
              label={it.label}
              value={it.value}
              max={max}
              display={it.display}
              onClick={it.onClick}
              ltr={it.ltr}
            />
          </li>
        ))}
      </ul>
    </div>
  )
}

export function Attribution({ propertyId, className }: { propertyId: string; className?: string }) {
  const a = useGeoAttribution(propertyId)
  if (!a) return null
  return (
    <p className={cn("text-caption text-faint", className)} data-testid="geoip-attribution">
      {a.url ? (
        <a href={a.url} target="_blank" rel="noreferrer" className="underline-offset-4 hover:underline">
          {a.text}
        </a>
      ) : (
        a.text
      )}
    </p>
  )
}

export function CsvButton({ run, testId }: { run: () => Promise<void>; testId?: string }) {
  const { t } = useLingui()
  const save = useMutation({ mutationFn: run })
  const share = useShare()
  if (share) return null
  return (
    <span className="inline-flex items-center gap-2">
      <ErrorLine error={save.error} className="text-small" />
      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={save.isPending}
        onClick={() => save.mutate()}
        title={t`Download CSV`}
        data-testid={testId}
      >
        <DownloadIcon />
        CSV
      </Button>
    </span>
  )
}

export function QueryProblem({ error }: { error: unknown }) {
  const { i18n } = useLingui()
  const { setRange } = useView()
  if (!error) return null
  if (error instanceof ApiError && error.code === "needs_raw_window") {
    const from = (error.body as { raw_window_from?: string } | undefined)?.raw_window_from
    const day = from ? formatDate(`${from}T12:00:00Z`, i18n.locale) : ""
    return (
      <Notice tone="warning" icon={HistoryIcon} data-testid="needs-raw-window">
        <p>
          {from ? (
            <Trans>
              These filters need the detailed records, which are kept since {day}. Earlier days only have totals per
              page, source, place and device.
            </Trans>
          ) : (
            <Trans>These filters need detailed records that are no longer kept for this range.</Trans>
          )}
        </p>
        {from && (
          <Button
            variant="outline"
            size="sm"
            className="w-fit"
            onClick={() => setRange({ from, to: todayIso() })}
            data-testid="narrow-range"
          >
            <Trans>Show from {day}</Trans>
          </Button>
        )}
      </Notice>
    )
  }
  if (error instanceof ApiError && error.code === "range_too_large") {
    return (
      <Notice tone="warning" icon={HistoryIcon}>
        <Trans>A range can be at most 400 days long. Pick a shorter one.</Trans>
      </Notice>
    )
  }
  return <ErrorLine error={error} />
}

export function PanelProblem({ error }: { error: unknown }) {
  if (error instanceof ApiError && (error.code === "needs_raw_window" || error.code === "range_too_large")) {
    return (
      <p className="px-2 py-6 text-center text-body text-faint">
        <Trans>Not available for this range.</Trans>
      </p>
    )
  }
  return <ErrorLine error={error} className="px-2 py-4" />
}

export function Panel({
  title,
  action,
  children,
  footer,
  className,
  dotted,
  testId,
}: {
  title?: React.ReactNode
  action?: React.ReactNode
  children: React.ReactNode
  footer?: React.ReactNode
  className?: string
  dotted?: boolean
  testId?: string
}) {
  return (
    <section className={cn("flex min-w-0 flex-col", className)} data-testid={testId}>
      {(title || action) && (
        <header className="flex min-h-8 flex-wrap items-center gap-x-3 gap-y-1 pb-2.5">
          {title && <h2 className="m-0 min-w-0 flex-1 text-base font-semibold tracking-[-0.01em]">{title}</h2>}
          {action}
        </header>
      )}
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden rounded-[10px] border bg-background">
        <div className={cn("flex min-w-0 flex-1 flex-col px-3 py-3", dotted && cn(chartDots, "px-4 pt-5"))}>
          {children}
        </div>
        {footer && <div className="flex flex-wrap items-center gap-2 border-t px-4 py-2">{footer}</div>}
      </div>
    </section>
  )
}

export function ConsentedOnly({ className }: { className?: string }) {
  const { t } = useLingui()
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full bg-muted px-2 py-px text-caption font-normal text-muted-foreground",
        className
      )}
      title={t`Only visitors who agreed to a persistent id are counted here. Everyone else is counted one day at a time.`}
      data-testid="consented-only"
    >
      <UserCheckIcon className="size-3 shrink-0" />
      <Trans>Consented visitors only</Trans>
    </span>
  )
}

export function offersPersistent(p: Pick<AnalyticsProperty, "consent_features">) {
  return p.consent_features.includes("persistent")
}

export function usePropertyInbox(inboxId?: string) {
  const ws = useOptionalSession()?.workspaceId ?? ""
  const inboxes = useQuery({
    queryKey: keys.inboxes(ws),
    queryFn: () => unwrap(api.GET("/v1/inboxes")).then((r) => r.items),
    staleTime: 60_000,
    enabled: !!ws && !!inboxId,
  })
  return inboxId ? inboxes.data?.find((i) => i.id === inboxId) : undefined
}

import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery } from "@tanstack/react-query"
import { DownloadIcon, HistoryIcon, UserCheckIcon } from "lucide-react"
import { useOutletContext } from "react-router"

import { ErrorLine, Notice } from "@/components/common"
import { formatDate } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { useGeoAttribution, useShare } from "@/features/analytics/queries"
import { todayIso, useView } from "@/features/analytics/state"
import { api, ApiError, unwrap, type AnalyticsProperty } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useOptionalSession } from "@/lib/session"
import { look } from "@/features/inbox/PageFrame"
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
    <div className="flex flex-col" data-testid={testId}>
      {head && (
        <div className={cn(look.over, "flex justify-between px-2 pb-1.5")}>
          <span>{head[0]}</span>
          <span>{head[1]}</span>
        </div>
      )}
      <ul className="flex flex-col gap-0.5">
        {items.map((it) => {
          const inner = (
            <>
              <span
                aria-hidden
                className="absolute inset-y-[3px] start-0 rounded-md bg-brand/10"
                style={{ width: `${Math.max(1.5, (it.value / max) * 100)}%` }}
              />
              <span className="relative min-w-0 flex-1 truncate text-start" dir={it.ltr ? "ltr" : undefined}>
                {it.label}
              </span>
              <span className="relative shrink-0 text-body font-semibold tabular-nums">{it.display}</span>
            </>
          )
          const cls =
            "relative flex h-[34px] w-full items-center gap-3 rounded-md px-2 text-body outline-none focus-visible:ring-2 focus-visible:ring-ring"
          return (
            <li key={it.key}>
              {it.onClick ? (
                <button
                  type="button"
                  onClick={it.onClick}
                  className={cn(cls, "transition-colors hover:bg-muted")}
                  data-testid="analytics-bar"
                >
                  {inner}
                </button>
              ) : (
                <div className={cls} data-testid="analytics-bar">
                  {inner}
                </div>
              )}
            </li>
          )
        })}
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
  testId,
}: {
  title?: React.ReactNode
  action?: React.ReactNode
  children: React.ReactNode
  footer?: React.ReactNode
  className?: string
  testId?: string
}) {
  return (
    <section className={cn("flex min-w-0 flex-col rounded-[14px] border bg-card", className)} data-testid={testId}>
      {(title || action) && (
        <header className="flex min-h-12 flex-wrap items-center gap-2 px-4 pt-2.5 pb-1">
          {title && <h2 className="min-w-0 flex-1 text-[14px] leading-5 font-semibold">{title}</h2>}
          {action}
        </header>
      )}
      <div className="flex min-w-0 flex-1 flex-col px-3 pb-3">{children}</div>
      {footer && <div className="flex flex-wrap items-center gap-2 border-t px-4 py-2">{footer}</div>}
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
